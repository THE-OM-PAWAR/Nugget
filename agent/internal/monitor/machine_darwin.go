//go:build darwin

package monitor

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CollectMachineMetrics collects system metrics on macOS.
func CollectMachineMetrics() (*MachineMetrics, error) {
	now := time.Now().UTC()
	m := &MachineMetrics{
		CollectedAt: now,
	}

	// 1. Load Average from sysctl
	if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		fields := strings.Fields(strings.Trim(string(out), "{ }"))
		if len(fields) >= 3 {
			m.Load1, _ = strconv.ParseFloat(fields[0], 64)
			m.Load5, _ = strconv.ParseFloat(fields[1], 64)
			m.Load15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// 2. Uptime from boottime
	if out, err := exec.Command("sysctl", "-n", "kern.boottime").Output(); err == nil {
		// Output format: { sec = 1731000000, usec = 123456 }
		str := string(out)
		if idx := strings.Index(str, "sec = "); idx != -1 {
			secStr := strings.Split(str[idx+6:], ",")[0]
			if bootSec, err := strconv.ParseInt(strings.TrimSpace(secStr), 10, 64); err == nil {
				uptime := time.Now().Unix() - bootSec
				if uptime > 0 {
					m.UptimeSeconds = uint64(uptime)
					m.UptimeFormatted = FormatUptime(m.UptimeSeconds)
				}
			}
		}
	}

	// 3. RAM
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil {
			m.TotalRAMBytes = val
			// Approximation for free memory
			m.FreeRAMBytes = val / 4
			m.UsedRAMBytes = val - m.FreeRAMBytes
			m.RAMUsagePct = (float64(m.UsedRAMBytes) / float64(val)) * 100.0
		}
	}

	// 4. Disk
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		total := uint64(stat.Blocks) * uint64(stat.Bsize)
		free := uint64(stat.Bavail) * uint64(stat.Bsize)
		m.TotalDiskBytes = total
		m.FreeDiskBytes = free
		if total >= free {
			m.UsedDiskBytes = total - free
		}
		if total > 0 {
			m.DiskUsagePct = (float64(m.UsedDiskBytes) / float64(total)) * 100.0
		}
	}

	// 5. CPU % approximation from load1
	if m.Load1 > 0 {
		m.CPUPercent = m.Load1 * 10.0
		if m.CPUPercent > 100.0 {
			m.CPUPercent = 100.0
		}
	}

	return m, nil
}

