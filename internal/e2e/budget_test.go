//go:build budget && linux

// The budget test holds the Agent to its resource budget: it builds the
// release binary, runs it at the default interval against a Dashboard on
// loopback, and fails if the binary, its peak memory or its CPU use grows past
// the limits below. It takes about a minute, so CI runs it separately with
//
//	go test -tags budget -v ./internal/e2e -run TestAgentBudget
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

const (
	budgetBinaryBytes = 10 << 20 // release binary, stripped
	budgetPeakRSS     = 25 << 20 // VmHWM over the run
	budgetCPUPercent  = 1.0      // of one core, averaged over the run
	budgetRun         = 60 * time.Second
	clockTicks        = 100 // USER_HZ, 100 on every Linux architecture Telltale ships for
)

func TestAgentBudget(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := dashboard.SetAdminPassword(context.Background(), st, "budget test password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(dashboard.New(dashboard.Options{Store: st}))
	defer srv.Close()
	cookie := call(t, srv, http.MethodPost, "/api/session", `{"username":"admin","password":"budget test password"}`, nil, http.StatusNoContent).Cookies()[0]
	var host struct{ Token string }
	json.NewDecoder(call(t, srv, http.MethodPost, "/api/hosts", `{"name":"budget"}`, cookie, http.StatusCreated).Body).Decode(&host)

	dir := t.TempDir()
	exe := filepath.Join(dir, "telltale-agent")
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", exe, "../../cmd/telltale-agent")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("binary: %.1f MiB (budget %d MiB)", float64(info.Size())/(1<<20), budgetBinaryBytes>>20)
	if info.Size() > budgetBinaryBytes {
		t.Errorf("binary is %d bytes, over the %d byte budget", info.Size(), budgetBinaryBytes)
	}

	// No interval in the config: the Agent runs at its default.
	config := filepath.Join(dir, "agent.toml")
	os.WriteFile(config, []byte("dashboard_url = \""+srv.URL+"\"\ntoken = \""+host.Token+"\"\n"), 0o600)
	cmd := exec.Command(exe, "-config", config)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()

	start := time.Now()
	time.Sleep(budgetRun)
	elapsed := time.Since(start)
	pid := strconv.Itoa(cmd.Process.Pid)

	stat, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		t.Fatalf("Agent exited early: %v", err)
	}
	// Fields after the parenthesised command name; utime and stime are the
	// 14th and 15th fields of the whole line.
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+2:]))
	utime, _ := strconv.ParseFloat(fields[11], 64)
	stime, _ := strconv.ParseFloat(fields[12], 64)
	cpu := (utime + stime) / clockTicks / elapsed.Seconds() * 100
	t.Logf("CPU: %.2f%% of one core over %s (budget %.1f%%)", cpu, elapsed.Round(time.Second), budgetCPUPercent)
	if cpu > budgetCPUPercent {
		t.Errorf("Agent used %.2f%% of one core, over the %.1f%% budget", cpu, budgetCPUPercent)
	}

	status, _ := os.ReadFile("/proc/" + pid + "/status")
	var rss int64
	for _, line := range strings.Split(string(status), "\n") {
		if kb, ok := strings.CutPrefix(line, "VmHWM:"); ok {
			n, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(kb), "kB")), 10, 64)
			rss = n << 10
		}
	}
	t.Logf("peak RSS: %.1f MiB (budget %d MiB)", float64(rss)/(1<<20), budgetPeakRSS>>20)
	if rss == 0 || rss > budgetPeakRSS {
		t.Errorf("Agent peak RSS is %d bytes, budget %d", rss, budgetPeakRSS)
	}

	// It must have been reporting the whole time, or the numbers mean nothing.
	var s struct {
		Hosts []struct{ Online bool } `json:"hosts"`
	}
	json.NewDecoder(call(t, srv, http.MethodGet, "/api/status", "", cookie, http.StatusOK).Body).Decode(&s)
	if len(s.Hosts) != 1 || !s.Hosts[0].Online {
		t.Fatal("the Agent was not reporting during the budget run")
	}
}
