// Command telltale-agent reports a Host's metrics to a Telltale Dashboard.
//
//	telltale-agent [-config path]
//	telltale-agent [-config path] service install      (Windows, macOS)
//	telltale-agent [-config path] service uninstall [--purge]
//
// On Linux, install with deploy/install.sh instead.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Doublefire-Chen/Telltale/internal/agent"
	"github.com/Doublefire-Chen/Telltale/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "telltale-agent:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fl := flag.NewFlagSet("telltale-agent", flag.ContinueOnError)
	configPath := fl.String("config", agent.DefaultConfigPath(), "path to the config file")
	showVersion := fl.Bool("version", false, "print the version and exit")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(version.Version)
		return nil
	}
	path, err := filepath.Abs(*configPath)
	if err != nil {
		return err
	}

	switch rest := fl.Args(); {
	case len(rest) == 0:
	case len(rest) == 2 && rest[0] == "service" && rest[1] == "install":
		if _, err := agent.LoadConfig(path); err != nil {
			return err
		}
		return installService(path)
	case len(rest) >= 2 && rest[0] == "service" && rest[1] == "uninstall":
		purge := len(rest) == 3 && rest[2] == "--purge"
		if len(rest) > 2 && !purge {
			return errors.New("usage: telltale-agent service uninstall [--purge]")
		}
		return uninstallService(path, purge)
	default:
		return fmt.Errorf("unknown command %q; usage: telltale-agent [-config path] [service install | service uninstall [--purge]]", rest)
	}

	cfg, err := agent.LoadConfig(path)
	if err != nil {
		return err
	}
	if handled, err := runAsService(cfg); handled {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	newSender(cfg, logger).Run(ctx)
	return nil
}

func newSender(cfg agent.Config, logger *slog.Logger) *agent.Sender {
	collector := agent.SystemCollector{ExcludeInterfaces: cfg.ExcludeInterfaces}
	return agent.NewSender(cfg, collector, version.Version, logger)
}
