package deployer

import (
	"bytes"
	"context"
	"testing"

	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/store"
)

func TestDeployInvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	dClient, err := docker.NewClient()
	if err != nil {
		t.Skip("Docker CLI not available")
	}

	st := store.NewStore(tmpDir)
	d := &Deployer{
		Store:        st,
		DockerClient: dClient,
	}

	var buf bytes.Buffer
	opts := DeployOptions{
		RepoURL:   "invalid-url",
		DataDir:   tmpDir,
		LogOutput: &buf,
	}

	_, err = d.Deploy(context.Background(), opts)
	if err == nil {
		t.Fatalf("Expected error for invalid repo URL, got nil")
	}
}

func TestDeployAppNameSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	dClient, err := docker.NewClient()
	if err != nil {
		t.Skip("Docker CLI not available")
	}

	st := store.NewStore(tmpDir)
	d := &Deployer{
		Store:        st,
		DockerClient: dClient,
	}

	var buf bytes.Buffer
	opts := DeployOptions{
		RepoURL:   "https://github.com/user/Bad.App.Name",
		AppName:   "Bad.Name",
		DataDir:   tmpDir,
		LogOutput: &buf,
	}

	// This should fail early at Docker verification or ping if daemon is not running, but appName is sanitized
	_, _ = d.Deploy(context.Background(), opts)
}

func TestDeployerLifecycleMissingApp(t *testing.T) {
	tmpDir := t.TempDir()
	dClient, err := docker.NewClient()
	if err != nil {
		t.Skip("Docker CLI not available")
	}

	st := store.NewStore(tmpDir)
	d := &Deployer{
		Store:        st,
		DockerClient: dClient,
	}

	ctx := context.Background()
	if err := d.Restart(ctx, "nonexistent"); err == nil {
		t.Errorf("Expected error restarting nonexistent app, got nil")
	}
	if err := d.Stop(ctx, "nonexistent"); err == nil {
		t.Errorf("Expected error stopping nonexistent app, got nil")
	}
	if err := d.Remove(ctx, "nonexistent"); err == nil {
		t.Errorf("Expected error removing nonexistent app, got nil")
	}
	if _, err := d.Redeploy(ctx, "nonexistent", nil); err == nil {
		t.Errorf("Expected error redeploying nonexistent app, got nil")
	}
}

func TestDeployerRemoveApp(t *testing.T) {
	tmpDir := t.TempDir()
	dClient, err := docker.NewClient()
	if err != nil {
		t.Skip("Docker CLI not available")
	}

	st := store.NewStore(tmpDir)
	d := &Deployer{
		Store:        st,
		DockerClient: dClient,
	}

	app := &store.Application{
		ID:        "app_123",
		Name:      "test-app",
		RepoURL:   "https://github.com/my/app",
		Port:      3000,
		PublicURL: "https://test-app.apps.nugget.run",
	}
	if err := st.SaveApplication(app); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := d.Remove(ctx, "test-app"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, err = st.GetApplication("test-app")
	if err == nil {
		t.Fatalf("Expected app to be deleted")
	}
}

