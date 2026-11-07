package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/daemon"
	"github.com/nugget-sh/nugget/agent/internal/sysinfo"
)

func TestRunDiagnostics_EmptyDirs(t *testing.T) {
	tempConfig := t.TempDir()
	tempData := t.TempDir()

	report := RunDiagnostics(tempConfig, tempData)
	if report == nil {
		t.Fatal("expected non-nil DoctorReport")
	}

	total := report.PassCount + report.WarnCount + report.FailCount
	if total != len(report.Results) {
		t.Errorf("tally mismatch: sum=%d, len=%d", total, len(report.Results))
	}

	// Should have results in multiple categories
	categories := make(map[string]int)
	for _, res := range report.Results {
		categories[res.Category]++
	}

	expectedCategories := []string{
		"Privileges",
		"Configuration",
		"Storage",
		"Daemon",
		"Service",
		"Docker",
		"Resources",
	}

	for _, cat := range expectedCategories {
		if categories[cat] == 0 {
			t.Errorf("expected diagnostic results in category %q, found none", cat)
		}
	}
}

func TestRunDiagnostics_WithValidConfig(t *testing.T) {
	tempConfig := t.TempDir()
	tempData := t.TempDir()

	cfg := &config.Config{
		MachineID:   "test-machine-id-12345",
		MachineName: "production-node-1",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		System: &sysinfo.SystemInfo{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	if err := config.Save(tempConfig, cfg, true); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	report := RunDiagnostics(tempConfig, tempData)
	if report == nil {
		t.Fatal("expected non-nil DoctorReport")
	}

	var foundConfigCheck bool
	for _, res := range report.Results {
		if res.Category == "Configuration" && res.Name == "Machine Identity" {
			foundConfigCheck = true
			if res.Level != LevelPass {
				t.Errorf("expected LevelPass for valid config, got %s: %s", res.Level, res.Message)
			}
		}
	}

	if !foundConfigCheck {
		t.Error("expected Machine Identity check in results")
	}
}

func TestRunDiagnostics_WithCorruptedConfig(t *testing.T) {
	tempConfig := t.TempDir()
	tempData := t.TempDir()

	corruptFile := filepath.Join(tempConfig, "config.json")
	if err := os.WriteFile(corruptFile, []byte("{invalid-json}"), 0600); err != nil {
		t.Fatalf("failed to write corrupted config: %v", err)
	}

	report := RunDiagnostics(tempConfig, tempData)
	var foundCorruptCheck bool
	for _, res := range report.Results {
		if res.Category == "Configuration" && res.Name == "Config File" {
			foundCorruptCheck = true
			if res.Level != LevelFail {
				t.Errorf("expected LevelFail for corrupt config, got %s", res.Level)
			}
		}
	}

	if !foundCorruptCheck {
		t.Error("expected Config File check failure in results")
	}
}

func TestRunDiagnostics_WithDaemonPID(t *testing.T) {
	tempConfig := t.TempDir()
	tempData := t.TempDir()

	// Acquire lock on tempData to simulate active daemon
	lock, err := daemon.AcquireLock(tempData)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer lock.Release()

	report := RunDiagnostics(tempConfig, tempData)
	var foundDaemonCheck bool
	for _, res := range report.Results {
		if res.Category == "Daemon" && res.Name == "Process State" {
			foundDaemonCheck = true
			if res.Level != LevelPass {
				t.Errorf("expected LevelPass for active daemon lock, got %s: %s", res.Level, res.Message)
			}
		}
	}

	if !foundDaemonCheck {
		t.Error("expected Process State check in results")
	}
}

func TestDoctorReport_JSONMarshal(t *testing.T) {
	report := &DoctorReport{
		PassCount: 1,
		WarnCount: 1,
		FailCount: 0,
		Results: []CheckResult{
			{
				Category: "Privileges",
				Name:     "User Permissions",
				Level:    LevelPass,
				Message:  "Running with administrative privileges",
			},
			{
				Category: "Storage",
				Name:     "Data Directory",
				Level:    LevelWarn,
				Message:  "Directory created with non-standard permissions",
				Advice:   "Run chmod 755 /var/lib/nugget",
			},
		},
	}

	bytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal DoctorReport to JSON: %v", err)
	}

	var parsed DoctorReport
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DoctorReport from JSON: %v", err)
	}

	if parsed.PassCount != 1 || parsed.WarnCount != 1 || len(parsed.Results) != 2 {
		t.Errorf("unmarshaled report does not match original: %+v", parsed)
	}
}
