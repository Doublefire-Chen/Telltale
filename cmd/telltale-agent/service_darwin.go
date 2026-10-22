package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Doublefire-Chen/Telltale/internal/agent"
)

// The launchd daemon and the dedicated user it runs as. The leading
// underscore is macOS's convention for daemon accounts.
const (
	launchdLabel = "io.github.doublefire-chen.telltale-agent"
	plistPath    = "/Library/LaunchDaemons/" + launchdLabel + ".plist"
	agentUser    = "_telltale-agent"
	logPath      = "/var/log/telltale-agent.log"
)

// Daemon accounts on macOS use IDs below 500; this range avoids Apple's.
const firstID, lastID = 300, 499

// command runs a system tool and returns its output; tests replace it.
var command = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return string(out), nil
}

func runAsService(agent.Config) (bool, error) { return false, nil }

func installService(configPath string) error {
	if os.Geteuid() != 0 {
		return errors.New("run as root (sudo telltale-agent service install)")
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if strings.HasPrefix(exe, "/Users/") {
		return fmt.Errorf("%s is inside a home folder, which %s cannot read; move it to /usr/local/bin first", exe, agentUser)
	}
	uid, gid, created, err := ensureUser()
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("created user %s (no shell, no home)\n", agentUser)
	}
	if err := os.Chown(configPath, uid, gid); err != nil {
		return err
	}
	if err := os.Chmod(configPath, 0o600); err != nil {
		return err
	}
	fmt.Printf("made %s readable only by %s\n", configPath, agentUser)

	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	log.Close()
	if err := os.Chown(logPath, uid, gid); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, launchdPlist(exe, configPath), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", plistPath)
	command("launchctl", "bootout", "system/"+launchdLabel) // in case an older one is loaded
	if _, err := command("launchctl", "bootstrap", "system", plistPath); err != nil {
		return err
	}
	fmt.Printf("started %s as %s; logs go to %s\n", launchdLabel, agentUser, logPath)
	return nil
}

func uninstallService(configPath string, purge bool) error {
	if os.Geteuid() != 0 {
		return errors.New("run as root (sudo telltale-agent service uninstall)")
	}
	if _, err := command("launchctl", "bootout", "system/"+launchdLabel); err == nil {
		fmt.Printf("stopped %s\n", launchdLabel)
	}
	paths := []string{plistPath, logPath}
	if purge {
		paths = append(paths, configPath)
	}
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			fmt.Printf("removed %s\n", p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if _, err := command("dscl", ".", "-read", "/Users/"+agentUser); err == nil {
		if _, err := command("dscl", ".", "-delete", "/Users/"+agentUser); err != nil {
			return err
		}
		command("dscl", ".", "-delete", "/Groups/"+agentUser)
		fmt.Printf("removed user %s\n", agentUser)
	}
	if !purge {
		fmt.Printf("kept %s (add --purge to remove it)\n", configPath)
	}
	return nil
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// ensureUser returns the IDs of the daemon user, creating it (and its group)
// if needed: hidden, shell /usr/bin/false, home /var/empty.
func ensureUser() (uid, gid int, created bool, err error) {
	if out, err := command("dscl", ".", "-read", "/Users/"+agentUser, "UniqueID", "PrimaryGroupID"); err == nil {
		uid, gid, err := parseIDs(out)
		return uid, gid, false, err
	}
	id, err := freeID()
	if err != nil {
		return 0, 0, false, err
	}
	sid := strconv.Itoa(id)
	steps := [][]string{
		{"-create", "/Groups/" + agentUser},
		{"-create", "/Groups/" + agentUser, "PrimaryGroupID", sid},
		{"-create", "/Groups/" + agentUser, "RealName", "Telltale Agent"},
		{"-create", "/Users/" + agentUser},
		{"-create", "/Users/" + agentUser, "UniqueID", sid},
		{"-create", "/Users/" + agentUser, "PrimaryGroupID", sid},
		{"-create", "/Users/" + agentUser, "UserShell", "/usr/bin/false"},
		{"-create", "/Users/" + agentUser, "NFSHomeDirectory", "/var/empty"},
		{"-create", "/Users/" + agentUser, "RealName", "Telltale Agent"},
		{"-create", "/Users/" + agentUser, "IsHidden", "1"},
	}
	for _, s := range steps {
		if _, err := command("dscl", append([]string{"."}, s...)...); err != nil {
			return 0, 0, false, err
		}
	}
	return id, id, true, nil
}

func parseIDs(out string) (uid, gid int, err error) {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(k) {
		case "UniqueID":
			uid = n
		case "PrimaryGroupID":
			gid = n
		}
	}
	if uid == 0 || gid == 0 {
		return 0, 0, fmt.Errorf("cannot read the IDs of %s", agentUser)
	}
	return uid, gid, nil
}

// freeID returns the lowest daemon-range ID used by no user and no group.
func freeID() (int, error) {
	used := map[int]bool{}
	for _, q := range [][]string{{"/Users", "UniqueID"}, {"/Groups", "PrimaryGroupID"}} {
		out, err := command("dscl", ".", "-list", q[0], q[1])
		if err != nil {
			return 0, err
		}
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) == 2 {
				if n, err := strconv.Atoi(f[1]); err == nil {
					used[n] = true
				}
			}
		}
	}
	for id := firstID; id <= lastID; id++ {
		if !used[id] {
			return id, nil
		}
	}
	return 0, errors.New("no free user ID for the daemon account")
}

// launchdPlist is the daemon definition: run at boot as the daemon user,
// restart if it exits, log to logPath.
func launchdPlist(exe, configPath string) []byte {
	var args bytes.Buffer
	for _, a := range []string{exe, "-config", configPath} {
		args.WriteString("\t\t<string>")
		xml.EscapeText(&args, []byte(a))
		args.WriteString("</string>\n")
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
` + args.String() + `	</array>
	<key>UserName</key>
	<string>` + agentUser + `</string>
	<key>GroupName</key>
	<string>` + agentUser + `</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>5</integer>
	<key>StandardOutPath</key>
	<string>` + logPath + `</string>
	<key>StandardErrorPath</key>
	<string>` + logPath + `</string>
</dict>
</plist>
`)
}
