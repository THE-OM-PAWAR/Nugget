package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	content := `# Database config
PORT=8080
DATABASE_URL="postgres://user:pass@localhost:5432/db"
NODE_ENV='production'
# Commented line
EMPTY=
   SPACED_KEY = value with spaces  
`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	env, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatalf("ParseEnvFile failed: %v", err)
	}

	if env["PORT"] != "8080" {
		t.Errorf("Expected PORT=8080, got %s", env["PORT"])
	}
	if env["DATABASE_URL"] != "postgres://user:pass@localhost:5432/db" {
		t.Errorf("Expected stripped quotes for DATABASE_URL, got %s", env["DATABASE_URL"])
	}
	if env["NODE_ENV"] != "production" {
		t.Errorf("Expected stripped single quotes for NODE_ENV, got %s", env["NODE_ENV"])
	}
	if env["SPACED_KEY"] != "value with spaces" {
		t.Errorf("Expected trimmed SPACED_KEY, got %s", env["SPACED_KEY"])
	}
}

func TestPortAvailability(t *testing.T) {
	port, err := FindAvailablePort(34567)
	if err != nil {
		t.Fatalf("FindAvailablePort failed: %v", err)
	}
	if port < 34567 {
		t.Errorf("Expected port >= 34567, got %d", port)
	}
	if !IsPortAvailable(port) {
		t.Errorf("Expected port %d to be available", port)
	}
}

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.DockerPath == "" {
		t.Errorf("Expected non-empty DockerPath")
	}
}

