package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	netutil "github.com/shirou/gopsutil/v4/net"
)

// Collector gathers hardware and OS stats with minimal overhead and background broadcasting.
type Collector struct {
	mu             sync.Mutex
	lastNetTime    time.Time
	lastBytesSent  uint64
	lastBytesRecv  uint64

	// Static hardware info cached once at startup
	staticHost     HostStats
	staticCPU      CPUStats

	// Cached disk partitions (refreshed periodically to avoid frequent OS partition enumeration)
	cachedPartitions   []disk.PartitionStat
	lastPartitionCheck time.Time

	// Real-time cached state for lock-free read
	latestSnapshot atomic.Pointer[SystemSnapshot]
	latestJSON     atomic.Pointer[[]byte]

	// SSE Broadcast broker
	broker *Broker
}

// NewCollector initializes and warms up metric state, caching static hardware attributes.
func NewCollector() *Collector {
	c := &Collector{
		broker: NewBroker(),
	}

	// 1. Warm up CPU and Net counters
	_, _ = cpu.Percent(0, false)
	_, _ = cpu.Percent(0, true)

	if ioCounters, err := netutil.IOCounters(false); err == nil && len(ioCounters) > 0 {
		c.lastNetTime = time.Now()
		c.lastBytesSent = ioCounters[0].BytesSent
		c.lastBytesRecv = ioCounters[0].BytesRecv
	}

	// 2. Cache immutable Host info once
	if hInfo, err := host.Info(); err == nil && hInfo != nil {
		c.staticHost = HostStats{
			Hostname:      hInfo.Hostname,
			OS:            hInfo.OS,
			Platform:      fmt.Sprintf("%s %s", hInfo.Platform, hInfo.PlatformVersion),
			KernelVersion: hInfo.KernelVersion,
			Arch:          hInfo.KernelArch,
			BootTime:      time.Unix(int64(hInfo.BootTime), 0),
		}
	}

	// 3. Cache immutable CPU specs once
	logicalCores, _ := cpu.Counts(true)
	physicalCores, _ := cpu.Counts(false)
	cpuInfoList, _ := cpu.Info()

	model := "Unknown CPU"
	var mhz float64
	if len(cpuInfoList) > 0 {
		model = cpuInfoList[0].ModelName
		mhz = math.Round(cpuInfoList[0].Mhz)
	}

	c.staticCPU = CPUStats{
		ModelName:     model,
		CoresLogical:  logicalCores,
		CoresPhysical: physicalCores,
		Mhz:           mhz,
	}

	return c
}

// Broker returns the SSE event broker.
func (c *Collector) Broker() *Broker {
	return c.broker
}

// GetLatestSnapshot returns the most recently collected system snapshot in a lock-free manner.
func (c *Collector) GetLatestSnapshot() *SystemSnapshot {
	return c.latestSnapshot.Load()
}

// GetLatestJSON returns the pre-marshaled JSON representation of the latest snapshot.
func (c *Collector) GetLatestJSON() []byte {
	if ptr := c.latestJSON.Load(); ptr != nil {
		return *ptr
	}
	return nil
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

// updateAndBroadcast runs a collection cycle, caches JSON, and notifies subscribers.
func (c *Collector) updateAndBroadcast() {
	snap, err := c.Collect()
	if err != nil {
		return
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return
	}

	c.latestSnapshot.Store(&snap)
	c.latestJSON.Store(&data)

	c.broker.Broadcast(data)
}

// Start launches a background goroutine that runs Collect() at the specified interval
// and streams updates to all SSE subscribers.
func (c *Collector) Start(ctx context.Context, interval time.Duration) {
	// Prime immediately on startup
	c.updateAndBroadcast()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.updateAndBroadcast()
			}
		}
	}()
}

// Collect compiles the snapshot of the system.
func (c *Collector) Collect() (SystemSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	snap := SystemSnapshot{
		Timestamp: now.Unix(),
	}

	// 1. Host Stats (Combine static specs with dynamic runtime metrics)
	snap.Host = c.staticHost
	if hInfo, err := host.Info(); err == nil && hInfo != nil {
		snap.Host.Uptime = hInfo.Uptime
		snap.Host.UptimeStr = FormatUptime(hInfo.Uptime)
		snap.Host.Procs = hInfo.Procs
		if snap.Host.Hostname == "" {
			snap.Host.Hostname = hInfo.Hostname
		}
	}

	// Load Average (handled gracefully if unsupported on platform)
	if lAvg, err := load.Avg(); err == nil && lAvg != nil {
		snap.Host.Load1 = math.Round(lAvg.Load1*100) / 100
		snap.Host.Load5 = math.Round(lAvg.Load5*100) / 100
		snap.Host.Load15 = math.Round(lAvg.Load15*100) / 100
	}

	// 2. CPU Stats (Combine static model/counts with dynamic usage percentages)
	cpuPercentages, _ := cpu.Percent(0, false)
	coresPercentages, _ := cpu.Percent(0, true)

	var totalUsage float64
	if len(cpuPercentages) > 0 {
		totalUsage = math.Round(cpuPercentages[0]*10) / 10
	}

	roundedCores := make([]float64, len(coresPercentages))
	for i, val := range coresPercentages {
		roundedCores[i] = math.Round(val*10) / 10
	}

	snap.CPU = c.staticCPU
	snap.CPU.UsagePercent = totalUsage
	snap.CPU.CoresUsage = roundedCores

	// 3. Memory Stats
	if vMem, err := mem.VirtualMemory(); err == nil && vMem != nil {
		snap.Memory.Total = vMem.Total
		snap.Memory.Used = vMem.Used
		snap.Memory.Available = vMem.Available
		snap.Memory.Free = vMem.Free
		snap.Memory.UsedPercent = math.Round(vMem.UsedPercent*10) / 10
	}

	if sMem, err := mem.SwapMemory(); err == nil && sMem != nil {
		snap.Memory.SwapTotal = sMem.Total
		snap.Memory.SwapUsed = sMem.Used
		snap.Memory.SwapFree = sMem.Free
		snap.Memory.SwapPercent = math.Round(sMem.UsedPercent*10) / 10
	}

	// 4. Disk Partitions (Refresh partition list only every 30 seconds to minimize syscalls)
	if now.Sub(c.lastPartitionCheck) > 30*time.Second || len(c.cachedPartitions) == 0 {
		if parts, err := disk.Partitions(false); err == nil {
			c.cachedPartitions = parts
			c.lastPartitionCheck = now
		}
	}

	seenMounts := make(map[string]bool)
	for _, part := range c.cachedPartitions {
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
