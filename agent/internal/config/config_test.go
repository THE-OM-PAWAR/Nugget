package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/nugget-sh/nugget/agent/internal/sysinfo"
)

func TestGenerateMachineID(t *testing.T) {
	id, err := GenerateMachineID()
	if err != nil {
		t.Fatalf("GenerateMachineID failed: %v", err)
	}

	// Validate UUID v4 regex: 8-4-4-4-12 hex with version 4 and variant 8,9,a,b
	uuidV4Regex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV4Regex.MatchString(id) {
		t.Errorf("GenerateMachineID() = %s; does not match UUID v4 format", id)
	}

	// Verify randomness across multiple calls
	id2, err := GenerateMachineID()
	if err != nil {
		t.Fatalf("GenerateMachineID second call failed: %v", err)
	}
	if id == id2 {
		t.Errorf("GenerateMachineID generated duplicate IDs: %s", id)
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		MachineID:   "test-machine-id-1234",
		MachineName: "ubuntu-server-prod",
		System: &sysinfo.SystemInfo{
			OS:       "Ubuntu 24.04 LTS",
			Arch:     "amd64",
			Hostname: "ubuntu-server",
			CPUCount: 4,
		},
	}

	// Save with overwrite=false (first time)
	if err := Save(tmpDir, cfg, false); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if !Exists(tmpDir) {
		t.Fatalf("Expected config file to exist at %s", tmpDir)
	}

	// Verify permissions (0600)
	fi, err := os.Stat(ConfigFilePath(tmpDir))
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("Expected config file permissions 0600, got %o", perm)
	}

	// Load and verify fields
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.MachineID != cfg.MachineID {
		t.Errorf("Expected MachineID %s, got %s", cfg.MachineID, loaded.MachineID)
	}
	if loaded.MachineName != cfg.MachineName {
		t.Errorf("Expected MachineName %s, got %s", cfg.MachineName, loaded.MachineName)
	}
	if loaded.System == nil || loaded.System.OS != "Ubuntu 24.04 LTS" {
		t.Errorf("Expected loaded system OS Ubuntu 24.04 LTS, got %+v", loaded.System)
	}
	if loaded.CreatedAt.IsZero() {
		t.Errorf("Expected CreatedAt to be set")
	}

	// Test accidental overwrite prevention
	err = Save(tmpDir, cfg, false)
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Errorf("Expected ErrAlreadyInitialized on second save without overwrite, got: %v", err)
	}

	// Test intentional overwrite
	cfg.MachineName = "updated-machine-name"
	if err := Save(tmpDir, cfg, true); err != nil {
		t.Fatalf("Save with overwrite=true failed: %v", err)
	}

	reloaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if reloaded.MachineName != "updated-machine-name" {
		t.Errorf("Expected updated MachineName, got %s", reloaded.MachineName)
	}
}

func TestDefaultConfigDir(t *testing.T) {
	customDir := filepath.Join(os.TempDir(), "custom-nugget-config")
	t.Setenv("NUGGET_CONFIG_DIR", customDir)

	if dir := DefaultConfigDir(); dir != customDir {
		t.Errorf("Expected DefaultConfigDir() = %s, got %s", customDir, dir)
	}
}

