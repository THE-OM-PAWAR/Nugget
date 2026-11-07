package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DeploymentStage tracks the lifecycle status of a deployment.
type DeploymentStage string

const (
	StagePending   DeploymentStage = "pending"
	StageCloning   DeploymentStage = "cloning"
	StageBuilding  DeploymentStage = "building"
	StageStarting  DeploymentStage = "starting"
	StageSucceeded DeploymentStage = "succeeded"
	StageFailed    DeploymentStage = "failed"
)

// Application stores the state and active deployment of a deployed application.
type Application struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	RepoURL            string            `json:"repo_url"`
	Branch             string            `json:"branch"`
	Port               int               `json:"port"`
	Status             string            `json:"status,omitempty"`
	PublicURL          string            `json:"public_url,omitempty"`
	Dockerfile         string            `json:"dockerfile,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	ActiveDeploymentID string            `json:"active_deployment_id,omitempty"`
	ActiveContainerID  string            `json:"active_container_id,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// Deployment records the execution history, logs, and state of a single deployment.
type Deployment struct {
	ID          string          `json:"id"`
	AppID       string          `json:"app_id"`
	AppName     string          `json:"app_name"`
	RepoURL     string          `json:"repo_url"`
	Branch      string          `json:"branch"`
	ImageTag    string          `json:"image_tag,omitempty"`
	ContainerID string          `json:"container_id,omitempty"`
	Port        int             `json:"port"`
	Status      DeploymentStage `json:"status"`
	Error       string          `json:"error,omitempty"`
	Log         string          `json:"log,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at,omitempty"`
}

// Store handles persistence of applications and deployments on the local filesystem.
type Store struct {
	DataDir string
	mu      sync.Mutex
}

// NewStore initializes a new Store rooted at dataDir.
func NewStore(dataDir string) *Store {
	return &Store{DataDir: dataDir}
}

func (s *Store) appsDir() string {
	return filepath.Join(s.DataDir, "apps")
}

func (s *Store) deploymentsDir() string {
	return filepath.Join(s.DataDir, "deployments")
}

// GenerateID produces a random identifier with the given prefix (e.g. "app_4f8a2c", "dep_9b1e3d").
func GenerateID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// SaveApplication writes an application record atomically.
func (s *Store) SaveApplication(app *Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if app.ID == "" {
		return errors.New("application ID is required")
	}

	dir := filepath.Join(s.appsDir(), app.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	now := time.Now().UTC()
	if app.CreatedAt.IsZero() {
		app.CreatedAt = now
	}
	app.UpdatedAt = now

	return atomicWriteJSON(filepath.Join(dir, "app.json"), app)
}

// GetApplication retrieves an application by ID or by Name.
func (s *Store) GetApplication(idOrName string) (*Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return nil, errors.New("application id or name required")
	}

	// Try direct by ID
	directPath := filepath.Join(s.appsDir(), idOrName, "app.json")
	if app, err := readAppFile(directPath); err == nil {
		return app, nil
	}

	// Try scan by name
	apps, err := s.listApplicationsUnlocked()
	if err != nil {
		return nil, err
	}
	for _, app := range apps {
		if strings.EqualFold(app.Name, idOrName) {
			return app, nil
		}
	}

	return nil, fmt.Errorf("application '%s' not found", idOrName)
}

// RemoveApplication deletes the application record and associated source files.
func (s *Store) RemoveApplication(idOrName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return errors.New("application id or name required")
	}

	var appID string
	// Check direct by ID
	directPath := filepath.Join(s.appsDir(), idOrName, "app.json")
	if _, err := os.Stat(directPath); err == nil {
		appID = idOrName
	} else {
		// Scan by name
		apps, err := s.listApplicationsUnlocked()
		if err != nil {
			return err
		}
		for _, app := range apps {
			if strings.EqualFold(app.Name, idOrName) {
				appID = app.ID
				break
			}
		}
	}

	if appID == "" {
		return fmt.Errorf("application '%s' not found", idOrName)
	}

	appDir := filepath.Join(s.appsDir(), appID)
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("failed to remove application directory %s: %w", appDir, err)
	}

	return nil
}

// ListApplications returns all stored applications sorted by UpdatedAt descending.
func (s *Store) ListApplications() ([]*Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listApplicationsUnlocked()
}

func (s *Store) listApplicationsUnlocked() ([]*Application, error) {
	dir := s.appsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var apps []*Application
	for _, entry := range entries {
		if entry.IsDir() {
			appPath := filepath.Join(dir, entry.Name(), "app.json")
			if app, err := readAppFile(appPath); err == nil {
				apps = append(apps, app)
			}
		}
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].UpdatedAt.After(apps[j].UpdatedAt)
	})

	return apps, nil
}

// SaveDeployment writes a deployment record atomically.
func (s *Store) SaveDeployment(dep *Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dep.ID == "" {
		return errors.New("deployment ID is required")
	}

	dir := s.deploymentsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create deployments directory: %w", err)
	}

	if dep.StartedAt.IsZero() {
		dep.StartedAt = time.Now().UTC()
	}

	return atomicWriteJSON(filepath.Join(dir, fmt.Sprintf("%s.json", dep.ID)), dep)
}

// GetDeployment reads a deployment by ID.
func (s *Store) GetDeployment(id string) (*Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.deploymentsDir(), fmt.Sprintf("%s.json", id))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("deployment %s not found: %w", id, err)
	}

	var dep Deployment
	if err := json.Unmarshal(data, &dep); err != nil {
		return nil, fmt.Errorf("failed to parse deployment %s: %w", id, err)
	}
	return &dep, nil
}

// ListDeployments returns deployments for an application, sorted by StartedAt descending.
func (s *Store) ListDeployments(appID string) ([]*Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.deploymentsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var deps []*Deployment
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err == nil {
				var dep Deployment
				if json.Unmarshal(data, &dep) == nil {
					if appID == "" || dep.AppID == appID {
						deps = append(deps, &dep)
					}
				}
			}
		}
	}

	sort.Slice(deps, func(i, j int) bool {
		return deps[i].StartedAt.After(deps[j].StartedAt)
	})

	return deps, nil
}

func readAppFile(path string) (*Application, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var app Application
	if err := json.Unmarshal(data, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// atomicWriteJSON serializes v to JSON and writes it atomically using a temporary file.
func atomicWriteJSON(targetPath string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	_ = os.Chmod(tmpName, 0644)

	if err := os.Rename(tmpName, targetPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to atomic rename %s -> %s: %w", tmpName, targetPath, err)
	}

	return nil
}

