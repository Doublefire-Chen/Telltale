package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

// writeConfig writes a config file pointing at a fresh data directory.
func writeConfig(t *testing.T) (configPath, dataDir string) {
	t.Helper()
	dir := t.TempDir()
	dataDir = filepath.Join(dir, "data")
	configPath = filepath.Join(dir, "dashboard.toml")
	content := "listen = [\"127.0.0.1:0\"]\ndata_dir = " + `"` + filepath.ToSlash(dataDir) + `"` + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, dataDir
}

func setPasswordCmd(t *testing.T, configPath, input string) error {
	t.Helper()
	var out bytes.Buffer
	return run([]string{"-config", configPath, "admin", "set-password"}, strings.NewReader(input), &out, &out)
}

func storedHash(t *testing.T, dataDir string) string {
	t.Helper()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, err := st.AdminPasswordHash(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestAdminSetPassword(t *testing.T) {
	configPath, dataDir := writeConfig(t)

	if err := setPasswordCmd(t, configPath, "first password\nfirst password\n"); err != nil {
		t.Fatal(err)
	}
	first := storedHash(t, dataDir)
	if ok, _ := dashboard.VerifyPassword("first password", first); !ok {
		t.Fatal("stored hash does not verify the password that was set")
	}

	// Running it again resets the password.
	if err := setPasswordCmd(t, configPath, "second password\nsecond password\n"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := dashboard.VerifyPassword("second password", storedHash(t, dataDir)); !ok {
		t.Fatal("password was not reset")
	}
}

func TestAdminSetPasswordRejectsBadInput(t *testing.T) {
	configPath, dataDir := writeConfig(t)
	for name, input := range map[string]string{
		"mismatch": "first password\nother password\n",
		"short":    "short\nshort\n",
		"eof":      "only one line\n",
	} {
		if err := setPasswordCmd(t, configPath, input); err == nil {
			t.Errorf("%s: set-password succeeded", name)
		}
	}
	if storedHash(t, dataDir) != "" {
		t.Fatal("a rejected password was stored")
	}
}

func TestRejectsUnknownCommandAndSetting(t *testing.T) {
	configPath, _ := writeConfig(t)
	var out bytes.Buffer
	if err := run([]string{"-config", configPath, "admin", "frobnicate"}, strings.NewReader(""), &out, &out); err == nil {
		t.Fatal("unknown command accepted")
	}

	bad := filepath.Join(t.TempDir(), "bad.toml")
	os.WriteFile(bad, []byte("listne = [\"127.0.0.1:8080\"]\n"), 0o600)
	if err := run([]string{"-config", bad, "admin", "set-password"}, strings.NewReader(""), &out, &out); err == nil ||
		!strings.Contains(err.Error(), "unknown setting") {
		t.Fatalf("misspelled setting: err = %v, want unknown setting", err)
	}

	if err := run([]string{"-config", filepath.Join(t.TempDir(), "missing.toml")}, strings.NewReader(""), &out, &out); err == nil {
		t.Fatal("explicit missing config file accepted")
	}
}
