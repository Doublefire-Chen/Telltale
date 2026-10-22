#!/bin/sh
# Telltale Agent installer for Linux.
#
#   install.sh install   --dashboard-url URL --token TOKEN (--binary PATH | --version vX.Y.Z) [--interval 2s]
#   install.sh install   (--binary PATH | --version vX.Y.Z)      # upgrade: keeps the existing config
#   install.sh uninstall
#
# Read it before you run it. It touches only these paths, and uninstall removes
# all of them:
#
#   /usr/local/bin/telltale-agent                   the Agent
#   /etc/telltale/agent.toml                        its config (mode 0600)
#   /etc/telltale/                                  removed on uninstall if empty
#   /etc/systemd/system/telltale-agent.service      its systemd unit
#   the telltale-agent system user and group        no shell, no home
#
# It never touches the motd, shell profiles, authorized_keys, crontab, sysctl,
# the firewall, DNS or package sources (ADR 0001).
set -eu

REPO="Doublefire-Chen/Telltale"
USER_NAME="telltale-agent"
BIN="/usr/local/bin/telltale-agent"
CONFIG_DIR="/etc/telltale"
CONFIG="$CONFIG_DIR/agent.toml"
UNIT="/etc/systemd/system/telltale-agent.service"

say() { printf '%s\n' "$*"; }
die() { printf 'telltale install: %s\n' "$*" >&2; exit 1; }

usage() {
	sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
}

has_systemd() { command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; }

write_unit() {
	cat > "$UNIT" <<'UNIT'
[Unit]
Description=Telltale Agent (reports this Host's metrics to a Telltale Dashboard)
Documentation=https://github.com/Doublefire-Chen/Telltale
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=telltale-agent
Group=telltale-agent
ExecStart=/usr/local/bin/telltale-agent -config /etc/telltale/agent.toml
Restart=always
RestartSec=5

# The Agent only reads /proc, /sys and filesystem sizes and talks to the
# Dashboard over HTTPS. It needs no privileges and writes nothing.
NoNewPrivileges=yes
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
# read-only rather than yes: disk usage needs statfs on /home if it is a
# separate filesystem.
ProtectHome=read-only
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
SystemCallFilter=~@privileged
# Go raises its open-file limit at start-up (setrlimit, in @resources), so
# that group stays allowed; anything filtered fails with EPERM, not SIGSYS.
SystemCallErrorNumber=EPERM
UMask=0077

[Install]
WantedBy=multi-user.target
UNIT
	chmod 0644 "$UNIT"
}

arch() {
	case "$(uname -m)" in
		x86_64 | amd64) echo amd64 ;;
		aarch64 | arm64) echo arm64 ;;
		*) die "unsupported architecture $(uname -m); use --binary" ;;
	esac
}

# fetch VERSION DEST downloads a release binary and checks it against the
# release's SHA256SUMS.
fetch() {
	command -v curl >/dev/null 2>&1 || die "curl is needed to download a release; or use --binary"
	asset="telltale-agent-linux-$(arch)"
	base="https://github.com/$REPO/releases/download/$1"
	tmp=$(mktemp -d)
	curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "could not download $base/$asset"
	curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS" || die "could not download $base/SHA256SUMS"
	(cd "$tmp" && grep " $asset\$" SHA256SUMS | sha256sum -c - >/dev/null) || die "checksum mismatch for $asset"
	cp "$tmp/$asset" "$2"
	rm -rf "$tmp"
}

create_user() {
	if id "$USER_NAME" >/dev/null 2>&1; then
		return
	fi
	nologin=$(command -v nologin || echo /usr/sbin/nologin)
	if command -v useradd >/dev/null 2>&1; then
		useradd --system --user-group --no-create-home --home-dir /nonexistent --shell "$nologin" "$USER_NAME"
	elif command -v adduser >/dev/null 2>&1; then # BusyBox (Alpine)
		addgroup -S "$USER_NAME"
		adduser -S -D -H -h /nonexistent -s "$nologin" -G "$USER_NAME" "$USER_NAME"
	else
		die "cannot create the $USER_NAME user: neither useradd nor adduser found"
	fi
	say "created user $USER_NAME (no shell, no home)"
}

install_agent() {
	url="" token="" binary="" version="" interval=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--dashboard-url) url="${2:?}"; shift 2 ;;
			--token) token="${2:?}"; shift 2 ;;
			--binary) binary="${2:?}"; shift 2 ;;
			--version) version="${2:?}"; shift 2 ;;
			--interval) interval="${2:?}"; shift 2 ;;
			*) usage ;;
		esac
	done
	[ -n "$binary" ] || [ -n "$version" ] || die "give --binary PATH or --version vX.Y.Z"
	[ -f "$CONFIG" ] || { [ -n "$url" ] && [ -n "$token" ]; } || die "first install needs --dashboard-url and --token"

	create_user

	tmp_bin=$(mktemp)
	if [ -n "$binary" ]; then
		cp "$binary" "$tmp_bin"
	else
		fetch "$version" "$tmp_bin"
	fi
	install -m 0755 "$tmp_bin" "$BIN"
	rm -f "$tmp_bin"
	say "installed $BIN ($("$BIN" -version))"

	if [ -n "$url" ] && [ -n "$token" ]; then
		mkdir -p "$CONFIG_DIR"
		chmod 0755 "$CONFIG_DIR"
		old_umask=$(umask)
		umask 077
		{
			printf 'dashboard_url = "%s"\n' "$url"
			printf 'token = "%s"\n' "$token"
			[ -z "$interval" ] || printf 'interval = "%s"\n' "$interval"
		} > "$CONFIG"
		umask "$old_umask"
		chown "$USER_NAME:$USER_NAME" "$CONFIG"
		chmod 0600 "$CONFIG"
		say "wrote $CONFIG (readable only by $USER_NAME)"
	else
		say "kept existing $CONFIG"
	fi

	write_unit
	say "wrote $UNIT"
	if has_systemd; then
		systemctl daemon-reload
		systemctl enable telltale-agent.service >/dev/null 2>&1
		systemctl restart telltale-agent.service
		say "started telltale-agent.service; check it with: systemctl status telltale-agent"
	else
		say "systemd is not running here; start the Agent yourself as $USER_NAME:"
		say "  $BIN -config $CONFIG"
	fi
}

uninstall_agent() {
	if has_systemd && [ -f "$UNIT" ]; then
		systemctl disable --now telltale-agent.service >/dev/null 2>&1 || true
	fi
	for path in "$UNIT" "$BIN" "$CONFIG"; do
		if [ -e "$path" ]; then
			rm -f "$path"
			say "removed $path"
		fi
	done
	if has_systemd; then
		systemctl daemon-reload
	fi
	if [ -d "$CONFIG_DIR" ] && [ -z "$(ls -A "$CONFIG_DIR")" ]; then
		rmdir "$CONFIG_DIR"
		say "removed $CONFIG_DIR"
	fi
	if id "$USER_NAME" >/dev/null 2>&1; then
		if command -v userdel >/dev/null 2>&1; then
			userdel "$USER_NAME"
			getent group "$USER_NAME" >/dev/null 2>&1 && groupdel "$USER_NAME"
		else
			deluser "$USER_NAME" 2>/dev/null || true
			delgroup "$USER_NAME" 2>/dev/null || true
		fi
		say "removed user $USER_NAME"
	fi
}

[ "$(id -u)" -eq 0 ] || die "run as root (e.g. sudo sh install.sh ...)"
case "${1:-}" in
	install) shift; install_agent "$@" ;;
	uninstall) uninstall_agent ;;
	*) usage ;;
esac
