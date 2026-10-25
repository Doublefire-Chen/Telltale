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

// AgentSHA256 lists the SHA-256 of each Agent binary released with this
// build, as "linux-amd64=HASH,darwin-arm64=HASH,...". The release build sets
// it on the Dashboard, so the install commands it shows can pin the download
// without the Host needing cosign. Empty for other builds.
var AgentSHA256 = ""

// AgentHashes parses AgentSHA256 into a map from "os-arch" to hash, leaving
// out anything malformed.
func AgentHashes(list string) map[string]string {
	hashes := map[string]string{}
	for _, pair := range strings.Split(list, ",") {
		platform, hash, ok := strings.Cut(pair, "=")
		if ok && platform != "" && len(hash) == 64 && strings.Trim(hash, "0123456789abcdef") == "" {
			hashes[platform] = hash
		}
	}
	return hashes
}

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
