package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	port := labels["traefik.http.services.nugget-web-frontend.loadbalancer.server.port"]
	if port != "3000" {
		t.Errorf("Expected port 3000, got %s", port)
	}

	if labels["traefik.docker.network"] != DefaultNetwork {
		t.Errorf("Expected network %s, got %s", DefaultNetwork, labels["traefik.docker.network"])
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

