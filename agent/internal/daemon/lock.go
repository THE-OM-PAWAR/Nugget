package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Lock manages the single-instance PID lock file.
type Lock struct {
	PIDPath string
}

// AcquireLock creates a PID file or fails if another daemon process is already active.
func AcquireLock(dataDir string) (*Lock, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	pidPath := filepath.Join(dataDir, "nugget.pid")

	if running, pid := isPIDFileActive(pidPath); running {
		return nil, fmt.Errorf("another nugget agent daemon (PID %d) is already running", pid)
	}

	currentPID := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(currentPID)), 0644); err != nil {
		return nil, fmt.Errorf("failed to write PID file %s: %w", pidPath, err)
	}

	return &Lock{PIDPath: pidPath}, nil
}

// Release deletes the PID lock file.
func (l *Lock) Release() {
	if l != nil && l.PIDPath != "" {
		_ = os.Remove(l.PIDPath)
	}
}

// IsDaemonRunning checks whether an active daemon process is running.
func IsDaemonRunning(dataDir string) (bool, int) {
	pidPath := filepath.Join(dataDir, "nugget.pid")
	return isPIDFileActive(pidPath)
}

func isPIDFileActive(pidPath string) (bool, int) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false, 0
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		_ = os.Remove(pidPath)
		return false, 0
	}

	if isProcessAlive(pid) {
		return true, pid
	}

	// Stale PID file from crash or machine reboot
	_ = os.Remove(pidPath)
	return false, 0
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, sending signal 0 checks for process existence without affecting it
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

