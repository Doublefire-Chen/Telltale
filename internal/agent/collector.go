package agent

import (
	"context"
	"path"
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

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

// DefaultExcludeInterfaces are network interfaces left out of Traffic by
// default: loopback, plus the virtual interfaces of containers, bridges and
// tunnels, whose traffic also crosses a physical interface and would
// otherwise be counted twice.
var DefaultExcludeInterfaces = []string{
	"lo", "lo0", "Loopback*",
	"docker*", "veth*", "br-*", "virbr*", "cni*", "flannel*", "cali*", "vxlan*", "kube-*",
	"tun*", "tap*", "utun*", "wg*",
	"awdl*", "llw*", "gif*", "stf*", "bridge*", "anpi*",
}

// excludedFSTypes are filesystems never counted as disk usage: pseudo, memory
// and network filesystems. On Linux gopsutil already drops "nodev" types;
// this list also covers macOS and anything that slips through.
var excludedFSTypes = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "ramfs": true, "overlay": true, "squashfs": true,
	"devfs": true, "autofs": true, "nullfs": true, "procfs": true,
	"nfs": true, "nfs4": true, "cifs": true, "smbfs": true, "afpfs": true, "webdav": true,
	"fuse.sshfs": true, "9p": true,
}

// SystemCollector reads metrics from the OS through gopsutil.
type SystemCollector struct {
	// ExcludeInterfaces are glob patterns of interface names left out of
	// Traffic. Nil means DefaultExcludeInterfaces.
	ExcludeInterfaces []string
}

func (c SystemCollector) Sample(ctx context.Context) v1.Report {
	var r v1.Report
	// With a zero interval, gopsutil reports usage since its previous call,
	// which is the previous Report.
	if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) == 1 {
		r.CPUPercent = &pcts[0]
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		r.MemoryUsed = &vm.Used
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		r.SwapUsed = &sw.Used
	}
	// Windows has no load average; gopsutil would emulate one with a
	// background goroutine, which costs CPU for a number that means little.
	if runtime.GOOS != "windows" {
		if avg, err := load.AvgWithContext(ctx); err == nil {
			r.Load1, r.Load5, r.Load15 = &avg.Load1, &avg.Load5, &avg.Load15
		}
	}
	r.DiskUsed = diskUsed(ctx)
	r.NetRxBytes, r.NetTxBytes = c.traffic(ctx)
	return r
}

func diskUsed(ctx context.Context) *uint64 {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil
	}
	var usages []diskUsage
	for _, p := range parts {
		if !countedPartition(p, runtime.GOOS) {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}
		usages = append(usages, diskUsage{device: p.Device, total: u.Total, free: u.Free, used: u.Used})
	}
	return sumDiskUsed(usages)
}

// countedPartition reports whether a mounted filesystem is real local storage.
func countedPartition(p disk.PartitionStat, goos string) bool {
	if excludedFSTypes[p.Fstype] {
		return false
	}
	// On macOS, system volumes (VM, Preboot, Data, …) and mounted disk images
	// are "nobrowse"; they share the root volume's APFS container or are files
	// on it, so only browsable volumes (root, external drives) are counted.
	if goos == "darwin" {
		for _, opt := range p.Opts {
			if opt == "nobrowse" {
				return false
			}
		}
	}
	return true
}

type diskUsage struct {
	device            string
	total, free, used uint64
}

// sumDiskUsed adds up used space, counting each device once. Volumes that
// share one storage pool (APFS containers and the like) all report the pool's
// size and free space, so identical (total, free) pairs are counted once too.
func sumDiskUsed(usages []diskUsage) *uint64 {
	type pool struct{ total, free uint64 }
	seenDevice := make(map[string]bool)
	seenPool := make(map[pool]bool)
	var sum uint64
	var counted bool
	for _, u := range usages {
		key := pool{u.total, u.free}
		if u.total == 0 || seenDevice[u.device] || seenPool[key] {
			continue
		}
		seenDevice[u.device] = true
		seenPool[key] = true
		sum += u.used
		counted = true
	}
	if !counted {
		return nil
	}
	return &sum
}

func (c SystemCollector) traffic(ctx context.Context) (rx, tx *uint64) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, nil
	}
	exclude := c.ExcludeInterfaces
	if exclude == nil {
		exclude = DefaultExcludeInterfaces
	}
	var sumRx, sumTx uint64
	for _, ic := range counters {
		if matchesAny(ic.Name, exclude) {
			continue
		}
		sumRx += ic.BytesRecv
		sumTx += ic.BytesSent
	}
	return &sumRx, &sumTx
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}
