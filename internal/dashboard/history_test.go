package dashboard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

type historyBody struct {
	Range  string     `json:"range"`
	T      []int64    `json:"t"`
	CPU    []*float64 `json:"cpu"`
	Memory []*float64 `json:"memory"`
	RxRate []*float64 `json:"rx_rate"`
}

// historyHarness runs the Dashboard's background work with short intervals,
// so flushing and downsampling happen promptly while the fake clock decides
// what time it is.
func historyHarness(t *testing.T, retention store.Retention) (*harness, func()) {
	t.Helper()
	h := newHarness(t, nil, func(o *dashboard.Options) {
		o.FlushInterval = 2 * time.Millisecond
		o.DownsampleInterval = 2 * time.Millisecond
		o.SweepInterval = time.Hour
		o.Retention = retention
		o.Logger = slog.New(slog.NewTextHandler(testWriter{t}, nil))
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.srv.Config.Handler.(*dashboard.Server).Run(ctx); close(done) }()
	stop := func() { cancel(); <-done }
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			stop()
		}
	})
	return h, stop
}

func (h *harness) history(session *http.Cookie, id, rng string) historyBody {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/hosts/"+id+"/history?range="+rng, "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var b historyBody
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		h.t.Fatal(err)
	}
	return b
}

// eventually polls the history until ok accepts it.
func (h *harness) eventually(session *http.Cookie, id, rng string, ok func(historyBody) bool) historyBody {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		b := h.history(session, id, rng)
		if ok(b) {
			return b
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("history %s never matched; last: %+v", rng, b)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func cpuReport(cpu float64) string {
	return fmt.Sprintf(`{"time":1,"interval_ms":2000,"cpu_percent":%g,"memory_used":%d}`, cpu, int(cpu)*1000)
}

func values(vs []*float64) []float64 {
	out := make([]float64, len(vs))
	for i, v := range vs {
		if v == nil {
			out[i] = -1
		} else {
			out[i] = *v
		}
	}
	return out
}

func TestRawHistoryIsWrittenInBatches(t *testing.T) {
	h, _ := historyHarness(t, store.Retention{})
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")

	for _, cpu := range []float64{10, 20, 30} {
		expectStatus(t, h.postReport("Bearer "+host.Token, cpuReport(cpu)), http.StatusNoContent)
		h.clock.Advance(2 * time.Second)
	}
	b := h.eventually(session, host.ID, "1h", func(b historyBody) bool { return len(b.T) == 3 })
	if got := values(b.CPU); fmt.Sprint(got) != "[10 20 30]" {
		t.Fatalf("raw cpu = %v", got)
	}
	start := h.clock.Now().Add(-6 * time.Second).Unix()
	if b.T[0] != start || b.T[2] != start+4 {
		t.Fatalf("raw times %v, want seconds from %d in 2s steps", b.T, start)
	}
	if b.Range != "1h" {
		t.Fatalf("range = %q", b.Range)
	}
}

func TestMinuteAndQuarterTiers(t *testing.T) {
	h, _ := historyHarness(t, store.Retention{})
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")

	// Start on a quarter-hour boundary (the fake clock starts at 12:00:00).
	// Minute 0: 10, 30; minute 1: 50; ... one Report every 30s for 16 minutes.
	var cpu float64
	for i := range 32 {
		cpu = float64(i)
		expectStatus(t, h.postReport("Bearer "+host.Token, cpuReport(cpu)), http.StatusNoContent)
		h.clock.Advance(30 * time.Second)
	}
	// 24h is served from 1-minute averages: one point per complete minute.
	minute := h.eventually(session, host.ID, "24h", func(b historyBody) bool { return len(b.T) == 16 })
	if got := values(minute.CPU)[:3]; fmt.Sprint(got) != "[0.5 2.5 4.5]" {
		t.Fatalf("first 1-minute averages = %v, want [0.5 2.5 4.5]", got)
	}
	if minute.T[1]-minute.T[0] != 60 {
		t.Fatalf("1-minute buckets %v apart", minute.T[1]-minute.T[0])
	}
	// 30d is served from 15-minute averages: the first quarter is complete.
	quarter := h.eventually(session, host.ID, "30d", func(b historyBody) bool { return len(b.T) == 1 })
	if got := values(quarter.CPU); fmt.Sprint(got) != "[14.5]" { // mean of minutes 0–14 = mean of 0..29
		t.Fatalf("15-minute average = %v, want [14.5]", got)
	}
	// 7d is also served from 1-minute averages.
	if b := h.history(session, host.ID, "7d"); len(b.T) != 16 {
		t.Fatalf("7d has %d points, want the 16 minute buckets", len(b.T))
	}
}

func TestRetentionIsConfigurable(t *testing.T) {
	h, _ := historyHarness(t, store.Retention{Raw: 10 * time.Minute, Minute: time.Hour, Quarter: 2 * time.Hour})
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")
	for range 4 {
		expectStatus(t, h.postReport("Bearer "+host.Token, cpuReport(5)), http.StatusNoContent)
		h.clock.Advance(30 * time.Second)
	}
	h.eventually(session, host.ID, "24h", func(b historyBody) bool { return len(b.T) == 2 })

	// Past raw retention: raw samples go, minute averages stay.
	h.clock.Advance(15 * time.Minute)
	h.eventually(session, host.ID, "1h", func(b historyBody) bool { return len(b.T) == 0 })
	if b := h.history(session, host.ID, "24h"); len(b.T) != 2 {
		t.Fatalf("minute tier lost data early: %d points", len(b.T))
	}
	// Past minute retention: minute averages go, the quarter average stays.
	h.clock.Advance(time.Hour)
	h.eventually(session, host.ID, "24h", func(b historyBody) bool { return len(b.T) == 0 })
	if b := h.history(session, host.ID, "30d"); len(b.T) != 1 {
		t.Fatalf("quarter tier lost data early: %d points", len(b.T))
	}
	// Past quarter retention: nothing is left.
	h.clock.Advance(2 * time.Hour)
	h.eventually(session, host.ID, "30d", func(b historyBody) bool { return len(b.T) == 0 })
}

func TestHistoryAccess(t *testing.T) {
	h, _ := historyHarness(t, store.Retention{})
	h.setPassword(testPassword)
	session := h.loginOK()
	visible := h.addHost(session, "visible")
	hidden := h.addHost(session, "hidden")
	expectStatus(t, h.patchHost(session, hidden.ID, map[string]any{"hidden": true}), http.StatusNoContent)

	expectError(t, h.do(http.MethodGet, "/api/hosts/"+visible.ID+"/history?range=2h", "", session), http.StatusBadRequest, "invalid_range")
	expectError(t, h.do(http.MethodGet, "/api/hosts/nope/history?range=1h", "", session), http.StatusNotFound, "not_found")
	expectError(t, h.do(http.MethodGet, "/api/hosts/"+visible.ID+"/history?range=1h", "", nil), http.StatusUnauthorized, "unauthenticated")

	h.setPublic(session, true)
	expectStatus(t, h.do(http.MethodGet, "/api/hosts/"+visible.ID+"/history?range=1h", "", nil), http.StatusOK)
	expectError(t, h.do(http.MethodGet, "/api/hosts/"+hidden.ID+"/history?range=1h", "", nil), http.StatusNotFound, "not_found")
	expectStatus(t, h.do(http.MethodGet, "/api/hosts/"+hidden.ID+"/history?range=1h", "", session), http.StatusOK)
}

func TestLatestStateSurvivesRestart(t *testing.T) {
	h, stop := historyHarness(t, store.Retention{})
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")
	expectStatus(t, h.postReport("Bearer "+host.Token, `{"time":1,"interval_ms":2000,"cpu_percent":42,"net_rx_bytes":7}`), http.StatusNoContent)
	stop() // shutting down writes what is buffered

	restarted := httptest.NewServer(dashboard.New(dashboard.Options{Store: h.store, Now: h.clock.Now}))
	defer restarted.Close()
	h2 := &harness{t: t, store: h.store, clock: h.clock, srv: restarted}
	rep := h2.latestReport(h2.loginOK())
	if rep == nil || rep["cpu_percent"] != 42.0 || rep["net_rx_bytes"] != 7.0 {
		t.Fatalf("latest Report after restart: %v", rep)
	}
	// It is still recognised as stale once the Host stops reporting.
	h.clock.Advance(time.Minute)
	if h2.onlineStates(h2.loginOK())["web-1"] {
		t.Fatal("restored Host counts as online although it has not reported")
	}
}

func TestDeletingAHostDeletesItsHistory(t *testing.T) {
	h, _ := historyHarness(t, store.Retention{})
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")
	for range 4 {
		expectStatus(t, h.postReport("Bearer "+host.Token, cpuReport(5)), http.StatusNoContent)
		h.clock.Advance(30 * time.Second)
	}
	h.eventually(session, host.ID, "24h", func(b historyBody) bool { return len(b.T) == 2 })

	expectStatus(t, h.do(http.MethodDelete, "/api/hosts/"+host.ID, "", session), http.StatusNoContent)
	for _, tier := range []store.Tier{store.TierRaw, store.TierMinute, store.TierQuarter} {
		rows, err := h.store.History(context.Background(), host.ID, tier, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows.T) != 0 {
			t.Fatalf("tier %d still holds %d rows for the deleted Host", tier, len(rows.T))
		}
	}
}

// testWriter sends Dashboard logs to the test log.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
