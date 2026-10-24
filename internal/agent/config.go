// Package agent implements the Telltale Agent: it samples a Host's metrics and
// sends them to the Dashboard. It is strictly one-way (ADR 0001): its
// configuration comes only from a local file and it acts on nothing the
// Dashboard sends back.
package agent

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

// DefaultInterval is the report interval when the config file sets none.
const DefaultInterval = 2 * time.Second

// DefaultConfigPath is where the Agent reads its config file when none is given.
func DefaultConfigPath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\Telltale\agent.toml`
	}
	return "/etc/telltale/agent.toml"
}

// Config is the Agent's local configuration.
type Config struct {
	// DashboardURL is the Dashboard's base URL. It must be HTTPS, except for a
	// loopback address, so the Host token never crosses a network in clear text.
	DashboardURL string `toml:"dashboard_url"`
	// Token is the Host token issued by the Dashboard.
	Token string `toml:"token"`
	// Interval is the time between Reports; at least v1.MinInterval.
	Interval duration `toml:"interval"`
	// ExcludeInterfaces are glob patterns of network interfaces left out of
	// Traffic. When unset, DefaultExcludeInterfaces applies; setting it
	// replaces the default list.
	ExcludeInterfaces []string `toml:"exclude_interfaces"`
}

// duration decodes TOML strings like "2s".
type duration struct{ time.Duration }

func (d *duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// LoadConfig reads and validates the config file at path.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg := Config{Interval: duration{DefaultInterval}}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("parse config %s: unknown setting %q", path, undecoded[0].String())
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	u, err := url.Parse(c.DashboardURL)
	if err != nil || u.Host == "" {
		return errors.New("dashboard_url must be an absolute URL such as https://status.example.com")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopback(u.Hostname()) {
			return errors.New("dashboard_url must use https (plain http is allowed only for a loopback address)")
		}
	default:
		return errors.New("dashboard_url must use https")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("dashboard_url must not have a query or fragment")
	}
	if u.User != nil {
		return errors.New("dashboard_url must not contain a user name or password; the Host token is the only credential")
	}
	if !strings.HasPrefix(c.Token, "tt_") || len(c.Token) <= len("tt_") {
		return errors.New(`token must be the Host token from the Dashboard (it starts with "tt_")`)
	}
	if c.Interval.Duration < v1.MinInterval {
		return fmt.Errorf("interval must be at least %s", v1.MinInterval)
	}
	for _, p := range c.ExcludeInterfaces {
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("exclude_interfaces: bad pattern %q", p)
		}
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
