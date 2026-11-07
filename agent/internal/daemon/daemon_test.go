package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/store"
)

func TestLockLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	lock1, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("AcquireLock 1 failed: %v", err)
	}

	running, pid := IsDaemonRunning(tmpDir)
	if !running || pid <= 0 {
		t.Errorf("Expected IsDaemonRunning=true, got running=%t, pid=%d", running, pid)
	}

	// Second lock acquisition should fail
	_, err = AcquireLock(tmpDir)
	if err == nil {
		t.Fatalf("Expected error when acquiring duplicate lock, got nil")
	}

	// Release lock
	lock1.Release()

	runningAfter, _ := IsDaemonRunning(tmpDir)
	if runningAfter {
		t.Errorf("Expected IsDaemonRunning=false after release")
	}
}

func TestLoggerWriting(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	logger, err := NewLogger(logDir, false)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	logger.Infof("Application started successfully: %s", "my-app")
	logger.Warnf("Warning message")
	logger.Errorf("Error message")

	// Ensure log was written
	if logger.bytesSent <= 0 {
		t.Errorf("Expected bytesSent > 0, got %d", logger.bytesSent)
	}
}

func TestReconcileInterruptedDeployments(t *testing.T) {
	tmpDir := t.TempDir()
	st := store.NewStore(tmpDir)

	// Create a stuck deployment
	dep := &store.Deployment{
		ID:        "dep_stuck",
		AppID:     "app_1",
		AppName:   "web",
		Status:    store.StageBuilding,
		StartedAt: time.Now().Add(-1 * time.Hour),
	}
	if err := st.SaveDeployment(dep); err != nil {
		t.Fatal(err)
	}

	d, err := NewDaemon(tmpDir, tmpDir, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	d.reconcileInterruptedDeployments()

	recovered, err := st.GetDeployment("dep_stuck")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != store.StageFailed {
		t.Errorf("Expected status StageFailed, got %s", recovered.Status)
	}
	if recovered.Error == "" {
		t.Errorf("Expected non-empty Error explaining interruption")
	}
}

func TestDaemonShortRun(t *testing.T) {
	tmpDir := t.TempDir()
	d, err := NewDaemon(tmpDir, tmpDir, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	if err := d.Run(ctx); err != nil {
		t.Fatalf("Daemon.Run returned error: %v", err)
	}
}

