package collector

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	netutil "github.com/shirou/gopsutil/v4/net"
)

// Collector gathers hardware and OS stats with minimal overhead.
type Collector struct {
	mu           sync.Mutex
	lastNetTime  time.Time
	lastBytesSent uint64
	lastBytesRecv uint64
	cachedCPU    CPUStats
	lastCPUTime  time.Time
}

// NewCollector initializes and warms up metric state.
func NewCollector() *Collector {
	c := &Collector{}
	// Warm up CPU and Net counters
	_, _ = cpu.Percent(0, false)
	_, _ = cpu.Percent(0, true)

	if ioCounters, err := netutil.IOCounters(false); err == nil && len(ioCounters) > 0 {
		c.lastNetTime = time.Now()
		c.lastBytesSent = ioCounters[0].BytesSent
		c.lastBytesRecv = ioCounters[0].BytesRecv
	}
	return c
}

// FormatUptime turns seconds into a readable string like "3d 4h 12m"
func FormatUptime(uptime uint64) string {
	days := uptime / (24 * 3600)
	hours := (uptime % (24 * 3600)) / 3600
	minutes := (uptime % 3600) / 60
	seconds := uptime % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

// Collect compiles the snapshot of the system.
func (c *Collector) Collect() (SystemSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	snap := SystemSnapshot{
		Timestamp: now.Unix(),
	}

	// 1. Host Stats
	hInfo, err := host.Info()
	if err == nil && hInfo != nil {
		snap.Host = HostStats{
			Hostname:      hInfo.Hostname,
			OS:            hInfo.OS,
			Platform:      fmt.Sprintf("%s %s", hInfo.Platform, hInfo.PlatformVersion),
			KernelVersion: hInfo.KernelVersion,
			Arch:          hInfo.KernelArch,
			Uptime:        hInfo.Uptime,
			UptimeStr:     FormatUptime(hInfo.Uptime),
			BootTime:      time.Unix(int64(hInfo.BootTime), 0),
			Procs:         hInfo.Procs,
		}
	}

	// Load Average (on Windows this may not be supported by OS, handle gracefully)
	if lAvg, err := load.Avg(); err == nil && lAvg != nil {
		snap.Host.Load1 = math.Round(lAvg.Load1*100) / 100
		snap.Host.Load5 = math.Round(lAvg.Load5*100) / 100
		snap.Host.Load15 = math.Round(lAvg.Load15*100) / 100
	}

	// 2. CPU Stats
	cpuPercentages, _ := cpu.Percent(0, false)
	coresPercentages, _ := cpu.Percent(0, true)
	cpuInfoList, _ := cpu.Info()

	var totalUsage float64
	if len(cpuPercentages) > 0 {
		totalUsage = math.Round(cpuPercentages[0]*10) / 10
	}

	roundedCores := make([]float64, len(coresPercentages))
	for i, val := range coresPercentages {
		roundedCores[i] = math.Round(val*10) / 10
	}

	logicalCores, _ := cpu.Counts(true)
	physicalCores, _ := cpu.Counts(false)

	model := "Unknown CPU"
	var mhz float64
	if len(cpuInfoList) > 0 {
		model = cpuInfoList[0].ModelName
		mhz = math.Round(cpuInfoList[0].Mhz)
	}

	snap.CPU = CPUStats{
		ModelName:     model,
		CoresLogical:  logicalCores,
		CoresPhysical: physicalCores,
		UsagePercent:  totalUsage,
		CoresUsage:    roundedCores,
		Mhz:           mhz,
	}

	// 3. Memory Stats
	vMem, err := mem.VirtualMemory()
	if err == nil && vMem != nil {
		snap.Memory.Total = vMem.Total
		snap.Memory.Used = vMem.Used
		snap.Memory.Available = vMem.Available
		snap.Memory.Free = vMem.Free
		snap.Memory.UsedPercent = math.Round(vMem.UsedPercent*10) / 10
	}

	sMem, err := mem.SwapMemory()
	if err == nil && sMem != nil {
		snap.Memory.SwapTotal = sMem.Total
		snap.Memory.SwapUsed = sMem.Used
		snap.Memory.SwapFree = sMem.Free
		snap.Memory.SwapPercent = math.Round(sMem.UsedPercent*10) / 10
	}

	// 4. Disk Partitions
	partitions, err := disk.Partitions(false)
	if err == nil {
		seenMounts := make(map[string]bool)
		for _, part := range partitions {
			// Avoid duplicates and virtual / read-only pseudo-filesystems when possible
			if seenMounts[part.Mountpoint] {
				continue
			}
			u, err := disk.Usage(part.Mountpoint)
			if err != nil || u.Total == 0 {
				continue
			}
			seenMounts[part.Mountpoint] = true
			snap.Disks = append(snap.Disks, DiskPartitionStats{
				Mountpoint:  part.Mountpoint,
				Device:      part.Device,
				Fstype:      part.Fstype,
				Total:       u.Total,
				Free:        u.Free,
				Used:        u.Used,
				UsedPercent: math.Round(u.UsedPercent*10) / 10,
			})
		}
	}

	// 5. Network Stats and Speed Rates
	ioCounters, err := netutil.IOCounters(false)
	if err == nil && len(ioCounters) > 0 {
		totalSent := ioCounters[0].BytesSent
		totalRecv := ioCounters[0].BytesRecv

		var txRate, rxRate float64
		if !c.lastNetTime.IsZero() {
			duration := now.Sub(c.lastNetTime).Seconds()
			if duration > 0.1 {
				if totalSent >= c.lastBytesSent {
					txRate = float64(totalSent-c.lastBytesSent) / duration
				}
				if totalRecv >= c.lastBytesRecv {
					rxRate = float64(totalRecv-c.lastBytesRecv) / duration
				}
			}
		}

		c.lastNetTime = now
		c.lastBytesSent = totalSent
		c.lastBytesRecv = totalRecv

		snap.Network = NetworkStats{
			BytesSent:   totalSent,
			BytesRecv:   totalRecv,
			PacketsSent: ioCounters[0].PacketsSent,
			PacketsRecv: ioCounters[0].PacketsRecv,
			TxRate:      math.Round(txRate),
			RxRate:      math.Round(rxRate),
		}
	}

	return snap, nil
}
