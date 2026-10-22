#!/bin/sh
# Downloads the DB-IP Lite country database (CC BY 4.0, https://db-ip.com)
# for embedding into the Dashboard. Tries this month, then last month, since
# a new month's file is published a few days in.
set -eu

out="${1:-internal/dashboard/geoip/data/dbip-country-lite.mmdb}"
this_month=$(date -u +%Y-%m)
last_month=$(date -u -v-1m +%Y-%m 2>/dev/null || date -u -d "$(date -u +%Y-%m-15) -1 month" +%Y-%m)

for month in "$this_month" "$last_month"; do
	url="https://download.db-ip.com/free/dbip-country-lite-$month.mmdb.gz"
	if curl -fsSL "$url" -o "$out.gz.tmp"; then
		gunzip -c "$out.gz.tmp" > "$out.tmp"
		rm -f "$out.gz.tmp"
		mv "$out.tmp" "$out"
		echo "fetched $url -> $out"
		exit 0
	fi
done
rm -f "$out.gz.tmp" "$out.tmp"
echo "could not download the DB-IP Lite country database" >&2
exit 1
