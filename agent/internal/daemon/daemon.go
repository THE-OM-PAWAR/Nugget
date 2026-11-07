package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/health"
	"github.com/nugget-sh/nugget/agent/internal/monitor"
	"github.com/nugget-sh/nugget/agent/internal/proxy"
	"github.com/nugget-sh/nugget/agent/internal/store"
)

const (
	DefaultMonitorInterval = 15 * time.Second
	MaxAutoRestartLimit    = 5
)

// Daemon coordinates continuous host monitoring, application health checks, and failure recovery.
type Daemon struct {
	ConfigDir    string
	DataDir      string
	Interval     time.Duration
	BaseDomain   string
	Store        *store.Store
	DockerClient *docker.Client
	Logger       *Logger
	Lock         *Lock

	crashCounts map[string]int
	mu          sync.Mutex
}

// NewDaemon creates a new daemon instance.
func NewDaemon(configDir, dataDir string, interval time.Duration) (*Daemon, error) {
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}
	if dataDir == "" {
		dataDir = config.DefaultDataDir(configDir)
	}
	if interval <= 0 {
		interval = DefaultMonitorInterval
	}

	dClient, _ := docker.NewClient()
	st := store.NewStore(dataDir)

	return &Daemon{
		ConfigDir:    configDir,
		DataDir:      dataDir,
		Interval:     interval,
		BaseDomain:   proxy.DefaultBaseDomain,
		Store:        st,
		DockerClient: dClient,
		crashCounts:  make(map[string]int),
	}, nil
}

// Run starts the daemon loop until context is canceled.
func (d *Daemon) Run(ctx context.Context) error {
	// 1. Acquire single-instance process lock
	lock, err := AcquireLock(d.DataDir)
	if err != nil {
		return err
	}
	d.Lock = lock
	defer d.Lock.Release()

	// 2. Initialize structured logging
	logDir := filepath.Join(d.DataDir, "logs")
	logger, err := NewLogger(logDir, true)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	d.Logger = logger
	defer d.Logger.Close()

	d.Logger.Infof("Nugget Agent daemon started (PID: %d)", os.Getpid())
	d.Logger.Infof("Configuration directory: %s | Data directory: %s", d.ConfigDir, d.DataDir)

	// 3. Reconcile interrupted states from previous unexpected shutdown
	d.reconcileInterruptedDeployments()

	// 4. Initial reconciliation of applications and proxy
	d.reconcileInfrastructure(ctx)

	// 5. Initial metrics and health check
	d.tick(ctx)

	// 6. Main monitoring and recovery loop
	ticker := time.NewTicker(d.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.Logger.Infof("Shutdown signal received. Stopping Nugget Agent daemon gracefully...")
			return nil
		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

func (d *Daemon) tick(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. Collect and persist machine metrics
	mMetrics, err := monitor.CollectMachineMetrics()
	if err == nil {
		metricsPath := filepath.Join(d.DataDir, "machine_metrics.json")
		if data, err := json.MarshalIndent(mMetrics, "", "  "); err == nil {
			_ = os.WriteFile(metricsPath, append(data, '\n'), 0644)
		}
	}

	// 2. Check and recover applications
	apps, err := d.Store.ListApplications()
	if err != nil {
		d.Logger.Warnf("Failed to list applications: %v", err)
		return
	}

	appMetrics := monitor.CollectAllApps(ctx, d.DockerClient, apps)
	appsMetricsPath := filepath.Join(d.DataDir, "apps_metrics.json")
	if data, err := json.MarshalIndent(appMetrics, "", "  "); err == nil {
		_ = os.WriteFile(appsMetricsPath, append(data, '\n'), 0644)
	}

	// 3. Application recovery
	for _, m := range appMetrics {
		if m.Status == string(health.StatusCrashLoop) {
			d.Logger.Warnf("Application '%s' is in crash loop (restart count: %d). Skipping automatic restarts to prevent loop.", m.AppName, m.RestartCount)
			continue
		}

		if m.Status == string(health.StatusStopped) && m.ContainerID != "" {
			count := d.crashCounts[m.AppName]
			if count < MaxAutoRestartLimit {
				d.crashCounts[m.AppName] = count + 1
				d.Logger.Warnf("Application '%s' container unexpectedly stopped. Attempting recovery (%d/%d)...", m.AppName, count+1, MaxAutoRestartLimit)
				if d.DockerClient != nil {
					if err := d.DockerClient.RestartContainer(ctx, m.ContainerID); err != nil {
						d.Logger.Errorf("Failed to restart '%s': %v", m.AppName, err)
					} else {
						d.Logger.Infof("Application '%s' successfully recovered.", m.AppName)
					}
				}
			} else {
				d.Logger.Errorf("Application '%s' reached maximum recovery limit (%d). Manual intervention required.", m.AppName, MaxAutoRestartLimit)
			}
		} else if m.Status == string(health.StatusRunning) {
			// Reset crash count on healthy run
			d.crashCounts[m.AppName] = 0
		}
	}
}

// reconcileInterruptedDeployments flags incomplete deployments from an unexpected shutdown.
func (d *Daemon) reconcileInterruptedDeployments() {
	deps, err := d.Store.ListDeployments("")
	if err != nil {
		return
	}

	for _, dep := range deps {
		if dep.Status == store.StagePending || dep.Status == store.StageCloning || dep.Status == store.StageBuilding || dep.Status == store.StageStarting {
			d.Logger.Warnf("Recovering interrupted deployment %s for '%s'...", dep.ID, dep.AppName)
			dep.Status = store.StageFailed
			dep.Error = "deployment interrupted by agent termination or machine reboot"
			dep.CompletedAt = time.Now().UTC()
			_ = d.Store.SaveDeployment(dep)
		}
	}
}

// reconcileInfrastructure ensures the reverse proxy and network are restored after reboot.
func (d *Daemon) reconcileInfrastructure(ctx context.Context) {
	if d.DockerClient == nil {
		return
	}

	if err := d.DockerClient.Ping(ctx); err != nil {
		d.Logger.Warnf("Docker daemon is not currently available: %v. Will retry on next interval.", err)
		return
	}

	// Ensure Traefik is running
	if err := proxy.EnsureProxyRunning(ctx, d.DockerClient, d.DataDir, d.BaseDomain); err != nil {
		d.Logger.Warnf("Failed to ensure Traefik reverse proxy: %v", err)
	} else {
		d.Logger.Infof("Traefik reverse proxy verified on %s.", proxy.DefaultNetwork)
	}
}

