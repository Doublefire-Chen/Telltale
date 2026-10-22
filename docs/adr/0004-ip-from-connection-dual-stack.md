# Host IPs come from the Agent's own connections, never third-party lookups

Nezha and Komari agents query several third-party "what is my IP" services (Cloudflare trace endpoints, ip.sb, ipify, …), and Komari's dashboard can send Host IPs to remote GeoIP APIs. Telltale's Agent contacts nothing but the Dashboard: it sends Host Info once over a forced-IPv4 connection and once over a forced-IPv6 connection, and the Dashboard records each connection's source IP as that Host's IPv4/IPv6 address. Country is resolved locally from a DB-IP Lite country database embedded at build time (no account or token needed, unlike MaxMind or ipinfo). Forwarded-for headers are trusted only from configured trusted proxies.

## Consequences

- The Dashboard must be reachable over both IPv4 and IPv6 (A and AAAA records) to learn both addresses; an IPv4-only Dashboard will only ever show IPv4.
- Country may be wrong for anycast or recently reassigned ranges, so each Host has a manual country override in the Dashboard.
