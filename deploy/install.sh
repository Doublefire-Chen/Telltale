#!/bin/sh
# Telltale Agent installer for Linux and macOS (Windows: install.ps1).
#
#   install.sh install   --dashboard-url URL (--binary PATH | --version vX.Y.Z [--sha256 HASH]...) [--interval 2s] [--token-file FILE]
#   install.sh install   (--binary PATH | --version vX.Y.Z [--sha256 HASH]...)   # upgrade: keeps the existing config
#   install.sh uninstall
#
# On a first install it asks for the Host token (or reads it from --token-file,
# "-" for stdin), so the token never appears in the process list or your shell
# history. --version checks the download's cosign signature, which needs
# cosign; without cosign, pass --sha256 with the binary's checksum, as HASH or
# as os-arch=HASH (once per platform; the Dashboard's command does this).
#
# Read it before you run it. It touches only these paths, and uninstall removes
# all of them:
#
#   /usr/local/bin/telltale-agent                   the Agent
#   /etc/telltale/agent.toml                        its config (root-owned, readable by the Agent's group)
#   /etc/telltale/                                  removed on uninstall if empty
#   Linux: /etc/systemd/system/telltale-agent.service (on OpenWrt the procd
#   service /etc/init.d/telltale-agent instead), and the telltale-agent system
#   user and group (no shell, no home)
#   macOS: what `telltale-agent service install` sets up: the launchd daemon
#   in /Library/LaunchDaemons, /var/log/telltale-agent.log and the hidden
#   _telltale-agent user
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
INIT_SCRIPT="/etc/init.d/telltale-agent"

say() { printf '%s\n' "$*"; }
die() { printf 'telltale install: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
	Linux) OS=linux ROOT_GROUP=root ;;
	Darwin) OS=darwin ROOT_GROUP=wheel USER_NAME="_telltale-agent" ;;
	*) die "this installer is for Linux and macOS; on Windows use install.ps1" ;;
esac

usage() {
	sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
}

has_systemd() { command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; }

# OpenWrt runs services under procd, and has no useradd or adduser.
is_openwrt() { [ -f /etc/openwrt_release ] && [ -x /sbin/procd ]; }

# write_init_script writes the procd service OpenWrt runs the Agent with: as
# its own user, restarted however often it exits.
write_init_script() {
	cat > "$INIT_SCRIPT" <<INIT
#!/bin/sh /etc/rc.common
# Telltale Agent, installed by install.sh.
START=99
USE_PROCD=1

start_service() {
	procd_open_instance
	procd_set_param command $BIN -config $CONFIG
	procd_set_param user $USER_NAME
	procd_set_param respawn 3600 5 0
	procd_set_param stdout 1
	procd_set_param stderr 1
	procd_close_instance
}
INIT
	chmod 0755 "$INIT_SCRIPT"
}

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
	# A shell under Rosetta reports x86_64 on Apple silicon.
	if [ "$OS" = darwin ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = 1 ]; then
		echo arm64
		return
	fi
	case "$(uname -m)" in
		x86_64 | amd64) echo amd64 ;;
		aarch64 | arm64) echo arm64 ;;
		*) die "unsupported architecture $(uname -m); use --binary" ;;
	esac
}

# matches VALUE REGEX succeeds if all of VALUE matches the extended REGEX.
matches() { printf '%s' "$1" | grep -Eqx "$2" && [ "$(printf '%s' "$1" | wc -l)" -eq 0 ]; }

# asset prints the name of this machine's Agent binary in a release.
asset() { printf 'telltale-agent-%s-%s' "$OS" "$(arch)"; }

# sha256_of FILE prints FILE's SHA-256 (older Macs have only shasum).
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d ' ' -f 1
}

# pick_sha256 ASSET HASHES prints the hash for ASSET among the --sha256
# values: the one given as os-arch=HASH for its platform, or a bare HASH.
pick_sha256() {
	for h in $2; do
		case "$h" in
			*=*) [ "telltale-agent-${h%%=*}" != "$1" ] || { printf '%s' "${h#*=}"; return; } ;;
			*) printf '%s' "$h"; return ;;
		esac
	done
}

# os_name OS prints OS as the Dashboard's install dialog names it.
os_name() {
	case "$1" in
		linux) echo Linux ;;
		darwin) echo macOS ;;
		windows) echo Windows ;;
		*) echo "$1" ;;
	esac
}

# wrong_os HASHES fails with a hint if the --sha256 values pin another OS's
# Agents, as when the Dashboard's command for that OS was copied by mistake.
wrong_os() {
	for h in $1; do
		case "$h" in
			"$OS"-*=*) ;;
			*-*=*) die "this command is for $(os_name "${h%%-*}"), but this machine runs $(os_name "$OS"): copy the $(os_name "$OS") install command from the Dashboard" ;;
		esac
	done
	return 1
}

# can_fetch VERSION SHA256S fails early if fetch could not check the download.
can_fetch() {
	command -v curl >/dev/null 2>&1 || die "curl is needed to download a release; or use --binary"
	if [ -n "$2" ]; then
		[ -n "$(pick_sha256 "$(asset)" "$2")" ] || wrong_os "$2" || die "no --sha256 given for $(asset)"
	elif ! command -v cosign >/dev/null 2>&1; then
		die "cosign is needed to check who published $1 (https://docs.sigstore.dev/cosign/system_config/installation/);
or pass --sha256 with the checksum of $(asset) from a SHA256SUMS you verified with cosign elsewhere"
	fi
}

# fetch VERSION SHA256S DEST downloads a release binary and checks it: against
# its hash among SHA256S if given, otherwise against the release's SHA256SUMS after checking
# that file's cosign signature, which proves this repository's release
# workflow made it at that tag. A SHA256SUMS that is only checksummed would
# prove nothing, since whoever could replace the binary could replace it too.
fetch() {
	asset=$(asset)
	base="https://github.com/$REPO/releases/download/$1"
	tmp=$(mktemp -d)
	curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "could not download $base/$asset"
	if [ -n "$2" ]; then
		printf '%s  %s\n' "$(pick_sha256 "$asset" "$2")" "$asset" > "$tmp/SHA256SUMS"
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
	got=$(sha256_of "$tmp/$asset")
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
	elif is_openwrt; then # with the helpers its own packages use
		(
			set +u
			. /lib/functions.sh
			gid=$(group_add_next "$USER_NAME")
			user_add "$USER_NAME" "" "$gid" "$USER_NAME" /nonexistent /bin/false
		)
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
			--sha256) sha256="$sha256 ${2:?}"; shift 2 ;;
			--interval) interval="${2:?}"; shift 2 ;;
			*) usage ;;
		esac
	done
	[ -n "$binary" ] || [ -n "$version" ] || die "give --binary PATH or --version vX.Y.Z"
	[ -z "$version" ] || matches "$version" 'v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?' || die "--version must look like v1.2.3"
	for h in $sha256; do
		matches "$h" '([a-z]+-[a-z0-9]+=)?[0-9a-f]{64}' || die "--sha256 must be 64 lowercase hex digits, or os-arch= and those"
	done
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

	[ "$OS" = darwin ] || create_user

	tmp_bin=$(mktemp)
	if [ -n "$binary" ]; then
		cp "$binary" "$tmp_bin"
	else
		fetch "$version" "$sha256" "$tmp_bin"
	fi
	mkdir -p "$(dirname "$BIN")"
	# Not install(1), which OpenWrt lacks; the rename also replaces a running
	# binary in one step.
	cp "$tmp_bin" "$BIN.new"
	rm -f "$tmp_bin"
	chown "root:$ROOT_GROUP" "$BIN.new"
	chmod 0755 "$BIN.new"
	mv -f "$BIN.new" "$BIN"
	say "installed $BIN (${version:-from $binary}, sha256 $(sha256_of "$BIN"))"

	mkdir -p "$CONFIG_DIR"
	chown "root:$ROOT_GROUP" "$CONFIG_DIR"
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
	if [ "$OS" = darwin ]; then
		# The Agent sets up its own launchd daemon and user, and gives the
		# config to that user's group. That needs root, so unlike on Linux the
		# new binary runs as root here, once; it was checked above.
		chown root:wheel "$CONFIG"
		chmod 0600 "$CONFIG"
		"$BIN" -config "$CONFIG" service install
		return
	fi
	# Not running the new binary here: it would run as root, unconfined.
	# Root owns the config so the Agent can read its token but not change
	# where it reports to.
	chown "root:$USER_NAME" "$CONFIG"
	chmod 0640 "$CONFIG"
	say "made $CONFIG readable only by root and the $USER_NAME group"

	if is_openwrt; then
		write_init_script
		say "wrote $INIT_SCRIPT"
		"$INIT_SCRIPT" enable
		"$INIT_SCRIPT" restart
		say "started telltale-agent; check it with: logread -e telltale-agent"
		return
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
	if [ "$OS" = darwin ]; then
		# The Agent removes its daemon, log, config and user.
		[ ! -x "$BIN" ] || "$BIN" -config "$CONFIG" service uninstall --purge
		if [ -e "$BIN" ]; then
			rm -f "$BIN"
			say "removed $BIN"
		fi
		if [ -d "$CONFIG_DIR" ] && [ -z "$(ls -A "$CONFIG_DIR")" ]; then
			rmdir "$CONFIG_DIR"
			say "removed $CONFIG_DIR"
		fi
		return
	fi
	if has_systemd && [ -f "$UNIT" ]; then
		systemctl disable --now telltale-agent.service >/dev/null 2>&1 || true
	fi
	if [ -x "$INIT_SCRIPT" ] && grep -q '^# Telltale Agent' "$INIT_SCRIPT"; then
		"$INIT_SCRIPT" stop >/dev/null 2>&1 || true
		"$INIT_SCRIPT" disable >/dev/null 2>&1 || true
		rm -f "$INIT_SCRIPT"
		say "removed $INIT_SCRIPT"
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
		elif is_openwrt; then # added by create_user with OpenWrt's helpers
			sed -i "/^$USER_NAME:/d" /etc/passwd /etc/shadow /etc/group
		else
			deluser "$USER_NAME" 2>/dev/null || true
			delgroup "$USER_NAME" 2>/dev/null || true
		fi
		say "removed user $USER_NAME"
	fi
}

# Root is needed; a user who can sudo gets asked for their password.
if [ "$(id -u)" -ne 0 ]; then
	command -v sudo >/dev/null 2>&1 || die "run as root"
	exec sudo sh "$0" "$@"
fi
case "${1:-}" in
	install) shift; install_agent "$@" ;;
	uninstall) uninstall_agent ;;
	*) usage ;;
esac
