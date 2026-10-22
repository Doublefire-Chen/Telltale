// Package geoip resolves IP addresses to countries using a local MaxMind-format
// database. No lookup ever leaves the Dashboard (ADR 0004).
//
// Release builds embed the DB-IP Lite country database
// (https://db-ip.com, CC BY 4.0), downloaded by `make geoip` into data/.
package geoip

import (
	"embed"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"strings"

	"github.com/oschwald/maxminddb-golang/v2"
)

// EmbeddedFile is the database file name inside data/.
const EmbeddedFile = "dbip-country-lite.mmdb"

//go:embed all:data
var data embed.FS

// DB looks up countries. A nil *DB knows no countries.
type DB struct {
	reader *maxminddb.Reader
}

// Open loads the database at path, or the embedded one if path is empty. With
// no path and nothing embedded it returns (nil, nil): countries stay unknown.
func Open(path string) (*DB, error) {
	var b []byte
	var err error
	if path != "" {
		b, err = os.ReadFile(path)
	} else {
		b, err = fs.ReadFile(data, "data/"+EmbeddedFile)
		if err != nil {
			return nil, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read GeoIP database: %w", err)
	}
	return FromBytes(b)
}

// FromBytes loads a database from its contents.
func FromBytes(b []byte) (*DB, error) {
	r, err := maxminddb.OpenBytes(b)
	if err != nil {
		return nil, fmt.Errorf("open GeoIP database: %w", err)
	}
	return &DB{reader: r}, nil
}

// Country returns the ISO 3166-1 alpha-2 code for ip (e.g. "JP"), or "" if unknown.
func (db *DB) Country(ip netip.Addr) string {
	if db == nil || !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return ""
	}
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := db.reader.Lookup(ip.Unmap()).Decode(&record); err != nil {
		return ""
	}
	return strings.ToUpper(record.Country.ISOCode)
}
