package proxy

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/docker"
)

func TestHostnameAndURL(t *testing.T) {
	appName := "my-api"
	baseDomain := "apps.nugget.run"

	hostname := GetHostname(appName, baseDomain)
	if hostname != "my-api.apps.nugget.run" {
		t.Errorf("Expected my-api.apps.nugget.run, got %s", hostname)
	}

	publicURL := GetPublicURL(appName, baseDomain)
	if publicURL != "https://my-api.apps.nugget.run" {
		t.Errorf("Expected https://my-api.apps.nugget.run, got %s", publicURL)
	}
}

func TestGenerateTraefikLabels(t *testing.T) {
	labels := GenerateTraefikLabels("web-frontend", "apps.nugget.run", 3000)

	if labels["traefik.enable"] != "true" {
		t.Errorf("Expected traefik.enable=true")
	}

	rule := labels["traefik.http.routers.nugget-web-frontend.rule"]
	if rule != "Host(`web-frontend.apps.nugget.run`)" {
		t.Errorf("Unexpected router rule: %s", rule)
	}

	entrypoints := labels["traefik.http.routers.nugget-web-frontend.entrypoints"]
	if entrypoints != "web" {
		t.Errorf("Expected entrypoints=web, got %s", entrypoints)
	}

	port := labels["traefik.http.services.nugget-web-frontend.loadbalancer.server.port"]
	if port != "3000" {
		t.Errorf("Expected port 3000, got %s", port)
	}

	if labels["traefik.docker.network"] != DefaultNetwork {
		t.Errorf("Expected network %s, got %s", DefaultNetwork, labels["traefik.docker.network"])
	}
}

func TestRequiredTraefikArgs(t *testing.T) {
	args := RequiredTraefikArgs("nugget-network")

	expected := []string{
		"--providers.docker=true",
		"--providers.docker.exposedbydefault=false",
		"--providers.docker.network=nugget-network",
		"--entrypoints.web.address=:80",
		"--api.dashboard=true",
		"--api.insecure=false",
	}

	argMap := make(map[string]bool)
	for _, a := range args {
		argMap[a] = true
	}

	for _, exp := range expected {
		if !argMap[exp] {
			t.Errorf("RequiredTraefikArgs missing argument: %s", exp)
		}
	}
}

func TestDockerSocketMountConstant(t *testing.T) {
	if DockerSocketMount != "/var/run/docker.sock:/var/run/docker.sock:ro" {
		t.Errorf("Unexpected DockerSocketMount: %s", DockerSocketMount)
	}
}

func TestIsTraefikConfigValid(t *testing.T) {
	// 1. Valid container state
	validState := &docker.ContainerState{}
	validState.State.Running = true
	validState.Config.Cmd = RequiredTraefikArgs(DefaultNetwork)
	validState.Mounts = []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Mode        string `json:"Mode"`
		RW          bool   `json:"RW"`
	}{
		{
			Type:        "bind",
			Source:      "/var/run/docker.sock",
			Destination: "/var/run/docker.sock",
			Mode:        "ro",
			RW:          false,
		},
	}

	if !IsTraefikConfigValid(validState, DefaultNetwork) {
		t.Errorf("Expected validState to pass validation")
	}

	// 2. Stopped container
	stoppedState := *validState
	stoppedState.State.Running = false
	if IsTraefikConfigValid(&stoppedState, DefaultNetwork) {
		t.Errorf("Stopped container should fail validation")
	}

	// 3. Outdated legacy container (Cmd: ["traefik"] only)
	legacyState := *validState
	legacyState.Config.Cmd = []string{"traefik"}
	if IsTraefikConfigValid(&legacyState, DefaultNetwork) {
		t.Errorf("Legacy container with Cmd: ['traefik'] must fail validation")
	}

	// 4. Missing read-only socket mount
	noSocketState := *validState
	noSocketState.Mounts = nil
	noSocketState.HostConfig.Binds = nil
	if IsTraefikConfigValid(&noSocketState, DefaultNetwork) {
		t.Errorf("Container without socket mount must fail validation")
	}

	// 5. Wrong network argument
	wrongNetworkState := *validState
	wrongNetworkState.Config.Cmd = []string{
		"--providers.docker=true",
		"--providers.docker.exposedbydefault=false",
		"--providers.docker.network=wrong-network",
		"--entrypoints.web.address=:80",
		"--api.dashboard=true",
		"--api.insecure=false",
	}
	if IsTraefikConfigValid(&wrongNetworkState, DefaultNetwork) {
		t.Errorf("Container with wrong network must fail validation")
	}
}

func TestDynamicConfigLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	err := WriteDynamicConfig(tmpDir, "sample-app", "apps.nugget.run", 8080)
	if err != nil {
		t.Fatalf("WriteDynamicConfig failed: %v", err)
	}

	configFile := filepath.Join(tmpDir, "proxy", "dynamic", "sample-app.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read dynamic config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "sample-app.apps.nugget.run") {
		t.Errorf("Dynamic config missing hostname: %s", content)
	}
	if !strings.Contains(content, "http://127.0.0.1:8080") {
		t.Errorf("Dynamic config missing host port: %s", content)
	}

	// Remove dynamic config
	if err := RemoveDynamicConfig(tmpDir, "sample-app"); err != nil {
		t.Fatalf("RemoveDynamicConfig failed: %v", err)
	}

	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Errorf("Expected config file to be removed")
	}
}

func TestEnsureProxyRunning_Reconciliation(t *testing.T) {
	dClient, err := docker.NewClient()
	if err != nil {
		t.Skip("Docker CLI not available, skipping integration test")
	}
	ctx := t.Context()
	if err := dClient.Ping(ctx); err != nil {
		t.Skip("Docker daemon not running, skipping integration test")
	}

	tmpDir := t.TempDir()

	// Ensure clean slate for test
	_ = dClient.StopAndRemoveContainer(ctx, ProxyContainerName)
	defer func() {
		_ = dClient.StopAndRemoveContainer(ctx, ProxyContainerName)
	}()

	// 1. First EnsureProxyRunning creates fresh container
	if err := EnsureProxyRunning(ctx, dClient, tmpDir, "apps.nugget.run"); err != nil {
		t.Fatalf("EnsureProxyRunning failed: %v", err)
	}

	state, err := dClient.InspectContainer(ctx, ProxyContainerName)
	if err != nil {
		t.Fatalf("Failed to inspect ProxyContainerName: %v", err)
	}
	if !state.State.Running {
		t.Fatalf("Expected Traefik container to be running")
	}
	if !IsTraefikConfigValid(state, DefaultNetwork) {
		t.Fatalf("Expected created container to have valid Traefik config: %+v", state.Config.Cmd)
	}

	// 2. Simulate an existing outdated legacy container (e.g. started with Cmd: ["traefik"])
	_ = dClient.StopAndRemoveContainer(ctx, ProxyContainerName)
	legacyOpts := docker.RunOptions{
		ImageTag:      TraefikImage,
		ContainerName: ProxyContainerName,
		Network:       DefaultNetwork,
		Cmd:           []string{"traefik"},
	}
	_, err = dClient.RunContainer(ctx, legacyOpts)
	if err != nil {
		t.Fatalf("Failed to start legacy container: %v", err)
	}

	legacyState, err := dClient.InspectContainer(ctx, ProxyContainerName)
	if err != nil || IsTraefikConfigValid(legacyState, DefaultNetwork) {
		t.Fatalf("Legacy container should be invalid")
	}

	// 3. Call EnsureProxyRunning again - it must reconcile, replace, and update
	if err := EnsureProxyRunning(ctx, dClient, tmpDir, "apps.nugget.run"); err != nil {
		t.Fatalf("EnsureProxyRunning reconciliation failed: %v", err)
	}

	reconciledState, err := dClient.InspectContainer(ctx, ProxyContainerName)
	if err != nil {
		t.Fatalf("Failed to inspect reconciled container: %v", err)
	}
	if !reconciledState.State.Running {
		t.Fatalf("Expected reconciled container to be running")
	}
	if !IsTraefikConfigValid(reconciledState, DefaultNetwork) {
		t.Fatalf("Expected reconciled container to have valid Traefik config: %+v", reconciledState.Config.Cmd)
	}
}

func TestTraefikDockerProviderRouting(t *testing.T) {
	dClient, err := docker.NewClient()
	if err != nil {
		t.Skip("Docker CLI not available, skipping integration test")
	}
	ctx := t.Context()
	if err := dClient.Ping(ctx); err != nil {
		t.Skip("Docker daemon not running, skipping integration test")
	}

	tmpDir := t.TempDir()

	// Ensure Traefik is running with Docker provider
	if err := EnsureProxyRunning(ctx, dClient, tmpDir, "apps.nugget.run"); err != nil {
		t.Fatalf("EnsureProxyRunning failed: %v", err)
	}
	defer func() {
		_ = dClient.StopAndRemoveContainer(ctx, ProxyContainerName)
	}()

	testAppName := "test-routing-app"
	testHost := "test-routing-app.apps.nugget.run"
	containerName := "nugget-test-routing-app"

	// Stop any existing test container
	_ = dClient.StopAndRemoveContainer(ctx, containerName)
	defer func() {
		_ = dClient.StopAndRemoveContainer(ctx, containerName)
	}()

	// Start a mock app container using nginx:alpine with Traefik routing labels
	labels := GenerateTraefikLabels(testAppName, "apps.nugget.run", 80)
	runOpts := docker.RunOptions{
		ImageTag:      "nginx:alpine",
		ContainerName: containerName,
		Network:       DefaultNetwork,
		Labels:        labels,
	}

	_, err = dClient.RunContainer(ctx, runOpts)
	if err != nil {
		t.Fatalf("Failed to run test container: %v", err)
	}

	// Poll Traefik on http://127.0.0.1:80 with Host header
	client := &http.Client{Timeout: 2 * time.Second}
	var success bool
	for attempt := 0; attempt < 20; attempt++ {
		time.Sleep(500 * time.Millisecond)

		req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:80/", nil)
		if err != nil {
			continue
		}
		req.Host = testHost

		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				success = true
				break
			}
		}
	}

	if !success {
		t.Errorf("Failed to receive HTTP 200 via Traefik routing for host %s", testHost)
	}

	// Verify that unregistered host returns 404
	reqUnknown, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:80/", nil)
	reqUnknown.Host = "unregistered.apps.nugget.run"
	respUnknown, err := client.Do(reqUnknown)
	if err == nil {
		defer respUnknown.Body.Close()
		if respUnknown.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 for unregistered host, got %d", respUnknown.StatusCode)
		}
	}
}
