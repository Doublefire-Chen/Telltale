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

ldflags="-s -w -X github.com/Doublefire-Chen/Telltale/internal/version.Version=$version"
rm -rf dist && mkdir dist

build() { # cmd goos goarch
	ext=""
	[ "$2" = windows ] && ext=.exe
	echo "building $1-$2-$3$ext"
	CGO_ENABLED=0 GOOS=$2 GOARCH=$3 go build -trimpath -ldflags "$ldflags" -o "dist/$1-$2-$3$ext" "./cmd/$1"
}
for goos in linux windows darwin; do
	for goarch in amd64 arm64; do
		build telltale-agent "$goos" "$goarch"
	done
done
for goarch in amd64 arm64; do
	build telltale-dashboard linux "$goarch"
done

if command -v sha256sum >/dev/null; then sum="sha256sum"; else sum="shasum -a 256"; fi
(cd dist && $sum -- * > ../SHA256SUMS.tmp) && mv SHA256SUMS.tmp dist/SHA256SUMS
cat dist/SHA256SUMS
