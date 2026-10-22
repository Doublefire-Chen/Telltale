// Package e2e runs the real Agent against a real Dashboard on localhost.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/agent"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

func TestAgentReportsToDashboard(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := dashboard.SetAdminPassword(context.Background(), st, "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(dashboard.New(dashboard.Options{Store: st}))
	defer srv.Close()

	// Log in and add a Host, as the admin would in the Admin area.
	cookie := call(t, srv, http.MethodPost, "/api/session", `{"username":"admin","password":"correct horse battery"}`, nil, http.StatusNoContent).Cookies()[0]
	var host struct{ ID, Token string }
	json.NewDecoder(call(t, srv, http.MethodPost, "/api/hosts", `{"name":"e2e"}`, cookie, http.StatusCreated).Body).Decode(&host)

	configPath := filepath.Join(t.TempDir(), "agent.toml")
	os.WriteFile(configPath, []byte("dashboard_url = \""+srv.URL+"\"\ntoken = \""+host.Token+"\"\ninterval = \"1s\"\n"), 0o600)
	cfg, err := agent.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { agent.NewSender(cfg, agent.SystemCollector{}, "v0.0.0-test", nil).Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var status struct {
			Hosts []struct {
				Report *struct {
					CPUPercent *float64 `json:"cpu_percent"`
					MemoryUsed *uint64  `json:"memory_used"`
				} `json:"report"`
			} `json:"hosts"`
		}
		json.NewDecoder(call(t, srv, http.MethodGet, "/api/status", "", cookie, http.StatusOK).Body).Decode(&status)
		if rep := status.Hosts[0].Report; rep != nil {
			if rep.CPUPercent == nil || rep.MemoryUsed == nil || *rep.MemoryUsed == 0 {
				t.Fatalf("real collector sent incomplete metrics: cpu=%v mem=%v", rep.CPUPercent, rep.MemoryUsed)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no Report reached the Dashboard")
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, cookie *http.Cookie, want int) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d", method, path, resp.StatusCode, want)
	}
	return resp
}
