package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreApplications(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)

	app := &Application{
		ID:      "app_test01",
		Name:    "frontend-web",
		RepoURL: "https://github.com/my-org/web",
		Branch:  "main",
		Port:    3000,
	}

	if err := s.SaveApplication(app); err != nil {
		t.Fatalf("SaveApplication failed: %v", err)
	}

	// Fetch by ID
	byID, err := s.GetApplication("app_test01")
	if err != nil {
		t.Fatalf("GetApplication by ID failed: %v", err)
	}
	if byID.Name != "frontend-web" {
		t.Errorf("Expected name 'frontend-web', got %s", byID.Name)
	}

	// Fetch by Name (case-insensitive)
	byName, err := s.GetApplication("Frontend-Web")
	if err != nil {
		t.Fatalf("GetApplication by Name failed: %v", err)
	}
	if byName.ID != "app_test01" {
		t.Errorf("Expected ID 'app_test01', got %s", byName.ID)
	}

	// Add second app and test listing
	app2 := &Application{
		ID:      "app_test02",
		Name:    "backend-api",
		RepoURL: "https://github.com/my-org/api",
		Branch:  "dev",
		Port:    8080,
	}
	if err := s.SaveApplication(app2); err != nil {
		t.Fatalf("SaveApplication 2 failed: %v", err)
	}

	list, err := s.ListApplications()
	if err != nil {
		t.Fatalf("ListApplications failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("Expected 2 applications, got %d", len(list))
	}

	// Remove by Name
	if err := s.RemoveApplication("frontend-web"); err != nil {
		t.Fatalf("RemoveApplication failed: %v", err)
	}

	remaining, err := s.ListApplications()
	if err != nil {
		t.Fatalf("ListApplications after remove failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("Expected 1 application remaining, got %d", len(remaining))
	}
}

func TestStoreDeployments(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)

	dep1 := &Deployment{
		ID:        "dep_01",
		AppID:     "app_1",
		AppName:   "web",
		RepoURL:   "https://github.com/org/web",
		Branch:    "main",
		Status:    StageSucceeded,
		Port:      3000,
		StartedAt: time.Now().Add(-10 * time.Minute),
	}

	dep2 := &Deployment{
		ID:        "dep_02",
		AppID:     "app_1",
		AppName:   "web",
		RepoURL:   "https://github.com/org/web",
		Branch:    "main",
		Status:    StageFailed,
		Error:     "build failed",
		Port:      3000,
		StartedAt: time.Now(),
	}

	if err := s.SaveDeployment(dep1); err != nil {
		t.Fatalf("SaveDeployment 1 failed: %v", err)
	}
	if err := s.SaveDeployment(dep2); err != nil {
		t.Fatalf("SaveDeployment 2 failed: %v", err)
	}

	loaded, err := s.GetDeployment("dep_01")
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if loaded.Status != StageSucceeded {
		t.Errorf("Expected status %s, got %s", StageSucceeded, loaded.Status)
	}

	deps, err := s.ListDeployments("app_1")
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("Expected 2 deployments, got %d", len(deps))
	}
	// Verify sorted by StartedAt descending (dep2 was more recent)
	if deps[0].ID != "dep_02" {
		t.Errorf("Expected most recent deployment first (dep_02), got %s", deps[0].ID)
	}
}

func TestGenerateID(t *testing.T) {
	id := GenerateID("app")
	if len(id) < 8 || filepath.Base(id) != id {
		t.Errorf("Invalid generated ID: %s", id)
	}
}

