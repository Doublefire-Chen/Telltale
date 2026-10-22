#!/bin/sh
# End-to-end test of deploy/nginx/telltale.conf: nginx terminating HTTPS in
# front of the Dashboard, in a disposable container with IPv6 enabled. Needs
# Docker; run from the repo root after `make build`.
#
# It checks that the Agent reaches the Dashboard through nginx over HTTP/2 and
# over both IP families (the Dashboard learns 127.0.0.1 and ::1), that the
# Status page's WebSocket upgrades through nginx, and that a client cannot
# fake its address with its own X-Forwarded-For header.
set -eu

NAME=telltale-nginx-test
IMAGE=telltale-nginx-test
PORT=18443
ARCH=$(docker info --format '{{.Architecture}}' | sed 's/x86_64/amd64/; s/aarch64/arm64/')
WORK=$(mktemp -d)
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }
run_in() { docker exec "$NAME" sh -c "$1"; }

echo "building linux/$ARCH binaries"
for cmd in telltale-agent telltale-dashboard; do
	GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o "$WORK/$cmd" "./cmd/$cmd"
done

docker build -q -t "$IMAGE" - >/dev/null <<'DOCKERFILE'
FROM nginx:stable
RUN apt-get update && apt-get install -y --no-install-recommends openssl ca-certificates curl && rm -rf /var/lib/apt/lists/*
DOCKERFILE
docker run -d --name "$NAME" -p "127.0.0.1:$PORT:443" \
	--sysctl net.ipv6.conf.all.disable_ipv6=0 --sysctl net.ipv6.conf.lo.disable_ipv6=0 \
	"$IMAGE" sleep infinity >/dev/null
docker cp "$WORK/telltale-agent" "$NAME:/usr/local/bin/telltale-agent"
docker cp "$WORK/telltale-dashboard" "$NAME:/usr/local/bin/telltale-dashboard"
docker cp deploy/nginx/telltale.conf "$NAME:/tmp/telltale.conf"

echo "setting up a local CA, nginx and the Dashboard"
run_in 'set -e
	mkdir -p /etc/telltale-test && cd /etc/telltale-test
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 -subj "/CN=Telltale test CA" -keyout ca.key -out ca.crt 2>/dev/null
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=localhost" -keyout server.key -out server.csr 2>/dev/null
	printf "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1\n" > san.ext
	openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 1 -extfile san.ext -out server.crt 2>/dev/null
	cp ca.crt /usr/local/share/ca-certificates/telltale-test.crt && update-ca-certificates >/dev/null
	sed -e "s/status.example.com/localhost/g" \
	    -e "s#/etc/letsencrypt/live/localhost/fullchain.pem#/etc/telltale-test/server.crt#" \
	    -e "s#/etc/letsencrypt/live/localhost/privkey.pem#/etc/telltale-test/server.key#" \
	    /tmp/telltale.conf > /etc/nginx/conf.d/default.conf
	rm -f /var/log/nginx/access.log /var/log/nginx/error.log # the image links them to stdout
	nginx -t 2>/dev/null && nginx
	printf "nginx test pw\nnginx test pw\n" | telltale-dashboard admin set-password >/dev/null
	nohup telltale-dashboard > /var/log/dashboard.log 2>&1 &
	for i in $(seq 20); do curl -fs -o /dev/null https://localhost/ && break; sleep 0.5; done'
ok "nginx accepts the sample config and serves the Dashboard over HTTPS"

TOKEN=$(run_in 'curl -fs -c /tmp/jar -o /dev/null -X POST -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"nginx test pw\"}" https://localhost/api/session
	C="telltale_session=$(awk "\$6==\"telltale_session\"{print \$7}" /tmp/jar)"
	curl -fs -b "$C" -X POST -H "Content-Type: application/json" -d "{\"name\":\"via-nginx\"}" https://localhost/api/hosts' | sed 's/.*"token":"\([^"]*\)".*/\1/')
run_in "printf 'dashboard_url = \"https://localhost\"\ntoken = \"$TOKEN\"\ninterval = \"1s\"\n' > /tmp/agent.toml
	nohup telltale-agent -config /tmp/agent.toml > /var/log/agent.log 2>&1 &"
sleep 5

STATUS=$(run_in 'C="telltale_session=$(awk "\$6==\"telltale_session\"{print \$7}" /tmp/jar)"; curl -fs -b "$C" https://localhost/api/status')
printf '%s' "$STATUS" | grep -q '"online":true' || { run_in 'cat /var/log/agent.log'; fail "Agent did not report through nginx: $STATUS"; }
printf '%s' "$STATUS" | grep -q '"ipv4":"127.0.0.1"' || fail "Dashboard did not learn the IPv4 address: $STATUS"
printf '%s' "$STATUS" | grep -q '"ipv6":"::1"' || fail "Dashboard did not learn the IPv6 address: $STATUS"
ok "the Agent reports through nginx, and Host Info arrives over IPv4 and IPv6"

run_in 'grep "POST /api/v1/report HTTP/2.0" /var/log/nginx/access.log >/dev/null' || fail "Reports did not use HTTP/2"
ok "Reports use HTTP/2"

UPGRADE=$(run_in 'C="telltale_session=$(awk "\$6==\"telltale_session\"{print \$7}" /tmp/jar)"
	curl -s -i --http1.1 --max-time 2 -b "$C" -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Sec-WebSocket-Version: 13" \
		-H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" -H "Origin: https://localhost" https://localhost/api/live' | head -1 || true)
printf '%s' "$UPGRADE" | grep -q " 101 " || fail "WebSocket did not upgrade through nginx: $UPGRADE"
ok "the Status page's WebSocket upgrades through nginx"

# From outside the container the client is not loopback, so it is not a
# trusted proxy: its own X-Forwarded-For must not decide its address.
curl -sk -o /dev/null -X POST -H "Authorization: Bearer $TOKEN" -H "X-Forwarded-For: 203.0.113.66" \
	-d '{"hostname":"spoof"}' "https://localhost:$PORT/api/v1/host-info"
STATUS=$(run_in 'C="telltale_session=$(awk "\$6==\"telltale_session\"{print \$7}" /tmp/jar)"; curl -fs -b "$C" https://localhost/api/status')
printf '%s' "$STATUS" | grep -q '203.0.113.66' && fail "a client faked its address through nginx: $STATUS"
ok "a client cannot fake its address with X-Forwarded-For"
echo PASS
