package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/store"
)

// RunLogs executes `nugget logs <app> [--follow] [--tail <n>]`.
func RunLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	followFlag := fs.Bool("follow", false, "Follow log output in real time")
	fs.BoolVar(followFlag, "f", false, "Follow log output (shorthand)")
	tailFlag := fs.Int("tail", 100, "Number of lines to show from the end of the logs")
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory")
	dataDirFlag := fs.String("data-dir", "", "Custom data directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Println("Usage: nugget logs <app-name> [--follow] [--tail <lines>]")
		return errors.New("application name is required")
	}
	appName := remaining[0]

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}
	dataDir := *dataDirFlag
	if dataDir == "" {
		dataDir = config.DefaultDataDir(configDir)
	}

	st := store.NewStore(dataDir)
	app, err := st.GetApplication(appName)
	if err != nil {
		return fmt.Errorf("application '%s' not found: %w", appName, err)
	}

	if app.ActiveContainerID == "" {
		return fmt.Errorf("application '%s' has no active container", app.Name)
	}

	dClient, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker engine unavailable: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C during follow mode cleanly
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	return dClient.Logs(ctx, app.ActiveContainerID, *followFlag, *tailFlag, os.Stdout)
}

