#!/usr/bin/env bash
#
# Nugget Agent Installer for Linux (Ubuntu amd64 / arm64)
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
    error "This installer must be run as root or with sudo:\n    sudo bash $0"
fi

# 2. Check Operating System
OS="$(uname -s)"
if [ "$OS" != "Linux" ]; then
    error "Nugget Agent installer is intended for Linux (detected: $OS)."
fi

if [ -f /etc/os-release ]; then
    . /etc/os-release
    info "Detected OS: ${PRETTY_NAME:-Linux}"
else
    warn "Could not read /etc/os-release; continuing on generic Linux."
fi

# 3. Detect and Validate Architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)
        TARGET_ARCH="amd64"
        ;;
    aarch64|arm64)
        TARGET_ARCH="arm64"
        ;;
    *)
        error "Unsupported system architecture: $ARCH. Nugget supports amd64 and arm64."
        ;;
esac
info "Detected architecture: $TARGET_ARCH"

# 4. Check System Dependencies
info "Checking system requirements..."
if ! command -v systemctl >/dev/null 2>&1; then
    warn "systemd / systemctl was not found. Background service management will be disabled."
fi

if ! command -v docker >/dev/null 2>&1; then
    warn "Docker is not currently installed."
    echo -e "   You can install Docker on Ubuntu by running:"
    echo -e "   ${YELLOW}sudo apt-get update && sudo apt-get install -y docker.io${NC}\n"
fi

# 5. Resolve Binary Source
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SOURCE_BINARY=""

if [ -n "${NUGGET_BINARY_PATH:-}" ] && [ -f "$NUGGET_BINARY_PATH" ]; then
    SOURCE_BINARY="$NUGGET_BINARY_PATH"
elif [ -f "${REPO_ROOT}/agent/bin/nugget-linux-${TARGET_ARCH}" ]; then
    SOURCE_BINARY="${REPO_ROOT}/agent/bin/nugget-linux-${TARGET_ARCH}"
elif [ -f "${REPO_ROOT}/agent/bin/nugget" ]; then
    SOURCE_BINARY="${REPO_ROOT}/agent/bin/nugget"
elif command -v go >/dev/null 2>&1 && [ -d "${REPO_ROOT}/agent" ]; then
    info "Compiling Nugget agent binary from source..."
    TMP_BUILD="$(mktemp -d)"
    (cd "${REPO_ROOT}/agent" && go build -o "${TMP_BUILD}/${BINARY_NAME}" ./cmd/nugget)
    SOURCE_BINARY="${TMP_BUILD}/${BINARY_NAME}"
fi

if [ -z "$SOURCE_BINARY" ] || [ ! -f "$SOURCE_BINARY" ]; then
    error "Could not find a prebuilt nugget binary or Go compiler to build from source.\nBuild the binary first with:\n    cd agent && go build -o bin/nugget ./cmd/nugget"
fi

# 6. Install Binary
info "Installing binary to ${TARGET_PATH}..."
mkdir -p "$INSTALL_DIR"
cp -f "$SOURCE_BINARY" "$TARGET_PATH"
chmod 755 "$TARGET_PATH"

# Restart systemd service if active to apply upgrade seamlessly
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet nugget 2>/dev/null; then
    info "Restarting running nugget systemd service to apply update..."
    systemctl restart nugget || true
fi

# 7. Verification
success "Nugget binary installed successfully:"
"$TARGET_PATH" version

echo ""
if [ -f "/etc/nugget/config.json" ]; then
    success "Nugget Agent updated successfully! Existing configuration and applications preserved."
    echo -e "Run 'nugget status' to inspect current host and applications.\n"
else
    success "Installation complete!"
    echo -e "Next step: Initialize this machine by running:"
    echo -e "    ${GREEN}sudo nugget init${NC}\n"
fi

