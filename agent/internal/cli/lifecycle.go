package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/deployer"
)

func parseLifecycleTarget(cmdName string, args []string) (string, string, error) {
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory")
	dataDirFlag := fs.String("data-dir", "", "Custom data directory")

	if err := fs.Parse(args); err != nil {
		return "", "", err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Printf("Usage: nugget %s <app-name>\n", cmdName)
		return "", "", fmt.Errorf("application name is required for 'nugget %s'", cmdName)
	}

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}

	dataDir := *dataDirFlag
	if dataDir == "" {
		dataDir = config.DefaultDataDir(configDir)
	}

	return remaining[0], dataDir, nil
}

// RunRestart restarts the running container of an application.
func RunRestart(args []string) error {
	appName, dataDir, err := parseLifecycleTarget("restart", args)
	if err != nil {
		return err
	}

	dep, err := deployer.NewDeployer(dataDir)
	if err != nil {
		return err
	}

	fmt.Printf("Restarting application '%s'...\n", appName)
	if err := dep.Restart(context.Background(), appName); err != nil {
		return fmt.Errorf("failed to restart '%s': %w", appName, err)
	}

	fmt.Printf("Application '%s' restarted successfully.\n", appName)
	return nil
}

// RunStop stops the container of an application without deleting its configuration.
func RunStop(args []string) error {
	appName, dataDir, err := parseLifecycleTarget("stop", args)
	if err != nil {
		return err
	}

	dep, err := deployer.NewDeployer(dataDir)
	if err != nil {
		return err
	}

	fmt.Printf("Stopping application '%s'...\n", appName)
	if err := dep.Stop(context.Background(), appName); err != nil {
		return fmt.Errorf("failed to stop '%s': %w", appName, err)
	}

	fmt.Printf("Application '%s' stopped successfully.\n", appName)
	return nil
}

// RunRemove removes the container, proxy routing, and metadata for an application.
func RunRemove(args []string) error {
	appName, dataDir, err := parseLifecycleTarget("remove", args)
	if err != nil {
		return err
	}

	dep, err := deployer.NewDeployer(dataDir)
	if err != nil {
		return err
	}

	fmt.Printf("Removing application '%s'...\n", appName)
	if err := dep.Remove(context.Background(), appName); err != nil {
		return fmt.Errorf("failed to remove '%s': %w", appName, err)
	}

	fmt.Printf("Application '%s' and its reverse proxy routing have been removed.\n", appName)
	return nil
}

// RunRedeploy pulls latest code, rebuilds, and updates an existing application with zero-downtime safety.
func RunRedeploy(args []string) error {
	appName, dataDir, err := parseLifecycleTarget("redeploy", args)
	if err != nil {
		return err
	}

	dep, err := deployer.NewDeployer(dataDir)
	if err != nil {
		return err
	}

	fmt.Printf("==> Triggering redeployment for '%s'...\n", appName)
	_, err = dep.Redeploy(context.Background(), appName, os.Stdout)
	return err
}
