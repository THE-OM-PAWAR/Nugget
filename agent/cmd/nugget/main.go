package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/nugget-sh/nugget/agent/internal/cli"
)

var (
	Version   = "0.1.0"
	GitCommit = "dev"
	BuildDate = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cli.RunInit(args)
	case "status":
		err = cli.RunStatus(args)
	case "deploy":
		err = cli.RunDeploy(args)
	case "apps":
		err = cli.RunApps(args)
	case "logs":
		err = cli.RunLogs(args)
	case "restart":
		err = cli.RunRestart(args)
	case "stop":
		err = cli.RunStop(args)
	case "remove":
		err = cli.RunRemove(args)
	case "redeploy":
		err = cli.RunRedeploy(args)
	case "agent":
		err = cli.RunAgent(args)
	case "doctor":
		err = cli.RunDoctor(args)
	case "update":
		err = cli.RunUpdate(args)
	case "uninstall":
		err = cli.RunUninstall(args)
	case "version", "-v", "--version":
		printVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Nugget - Self-hosted application deployment & server management platform

Usage:
  nugget <command> [arguments]

Application Management Commands:
  deploy        Deploy an application from a public GitHub repository directly to Docker
  apps          List all deployed applications, health statuses, ports, and public URLs
  logs          Stream or view container logs for an application
  restart       Restart the running container for an application
  stop          Stop an application container without deleting its configuration
  remove        Stop and completely remove an application container and proxy routing
  redeploy      Pull latest code and redeploy an application with zero-downtime safety

Server & Agent Commands:
  init          Initialize Nugget on this machine (detects specs, generates ID, sets up service)
  status        Check machine specifications or inspect a specific application (nugget status <app>)
  agent         Manage the background agent daemon (start, stop, daemon)

Operations & Maintenance Commands:
  doctor        Run diagnostic checks on host privileges, config, Docker, proxy, and resources
  update        Safely upgrade the Nugget agent binary while preserving all configurations and data
  uninstall     Stop service, remove binary, and optionally purge data (--purge)
  version       Display Nugget version and build information
  help          Display help for Nugget commands

Use "nugget <command> --help" for more information about a command.`)
}

func printVersion() {
	fmt.Printf("nugget version %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  commit:    %s\n", GitCommit)
	fmt.Printf("  built:     %s\n", BuildDate)
	fmt.Printf("  compiler:  %s\n", runtime.Version())
}
