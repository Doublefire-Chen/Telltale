//go:build servicetest

// The service test installs the real Agent as a system service, so it needs
// admin rights and changes the machine: CI runs it on disposable Windows and
// macOS runners with
//
//	go test -tags servicetest ./internal/e2e -run TestService   (as Administrator / root)
package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

func TestServiceInstallReportsAndUninstalls(t *testing.T) {
	var exe, config string
	switch runtime.GOOS {
	case "windows":
		exe, config = `C:\Program Files\Telltale\telltale-agent.exe`, `C:\ProgramData\Telltale\agent.toml`
	case "darwin":
		exe, config = "/usr/local/bin/telltale-agent", "/etc/telltale/agent.toml"
	default:
		t.Skip("Linux uses deploy/install.sh; see scripts/test-install-in-container.sh")
	}

	// A Dashboard on loopback, with one Host.
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := dashboard.SetAdminPassword(context.Background(), st, "service test password"); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(dashboard.New(dashboard.Options{Store: st}))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	cookie := call(t, srv, http.MethodPost, "/api/session", `{"username":"admin","password":"service test password"}`, nil, http.StatusNoContent).Cookies()[0]
	var host struct{ Token string }
	json.NewDecoder(call(t, srv, http.MethodPost, "/api/hosts", `{"name":""}`, cookie, http.StatusCreated).Body).Decode(&host)

	// Build the Agent where the service account can read it, and write its config.
	for _, dir := range []string{filepath.Dir(exe), filepath.Dir(config)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("go", "build", "-o", exe, "../../cmd/telltale-agent").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cfg := "dashboard_url = \"" + srv.URL + "\"\ntoken = \"" + host.Token + "\"\ninterval = \"1s\"\n"
	if err := os.WriteFile(config, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	agent := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(exe, append([]string{"-config", config}, args...)...).CombinedOutput()
		t.Logf("telltale-agent %s:\n%s", strings.Join(args, " "), out)
		if err != nil {
			t.Fatalf("telltale-agent %s: %v", strings.Join(args, " "), err)
		}
		return string(out)
	}

	agent("service", "install")
	uninstalled := false
	defer func() {
		if !uninstalled {
			exec.Command(exe, "-config", config, "service", "uninstall", "--purge").Run()
		}
	}()

	// The service reports, with Host Info, and without a load average on Windows.
	var status struct {
		Hosts []struct {
			Online   bool           `json:"online"`
			HostInfo map[string]any `json:"host_info"`
			Report   map[string]any `json:"report"`
		} `json:"hosts"`
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		json.NewDecoder(call(t, srv, http.MethodGet, "/api/status", "", cookie, http.StatusOK).Body).Decode(&status)
		if h := status.Hosts[0]; h.Online && h.HostInfo != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the service never reported: %+v", status)
		}
		time.Sleep(time.Second)
	}
	rep := status.Hosts[0].Report
	for _, field := range []string{"cpu_percent", "memory_used", "disk_used", "net_rx_bytes"} {
		if rep[field] == nil {
			t.Errorf("Report lacks %s: %v", field, rep)
		}
	}
	if _, hasLoad := rep["load1"]; hasLoad == (runtime.GOOS == "windows") {
		t.Errorf("load1 present=%v on %s", hasLoad, runtime.GOOS)
	}

	// It runs as its own unprivileged account.
	switch runtime.GOOS {
	case "windows":
		out, _ := exec.Command("sc.exe", "qc", "telltale-agent").CombinedOutput()
		if !strings.Contains(string(out), `NT SERVICE\telltale-agent`) {
			t.Errorf("service does not run as NT SERVICE\\telltale-agent:\n%s", out)
		}
	case "darwin":
		out, _ := exec.Command("ps", "-axo", "user=,comm=").CombinedOutput()
		if !strings.Contains(string(out), "_telltale-agent") {
			t.Errorf("no process runs as _telltale-agent:\n%s", out)
		}
	}

	out := agent("service", "uninstall", "--purge")
	uninstalled = true
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Errorf("config left behind: %v", err)
	}
	switch runtime.GOOS {
	case "windows":
		if err := exec.Command("sc.exe", "query", "telltale-agent").Run(); err == nil {
			t.Error("service still registered")
		}
	case "darwin":
		if err := exec.Command("launchctl", "print", "system/io.github.doublefire-chen.telltale-agent").Run(); err == nil {
			t.Error("daemon still loaded")
		}
		if err := exec.Command("dscl", ".", "-read", "/Users/_telltale-agent").Run(); err == nil {
			t.Error("user still exists")
		}
		if !strings.Contains(out, "removed user _telltale-agent") {
			t.Error("uninstall did not report removing the user")
		}
	}
}
