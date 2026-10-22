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
}

// HostInfo is the static description of a Host. Its fields arrive in a later slice.
type HostInfo struct{}
