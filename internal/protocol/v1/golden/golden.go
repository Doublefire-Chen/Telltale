// Package golden holds the request bodies released Agents send, one directory
// per release. Only tests import it, to prove the protocol only grows (ADR 0002).
package golden

import (
	"embed"
	"io/fs"
	"path"
	"sort"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

//go:embed v*
var payloads embed.FS

// Payload is one recorded request body from a released Agent.
type Payload struct {
	// Version is the Agent release that sent it, e.g. "v0.1.0".
	Version string
	// Path is the endpoint it was sent to: v1.ReportPath or v1.HostInfoPath.
	Path string
	Body []byte
}

var files = []struct{ name, path string }{
	{"host_info.json", v1.HostInfoPath},
	{"report.json", v1.ReportPath},
}

// All returns every recorded payload, by version (in lexical order).
func All() ([]Payload, error) {
	dirs, err := fs.ReadDir(payloads, ".")
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, d := range dirs {
		if d.IsDir() {
			versions = append(versions, d.Name())
		}
	}
	sort.Strings(versions)
	var out []Payload
	for _, v := range versions {
		for _, f := range files {
			b, err := fs.ReadFile(payloads, path.Join(v, f.name))
			if err != nil {
				return nil, err
			}
			out = append(out, Payload{Version: v, Path: f.path, Body: b})
		}
	}
	return out, nil
}
