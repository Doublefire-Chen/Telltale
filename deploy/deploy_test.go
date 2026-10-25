// Package deploy holds deployment files: the Agent installer, systemd units,
// and sample configuration.
package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestInstallerUnitMatchesFile keeps the unit embedded in install.sh identical
// to systemd/telltale-agent.service, so what is reviewed is what is installed.
func TestInstallerUnitMatchesFile(t *testing.T) {
	script, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile("systemd/telltale-agent.service")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(script), "<<'UNIT'\n")
	embedded, _, ok2 := strings.Cut(rest, "UNIT\n")
	if !ok || !ok2 {
		t.Fatal("install.sh has no UNIT heredoc")
	}
	if embedded != string(unit) {
		t.Fatal("the unit in install.sh differs from systemd/telltale-agent.service")
	}
}

// TestInstallerNamesTheRightCommandOnTheWrongOS runs the Dashboard's install
// command for another OS, as if copied by mistake, and expects install.sh to
// say which command to copy instead, before it changes anything.
func TestInstallerNamesTheRightCommandOnTheWrongOS(t *testing.T) {
	names := map[string]string{"linux": "Linux", "darwin": "macOS"}
	here, ok := names[runtime.GOOS]
	if !ok {
		t.Skip("install.sh runs on Linux and macOS")
	}
	other := map[string]string{"linux": "darwin", "darwin": "linux"}[runtime.GOOS]
	// A stand-in id passes the root check; the script stops before needing root.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "id"), []byte("#!/bin/sh\necho 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	cmd := exec.Command("sh", "install.sh", "install", "--dashboard-url", "https://status.example.com", "--version", "v1.2.3",
		"--sha256", other+"-amd64="+hash, "--sha256", other+"-arm64="+hash, "--token-file", "-")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader("tt_test\n")
	out, err := cmd.CombinedOutput()
	want := "this command is for " + names[other] + ", but this machine runs " + here + ": copy the " + here + " install command from the Dashboard"
	if err == nil || !strings.Contains(string(out), want) {
		t.Fatalf("got %v:\n%s\nwant %q", err, out, want)
	}
}

// TestWindowsInstallerRefusesOtherOSes runs install.ps1 in PowerShell 7 on
// Linux or macOS, and expects it to point at install.sh.
func TestWindowsInstallerRefusesOtherOSes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.ps1 runs here")
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no PowerShell 7")
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", "install.ps1",
		"-DashboardUrl", "https://status.example.com", "-Version", "v1.2.3").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "this installer is for Windows") {
		t.Fatalf("got %v:\n%s", err, out)
	}
}
