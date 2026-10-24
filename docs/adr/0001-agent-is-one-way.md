# The Agent is strictly one-way and never updates itself

Telltale exists because Nezha and Komari let the controller act on monitored machines (WebSSH, remote commands, agent auto-update, editing the SSH motd), which caused security incidents and fleet-wide breakage. Telltale's Agent only sends data: it has no code path that executes commands, writes files, or changes its own config based on anything the Dashboard sends, and reads nothing from responses beyond the status code. (To read some metrics, its metrics library runs a few fixed, read-only system tools with fixed arguments, such as `netstat` and `sw_vers` on macOS and `lsb_release` on Linux; nothing about them comes from the Dashboard.) Configuration is a local file on the Host, the Agent never downloads or replaces its own binary (not even opt-in), and neither the Agent nor its installer touches any file other than its own (no motd, shell profiles, crontab, authorized_keys, etc.).

## Consequences

- Changing settings across many Hosts means editing each Host's config file (or using the user's own tooling such as Ansible); there is no remote config push.
- Features that require the Dashboard to act on a Host — WebSSH, remote exec, file manager, self-update — are out of scope permanently, not just for v1.
