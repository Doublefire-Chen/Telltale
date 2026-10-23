package agent

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
)

// connections reads the kernel's socket totals from /proc/net/sockstat and
// sockstat6: two tiny files whose cost does not grow with the number of
// connections, unlike listing every socket.
func connections(context.Context) (tcp, udp *uint64) {
	v4, err := os.ReadFile("/proc/net/sockstat")
	if err != nil {
		return nil, nil
	}
	v6, _ := os.ReadFile("/proc/net/sockstat6") // absent when IPv6 is disabled
	return sumInUse(v4, v6, "TCP"), sumInUse(v4, v6, "UDP")
}

// sumInUse adds the "inuse" counts of proto ("TCP") in sockstat and of its
// IPv6 counterpart ("TCP6") in sockstat6.
func sumInUse(v4, v6 []byte, proto string) *uint64 {
	n, ok := inUse(v4, proto)
	if !ok {
		return nil
	}
	if m, ok := inUse(v6, proto+"6"); ok {
		n += m
	}
	return &n
}

// inUse finds a line such as "TCP: inuse 12 orphan 0 tw 3 alloc 15 mem 2"
// and returns its inuse value.
func inUse(sockstat []byte, proto string) (uint64, bool) {
	sc := bufio.NewScanner(bytes.NewReader(sockstat))
	for sc.Scan() {
		rest, ok := strings.CutPrefix(sc.Text(), proto+": ")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		for i := 0; i+1 < len(fields); i += 2 {
			if fields[i] == "inuse" {
				n, err := strconv.ParseUint(fields[i+1], 10, 64)
				return n, err == nil
			}
		}
	}
	return 0, false
}
