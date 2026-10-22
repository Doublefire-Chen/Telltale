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

func (l *latestReports) get(hostID string) (reportView, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	r, ok := l.byHost[hostID]
	return r, ok
}

// liveMessage is pushed to Status page browsers over the live WebSocket.
type liveMessage struct {
	// Type is "report" (a Host sent a Report) or "hosts_changed" (refetch the Host list).
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
