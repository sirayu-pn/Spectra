package collector

import "time"

// HostStats holds system and OS level information
type HostStats struct {
	Hostname      string    `json:"hostname"`
	OS            string    `json:"os"`
	Platform      string    `json:"platform"`
	KernelVersion string    `json:"kernel_version"`
	Arch          string    `json:"arch"`
	Uptime        uint64    `json:"uptime"`
	UptimeStr     string    `json:"uptime_str"`
	BootTime      time.Time `json:"boot_time"`
	Procs         uint64    `json:"procs"`
	Load1         float64   `json:"load_1"`
	Load5         float64   `json:"load_5"`
	Load15        float64   `json:"load_15"`
}

// CPUStats holds overall and per-core CPU metrics
type CPUStats struct {
	ModelName      string    `json:"model_name"`
	CoresLogical   int       `json:"cores_logical"`
	CoresPhysical  int       `json:"cores_physical"`
	UsagePercent   float64   `json:"usage_percent"`
	CoresUsage     []float64 `json:"cores_usage"`
	Mhz            float64   `json:"mhz"`
}

// MemoryStats holds RAM and Swap metrics
type MemoryStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
	SwapTotal   uint64  `json:"swap_total"`
	SwapUsed    uint64  `json:"swap_used"`
	SwapFree    uint64  `json:"swap_free"`
	SwapPercent float64 `json:"swap_percent"`
}

// DiskPartitionStats holds mount and usage data for a filesystem partition
type DiskPartitionStats struct {
	Mountpoint  string  `json:"mountpoint"`
	Device      string  `json:"device"`
	Fstype      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Free        uint64  `json:"free"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"used_percent"`
}

// NetworkStats holds cumulative counters and calculated real-time rates
type NetworkStats struct {
	BytesSent   uint64  `json:"bytes_sent"`
	BytesRecv   uint64  `json:"bytes_recv"`
	PacketsSent uint64  `json:"packets_sent"`
	PacketsRecv uint64  `json:"packets_recv"`
	TxRate      float64 `json:"tx_rate_bps"` // Bytes per second sent
	RxRate      float64 `json:"rx_rate_bps"` // Bytes per second received
}

// SystemSnapshot represents the complete metrics payload sent to clients
type SystemSnapshot struct {
	Timestamp int64                `json:"timestamp"` // Unix timestamp in seconds
	Host      HostStats            `json:"host"`
	CPU       CPUStats             `json:"cpu"`
	Memory    MemoryStats          `json:"memory"`
	Disks     []DiskPartitionStats `json:"disks"`
	Network   NetworkStats         `json:"network"`
}
