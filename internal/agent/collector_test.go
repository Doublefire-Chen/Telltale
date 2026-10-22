package agent

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestDefaultExclusionsCoverVirtualInterfaces(t *testing.T) {
	for _, name := range []string{"lo", "lo0", "docker0", "veth1a2b", "br-0f3a", "virbr0", "tun0", "utun3", "wg0", "awdl0", "Loopback Pseudo-Interface 1"} {
		if !matchesAny(name, DefaultExcludeInterfaces) {
			t.Errorf("%s is counted by default", name)
		}
	}
	for _, name := range []string{"eth0", "ens3", "enp0s31f6", "en0", "wlan0", "Ethernet", "bond0"} {
		if matchesAny(name, DefaultExcludeInterfaces) {
			t.Errorf("%s is excluded by default", name)
		}
	}
}

func TestConfigExcludeInterfaces(t *testing.T) {
	const base = "dashboard_url = \"https://status.example.com\"\ntoken = \"tt_abc\"\n"

	cfg, err := loadConfigString(t, base)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExcludeInterfaces != nil {
		t.Fatalf("unset exclude_interfaces = %v, want nil (defaults)", cfg.ExcludeInterfaces)
	}

	cfg, err = loadConfigString(t, base+"exclude_interfaces = [\"eth1\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if !matchesAny("eth1", cfg.ExcludeInterfaces) || matchesAny("docker0", cfg.ExcludeInterfaces) {
		t.Fatalf("exclude_interfaces = %v should replace the defaults", cfg.ExcludeInterfaces)
	}

	// An explicit empty list counts every interface.
	cfg, err = loadConfigString(t, base+"exclude_interfaces = []\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExcludeInterfaces == nil || len(cfg.ExcludeInterfaces) != 0 {
		t.Fatalf("empty exclude_interfaces = %#v, want an empty non-nil list", cfg.ExcludeInterfaces)
	}

	if _, err := loadConfigString(t, base+"exclude_interfaces = [\"eth[\"]\n"); err == nil {
		t.Fatal("malformed pattern accepted")
	}
}

// TestSystemCollectorSmoke checks the real collectors on whatever OS runs the tests.
func TestSystemCollectorSmoke(t *testing.T) {
	r := SystemCollector{}.Sample(context.Background())
	if r.CPUPercent == nil || *r.CPUPercent < 0 || *r.CPUPercent > 100 {
		t.Errorf("cpu_percent = %v", r.CPUPercent)
	}
	if r.MemoryUsed == nil || *r.MemoryUsed == 0 {
		t.Errorf("memory_used = %v", r.MemoryUsed)
	}
	if r.SwapUsed == nil {
		t.Error("swap_used missing")
	}
	if r.DiskUsed == nil || *r.DiskUsed == 0 {
		t.Errorf("disk_used = %v", r.DiskUsed)
	}
	if r.NetRxBytes == nil || r.NetTxBytes == nil {
		t.Error("network counters missing")
	}
	if hasLoad := r.Load1 != nil && r.Load5 != nil && r.Load15 != nil; hasLoad == (runtime.GOOS == "windows") {
		t.Errorf("load average present=%v on %s", hasLoad, runtime.GOOS)
	}

	info := SystemCollector{}.HostInfo(context.Background())
	if info.Hostname == "" || info.OS != runtime.GOOS || info.Arch == "" || info.KernelVersion == "" {
		t.Errorf("host info identity incomplete: %+v", info)
	}
	if info.CPUCores == nil || *info.CPUCores < 1 || info.MemoryTotal == nil || info.DiskTotal == nil || info.BootTime == nil {
		t.Errorf("host info capacities incomplete: %+v", info)
	}
	if info.DiskTotal != nil && r.DiskUsed != nil && *r.DiskUsed > *info.DiskTotal {
		t.Errorf("disk used %d exceeds disk total %d", *r.DiskUsed, *info.DiskTotal)
	}

	// Excluding every interface leaves zero Traffic, not a missing value.
	r = SystemCollector{ExcludeInterfaces: []string{"*"}}.Sample(context.Background())
	if r.NetRxBytes == nil || *r.NetRxBytes != 0 {
		t.Errorf("all interfaces excluded: net_rx_bytes = %v, want 0", r.NetRxBytes)
	}
}

func TestDiskCountsEachStorageOnce(t *testing.T) {
	used, total := sumDisk([]fsUsage{
		{device: "/dev/sda1", total: 100, free: 40, used: 60},
		{device: "/dev/sda1", total: 100, free: 40, used: 60}, // bind mount of the same device
		{device: "/dev/disk3s1", total: 500, free: 100, used: 400},
		{device: "/dev/disk3s5", total: 500, free: 100, used: 400}, // another volume in the same APFS container
		{device: "/dev/sdb1", total: 200, free: 150, used: 50},
		{device: "/dev/disk1s2", total: 0, free: 0, used: 0},
	})
	if used == nil || *used != 60+400+50 || total == nil || *total != 100+500+200 {
		t.Fatalf("disk used=%v total=%v, want 510 and 800", used, total)
	}
	if used, total := sumDisk(nil); used != nil || total != nil {
		t.Fatal("no filesystems should give unknown, not zero")
	}
}

func TestCountedPartition(t *testing.T) {
	for _, tc := range []struct {
		p    disk.PartitionStat
		goos string
		want bool
	}{
		{disk.PartitionStat{Fstype: "ext4", Opts: []string{"rw"}}, "linux", true},
		{disk.PartitionStat{Fstype: "tmpfs"}, "linux", false},
		{disk.PartitionStat{Fstype: "overlay"}, "linux", false},
		{disk.PartitionStat{Fstype: "nfs4"}, "linux", false},
		{disk.PartitionStat{Fstype: "apfs", Opts: []string{"ro", "local"}}, "darwin", true},
		{disk.PartitionStat{Fstype: "apfs", Opts: []string{"rw", "nobrowse", "local"}}, "darwin", false},
		{disk.PartitionStat{Fstype: "devfs"}, "darwin", false},
	} {
		if got := countedPartition(tc.p, tc.goos); got != tc.want {
			t.Errorf("%s %v on %s: counted=%v, want %v", tc.p.Fstype, tc.p.Opts, tc.goos, got, tc.want)
		}
	}
}

func TestDiskFallsBackToRootInContainers(t *testing.T) {
	usage := func(path string) (*disk.UsageStat, error) {
		switch path {
		case "/":
			return &disk.UsageStat{Total: 100, Free: 70, Used: 30}, nil
		case "/data":
			return &disk.UsageStat{Total: 500, Free: 100, Used: 400}, nil
		}
		return nil, fmt.Errorf("no such mount %s", path)
	}
	// A container: root is overlay, the rest are pseudo filesystems.
	container := []disk.PartitionStat{
		{Device: "overlay", Mountpoint: "/", Fstype: "overlay"},
		{Device: "tmpfs", Mountpoint: "/dev", Fstype: "tmpfs"},
	}
	used, total := diskUsageOf(container, "linux", usage)
	if used == nil || *used != 30 || *total != 100 {
		t.Fatalf("container: used=%v total=%v, want the root filesystem's 30/100", used, total)
	}
	// With a real filesystem, root is not added on top.
	real := append(container, disk.PartitionStat{Device: "/dev/vdb1", Mountpoint: "/data", Fstype: "ext4"})
	used, total = diskUsageOf(real, "linux", usage)
	if *used != 400 || *total != 500 {
		t.Fatalf("with a real filesystem: used=%d total=%d, want 400/500", *used, *total)
	}
}
