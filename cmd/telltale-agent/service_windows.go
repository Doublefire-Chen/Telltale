package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/Doublefire-Chen/Telltale/internal/agent"
)

const serviceName = "telltale-agent"

// configSDDL lets only SYSTEM and Administrators (full control) and the
// LocalService account the Agent runs as (read) open the config file, since
// it holds the Host token. P: inherited permissions are not applied.
const configSDDL = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;LS)"

func installService(configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(exe), `c:\users\`) {
		return fmt.Errorf(`%s is inside a user profile, which LocalService cannot read; move it to C:\Program Files\Telltale first`, exe)
	}
	if err := restrictConfig(configPath); err != nil {
		return fmt.Errorf("restrict %s: %w", configPath, err)
	}
	fmt.Printf("made %s readable only by LocalService (and SYSTEM, Administrators)\n", configPath)

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to the service manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()
	if s, err := m.OpenService(serviceName); err == nil {
		s.Close()
		return errors.New("the service is already installed; run `telltale-agent service uninstall` first")
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName:      "Telltale Agent",
		Description:      "Reports this Host's metrics to a Telltale Dashboard.",
		StartType:        mgr.StartAutomatic,
		ServiceStartName: `NT AUTHORITY\LocalService`,
	}, "-config", configPath)
	if err != nil {
		return err
	}
	defer s.Close()
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 5 * time.Second}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 24*60*60); err != nil {
		return err
	}
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil &&
		!strings.Contains(err.Error(), "exists") {
		return err
	}
	fmt.Printf("registered service %s (runs as LocalService)\n", serviceName)
	if err := s.Start(); err != nil {
		return err
	}
	fmt.Printf("started %s; its logs are in the Event Viewer under Windows Logs > Application\n", serviceName)
	return nil
}

func uninstallService(configPath string, purge bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to the service manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()
	if s, err := m.OpenService(serviceName); err == nil {
		if st, err := s.Control(svc.Stop); err == nil {
			for deadline := time.Now().Add(15 * time.Second); st.State != svc.Stopped && time.Now().Before(deadline); {
				time.Sleep(200 * time.Millisecond)
				if st, err = s.Query(); err != nil {
					break
				}
			}
			fmt.Printf("stopped %s\n", serviceName)
		}
		err := s.Delete()
		s.Close()
		if err != nil {
			return err
		}
		fmt.Printf("removed service %s\n", serviceName)
	}
	if err := eventlog.Remove(serviceName); err == nil {
		fmt.Printf("removed event log source %s\n", serviceName)
	}
	if purge {
		if err := os.Remove(configPath); err == nil {
			fmt.Printf("removed %s\n", configPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		fmt.Printf("kept %s (add --purge to remove it)\n", configPath)
	}
	return nil
}

func restrictConfig(path string) error {
	sd, err := windows.SecurityDescriptorFromString(configSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// runAsService runs the Agent under the Service Control Manager when it was
// started by it, logging to the Event Log.
func runAsService(cfg agent.Config) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, nil
	}
	return true, svc.Run(serviceName, &service{cfg: cfg})
}

type service struct {
	cfg agent.Config
}

func (s *service) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	var out io.Writer = io.Discard
	if log, err := eventlog.Open(serviceName); err == nil {
		defer log.Close()
		out = eventWriter{log}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		newSender(s.cfg, slog.New(slog.NewTextHandler(out, nil))).Run(ctx)
		close(done)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for req := range requests {
		switch req.Cmd {
		case svc.Interrogate:
			status <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			cancel()
			<-done
			return false, 0
		}
	}
	cancel()
	<-done
	return false, 0
}

// eventWriter sends each log line to the Event Log, at the matching level.
type eventWriter struct{ log *eventlog.Log }

func (w eventWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	switch {
	case strings.Contains(line, "level=ERROR"):
		w.log.Error(1, line)
	case strings.Contains(line, "level=WARN"):
		w.log.Warning(1, line)
	default:
		w.log.Info(1, line)
	}
	return len(p), nil
}
