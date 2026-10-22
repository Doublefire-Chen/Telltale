#!/bin/sh
# End-to-end test of deploy/install.sh and both systemd units, in a disposable
# Debian container running real systemd. Needs Docker; run from the repo root
# after `make build` (the web app and GeoIP database get embedded).
#
# It checks that install creates exactly the documented paths, that the Agent
# runs unprivileged under its hardened unit and reports every metric to a
# Dashboard running under its own hardened unit, and that uninstall removes
# everything again.
set -eu

NAME=telltale-install-test
IMAGE=telltale-systemd-test
ARCH=$(docker info --format '{{.Architecture}}' | sed 's/x86_64/amd64/; s/aarch64/arm64/')
WORK=$(mktemp -d)
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }
run_in() { docker exec "$NAME" sh -c "$1"; }

echo "building linux/$ARCH binaries"
GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X github.com/Doublefire-Chen/Telltale/internal/version.Version=v0.0.0-test' -o "$WORK/telltale-agent" ./cmd/telltale-agent
GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X github.com/Doublefire-Chen/Telltale/internal/version.Version=v0.0.0-test' -o "$WORK/telltale-dashboard" ./cmd/telltale-dashboard

docker build -q -t "$IMAGE" - >/dev/null <<'DOCKERFILE'
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends systemd systemd-sysv curl ca-certificates procps && rm -rf /var/lib/apt/lists/*
CMD ["/sbin/init"]
DOCKERFILE
docker run -d --name "$NAME" --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock "$IMAGE" >/dev/null
for _ in $(seq 30); do
	state=$(docker exec "$NAME" systemctl is-system-running 2>/dev/null || true)
	case "$state" in running | degraded) break ;; esac
	sleep 1
done

docker cp "$WORK/telltale-agent" "$NAME:/tmp/telltale-agent"
docker cp "$WORK/telltale-dashboard" "$NAME:/tmp/telltale-dashboard"
docker cp deploy/install.sh "$NAME:/tmp/install.sh"
docker cp deploy/systemd/telltale-dashboard.service "$NAME:/etc/systemd/system/telltale-dashboard.service"
docker cp deploy/dashboard.toml "$NAME:/tmp/dashboard.toml"

echo "starting the Dashboard under its hardened unit"
run_in 'useradd --system --user-group --no-create-home --shell /usr/sbin/nologin telltale
	install -m 0755 /tmp/telltale-dashboard /usr/local/bin/telltale-dashboard
	mkdir -p /etc/telltale && install -m 0644 /tmp/dashboard.toml /etc/telltale/dashboard.toml
	printf "container test pw\ncontainer test pw\n" | telltale-dashboard admin set-password >/dev/null
	chown -R telltale:telltale /var/lib/telltale
	systemctl daemon-reload && systemctl start telltale-dashboard
	for i in $(seq 20); do curl -fs -o /dev/null http://127.0.0.1:8080/ && break; sleep 0.5; done'
run_in 'systemctl is-active -q telltale-dashboard' || { run_in 'journalctl -u telltale-dashboard --no-pager | tail -20'; fail "Dashboard unit not active"; }
ok "Dashboard runs under its hardened unit"
run_in 'test "$(stat -c %U:%a /var/lib/telltale)" = telltale:700' || fail "/var/lib/telltale is not telltale-owned 0700"
# Inside the Dashboard's mount namespace even root cannot write outside the
# data directory: ProtectSystem=strict made everything else read-only.
run_in 'nsenter -t $(systemctl show -p MainPID --value telltale-dashboard) -m touch /etc/telltale/x 2>/dev/null' && fail "Dashboard can write outside its data directory"
run_in 'nsenter -t $(systemctl show -p MainPID --value telltale-dashboard) -m touch /var/lib/telltale/x && rm /var/lib/telltale/x' || fail "Dashboard cannot write its data directory"
ok "Dashboard can write only to /var/lib/telltale"

TOKEN=$(run_in 'curl -fs -c /tmp/jar -o /dev/null -X POST -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"container test pw\"}" http://127.0.0.1:8080/api/session
	C="telltale_session=$(awk "\$6==\"telltale_session\"{print \$7}" /tmp/jar)"
	curl -fs -b "$C" -X POST -H "Content-Type: application/json" -d "{\"name\":\"\"}" http://127.0.0.1:8080/api/hosts' | sed 's/.*"token":"\([^"]*\)".*/\1/')

echo "installing the Agent"
run_in 'touch /tmp/marker && sleep 1'
run_in "sh /tmp/install.sh install --dashboard-url http://127.0.0.1:8080 --token $TOKEN --binary /tmp/telltale-agent --interval 1s"
CHANGED=$(run_in 'find / -xdev \( -path /proc -o -path /sys -o -path /tmp -o -path /var/tmp -o -path /run -o -path /var/lib/telltale -o -path /var/log -o -path /var/lib/systemd -o -path /var/cache \) -prune -o -newer /tmp/marker -print' | sort)
EXPECTED=$(printf '%s\n' /etc /etc/.pwd.lock /etc/group /etc/group- /etc/gshadow /etc/gshadow- /etc/passwd /etc/passwd- /etc/shadow /etc/shadow- \
	/etc/systemd/system /etc/systemd/system/multi-user.target.wants /etc/systemd/system/multi-user.target.wants/telltale-agent.service \
	/etc/systemd/system/telltale-agent.service /etc/telltale /etc/telltale/agent.toml /usr/local/bin /usr/local/bin/telltale-agent | sort)
UNEXPECTED=$(printf '%s\n' "$CHANGED" | grep -vxF "$EXPECTED" || true)
[ -z "$UNEXPECTED" ] || fail "install touched paths outside its own: $UNEXPECTED"
ok "install wrote only its own paths (plus the user database)"

run_in 'test "$(stat -c %U:%a /etc/telltale/agent.toml)" = telltale-agent:600' || fail "config is not 0600 telltale-agent"
run_in 'getent passwd telltale-agent | grep -q ":/usr/sbin/nologin$"' || fail "telltale-agent has a login shell"
ok "config is 0600 and the user has no shell"

sleep 4
run_in 'test "$(ps -o user= -C telltale-agent)" = telltale-agent' || { run_in 'journalctl -u telltale-agent --no-pager | tail -20'; fail "Agent is not running as telltale-agent"; }
run_in 'test "$(awk "/^CapEff/{print \$2}" /proc/$(pgrep -x telltale-agent)/status)" = 0000000000000000' || fail "Agent has capabilities"
ok "Agent runs as telltale-agent with no capabilities"

STATUS=$(run_in 'C="telltale_session=$(awk "\$6==\"telltale_session\"{print \$7}" /tmp/jar)"; curl -fs -b "$C" http://127.0.0.1:8080/api/status')
for field in cpu_percent memory_used swap_used load1 disk_used net_rx_bytes rx_rate '"hostname"' boot_time memory_total; do
	printf '%s' "$STATUS" | grep -q "$field" || fail "Dashboard lacks $field from the unprivileged Agent: $STATUS"
done
printf '%s' "$STATUS" | grep -q '"online":true' || fail "Host is not online"
ok "the unprivileged Agent reports every metric"

echo "uninstalling"
OUT=$(run_in 'sh /tmp/install.sh uninstall')
printf '%s\n' "$OUT"
for item in /etc/systemd/system/telltale-agent.service /usr/local/bin/telltale-agent /etc/telltale/agent.toml "user telltale-agent"; do
	printf '%s' "$OUT" | grep -q "removed $item" || fail "uninstall did not report removing $item"
done
run_in 'test ! -e /usr/local/bin/telltale-agent && test ! -e /etc/telltale/agent.toml && test ! -e /etc/systemd/system/telltale-agent.service && ! id telltale-agent 2>/dev/null && ! pgrep -x telltale-agent' || fail "something was left behind"
ok "uninstall removed everything and said so"
echo PASS
