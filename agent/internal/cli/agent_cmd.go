package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/daemon"
	"github.com/nugget-sh/nugget/agent/internal/monitor"
	"github.com/nugget-sh/nugget/agent/internal/service"
)

// RunAgent handles the `nugget agent <subcommand>` routing.
func RunAgent(args []string) error {
	if len(args) == 0 {
		printAgentHelp()
		return nil
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "start":
		return runAgentStart(subArgs)
	case "stop":
		return runAgentStop(subArgs)
	case "status":
		return runAgentStatus(subArgs)
	case "daemon", "run":
		return runAgentDaemon(subArgs)
	case "help", "--help", "-h":
		printAgentHelp()
		return nil
	default:
		return fmt.Errorf("unknown agent command '%s'. Run 'nugget agent --help'", subcmd)
	}
}

func printAgentHelp() {
	fmt.Println(`Usage: nugget agent <command>

Manage the background Nugget agent service or run the daemon process.

Commands:
  start     Start the background systemd service (requires sudo)
  stop      Stop the background systemd service (requires sudo)
  status    Check whether the agent daemon process and service are running
  daemon    Run the persistent agent daemon in the foreground (used by systemd)`)
}

func runAgentStart(args []string) error {
	mgr := service.NewManager()
	if !service.IsSystemdAvailable() {
		fmt.Println("Warning: systemd is not available on this system.")
		fmt.Println("To run the agent in the foreground, use: nugget agent daemon")
		return nil
	}

	fmt.Println("Starting Nugget background service...")
	if err := mgr.Start(); err != nil {
		return err
	}
	fmt.Println("Service started successfully. Run 'nugget status' to verify.")
	return nil
}

func runAgentStop(args []string) error {
	mgr := service.NewManager()
	if !service.IsSystemdAvailable() {
		fmt.Println("Warning: systemd is not available on this system.")
		return nil
	}

	fmt.Println("Stopping Nugget background service...")
	if err := mgr.Stop(); err != nil {
		return err
	}
	fmt.Println("Service stopped successfully.")
	return nil
}

func runAgentStatus(args []string) error {
	fs := flag.NewFlagSet("agent status", flag.ContinueOnError)
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}
	dataDir := config.DefaultDataDir(configDir)

	running, pid := daemon.IsDaemonRunning(dataDir)
	mgr := service.NewManager()
	svcStatus, _ := mgr.Status()

	fmt.Println("=== Nugget Agent Daemon Status ===")
	if running {
		fmt.Printf("Daemon Process: Active (PID: %d)\n", pid)
	} else {
		fmt.Println("Daemon Process: Inactive / Stopped")
	}

	stateDesc := svcStatus.State
	if svcStatus.Active {
		stateDesc = "active (running)"
	}
	fmt.Printf("Systemd Service:%s\n", stateDesc)
	fmt.Printf("Data Directory: %s\n", dataDir)
	fmt.Printf("Log File:       %s/logs/nugget.log\n", dataDir)

	metricsFile := filepath.Join(dataDir, "machine_metrics.json")
	if data, err := os.ReadFile(metricsFile); err == nil {
		var m monitor.MachineMetrics
		if json.Unmarshal(data, &m) == nil {
			fmt.Println("\n[Last Collected Metrics]")
			fmt.Printf("  CPU Usage:    %.1f%%\n", m.CPUPercent)
			fmt.Printf("  Load Average: %.2f, %.2f, %.2f\n", m.Load1, m.Load5, m.Load15)
			fmt.Printf("  RAM Usage:    %.1f%% (%s / %s)\n", m.RAMUsagePct, monitor.FormatBytes(m.UsedRAMBytes), monitor.FormatBytes(m.TotalRAMBytes))
			fmt.Printf("  Disk Usage:   %.1f%% (%s / %s)\n", m.DiskUsagePct, monitor.FormatBytes(m.UsedDiskBytes), monitor.FormatBytes(m.TotalDiskBytes))
			if m.UptimeFormatted != "" {
				fmt.Printf("  Uptime:       %s\n", m.UptimeFormatted)
			}
		}
	}

	return nil
}

func runAgentDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory")
	intervalFlag := fs.Duration("interval", 15*time.Second, "Monitoring collection interval")
	if err := fs.Parse(args); err != nil {
		return err
	}

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}
	dataDir := config.DefaultDataDir(configDir)

	d, err := daemon.NewDaemon(configDir, dataDir, *intervalFlag)
	if err != nil {
		return fmt.Errorf("failed to initialize daemon: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	return d.Run(ctx)
}
