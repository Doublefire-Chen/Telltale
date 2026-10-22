package geoip_test

import (
	"net/netip"
	"testing"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/geoip"
	"github.com/Doublefire-Chen/Telltale/internal/dashboard/geoip/geoiptest"
)

func TestCountry(t *testing.T) {
	db := geoiptest.New(t, map[string]string{"203.0.113.0/24": "jp", "2001:db8::/32": "SE"})
	for ip, want := range map[string]string{
		"203.0.113.7":        "JP",
		"::ffff:203.0.113.7": "JP",
		"2001:db8::1":        "SE",
		"198.51.100.1":       "",
		"10.0.0.1":           "",
		"127.0.0.1":          "",
	} {
		if got := db.Country(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Country(%s) = %q, want %q", ip, got, want)
		}
	}
	var none *geoip.DB
	if none.Country(netip.MustParseAddr("203.0.113.7")) != "" {
		t.Error("nil DB returned a country")
	}
}

func TestOpenWithoutEmbeddedDatabase(t *testing.T) {
	// Checkouts without `make geoip` embed no database; that must not fail.
	if _, err := geoip.Open(""); err != nil {
		t.Fatal(err)
	}
	if _, err := geoip.Open("/nonexistent/db.mmdb"); err == nil {
		t.Fatal("missing configured database accepted")
	}
}
