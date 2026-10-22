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

// HostInfo is the static description of a Host. Its fields arrive in a later slice.
type HostInfo struct{}
