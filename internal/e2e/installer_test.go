//go:build servicetest

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

// installedRelease is the release the installer test downloads first, as a
// new Host would; it then upgrades to this checkout's Agent.
const installedRelease = "v0.2.3"

// TestServiceViaInstaller runs deploy/install.sh (macOS) or deploy/install.ps1
// (Windows) the way the Dashboard's install command does: it downloads a
// release pinned by hash, upgrades it in place keeping the config, and
// uninstalls it, leaving nothing behind.
func TestServiceViaInstaller(t *testing.T) {
	var exe, config string
	switch runtime.GOOS {
	case "windows":
		exe, config = `C:\Program Files\Telltale\telltale-agent.exe`, `C:\ProgramData\Telltale\agent.toml`
	case "darwin":
		exe, config = "/usr/local/bin/telltale-agent", "/etc/telltale/agent.toml"
	default:
		t.Skip("Linux uses deploy/install.sh; see scripts/test-install-in-container.sh")
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := dashboard.SetAdminPassword(context.Background(), st, "installer test password"); err != nil {
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
	cookie := call(t, srv, http.MethodPost, "/api/session", `{"username":"admin","password":"installer test password"}`, nil, http.StatusNoContent).Cookies()[0]
	var host struct{ Token string }
	json.NewDecoder(call(t, srv, http.MethodPost, "/api/hosts", `{"name":""}`, cookie, http.StatusCreated).Body).Decode(&host)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(host.Token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The command pins every platform's hash for this OS, as the Dashboard's does.
	hashes := releaseHashes(t, installedRelease)
	var pins []string
	for _, arch := range []string{"amd64", "arm64"} {
		platform := runtime.GOOS + "-" + arch
		if hashes[platform] == "" {
			t.Fatalf("%s has no hash for %s", installedRelease, platform)
		}
		pins = append(pins, platform+"="+hashes[platform])
	}

	installer := func(args ...string) string {
		t.Helper()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			script, _ := filepath.Abs(`..\..\deploy\install.ps1`)
			cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
				fmt.Sprintf("& ([scriptblock]::Create((Get-Content -Raw '%s'))) %s", script, strings.Join(args, " ")))
		} else {
			cmd = exec.Command("sh", append([]string{"../../deploy/install.sh"}, args...)...)
		}
		out, err := cmd.CombinedOutput()
		t.Logf("installer %s:\n%s", strings.Join(args, " "), out)
		if err != nil {
			t.Fatalf("installer %s: %v", strings.Join(args, " "), err)
		}
		return string(out)
	}
	// flags spells the installer's options for this OS.
	flags := func(pairs ...string) []string {
		var args []string
		for i := 0; i < len(pairs); i += 2 {
			name, value := pairs[i], pairs[i+1]
			if runtime.GOOS == "windows" {
				pwshName := map[string]string{"dashboard-url": "-DashboardUrl", "version": "-Version", "sha256": "-Sha256", "token-file": "-TokenFile", "interval": "-Interval", "binary": "-Binary"}[name]
				if name == "sha256" {
					value = "'" + strings.ReplaceAll(value, " ", "','") + "'"
				} else {
					value = "'" + value + "'"
				}
				args = append(args, pwshName, value)
				continue
			}
			if name == "sha256" {
				for _, v := range strings.Fields(value) {
					args = append(args, "--sha256", v)
				}
				continue
			}
			args = append(args, "--"+name, value)
		}
		return args
	}
	uninstalled := false
	defer func() {
		if !uninstalled {
			exec.Command(exe, "-config", config, "service", "uninstall", "--purge").Run()
		}
	}()

	// A first install of a release, downloaded and checked against its hash.
	installer(append([]string{"install"}, flags("dashboard-url", srv.URL, "version", installedRelease,
		"sha256", strings.Join(pins, " "), "token-file", tokenFile, "interval", "1s")...)...)
	waitForAgent(t, srv, cookie, installedRelease)

	// An upgrade to this checkout's Agent keeps the config and restarts it.
	build := filepath.Join(t.TempDir(), "telltale-agent"+map[bool]string{true: ".exe"}[runtime.GOOS == "windows"])
	if out, err := exec.Command("go", "build", "-o", build, "../../cmd/telltale-agent").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out := installer(append([]string{"install"}, flags("binary", build)...)...)
	if !strings.Contains(out, "kept existing") {
		t.Error("upgrade did not keep the config")
	}
	waitForAgent(t, srv, cookie, "dev")

	installer("uninstall")
	uninstalled = true
	for _, path := range []string{exe, config, filepath.Dir(config)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s left behind: %v", path, err)
		}
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
	}
}

// releaseHashes reads a release's SHA256SUMS into a map from "os-arch" to hash.
func releaseHashes(t *testing.T, release string) map[string]string {
	t.Helper()
	resp, err := http.Get("https://github.com/Doublefire-Chen/Telltale/releases/download/" + release + "/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SHA256SUMS of %s: status %d", release, resp.StatusCode)
	}
	hashes := map[string]string{}
	for sc := bufio.NewScanner(resp.Body); sc.Scan(); {
		hash, name, ok := strings.Cut(sc.Text(), "  ")
		if platform, isAgent := strings.CutPrefix(name, "telltale-agent-"); ok && isAgent {
			hashes[strings.TrimSuffix(platform, ".exe")] = hash
		}
	}
	return hashes
}

// waitForAgent waits until the Host reports, with Host Info from the given Agent version.
func waitForAgent(t *testing.T, srv *httptest.Server, cookie *http.Cookie, version string) {
	t.Helper()
	var status struct {
		Hosts []struct {
			Online   bool `json:"online"`
			HostInfo *struct {
				AgentVersion string `json:"agent_version"`
			} `json:"host_info"`
		} `json:"hosts"`
	}
	for deadline := time.Now().Add(60 * time.Second); ; {
		json.NewDecoder(call(t, srv, http.MethodGet, "/api/status", "", cookie, http.StatusOK).Body).Decode(&status)
		if h := status.Hosts[0]; h.Online && h.HostInfo != nil && h.HostInfo.AgentVersion == version {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no Report from Agent %s: %+v", version, status)
		}
		time.Sleep(time.Second)
	}
}
