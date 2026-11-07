package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateUnitFile(t *testing.T) {
	binaryPath := "/usr/local/bin/nugget"

	// Test default config dir (/etc/nugget) -> no explicit --config-dir needed
	unit, err := GenerateUnitFile(binaryPath, "/etc/nugget")
	if err != nil {
		t.Fatalf("GenerateUnitFile failed: %v", err)
	}

	if !strings.Contains(unit, "ExecStart=/usr/local/bin/nugget agent daemon") {
		t.Errorf("Expected ExecStart with binary path, got:\n%s", unit)
	}
	if !strings.Contains(unit, "Restart=always") {
		t.Errorf("Expected Restart=always in unit")
	}
	if !strings.Contains(unit, "WantedBy=multi-user.target") {
		t.Errorf("Expected WantedBy=multi-user.target in unit")
	}

	// Test custom config dir
	unitCustom, err := GenerateUnitFile(binaryPath, "/custom/dir")
	if err != nil {
		t.Fatalf("GenerateUnitFile failed: %v", err)
	}
	if !strings.Contains(unitCustom, "--config-dir /custom/dir") {
		t.Errorf("Expected --config-dir /custom/dir in unit, got:\n%s", unitCustom)
	}
}

func TestManagerStatusNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	unitPath := filepath.Join(tmpDir, "nugget.service")

	mgr := &Manager{
		UnitPath:    unitPath,
		ServiceName: "nugget",
	}

	status, err := mgr.Status()
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}
	if status.Installed {
		t.Errorf("Expected Installed=false when unit file doesn't exist")
	}
	if status.Active {
		t.Errorf("Expected Active=false")
	}

	// Simulate installed unit file
	if err := os.WriteFile(unitPath, []byte("[Unit]\n"), 0644); err != nil {
		t.Fatalf("Failed to write mock unit file: %v", err)
	}

	statusAfter, err := mgr.Status()
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}
	if !statusAfter.Installed {
		t.Errorf("Expected Installed=true when unit file exists")
	}
}

