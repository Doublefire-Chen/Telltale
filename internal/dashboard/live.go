package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

const (
	liveSubscriberBuffer = 64
	livePingInterval     = 30 * time.Second
	liveWriteTimeout     = 10 * time.Second
)

// reportView is a Host's latest Report as the frontend sees it.
type reportView struct {
	// ReceivedAt is when the Dashboard received the Report, in Unix milliseconds.
	ReceivedAt int64 `json:"received_at"`
	v1.Report
	// RxRate and TxRate are the Bandwidth in bytes per second, derived from
	// this Report and the previous one; nil when there is no usable previous
	// Report (the first one, or after a reboot reset the counters).
	RxRate *float64 `json:"rx_rate,omitempty"`
	TxRate *float64 `json:"tx_rate,omitempty"`
}

// withBandwidth fills in the Bandwidth of cur relative to the Host's previous Report.
func withBandwidth(cur reportView, prev reportView, havePrev bool) reportView {
	if !havePrev {
		return cur
	}
	seconds := float64(cur.Time-prev.Time) / 1000
	if seconds <= 0 {
		return cur
	}
	cur.RxRate = rate(prev.NetRxBytes, cur.NetRxBytes, seconds)
	cur.TxRate = rate(prev.NetTxBytes, cur.NetTxBytes, seconds)
	return cur
}

func rate(prev, cur *uint64, seconds float64) *float64 {
	// A counter that went down was reset, normally by a reboot: Traffic
	// restarts from zero and there is no meaningful rate for this interval.
	if prev == nil || cur == nil || *cur < *prev {
		return nil
	}
	r := float64(*cur-*prev) / seconds
	return &r
}

// latestReports holds the most recent Report of each Host, in memory.
type latestReports struct {
	mu     sync.RWMutex
	byHost map[string]reportView
}

func (l *latestReports) set(hostID string, r reportView) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byHost == nil {
		l.byHost = make(map[string]reportView)
	}
	l.byHost[hostID] = r
}

// each calls f for every Host's latest Report.
func (l *latestReports) each(f func(hostID string, r reportView)) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for id, r := range l.byHost {
		f(id, r)
	}
}

func (l *latestReports) get(hostID string) (reportView, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	r, ok := l.byHost[hostID]
	return r, ok
}

// offlineAfterIntervals is how many report intervals may pass without a
// Report before a Host is Offline.
const offlineAfterIntervals = 3

// online reports whether a Host whose latest Report is r counts as online at now.
func online(r reportView, now time.Time) bool {
	return now.UnixMilli()-r.ReceivedAt <= offlineAfterIntervals*r.IntervalMS
}

// liveMessage is pushed to Status page browsers over the live WebSocket.
type liveMessage struct {
	// Type is "report" (a Host sent a Report, so it is online), "offline" (a
	// Host missed three intervals) or "hosts_changed" (refetch the Host list).
	Type   string      `json:"type"`
	HostID string      `json:"host_id,omitempty"`
	Report *reportView `json:"report,omitempty"`
}

// hub fans live messages out to connected browsers. A subscriber that falls
// behind is dropped; its browser reconnects and refetches the full state.
type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (h *hub) subscribe() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = make(map[chan []byte]struct{})
	}
	ch := make(chan []byte, liveSubscriberBuffer)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *hub) publish(msg liveMessage) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

// handleLive streams live messages to a logged-in browser until it disconnects
// or its session ends.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	// Accept rejects cross-origin upgrades by default.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)
	ctx := c.CloseRead(r.Context())
	ticker := time.NewTicker(livePingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				c.Close(websocket.StatusTryAgainLater, "fell behind")
				return
			}
			if err := s.writeLive(ctx, c, msg); err != nil {
				return
			}
		case <-ticker.C:
			if ok, err := s.authenticated(r); err != nil || !ok {
				c.Close(websocket.StatusPolicyViolation, "session ended")
				return
			}
			pingCtx, cancel := context.WithTimeout(ctx, liveWriteTimeout)
			err := c.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (s *Server) writeLive(ctx context.Context, c *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, liveWriteTimeout)
	defer cancel()
	return c.Write(ctx, websocket.MessageText, msg)
}
