//go:build !linux && !windows

package agent

import "context"

// connections is unknown here: on macOS listing sockets means running lsof,
// far too costly to do for every Report.
func connections(context.Context) (tcp, udp *uint64) { return nil, nil }
