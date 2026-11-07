package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/cli"
	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/daemon"
	"github.com/nugget-sh/nugget/agent/internal/doctor"
	"github.com/nugget-sh/nugget/agent/internal/store"
)

func TestEndToEndAgentLifecycle(t *testing.T) {
	tempRoot := t.TempDir()
	configDir := filepath.Join(tempRoot, "etc_nugget")
	dataDir := filepath.Join(tempRoot, "var_nugget")
	binDir := filepath.Join(tempRoot, "bin")

	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	fakeBinary := filepath.Join(binDir, "nugget")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\necho nugget v1.0.0\n"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// ---------------------------------------------------------
	// Step 1: Agent Initialization
	// ---------------------------------------------------------
	t.Log("Step 1: Running Agent Initialization...")
	initOpts := cli.InitOptions{
		MachineName:    "prod-worker-node-alpha",
		ConfigDir:      configDir,
		Force:          false,
		NonInteractive: true,
	}

	if err := cli.ExecuteInit(initOpts, nil); err != nil {
		t.Fatalf("Initialization failed: %v", err)
	}

	// Verify configuration
	cfg, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}
	if cfg.MachineName != "prod-worker-node-alpha" {
		t.Errorf("Expected machine name 'prod-worker-node-alpha', got %q", cfg.MachineName)
	}
	if cfg.MachineID == "" {
		t.Fatal("Expected generated non-empty machine ID")
	}

	// ---------------------------------------------------------
	// Step 2: System Health Diagnostics (Doctor)
	// ---------------------------------------------------------
	t.Log("Step 2: Running Diagnostics (Doctor)...")
	report := doctor.RunDiagnostics(configDir, dataDir)
	if report == nil {
		t.Fatal("Doctor returned nil report")
	}

	// Confirm that the initialized config is recognized and valid
	var configCheckPassed bool
	for _, res := range report.Results {
		if res.Category == "Configuration" && res.Name == "Machine Identity" && res.Level == doctor.LevelPass {
			configCheckPassed = true
		}
	}
	if !configCheckPassed {
		t.Error("Doctor failed to recognize valid machine identity")
	}

	// ---------------------------------------------------------
	// Step 3: Application State Store Lifecycle
	// ---------------------------------------------------------
	t.Log("Step 3: Testing Application Store & State Tracking...")
	appStore := store.NewStore(dataDir)

	testApp := &store.Application{
		ID:                "app_test12345",
		Name:              "web-landing",
		RepoURL:           "https://github.com/nugget-sh/example-app.git",
		Branch:            "main",
		Status:            "running",
		Port:              8080,
		PublicURL:         "http://web-landing.apps.nugget.run",
		ActiveContainerID: "mock-container-12345",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := appStore.SaveApplication(testApp); err != nil {
		t.Fatalf("Failed to save application: %v", err)
	}

	apps, err := appStore.ListApplications()
	if err != nil {
		t.Fatalf("Failed to list applications: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "web-landing" {
		t.Errorf("Unexpected applications listed: %+v", apps)
	}

	// ---------------------------------------------------------
	// Step 4: Daemon Process Lock and Single-Instance Guarantee
	// ---------------------------------------------------------
	t.Log("Step 4: Testing Daemon Process Lock...")
	lock1, err := daemon.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("Failed to acquire initial daemon lock: %v", err)
	}

	running, pid := daemon.IsDaemonRunning(dataDir)
	if !running || pid <= 0 {
		t.Errorf("Expected daemon to be detected as running with active PID, got running=%v, pid=%d", running, pid)
	}

	// Attempt second lock (should fail / report already running)
	_, err = daemon.AcquireLock(dataDir)
	if err == nil {
		t.Error("Expected error attempting to acquire duplicate daemon lock")
	}

	lock1.Release()

	// ---------------------------------------------------------
	// Step 5: Safe Agent Upgrade
	// ---------------------------------------------------------
	t.Log("Step 5: Testing Safe Agent Upgrade...")
	newBinary := filepath.Join(tempRoot, "nugget-v2")
	if err := os.WriteFile(newBinary, []byte("#!/bin/sh\necho nugget v2.0.0\n"), 0755); err != nil {
		t.Fatalf("Failed to write upgraded binary: %v", err)
	}

	if err := cli.RunUpdate([]string{"--binary", newBinary, "--target", fakeBinary, "--force"}); err != nil {
		t.Fatalf("Agent update failed: %v", err)
	}

	// Verify configuration & app data remained intact
	reloadedCfg, err := config.Load(configDir)
	if err != nil || reloadedCfg.MachineID != cfg.MachineID {
		t.Fatalf("Configuration compromised during update: %v", err)
	}

	reloadedApps, err := appStore.ListApplications()
	if err != nil || len(reloadedApps) != 1 || reloadedApps[0].Name != "web-landing" {
		t.Fatalf("Application data compromised during update: %v", err)
	}

	// ---------------------------------------------------------
	// Step 6: Safe Agent Uninstallation (Preserving Data)
	// ---------------------------------------------------------
	t.Log("Step 6: Testing Agent Uninstall (Preserving Data)...")
	if err := cli.RunUninstall([]string{"--yes", "--target", fakeBinary}); err != nil {
		t.Fatalf("Agent uninstall failed: %v", err)
	}

	if _, err := os.Stat(fakeBinary); !os.IsNotExist(err) {
		t.Errorf("Expected binary %s to be removed", fakeBinary)
	}

	// Verify config and app data were NOT deleted
	if !config.Exists(configDir) {
		t.Error("Expected config directory to be preserved during standard uninstall")
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("Expected data directory to be preserved during standard uninstall")
	}

	t.Log("✓ All End-to-End Agent Lifecycle stages completed successfully!")
}

func TestEndToEndDiagnosticsJSONOutput(t *testing.T) {
	tempRoot := t.TempDir()
	configDir := filepath.Join(tempRoot, "config")
	dataDir := filepath.Join(tempRoot, "data")

	report := doctor.RunDiagnostics(configDir, dataDir)
	jsonBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("DoctorReport failed to serialize to JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Invalid JSON produced by DoctorReport: %v", err)
	}

	if _, exists := parsed["results"]; !exists {
		t.Errorf("Expected 'results' key in doctor JSON output")
	}
	if _, exists := parsed["pass_count"]; !exists {
		t.Errorf("Expected 'pass_count' key in doctor JSON output")
	}
}
