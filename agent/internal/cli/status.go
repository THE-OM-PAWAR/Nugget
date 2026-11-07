package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/daemon"
	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/health"
	"github.com/nugget-sh/nugget/agent/internal/monitor"
	"github.com/nugget-sh/nugget/agent/internal/service"
	"github.com/nugget-sh/nugget/agent/internal/store"
	"github.com/nugget-sh/nugget/agent/internal/sysinfo"
)

// AppDetailStatus encapsulates detailed status for a single application.
type AppDetailStatus struct {
	App         *store.Application  `json:"application"`
	Health      *health.AppHealth   `json:"health"`
	Deployments []*store.Deployment `json:"recent_deployments,omitempty"`
}

// StatusOutput represents structured status information for JSON output.
type StatusOutput struct {
	Initialized  bool                    `json:"initialized"`
	ConfigDir    string                  `json:"config_dir"`
	Config       *config.Config          `json:"config,omitempty"`
	Docker       *sysinfo.DockerInfo     `json:"docker"`
	Service      *service.ServiceStatus  `json:"service"`
	Metrics      *monitor.MachineMetrics `json:"metrics,omitempty"`
	DaemonActive bool                    `json:"daemon_active"`
	DaemonPID    int                     `json:"daemon_pid,omitempty"`
	Applications []*store.Application    `json:"applications,omitempty"`
}

// RunStatus executes the `nugget status` command.
func RunStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory (default: /etc/nugget)")
	jsonFlag := fs.Bool("json", false, "Output status in JSON format")
	machineFlag := fs.Bool("machine", false, "Show detailed machine resource monitoring metrics")
	appsFlag := fs.Bool("apps", false, "Show detailed application resource metrics")

	if err := fs.Parse(args); err != nil {
		return err
	}

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}

	remaining := fs.Args()
	if len(remaining) > 0 {
		return runApplicationStatus(remaining[0], configDir, *jsonFlag)
	}

	dataDir := config.DefaultDataDir(configDir)

	if *machineFlag {
		return runMachineStatus(jsonFlag)
	}

	if *appsFlag {
		return runAppsMonitoringStatus(dataDir, jsonFlag)
	}

	exists := config.Exists(configDir)
	var cfg *config.Config
	if exists {
		var err error
		cfg, err = config.Load(configDir)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}
	}

	dockerInfo := sysinfo.CheckDocker()
	st := store.NewStore(dataDir)
	apps, _ := st.ListApplications()

	svcMgr := service.NewManager()
	svcStatus, _ := svcMgr.Status()

	daemonActive, daemonPID := daemon.IsDaemonRunning(dataDir)
	mMetrics, _ := monitor.CollectMachineMetrics()

	if *jsonFlag {
		out := StatusOutput{
			Initialized:  exists,
			ConfigDir:    configDir,
			Config:       cfg,
			Docker:       dockerInfo,
			Service:      svcStatus,
			Metrics:      mMetrics,
			DaemonActive: daemonActive,
			DaemonPID:    daemonPID,
			Applications: apps,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Plain text format
	fmt.Println("=== Nugget Agent Status ===")
	if !exists {
		fmt.Printf("State:       Not Initialized\n")
		fmt.Printf("Config Dir:  %s\n\n", configDir)
		fmt.Println("Run 'nugget init' to initialize this machine.")
		return nil
	}

	fmt.Printf("Machine Name: %s\n", cfg.MachineName)
	fmt.Printf("Machine ID:   %s\n", cfg.MachineID)
	fmt.Printf("Initialized:  %s\n", cfg.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("Config File:  %s\n\n", config.ConfigFilePath(configDir))

	fmt.Println("[System Specifications & Utilization]")
	if cfg.System != nil {
		fmt.Printf("  OS:        %s (%s)\n", cfg.System.OS, cfg.System.Arch)
		fmt.Printf("  Hostname:  %s\n", cfg.System.Hostname)
		fmt.Printf("  CPU:       %d cores (%s)\n", cfg.System.CPUCount, cfg.System.CPUModel)
	}
	if mMetrics != nil {
		if mMetrics.UptimeFormatted != "" {
			fmt.Printf("  Uptime:    %s\n", mMetrics.UptimeFormatted)
		}
		fmt.Printf("  CPU Load:  %.1f%% (1m: %.2f, 5m: %.2f, 15m: %.2f)\n", mMetrics.CPUPercent, mMetrics.Load1, mMetrics.Load5, mMetrics.Load15)
		if mMetrics.TotalRAMBytes > 0 {
			fmt.Printf("  RAM:       %s / %s (%.1f%%)\n", monitor.FormatBytes(mMetrics.UsedRAMBytes), monitor.FormatBytes(mMetrics.TotalRAMBytes), mMetrics.RAMUsagePct)
		}
		if mMetrics.TotalDiskBytes > 0 {
			fmt.Printf("  Disk:      %s / %s (%.1f%%)\n", monitor.FormatBytes(mMetrics.UsedDiskBytes), monitor.FormatBytes(mMetrics.TotalDiskBytes), mMetrics.DiskUsagePct)
		}
	}

	fmt.Println("\n[Daemon & Background Service]")
	if daemonActive {
		fmt.Printf("  Daemon:    Running (PID: %d)\n", daemonPID)
	} else {
		fmt.Println("  Daemon:    Stopped (Run 'sudo nugget agent start' to start)")
	}

	if svcStatus.Installed {
		stateDesc := svcStatus.State
		if svcStatus.Active {
			stateDesc = "active (running)"
		}
		fmt.Printf("  Systemd:   %s (Auto-boot: %t)\n", stateDesc, svcStatus.Enabled)
	} else {
		fmt.Println("  Systemd:   Not installed")
	}

	fmt.Println("\n[Docker Status]")
	if dockerInfo.Installed {
		fmt.Printf("  Status:    Installed (%s)\n", dockerInfo.Version)
		if dockerInfo.DaemonRunning {
			fmt.Println("  Daemon:    Running and accessible")
		} else {
			fmt.Println("  Daemon:    Not accessible or stopped")
		}
	} else {
		fmt.Println("  Status:    Not Installed")
		fmt.Printf("  Advice:    %s\n", dockerInfo.Message)
	}

	fmt.Println("\n[Deployed Applications]")
	if len(apps) == 0 {
		fmt.Println("  No applications currently deployed.")
		fmt.Println("  Hint: Deploy an app with 'nugget deploy <github-repository-url>'")
	} else {
		for _, app := range apps {
			fmt.Printf("  • %-16s Status: %-10s Port: %-5d URL: %s\n",
				app.Name,
				app.Status,
				app.Port,
				app.PublicURL,
			)
		}
	}

	return nil
}

func runMachineStatus(jsonFlag *bool) error {
	m, err := monitor.CollectMachineMetrics()
	if err != nil {
		return fmt.Errorf("failed to collect machine metrics: %w", err)
	}

	if jsonFlag != nil && *jsonFlag {
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("=== Nugget Host Machine Metrics ===")
	if m.UptimeFormatted != "" {
		fmt.Printf("Uptime:        %s\n", m.UptimeFormatted)
	}
	fmt.Printf("CPU Usage:     %.1f%%\n", m.CPUPercent)
	fmt.Printf("Load Average:  %.2f (1m), %.2f (5m), %.2f (15m)\n\n", m.Load1, m.Load5, m.Load15)

	fmt.Println("[Memory]")
	fmt.Printf("  Total:       %s\n", monitor.FormatBytes(m.TotalRAMBytes))
	fmt.Printf("  Used:        %s (%.1f%%)\n", monitor.FormatBytes(m.UsedRAMBytes), m.RAMUsagePct)
	fmt.Printf("  Free:        %s\n\n", monitor.FormatBytes(m.FreeRAMBytes))

	fmt.Println("[Disk (/)]")
	fmt.Printf("  Total:       %s\n", monitor.FormatBytes(m.TotalDiskBytes))
	fmt.Printf("  Used:        %s (%.1f%%)\n", monitor.FormatBytes(m.UsedDiskBytes), m.DiskUsagePct)
	fmt.Printf("  Free:        %s\n\n", monitor.FormatBytes(m.FreeDiskBytes))

	if m.NetworkRxBytes > 0 || m.NetworkTxBytes > 0 {
		fmt.Println("[Network (Total)]")
		fmt.Printf("  Received:    %s\n", monitor.FormatBytes(m.NetworkRxBytes))
		fmt.Printf("  Transmitted: %s\n", monitor.FormatBytes(m.NetworkTxBytes))
	}

	return nil
}

func runAppsMonitoringStatus(dataDir string, jsonFlag *bool) error {
	st := store.NewStore(dataDir)
	apps, err := st.ListApplications()
	if err != nil {
		return err
	}

	dClient, _ := docker.NewClient()
	appMetrics := monitor.CollectAllApps(context.Background(), dClient, apps)

	if jsonFlag != nil && *jsonFlag {
		data, err := json.MarshalIndent(appMetrics, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(appMetrics) == 0 {
		fmt.Println("No applications deployed yet.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tCPU%\tMEM%\tRESTARTS\tINTERNAL PORT\tPUBLIC URL")
	for _, m := range appMetrics {
		fmt.Fprintf(w, "%s\t%s\t%.1f%%\t%.1f%%\t%d\thttp://localhost:%d\t%s\n",
			m.AppName,
			m.Status,
			m.CPUPercent,
			m.MemoryPercent,
			m.RestartCount,
			m.InternalPort,
			m.PublicURL,
		)
	}
	return w.Flush()
}

func runApplicationStatus(appName, configDir string, jsonOut bool) error {
	dataDir := config.DefaultDataDir(configDir)
	st := store.NewStore(dataDir)

	app, err := st.GetApplication(appName)
	if err != nil {
		return fmt.Errorf("application '%s' not found: %w", appName, err)
	}

	dClient, _ := docker.NewClient()
	ctx := context.Background()
	appHealth := health.CheckAppHealth(ctx, dClient, app.ActiveContainerID, app.Port)
	deps, _ := st.ListDeployments(app.ID)

	if jsonOut {
		detail := AppDetailStatus{
			App:         app,
			Health:      appHealth,
			Deployments: deps,
		}
		data, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("=== Application: %s (ID: %s) ===\n", app.Name, app.ID)
	fmt.Printf("Status:        %s\n", appHealth.Status)
	fmt.Printf("Internal Port: http://localhost:%d\n", app.Port)
	if app.PublicURL != "" {
		fmt.Printf("Public URL:    %s\n", app.PublicURL)
	}
	fmt.Printf("Repository:    %s\n", app.RepoURL)
	if app.Branch != "" {
		fmt.Printf("Branch:        %s\n", app.Branch)
	}
	if app.ActiveContainerID != "" {
		fmt.Printf("Container ID:  %s\n", app.ActiveContainerID)
	}
	if app.ActiveDeploymentID != "" {
		fmt.Printf("Active Deploy: %s\n", app.ActiveDeploymentID)
	}

	fmt.Println("\n[Health Checks]")
	fmt.Printf("  Container:   %s\n", appHealth.ContainerStatus)
	fmt.Printf("  Responsive:  %t\n", appHealth.Responsive)
	fmt.Printf("  Restarts:    %d\n", appHealth.RestartCount)
	fmt.Printf("  Detail:      %s\n", appHealth.Message)

	if len(deps) > 0 {
		fmt.Println("\n[Recent Deployments]")
		for i, d := range deps {
			if i >= 5 {
				break
			}
			errStr := ""
			if d.Error != "" {
				errStr = fmt.Sprintf(" (Error: %s)", d.Error)
			}
			fmt.Printf("  • %s  %-9s  %s%s\n",
				d.ID,
				d.Status,
				d.StartedAt.Format("2006-01-02 15:04:05 UTC"),
				errStr,
			)
		}
	}

	return nil
}
