# Nugget

Nugget is a self-hosted application deployment and server management platform.

This repository contains the **Nugget Agent**, a robust, modular Go CLI capable of:
- **Phase 1: Machine Setup & Daemon**: Initializing host servers, inspecting hardware specifications, Docker engine validation, and managing systemd background daemons without dashboard connection.
- **Phase 2: Application Deployment Engine**: Application deployment directly from public GitHub repositories into isolated Docker containers with automated project detection and local state management.
- **Phase 3: Application Management & Networking**: Reverse proxy host routing (Traefik), Cloudflare Tunnel automatic public HTTPS domains, health checks, and zero-downtime redeployments.
- **Phase 4: Machine Monitoring & Reliability**: Persistent background monitoring, host resource utilization tracking (CPU, Load, RAM, Disk, Uptime, Network), application health checks, crash-loop backoff protection, state recovery after machine reboot, and structured rotating logs.
- **Phase 5: Production Readiness & Finalization**: Diagnostic engine (`nugget doctor`), safe binary updates (`nugget update`), automated uninstaller (`nugget uninstall`), full state preservation, comprehensive end-to-end integration testing, and zero external third-party Go dependencies.

---

## Features

### Phase 1: Machine Setup & Daemon
- **One-Command Installation**: Automated bash installer for Ubuntu on `amd64` and `arm64`.
- **System Detection**: Inspects OS release, kernel, architecture, hostname, CPU cores, RAM, and disk storage.
- **Docker Validation**: Probes Docker engine and daemon status with friendly remediation instructions.
- **Local Machine Identity**: RFC 4122 compliant UUID v4 machine identity stored securely in `/etc/nugget/config.json` (`0600`).
- **Background Service**: Auto-configures, enables, and controls a systemd service (`nugget.service`).

### Phase 2: Application Deployment Engine
- **Direct GitHub Deployment**: Deploy applications directly with `nugget deploy <github-repository-url>`.
- **Project Detection**: Detects Next.js, React / Vite, generic Node.js, and custom or existing Dockerfiles.
- **Secure Container Execution**: Never runs untrusted repository code on the host machine; all builds and runs happen inside Docker containers.
- **Secret Protection**: Injects environment variables via temporary env files with restricted permissions (`0600`), preventing secret exposure in `ps aux` or shell histories.

### Phase 3: Application Management & Networking
- **Full Lifecycle Commands**: `nugget apps`, `logs`, `restart`, `stop`, `remove`, `status <app>`, and `redeploy`.
- **Reverse Proxy Integration (Traefik)**: Automatically routes incoming HTTP traffic by hostname on `nugget-network`.
- **Automatic Public Domains (Cloudflare Tunnel)**: Supports generated subdomains (`https://<app>.apps.nugget.run`) via wildcard DNS routing.

### Phase 4: Machine Monitoring & Reliability
- **Host Resource Monitoring**:
  - Real-time CPU usage % and 1m/5m/15m system load averages.
  - Memory (RAM) total, used, free, and usage percentage.
  - Disk total, used, free, and percentage.
  - Machine uptime and network traffic statistics (Rx/Tx bytes).
  - Exposed via `nugget status --machine`.
- **Application Resource Tracking**:
  - Live container CPU % and Memory % metrics.
  - Exposed via `nugget status --apps` and `nugget apps`.
- **Agent Reliability & Failure Recovery**:
  - Persistent background daemon (`nugget agent daemon`) managed by `systemd`.
  - Single-instance locking via PID file (`nugget.pid`) prevents duplicate daemon processes.
  - State recovery across machine reboots: restores application states and ensures Traefik proxy is active.
  - Interrupted deployment recovery: automatically recovers and logs stuck builds if the process or machine restarted.
- **Crash Loop & Backoff Protection**:
  - Detects repeated application crashes.
  - Limits automatic restart attempts to 5, preventing uncontrolled crash restart loops.
- **Structured Logging & Rotation**:
  - Writes structured logs to `<dataDir>/logs/nugget.log`.
  - Automatic log rotation (10 MB per file, up to 3 archive generations).
  - Never logs credentials, tokens, or environment variables.

### Phase 5: Production Readiness & Finalization
- **Diagnostic Engine (`nugget doctor`)**:
  - Runs 9 comprehensive subsystem checks across host privileges, config file integrity, data directory permissions, daemon lock, systemd service, Docker CLI/daemon/socket connectivity, Traefik proxy network, Cloudflare tunnel token, and disk/RAM thresholds.
  - Supports `--json` for automated script verification and monitoring probes.
- **Safe Agent Upgrades (`nugget update`)**:
  - Atomic binary replacement with rollback protection.
  - Preserves all machine configuration (`/etc/nugget`) and application deployment records (`/var/lib/nugget`).
  - Automatically reloads and restarts running background systemd services.
- **Safe Uninstallation (`nugget uninstall`)**:
  - Stops running daemons, disables and removes `nugget.service` systemd unit file, and removes the binary.
  - Default mode safely preserves configuration and application data; `--purge` flag permanently deletes everything when explicitly requested.
  - Complementary root bash script: `sudo ./scripts/uninstall.sh [--purge] [-y]`.
- **Zero Third-Party Dependencies**: Pure standard library Go architecture guarantees minimal attack surface, fast compilation, and reliable cross-platform execution.
- **End-to-End Test Suite**: Complete automated test coverage for the full agent lifecycle (`agent/internal/e2e`).

---

## Quick Start

### 1. Install Nugget

```bash
sudo ./scripts/install.sh
```

### 2. Initialize the Machine

```bash
sudo nugget init
```

### 3. Verify System Health

```bash
nugget doctor
```

### 4. Deploy an Application

```bash
nugget deploy https://github.com/vercel/next.js/tree/canary/examples/hello-world --name "my-app"
```

---

## CLI Reference

### System Health & Maintenance

#### `nugget doctor`
Runs diagnostic checks across all host requirements, services, and configurations:

```bash
nugget doctor
nugget doctor --json
```

Output highlights:
- `[✓] PASS`: Component is fully operational.
- `[!] WARN`: Non-critical issue or optional feature not configured (e.g. Tunnel token).
- `[✗] FAIL`: Critical blocker requiring immediate remediation.

#### `nugget update`
Safely upgrades the Nugget Agent binary while preserving all configurations and data:

```bash
sudo nugget update
sudo nugget update --binary /path/to/new/nugget
```

#### `nugget uninstall`
Stops background services, deletes the binary, and prompts for data preservation:

```bash
sudo nugget uninstall
sudo nugget uninstall --yes          # Non-interactive, preserves /etc/nugget and /var/lib/nugget
sudo nugget uninstall --purge --yes  # Non-interactive, purges all configuration and data
```

#### `nugget version`
Displays current version, Git commit SHA, build timestamp, Go compiler, and platform architecture:

```bash
nugget version
```

---

### Monitoring & Status

#### `nugget status`
Displays overall host status, system utilization summary, daemon/systemd state, and deployed applications.

```bash
nugget status
nugget status --json
```

#### `nugget status --machine`
Displays detailed host machine resource monitoring metrics (CPU %, Load, RAM, Disk, Uptime, Network):

```bash
nugget status --machine
nugget status --machine --json
```

#### `nugget status --apps`
Displays real-time application health and container resource consumption (CPU %, Memory %, Restarts):

```bash
nugget status --apps
nugget status --apps --json
```

#### `nugget status <app>`
Displays detailed health, port responsiveness, restart counts, and recent deployment history for a specific application:

```bash
nugget status my-app
```

---

### Application Management

#### `nugget apps`
Lists all deployed applications, current health status, internal port, public URL, and container ID.

```bash
nugget apps
```

#### `nugget logs <app>`
Streams or views container logs for an application.

```bash
nugget logs my-app
nugget logs my-app --follow
nugget logs my-app --tail 50
```

#### `nugget restart <app>`
Restarts an application container.

```bash
nugget restart my-app
```

#### `nugget stop <app>`
Stops an application container without deleting its configuration or deployment history.

```bash
nugget stop my-app
```

#### `nugget remove <app>`
Stops the container, removes reverse proxy routing, and cleans up local application metadata.

```bash
nugget remove my-app
```

#### `nugget redeploy <app>`
Pulls latest code from the repository branch, rebuilds the Docker image, and upgrades the application with zero downtime.

```bash
nugget redeploy my-app
```

---

### Agent & Background Daemon

```bash
nugget agent status       # Inspect running daemon process, PID, and service state
sudo nugget agent start   # Start background systemd service
sudo nugget agent stop    # Stop background systemd service
nugget agent daemon       # Run persistent monitoring & recovery loop in the foreground
```

---

## Local Development & Testing

```bash
cd agent

# Run all unit, diagnostic, and end-to-end integration tests
go test -v ./...

# Build local binary
go build -o bin/nugget ./cmd/nugget

# Cross-compile for Ubuntu Linux
GOOS=linux GOARCH=amd64 go build -o bin/nugget-linux-amd64 ./cmd/nugget
GOOS=linux GOARCH=arm64 go build -o bin/nugget-linux-arm64 ./cmd/nugget
```
