package agent

import (
	"context"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
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
	// HostInfo returns the Host's static description. AgentVersion is filled
	// in by the caller; anything that cannot be read is left empty.
	HostInfo(ctx context.Context) v1.HostInfo
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
	r.DiskUsed, _ = diskUsage(ctx)
	r.NetRxBytes, r.NetTxBytes = c.traffic(ctx)
	return r
}

func (SystemCollector) HostInfo(ctx context.Context) v1.HostInfo {
	info := v1.HostInfo{OS: runtime.GOOS}
	if name, err := os.Hostname(); err == nil {
		info.Hostname = name
	}
	if platform, _, version, err := host.PlatformInformationWithContext(ctx); err == nil {
		info.Platform, info.PlatformVersion = platform, version
	}
	if v, err := host.KernelVersionWithContext(ctx); err == nil {
		info.KernelVersion = v
	}
	if a, err := host.KernelArch(); err == nil {
		info.Arch = a
	}
	if cpus, err := cpu.InfoWithContext(ctx); err == nil && len(cpus) > 0 {
		info.CPUModel = strings.TrimSpace(cpus[0].ModelName)
	}
	if n, err := cpu.CountsWithContext(ctx, true); err == nil && n > 0 {
		info.CPUCores = &n
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		info.MemoryTotal = &vm.Total
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		info.SwapTotal = &sw.Total
	}
	_, info.DiskTotal = diskUsage(ctx)
	if system, role, err := host.VirtualizationWithContext(ctx); err == nil && role == "guest" {
		info.Virtualization = system
	}
	if bt, err := host.BootTimeWithContext(ctx); err == nil && bt > 0 {
		t := int64(bt)
		info.BootTime = &t
	}
	return info
}

// diskUsage returns used and total bytes over real local filesystems.
func diskUsage(ctx context.Context) (used, total *uint64) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, nil
	}
	return diskUsageOf(parts, runtime.GOOS, func(path string) (*disk.UsageStat, error) {
		return disk.UsageWithContext(ctx, path)
	})
}

// diskUsageOf sums usage over the counted partitions. Where none counts, as
// in a container whose root is overlay (Docker, many LXC VPSes), it falls
// back to the usage of the root filesystem, which reports the storage the
// container actually lives on.
func diskUsageOf(parts []disk.PartitionStat, goos string, usage func(string) (*disk.UsageStat, error)) (used, total *uint64) {
	var usages []fsUsage
	for _, p := range parts {
		if !countedPartition(p, goos) {
			continue
		}
		u, err := usage(p.Mountpoint)
		if err != nil {
			continue
		}
		usages = append(usages, fsUsage{device: p.Device, total: u.Total, free: u.Free, used: u.Used})
	}
	if used, total = sumDisk(usages); used != nil {
		return used, total
	}
	root := "/"
	if goos == "windows" {
		root = `C:\`
	}
	if u, err := usage(root); err == nil && u.Total > 0 {
		return &u.Used, &u.Total
	}
	return nil, nil
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

type fsUsage struct {
	device            string
	total, free, used uint64
}

// sumDisk adds up used and total space, counting each device once. Volumes
// that share one storage pool (APFS containers and the like) all report the
// pool's size and free space, so identical (total, free) pairs count once too.
func sumDisk(usages []fsUsage) (used, total *uint64) {
	type pool struct{ total, free uint64 }
	seenDevice := make(map[string]bool)
	seenPool := make(map[pool]bool)
	var sumUsed, sumTotal uint64
	var counted bool
	for _, u := range usages {
		key := pool{u.total, u.free}
		if u.total == 0 || seenDevice[u.device] || seenPool[key] {
			continue
		}
		seenDevice[u.device] = true
		seenPool[key] = true
		sumUsed += u.used
		sumTotal += u.total
		counted = true
	}
	if !counted {
		return nil, nil
	}
	return &sumUsed, &sumTotal
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
