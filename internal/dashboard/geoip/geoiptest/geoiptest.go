// Package geoiptest builds small GeoIP databases for tests.
package geoiptest

import (
	"bytes"
	"net"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"

	"github.com/Doublefire-Chen/Telltale/internal/dashboard/geoip"
)

// New builds a database mapping each CIDR to an ISO country code.
func New(t testing.TB, countries map[string]string) *geoip.DB {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "DBIP-Country-Lite", IncludeReservedNetworks: true})
	if err != nil {
		t.Fatal(err)
	}
	for cidr, code := range countries {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		record := mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String(code)}}
		if err := w.Insert(network, record); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	db, err := geoip.FromBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return db
}
