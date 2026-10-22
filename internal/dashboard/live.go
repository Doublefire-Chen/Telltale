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

func (l *latestReports) delete(hostID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byHost, hostID)
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
	subs map[*subscriber]struct{}
}

// subscriber is one connected browser: the admin, or an anonymous viewer of
// the public Status page, who must not learn anything about Hidden Hosts.
type subscriber struct {
	ch    chan []byte
	admin bool
}

func (h *hub) subscribe(admin bool) *subscriber {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = make(map[*subscriber]struct{})
	}
	sub := &subscriber{ch: make(chan []byte, liveSubscriberBuffer), admin: admin}
	h.subs[sub] = struct{}{}
	return sub
}

func (h *hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop(sub)
}

// drop removes sub and closes its channel; the caller must hold h.mu.
func (h *hub) drop(sub *subscriber) {
	if _, ok := h.subs[sub]; ok {
		delete(h.subs, sub)
		close(sub.ch)
	}
}

// publish sends msg to the admin, and to anonymous viewers if public is set.
func (h *hub) publish(msg liveMessage, public bool) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if !sub.admin && !public {
			continue
		}
		select {
		case sub.ch <- b:
		default:
			h.drop(sub)
		}
	}
}

// dropAnonymous disconnects every anonymous viewer, e.g. when the Status page
// stops being public.
func (h *hub) dropAnonymous() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if !sub.admin {
			h.drop(sub)
		}
	}
}

// publishHost sends a message about one Host, withholding it from anonymous
// viewers if the Host is Hidden.
func (s *Server) publishHost(msg liveMessage) {
	hidden, err := s.isHidden(msg.HostID)
	if err != nil {
		s.logger.Error("look up Hidden Hosts", "err", err)
		hidden = true
	}
	s.hub.publish(msg, !hidden)
}

// hostsChanged tells every viewer to refetch the Host list, and refreshes the
// cache of Hidden Hosts.
func (s *Server) hostsChanged() {
	s.hiddenMu.Lock()
	s.hidden = nil
	s.hiddenMu.Unlock()
	s.hub.publish(liveMessage{Type: "hosts_changed"}, true)
}

func (s *Server) isHidden(hostID string) (bool, error) {
	s.hiddenMu.Lock()
	defer s.hiddenMu.Unlock()
	if s.hidden == nil {
		ids, err := s.store.HiddenHostIDs(context.Background())
		if err != nil {
			return false, err
		}
		s.hidden = ids
	}
	return s.hidden[hostID], nil
}

// handleLive streams live messages to a viewer until it disconnects or may no
// longer view: the admin's session ended, or the Status page went private.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	admin, err := s.authenticated(r)
	if err != nil {
		s.internalError(w, err)
		return
	}
	// Accept rejects cross-origin upgrades by default.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	sub := s.hub.subscribe(admin)
	defer s.hub.unsubscribe(sub)
	ch := sub.ch
	ctx := c.CloseRead(r.Context())
	ticker := time.NewTicker(livePingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				// Dropped: it fell behind, or may no longer view. Either way
				// the browser reconnects and refetches (or is refused).
				c.Close(websocket.StatusTryAgainLater, "reconnect")
				return
			}
			if err := s.writeLive(ctx, c, msg); err != nil {
				return
			}
		case <-ticker.C:
			if ok, err := s.mayView(r, admin); err != nil || !ok {
				c.Close(websocket.StatusPolicyViolation, "no longer allowed")
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
