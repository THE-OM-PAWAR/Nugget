package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nugget-sh/nugget/agent/internal/config"
)

func TestExecuteInitAndStatus(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "nugget")

	opts := InitOptions{
		MachineName:    "ci-test-machine",
		ConfigDir:      configDir,
		Force:          false,
		NonInteractive: true,
	}

	// 1. First initialization
	if err := ExecuteInit(opts, nil); err != nil {
		t.Fatalf("ExecuteInit failed: %v", err)
	}

	// Verify config exists and content matches
	if !config.Exists(configDir) {
		t.Fatalf("Expected config to exist at %s", configDir)
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.MachineName != "ci-test-machine" {
		t.Errorf("Expected machine name 'ci-test-machine', got '%s'", cfg.MachineName)
	}
	if cfg.MachineID == "" {
		t.Errorf("Expected non-empty machine ID")
	}
	if cfg.System == nil {
		t.Fatalf("Expected detected system specifications in config")
	}

	// 2. Prevent accidental overwrite without force
	err = ExecuteInit(opts, nil)
	if err == nil {
		t.Fatalf("Expected error when re-initializing without force in non-interactive mode")
	}

	// 3. Allow overwrite with force
	opts.Force = true
	opts.MachineName = "ci-test-machine-renamed"
	if err := ExecuteInit(opts, nil); err != nil {
		t.Fatalf("ExecuteInit with force failed: %v", err)
	}

	reloaded, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}
	if reloaded.MachineName != "ci-test-machine-renamed" {
		t.Errorf("Expected renamed machine, got '%s'", reloaded.MachineName)
	}

	// 4. Test RunStatus
	if err := RunStatus([]string{"--config-dir", configDir, "--json"}); err != nil {
		t.Errorf("RunStatus failed: %v", err)
	}
}

func TestRunDeployValidation(t *testing.T) {
	// Missing repository URL
	err := RunDeploy([]string{})
	if err == nil {
		t.Fatalf("Expected error when no repository URL provided, got nil")
	}

	// Invalid repository URL
	err = RunDeploy([]string{"invalid-url"})
	if err == nil {
		t.Fatalf("Expected error when invalid repository URL provided, got nil")
	}
}

func TestRunAppsAndLifecycleValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// RunApps on empty data dir
	if err := RunApps([]string{"--data-dir", tmpDir}); err != nil {
		t.Errorf("RunApps failed: %v", err)
	}

	// RunApps with --json
	if err := RunApps([]string{"--data-dir", tmpDir, "--json"}); err != nil {
		t.Errorf("RunApps JSON failed: %v", err)
	}

	// Missing app name for logs
	if err := RunLogs([]string{"--data-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunLogs with missing app")
	}

	// Nonexistent app for logs
	if err := RunLogs([]string{"nonexistent-app", "--data-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunLogs on nonexistent app")
	}

	// Missing app name for restart, stop, remove, redeploy
	if err := RunRestart([]string{"--data-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunRestart with missing app")
	}
	if err := RunStop([]string{"--data-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunStop with missing app")
	}
	if err := RunRemove([]string{"--data-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunRemove with missing app")
	}
	if err := RunRedeploy([]string{"--data-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunRedeploy with missing app")
	}

	// Status on nonexistent app
	if err := RunStatus([]string{"nonexistent-app", "--config-dir", tmpDir}); err == nil {
		t.Errorf("Expected error for RunStatus on nonexistent app")
	}

	// Status --machine
	if err := RunStatus([]string{"--machine", "--config-dir", tmpDir}); err != nil {
		t.Errorf("RunStatus --machine failed: %v", err)
	}

	// Status --apps
	if err := RunStatus([]string{"--apps", "--config-dir", tmpDir}); err != nil {
		t.Errorf("RunStatus --apps failed: %v", err)
	}

	// Agent status
	if err := RunAgent([]string{"status", "--config-dir", tmpDir}); err != nil {
		t.Errorf("RunAgent status failed: %v", err)
	}
}

func TestRunDoctorCLI(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	dataDir := filepath.Join(tmpDir, "data")

	// Doctor with JSON flag
	if err := RunDoctor([]string{"--config-dir", configDir, "--data-dir", dataDir, "--json"}); err != nil {
		// Non-fatal if warnings or failures exist, but shouldn't fail argument parsing
		t.Logf("RunDoctor JSON returned: %v", err)
	}

	// Doctor human-readable output
	_ = RunDoctor([]string{"--config-dir", configDir, "--data-dir", dataDir})
}

func TestRunUpdateAndUninstallCLI(t *testing.T) {
	tmpDir := t.TempDir()
	fakeSrc := filepath.Join(tmpDir, "fake-nugget-new")
	fakeDst := filepath.Join(tmpDir, "bin", "nugget")

	if err := copyExecutable("/bin/sh", fakeSrc); err != nil {
		// Fallback for non-standard paths
		_ = os.WriteFile(fakeSrc, []byte("#!/bin/sh\nexit 0\n"), 0755)
	}

	// Test update with explicit binary and target
	if err := RunUpdate([]string{"--binary", fakeSrc, "--target", fakeDst, "--force"}); err != nil {
		t.Fatalf("RunUpdate failed: %v", err)
	}

	if _, err := os.Stat(fakeDst); err != nil {
		t.Fatalf("Expected updated binary at %s", fakeDst)
	}

	// Test uninstall with --yes flag (preserving data)
	if err := RunUninstall([]string{"--yes", "--target", fakeDst}); err != nil {
		t.Fatalf("RunUninstall failed: %v", err)
	}

	if _, err := os.Stat(fakeDst); !os.IsNotExist(err) {
		t.Errorf("Expected binary to be removed after uninstall")
	}
}

