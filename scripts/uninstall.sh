#!/usr/bin/env bash
#
# Nugget Agent Uninstaller for Linux (Ubuntu amd64 / arm64)
#
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

INSTALL_DIR="/usr/local/bin"
BINARY_NAME="nugget"
TARGET_PATH="${INSTALL_DIR}/${BINARY_NAME}"
SERVICE_UNIT="/etc/systemd/system/nugget.service"
CONFIG_DIR="/etc/nugget"
DATA_DIR="/var/lib/nugget"

info() {
    echo -e "${BLUE}==>${NC} $1"
}

success() {
    echo -e "${GREEN}==>${NC} $1"
}

warn() {
    echo -e "${YELLOW}Warning:${NC} $1"
}

error() {
    echo -e "${RED}Error:${NC} $1" >&2
    exit 1
}

# 1. Check Root Permissions
if [ "$(id -u)" -ne 0 ]; then
    error "This uninstaller must be run as root or with sudo:\n    sudo bash $0"
fi

PURGE=false
NON_INTERACTIVE=false

for arg in "$@"; do
    case "$arg" in
        --purge)
            PURGE=true
            ;;
        -y|--yes|--non-interactive)
            NON_INTERACTIVE=true
            ;;
        --help|-h)
            echo "Usage: sudo ./scripts/uninstall.sh [--purge] [-y]"
            echo ""
            echo "Options:"
            echo "  --purge    Also delete all configuration (/etc/nugget) and data (/var/lib/nugget)"
            echo "  -y, --yes  Proceed without interactive confirmation"
            exit 0
            ;;
    esac
done

if [ "$NON_INTERACTIVE" = false ]; then
    echo -e "${YELLOW}This will stop and remove the Nugget Agent from this system.${NC}"
    if [ "$PURGE" = true ]; then
        echo -e "${RED}WARNING: --purge flag specified. All configuration, applications and deployment records will be permanently deleted.${NC}"
    else
        echo -e "Configuration (${CONFIG_DIR}) and application data (${DATA_DIR}) will be preserved."
    fi
    read -rp "Are you sure you want to continue? [y/N]: " confirm
    case "$confirm" in
        [yY][eE][sS]|[yY])
            ;;
        *)
            echo "Uninstallation cancelled."
            exit 0
            ;;
    esac
fi

# 2. Stop and Disable Systemd Service
if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet nugget 2>/dev/null; then
        info "Stopping nugget systemd service..."
        systemctl stop nugget || true
    fi
    if systemctl is-enabled --quiet nugget 2>/dev/null; then
        info "Disabling nugget systemd service..."
        systemctl disable nugget || true
    fi
fi

# 3. Remove Service Unit
if [ -f "$SERVICE_UNIT" ]; then
    info "Removing systemd unit ${SERVICE_UNIT}..."
    rm -f "$SERVICE_UNIT"
    if command -v systemctl >/dev/null 2>&1; then
        systemctl daemon-reload || true
    fi
fi

# 4. Remove Binary
if [ -f "$TARGET_PATH" ]; then
    info "Removing binary ${TARGET_PATH}..."
    rm -f "$TARGET_PATH"
fi

# 5. Handle Data and Configuration Purge
if [ "$PURGE" = true ]; then
    info "Purging configuration directory ${CONFIG_DIR}..."
    rm -rf "$CONFIG_DIR"
    info "Purging data directory ${DATA_DIR}..."
    rm -rf "$DATA_DIR"
    success "Nugget configuration and data purged."
else
    echo ""
    info "Preserved configuration: ${CONFIG_DIR}"
    info "Preserved application data: ${DATA_DIR}"
    echo "  (To remove them later, run: sudo rm -rf ${CONFIG_DIR} ${DATA_DIR})"
fi

echo ""
success "Nugget has been successfully uninstalled."

