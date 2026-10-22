// Command telltale-agent reports a Host's metrics to a Telltale Dashboard.
//
//	telltale-agent [-config path]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Doublefire-Chen/Telltale/internal/agent"
	"github.com/Doublefire-Chen/Telltale/internal/version"
)

func main() {
	configPath := flag.String("config", agent.DefaultConfigPath(), "path to the config file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "telltale-agent:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	collector := agent.SystemCollector{ExcludeInterfaces: cfg.ExcludeInterfaces}
	agent.NewSender(cfg, collector, version.Version, logger).Run(ctx)
}
