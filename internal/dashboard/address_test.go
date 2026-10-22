package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/geoip/geoiptest"
	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

type addressView struct {
	Name            string   `json:"name"`
	IPv4            string   `json:"ipv4"`
	IPv6            string   `json:"ipv6"`
	Countries       []string `json:"countries"`
	CountryOverride string   `json:"country_override"`
}

func (h *harness) addresses(session *http.Cookie) map[string]addressView {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/status", "", session)
	expectStatus(h.t, resp, http.StatusOK)
	var body struct {
		Hosts []addressView `json:"hosts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		h.t.Fatal(err)
	}
	out := map[string]addressView{}
	for _, v := range body.Hosts {
		out[v.Name] = v
	}
	return out
}

// hostInfoFrom sends Host Info as if it arrived from remote (an "ip:port"),
// through a proxy that adds headers, by calling the handler directly.
func hostInfoFrom(t *testing.T, srv http.Handler, token, remote string, headers map[string]string) int {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, v1.HostInfoPath, strings.NewReader(`{"hostname":"x"}`))
	r.RemoteAddr = remote
	r.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w.Code
}

// addressHarness is a harness whose handler is also reachable directly, so
// tests can set the connection's source address.
func addressHarness(t *testing.T, proxies []netip.Prefix) (*harness, http.Handler) {
	t.Helper()
	geo := geoiptest.New(t, map[string]string{
		"203.0.113.0/24": "JP", "198.51.100.0/24": "US", "2001:db8:1::/48": "JP", "2001:db8:2::/48": "DE",
	})
	var handler http.Handler
	h := newHarness(t, nil, func(o *dashboard.Options) {
		o.GeoIP = geo
		o.TrustedProxies = proxies
	})
	handler = h.srv.Config.Handler
	return h, handler
}

func TestHostAddressesFromEachIPFamily(t *testing.T) {
	h, handler := addressHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	same := h.addHost(session, "same")
	split := h.addHost(session, "split")
	v4only := h.addHost(session, "v4only")

	for _, c := range []struct{ token, remote string }{
		{same.Token, "203.0.113.7:50000"}, {same.Token, "[2001:db8:1::7]:50000"},
		{split.Token, "198.51.100.9:50000"}, {split.Token, "[2001:db8:2::9]:50000"},
		{v4only.Token, "[::ffff:203.0.113.8]:50000"},
	} {
		if code := hostInfoFrom(t, handler, c.token, c.remote, nil); code != http.StatusNoContent {
			t.Fatalf("Host Info from %s: %d", c.remote, code)
		}
	}

	got := h.addresses(session)
	if v := got["same"]; v.IPv4 != "203.0.113.7" || v.IPv6 != "2001:db8:1::7" || strings.Join(v.Countries, ",") != "JP" {
		t.Errorf("same: %+v, want both addresses and one JP flag", v)
	}
	if v := got["split"]; strings.Join(v.Countries, ",") != "US,DE" {
		t.Errorf("split: countries %v, want US,DE (two flags)", v.Countries)
	}
	if v := got["v4only"]; v.IPv4 != "203.0.113.8" || v.IPv6 != "" {
		t.Errorf("v4only: %+v, want an unmapped IPv4 and no IPv6", v)
	}
}

func TestForwardedHeadersOnlyFromTrustedProxies(t *testing.T) {
	h, handler := addressHarness(t, nil) // default: loopback proxies are trusted
	h.setPassword(testPassword)
	session := h.loginOK()
	viaProxy := h.addHost(session, "via-proxy")
	chain := h.addHost(session, "chain")
	realIP := h.addHost(session, "real-ip")
	spoofed := h.addHost(session, "spoofed")

	hostInfoFrom(t, handler, viaProxy.Token, "127.0.0.1:40000", map[string]string{"X-Forwarded-For": "203.0.113.7"})
	// A client-supplied hop before the real client is ignored; the rightmost untrusted hop wins.
	hostInfoFrom(t, handler, chain.Token, "127.0.0.1:40000", map[string]string{"X-Forwarded-For": "198.51.100.66, 203.0.113.9, 127.0.0.1"})
	hostInfoFrom(t, handler, realIP.Token, "[::1]:40000", map[string]string{"X-Real-IP": "2001:db8:2::5"})
	// Not from a trusted proxy: the header is ignored.
	hostInfoFrom(t, handler, spoofed.Token, "198.51.100.20:40000", map[string]string{"X-Forwarded-For": "203.0.113.1"})

	got := h.addresses(session)
	if v := got["via-proxy"].IPv4; v != "203.0.113.7" {
		t.Errorf("via trusted proxy: %q, want the forwarded client", v)
	}
	if v := got["chain"].IPv4; v != "203.0.113.9" {
		t.Errorf("forwarded chain: %q, want the rightmost untrusted hop", v)
	}
	if v := got["real-ip"].IPv6; v != "2001:db8:2::5" {
		t.Errorf("X-Real-IP: %q", v)
	}
	if v := got["spoofed"].IPv4; v != "198.51.100.20" {
		t.Errorf("untrusted forwarded header: %q, want the connection's own address", v)
	}
}

func TestTrustedProxiesAreConfigurable(t *testing.T) {
	h, handler := addressHarness(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	h.setPassword(testPassword)
	session := h.loginOK()
	a := h.addHost(session, "a")
	b := h.addHost(session, "b")

	hostInfoFrom(t, handler, a.Token, "10.1.2.3:40000", map[string]string{"X-Forwarded-For": "203.0.113.7"})
	hostInfoFrom(t, handler, b.Token, "127.0.0.1:40000", map[string]string{"X-Forwarded-For": "203.0.113.8"})
	got := h.addresses(session)
	if got["a"].IPv4 != "203.0.113.7" || got["b"].IPv4 != "127.0.0.1" {
		t.Fatalf("a=%q b=%q: only the configured proxy range should be trusted", got["a"].IPv4, got["b"].IPv4)
	}
}

func TestCountryOverride(t *testing.T) {
	h, handler := addressHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "anycast")
	hostInfoFrom(t, handler, host.Token, "198.51.100.9:50000", nil)

	put := func(country string) *http.Response {
		body, _ := json.Marshal(map[string]string{"country": country})
		return h.do(http.MethodPut, "/api/hosts/"+host.ID+"/country", string(body), session)
	}
	expectStatus(t, put("jp"), http.StatusNoContent)
	if v := h.addresses(session)["anycast"]; strings.Join(v.Countries, ",") != "JP" || v.CountryOverride != "JP" {
		t.Fatalf("override: %+v, want JP replacing GeoIP's US", v)
	}
	expectStatus(t, put(""), http.StatusNoContent)
	if v := h.addresses(session)["anycast"]; strings.Join(v.Countries, ",") != "US" {
		t.Fatalf("cleared override: %v, want GeoIP's US back", v.Countries)
	}

	expectError(t, put("Japan"), http.StatusBadRequest, "invalid_country")
	body, _ := json.Marshal(map[string]string{"country": "JP"})
	expectError(t, h.do(http.MethodPut, "/api/hosts/nope/country", string(body), session), http.StatusNotFound, "not_found")
	expectError(t, h.do(http.MethodPut, "/api/hosts/"+host.ID+"/country", string(body), nil), http.StatusUnauthorized, "unauthenticated")
}

func TestStaleAddressesDisappear(t *testing.T) {
	h, handler := addressHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "lost-v6")
	hostInfoFrom(t, handler, host.Token, "203.0.113.7:50000", nil)
	hostInfoFrom(t, handler, host.Token, "[2001:db8:2::1]:50000", nil)

	// Two hours on, only IPv4 keeps arriving: IPv6 is still shown...
	h.clock.Advance(2 * time.Hour)
	hostInfoFrom(t, handler, host.Token, "203.0.113.7:50000", nil)
	if v := h.addresses(session)["lost-v6"]; v.IPv6 == "" {
		t.Fatal("IPv6 dropped too early")
	}
	// ...but after more than three hours without it, it is gone.
	h.clock.Advance(90 * time.Minute)
	hostInfoFrom(t, handler, host.Token, "203.0.113.7:50000", nil)
	if v := h.addresses(session)["lost-v6"]; v.IPv6 != "" || strings.Join(v.Countries, ",") != "JP" {
		t.Fatalf("after 3.5h: %+v, want IPv6 and its DE flag gone", v)
	}
}

func TestUnknownCountryWithoutGeoIP(t *testing.T) {
	h := newHarness(t, nil)
	h.setPassword(testPassword)
	session := h.loginOK()
	host := h.addHost(session, "x")
	hostInfoFrom(t, h.srv.Config.Handler, host.Token, "203.0.113.7:50000", nil)
	if v := h.addresses(session)["x"]; v.IPv4 != "203.0.113.7" || len(v.Countries) != 0 {
		t.Fatalf("%+v, want the address with no countries", v)
	}
}
