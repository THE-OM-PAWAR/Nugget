package tunnel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/proxy"
)

const (
	TunnelContainerName = "nugget-cloudflared"
	CloudflaredImage    = "cloudflare/cloudflared:latest"
)

// TunnelStatus encapsulates Cloudflare Tunnel operational and configuration state.
type TunnelStatus struct {
	Configured   bool   `json:"configured"`
	Running      bool   `json:"running"`
	ContainerID  string `json:"container_id,omitempty"`
	BaseDomain   string `json:"base_domain"`
	WildcardRule string `json:"wildcard_rule"`
	Message      string `json:"message"`
}

// GetTunnelToken returns the Cloudflare Tunnel token from environment or config.
func GetTunnelToken(configuredToken string) string {
	if token := os.Getenv("NUGGET_TUNNEL_TOKEN"); token != "" {
		return strings.TrimSpace(token)
	}
	return strings.TrimSpace(configuredToken)
}

// InspectTunnel checks whether Cloudflare Tunnel is configured and running.
func InspectTunnel(ctx context.Context, dClient *docker.Client, configuredToken, baseDomain string) *TunnelStatus {
	domain := proxy.GetBaseDomain(baseDomain)
	token := GetTunnelToken(configuredToken)
	wildcard := fmt.Sprintf("*.%s", domain)

	status := &TunnelStatus{
		Configured:   token != "",
		Running:      false,
		BaseDomain:   domain,
		WildcardRule: fmt.Sprintf("%s -> Cloudflare Tunnel -> Local Traefik (port 80)", wildcard),
	}

	if !status.Configured {
		status.Message = fmt.Sprintf("Cloudflare Tunnel is not configured. Set NUGGET_TUNNEL_TOKEN to enable automatic public HTTPS routing for *.%s.", domain)
		return status
	}

	// 1. Check if nugget-cloudflared docker container is running
	if dClient != nil {
		state, err := dClient.InspectContainer(ctx, TunnelContainerName)
		if err == nil && state.State.Running {
			status.Running = true
			status.ContainerID = state.ID
			if len(status.ContainerID) > 12 {
				status.ContainerID = status.ContainerID[:12]
			}
			status.Message = fmt.Sprintf("Cloudflare Tunnel is connected and actively routing %s to Traefik.", wildcard)
			return status
		}
	}

	// 2. Check if local cloudflared daemon is running on host
	if _, err := exec.LookPath("cloudflared"); err == nil {
		checkCmd := exec.CommandContext(ctx, "pgrep", "-x", "cloudflared")
		if err := checkCmd.Run(); err == nil {
			status.Running = true
			status.Message = fmt.Sprintf("Cloudflare Tunnel host daemon (cloudflared) is running and routing %s.", wildcard)
			return status
		}
	}

	status.Message = "Cloudflare Tunnel token is configured, but tunnel daemon is not running."
	return status
}

// EnsureTunnel starts the Cloudflare Tunnel container on nugget-network if token is present.
func EnsureTunnel(ctx context.Context, dClient *docker.Client, token, baseDomain string) error {
	token = GetTunnelToken(token)
	if token == "" {
		return nil // Not configured; bypass gracefully
	}

	// Check if already running
	state, err := dClient.InspectContainer(ctx, TunnelContainerName)
	if err == nil && state.State.Running {
		return nil
	}

	_ = dClient.StopAndRemoveContainer(ctx, TunnelContainerName)

	args := []string{
		"run", "-d",
		"--name", TunnelContainerName,
		"--network", proxy.DefaultNetwork,
		"--restart", "unless-stopped",
		"--label", "nugget.managed=true",
		"--label", "nugget.role=tunnel",
		CloudflaredImage,
		"tunnel", "--no-autoupdate", "run", "--token", token,
	}

	cmd := exec.CommandContext(ctx, dClient.DockerPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start cloudflared container: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	return nil
}
