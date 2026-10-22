//go:build !windows && !darwin

package main

import (
	"errors"

	"github.com/Doublefire-Chen/Telltale/internal/agent"
)

var errUseInstaller = errors.New("on Linux, install the Agent with deploy/install.sh")

func installService(string) error             { return errUseInstaller }
func uninstallService(string, bool) error     { return errUseInstaller }
func runAsService(agent.Config) (bool, error) { return false, nil }
