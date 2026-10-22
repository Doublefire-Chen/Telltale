// Package v1 defines protocol version 1: the wire format Agents use to send
// Host Info and Reports to the Dashboard.
//
// Within v1 the format only grows (ADR 0002): fields may be added but never
// renamed, removed or redefined. Optional fields are pointers so that a field
// an Agent did not send decodes as nil ("unknown"), not as zero.
package v1

import "time"

// Endpoint paths on the Dashboard. Agents authenticate with
// "Authorization: Bearer <Host token>" and read only the response status code.
const (
	ReportPath   = "/api/v1/report"
	HostInfoPath = "/api/v1/host-info"
)

// MaxBodyBytes is the largest request body the Dashboard accepts on these endpoints.
const MaxBodyBytes = 64 << 10

// MinInterval is the shortest report interval an Agent may use.
const MinInterval = time.Second

// Report is one periodic snapshot of a Host's changing metrics.
type Report struct {
	// Time is when the Agent took the sample, in Unix milliseconds.
	Time int64 `json:"time"`
	// IntervalMS is the Agent's configured report interval in milliseconds.
	IntervalMS int64 `json:"interval_ms"`

	// CPUPercent is overall CPU usage across all cores, 0–100.
	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	// MemoryUsed is memory in use, in bytes.
	MemoryUsed *uint64 `json:"memory_used,omitempty"`
	// SwapUsed is swap in use, in bytes.
	SwapUsed *uint64 `json:"swap_used,omitempty"`
	// Load1, Load5 and Load15 are the 1-, 5- and 15-minute load averages.
	// Agents send them only on systems that have them (Linux, macOS).
	Load1  *float64 `json:"load1,omitempty"`
	Load5  *float64 `json:"load5,omitempty"`
	Load15 *float64 `json:"load15,omitempty"`
	// DiskUsed is space in use, in bytes, summed over real local filesystems
	// (no tmpfs, overlay or network filesystems), each device counted once.
	DiskUsed *uint64 `json:"disk_used,omitempty"`
	// NetRxBytes and NetTxBytes are bytes received and sent since boot,
	// summed over the network interfaces the Agent is configured to count.
	NetRxBytes *uint64 `json:"net_rx_bytes,omitempty"`
	NetTxBytes *uint64 `json:"net_tx_bytes,omitempty"`
}

// HostInfo is the static description of a Host. Agents send it at start,
// whenever it changes, and periodically. Empty strings and nil pointers mean
// unknown.
type HostInfo struct {
	// Hostname is the OS hostname.
	Hostname string `json:"hostname,omitempty"`
	// OS is the operating system family: "linux", "windows" or "darwin".
	OS string `json:"os,omitempty"`
	// Platform and PlatformVersion name the distribution or edition, e.g.
	// "ubuntu" "24.04", "Microsoft Windows Server 2022" "10.0.20348", "darwin" "15.1".
	Platform        string `json:"platform,omitempty"`
	PlatformVersion string `json:"platform_version,omitempty"`
	KernelVersion   string `json:"kernel_version,omitempty"`
	// Arch is the machine architecture as the kernel reports it, e.g. "x86_64", "aarch64".
	Arch string `json:"arch,omitempty"`
	// CPUModel is the processor's model name; CPUCores counts logical cores.
	CPUModel string `json:"cpu_model,omitempty"`
	CPUCores *int   `json:"cpu_cores,omitempty"`
	// MemoryTotal, SwapTotal and DiskTotal are capacities in bytes. DiskTotal
	// covers the same filesystems as Report.DiskUsed.
	MemoryTotal *uint64 `json:"memory_total,omitempty"`
	SwapTotal   *uint64 `json:"swap_total,omitempty"`
	DiskTotal   *uint64 `json:"disk_total,omitempty"`
	// Virtualization is the hypervisor or container system the Host runs
	// under (e.g. "kvm", "lxc", "docker"), when it can be detected.
	Virtualization string `json:"virtualization,omitempty"`
	// BootTime is when the Host last booted, in Unix seconds.
	BootTime *int64 `json:"boot_time,omitempty"`
	// AgentVersion is the Agent's release version.
	AgentVersion string `json:"agent_version,omitempty"`
}
