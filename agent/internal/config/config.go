package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/sysinfo"
)

var (
	// ErrAlreadyInitialized is returned when configuration already exists and overwrite is false.
	ErrAlreadyInitialized = errors.New("nugget agent is already initialized on this machine")
)

// Config represents the persisted identity and configuration of the Nugget agent.
type Config struct {
	MachineID   string              `json:"machine_id"`
	MachineName string              `json:"machine_name"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	System      *sysinfo.SystemInfo `json:"system,omitempty"`

	// Future dashboard communication placeholders
	ServerURL  string `json:"server_url,omitempty"`
	AgentToken string `json:"agent_token,omitempty"`
	Port       string `json:"port,omitempty"`
	LogLevel   string `json:"log_level,omitempty"`
}

// DefaultConfigDir returns the standard configuration directory.
// Uses NUGGET_CONFIG_DIR if set, otherwise /etc/nugget for root/Linux,
// or ~/.config/nugget for non-root local environments.
func DefaultConfigDir() string {
	if dir := os.Getenv("NUGGET_CONFIG_DIR"); dir != "" {
		return dir
	}

	if os.Geteuid() == 0 || runtime.GOOS == "linux" {
		return "/etc/nugget"
	}

	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".config", "nugget")
	}

	return "/etc/nugget"
}

// DefaultDataDir returns the storage directory for applications and deployment artifacts.
// Uses NUGGET_DATA_DIR if set, otherwise /var/lib/nugget for root,
// or filepath.Join(configDir, "data") for non-root / local development.
func DefaultDataDir(configDir string) string {
	if dir := os.Getenv("NUGGET_DATA_DIR"); dir != "" {
		return dir
	}

	if os.Geteuid() == 0 && runtime.GOOS == "linux" {
		return "/var/lib/nugget"
	}

	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	return filepath.Join(configDir, "data")
}

// ConfigFilePath returns the full path to config.json.
func ConfigFilePath(configDir string) string {
	return filepath.Join(configDir, "config.json")
}

// Exists checks whether a configuration file already exists.
func Exists(configDir string) bool {
	path := ConfigFilePath(configDir)
	_, err := os.Stat(path)
	return err == nil
}

// Load loads the configuration from the given directory.
func Load(configDir string) (*Config, error) {
	path := ConfigFilePath(configDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON at %s: %w", path, err)
	}

	return &cfg, nil
}

// Save persists the configuration to disk.
// If overwrite is false and configuration exists, ErrAlreadyInitialized is returned.
func Save(configDir string, cfg *Config, overwrite bool) error {
	if !overwrite && Exists(configDir) {
		return ErrAlreadyInitialized
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	path := ConfigFilePath(configDir)
	// Write with 0600 permissions to protect machine identity & tokens
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}

	return nil
}
