package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

const testToken = "tt_test-token"

type fakeCollector struct{}

func (fakeCollector) Sample(context.Context) v1.Report {
	cpu, mem := 12.5, uint64(512<<20)
	return v1.Report{CPUPercent: &cpu, MemoryUsed: &mem}
}

// received is one request as seen by the fake Dashboard.
type received struct {
	method, path, rawQuery string
	proto                  int
	header                 http.Header
	report                 v1.Report
}

// fakeDashboard records requests and answers each with respond.
type fakeDashboard struct {
	mu       sync.Mutex
	requests []received
	respond  func(w http.ResponseWriter)
}

func (d *fakeDashboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var rep v1.Report
	json.Unmarshal(body, &rep)
	d.mu.Lock()
	d.requests = append(d.requests, received{r.Method, r.URL.Path, r.URL.RawQuery, r.ProtoMajor, r.Header.Clone(), rep})
	respond := d.respond
	d.mu.Unlock()
	if respond != nil {
		respond(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *fakeDashboard) snapshot() []received {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]received(nil), d.requests...)
}

// runFor runs the sender until it has sent n requests (or a timeout).
func runFor(t *testing.T, s *Sender, d *fakeDashboard, n int) []received {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if reqs := d.snapshot(); len(reqs) >= n {
			return reqs
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("got %d requests, want %d", len(d.snapshot()), n)
	return nil
}

func TestSendsReportsOverHTTP2WithBearerToken(t *testing.T) {
	d := &fakeDashboard{}
	srv := httptest.NewUnstartedServer(d)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	s := newSender(srv.URL+"/", testToken, 20*time.Millisecond, fakeCollector{}, nil, srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs)
	reqs := runFor(t, s, d, 3)

	for _, r := range reqs {
		if r.method != http.MethodPost || r.path != v1.ReportPath {
			t.Fatalf("request %s %s, want POST %s", r.method, r.path, v1.ReportPath)
		}
		if r.proto != 2 {
			t.Fatalf("request used HTTP/%d, want HTTP/2", r.proto)
		}
		if got := r.header.Get("Authorization"); got != "Bearer "+testToken {
			t.Fatalf("Authorization = %q", got)
		}
		if strings.Contains(r.rawQuery, testToken) || strings.Contains(r.path, testToken) {
			t.Fatal("Host token appears in the URL")
		}
		if r.report.IntervalMS != 20 || r.report.Time == 0 {
			t.Fatalf("report interval_ms=%d time=%d", r.report.IntervalMS, r.report.Time)
		}
		if r.report.CPUPercent == nil || *r.report.CPUPercent != 12.5 || r.report.MemoryUsed == nil {
			t.Fatalf("report metrics not sent: %+v", r.report)
		}
	}
}

// TestIgnoresEverythingButTheStatusCode checks the one-way property (ADR 0001):
// whatever the Dashboard answers, the Agent keeps sending the same Reports to
// the same place, follows no redirects, keeps no cookies and writes no files.
func TestIgnoresEverythingButTheStatusCode(t *testing.T) {
	elsewhere := &fakeDashboard{}
	other := httptest.NewServer(elsewhere)
	defer other.Close()

	responses := []func(w http.ResponseWriter){
		func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"command":"rm -rf /","interval_ms":1,"dashboard_url":"`+other.URL+`","update":"https://evil.example/agent"}`)
		},
		func(w http.ResponseWriter) {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "planted"})
			w.WriteHeader(http.StatusNoContent)
		},
		func(w http.ResponseWriter) {
			w.Header().Set("Location", other.URL+v1.ReportPath)
			w.WriteHeader(http.StatusTemporaryRedirect)
		},
		func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, strings.Repeat("x", 1<<20))
		},
	}
	var i int
	d := &fakeDashboard{}
	d.respond = func(w http.ResponseWriter) {
		responses[i%len(responses)](w)
		i++
	}
	srv := httptest.NewServer(d)
	defer srv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "agent.toml")
	config := "dashboard_url = \"" + srv.URL + "\"\ntoken = \"" + testToken + "\"\ninterval = \"1s\"\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	before := listDir(t, dir)
	beforeCwd := listDir(t, cwd)

	s := NewSender(cfg, fakeCollector{}, nil)
	s.interval = 20 * time.Millisecond
	reqs := runFor(t, s, d, 2*len(responses)+1)

	for _, r := range reqs {
		if r.path != v1.ReportPath || r.header.Get("Authorization") != "Bearer "+testToken {
			t.Fatalf("Agent changed what it sends: %s %q", r.path, r.header.Get("Authorization"))
		}
		if r.header.Get("Cookie") != "" {
			t.Fatal("Agent sent a cookie the Dashboard set")
		}
		if r.report.IntervalMS != 20 {
			t.Fatalf("Agent changed its interval to %d ms", r.report.IntervalMS)
		}
	}
	if n := len(elsewhere.snapshot()); n != 0 {
		t.Fatalf("Agent followed the Dashboard to another server (%d requests)", n)
	}
	if after, _ := os.ReadFile(configPath); string(after) != config {
		t.Fatal("config file changed")
	}
	if after := listDir(t, dir); after != before {
		t.Fatalf("config directory changed: %q -> %q", before, after)
	}
	if after := listDir(t, cwd); after != beforeCwd {
		t.Fatalf("working directory changed: %q -> %q", beforeCwd, after)
	}
}

func TestKeepsSendingWhenRejected(t *testing.T) {
	d := &fakeDashboard{respond: func(w http.ResponseWriter) { w.WriteHeader(http.StatusUnauthorized) }}
	srv := httptest.NewServer(d)
	defer srv.Close()
	runFor(t, newSender(srv.URL, testToken, 20*time.Millisecond, fakeCollector{}, nil, nil), d, 3)
}

func listDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		info, _ := e.Info()
		names = append(names, e.Name()+"@"+info.ModTime().String())
	}
	return strings.Join(names, ",")
}
