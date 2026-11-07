package deployer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/detector"
	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/git"
	"github.com/nugget-sh/nugget/agent/internal/proxy"
	"github.com/nugget-sh/nugget/agent/internal/store"
	"github.com/nugget-sh/nugget/agent/internal/tunnel"
)

var validAppNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// DeployOptions holds all user-supplied parameters for deploying an application.
type DeployOptions struct {
	RepoURL        string
	AppName        string
	Branch         string
	Port           int
	EnvFile        string
	Dockerfile     string
	DataDir        string
	BaseDomain     string
	NonInteractive bool
	LogOutput      io.Writer
}

// Deployer orchestrates application cloning, building, running, and local state management.
type Deployer struct {
	Store        *store.Store
	DockerClient *docker.Client
}

// NewDeployer creates a new deployment engine instance.
func NewDeployer(dataDir string) (*Deployer, error) {
	dClient, err := docker.NewClient()
	if err != nil {
		return nil, err
	}
	st := store.NewStore(dataDir)
	return &Deployer{
		Store:        st,
		DockerClient: dClient,
	}, nil
}

// Deploy executes the complete application deployment lifecycle.
func (d *Deployer) Deploy(ctx context.Context, opts DeployOptions) (*store.Deployment, error) {
	out := opts.LogOutput
	if out == nil {
		out = os.Stdout
	}

	// 1. Validate Repository URL
	if err := git.ValidateURL(opts.RepoURL); err != nil {
		return nil, fmt.Errorf("invalid repository: %w", err)
	}

	// 2. Validate Docker availability
	if err := d.DockerClient.Ping(ctx); err != nil {
		return nil, fmt.Errorf("docker engine verification failed: %w", err)
	}

	// 3. Resolve and sanitize application name
	appName := strings.TrimSpace(strings.ToLower(opts.AppName))
	if appName == "" {
		appName = strings.ToLower(git.ParseRepoName(opts.RepoURL))
	}
	appName = strings.ReplaceAll(appName, ".", "-")
	if !validAppNameRegex.MatchString(appName) {
		return nil, fmt.Errorf("invalid application name '%s': must use lowercase letters, numbers, hyphens or underscores", appName)
	}

	baseDomain := proxy.GetBaseDomain(opts.BaseDomain)

	// 4. Resolve or create Application entity
	app, err := d.Store.GetApplication(appName)
	if err != nil {
		app = &store.Application{
			ID:         store.GenerateID("app"),
			Name:       appName,
			RepoURL:    opts.RepoURL,
			Branch:     opts.Branch,
			PublicURL:  proxy.GetPublicURL(appName, baseDomain),
			Dockerfile: opts.Dockerfile,
		}
	}
	app.RepoURL = opts.RepoURL
	if opts.Branch != "" {
		app.Branch = opts.Branch
	}
	if opts.Dockerfile != "" {
		app.Dockerfile = opts.Dockerfile
	}
	app.PublicURL = proxy.GetPublicURL(app.Name, baseDomain)

	// 5. Create Deployment record
	depID := store.GenerateID("dep")
	dep := &store.Deployment{
		ID:        depID,
		AppID:     app.ID,
		AppName:   app.Name,
		RepoURL:   opts.RepoURL,
		Branch:    app.Branch,
		Status:    store.StagePending,
		StartedAt: time.Now().UTC(),
	}
	if err := d.Store.SaveDeployment(dep); err != nil {
		return nil, fmt.Errorf("failed to initialize deployment record: %w", err)
	}

	fmt.Fprintf(out, "==> Deploying application '%s' (Deployment ID: %s)\n", app.Name, dep.ID)

	// Step 1: Git Clone
	fmt.Fprintf(out, "\n[1/5] Cloning repository...\n")
	fmt.Fprintf(out, "      URL:    %s\n", opts.RepoURL)
	if app.Branch != "" {
		fmt.Fprintf(out, "      Branch: %s\n", app.Branch)
	}

	dep.Status = store.StageCloning
	_ = d.Store.SaveDeployment(dep)

	sourceDir := filepath.Join(d.Store.DataDir, "apps", app.ID, "source")
	if err := git.Clone(ctx, opts.RepoURL, app.Branch, sourceDir); err != nil {
		return d.failDeployment(dep, err, "cloning failed")
	}

	// Step 2: Project Detection
	fmt.Fprintf(out, "\n[2/5] Detecting project type...\n")
	spec, err := detector.Detect(sourceDir, app.Dockerfile)
	if err != nil {
		return d.failDeployment(dep, err, "project detection failed")
	}
	fmt.Fprintf(out, "      Type: %s\n", spec.Type)

	if spec.GeneratedDocker {
		dfPath := filepath.Join(sourceDir, spec.DockerfilePath)
		if err := os.WriteFile(dfPath, []byte(spec.DockerfileContent), 0644); err != nil {
			return d.failDeployment(dep, err, "failed to write generated Dockerfile")
		}
		fmt.Fprintf(out, "      Note: Automatically generated production Dockerfile for %s\n", spec.Type)
	}

	// Resolve Port
	targetPort := opts.Port
	if targetPort <= 0 {
		if app.Port > 0 {
			targetPort = app.Port
		} else {
			p, err := docker.FindAvailablePort(spec.DefaultPort)
			if err != nil {
				return d.failDeployment(dep, err, "failed to assign port")
			}
			targetPort = p
		}
	}
	dep.Port = targetPort

	// Step 3: Docker Build
	fmt.Fprintf(out, "\n[3/5] Building Docker image...\n")
	imageTag := fmt.Sprintf("nugget-%s:%s", app.Name, dep.ID)
	dep.ImageTag = imageTag
	dep.Status = store.StageBuilding
	_ = d.Store.SaveDeployment(dep)

	var buildLog bytes.Buffer
	fmt.Fprintf(out, "      Image Tag: %s\n", imageTag)
	if err := d.DockerClient.BuildImage(ctx, sourceDir, spec.DockerfilePath, imageTag, &buildLog); err != nil {
		dep.Log = buildLog.String()
		return d.failDeployment(dep, err, "build failed")
	}
	dep.Log = buildLog.String()

	// Step 4: Configure Reverse Proxy Network & Routing
	fmt.Fprintf(out, "\n[4/5] Configuring reverse proxy & networking...\n")
	_ = proxy.EnsureProxyRunning(ctx, d.DockerClient, d.Store.DataDir, baseDomain)

	// Combine Traefik labels
	labels := proxy.GenerateTraefikLabels(app.Name, baseDomain, spec.DefaultPort)
	labels["nugget.managed"] = "true"
	labels["nugget.app_id"] = app.ID
	labels["nugget.deployment_id"] = dep.ID

	// Write dynamic file routing config
	_ = proxy.WriteDynamicConfig(d.Store.DataDir, app.Name, baseDomain, targetPort)
	fmt.Fprintf(out, "      Routing Host: %s\n", proxy.GetHostname(app.Name, baseDomain))

	// Step 5: Container Run & Port Binding
	fmt.Fprintf(out, "\n[5/5] Starting application container...\n")
	dep.Status = store.StageStarting
	_ = d.Store.SaveDeployment(dep)

	// Parse environment file or preserve existing environment
	envMap := make(map[string]string)
	if len(app.Env) > 0 {
		for k, v := range app.Env {
			envMap[k] = v
		}
	}
	if opts.EnvFile != "" {
		fileEnv, err := docker.ParseEnvFile(opts.EnvFile)
		if err != nil {
			return d.failDeployment(dep, err, "invalid environment file")
		}
		for k, v := range fileEnv {
			envMap[k] = v
		}
		fmt.Fprintf(out, "      Loaded %d environment variables from %s\n", len(fileEnv), filepath.Base(opts.EnvFile))
	}
	app.Env = envMap

	containerName := fmt.Sprintf("nugget-%s-%s", app.Name, dep.ID)
	runOpts := docker.RunOptions{
		ImageTag:      imageTag,
		ContainerName: containerName,
		HostPort:      targetPort,
		ContainerPort: spec.DefaultPort,
		Network:       proxy.DefaultNetwork,
		Env:           envMap,
		RestartPolicy: "unless-stopped",
		Labels:        labels,
	}

	containerID, err := d.DockerClient.RunContainer(ctx, runOpts)
	if err != nil {
		return d.failDeployment(dep, err, "container start failed")
	}

	dep.ContainerID = containerID
	fmt.Fprintf(out, "      Container ID: %s\n", containerID)
	fmt.Fprintf(out, "      Internal Port: http://localhost:%d\n", targetPort)
	fmt.Fprintf(out, "      Public URL:    %s\n", app.PublicURL)

	// Check tunnel status
	tStatus := tunnel.InspectTunnel(ctx, d.DockerClient, "", baseDomain)
	if tStatus.Running {
		fmt.Fprintf(out, "      Tunnel:        Connected & Routing (*.%s)\n", baseDomain)
	} else {
		fmt.Fprintf(out, "      Tunnel:        Not active (%s)\n", tStatus.Message)
	}

	// Gracefully stop old container if this was an update
	oldContainerID := app.ActiveContainerID
	if oldContainerID != "" && oldContainerID != containerID {
		_ = d.DockerClient.StopAndRemoveContainer(ctx, oldContainerID)
	}

	// Update Application State
	app.ActiveDeploymentID = dep.ID
	app.ActiveContainerID = containerID
	app.Port = targetPort
	app.Status = "running"
	if err := d.Store.SaveApplication(app); err != nil {
		fmt.Fprintf(out, "Warning: failed to persist active application state: %v\n", err)
	}

	// Update Deployment Status to Succeeded
	dep.Status = store.StageSucceeded
	dep.CompletedAt = time.Now().UTC()
	_ = d.Store.SaveDeployment(dep)

	fmt.Fprintf(out, "\n==> Deployment succeeded!\n")
	fmt.Fprintf(out, "    Local:  http://localhost:%d\n", targetPort)
	fmt.Fprintf(out, "    Public: %s\n", app.PublicURL)
	return dep, nil
}

// Redeploy rebuilds and updates an existing application while preserving its configuration and port.
func (d *Deployer) Redeploy(ctx context.Context, appNameOrID string, out io.Writer) (*store.Deployment, error) {
	app, err := d.Store.GetApplication(appNameOrID)
	if err != nil {
		return nil, err
	}

	opts := DeployOptions{
		RepoURL:    app.RepoURL,
		AppName:    app.Name,
		Branch:     app.Branch,
		Port:       app.Port,
		Dockerfile: app.Dockerfile,
		DataDir:    d.Store.DataDir,
		LogOutput:  out,
	}

	return d.Deploy(ctx, opts)
}

// Restart restarts the running container of an application.
func (d *Deployer) Restart(ctx context.Context, appNameOrID string) error {
	app, err := d.Store.GetApplication(appNameOrID)
	if err != nil {
		return err
	}
	if app.ActiveContainerID == "" {
		return fmt.Errorf("application '%s' has no active container to restart", app.Name)
	}

	if err := d.DockerClient.RestartContainer(ctx, app.ActiveContainerID); err != nil {
		return err
	}
	app.Status = "running"
	return d.Store.SaveApplication(app)
}

// Stop stops the container of an application without deleting its configuration.
func (d *Deployer) Stop(ctx context.Context, appNameOrID string) error {
	app, err := d.Store.GetApplication(appNameOrID)
	if err != nil {
		return err
	}
	if app.ActiveContainerID == "" {
		return fmt.Errorf("application '%s' has no active container", app.Name)
	}

	if err := d.DockerClient.StopContainer(ctx, app.ActiveContainerID); err != nil {
		return err
	}
	app.Status = "stopped"
	return d.Store.SaveApplication(app)
}

// Remove stops, deletes the container, clears proxy routing, and removes application metadata.
func (d *Deployer) Remove(ctx context.Context, appNameOrID string) error {
	app, err := d.Store.GetApplication(appNameOrID)
	if err != nil {
		return err
	}

	// 1. Stop and remove container
	if app.ActiveContainerID != "" {
		_ = d.DockerClient.StopAndRemoveContainer(ctx, app.ActiveContainerID)
	}

	// 2. Remove proxy routing config
	_ = proxy.RemoveDynamicConfig(d.Store.DataDir, app.Name)

	// 3. Remove application metadata and source files
	return d.Store.RemoveApplication(app.ID)
}

func (d *Deployer) failDeployment(dep *store.Deployment, err error, reason string) (*store.Deployment, error) {
	dep.Status = store.StageFailed
	dep.Error = fmt.Sprintf("%s: %v", reason, err)
	dep.CompletedAt = time.Now().UTC()
	_ = d.Store.SaveDeployment(dep)
	return dep, fmt.Errorf("%s: %w", reason, err)
}
