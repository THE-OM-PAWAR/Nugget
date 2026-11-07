package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	DefaultServiceName = "nugget"
	DefaultUnitPath    = "/etc/systemd/system/nugget.service"
)

var unitTemplate = `[Unit]
Description=Nugget Agent - Host Deployment Daemon
Documentation=https://github.com/nugget-sh/nugget
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart={{.BinaryPath}} agent daemon{{if .ConfigDirArg}} --config-dir {{.ConfigDirArg}}{{end}}
Restart=always
RestartSec=5s
KillMode=process
StandardOutput=journal
StandardError=journal
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`

// UnitParams holds values injected into the systemd unit file.
type UnitParams struct {
	BinaryPath   string
	ConfigDirArg string
}

// ServiceStatus details the current state of the background systemd service.
type ServiceStatus struct {
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	State     string `json:"state"`
	Enabled   bool   `json:"enabled"`
	Details   string `json:"details,omitempty"`
}

// Manager controls the lifecycle of the systemd service.
type Manager struct {
	UnitPath    string
	ServiceName string
}

// NewManager creates a systemd service manager with default paths.
func NewManager() *Manager {
	return &Manager{
		UnitPath:    DefaultUnitPath,
		ServiceName: DefaultServiceName,
	}
}

// IsSystemdAvailable checks if systemd is active on the host.
func IsSystemdAvailable() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// GenerateUnitFile renders the systemd unit file contents.
func GenerateUnitFile(binaryPath, configDir string) (string, error) {
	tmpl, err := template.New("systemd").Parse(unitTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse unit template: %w", err)
	}

	var buf bytes.Buffer
	params := UnitParams{
		BinaryPath: binaryPath,
	}
	if configDir != "" && configDir != "/etc/nugget" {
		params.ConfigDirArg = configDir
	}

	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to render unit template: %w", err)
	}

	return buf.String(), nil
}

// Install writes the systemd unit file, reloads systemd, and enables the service.
func (m *Manager) Install(binaryPath, configDir string) error {
	if !IsSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}

	unitContent, err := GenerateUnitFile(binaryPath, configDir)
	if err != nil {
		return err
	}

	// Ensure parent dir exists
	dir := filepath.Dir(m.UnitPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd unit directory %s: %w", dir, err)
	}

	if err := os.WriteFile(m.UnitPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("failed to write unit file %s: %w", m.UnitPath, err)
	}

	// Reload daemon
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	// Enable service for boot startup
	if out, err := exec.Command("systemctl", "enable", m.ServiceName).CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable %s failed: %s (%w)", m.ServiceName, strings.TrimSpace(string(out)), err)
	}

	return nil
}

// Start starts the systemd service.
func (m *Manager) Start() error {
	if !IsSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}
	out, err := exec.Command("systemctl", "start", m.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start %s: %s (%w)", m.ServiceName, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Stop stops the systemd service.
func (m *Manager) Stop() error {
	if !IsSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}
	out, err := exec.Command("systemctl", "stop", m.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop %s: %s (%w)", m.ServiceName, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Restart restarts the systemd service.
func (m *Manager) Restart() error {
	if !IsSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}
	out, err := exec.Command("systemctl", "restart", m.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart %s: %s (%w)", m.ServiceName, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Uninstall stops, disables, and removes the systemd unit file.
func (m *Manager) Uninstall() error {
	if !IsSystemdAvailable() {
		// Just remove unit file if systemd is unavailable
		if _, err := os.Stat(m.UnitPath); err == nil {
			return os.Remove(m.UnitPath)
		}
		return nil
	}

	// Stop service if active
	_ = exec.Command("systemctl", "stop", m.ServiceName).Run()

	// Disable service if enabled
	_ = exec.Command("systemctl", "disable", m.ServiceName).Run()

	// Remove unit file
	if err := os.Remove(m.UnitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service unit file %s: %w", m.UnitPath, err)
	}

	// Reload systemd daemon
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// Status checks the status of the systemd service.
func (m *Manager) Status() (*ServiceStatus, error) {
	status := &ServiceStatus{
		Installed: false,
		Active:    false,
		State:     "not-installed",
		Enabled:   false,
	}

	if _, err := os.Stat(m.UnitPath); err != nil {
		return status, nil
	}
	status.Installed = true

	if !IsSystemdAvailable() {
		status.State = "installed (systemd unavailable)"
		return status, nil
	}

	// Check is-active
	out, err := exec.Command("systemctl", "is-active", m.ServiceName).CombinedOutput()
	state := strings.TrimSpace(string(out))
	status.State = state
	status.Active = (err == nil && state == "active")

	// Check is-enabled
	outEnabled, _ := exec.Command("systemctl", "is-enabled", m.ServiceName).CombinedOutput()
	status.Enabled = (strings.TrimSpace(string(outEnabled)) == "enabled")

	return status, nil
}

