package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SystemInfo encapsulates detected host system specifications.
type SystemInfo struct {
	OS             string `json:"os"`
	Kernel         string `json:"kernel"`
	Arch           string `json:"arch"`
	Hostname       string `json:"hostname"`
	CPUCount       int    `json:"cpu_count"`
	CPUModel       string `json:"cpu_model"`
	TotalRAMBytes  uint64 `json:"total_ram_bytes"`
	TotalDiskBytes uint64 `json:"total_disk_bytes"`
	FreeDiskBytes  uint64 `json:"free_disk_bytes"`
}

// DockerInfo represents Docker engine availability and status.
type DockerInfo struct {
	Installed     bool   `json:"installed"`
	Version       string `json:"version,omitempty"`
	DaemonRunning bool   `json:"daemon_running"`
	Message       string `json:"message"`
}

// Collect detects and returns the current system specifications.
func Collect() (*SystemInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	info := &SystemInfo{
		OS:       detectOS(),
		Kernel:   detectKernel(),
		Arch:     runtime.GOARCH,
		Hostname: hostname,
		CPUCount: runtime.NumCPU(),
		CPUModel: detectCPUModel(),
	}

	info.TotalRAMBytes = detectTotalRAM()

	totalDisk, freeDisk, err := getDiskSpace("/")
	if err == nil {
		info.TotalDiskBytes = totalDisk
		info.FreeDiskBytes = freeDisk
	}

	return info, nil
}

// CheckDocker checks if Docker CLI and daemon are available.
func CheckDocker() *DockerInfo {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return &DockerInfo{
			Installed:     false,
			DaemonRunning: false,
			Message:       "Docker is not installed. To install on Ubuntu: sudo apt-get update && sudo apt-get install -y docker.io",
		}
	}

	// Read docker version
	verOut, err := exec.Command(dockerPath, "--version").Output()
	versionStr := ""
	if err == nil {
		versionStr = strings.TrimSpace(string(verOut))
	}

	// Check if daemon socket is accessible or docker info succeeds
	daemonRunning := false
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		daemonRunning = true
	} else {
		// Try running docker info with short timeout
		if err := exec.Command(dockerPath, "info").Run(); err == nil {
			daemonRunning = true
		}
	}

	msg := "Docker is installed and running."
	if !daemonRunning {
		msg = "Docker CLI is installed, but daemon is not running or current user lacks access to /var/run/docker.sock."
	}

	return &DockerInfo{
		Installed:     true,
		Version:       versionStr,
		DaemonRunning: daemonRunning,
		Message:       msg,
	}
}

func detectOS() string {
	if runtime.GOOS == "linux" {
		if prettyName := parseOSRelease("/etc/os-release"); prettyName != "" {
			return prettyName
		}
		return "Linux"
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err == nil {
			return fmt.Sprintf("macOS %s", strings.TrimSpace(string(out)))
		}
		return "macOS"
	}
	return runtime.GOOS
}

func parseOSRelease(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var prettyName string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			prettyName = strings.Trim(val, "\"")
			break
		}
	}
	if err := scanner.Err(); err != nil && prettyName == "" {
		return ""
	}
	return prettyName
}

func detectKernel() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/sys/kernel/osrelease")
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	out, err := exec.Command("uname", "-r").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "unknown"
}

func detectCPUModel() string {
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer file.Close()
			var cpuModel string
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						cpuModel = strings.TrimSpace(parts[1])
						break
					}
				}
			}
			if err := scanner.Err(); err == nil && cpuModel != "" {
				return cpuModel
			}
		}
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil && len(out) > 0 {
			return strings.TrimSpace(string(out))
		}
	}
	return fmt.Sprintf("%s (%d cores)", runtime.GOARCH, runtime.NumCPU())
}

func detectTotalRAM() uint64 {
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/meminfo")
		if err == nil {
			defer file.Close()
			var ram uint64
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						kb, err := strconv.ParseUint(fields[1], 10, 64)
						if err == nil {
							ram = kb * 1024
							break
						}
					}
				}
			}
			if err := scanner.Err(); err == nil && ram > 0 {
				return ram
			}
		}
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			val, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
			if err == nil {
				return val
			}
		}
	}
	return 0
}

// FormatBytes formats byte counts into human-readable strings (e.g. 16.0 GB).
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

