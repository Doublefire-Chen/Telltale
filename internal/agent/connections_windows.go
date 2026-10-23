package agent

import (
	"context"

	"github.com/shirou/gopsutil/v4/net"
)

// connections counts sockets from the system's TCP and UDP tables
// (GetExtendedTcpTable and GetExtendedUdpTable): one call each, no process
// lookups.
func connections(ctx context.Context) (tcp, udp *uint64) {
	return count(ctx, "tcp"), count(ctx, "udp")
}

func count(ctx context.Context, kind string) *uint64 {
	conns, err := net.ConnectionsWithContext(ctx, kind)
	if err != nil {
		return nil
	}
	n := uint64(len(conns))
	return &n
}
