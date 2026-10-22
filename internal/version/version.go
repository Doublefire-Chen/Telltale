// Package version holds Telltale's release version, set at build time:
//
//	go build -ldflags "-X github.com/Doublefire-Chen/Telltale/internal/version.Version=v1.2.3"
//
// Agent and Dashboard are released together under one version.
package version

import (
	"strconv"
	"strings"
)

// Version is the release version, or "dev" for an unreleased build.
var Version = "dev"

// Older reports whether release version a is older than b. Versions look like
// "v1.2.3" (a leading "v" and any "-pre" or "+build" suffix are ignored).
// Anything unparseable, such as "dev", is never considered older.
func Older(a, b string) bool {
	va, okA := parse(a)
	vb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] < vb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
