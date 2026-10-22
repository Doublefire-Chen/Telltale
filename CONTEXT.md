# Telltale

A lightweight, self-hosted VPS monitor. Agents on each machine passively report system state to a Dashboard that stores and displays it; nothing ever flows back to act on the machine.

## Components

**Agent**:
The program running on a monitored machine that collects metrics and sends them to the Dashboard. It only reports; it never receives instructions.
_Avoid_: client, probe, node, daemon

**Dashboard**:
The program that receives data from Agents, stores it, and serves the web UI and API.
_Avoid_: server, controller, control panel, panel, master

**Status page**:
The read-only view of the Dashboard showing Hosts and their metrics; may be public or private.
_Avoid_: frontend, homepage

**Admin area**:
The part of the Dashboard where the single admin manages Hosts and settings.
_Avoid_: backend, console

## Monitored machines

**Host**:
A machine registered in the Dashboard, monitored by exactly one Agent.
_Avoid_: server, node, client, VPS (in code and UI)

**Host token**:
The secret issued by the Dashboard for one Host, used by its Agent to prove which Host it reports for.
_Avoid_: key, password, global secret

**Hidden Host**:
A Host left out of the public Status page but still visible in the Admin area.

## Data

**Host Info**:
The static description of a Host (OS, kernel, architecture, CPU model, totals, virtualization, boot time, Agent version), sent at Agent start, on change, and periodically — once over each IP family.
_Avoid_: basic info, system info

**Report**:
One periodic snapshot of a Host's changing metrics (CPU, load, memory, swap, disk used, network byte counters).
_Avoid_: state, heartbeat, sample, status

**Traffic**:
Inbound and outbound bytes a Host has transferred since its last boot, as counted by the OS.
_Avoid_: usage, quota, monthly traffic

**Bandwidth**:
The current transfer rate of a Host, derived from the difference between consecutive Reports.
_Avoid_: speed, throughput

**Uptime**:
Time since a Host's boot, derived by the Dashboard from the boot time in Host Info.

**Offline**:
The condition of a Host from which no Report has arrived within three report intervals.
_Avoid_: down, disconnected, dead

**Protocol version**:
The version of the Agent-to-Dashboard wire format, independent of Telltale's release version.
_Avoid_: API version, agent version
