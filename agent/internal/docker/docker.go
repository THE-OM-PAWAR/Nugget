package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunOptions configures the creation and execution of an application container.
type RunOptions struct {
	ImageTag      string
	ContainerName string
	HostPort      int
	ContainerPort int
	Ports         []string
	Volumes       []string
	Env           map[string]string
	RestartPolicy string
	Labels        map[string]string
	Network       string
	Cmd           []string
}

// ContainerState represents the operational status of a container from docker inspect.
type ContainerState struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	State        struct {
		Status     string `json:"Status"` // "running", "exited", "restarting", etc.
		Running    bool   `json:"Running"`
		Restarting bool   `json:"Restarting"`
		ExitCode   int    `json:"ExitCode"`
		Error      string `json:"Error"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
	} `json:"State"`
	Config struct {
		Cmd    []string          `json:"Cmd"`
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		Binds []string `json:"Binds"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Mode        string `json:"Mode"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

// Client wraps safe interactions with the Docker CLI.
type Client struct {
	DockerPath string
}

// NewClient creates a new Docker client, ensuring the Docker CLI binary exists.
func NewClient() (*Client, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker is not installed or not in PATH: %w", err)
	}
	return &Client{DockerPath: dockerPath}, nil
}

// Ping verifies that the Docker daemon is running and responsive.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.DockerPath, "info")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker daemon is not running or accessible: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SupportsBuildx checks whether the Docker buildx plugin is installed and operational.
func (c *Client) SupportsBuildx(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, c.DockerPath, "buildx", "version")
	return cmd.Run() == nil
}

// BuildImage executes `docker build` safely with timeout and output streaming.
// It prefers BuildKit/buildx when available to avoid legacy builder deprecation warnings,
// falling back safely to standard docker build if buildx is not present.
func (c *Client) BuildImage(ctx context.Context, buildDir, dockerfilePath, imageTag string, logOut io.Writer) error {
	if buildDir == "" {
		return errors.New("build directory cannot be empty")
	}
	if imageTag == "" {
		return errors.New("image tag cannot be empty")
	}

	fullDockerfilePath := filepath.Join(buildDir, dockerfilePath)
	if _, err := os.Stat(fullDockerfilePath); err != nil {
		return fmt.Errorf("dockerfile not found at %s: %w", fullDockerfilePath, err)
	}

	buildCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	hasBuildx := c.SupportsBuildx(buildCtx)

	var args []string
	if hasBuildx {
		args = []string{
			"buildx", "build",
			"--load",
			"-f", fullDockerfilePath,
			"-t", imageTag,
			buildDir,
		}
	} else {
		args = []string{
			"build",
			"-f", fullDockerfilePath,
			"-t", imageTag,
			buildDir,
		}
	}

	cmd := exec.CommandContext(buildCtx, c.DockerPath, args...)
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")

	var buf bytes.Buffer
	var writer io.Writer = &buf
	if logOut != nil {
		writer = io.MultiWriter(&buf, logOut)
	}

	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		// If buildx failed, try fallback to standard build with BuildKit
		if hasBuildx {
			fallbackArgs := []string{
				"build",
				"-f", fullDockerfilePath,
				"-t", imageTag,
				buildDir,
			}
			fallbackCmd := exec.CommandContext(buildCtx, c.DockerPath, fallbackArgs...)
			fallbackCmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")

			var fallbackBuf bytes.Buffer
			var fallbackWriter io.Writer = &fallbackBuf
			if logOut != nil {
				fallbackWriter = io.MultiWriter(&fallbackBuf, logOut)
			}
			fallbackCmd.Stdout = fallbackWriter
			fallbackCmd.Stderr = fallbackWriter

			if fallbackErr := fallbackCmd.Run(); fallbackErr == nil {
				return nil
			}
		}

		if buildCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("docker build timed out after 15 minutes")
		}
		return fmt.Errorf("docker build failed: %s (%w)", strings.TrimSpace(buf.String()), err)
	}

	return nil
}

// RunContainer starts an application container with configured ports, restart policies, and environment.
func (c *Client) RunContainer(ctx context.Context, opts RunOptions) (string, error) {
	if opts.ImageTag == "" {
		return "", errors.New("image tag is required")
	}
	if opts.ContainerName == "" {
		return "", errors.New("container name is required")
	}

	args := []string{"run", "-d", "--name", opts.ContainerName}

	restart := opts.RestartPolicy
	if restart == "" {
		restart = "unless-stopped"
	}
	args = append(args, "--restart", restart)

	if opts.HostPort > 0 && opts.ContainerPort > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:%d", opts.HostPort, opts.ContainerPort))
	}

	for _, p := range opts.Ports {
		if p != "" {
			args = append(args, "-p", p)
		}
	}

	for _, v := range opts.Volumes {
		if v != "" {
			args = append(args, "-v", v)
		}
	}

	if opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}

	for k, v := range opts.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	// Securely pass environment variables via a temporary env-file with 0600 permissions
	var tempEnvFile string
	if len(opts.Env) > 0 {
		f, err := os.CreateTemp("", "nugget-env-*.env")
		if err != nil {
			return "", fmt.Errorf("failed to create temporary env file: %w", err)
		}
		tempEnvFile = f.Name()
		defer os.Remove(tempEnvFile)

		// Set restricted permissions
		_ = os.Chmod(tempEnvFile, 0600)

		for k, v := range opts.Env {
			if _, err := fmt.Fprintf(f, "%s=%s\n", k, v); err != nil {
				_ = f.Close()
				return "", fmt.Errorf("failed to write to temporary env file: %w", err)
			}
		}
		_ = f.Close()

		args = append(args, "--env-file", tempEnvFile)
	}

	args = append(args, opts.ImageTag)
	if len(opts.Cmd) > 0 {
		args = append(args, opts.Cmd...)
	}

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, c.DockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	containerID := strings.TrimSpace(string(out))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	// Verify container didn't crash immediately
	time.Sleep(1 * time.Second)
	inspectCtx, cancelInspect := context.WithTimeout(ctx, 10*time.Second)
	defer cancelInspect()

	state, err := c.InspectContainer(inspectCtx, containerID)
	if err == nil && !state.State.Running && state.State.ExitCode != 0 {
		return containerID, fmt.Errorf("container exited immediately with code %d: %s", state.State.ExitCode, state.State.Error)
	}

	return containerID, nil
}

// StopAndRemoveContainer stops and removes a container by name or ID.
func (c *Client) StopAndRemoveContainer(ctx context.Context, containerNameOrID string) error {
	if containerNameOrID == "" {
		return nil
	}

	stopCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// Stop container
	_ = exec.CommandContext(stopCtx, c.DockerPath, "stop", "-t", "5", containerNameOrID).Run()

	// Remove container
	rmCmd := exec.CommandContext(stopCtx, c.DockerPath, "rm", "-f", containerNameOrID)
	_ = rmCmd.Run()

	return nil
}

// InspectContainer queries container state via `docker inspect`.
func (c *Client) InspectContainer(ctx context.Context, containerNameOrID string) (*ContainerState, error) {
	cmd := exec.CommandContext(ctx, c.DockerPath, "inspect", containerNameOrID)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect failed for %s: %w", containerNameOrID, err)
	}

	var results []ContainerState
	if err := json.Unmarshal(out, &results); err != nil || len(results) == 0 {
		return nil, fmt.Errorf("invalid docker inspect output: %w", err)
	}

	return &results[0], nil
}

// EnsureNetwork creates a bridge docker network if it doesn't already exist.
func (c *Client) EnsureNetwork(ctx context.Context, networkName string) error {
	if networkName == "" {
		return nil
	}
	checkCmd := exec.CommandContext(ctx, c.DockerPath, "network", "inspect", networkName)
	if err := checkCmd.Run(); err == nil {
		return nil // already exists
	}

	createCmd := exec.CommandContext(ctx, c.DockerPath, "network", "create", networkName)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create docker network %s: %s (%w)", networkName, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Logs streams or fetches container logs.
func (c *Client) Logs(ctx context.Context, containerNameOrID string, follow bool, tail int, out io.Writer) error {
	if containerNameOrID == "" {
		return errors.New("container name or id required")
	}
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	} else if !follow {
		args = append(args, "--tail", "100")
	}
	args = append(args, containerNameOrID)

	cmd := exec.CommandContext(ctx, c.DockerPath, args...)
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("failed to fetch container logs: %w", err)
	}
	return nil
}

// RestartContainer restarts a container by name or ID.
func (c *Client) RestartContainer(ctx context.Context, containerNameOrID string) error {
	if containerNameOrID == "" {
		return errors.New("container name or id required")
	}
	cmd := exec.CommandContext(ctx, c.DockerPath, "restart", containerNameOrID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker restart failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// StopContainer stops a container by name or ID without deleting it.
func (c *Client) StopContainer(ctx context.Context, containerNameOrID string) error {
	if containerNameOrID == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, c.DockerPath, "stop", containerNameOrID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ParseEnvFile parses a KEY=VALUE environment file securely.
func ParseEnvFile(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			// Strip surrounding quotes
			if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
				v = v[1 : len(v)-1]
			}
			if k != "" {
				env[k] = v
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read env file: %w", err)
	}
	return env, nil
}

// IsPortAvailable checks if a TCP port is currently free on localhost.
func IsPortAvailable(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// FindAvailablePort finds an open TCP port on the host, starting from preferred.
func FindAvailablePort(preferred int) (int, error) {
	if preferred > 0 && IsPortAvailable(preferred) {
		return preferred, nil
	}
	start := preferred
	if start <= 0 {
		start = 3000
	}
	for p := start; p < 65535; p++ {
		if IsPortAvailable(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free ports available")
}

