package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nugget-sh/nugget/agent/internal/docker"
)

const (
	DefaultNetwork       = "nugget-network"
	DefaultBaseDomain    = "apps.nugget.run"
	ProxyContainerName   = "nugget-traefik"
	TraefikImage         = "traefik:v3.1"
	TraefikHTTPPort      = 80
	TraefikDashboardPort = 8080
)

// ProxyStatus describes the operational state of the Traefik reverse proxy.
type ProxyStatus struct {
	Running     bool   `json:"running"`
	ContainerID string `json:"container_id,omitempty"`
	Network     string `json:"network"`
	BaseDomain  string `json:"base_domain"`
	Details     string `json:"details"`
}

// GetBaseDomain returns the configured base domain, falling back to DefaultBaseDomain.
func GetBaseDomain(configured string) string {
	if domain := os.Getenv("NUGGET_BASE_DOMAIN"); domain != "" {
		return strings.TrimPrefix(domain, ".")
	}
	if configured != "" {
		return strings.TrimPrefix(configured, ".")
	}
	return DefaultBaseDomain
}

// GetHostname returns the full virtual host for an application (e.g. "my-app.apps.nugget.run").
func GetHostname(appName, baseDomain string) string {
	domain := GetBaseDomain(baseDomain)
	return fmt.Sprintf("%s.%s", strings.ToLower(appName), domain)
}

// GetPublicURL returns the HTTPS public endpoint for an application.
func GetPublicURL(appName, baseDomain string) string {
	return fmt.Sprintf("https://%s", GetHostname(appName, baseDomain))
}

// GenerateTraefikLabels produces Docker container labels for Traefik reverse proxy routing.
func GenerateTraefikLabels(appName, baseDomain string, containerPort int) map[string]string {
	hostname := GetHostname(appName, baseDomain)
	routerName := fmt.Sprintf("nugget-%s", appName)
	serviceName := fmt.Sprintf("nugget-%s", appName)

	return map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", routerName):       fmt.Sprintf("Host(`%s`)", hostname),
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName): "web",
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName): fmt.Sprintf("%d", containerPort),
		"traefik.docker.network": DefaultNetwork,
	}
}

// WriteDynamicConfig writes a file-based Traefik YAML configuration for local routing.
func WriteDynamicConfig(dataDir, appName, baseDomain string, hostPort int) error {
	dynamicDir := filepath.Join(dataDir, "proxy", "dynamic")
	if err := os.MkdirAll(dynamicDir, 0755); err != nil {
		return fmt.Errorf("failed to create proxy dynamic config dir: %w", err)
	}

	hostname := GetHostname(appName, baseDomain)
	content := fmt.Sprintf(`http:
  routers:
    %s:
      rule: "Host(\"%s\")"
      entryPoints:
        - "web"
      service: "%s-svc"
  services:
    %s-svc:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:%d"
`, appName, hostname, appName, appName, hostPort)

	configPath := filepath.Join(dynamicDir, fmt.Sprintf("%s.yaml", appName))
	return os.WriteFile(configPath, []byte(content), 0644)
}

// RemoveDynamicConfig cleans up the dynamic routing configuration file for an application.
func RemoveDynamicConfig(dataDir, appName string) error {
	configPath := filepath.Join(dataDir, "proxy", "dynamic", fmt.Sprintf("%s.yaml", appName))
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove proxy dynamic config %s: %w", configPath, err)
	}
	return nil
}

// InspectProxyStatus inspects whether the Traefik proxy container is active.
func InspectProxyStatus(ctx context.Context, dClient *docker.Client, baseDomain string) *ProxyStatus {
	domain := GetBaseDomain(baseDomain)
	status := &ProxyStatus{
		Running:    false,
		Network:    DefaultNetwork,
		BaseDomain: domain,
		Details:    "Traefik reverse proxy is not running.",
	}

	state, err := dClient.InspectContainer(ctx, ProxyContainerName)
	if err == nil && state.State.Running {
		status.Running = true
		status.ContainerID = state.ID
		if len(status.ContainerID) > 12 {
			status.ContainerID = status.ContainerID[:12]
		}
		status.Details = "Traefik reverse proxy is active and routing requests on port 80."
	}

	return status
}

// EnsureProxyRunning ensures the Docker network exists and Traefik container is running.
func EnsureProxyRunning(ctx context.Context, dClient *docker.Client, dataDir, baseDomain string) error {
	// 1. Ensure docker network
	if err := dClient.EnsureNetwork(ctx, DefaultNetwork); err != nil {
		return fmt.Errorf("failed to ensure proxy network: %w", err)
	}

	// 2. Check if Traefik is already running
	state, err := dClient.InspectContainer(ctx, ProxyContainerName)
	if err == nil && state.State.Running {
		return nil
	}

	// Stop any stale stopped container
	_ = dClient.StopAndRemoveContainer(ctx, ProxyContainerName)

	dynamicDir := filepath.Join(dataDir, "proxy", "dynamic")
	_ = os.MkdirAll(dynamicDir, 0755)

	// Run Traefik container
	runOpts := docker.RunOptions{
		ImageTag:      TraefikImage,
		ContainerName: ProxyContainerName,
		HostPort:      TraefikHTTPPort,
		ContainerPort: TraefikHTTPPort,
		Network:       DefaultNetwork,
		RestartPolicy: "unless-stopped",
		Labels: map[string]string{
			"nugget.managed": "true",
			"nugget.role":    "proxy",
		},
	}

	_, err = dClient.RunContainer(ctx, runOpts)
	if err != nil {
		return fmt.Errorf("failed to start Traefik reverse proxy: %w", err)
	}

	return nil
}

