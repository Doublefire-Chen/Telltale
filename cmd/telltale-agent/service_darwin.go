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
	"syscall"
	"time"

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
	// Whoever can replace the Agent runs code as its user, and whoever can
	// swap the config redirects its Reports.
	owners := trustedOwners()
	if err := changeableOnlyBy(exe, owners); err != nil {
		return fmt.Errorf("%w, so others could replace %s; move it to /usr/local/bin first", err, exe)
	}
	config, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return err
	}
	if err := changeableOnlyBy(filepath.Dir(config), owners); err != nil {
		return fmt.Errorf("%w, so others could replace %s; keep it in a folder only root can change, such as /etc/telltale", err, config)
	}
	uid, gid, created, err := ensureUser()
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("created user %s (no shell, no home, no password)\n", agentUser)
	}
	// Root owns the config so the Agent can read its token but not change
	// where it reports to.
	if err := os.Chown(configPath, 0, gid); err != nil {
		return err
	}
	if err := os.Chmod(configPath, 0o640); err != nil {
		return err
	}
	fmt.Printf("made %s readable only by root and the %s group\n", configPath, agentUser)

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
	if _, err := command("launchctl", "bootout", "system/"+launchdLabel); err == nil { // an older one was loaded
		waitUnloaded()
	}
	if _, err := command("launchctl", "bootstrap", "system", plistPath); err != nil {
		return err
	}
	fmt.Printf("started %s as %s; logs go to %s\n", launchdLabel, agentUser, logPath)
	return nil
}

// sleep waits between checks in waitUnloaded; tests replace it.
var sleep = time.Sleep

// waitUnloaded waits (up to 10s) until launchd has removed the daemon.
// bootout returns before it has, and bootstrapping the daemon again before
// then fails with "Bootstrap failed: 5: Input/output error".
func waitUnloaded() {
	for range 100 {
		if _, err := command("launchctl", "print", "system/"+launchdLabel); err != nil {
			return
		}
		sleep(100 * time.Millisecond)
	}
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

// trustedOwners are the users who may own the Agent and the folders above it
// and its config: root, and the admin running the install with sudo (who owns
// /usr/local/bin on Intel Macs with Homebrew, and could become root anyway).
func trustedOwners() map[uint32]bool {
	owners := map[uint32]bool{0: true}
	if uid, err := strconv.ParseUint(os.Getenv("SUDO_UID"), 10, 32); err == nil {
		owners[uint32(uid)] = true
	}
	return owners
}

// changeableOnlyBy returns an error unless path and every folder above it are
// owned by one of owners and writable by no one else, other than the wheel
// and admin groups. path must have no symlinks in it.
func changeableOnlyBy(path string, owners map[uint32]bool) error {
	const wheel, admin = 0, 80
	for p := path; ; p = filepath.Dir(p) {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("cannot read the owner of %s", p)
		}
		switch perm := fi.Mode().Perm(); {
		case !owners[st.Uid]:
			return fmt.Errorf("%s is owned by user %d, not root", p, st.Uid)
		case perm&0o002 != 0:
			return fmt.Errorf("%s is writable by everyone", p)
		case perm&0o020 != 0 && st.Gid != wheel && st.Gid != admin:
			return fmt.Errorf("%s is writable by group %d", p, st.Gid)
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// ensureUser returns the IDs of the daemon user, creating it (and its group)
// if needed: hidden, shell /usr/bin/false, home /var/empty, and the password
// "*", which matches no password, so nobody can log in as it.
func ensureUser() (uid, gid int, created bool, err error) {
	if out, err := command("dscl", ".", "-read", "/Users/"+agentUser, "UniqueID", "PrimaryGroupID"); err == nil {
		uid, gid, err := parseIDs(out)
		if err != nil {
			return 0, 0, false, err
		}
		// Users made by v0.1.0 had no password set.
		_, err = command("dscl", ".", "-create", "/Users/"+agentUser, "Password", "*")
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
		{"-create", "/Users/" + agentUser, "Password", "*"},
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
