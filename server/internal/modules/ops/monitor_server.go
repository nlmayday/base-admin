package ops

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"go.uber.org/zap"
)

// cpuPrimeInterval is sampled on the first collection; later collections
// compare with the previous call.
const cpuPrimeInterval = 200 * time.Millisecond

type hostInfo struct {
	Hostname        string    `json:"hostname"`
	OS              string    `json:"os"`
	Platform        string    `json:"platform"`
	PlatformVersion string    `json:"platformVersion"`
	KernelVersion   string    `json:"kernelVersion"`
	KernelArch      string    `json:"kernelArch"`
	UptimeSeconds   uint64    `json:"uptimeSeconds"`
	BootTime        time.Time `json:"bootTime"`
	Procs           uint64    `json:"procs"`
}

type cpuInfo struct {
	ModelName     string    `json:"modelName"`
	LogicalCores  int       `json:"logicalCores"`
	PhysicalCores int       `json:"physicalCores"`
	Percent       float64   `json:"percent"`
	PerCore       []float64 `json:"perCore"`
}

type loadInfo struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type memInfo struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Percent   float64 `json:"percent"`
}

type diskInfo struct {
	Mount   string  `json:"mount"`
	Device  string  `json:"device"`
	FSType  string  `json:"fsType"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

// serverStats is the host view. Sections that cannot be collected on this
// platform are null (load) or empty (disks).
type serverStats struct {
	CollectedAt time.Time  `json:"collectedAt"`
	Host        hostInfo   `json:"host"`
	CPU         cpuInfo    `json:"cpu"`
	Load        *loadInfo  `json:"load"`
	Memory      memInfo    `json:"memory"`
	Swap        memInfo    `json:"swap"`
	Disks       []diskInfo `json:"disks"`
}

func (mo *monitor) collectServer(ctx context.Context) (serverStats, error) {
	out := serverStats{CollectedAt: time.Now(), Disks: []diskInfo{}}
	var errs []error
	keep := func(section string, err error) {
		if err != nil {
			mo.logger.Debug("monitor section unavailable", zap.String("section", section), zap.Error(err))
			errs = append(errs, err)
		}
	}
	keep("host", mo.fillHost(ctx, &out))
	keep("cpu", mo.fillCPU(ctx, &out))
	keep("memory", fillMemory(ctx, &out))
	keep("disk", fillDisks(ctx, &out))
	if avg, err := load.AvgWithContext(ctx); err == nil {
		out.Load = &loadInfo{Load1: round2(avg.Load1), Load5: round2(avg.Load5), Load15: round2(avg.Load15)}
	}
	if len(errs) == 4 {
		return serverStats{}, errMonitor().WithCause(errors.Join(errs...))
	}
	return out, nil
}

func (mo *monitor) fillHost(ctx context.Context, out *serverStats) error {
	h, err := host.InfoWithContext(ctx)
	if err != nil {
		return err
	}
	out.Host = hostInfo{
		Hostname: h.Hostname, OS: h.OS, Platform: h.Platform, PlatformVersion: h.PlatformVersion,
		KernelVersion: h.KernelVersion, KernelArch: h.KernelArch, UptimeSeconds: h.Uptime,
		BootTime: time.Unix(int64(min(h.BootTime, math.MaxInt64)), 0).UTC(), Procs: h.Procs, // #nosec G115 -- clamped
	}
	return nil
}

// fillCPU runs under the server cache lock, so cpuPrimed needs no extra lock.
func (mo *monitor) fillCPU(ctx context.Context, out *serverStats) error {
	interval := time.Duration(0)
	if !mo.cpuPrimed {
		interval = cpuPrimeInterval
	}
	total, err := cpu.PercentWithContext(ctx, interval, false)
	if err != nil {
		return err
	}
	perCore, err := cpu.PercentWithContext(ctx, interval, true)
	if err != nil {
		return err
	}
	mo.cpuPrimed = true
	info := cpuInfo{PerCore: make([]float64, len(perCore))}
	for i, p := range perCore {
		info.PerCore[i] = round2(p)
	}
	if len(total) > 0 {
		info.Percent = round2(total[0])
	}
	info.LogicalCores, _ = cpu.CountsWithContext(ctx, true)
	info.PhysicalCores, _ = cpu.CountsWithContext(ctx, false)
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		info.ModelName = strings.TrimSpace(infos[0].ModelName)
	}
	out.CPU = info
	return nil
}

func fillMemory(ctx context.Context, out *serverStats) error {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return err
	}
	out.Memory = memInfo{Total: vm.Total, Used: vm.Used, Available: vm.Available, Percent: round2(vm.UsedPercent)}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		out.Swap = memInfo{Total: sw.Total, Used: sw.Used, Available: sw.Free, Percent: round2(sw.UsedPercent)}
	}
	return nil
}

// pseudoFS are filesystems that do not represent storage.
var pseudoFS = map[string]bool{
	"autofs": true, "binfmt_misc": true, "bpf": true, "cgroup": true, "cgroup2": true, "configfs": true,
	"debugfs": true, "devfs": true, "devpts": true, "devtmpfs": true, "fusectl": true, "hugetlbfs": true,
	"mqueue": true, "nsfs": true, "nullfs": true, "proc": true, "pstore": true, "ramfs": true,
	"rpc_pipefs": true, "securityfs": true, "selinuxfs": true, "squashfs": true, "sysfs": true,
	"tmpfs": true, "tracefs": true, "efivarfs": true, "fuse.lxcfs": true,
}

// skipMount drops pseudo filesystems, snap images and macOS system volumes
// (they share the root APFS container).
// skippedMountPrefixes are read-only images and OS-managed volumes that say
// nothing about the space the application can use.
var skippedMountPrefixes = []string{
	"/snap/", "/System/Volumes/", "/System/Library/", "/private/var/vm", "/Library/Developer/CoreSimulator/",
}

// minDiskBytes hides tiny volumes (boot, recovery and asset images).
const minDiskBytes = 1 << 30

func skipMount(p disk.PartitionStat) bool {
	if pseudoFS[p.Fstype] {
		return true
	}
	for _, prefix := range skippedMountPrefixes {
		if strings.HasPrefix(p.Mountpoint, prefix) {
			return true
		}
	}
	return false
}

func fillDisks(ctx context.Context, out *serverStats) error {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range parts {
		if skipMount(p) || seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total < minDiskBytes {
			continue
		}
		out.Disks = append(out.Disks, diskInfo{Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype,
			Total: u.Total, Used: u.Used, Free: u.Free, Percent: round2(u.UsedPercent)})
	}
	return nil
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
