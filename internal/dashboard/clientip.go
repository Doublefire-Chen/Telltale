package dashboard

import (
	"net/http"
	"net/netip"
	"strings"
)

// DefaultTrustedProxies are the reverse proxies whose forwarded-for headers
// are believed by default: a proxy on the same machine.
var DefaultTrustedProxies = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("::1/128"),
}

// clientIP returns the address of the client that made r. Forwarded-for
// headers are honoured only when the connection comes from a trusted proxy,
// so nobody else can fake their address (and so their Host's country).
func (s *Server) clientIP(r *http.Request) netip.Addr {
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	addr := remote.Addr().Unmap()
	if !s.trusted(addr) {
		return addr
	}
	// Walk X-Forwarded-For from the right (the hop nearest us), skipping
	// trusted proxies; the first untrusted hop is the client.
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		hop = hop.Unmap()
		if !s.trusted(hop) || i == 0 {
			return hop
		}
	}
	if real, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
		return real.Unmap()
	}
	return addr
}

func (s *Server) trusted(addr netip.Addr) bool {
	for _, p := range s.trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
