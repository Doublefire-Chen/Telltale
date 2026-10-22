package main

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
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
		"dscl . -create /Groups/_telltale-agent PrimaryGroupID 302",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
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
