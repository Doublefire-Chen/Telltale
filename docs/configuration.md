# Configuration reference

Both programs read a TOML file. Unknown settings are rejected, so a typo is an
error rather than silently ignored. Neither program can be configured
remotely: the Agent's settings live only in its file on the Host
([ADR 0001](adr/0001-agent-is-one-way.md)).

## Dashboard

`/etc/telltale/dashboard.toml` by default; pass `-config PATH` to use another
file. Every setting is optional. A commented example is in
[`deploy/dashboard.toml`](../deploy/dashboard.toml).

| Setting | Default | Meaning |
|---|---|---|
| `listen` | `["127.0.0.1:8080", "[::1]:8080"]` | Addresses to serve on. Keep them on loopback and put a reverse proxy in front for HTTPS. Addresses that cannot be bound (e.g. no IPv6) are skipped with a warning. |
| `data_dir` | `"/var/lib/telltale"` | Where all state lives: one SQLite database. Back up this directory to back up the Dashboard. |
| `trusted_proxies` | `["127.0.0.0/8", "::1"]` | Addresses or CIDR ranges of reverse proxies whose `X-Forwarded-For` / `X-Real-IP` headers are believed. Only list proxies you run. |
| `history_raw` | `"1h"` | How long every Report is kept. |
| `history_minute` | `"7d"` | How long 1-minute averages are kept. |
| `history_quarter` | `"90d"` | How long 15-minute averages are kept. |
| `geoip_db` | (built in) | A MaxMind-format country database to use instead of the built-in DB-IP Lite. |

Durations take Go's units (`90m`, `36h`) or whole days (`7d`).

Charts use the tier that covers their range: 1h uses every Report, 24h and 7d
use 1-minute averages, 30d uses 15-minute averages. Shortening a retention
period empties the matching chart ranges sooner.

### Behind Cloudflare or another CDN

The Dashboard sees the CDN's address unless you trust it. Either add the
CDN's published ranges to `trusted_proxies`, or have nginx restore the real
address with its `real_ip` module (then nginx's own `$remote_addr` is the
client and the defaults work).

### Settings in the Admin area

These are stored in the database, not the file:

- **Make the Status page public** (off by default). Anonymous visitors then
  see Hosts, flags and metrics, but never IP addresses or Hidden Hosts.
- Per Host: display name, group, Hidden, country override, order, token.

### Commands

```
telltale-dashboard [-config PATH]                       # serve (the default)
telltale-dashboard [-config PATH] admin set-password    # set or reset the admin password
telltale-dashboard -version
```

`admin set-password` is the only way to create the admin account (username
`admin`); there is no web setup wizard. Resetting the password logs out every
session.

## Agent

`/etc/telltale/agent.toml` on Linux and macOS, `C:\ProgramData\Telltale\agent.toml`
on Windows; pass `-config PATH` to use another file.

| Setting | Default | Meaning |
|---|---|---|
| `dashboard_url` | (required) | The Dashboard's base URL. Must be `https://`, except a loopback address. |
| `token` | (required) | The Host token the Dashboard showed when you added the Host (starts with `tt_`). |
| `interval` | `"2s"` | Time between Reports; at least `1s`. A Host is shown Offline after three missed intervals. |
| `exclude_interfaces` | see below | Glob patterns of network interfaces left out of Traffic and Bandwidth. Setting it replaces the default list; `[]` counts every interface. |

The default `exclude_interfaces` leaves out loopback and virtual interfaces,
whose traffic also crosses a physical one and would otherwise be counted
twice:

```
lo  lo0  Loopback*  docker*  veth*  br-*  virbr*  cni*  flannel*  cali*  vxlan*
kube-*  tun*  tap*  utun*  wg*  awdl*  llw*  gif*  stf*  bridge*  anpi*
```

Example:

```toml
dashboard_url = "https://status.example.com"
token = "tt_…"
interval = "2s"
# Count a WireGuard tunnel too:
# exclude_interfaces = ["lo", "docker*", "veth*"]
```

The config file holds the Host token: keep it readable only by the Agent. The
Linux installer and `service install` on Windows and macOS do this for you.

### Commands

```
telltale-agent [-config PATH]                                # run in the foreground
telltale-agent [-config PATH] service install                # Windows, macOS: install and start as a service
telltale-agent [-config PATH] service uninstall [--purge]    # remove it (--purge: the config too)
telltale-agent -version
```

On Linux use [`deploy/install.sh`](../deploy/install.sh) instead.
