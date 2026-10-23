package agent

import "testing"

func TestSockstatCounts(t *testing.T) {
	v4 := []byte("sockets: used 412\nTCP: inuse 12 orphan 0 tw 3 alloc 15 mem 2\nUDP: inuse 4 mem 1\nUDPLITE: inuse 0\nRAW: inuse 0\nFRAG: inuse 0 memory 0\n")
	v6 := []byte("TCP6: inuse 5\nUDP6: inuse 2\nUDPLITE6: inuse 0\nRAW6: inuse 0\nFRAG6: inuse 0 memory 0\n")

	if got := sumInUse(v4, v6, "TCP"); got == nil || *got != 17 {
		t.Errorf("TCP = %v, want 17", got)
	}
	if got := sumInUse(v4, v6, "UDP"); got == nil || *got != 6 {
		t.Errorf("UDP = %v, want 6 (UDPLITE must not match)", got)
	}
	// IPv6 disabled: sockstat6 is missing, IPv4 alone still counts.
	if got := sumInUse(v4, nil, "TCP"); got == nil || *got != 12 {
		t.Errorf("TCP without IPv6 = %v, want 12", got)
	}
	if got := sumInUse([]byte("garbage"), v6, "TCP"); got != nil {
		t.Errorf("unreadable sockstat = %v, want unknown", *got)
	}
}
