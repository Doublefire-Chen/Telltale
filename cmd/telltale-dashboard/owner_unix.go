//go:build unix

package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// checkDataDirOwner refuses to run as root against a data directory owned by
// another user: SQLite files root created there would be unreadable to the
// Dashboard's service account.
func checkDataDirOwner(dataDir string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return nil // not created yet
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 {
		return nil
	}
	name := strconv.Itoa(int(st.Uid))
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	return fmt.Errorf("%s belongs to %s; run this as that user: sudo -u %s telltale-dashboard admin set-password", dataDir, name, name)
}
