package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/service"
	"github.com/nugget-sh/nugget/agent/internal/sysinfo"
)

// InitOptions contains flags and parameters for `nugget init`.
type InitOptions struct {
	MachineName    string
	ConfigDir      string
	Force          bool
	NonInteractive bool
}

// RunInit executes the `nugget init` command.
func RunInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	nameFlag := fs.String("name", "", "Human-readable name for this machine")
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory (default: /etc/nugget)")
	forceFlag := fs.Bool("force", false, "Force re-initialization even if machine identity exists")
	nonInteractiveFlag := fs.Bool("non-interactive", false, "Do not prompt for interactive input")

	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := InitOptions{
		MachineName:    *nameFlag,
		ConfigDir:      *configDirFlag,
		Force:          *forceFlag,
		NonInteractive: *nonInteractiveFlag,
	}

	return ExecuteInit(opts, os.Stdin)
}

// ExecuteInit carries out initialization with provided input reader (testable).
func ExecuteInit(opts InitOptions, stdin *os.File) error {
	configDir := opts.ConfigDir
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}

	fmt.Println("==> Initializing Nugget Agent")
	fmt.Printf("    Configuration directory: %s\n\n", configDir)

	// Check existing configuration
	if config.Exists(configDir) {
		existingCfg, err := config.Load(configDir)
		if err == nil {
			fmt.Printf("Notice: An existing Nugget configuration was detected.\n")
			fmt.Printf("        Machine ID:   %s\n", existingCfg.MachineID)
			fmt.Printf("        Machine Name: %s\n\n", existingCfg.MachineName)
		}

		if !opts.Force {
			if opts.NonInteractive || stdin == nil {
				return fmt.Errorf("machine is already initialized. Use --force to re-initialize")
			}

			fmt.Print("Do you want to overwrite existing machine identity? (y/N): ")
			reader := bufio.NewReader(stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Initialization aborted.")
				return nil
			}
		}
	}

	// 1. Detect System Information
	fmt.Println("[1/4] Detecting host system specifications...")
	sys, err := sysinfo.Collect()
	if err != nil {
		return fmt.Errorf("system detection failed: %w", err)
	}
	fmt.Printf("      OS:        %s (%s)\n", sys.OS, sys.Arch)
	fmt.Printf("      Hostname:  %s\n", sys.Hostname)
	fmt.Printf("      CPU:       %d cores (%s)\n", sys.CPUCount, sys.CPUModel)
	if sys.TotalRAMBytes > 0 {
		fmt.Printf("      RAM:       %s\n", sysinfo.FormatBytes(sys.TotalRAMBytes))
	}
	if sys.TotalDiskBytes > 0 {
		fmt.Printf("      Disk:      %s total, %s free\n", sysinfo.FormatBytes(sys.TotalDiskBytes), sysinfo.FormatBytes(sys.FreeDiskBytes))
	}

	// 2. Check Docker Availability
	fmt.Println("\n[2/4] Checking Docker dependencies...")
	docker := sysinfo.CheckDocker()
	if docker.Installed {
		fmt.Printf("      Docker:    %s\n", docker.Version)
		if docker.DaemonRunning {
			fmt.Println("      Daemon:    Running and accessible")
		} else {
			fmt.Println("      Warning:   Docker daemon is not running or accessible")
		}
	} else {
		fmt.Printf("      Warning:   Docker is not installed.\n")
		fmt.Printf("                 %s\n", docker.Message)
	}

	// 3. Resolve Machine Name & Generate Machine ID
	machineName := strings.TrimSpace(opts.MachineName)
	if machineName == "" {
		defaultName := sys.Hostname
		if defaultName == "" || defaultName == "unknown" {
			defaultName = "nugget-server"
		}

		if opts.NonInteractive || stdin == nil {
			machineName = defaultName
		} else {
			fmt.Printf("\nEnter machine name [%s]: ", defaultName)
			reader := bufio.NewReader(stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input != "" {
				machineName = input
			} else {
				machineName = defaultName
			}
		}
	}

	fmt.Println("\n[3/4] Generating unique machine identity...")
	machineID, err := config.GenerateMachineID()
	if err != nil {
		return fmt.Errorf("failed to generate machine ID: %w", err)
	}
	fmt.Printf("      Machine Name: %s\n", machineName)
	fmt.Printf("      Machine ID:   %s\n", machineID)

	cfg := &config.Config{
		MachineID:   machineID,
		MachineName: machineName,
		System:      sys,
		Port:        "8443",
		LogLevel:    "info",
	}

	if err := config.Save(configDir, cfg, true); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}
	fmt.Printf("      Config saved: %s\n", config.ConfigFilePath(configDir))

	// 4. Install Background Service (systemd)
	fmt.Println("\n[4/4] Configuring background service...")
	binaryPath, err := os.Executable()
	if err != nil {
		binaryPath = "/usr/local/bin/nugget"
	}

	svcMgr := service.NewManager()
	if service.IsSystemdAvailable() {
		if os.Geteuid() == 0 {
			if err := svcMgr.Install(binaryPath, configDir); err != nil {
				fmt.Printf("      Warning: Could not install systemd service: %v\n", err)
			} else {
				fmt.Println("      Systemd service installed and enabled at boot.")
				fmt.Println("      To start now: sudo nugget agent start")
			}
		} else {
			fmt.Println("      Systemd service installation skipped (requires root/sudo).")
			fmt.Println("      Run with sudo or later run: sudo nugget agent start")
		}
	} else {
		fmt.Println("      Systemd not detected (standalone or container environment).")
		fmt.Println("      You can run the agent directly with: nugget agent daemon")
	}

	fmt.Println("\n==> Nugget Agent initialized successfully!")
	fmt.Printf("    Run 'nugget status' to inspect current host and agent state.\n")
	return nil
}

