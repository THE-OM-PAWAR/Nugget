package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/service"
)

// RunUpdate upgrades the Nugget Agent binary safely while preserving configuration and data.
func RunUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	binaryPath := fs.String("binary", "", "Path to custom binary to install")
	targetPath := fs.String("target", "/usr/local/bin/nugget", "Target installation path")
	force := fs.Bool("force", false, "Force update without version check")

	if err := fs.Parse(args); err != nil {
		return err
	}

	currentExec, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine current executable path: %w", err)
	}
	currentExec, _ = filepath.EvalSymlinks(currentExec)

	// If targetPath doesn't exist, use current executable if it's in /usr/local/bin
	installTarget := *targetPath
	if _, err := os.Stat(installTarget); os.IsNotExist(err) && strings.Contains(currentExec, "nugget") {
		installTarget = currentExec
	}

	sourceBinary := *binaryPath

	// If no binary specified, look for scripts/install.sh in repo or check local build
	if sourceBinary == "" {
		repoInstallScript := "./scripts/install.sh"
		if _, err := os.Stat(repoInstallScript); err == nil {
			fmt.Printf("==> Running local installation script: %s\n", repoInstallScript)
			cmd := exec.Command("bash", repoInstallScript)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("installer execution failed: %w", err)
			}
			return nil
		}

		return fmt.Errorf("no update source specified. Provide a binary with --binary <path> or run scripts/install.sh")
	}

	// Verify source binary
	srcInfo, err := os.Stat(sourceBinary)
	if err != nil {
		return fmt.Errorf("invalid source binary: %w", err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source binary %s is a directory", sourceBinary)
	}

	fmt.Printf("==> Updating Nugget Agent...\n")
	fmt.Printf("    Source: %s\n", sourceBinary)
	fmt.Printf("    Target: %s\n", installTarget)

	// Check if source and target are identical
	if !*force && filepath.Clean(sourceBinary) == filepath.Clean(installTarget) {
		fmt.Println("Target is already identical to source. Use --force to reinstall.")
		return nil
	}

	// Atomically copy binary into place
	tmpTarget := installTarget + ".tmp"
	if err := copyExecutable(sourceBinary, tmpTarget); err != nil {
		return fmt.Errorf("failed to copy binary to temporary target: %w", err)
	}

	if err := os.Rename(tmpTarget, installTarget); err != nil {
		_ = os.Remove(tmpTarget)
		return fmt.Errorf("failed to replace binary at %s: %w (try running with sudo)", installTarget, err)
	}

	fmt.Printf("==> Binary updated successfully at %s\n", installTarget)

	// Restart systemd service if running
	mgr := service.NewManager()
	svcStatus, err := mgr.Status()
	if err == nil && svcStatus.Active {
		fmt.Printf("==> Restarting nugget background service...\n")
		if restartErr := mgr.Restart(); restartErr != nil {
			fmt.Printf("    Warning: failed to restart background service: %v\n", restartErr)
			fmt.Println("    Please restart manually with: sudo systemctl restart nugget")
		} else {
			fmt.Printf("    Background service restarted successfully.\n")
		}
	}

	// Verify that existing configuration and applications are preserved
	configDir := config.DefaultConfigDir()
	dataDir := config.DefaultDataDir(configDir)
	if _, err := os.Stat(filepath.Join(configDir, "config.json")); err == nil {
		fmt.Println("==> Configuration preserved: " + configDir)
	}
	if _, err := os.Stat(dataDir); err == nil {
		fmt.Println("==> Application data preserved: " + dataDir)
	}

	fmt.Println("\n✓ Nugget update complete!")
	return nil
}

// RunUninstall safely uninstalls the Nugget Agent.
func RunUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	purge := fs.Bool("purge", false, "Purge all configuration, applications, and deployment data")
	yes := fs.Bool("yes", false, "Proceed without interactive confirmation")
	yShort := fs.Bool("y", false, "Proceed without interactive confirmation (alias)")
	targetPath := fs.String("target", "/usr/local/bin/nugget", "Path to Nugget executable")

	if err := fs.Parse(args); err != nil {
		return err
	}

	nonInteractive := *yes || *yShort

	if !nonInteractive {
		fmt.Println("================================================================================")
		fmt.Println("                            Nugget Agent Uninstallation                         ")
		fmt.Println("================================================================================")
		fmt.Println("This operation will:")
		fmt.Println("  1. Stop and remove the Nugget systemd service (if installed).")
		fmt.Printf("  2. Delete the binary at %s.\n", *targetPath)

		configDir := config.DefaultConfigDir()
		dataDir := config.DefaultDataDir(configDir)

		if *purge {
			fmt.Printf("  3. \033[31mPURGE all configuration (%s) and data (%s).\033[0m\n", configDir, dataDir)
		} else {
			fmt.Printf("  3. \033[32mPRESERVE configuration (%s) and data (%s).\033[0m\n", configDir, dataDir)
		}

		fmt.Print("\nAre you sure you want to proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Uninstallation aborted.")
			return nil
		}
	}

	fmt.Println("\n==> Stopping and removing background service...")
	mgr := service.NewManager()
	if err := mgr.Uninstall(); err != nil {
		fmt.Printf("    Notice: service cleanup returned: %v\n", err)
	} else {
		fmt.Println("    Service stopped and unit file removed.")
	}

	// Remove binary
	fmt.Printf("==> Removing binary at %s...\n", *targetPath)
	if err := os.Remove(*targetPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("    Warning: could not delete %s: %v (try running with sudo)\n", *targetPath, err)
	} else {
		fmt.Println("    Binary deleted.")
	}

	configDir := config.DefaultConfigDir()
	dataDir := config.DefaultDataDir(configDir)

	if *purge {
		fmt.Println("==> Purging configuration and application data...")
		if err := os.RemoveAll(configDir); err != nil {
			fmt.Printf("    Warning: could not remove %s: %v\n", configDir, err)
		} else {
			fmt.Printf("    Removed %s\n", configDir)
		}
		if err := os.RemoveAll(dataDir); err != nil {
			fmt.Printf("    Warning: could not remove %s: %v\n", dataDir, err)
		} else {
			fmt.Printf("    Removed %s\n", dataDir)
		}
		fmt.Println("\n✓ Nugget uninstalled and all data purged.")
	} else {
		fmt.Println("\n==> Data Preservation Notice:")
		fmt.Printf("    Configuration directory retained at: %s\n", configDir)
		fmt.Printf("    Application data retained at:       %s\n", dataDir)
		fmt.Println("    To delete later, run: sudo rm -rf " + configDir + " " + dataDir)
		fmt.Println("\n✓ Nugget uninstalled successfully.")
	}

	return nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

