// Package deploy holds deployment files: the Agent installer, systemd units,
// and sample configuration.
package deploy

import (
	"os"
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
