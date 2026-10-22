package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadConfigString(t *testing.T, content string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(path)
}

func TestConfigDefaultsInterval(t *testing.T) {
	cfg, err := loadConfigString(t, "dashboard_url = \"https://status.example.com\"\ntoken = \"tt_abc\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval.Duration != 2*time.Second {
		t.Fatalf("interval = %s, want 2s", cfg.Interval.Duration)
	}
}

func TestConfigRejectsInvalidSettings(t *testing.T) {
	const ok = "token = \"tt_abc\"\n"
	for name, tc := range map[string]struct{ content, want string }{
		"short interval":  {"dashboard_url = \"https://status.example.com\"\n" + ok + "interval = \"500ms\"\n", "at least 1s"},
		"plain http":      {"dashboard_url = \"http://status.example.com\"\n" + ok, "https"},
		"not a URL":       {"dashboard_url = \"status.example.com\"\n" + ok, "absolute URL"},
		"query string":    {"dashboard_url = \"https://status.example.com/?token=x\"\n" + ok, "query"},
		"missing token":   {"dashboard_url = \"https://status.example.com\"\n", "tt_"},
		"wrong token":     {"dashboard_url = \"https://status.example.com\"\ntoken = \"abc\"\n", "tt_"},
		"unknown setting": {"dashboard_url = \"https://status.example.com\"\n" + ok + "intervall = \"2s\"\n", "unknown setting"},
	} {
		_, err := loadConfigString(t, tc.content)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}

func TestConfigAllowsPlainHTTPOnLoopback(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "http://localhost:8080"} {
		if _, err := loadConfigString(t, "dashboard_url = \""+u+"\"\ntoken = \"tt_abc\"\n"); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
}
