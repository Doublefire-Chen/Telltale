#!/bin/sh
# End-to-end test of deploy/install.sh on OpenWrt, in a disposable OpenWrt
# container running procd. Needs Docker on an amd64 host; run from the repo
# root.
#
# It checks that install creates the telltale-agent user with OpenWrt's own
# helpers and a procd service that runs the Agent as that user, that the Agent
# reports to a Dashboard, that an upgrade keeps the config, and that uninstall
# removes everything again.
set -eu

NAME=telltale-openwrt-test
IMAGE=openwrt/rootfs:x86-64-v25.12.4
PORT=18480
WORK=$(mktemp -d)
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }
run_in() { docker exec "$NAME" sh -c "$1"; }

echo "building linux/amd64 binaries"
for cmd in telltale-agent telltale-dashboard; do
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X github.com/Doublefire-Chen/Telltale/internal/version.Version=v0.0.0-test' -o "$WORK/$cmd" "./cmd/$cmd"
done

docker run -d --name "$NAME" --privileged -p "127.0.0.1:$PORT:8080" "$IMAGE" /sbin/init >/dev/null
for _ in $(seq 30); do
	run_in 'ubus call service list >/dev/null 2>&1' && break
	sleep 1
done
run_in 'ubus call service list >/dev/null' || fail "procd did not come up"

docker cp "$WORK/telltale-agent" "$NAME:/tmp/telltale-agent"
docker cp "$WORK/telltale-dashboard" "$NAME:/tmp/telltale-dashboard"
docker cp deploy/install.sh "$NAME:/tmp/install.sh"

echo "starting a Dashboard in the container"
run_in 'mkdir -p /tmp/dash && printf "listen = [\"0.0.0.0:8080\"]\ndata_dir = \"/tmp/dash\"\n" > /tmp/dash.toml
	printf "openwrt test pw\nopenwrt test pw\n" | /tmp/telltale-dashboard -config /tmp/dash.toml admin set-password >/dev/null'
docker exec -d "$NAME" /tmp/telltale-dashboard -config /tmp/dash.toml
for _ in $(seq 20); do curl -fs -o /dev/null "http://127.0.0.1:$PORT/" && break; sleep 0.5; done
curl -fs -c "$WORK/jar" -o /dev/null -X POST -H 'Content-Type: application/json' \
	-d '{"username":"admin","password":"openwrt test pw"}' "http://127.0.0.1:$PORT/api/session" || fail "cannot log in to the Dashboard"
TOKEN=$(curl -fs -b "$WORK/jar" -X POST -H 'Content-Type: application/json' -d '{"name":""}' "http://127.0.0.1:$PORT/api/hosts" |
	sed 's/.*"token":"\([^"]*\)".*/\1/')

echo "installing the Agent"
run_in "printf %s $TOKEN | sh /tmp/install.sh install --dashboard-url http://127.0.0.1:8080 --token-file - --binary /tmp/telltale-agent --interval 1s"
run_in 'grep -q "^telltale-agent:x:[0-9]*:[0-9]*:telltale-agent:/nonexistent:/bin/false$" /etc/passwd' || fail "telltale-agent is not a no-shell user"
gid=$(run_in 'grep "^telltale-agent:" /etc/passwd | cut -d: -f4')
run_in "grep -q '^telltale-agent:x:$gid:' /etc/group" || fail "telltale-agent has no group of its own"
run_in 'ls -ln /etc/telltale/agent.toml' | awk -v g="$gid" '$1 != "-rw-r-----" || $3 != 0 || $4 != g { exit 1 }' ||
	fail "config is not root:telltale-agent 0640"
run_in 'test -x /etc/init.d/telltale-agent && ls /etc/rc.d/ | grep -q "^S99telltale-agent$"' || fail "the procd service is not enabled"
ok "install created the user, the config and an enabled procd service"

sleep 4
pid=$(run_in 'pidof telltale-agent') || { run_in 'logread | tail -20'; fail "the Agent is not running"; }
run_in "grep -q '^Uid:[[:space:]]*[1-9]' /proc/$pid/status" || fail "the Agent runs as root"
run_in "test \"\$(awk '/^CapEff/{print \$2}' /proc/$pid/status)\" = 0000000000000000" || fail "the Agent has capabilities"
ok "the Agent runs as telltale-agent with no capabilities"

STATUS=$(curl -fs -b "$WORK/jar" "http://127.0.0.1:$PORT/api/status")
printf '%s' "$STATUS" | grep -q '"online":true' || fail "Host is not online: $STATUS"
ok "the Agent reports"

run_in 'kill $(pidof telltale-agent)'
sleep 7
run_in 'pidof telltale-agent >/dev/null' || fail "procd did not restart the Agent"
ok "procd restarts the Agent"

OUT=$(run_in 'sh /tmp/install.sh install --binary /tmp/telltale-agent')
printf '%s' "$OUT" | grep -q "kept existing /etc/telltale/agent.toml" || fail "upgrade did not keep the config"
sleep 2
run_in 'pidof telltale-agent >/dev/null' || fail "the Agent is not running after the upgrade"
ok "upgrade keeps the config and restarts the Agent"

echo "uninstalling"
OUT=$(run_in 'sh /tmp/install.sh uninstall')
printf '%s\n' "$OUT"
for item in /etc/init.d/telltale-agent /usr/local/bin/telltale-agent /etc/telltale/agent.toml "user telltale-agent"; do
	printf '%s' "$OUT" | grep -q "removed $item" || fail "uninstall did not report removing $item"
done
run_in 'test ! -e /usr/local/bin/telltale-agent && test ! -e /etc/telltale && test ! -e /etc/init.d/telltale-agent &&
	! ls /etc/rc.d | grep -q telltale && ! grep -q telltale-agent /etc/passwd /etc/shadow /etc/group && ! pidof telltale-agent' ||
	fail "something was left behind"
ok "uninstall removed everything and said so"
echo PASS
