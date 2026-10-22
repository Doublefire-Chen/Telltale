package agent

import (
	"context"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
)

// Collector samples a Host's metrics. It is the only place the Agent reads
// system state; the rest of the Agent never imports gopsutil, so any metric
// can be replaced by a hand-written collector.
type Collector interface {
	// Sample returns the current metrics. Time and IntervalMS are filled in by
	// the caller. A metric that cannot be read is left nil rather than failing
	// the whole Report.
	Sample(ctx context.Context) v1.Report
}

// SystemCollector reads metrics from the OS through gopsutil.
type SystemCollector struct{}

func (SystemCollector) Sample(ctx context.Context) v1.Report {
	var r v1.Report
	// With a zero interval, gopsutil reports usage since its previous call,
	// which is the previous Report.
	if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) == 1 {
		r.CPUPercent = &pcts[0]
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		r.MemoryUsed = &vm.Used
	}
	return r
}
