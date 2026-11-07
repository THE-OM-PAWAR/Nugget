package monitor

import (
	"fmt"
	"time"
)

// MachineMetrics captures real-time host resource utilization and health indicators.
type MachineMetrics struct {
	CPUPercent     float64   `json:"cpu_percent"`
	Load1          float64   `json:"load_1m"`
	Load5          float64   `json:"load_5m"`
	Load15         float64   `json:"load_15m"`
	TotalRAMBytes  uint64    `json:"total_ram_bytes"`
	UsedRAMBytes   uint64    `json:"used_ram_bytes"`
	FreeRAMBytes   uint64    `json:"free_ram_bytes"`
	RAMUsagePct    float64   `json:"ram_usage_percent"`
	TotalDiskBytes uint64    `json:"total_disk_bytes"`
	UsedDiskBytes  uint64    `json:"used_disk_bytes"`
	FreeDiskBytes  uint64    `json:"free_disk_bytes"`
	DiskUsagePct   float64   `json:"disk_usage_percent"`
	UptimeSeconds  uint64    `json:"uptime_seconds"`
	UptimeFormatted string   `json:"uptime_formatted"`
	NetworkRxBytes uint64    `json:"network_rx_bytes"`
	NetworkTxBytes uint64    `json:"network_tx_bytes"`
	CollectedAt    time.Time `json:"collected_at"`
}

// FormatUptime formats seconds into human-readable string (e.g. "3d 4h 12m").
func FormatUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	secs := seconds % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

