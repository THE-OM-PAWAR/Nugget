package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/daemon"
	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/monitor"
	"github.com/nugget-sh/nugget/agent/internal/proxy"
	"github.com/nugget-sh/nugget/agent/internal/service"
	"github.com/nugget-sh/nugget/agent/internal/tunnel"
)

// CheckLevel indicates the severity of a diagnostic result.
type CheckLevel string

const (
	LevelPass CheckLevel = "PASS"
	LevelWarn CheckLevel = "WARN"
	LevelFail CheckLevel = "FAIL"
)

// CheckResult represents an individual diagnostic check.
type CheckResult struct {
	Category string     `json:"category"`
	Name     string     `json:"name"`
	Level    CheckLevel `json:"level"`
	Message  string     `json:"message"`
	Advice   string     `json:"advice,omitempty"`
}

// DoctorReport contains all diagnostic results and a final summary.
type DoctorReport struct {
	Results   []CheckResult `json:"results"`
	PassCount int           `json:"pass_count"`
	WarnCount int           `json:"warn_count"`
	FailCount int           `json:"fail_count"`
}

// RunDiagnostics executes all health and configuration checks across the host.
func RunDiagnostics(configDir, dataDir string) *DoctorReport {
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}
	if dataDir == "" {
		dataDir = config.DefaultDataDir(configDir)
	}

	report := &DoctorReport{}

	// 1. Privileges
	checkPrivileges(report)

	// 2. Configuration
	checkConfiguration(report, configDir)

	// 3. Storage & Permissions
	checkStorage(report, dataDir)

	// 4. Daemon & Process Lock
	checkDaemon(report, dataDir)

	// 5. Systemd Background Service
	checkService(report)

	// 6. Docker Engine
	checkDocker(report)

	// 7. Reverse Proxy & Networking
	checkProxy(report)

	// 8. Cloudflare Tunnel
	checkTunnel(report)

	// 9. Host Resources
	checkResources(report)

	// Tally counts
	for _, r := range report.Results {
		switch r.Level {
		case LevelPass:
			report.PassCount++
		case LevelWarn:
			report.WarnCount++
		case LevelFail:
			report.FailCount++
		}
	}

	return report
}

func (r *DoctorReport) add(category, name string, level CheckLevel, message, advice string) {
	r.Results = append(r.Results, CheckResult{
		Category: category,
		Name:     name,
		Level:    level,
		Message:  message,
		Advice:   advice,
	})
}

func checkPrivileges(r *DoctorReport) {
	if os.Geteuid() == 0 {
		r.add("Privileges", "User Permissions", LevelPass, "Running with root/sudo privileges.", "")
	} else {
		r.add("Privileges", "User Permissions", LevelWarn, "Running as standard non-root user.", "Some operations (systemd service management, binding port 80) require root/sudo.")
	}
}

func checkConfiguration(r *DoctorReport, configDir string) {
	if !config.Exists(configDir) {
		r.add("Configuration", "Initialization", LevelWarn,
			fmt.Sprintf("No configuration file found in %s.", configDir),
			"Run 'sudo nugget init' to initialize this machine.")
		return
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		r.add("Configuration", "Config File", LevelFail,
			fmt.Sprintf("Failed to parse config file: %v", err),
			"Check config file syntax or re-initialize with 'sudo nugget init --force'.")
		return
	}

	// Verify permissions (should be 0600)
	configPath := config.ConfigFilePath(configDir)
	if fi, err := os.Stat(configPath); err == nil {
		perm := fi.Mode().Perm()
		if perm > 0644 {
			r.add("Configuration", "File Permissions", LevelWarn,
				fmt.Sprintf("Config file has loose permissions (%o).", perm),
				fmt.Sprintf("Fix with: chmod 600 %s", configPath))
		} else {
			r.add("Configuration", "File Permissions", LevelPass,
				fmt.Sprintf("Config file permissions are secure (%o).", perm), "")
		}
	}

	if cfg.MachineID != "" {
		r.add("Configuration", "Machine Identity", LevelPass,
			fmt.Sprintf("Machine ID: %s (Name: %s)", cfg.MachineID, cfg.MachineName), "")
	} else {
		r.add("Configuration", "Machine Identity", LevelFail,
			"Machine ID is empty.", "Re-initialize with 'sudo nugget init --force'.")
	}
}

func checkStorage(r *DoctorReport, dataDir string) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		r.add("Storage", "Data Directory", LevelFail,
			fmt.Sprintf("Cannot access data directory %s: %v", dataDir, err),
			"Ensure the data directory exists and has write permissions.")
		return
	}

	// Test write
	testFile := filepath.Join(dataDir, ".write_test")
	if err := os.WriteFile(testFile, []byte("ok"), 0600); err != nil {
		r.add("Storage", "Write Permissions", LevelFail,
			fmt.Sprintf("Data directory %s is not writable: %v", dataDir, err),
			fmt.Sprintf("Fix permissions with: sudo chown -R $(whoami) %s", dataDir))
	} else {
		_ = os.Remove(testFile)
		r.add("Storage", "Write Permissions", LevelPass,
			fmt.Sprintf("Data directory %s is writable.", dataDir), "")
	}
}

func checkDaemon(r *DoctorReport, dataDir string) {
	running, pid := daemon.IsDaemonRunning(dataDir)
	if running {
		r.add("Daemon", "Process State", LevelPass,
			fmt.Sprintf("Background daemon is active (PID: %d).", pid), "")
	} else {
		r.add("Daemon", "Process State", LevelWarn,
			"Background daemon is not running.",
			"Start daemon with: sudo nugget agent start (or 'nugget agent daemon')")
	}
}

func checkService(r *DoctorReport) {
	if !service.IsSystemdAvailable() {
		r.add("Service", "Systemd Availability", LevelWarn,
			"systemd is not active on this host (standalone or container environment).", "")
		return
	}

	mgr := service.NewManager()
	status, _ := mgr.Status()
	if status.Installed {
		if status.Active {
			r.add("Service", "Systemd Service", LevelPass,
				"nugget.service is installed, active and running.", "")
		} else {
			r.add("Service", "Systemd Service", LevelWarn,
				"nugget.service is installed but stopped.",
				"Start with: sudo systemctl start nugget")
		}
	} else {
		r.add("Service", "Systemd Service", LevelWarn,
			"nugget.service is not installed.",
			"Install with: sudo nugget init")
	}
}

func checkDocker(r *DoctorReport) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		r.add("Docker", "CLI Installation", LevelFail,
			"Docker CLI is not installed or not in PATH.",
			"Install Docker on Ubuntu: sudo apt-get update && sudo apt-get install -y docker.io")
		return
	}
	r.add("Docker", "CLI Installation", LevelPass,
		fmt.Sprintf("Docker binary found at %s.", dockerPath), "")

	// Check socket / daemon
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		r.add("Docker", "Daemon Socket", LevelWarn,
			"/var/run/docker.sock does not exist or is inaccessible.",
			"Ensure Docker daemon is running: sudo systemctl start docker")
	} else {
		r.add("Docker", "Daemon Socket", LevelPass,
			"Docker daemon socket /var/run/docker.sock is accessible.", "")
	}

	dClient, err := docker.NewClient()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := dClient.Ping(ctx); err == nil {
			r.add("Docker", "Engine Connectivity", LevelPass,
				"Docker daemon responded to ping.", "")
		} else {
			r.add("Docker", "Engine Connectivity", LevelWarn,
				fmt.Sprintf("Docker daemon ping failed: %v", err),
				"Start Docker: sudo systemctl start docker")
		}
	}
}

func checkProxy(r *DoctorReport) {
	dClient, err := docker.NewClient()
	if err != nil {
		return
	}
	ctx := context.Background()

	// Check network
	checkNetCmd := exec.Command("docker", "network", "inspect", proxy.DefaultNetwork)
	if err := checkNetCmd.Run(); err == nil {
		r.add("Proxy", "Docker Network", LevelPass,
			fmt.Sprintf("Bridge network '%s' exists.", proxy.DefaultNetwork), "")
	} else {
		r.add("Proxy", "Docker Network", LevelWarn,
			fmt.Sprintf("Bridge network '%s' does not exist yet.", proxy.DefaultNetwork),
			"Network will be auto-created on first application deployment.")
	}

	// Check Traefik container
	state, err := dClient.InspectContainer(ctx, proxy.ProxyContainerName)
	if err == nil && state.State.Running {
		r.add("Proxy", "Traefik Container", LevelPass,
			fmt.Sprintf("Traefik reverse proxy is running (ID: %s).", state.ID[:12]), "")
	} else {
		r.add("Proxy", "Traefik Container", LevelWarn,
			"Traefik reverse proxy is not currently running.",
			"Traefik starts automatically on first deployment or via 'nugget agent daemon'.")
	}
}

func checkTunnel(r *DoctorReport) {
	status := tunnel.InspectTunnel(context.Background(), nil, "", proxy.DefaultBaseDomain)
	if status.Configured {
		if status.Running {
			r.add("Tunnel", "Cloudflare Tunnel", LevelPass,
				fmt.Sprintf("Tunnel is active and routing *.%s.", status.BaseDomain), "")
		} else {
			r.add("Tunnel", "Cloudflare Tunnel", LevelWarn,
				"Tunnel token is configured, but tunnel daemon is not running.",
				"Check cloudflared process or restart agent daemon.")
		}
	} else {
		r.add("Tunnel", "Cloudflare Tunnel", LevelWarn,
			fmt.Sprintf("Cloudflare Tunnel token not configured (*.%s).", status.BaseDomain),
			"To enable public HTTPS subdomains, export NUGGET_TUNNEL_TOKEN=<token>.")
	}
}

func checkResources(r *DoctorReport) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		freeBytes := uint64(stat.Bavail) * uint64(stat.Bsize)
		if freeBytes < 2*1024*1024*1024 { // < 2 GB
			r.add("Resources", "Free Disk Space", LevelWarn,
				fmt.Sprintf("Low free disk space: %s available.", monitor.FormatBytes(freeBytes)),
				"Free up disk space to ensure Docker builds and containers run reliably.")
		} else {
			r.add("Resources", "Free Disk Space", LevelPass,
				fmt.Sprintf("%s free disk space available.", monitor.FormatBytes(freeBytes)), "")
		}
	}

	metrics, err := monitor.CollectMachineMetrics()
	if err == nil && metrics.FreeRAMBytes > 0 {
		if metrics.FreeRAMBytes < 256*1024*1024 { // < 256 MB
			r.add("Resources", "Free Memory", LevelWarn,
				fmt.Sprintf("Low available RAM: %s free.", monitor.FormatBytes(metrics.FreeRAMBytes)),
				"Close unused processes or add swap space.")
		} else {
			r.add("Resources", "Free Memory", LevelPass,
				fmt.Sprintf("%s available RAM.", monitor.FormatBytes(metrics.FreeRAMBytes)), "")
		}
	}
}
