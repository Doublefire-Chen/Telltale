// Package config loads the Dashboard's configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
)

// DefaultPath is where the Dashboard looks for its config file when none is given.
const DefaultPath = "/etc/telltale/dashboard.toml"

// Config is the Dashboard's configuration.
type Config struct {
	// Listen holds the addresses the Dashboard serves on. Loopback-only by
	// default: HTTPS is terminated by a reverse proxy in front of it.
	Listen []string `toml:"listen"`
	// DataDir holds all Dashboard state (the SQLite database).
	DataDir string `toml:"data_dir"`
	// TrustedProxies are the addresses or CIDR ranges of reverse proxies whose
	// X-Forwarded-For / X-Real-IP headers are believed. Defaults to loopback.
	TrustedProxies []string `toml:"trusted_proxies"`
	// GeoIPDB replaces the embedded GeoIP database with a MaxMind-format file.
	GeoIPDB string `toml:"geoip_db"`
	// HistoryRaw, HistoryMinute and HistoryQuarter are how long raw
	// Reports, 1-minute averages and 15-minute averages are kept, e.g.
	// "1h", "7d", "90d".
	HistoryRaw     string `toml:"history_raw"`
	HistoryMinute  string `toml:"history_minute"`
	HistoryQuarter string `toml:"history_quarter"`
}

// Retention parses the history settings.
func (c Config) Retention() (store.Retention, error) {
	var r store.Retention
	for _, f := range []struct {
		name, value string
		dst         *time.Duration
	}{
		{"history_raw", c.HistoryRaw, &r.Raw},
		{"history_minute", c.HistoryMinute, &r.Minute},
		{"history_quarter", c.HistoryQuarter, &r.Quarter},
	} {
		d, err := store.ParseRetention(f.value)
		if err != nil {
			return store.Retention{}, fmt.Errorf("%s: %w", f.name, err)
		}
		*f.dst = d
	}
	return r, nil
}

// Proxies parses TrustedProxies; plain addresses become single-address ranges.
func (c Config) Proxies() ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(c.TrustedProxies))
	for _, v := range c.TrustedProxies {
		if p, err := netip.ParsePrefix(v); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("trusted_proxies: %q is not an address or CIDR range", v)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// Default returns the configuration used when no file overrides it.
func Default() Config {
	return Config{
		Listen:         []string{"127.0.0.1:8080", "[::1]:8080"},
		DataDir:        "/var/lib/telltale",
		TrustedProxies: []string{"127.0.0.0/8", "::1"},
		HistoryRaw:     "1h",
		HistoryMinute:  "7d",
		HistoryQuarter: "90d",
	}
}

// Load reads the config file at path on top of the defaults. A missing file is
// an error unless allowMissing is set, in which case the defaults are returned.
func Load(path string, allowMissing bool) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if allowMissing && errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("parse config %s: unknown setting %q", path, undecoded[0].String())
	}
	if len(cfg.Listen) == 0 {
		return Config{}, fmt.Errorf("config %s: listen must not be empty", path)
	}
	if cfg.DataDir == "" {
		return Config{}, fmt.Errorf("config %s: data_dir must not be empty", path)
	}
	if _, err := cfg.Proxies(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if _, err := cfg.Retention(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}
