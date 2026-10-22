// Package config loads the Dashboard's configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
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
}

// Default returns the configuration used when no file overrides it.
func Default() Config {
	return Config{
		Listen:  []string{"127.0.0.1:8080", "[::1]:8080"},
		DataDir: "/var/lib/telltale",
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
	return cfg, nil
}
