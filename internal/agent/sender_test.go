package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

const testToken = "tt_test-token"

// fakeCollector returns fixed metrics and a Host Info whose hostname tests can change.
type fakeCollector struct {
	mu       sync.Mutex
	hostname string
}

func (*fakeCollector) Sample(context.Context) v1.Report {
	cpu, mem := 12.5, uint64(512<<20)
	return v1.Report{CPUPercent: &cpu, MemoryUsed: &mem}
}

func (c *fakeCollector) HostInfo(context.Context) v1.HostInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := c.hostname
	if name == "" {
		name = "fake-host"
	}
	return v1.HostInfo{Hostname: name, OS: "linux"}
}

func (c *fakeCollector) setHostname(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hostname = name
}

// received is one request as seen by the fake Dashboard.
type received struct {
	method, path, rawQuery string
	proto                  int
	header                 http.Header
	report                 v1.Report
	hostInfo               v1.HostInfo
}

// fakeDashboard records requests and answers each with respond.
type fakeDashboard struct {
	mu       sync.Mutex
	requests []received
	respond  func(w http.ResponseWriter, r *http.Request)
}

func (d *fakeDashboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	got := received{method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery, proto: r.ProtoMajor, header: r.Header.Clone()}
	if r.URL.Path == v1.HostInfoPath {
		json.Unmarshal(body, &got.hostInfo)
	} else {
		json.Unmarshal(body, &got.report)
	}
	d.mu.Lock()
	d.requests = append(d.requests, got)
	respond := d.respond
	d.mu.Unlock()
	if respond != nil {
		respond(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *fakeDashboard) snapshot() []received {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]received(nil), d.requests...)
}

// onPath returns the requests to one endpoint.
func onPath(reqs []received, path string) []received {
	var out []received
	for _, r := range reqs {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

// runUntil runs the sender until done reports true for the requests so far (or a timeout).
func runUntil(t *testing.T, s *Sender, d *fakeDashboard, done func([]received) bool) []received {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { s.Run(ctx); close(stopped) }()
	defer func() { cancel(); <-stopped }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if reqs := d.snapshot(); done(reqs) {
			return reqs
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met after %d requests", len(d.snapshot()))
	return nil
}

// runFor runs the sender until it has sent n Reports.
func runFor(t *testing.T, s *Sender, d *fakeDashboard, n int) []received {
	t.Helper()
	return runUntil(t, s, d, func(reqs []received) bool { return len(onPath(reqs, v1.ReportPath)) >= n })
}

func TestSendsReportsOverHTTP2WithBearerToken(t *testing.T) {
	d := &fakeDashboard{}
	srv := httptest.NewUnstartedServer(d)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	s := newSender(srv.URL+"/", testToken, 20*time.Millisecond, "v1.2.3", &fakeCollector{}, nil, srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs)
	reqs := runFor(t, s, d, 3)

	if reqs[0].path != v1.HostInfoPath || reqs[0].hostInfo.Hostname != "fake-host" || reqs[0].hostInfo.AgentVersion != "v1.2.3" {
		t.Fatalf("first request %s with Host Info %+v, want Host Info with the Agent version", reqs[0].path, reqs[0].hostInfo)
	}
	for _, r := range reqs {
		if r.method != http.MethodPost || (r.path != v1.ReportPath && r.path != v1.HostInfoPath) {
			t.Fatalf("request %s %s", r.method, r.path)
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
		if r.path != v1.ReportPath {
			continue
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
	d.respond = func(w http.ResponseWriter, _ *http.Request) {
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

	s := NewSender(cfg, &fakeCollector{}, "v1.2.3", nil)
	s.interval = 20 * time.Millisecond
	s.hostInfoCheck = 0 // re-read Host Info every tick, so it is sent under every kind of response
	reqs := runFor(t, s, d, 2*len(responses)+1)

	for _, r := range reqs {
		if r.path == v1.HostInfoPath {
			if r.header.Get("Authorization") != "Bearer "+testToken || r.header.Get("Cookie") != "" {
				t.Fatal("Agent changed how it sends Host Info")
			}
			continue
		}
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
	d := &fakeDashboard{respond: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }}
	srv := httptest.NewServer(d)
	defer srv.Close()
	runFor(t, newSender(srv.URL, testToken, 20*time.Millisecond, "dev", &fakeCollector{}, nil, nil), d, 3)
}

func TestHostInfoSentOnChangeAndRefresh(t *testing.T) {
	d := &fakeDashboard{}
	srv := httptest.NewServer(d)
	defer srv.Close()
	c := &fakeCollector{}
	s := newSender(srv.URL, testToken, 10*time.Millisecond, "dev", c, nil, nil)
	s.hostInfoCheck = 0

	// Unchanged Host Info is not resent on every check.
	reqs := runFor(t, s, d, 10)
	if n := len(onPath(reqs, v1.HostInfoPath)); n != 1 {
		t.Fatalf("unchanged Host Info sent %d times, want once", n)
	}

	// A change is sent on the next check.
	c.setHostname("renamed")
	reqs = runUntil(t, s, d, func(reqs []received) bool {
		infos := onPath(reqs, v1.HostInfoPath)
		return len(infos) >= 2 && infos[len(infos)-1].hostInfo.Hostname == "renamed"
	})
	if n := len(onPath(reqs, v1.HostInfoPath)); n != 2 {
		t.Fatalf("Host Info sent %d times after one change, want 2", n)
	}

	// Once the refresh period passes, it is resent unchanged.
	s.hostInfoRefresh = 0
	runUntil(t, s, d, func(reqs []received) bool { return len(onPath(reqs, v1.HostInfoPath)) >= 4 })
}

func TestHostInfoRetriedUntilAccepted(t *testing.T) {
	var mu sync.Mutex
	failures := 3
	d := &fakeDashboard{}
	d.respond = func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == v1.HostInfoPath && failures > 0 {
			failures--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	srv := httptest.NewServer(d)
	defer srv.Close()
	s := newSender(srv.URL, testToken, 10*time.Millisecond, "dev", &fakeCollector{}, nil, nil)

	reqs := runFor(t, s, d, 10)
	// Three refusals, then one accepted send, then nothing more (unchanged).
	if n := len(onPath(reqs, v1.HostInfoPath)); n != 4 {
		t.Fatalf("Host Info sent %d times, want 4 (3 refused + 1 accepted)", n)
	}
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

// TestHostInfoSentOverBothIPFamilies serves one fake Dashboard on 127.0.0.1
// and ::1 under the name "localhost" and checks Host Info reaches it over both.
func TestHostInfoSentOverBothIPFamilies(t *testing.T) {
	ln4, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln4.Addr().(*net.TCPAddr).Port
	ln6, err := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", port))
	if err != nil {
		ln4.Close()
		t.Skipf("no IPv6 loopback: %v", err)
	}
	if addrs, _ := net.LookupHost("localhost"); !slices.Contains(addrs, "::1") || !slices.Contains(addrs, "127.0.0.1") {
		ln4.Close()
		ln6.Close()
		t.Skipf("localhost does not resolve to both families: %v", addrs)
	}

	var mu sync.Mutex
	from := map[string]bool{}
	d := &fakeDashboard{}
	d.respond = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == v1.HostInfoPath {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			mu.Lock()
			from[host] = true
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}
	srv := &http.Server{Handler: d}
	go srv.Serve(ln4)
	go srv.Serve(ln6)
	defer srv.Close()

	s := newSender(fmt.Sprintf("http://localhost:%d", port), testToken, 10*time.Millisecond, "dev", &fakeCollector{}, nil, nil)
	runUntil(t, s, d, func([]received) bool {
		mu.Lock()
		defer mu.Unlock()
		return from["127.0.0.1"] && from["::1"]
	})
}
