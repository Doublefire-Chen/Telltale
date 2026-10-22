package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
)

func (h *harness) onlineStates(session *http.Cookie) map[string]bool {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var body struct {
		Hosts []struct {
			Name   string `json:"name"`
			Online bool   `json:"online"`
		} `json:"hosts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		h.t.Fatal(err)
	}
	out := map[string]bool{}
	for _, v := range body.Hosts {
		out[v.Name] = v.Online
	}
	return out
}

func TestOfflineAfterThreeMissedIntervals(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	token := "Bearer " + h.addHost(session, "web-1").Token
	h.addHost(session, "never-reported")

	expectStatus(t, h.postReport(token, `{"time":1,"interval_ms":2000}`), http.StatusNoContent)
	if got := h.onlineStates(session); !got["web-1"] || got["never-reported"] {
		t.Fatalf("right after a Report: %v", got)
	}

	h.clock.Advance(6 * time.Second) // exactly three intervals: still online
	if !h.onlineStates(session)["web-1"] {
		t.Fatal("Offline after only three intervals")
	}
	h.clock.Advance(time.Millisecond)
	if h.onlineStates(session)["web-1"] {
		t.Fatal("still online after more than three missed intervals")
	}

	expectStatus(t, h.postReport(token, `{"time":2,"interval_ms":2000}`), http.StatusNoContent)
	if !h.onlineStates(session)["web-1"] {
		t.Fatal("not back online after its next Report")
	}
}

func TestOfflineUsesEachHostsInterval(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	fast := "Bearer " + h.addHost(session, "fast").Token
	slow := "Bearer " + h.addHost(session, "slow").Token
	expectStatus(t, h.postReport(fast, `{"time":1,"interval_ms":1000}`), http.StatusNoContent)
	expectStatus(t, h.postReport(slow, `{"time":1,"interval_ms":30000}`), http.StatusNoContent)

	h.clock.Advance(10 * time.Second)
	if got := h.onlineStates(session); got["fast"] || !got["slow"] {
		t.Fatalf("after 10s: %v, want fast Offline and slow online", got)
	}
}

func TestOfflineIsPushedLive(t *testing.T) {
	h := newHarness(t, nil, func(o *dashboard.Options) { o.SweepInterval = 5 * time.Millisecond })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go h.srv.Config.Handler.(*dashboard.Server).Run(ctx)

	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")
	header := http.Header{}
	header.Set("Cookie", session.Name+"="+session.Value)
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.srv.URL, "http")+"/api/live", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	next := func() (typ, hostID string) {
		t.Helper()
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg struct {
			Type   string `json:"type"`
			HostID string `json:"host_id"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatal(err)
		}
		return msg.Type, msg.HostID
	}

	for round := range 2 {
		expectStatus(t, h.postReport("Bearer "+host.Token, `{"time":1,"interval_ms":2000}`), http.StatusNoContent)
		if typ, id := next(); typ != "report" || id != host.ID {
			t.Fatalf("round %d: got %s for %s, want the Report", round, typ, id)
		}
		h.clock.Advance(7 * time.Second)
		if typ, id := next(); typ != "offline" || id != host.ID {
			t.Fatalf("round %d: got %s for %s, want offline", round, typ, id)
		}
	}
}

func TestRejectedAgentRequestsAreRateLimited(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	token := "Bearer " + h.addHost(session, "web-1").Token
	const report = `{"time":1,"interval_ms":2000}`

	for range 20 {
		expectStatus(t, h.postReport("Bearer tt_guess", report), http.StatusUnauthorized)
	}
	// This address is now blocked, even with a valid token, for both endpoints.
	resp := h.postReport(token, report)
	expectStatus(t, resp, http.StatusTooManyRequests)
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("no Retry-After header")
	}
	expectStatus(t, h.postHostInfo(token, `{}`), http.StatusTooManyRequests)

	// Another address is unaffected.
	if code := hostInfoFrom(t, h.srv.Config.Handler, strings.TrimPrefix(token, "Bearer "), "203.0.113.7:4000", nil); code != http.StatusNoContent {
		t.Fatalf("other address: %d, want 204", code)
	}

	h.clock.Advance(time.Minute + time.Second)
	expectStatus(t, h.postReport(token, report), http.StatusNoContent)
}
