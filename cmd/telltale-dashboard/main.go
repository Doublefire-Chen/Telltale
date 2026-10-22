// Command telltale-dashboard runs the Telltale Dashboard.
//
//	telltale-dashboard [-config path] [serve]
//	telltale-dashboard [-config path] admin set-password
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/config"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/geoip"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/store"
	"github.com/Doublefire-Chen/Telltale/internal/version"
	"github.com/Doublefire-Chen/Telltale/web"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "telltale-dashboard:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fl := flag.NewFlagSet("telltale-dashboard", flag.ContinueOnError)
	fl.SetOutput(stderr)
	configPath := fl.String("config", "", "path to the config file (default "+config.DefaultPath+")")
	showVersion := fl.Bool("version", false, "print the version and exit")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, version.Version)
		return nil
	}

	path, allowMissing := *configPath, false
	if path == "" {
		path, allowMissing = config.DefaultPath, true
	}
	cfg, err := config.Load(path, allowMissing)
	if err != nil {
		return err
	}

	switch rest := fl.Args(); {
	case len(rest) == 0 || (len(rest) == 1 && rest[0] == "serve"):
		return serve(cfg, stderr)
	case len(rest) == 2 && rest[0] == "admin" && rest[1] == "set-password":
		return setPassword(cfg, stdin, stdout)
	default:
		return fmt.Errorf("unknown command %q; usage: telltale-dashboard [-config path] [serve | admin set-password]", strings.Join(rest, " "))
	}
}

func serve(cfg config.Config, stderr io.Writer) error {
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	proxies, err := cfg.Proxies()
	if err != nil {
		return err
	}
	geo, err := geoip.Open(cfg.GeoIPDB)
	if err != nil {
		return err
	}
	if geo == nil {
		logger.Warn("no GeoIP database built into this binary; countries will be unknown (run `make geoip` before building)")
	}

	app := web.App()
	if app == nil {
		logger.Warn("web app not built into this binary; serving a placeholder page")
	}
	srv := &http.Server{
		Handler: dashboard.New(dashboard.Options{
			Store: st, App: app, Logger: logger, TrustedProxies: proxies, GeoIP: geo,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	// Serve on every address that can be bound, so a host without IPv6
	// loopback still works with the default listen list.
	var listeners []net.Listener
	for _, addr := range cfg.Listen {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Warn("cannot listen", "addr", addr, "err", err)
			continue
		}
		listeners = append(listeners, ln)
	}
	if len(listeners) == 0 {
		return errors.New("could not listen on any configured address")
	}

	errc := make(chan error, len(listeners))
	for _, ln := range listeners {
		logger.Info("listening", "addr", ln.Addr().String())
		go func() { errc <- srv.Serve(ln) }()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func setPassword(cfg config.Config, stdin io.Reader, stdout io.Writer) error {
	read := lineReader(stdin, stdout)
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		read = func(prompt string) (string, error) {
			fmt.Fprint(stdout, prompt)
			b, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(stdout)
			return string(b), err
		}
	}

	password, err := read("New admin password: ")
	if err != nil {
		return err
	}
	if err := dashboard.ValidatePassword(password); err != nil {
		return err
	}
	confirm, err := read("Repeat password: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("passwords do not match")
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := dashboard.SetAdminPassword(context.Background(), st, password); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Admin password set. Log in as %q; existing sessions have been ended.\n", dashboard.AdminUsername)
	return nil
}

// lineReader reads one secret per line, for non-interactive use.
func lineReader(stdin io.Reader, stdout io.Writer) func(string) (string, error) {
	scanner := bufio.NewScanner(stdin)
	return func(prompt string) (string, error) {
		fmt.Fprint(stdout, prompt)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", io.ErrUnexpectedEOF
		}
		fmt.Fprintln(stdout)
		return strings.TrimRight(scanner.Text(), "\r"), nil
	}
}
