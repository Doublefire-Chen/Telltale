package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeDSCL answers dscl the way a Mac with users 0, 1, 300 and groups 0, 301 would.
func fakeDSCL(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	orig := command
	t.Cleanup(func() { command = orig })
	command = func(name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		switch call {
		case "dscl . -list /Users UniqueID":
			return "root  0\ndaemon 1\n_other 300\n", nil
		case "dscl . -list /Groups PrimaryGroupID":
			return "wheel 0\n_othergroup 301\n", nil
		case "dscl . -read /Users/_telltale-agent UniqueID PrimaryGroupID":
			return "", fmt.Errorf("eDSRecordNotFound")
		}
		return "", nil
	}
	return &calls
}

func TestFreeIDSkipsUsersAndGroups(t *testing.T) {
	fakeDSCL(t)
	id, err := freeID()
	if err != nil || id != 302 {
		t.Fatalf("freeID = %d, %v; want 302 (300 is a user, 301 a group)", id, err)
	}
}

func TestEnsureUserCreatesAHiddenShelllessAccount(t *testing.T) {
	calls := fakeDSCL(t)
	uid, gid, created, err := ensureUser()
	if err != nil || !created || uid != 302 || gid != 302 {
		t.Fatalf("ensureUser = %d, %d, %v, %v", uid, gid, created, err)
	}
	all := strings.Join(*calls, "\n")
	for _, want := range []string{
		"dscl . -create /Users/_telltale-agent UserShell /usr/bin/false",
		"dscl . -create /Users/_telltale-agent NFSHomeDirectory /var/empty",
		"dscl . -create /Users/_telltale-agent IsHidden 1",
		"dscl . -create /Users/_telltale-agent Password *",
		"dscl . -create /Groups/_telltale-agent PrimaryGroupID 302",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
}

func TestEnsureUserLocksAnExistingAccount(t *testing.T) {
	var calls []string
	orig := command
	t.Cleanup(func() { command = orig })
	command = func(name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if call == "dscl . -read /Users/_telltale-agent UniqueID PrimaryGroupID" {
			return "PrimaryGroupID: 302\nUniqueID: 302\n", nil
		}
		return "", nil
	}
	uid, gid, created, err := ensureUser()
	if err != nil || created || uid != 302 || gid != 302 {
		t.Fatalf("ensureUser = %d, %d, %v, %v", uid, gid, created, err)
	}
	if !slices.Contains(calls, "dscl . -create /Users/_telltale-agent Password *") {
		t.Errorf("existing user not given the password *: %q", calls)
	}
}

func TestParseIDs(t *testing.T) {
	uid, gid, err := parseIDs("PrimaryGroupID: 302\nUniqueID: 302\n")
	if err != nil || uid != 302 || gid != 302 {
		t.Fatalf("parseIDs = %d, %d, %v", uid, gid, err)
	}
	if _, _, err := parseIDs("UniqueID: 302\n"); err == nil {
		t.Fatal("missing group accepted")
	}
}

func TestLaunchdPlist(t *testing.T) {
	p := launchdPlist("/usr/local/bin/telltale-agent", "/etc/telltale/a&b.toml")
	if err := xml.Unmarshal(p, new(any)); err != nil {
		t.Fatalf("plist is not valid XML: %v\n%s", err, p)
	}
	s := string(p)
	for _, want := range []string{
		"<string>/usr/local/bin/telltale-agent</string>",
		"<string>/etc/telltale/a&amp;b.toml</string>",
		"<key>UserName</key>\n\t<string>_telltale-agent</string>",
		"<key>KeepAlive</key>\n\t<true/>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("plist lacks %q:\n%s", want, s)
		}
	}
}

func TestChangeableOnlyBy(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "telltale-agent")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	// The temporary folder is ours, and the folders above it root's.
	us := map[uint32]bool{0: true, uint32(os.Getuid()): true}
	if err := changeableOnlyBy(exe, us); err != nil {
		t.Fatalf("own file in own folder: %v", err)
	}
	if err := changeableOnlyBy(exe, map[uint32]bool{0: true}); err == nil || !strings.Contains(err.Error(), "not root") {
		t.Fatalf("file owned by another user: %v", err)
	}
	os.Chmod(dir, 0o777)
	defer os.Chmod(dir, 0o700)
	if err := changeableOnlyBy(exe, us); err == nil || !strings.Contains(err.Error(), dir+" is writable by everyone") {
		t.Fatalf("world-writable folder: %v", err)
	}
	os.Chmod(dir, 0o700)
	if os.Getgid() != 0 && os.Getgid() != 80 {
		os.Chmod(exe, 0o775)
		if err := changeableOnlyBy(exe, us); err == nil || !strings.Contains(err.Error(), "writable by group") {
			t.Fatalf("group-writable file: %v", err)
		}
	}
}

// TestWaitUnloadedWaitsForLaunchd checks that a reinstall bootstraps the
// daemon only once launchd has finished removing the old one.
func TestWaitUnloadedWaitsForLaunchd(t *testing.T) {
	orig, origSleep := command, sleep
	t.Cleanup(func() { command, sleep = orig, origSleep })
	var calls []string
	stillLoaded := 3
	command = func(name string, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if stillLoaded > 0 {
			stillLoaded--
			return "state = running", nil
		}
		return "", fmt.Errorf("Could not find service")
	}
	sleeps := 0
	sleep = func(time.Duration) { sleeps++ }

	waitUnloaded()
	if len(calls) != 4 || sleeps != 3 {
		t.Fatalf("checked %d times and slept %d times, want 4 and 3: %v", len(calls), sleeps, calls)
	}
}
