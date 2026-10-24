# Telltale

**English** · [简体中文](README.zh-CN.md)

A lightweight, self-hosted monitor for your VPSes. A small **Agent** on each
machine reports CPU, memory, disk, swap, load, network Traffic and Bandwidth,
uptime and system details to a **Dashboard** that shows them live and keeps
history. A telltale shows a state without changing anything, and that is the
whole idea:

> **Telltale observes and never acts.**

The Agent sends data out and acts on nothing that comes back. There is no
WebSSH, no remote command execution, no file manager, no remote
configuration and no self-update — not as options, not ever. Nothing on the
Dashboard side can run code on, reconfigure or upgrade your machines, and an
upstream release can never break your fleet on its own. See
[why](docs/adr/0001-agent-is-one-way.md).

## Features

- **Live Status page**: a card per Host with its flag and OS logo, usage bars
  for CPU, memory, swap and disk, Bandwidth, Traffic, load, uptime, and process
  and connection counts, updating as Reports arrive (every 2 seconds by
  default); Offline within three missed intervals.
- **History**: charts of CPU, memory, swap, disk, bandwidth and load for 1
  hour, 24 hours, 7 days and 30 days, kept in SQLite as raw Reports, 1-minute
  and 15-minute averages. Only the admin sees them, unless a Host's history
  is made public.
- **Both IP addresses** of each Host and their countries, learned from the
  Agent's own connections to the Dashboard — no third-party lookups.
- **Private by default**, or a public Status page that never shows IP
  addresses or Hidden Hosts, and shows history only for the Hosts you choose.
- **An admin panel** after Nezha V0's: Hosts in a table with batch actions,
  one-time tokens with a ready-made install command, and settings.
- **Upgrades never break a fleet**: old Agents keep working with new
  Dashboards, and the other way round ([protocol](docs/protocol.md)).
- **Light**: the Agent is a single static binary needing no root; it uses
  around 10–15 MB of memory and a fraction of a percent of one core.
- **English and Simplified Chinese** UI.
- Agent for **Linux, Windows and macOS** (amd64 and arm64); Dashboard for Linux.

## Install

Binaries are attached to each [release](https://github.com/Doublefire-Chen/Telltale/releases):
`telltale-agent-<os>-<arch>` (Linux, Windows, macOS; amd64 and arm64) and
`telltale-dashboard-linux-<arch>`, all built from one tag under one version.
To build from source instead you need Go and Node.js:

```sh
make geoip   # download the DB-IP Lite country database to embed (optional)
make build   # bin/telltale-dashboard and bin/telltale-agent
```

### Verifying a download

`SHA256SUMS` lists every binary's checksum, and `SHA256SUMS.sigstore.json`
is a [cosign](https://docs.sigstore.dev/cosign/system_config/installation/)
signature proving the file was made by this repository's release workflow at
that tag. There is no signing key to steal: the signature is tied to the
workflow's GitHub identity. With `VERSION` set to the release, for example
`VERSION=v0.1.0`:

```sh
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity "https://github.com/Doublefire-Chen/Telltale/.github/workflows/release.yml@refs/tags/$VERSION"
sha256sum --ignore-missing -c SHA256SUMS   # macOS: shasum -a 256 --ignore-missing -c SHA256SUMS
```

`install.sh --version` does both checks itself when `cosign` is installed.
Without it, the installer refuses to download unless you pass `--sha256` with
the binary's checksum from a `SHA256SUMS` you verified as above on another
machine.

### 1. The Dashboard (Linux)

```sh
sudo install -m 0755 telltale-dashboard /usr/local/bin/
sudo useradd --system --user-group --no-create-home --shell /usr/sbin/nologin telltale
sudo install -D -m 0644 deploy/dashboard.toml /etc/telltale/dashboard.toml
sudo install -m 0644 deploy/systemd/telltale-dashboard.service /etc/systemd/system/
sudo systemctl enable --now telltale-dashboard

# Create the admin account (username "admin"). There is no web setup wizard.
sudo -u telltale telltale-dashboard admin set-password
```

The Dashboard listens on `127.0.0.1:8080` and `[::1]:8080`. Put a reverse
proxy in front for HTTPS: [`deploy/nginx/telltale.conf`](deploy/nginx/telltale.conf)
is a ready-made nginx config (HTTP/2, IPv4 and IPv6, WebSocket).

**Give the Dashboard's name both an A and an AAAA record.** Each Agent
reports once over IPv4 and once over IPv6, which is how the Dashboard learns
both of a Host's addresses. An IPv4-only Dashboard only ever shows IPv4.

### 2. Add a Host

Log in, open the **Admin panel** and click **Add Host** (leave the name blank
to use the machine's hostname). It shows the Host token with a ready-made
Linux install command and the Agent config, **only once**; the Dashboard keeps
just its hash. You can regenerate the token later, which immediately revokes
the old one.

### 3. The Agent

**Linux** — read [`deploy/install.sh`](deploy/install.sh) first; it is short
and says exactly which files it touches.

```sh
sudo sh install.sh install --dashboard-url https://status.example.com --version vX.Y.Z
# it asks for the Host token; or pass --token-file FILE (- for stdin)
# without cosign: add --sha256 HASH; with a binary you already have: --binary ./telltale-agent
```

It creates an unprivileged `telltale-agent` user, writes a root-owned config
only that user's group can read, and starts a hardened systemd unit. It never
runs the new binary itself. To upgrade, run it again with just `--version` or
`--binary`. To remove everything: `sudo sh install.sh uninstall`.

**Windows** (as Administrator) — put `telltale-agent.exe` in
`C:\Program Files\Telltale\`, save the config as
`C:\ProgramData\Telltale\agent.toml`, then:

```powershell
& "C:\Program Files\Telltale\telltale-agent.exe" service install
```

It runs as LocalService; its logs are in the Event Viewer. It refuses to
install an exe outside Program Files, where other users might replace it.

**macOS** — put `telltale-agent` in `/usr/local/bin/`, save the config as
`/etc/telltale/agent.toml`, then:

```sh
sudo telltale-agent service install
```

It runs as a hidden `_telltale-agent` user and logs to
`/var/log/telltale-agent.log`. Uninstall with `service uninstall`
(add `--purge` to delete the config too).

The installers never touch your motd, shell profiles, `authorized_keys`,
crontab, sysctl, firewall, DNS or package sources.

### Upgrading

Upgrade the Dashboard and the Agents independently, in any order. An Agent
older than the Dashboard gets an "Agent outdated" badge and keeps working.
Agents never update themselves.

## Documentation

- [Configuration reference](docs/configuration.md)
- [The Agent–Dashboard protocol and its compatibility rules](docs/protocol.md)
- [Glossary](CONTEXT.md): the project's terms — Agent, Dashboard, Host,
  Report, Host Info, Traffic, Bandwidth, Offline…
- [Architecture decisions](docs/adr/), including what Telltale will never do

## Contributing

Read [`CONTEXT.md`](CONTEXT.md) and [`docs/adr/`](docs/adr/) first. In
particular, features that let the Dashboard act on a Host — remote shells or
commands, file access, remote configuration, self-update — are out of scope
permanently, not just for now.

```sh
make test   # Go and web tests
./scripts/test-install-in-container.sh   # installer and systemd units (Docker)
./scripts/test-nginx-in-container.sh     # the sample nginx config (Docker)
go test -tags budget ./internal/e2e      # the Agent's CPU, memory and size budget (Linux, ~1 min)
```

To release, push a tag such as `v0.1.0`: the release workflow tests, builds,
signs and publishes everything.

## License

GPL-3.0. IP geolocation by [DB-IP](https://db-ip.com) (CC BY 4.0). Flags from
[flag-icons](https://github.com/lipis/flag-icons) (MIT), OS logos from
[font-logos](https://github.com/Lukas-W/font-logos) (public domain), icons from
[Bootstrap Icons](https://icons.getbootstrap.com) (MIT). The Status page's look
follows the AngelKanade theme for Nezha Monitoring by
[夜灭](https://yujie.pro).
