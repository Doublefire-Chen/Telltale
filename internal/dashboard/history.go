package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

// Defaults for how often buffered Reports are written, how often history is
// downsampled, and how long each tier is kept.
const (
	defaultFlushInterval      = 10 * time.Second
	defaultDownsampleInterval = time.Minute
)

// DefaultRetention keeps raw Reports for an hour, 1-minute averages for a
// week and 15-minute averages for 90 days.
var DefaultRetention = store.Retention{Raw: time.Hour, Minute: 7 * 24 * time.Hour, Quarter: 90 * 24 * time.Hour}

// historyRanges maps each chart range to the tier that serves it.
var historyRanges = map[string]struct {
	span time.Duration
	tier store.Tier
}{
	"1h":  {time.Hour, store.TierRaw},
	"24h": {24 * time.Hour, store.TierMinute},
	"7d":  {7 * 24 * time.Hour, store.TierMinute},
	"30d": {30 * 24 * time.Hour, store.TierQuarter},
}

// recorder buffers Reports in memory until the next flush, so the database
// sees one transaction every few seconds rather than one write per Report.
type recorder struct {
	mu      sync.Mutex
	samples []store.Sample
	latest  map[string][]byte
	// downsampledAt is when downsampling last ran (zero: never, so the
	// first run covers everything still in the raw tier), and flushedSince
	// the oldest sample time (Unix ms) written since then (0: none).
	downsampledAt time.Time
	flushedSince  int64
}

// record queues a Report for the next flush.
func (s *Server) record(hostID string, r reportView) {
	latest, err := json.Marshal(r)
	if err != nil {
		return
	}
	smp := store.Sample{
		HostID: hostID, T: r.ReceivedAt,
		CPU: r.CPUPercent, Memory: toFloat(r.MemoryUsed), Swap: toFloat(r.SwapUsed), Disk: toFloat(r.DiskUsed),
		RxRate: r.RxRate, TxRate: r.TxRate, Load1: r.Load1,
	}
	s.rec.mu.Lock()
	defer s.rec.mu.Unlock()
	s.rec.samples = append(s.rec.samples, smp)
	if s.rec.latest == nil {
		s.rec.latest = make(map[string][]byte)
	}
	s.rec.latest[hostID] = latest
}

// forget drops anything queued for a deleted Host.
func (s *Server) forget(hostID string) {
	s.rec.mu.Lock()
	defer s.rec.mu.Unlock()
	kept := s.rec.samples[:0]
	for _, smp := range s.rec.samples {
		if smp.HostID != hostID {
			kept = append(kept, smp)
		}
	}
	s.rec.samples = kept
	delete(s.rec.latest, hostID)
}

func toFloat(v *uint64) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

// flush writes queued Reports to the database.
func (s *Server) flush(ctx context.Context) {
	s.rec.mu.Lock()
	samples, latest := s.rec.samples, s.rec.latest
	s.rec.samples, s.rec.latest = nil, nil
	s.rec.mu.Unlock()
	if len(samples) == 0 && len(latest) == 0 {
		return
	}
	if err := s.store.WriteBatch(ctx, samples, latest); err != nil {
		s.logger.Error("write Reports", "err", err, "dropped", len(samples))
		return
	}
	s.rec.mu.Lock()
	defer s.rec.mu.Unlock()
	for _, smp := range samples {
		if s.rec.flushedSince == 0 || smp.T < s.rec.flushedSince {
			s.rec.flushedSince = smp.T
		}
	}
}

// downsample aggregates history into the coarser tiers and applies retention.
// Each run recomputes from the earlier of its previous run (buckets that were
// still incomplete then) and the oldest sample flushed since (Reports that
// arrived late), so no bucket is ever skipped.
func (s *Server) downsample(ctx context.Context) {
	now := s.now()
	s.rec.mu.Lock()
	since := s.rec.downsampledAt.UnixMilli()
	if s.rec.downsampledAt.IsZero() {
		since = 0
	} else if s.rec.flushedSince != 0 && s.rec.flushedSince < since {
		since = s.rec.flushedSince
	}
	s.rec.flushedSince = 0
	s.rec.mu.Unlock()

	if err := s.store.Downsample(ctx, since, now, s.retention); err != nil {
		s.logger.Error("downsample history", "err", err)
		s.rec.mu.Lock()
		if s.rec.flushedSince == 0 || since < s.rec.flushedSince {
			s.rec.flushedSince = since // retry this range next time
		}
		s.rec.mu.Unlock()
		return
	}
	s.rec.mu.Lock()
	s.rec.downsampledAt = now
	s.rec.mu.Unlock()
}

// restoreLatest reloads each Host's last Report, so the Status page is not
// blank after a restart. Hosts that have since stopped reporting show Offline.
func (s *Server) restoreLatest(ctx context.Context) {
	stored, err := s.store.LatestReports(ctx)
	if err != nil {
		s.logger.Error("load latest Reports", "err", err)
		return
	}
	for id, b := range stored {
		var r reportView
		if err := json.Unmarshal(b, &r); err == nil {
			s.latest.set(id, r)
		}
	}
}

type historyResponse struct {
	Range string `json:"range"`
	// T holds sample times in Unix seconds, the unit uPlot uses.
	T      []int64    `json:"t"`
	CPU    []*float64 `json:"cpu"`
	Memory []*float64 `json:"memory"`
	Swap   []*float64 `json:"swap"`
	Disk   []*float64 `json:"disk"`
	RxRate []*float64 `json:"rx_rate"`
	TxRate []*float64 `json:"tx_rate"`
	Load1  []*float64 `json:"load1"`
}

// handleHistory serves one Host's charted metrics for a range (1h, 24h, 7d or
// 30d) from the tier that covers it. Hidden Hosts are not found by anonymous viewers.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	spec, ok := historyRanges[rng]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	id := r.PathValue("id")
	admin, err := s.authenticated(r)
	if err != nil {
		s.internalError(w, err)
		return
	}
	exists, err := s.store.HostExists(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	hidden, err := s.isHidden(id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if !exists || (hidden && !admin) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}

	h, err := s.store.History(r.Context(), id, spec.tier, s.now().Add(-spec.span).UnixMilli())
	if err != nil {
		s.internalError(w, err)
		return
	}
	resp := historyResponse{
		Range: rng, T: make([]int64, len(h.T)),
		CPU: h.CPU, Memory: h.Memory, Swap: h.Swap, Disk: h.Disk, RxRate: h.RxRate, TxRate: h.TxRate, Load1: h.Load1,
	}
	for i, t := range h.T {
		resp.T[i] = t / 1000
	}
	writeJSON(w, http.StatusOK, resp)
}
