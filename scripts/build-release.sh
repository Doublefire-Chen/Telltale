#!/bin/sh
# Builds every release binary under one version into dist/, with SHA256SUMS.
# Run from the repo root after the web app is built and the GeoIP database is
# fetched (`make release` does both):
#
#	scripts/build-release.sh v1.2.3
set -eu

version="${1:?usage: $0 vX.Y.Z}"
case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*) echo "version must look like vX.Y.Z, got $version" >&2; exit 1 ;;
esac
[ -s internal/dashboard/geoip/data/dbip-country-lite.mmdb ] || { echo "no GeoIP database; run make geoip" >&2; exit 1; }
[ -s web/dist/app/index.html ] || { echo "no web app build; run make web" >&2; exit 1; }

pkg=github.com/Doublefire-Chen/Telltale/internal/version
ldflags="-s -w -X $pkg.Version=$version"
rm -rf dist && mkdir dist
if command -v sha256sum >/dev/null; then sum="sha256sum"; else sum="shasum -a 256"; fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

build() { # cmd goos goarch [extra ldflags]
	ext=""
	[ "$2" = windows ] && ext=.exe
	echo "building $1-$2-$3$ext"
	CGO_ENABLED=0 GOOS=$2 GOARCH=$3 go build -trimpath -ldflags "$ldflags ${4:-}" -o "dist/$1-$2-$3$ext" "./cmd/$1"
}

# An Agent's fingerprint (version.Build) is the hash of the Agent built with a
# placeholder version and no VCS stamp. It stays the same across releases that
# did not change the code built for that platform, so the Dashboard does not
# call such Agents outdated. Go's builds are reproducible, given the exact
# toolchain go.mod pins.
fingerprint() { # goos goarch
	CGO_ENABLED=0 GOOS=$1 GOARCH=$2 go build -trimpath -buildvcs=false -ldflags "-s -w -X $pkg.Version=v0.0.0-fingerprint" \
		-o "$work/agent" ./cmd/telltale-agent
	$sum "$work/agent" | awk '{ print $1 }'
}
builds=""
for goos in linux windows darwin; do
	for goarch in amd64 arm64; do
		fp=$(fingerprint "$goos" "$goarch")
		builds="$builds${builds:+,}$goos-$goarch=$fp"
		build telltale-agent "$goos" "$goarch" "-X $pkg.Build=$fp"
	done
done
# A fingerprint that differed between two identical builds would flag every
# Agent on every release; make sure a cold rebuild gives the same one.
[ "linux-amd64=$(GOCACHE="$work/cache" fingerprint linux amd64)" = "${builds%%,*}" ] ||
	{ echo "the Agent's fingerprint is not reproducible" >&2; exit 1; }

# The Dashboard carries the Agents' hashes, so the install commands it shows
# can pin the download on Hosts without cosign.
agents=$(cd dist && $sum -- telltale-agent-* | awk '{ sub(/^telltale-agent-/, "", $2); sub(/\.exe$/, "", $2); printf "%s%s=%s", sep, $2, $1; sep = "," }')
ldflags="$ldflags -X $pkg.AgentSHA256=$agents -X $pkg.AgentBuilds=$builds"
for goarch in amd64 arm64; do
	build telltale-dashboard linux "$goarch"
done

(cd dist && $sum -- * > ../SHA256SUMS.tmp) && mv SHA256SUMS.tmp dist/SHA256SUMS
cat dist/SHA256SUMS
