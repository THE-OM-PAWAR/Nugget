//go:build linux

package monitor

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	lastCPUTotal uint64
	lastCPUIdle  uint64
)

// CollectMachineMetrics collects real-time system metrics from /proc on Linux.
func CollectMachineMetrics() (*MachineMetrics, error) {
	now := time.Now().UTC()
	m := &MachineMetrics{
		CollectedAt: now,
	}

	// 1. Load Average
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			m.Load1, _ = strconv.ParseFloat(fields[0], 64)
			m.Load5, _ = strconv.ParseFloat(fields[1], 64)
			m.Load15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// 2. Uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			if upSecs, err := strconv.ParseFloat(fields[0], 64); err == nil {
				m.UptimeSeconds = uint64(upSecs)
				m.UptimeFormatted = FormatUptime(m.UptimeSeconds)
			}
		}
	}

	// 3. RAM from /proc/meminfo
	if file, err := os.Open("/proc/meminfo"); err == nil {
		defer file.Close()
		var memTotal, memFree, memAvail uint64
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				switch fields[0] {
				case "MemTotal:":
					val, _ := strconv.ParseUint(fields[1], 10, 64)
					memTotal = val * 1024
				case "MemFree:":
					val, _ := strconv.ParseUint(fields[1], 10, 64)
					memFree = val * 1024
				case "MemAvailable:":
					val, _ := strconv.ParseUint(fields[1], 10, 64)
					memAvail = val * 1024
				}
			}
		}
		_ = scanner.Err()

		m.TotalRAMBytes = memTotal
		if memAvail > 0 {
			m.FreeRAMBytes = memAvail
		} else {
			m.FreeRAMBytes = memFree
		}
		if m.TotalRAMBytes > m.FreeRAMBytes {
			m.UsedRAMBytes = m.TotalRAMBytes - m.FreeRAMBytes
		}
		if m.TotalRAMBytes > 0 {
			m.RAMUsagePct = (float64(m.UsedRAMBytes) / float64(m.TotalRAMBytes)) * 100.0
		}
	}

	// 4. Disk utilization from syscall.Statfs
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

	// 5. Network statistics from /proc/net/dev
	if file, err := os.Open("/proc/net/dev"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		var totalRx, totalTx uint64
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					iface := strings.TrimSpace(parts[0])
					if iface == "lo" {
						continue // Skip loopback
					}
					fields := strings.Fields(parts[1])
					if len(fields) >= 9 {
						rx, _ := strconv.ParseUint(fields[0], 10, 64)
						tx, _ := strconv.ParseUint(fields[8], 10, 64)
						totalRx += rx
						totalTx += tx
					}
				}
			}
		}
		_ = scanner.Err()
		m.NetworkRxBytes = totalRx
		m.NetworkTxBytes = totalTx
	}

	// 6. CPU Usage % from /proc/stat
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)[1:]
				var total, idle uint64
				for i, f := range fields {
					val, _ := strconv.ParseUint(f, 10, 64)
					total += val
					if i == 3 { // idle field
						idle = val
					}
				}
				if lastCPUTotal > 0 && total > lastCPUTotal {
					diffTotal := float64(total - lastCPUTotal)
					diffIdle := float64(idle - lastCPUIdle)
					m.CPUPercent = ((diffTotal - diffIdle) / diffTotal) * 100.0
				}
				lastCPUTotal = total
				lastCPUIdle = idle
				break
			}
		}
	}

	return m, nil
}

