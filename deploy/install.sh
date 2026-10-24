#!/bin/sh
# Telltale Agent installer for Linux.
#
#   install.sh install   --dashboard-url URL (--binary PATH | --version vX.Y.Z [--sha256 HASH]) [--interval 2s] [--token-file FILE]
#   install.sh install   (--binary PATH | --version vX.Y.Z [--sha256 HASH])   # upgrade: keeps the existing config
#   install.sh uninstall
#
# On a first install it asks for the Host token (or reads it from --token-file,
# "-" for stdin), so the token never appears in the process list or your shell
# history. --version checks the download's cosign signature, which needs
# cosign; without cosign, pass --sha256 with the binary's checksum from a
# SHA256SUMS you verified elsewhere.
#
# Read it before you run it. It touches only these paths, and uninstall removes
# all of them:
#
#   /usr/local/bin/telltale-agent                   the Agent
#   /etc/telltale/agent.toml                        its config (root-owned, readable by the Agent's group)
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
	sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
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
RemoveIPC=yes
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

# matches VALUE REGEX succeeds if all of VALUE matches the extended REGEX.
matches() { printf '%s' "$1" | grep -Eqx "$2" && [ "$(printf '%s' "$1" | wc -l)" -eq 0 ]; }

# can_fetch VERSION SHA256 fails early if fetch could not check the download.
can_fetch() {
	command -v curl >/dev/null 2>&1 || die "curl is needed to download a release; or use --binary"
	if [ -z "$2" ] && ! command -v cosign >/dev/null 2>&1; then
		die "cosign is needed to check who published $1 (https://docs.sigstore.dev/cosign/system_config/installation/);
or pass --sha256 with the checksum of telltale-agent-linux-$(arch) from a SHA256SUMS you verified with cosign elsewhere"
	fi
}

# fetch VERSION SHA256 DEST downloads a release binary and checks it: against
# SHA256 if given, otherwise against the release's SHA256SUMS after checking
# that file's cosign signature, which proves this repository's release
# workflow made it at that tag. A SHA256SUMS that is only checksummed would
# prove nothing, since whoever could replace the binary could replace it too.
fetch() {
	asset="telltale-agent-linux-$(arch)"
	base="https://github.com/$REPO/releases/download/$1"
	tmp=$(mktemp -d)
	curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "could not download $base/$asset"
	if [ -n "$2" ]; then
		printf '%s  %s\n' "$2" "$asset" > "$tmp/SHA256SUMS"
	else
		curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS" || die "could not download $base/SHA256SUMS"
		curl -fsSL "$base/SHA256SUMS.sigstore.json" -o "$tmp/SHA256SUMS.sigstore.json" ||
			die "could not download $base/SHA256SUMS.sigstore.json"
		cosign verify-blob "$tmp/SHA256SUMS" --bundle "$tmp/SHA256SUMS.sigstore.json" \
			--certificate-oidc-issuer https://token.actions.githubusercontent.com \
			--certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$1" >/dev/null 2>&1 ||
			die "SHA256SUMS of $1 does not carry a valid signature from the $REPO release workflow"
		say "checked the signature of $1's SHA256SUMS"
	fi
	# Compare the hash ourselves: given no line at all, some sha256sum -c
	# implementations check nothing and still succeed.
	want=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/SHA256SUMS")
	matches "$want" '[0-9a-f]{64}' || die "SHA256SUMS has no single checksum for $asset"
	got=$(sha256sum "$tmp/$asset" | cut -d ' ' -f 1)
	[ "$got" = "$want" ] || die "checksum mismatch for $asset"
	cp "$tmp/$asset" "$3"
	rm -rf "$tmp"
}

# read_token FILE prints the Host token from FILE ("-" for stdin), or asks for
# it on the terminal without echoing it.
read_token() {
	if [ -n "$1" ]; then
		if [ "$1" = - ]; then head -n 1; else head -n 1 "$1"; fi | tr -d '\r'
		return
	fi
	[ -r /dev/tty ] || die "no terminal to ask for the Host token on; use --token-file FILE (or - for stdin)"
	printf 'Host token (input hidden): ' > /dev/tty
	stty -echo < /dev/tty
	IFS= read -r t < /dev/tty || t=""
	stty echo < /dev/tty
	printf '\n' > /dev/tty
	printf '%s' "$t"
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
	url="" token="" token_file="" binary="" version="" sha256="" interval=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--dashboard-url) url="${2:?}"; shift 2 ;;
			--token-file) token_file="${2:?}"; shift 2 ;;
			--binary) binary="${2:?}"; shift 2 ;;
			--version) version="${2:?}"; shift 2 ;;
			--sha256) sha256="${2:?}"; shift 2 ;;
			--interval) interval="${2:?}"; shift 2 ;;
			*) usage ;;
		esac
	done
	[ -n "$binary" ] || [ -n "$version" ] || die "give --binary PATH or --version vX.Y.Z"
	[ -z "$version" ] || matches "$version" 'v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?' || die "--version must look like v1.2.3"
	[ -z "$sha256" ] || matches "$sha256" '[0-9a-f]{64}' || die "--sha256 must be 64 lowercase hex digits"
	if [ -n "$interval" ]; then
		matches "$interval" '[0-9]+(\.[0-9]+)?(ms|s|m|h)' || die "--interval must be a duration such as 2s"
		# The Agent refuses to start below one second.
		echo "$interval" | awk '{ n = $0 + 0; u = $0; sub(/^[0-9.]+/, "", u)
			ms = n * (u == "ms" ? 1 : u == "s" ? 1000 : u == "m" ? 60000 : 3600000)
			exit !(ms >= 1000) }' || die "--interval must be at least 1s"
	fi
	if [ -n "$url" ]; then
		# Quotes, backslashes and whitespace would break out of the TOML
		# string; the Agent itself checks the rest of the URL when it starts.
		matches "$url" 'https?://[^[:space:]"\\@/?#]+(/[^[:space:]"\\?#]*)?' ||
			die "--dashboard-url must be a plain http(s) URL such as https://status.example.com"
		# As the Agent does: plain http only to this machine, since the
		# token would otherwise cross the network in the clear.
		matches "$url" 'https://.*|http://(localhost|127\.[0-9]+\.[0-9]+\.[0-9]+|\[::1\])(:[0-9]+)?(/.*)?' ||
			die "--dashboard-url must use https (plain http is allowed only for a loopback address)"
		token=$(read_token "$token_file")
		matches "$token" 'tt_[A-Za-z0-9_-]+' || die 'the Host token must be the one the Dashboard showed (it starts with "tt_")'
	elif [ -n "$token_file" ] || [ -n "$interval" ]; then
		die "a token or interval needs --dashboard-url too"
	fi
	[ -f "$CONFIG" ] || [ -n "$url" ] || die "first install needs --dashboard-url"
	[ -n "$binary" ] || can_fetch "$version" "$sha256"

	create_user

	tmp_bin=$(mktemp)
	if [ -n "$binary" ]; then
		cp "$binary" "$tmp_bin"
	else
		fetch "$version" "$sha256" "$tmp_bin"
	fi
	install -m 0755 "$tmp_bin" "$BIN"
	rm -f "$tmp_bin"
	# Not running the new binary here: it would run as root, unconfined.
	say "installed $BIN (${version:-from $binary}, sha256 $(sha256sum "$BIN" | cut -d ' ' -f 1))"

	mkdir -p "$CONFIG_DIR"
	chown root:root "$CONFIG_DIR"
	chmod 0755 "$CONFIG_DIR"
	if [ -n "$url" ]; then
		old_umask=$(umask)
		umask 077
		{
			printf 'dashboard_url = "%s"\n' "$url"
			printf 'token = "%s"\n' "$token"
			[ -z "$interval" ] || printf 'interval = "%s"\n' "$interval"
		} > "$CONFIG"
		umask "$old_umask"
		say "wrote $CONFIG"
	else
		say "kept existing $CONFIG"
	fi
	# Root owns the config so the Agent can read its token but not change
	# where it reports to.
	chown "root:$USER_NAME" "$CONFIG"
	chmod 0640 "$CONFIG"
	say "made $CONFIG readable only by root and the $USER_NAME group"

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
