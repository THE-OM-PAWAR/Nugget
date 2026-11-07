package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/docker"
)

// Status represents the operational health status of an application.
type Status string

const (
	StatusRunning   Status = "running"
	StatusStopped   Status = "stopped"
	StatusUnhealthy Status = "unhealthy"
	StatusCrashLoop Status = "crash_loop"
	StatusUnknown   Status = "unknown"
)

// AppHealth details the runtime health evaluation of an application container.
type AppHealth struct {
	Status          Status    `json:"status"`
	ContainerStatus string    `json:"container_status"`
	RestartCount    int       `json:"restart_count"`
	ExitCode        int       `json:"exit_code"`
	Responsive      bool      `json:"responsive"`
	Message         string    `json:"message"`
	LastChecked     time.Time `json:"last_checked"`
}

// CheckAppHealth inspects container runtime state and probes port accessibility.
func CheckAppHealth(ctx context.Context, dClient *docker.Client, containerID string, hostPort int) *AppHealth {
	now := time.Now().UTC()
	if containerID == "" {
		return &AppHealth{
			Status:          StatusStopped,
			ContainerStatus: "none",
			Message:         "No active container assigned.",
			LastChecked:     now,
		}
	}

	if dClient == nil {
		return &AppHealth{
			Status:          StatusUnknown,
			ContainerStatus: "unknown",
			Message:         "Docker client unavailable.",
			LastChecked:     now,
		}
	}

	state, err := dClient.InspectContainer(ctx, containerID)
	if err != nil {
		return &AppHealth{
			Status:          StatusStopped,
			ContainerStatus: "not_found",
			Message:         fmt.Sprintf("Container %s not found.", containerID),
			LastChecked:     now,
		}
	}

	health := &AppHealth{
		ContainerStatus: state.State.Status,
		RestartCount:    state.RestartCount,
		ExitCode:        state.State.ExitCode,
		LastChecked:     now,
	}

	// 1. Detect restart loop
	if state.State.Restarting || state.RestartCount >= 5 {
		health.Status = StatusCrashLoop
		health.Message = fmt.Sprintf("Application is crashing repeatedly (restart count: %d).", state.RestartCount)
		return health
	}

	// 2. Stopped or exited container
	if !state.State.Running {
		health.Status = StatusStopped
		if state.State.ExitCode != 0 {
			health.Message = fmt.Sprintf("Container exited with code %d: %s", state.State.ExitCode, state.State.Error)
		} else {
			health.Message = "Container is stopped."
		}
		return health
	}

	// 3. Running: Check port connectivity
	if hostPort > 0 {
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		responsive := probePort(probeCtx, hostPort)
		health.Responsive = responsive
		if responsive {
			health.Status = StatusRunning
			health.Message = "Application container is running and responding."
		} else {
			health.Status = StatusUnhealthy
			health.Message = fmt.Sprintf("Container is running but port %d is not responding.", hostPort)
		}
		return health
	}

	health.Status = StatusRunning
	health.Responsive = true
	health.Message = "Application container is running."
	return health
}

func probePort(ctx context.Context, port int) bool {
	// First try quick HTTP GET
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err == nil {
		client := &http.Client{Timeout: 1500 * time.Millisecond}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return true
		}
	}

	// Fallback to TCP dial
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		_ = conn.Close()
		return true
	}

	return false
}

