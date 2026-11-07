package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollect(t *testing.T) {
	info, err := Collect()
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}

	if info.OS == "" {
		t.Errorf("Expected OS to be populated, got empty")
	}
	if info.Arch == "" {
		t.Errorf("Expected Arch to be populated, got empty")
	}
	if info.Hostname == "" {
		t.Errorf("Expected Hostname to be populated, got empty")
	}
	if info.CPUCount <= 0 {
		t.Errorf("Expected CPUCount > 0, got %d", info.CPUCount)
	}
}

func TestParseOSRelease(t *testing.T) {
	tmpDir := t.TempDir()
	osReleaseFile := filepath.Join(tmpDir, "os-release")

	content := `NAME="Ubuntu"
VERSION="24.04 LTS (Noble Numbat)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 24.04 LTS"
VERSION_ID="24.04"
`
	if err := os.WriteFile(osReleaseFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write mock os-release: %v", err)
	}

	prettyName := parseOSRelease(osReleaseFile)
	if prettyName != "Ubuntu 24.04 LTS" {
		t.Errorf("Expected 'Ubuntu 24.04 LTS', got '%s'", prettyName)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{16 * 1024 * 1024 * 1024, "16.0 GB"},
	}

	for _, tt := range tests {
		actual := FormatBytes(tt.bytes)
		if actual != tt.expected {
			t.Errorf("FormatBytes(%d) = %s; want %s", tt.bytes, actual, tt.expected)
		}
	}
}

func TestCheckDocker(t *testing.T) {
	docker := CheckDocker()
	if docker == nil {
		t.Fatal("CheckDocker() returned nil")
	}
	if docker.Message == "" {
		t.Errorf("Expected DockerInfo.Message to not be empty")
	}
}

