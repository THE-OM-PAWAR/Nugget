package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/deployer"
)

// RunDeploy executes the `nugget deploy <repo-url>` command.
func RunDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	nameFlag := fs.String("name", "", "Application name (default: repository name)")
	branchFlag := fs.String("branch", "", "Git branch to clone (default: default branch)")
	portFlag := fs.Int("port", 0, "Host port for application (default: auto-assigned)")
	envFileFlag := fs.String("env-file", "", "Path to environment variable file")
	dockerfileFlag := fs.String("dockerfile", "", "Custom Dockerfile path inside repository")
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory")
	dataDirFlag := fs.String("data-dir", "", "Custom data directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remainingArgs := fs.Args()
	if len(remainingArgs) == 0 {
		printDeployUsage()
		return errors.New("repository URL is required")
	}

	repoURL := remainingArgs[0]

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}

	dataDir := *dataDirFlag
	if dataDir == "" {
		dataDir = config.DefaultDataDir(configDir)
	}

	dep, err := deployer.NewDeployer(dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize deployment engine: %w", err)
	}

	opts := deployer.DeployOptions{
		RepoURL:    repoURL,
		AppName:    *nameFlag,
		Branch:     *branchFlag,
		Port:       *portFlag,
		EnvFile:    *envFileFlag,
		Dockerfile: *dockerfileFlag,
		DataDir:    dataDir,
		LogOutput:  os.Stdout,
	}

	_, err = dep.Deploy(context.Background(), opts)
	return err
}

func printDeployUsage() {
	fmt.Println(`Usage: nugget deploy <github-repository-url> [flags]

Deploy an application directly from a public GitHub repository.

Flags:
  --name string        Application name (defaults to repository name)
  --branch string      Git branch (defaults to repository default branch)
  --port int           Host port for the application (auto-assigned if omitted)
  --env-file string    Path to environment variables file (.env)
  --dockerfile string  Custom Dockerfile path inside the repository
  --config-dir string  Custom configuration directory
  --data-dir string    Custom data and deployment storage directory`)
}

