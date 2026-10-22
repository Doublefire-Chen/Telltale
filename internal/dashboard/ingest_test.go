package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

type addedHost struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (h *harness) addHost(session *http.Cookie, name string) addedHost {
	h.t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	resp := h.do(http.MethodPost, "/api/hosts", string(body), session)
	expectStatus(h.t, resp, http.StatusCreated)
	var host addedHost
	if err := json.NewDecoder(resp.Body).Decode(&host); err != nil {
		h.t.Fatal(err)
	}
	return host
}

// postReport sends a Report body the way an Agent does.
func (h *harness) postReport(authorization, body string) *http.Response {
	h.t.Helper()
	r, err := http.NewRequest(http.MethodPost, h.srv.URL+v1.ReportPath, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

type statusBody struct {
	Hosts []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Report *struct {
			ReceivedAt int64    `json:"received_at"`
			Time       int64    `json:"time"`
			IntervalMS int64    `json:"interval_ms"`
			CPUPercent *float64 `json:"cpu_percent"`
			MemoryUsed *uint64  `json:"memory_used"`
		} `json:"report"`
	} `json:"hosts"`
}

func (h *harness) status(session *http.Cookie) statusBody {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var s statusBody
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		h.t.Fatal(err)
	}
	return s
}

func TestAddHostRevealsTokenOnce(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()

	host := h.addHost(session, "  Tokyo-Vultr  ")
	if host.Name != "Tokyo-Vultr" {
		t.Fatalf("name = %q, want trimmed", host.Name)
	}
	if !strings.HasPrefix(host.Token, "tt_") || len(host.Token) != len("tt_")+43 {
		t.Fatalf("token %q is not tt_ + 43 base64url characters", host.Token)
	}

	// The token is never served again, and the database holds only its hash.
	status := h.status(session)
	if len(status.Hosts) != 1 || status.Hosts[0].ID != host.ID || status.Hosts[0].Report != nil {
		t.Fatalf("status = %+v", status)
	}
	raw, _ := json.Marshal(status)
	if bytes.Contains(raw, []byte(host.Token)) {
		t.Fatal("status response contains the Host token")
	}
	h.store.Close()
	for _, suffix := range []string{"", "-wal"} {
		data, _ := os.ReadFile(filepath.Join(h.dataDir, "telltale.db"+suffix))
		if bytes.Contains(data, []byte(host.Token)) {
			t.Fatalf("database file %q contains the Host token in clear text", suffix)
		}
	}
}

func TestAddHostValidation(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()

	for _, name := range []string{"", "   ", strings.Repeat("名", 65)} {
		body, _ := json.Marshal(map[string]string{"name": name})
		expectError(t, h.do(http.MethodPost, "/api/hosts", string(body), session), http.StatusBadRequest, "invalid_name")
	}
	expectError(t, h.do(http.MethodPost, "/api/hosts", `{"name":"x"}`, nil), http.StatusUnauthorized, "unauthenticated")
}

func TestReportReachesStatusPage(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")

	resp := h.postReport("Bearer "+host.Token, `{"time":1761134400000,"interval_ms":2000,"cpu_percent":37.5,"memory_used":1073741824}`)
	expectStatus(t, resp, http.StatusNoContent)

	rep := h.status(session).Hosts[0].Report
	if rep == nil {
		t.Fatal("no Report on the Status page")
	}
	if *rep.CPUPercent != 37.5 || *rep.MemoryUsed != 1<<30 || rep.IntervalMS != 2000 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.ReceivedAt != h.clock.Now().UnixMilli() {
		t.Fatalf("received_at = %d, want the Dashboard's clock", rep.ReceivedAt)
	}
}

func TestReportRejections(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")
	valid := `{"time":1,"interval_ms":2000}`

	for name, auth := range map[string]string{
		"no header":     "",
		"unknown token": "Bearer tt_" + strings.Repeat("A", 43),
		"wrong scheme":  "Basic " + host.Token,
	} {
		if resp := h.postReport(auth, valid); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, resp.StatusCode)
		}
	}
	expectStatus(t, h.postReport("Bearer "+host.Token, `{"cpu_percent":`), http.StatusBadRequest)
	expectStatus(t, h.postReport("Bearer "+host.Token, strings.Repeat(" ", v1.MaxBodyBytes+1)+"{}"), http.StatusBadRequest)
	if h.status(session).Hosts[0].Report != nil {
		t.Fatal("a rejected Report reached the Status page")
	}
}

func TestReportToleratesUnknownAndMissingFields(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")

	resp := h.postReport("Bearer "+host.Token, `{"time":1,"interval_ms":200,"from_the_future":{"x":1}}`)
	expectStatus(t, resp, http.StatusNoContent)
	rep := h.status(session).Hosts[0].Report
	if rep.CPUPercent != nil || rep.MemoryUsed != nil {
		t.Fatalf("missing metrics should stay unknown, got %+v", rep)
	}
	if rep.IntervalMS != 1000 {
		t.Fatalf("interval_ms = %d, want it raised to the 1s minimum", rep.IntervalMS)
	}
}

func TestLiveFeedPushesReports(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "web-1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/api/live"

	// Anonymous browsers get no live feed.
	if _, resp, err := websocket.Dial(ctx, wsURL, nil); err == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous dial: err=%v status=%v, want 401", err, resp)
	}

	header := http.Header{}
	header.Set("Cookie", session.Name+"="+session.Value)
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	expectStatus(t, h.postReport("Bearer "+host.Token, `{"time":1,"interval_ms":2000,"cpu_percent":5}`), http.StatusNoContent)
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Type   string `json:"type"`
		HostID string `json:"host_id"`
		Report struct {
			CPUPercent float64 `json:"cpu_percent"`
		} `json:"report"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "report" || msg.HostID != host.ID || msg.Report.CPUPercent != 5 {
		t.Fatalf("live message = %s", data)
	}

	h.addHost(session, "web-2")
	_, data, err = c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"hosts_changed"`) {
		t.Fatalf("after adding a Host, live message = %s", data)
	}
}

func TestLiveFeedRejectsCrossOrigin(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{}
	header.Set("Cookie", session.Name+"="+session.Value)
	header.Set("Origin", "https://evil.example")
	_, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.srv.URL, "http")+"/api/live",
		&websocket.DialOptions{HTTPHeader: header})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin dial: err=%v, want 403", err)
	}
}
