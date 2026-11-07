package monitor

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/health"
	"github.com/nugget-sh/nugget/agent/internal/store"
)

// AppMetrics encapsulates real-time container metrics and health for an application.
type AppMetrics struct {
	AppName       string    `json:"app_name"`
	AppID         string    `json:"app_id"`
	ContainerID   string    `json:"container_id"`
	Status        string    `json:"status"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryBytes   uint64    `json:"memory_bytes"`
	MemoryPercent float64   `json:"memory_percent"`
	RestartCount  int       `json:"restart_count"`
	Responsive    bool      `json:"responsive"`
	PublicURL     string    `json:"public_url"`
	InternalPort  int       `json:"internal_port"`
	Message       string    `json:"message"`
	LastChecked   time.Time `json:"last_checked"`
}

// CollectAllApps evaluates the health and resource consumption of all applications.
func CollectAllApps(ctx context.Context, dClient *docker.Client, apps []*store.Application) []AppMetrics {
	var metrics []AppMetrics
	now := time.Now().UTC()

	for _, app := range apps {
		m := AppMetrics{
			AppName:      app.Name,
			AppID:        app.ID,
			ContainerID:  app.ActiveContainerID,
			PublicURL:    app.PublicURL,
			InternalPort: app.Port,
			LastChecked:  now,
		}

		if app.ActiveContainerID == "" {
			m.Status = "stopped"
			m.Message = "No active container assigned."
			metrics = append(metrics, m)
			continue
		}

		// Health Check
		h := health.CheckAppHealth(ctx, dClient, app.ActiveContainerID, app.Port)
		m.Status = string(h.Status)
		m.RestartCount = h.RestartCount
		m.Responsive = h.Responsive
		m.Message = h.Message

		// If running, query docker stats for CPU & Memory
		if dClient != nil && h.Status == health.StatusRunning {
			cpu, memBytes, memPct := fetchContainerStats(ctx, dClient, app.ActiveContainerID)
			m.CPUPercent = cpu
			m.MemoryBytes = memBytes
			m.MemoryPercent = memPct
		}

		metrics = append(metrics, m)
	}

	return metrics
}

func fetchContainerStats(ctx context.Context, dClient *docker.Client, containerID string) (float64, uint64, float64) {
	statCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(statCtx, dClient.DockerPath, "stats", "--no-stream", "--format", "{{.CPUPerc}}\t{{.MemPerc}}", containerID)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(parts) >= 2 {
		cpuStr := strings.TrimSuffix(parts[0], "%")
		memStr := strings.TrimSuffix(parts[1], "%")
		cpu, _ := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64)
		memPct, _ := strconv.ParseFloat(strings.TrimSpace(memStr), 64)
		return cpu, 0, memPct
	}
	return 0, 0, 0
}

// FormatBytes formats byte counts into human-readable strings.
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

