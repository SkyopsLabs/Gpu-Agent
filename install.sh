#!/bin/bash
set -e

# SkyOps Installation Script
echo "🚀 Installing SkyOps Agent..."

# Configuration
GITHUB_REPO="yourusername/skyops"  # Replace with your GitHub repo
PACKAGE_NAME="skyops"
TEMP_DIR="/tmp/skyops-install"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   log_error "This script should not be run as root. Run without sudo."
   exit 1
fi

# Check for required tools
command -v wget >/dev/null 2>&1 || { 
    log_error "wget is required but not installed. Install it with: sudo apt install wget"
    exit 1
}

# Detect architecture
ARCH=$(dpkg --print-architecture)
log_info "Detected architecture: $ARCH"

# Create temp directory
mkdir -p "$TEMP_DIR"
cd "$TEMP_DIR"

# Get latest release info from GitHub API
log_info "Fetching latest release information..."
LATEST_RELEASE=$(wget -qO- "https://api.github.com/repos/$GITHUB_REPO/releases/latest" 2>/dev/null || echo "")

if [ -z "$LATEST_RELEASE" ]; then
    log_error "Failed to fetch release information. Please check your internet connection and repository URL."
    exit 1
fi

# Extract download URL for the .deb file
DOWNLOAD_URL=$(echo "$LATEST_RELEASE" | grep -o "https://github.com/$GITHUB_REPO/releases/download/[^\"]*${ARCH}.deb" | head -1)

if [ -z "$DOWNLOAD_URL" ]; then
    log_error "No .deb package found for architecture $ARCH in the latest release."
    log_info "Available files:"
    echo "$LATEST_RELEASE" | grep -o "https://github.com/$GITHUB_REPO/releases/download/[^\"]*\.deb" || echo "No .deb files found"
    exit 1
fi

# Extract version from URL
VERSION=$(echo "$DOWNLOAD_URL" | grep -o "v[0-9]\+\.[0-9]\+\.[0-9]\+" || echo "unknown")
log_info "Latest version: $VERSION"

# Download the package
PACKAGE_FILE="skyops_${VERSION#v}-1_${ARCH}.deb"
log_info "Downloading $PACKAGE_FILE..."
wget -O "$PACKAGE_FILE" "$DOWNLOAD_URL" || {
    log_error "Failed to download package"
    exit 1
}

# Verify the package
log_info "Verifying package..."
if ! dpkg --info "$PACKAGE_FILE" >/dev/null 2>&1; then
    log_error "Downloaded file is not a valid Debian package"
    exit 1
fi

# Install the package
log_info "Installing SkyOps package..."
sudo dpkg -i "$PACKAGE_FILE" || {
    log_warn "Package installation failed, attempting to fix dependencies..."
    sudo apt-get update
    sudo apt-get install -f -y
    sudo dpkg -i "$PACKAGE_FILE" || {
        log_error "Failed to install package even after fixing dependencies"
        exit 1
    }
}

# Source the profile to update PATH
log_info "Updating PATH..."
if [ -f /etc/profile.d/skyops.sh ]; then
    source /etc/profile.d/skyops.sh
fi

# Add to current shell profiles if not already present
for shell_rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [ -f "$shell_rc" ] && ! grep -q "skyops" "$shell_rc"; then
        echo "" >> "$shell_rc"
        echo "# SkyOps PATH (added by installer)" >> "$shell_rc"
        echo 'export PATH="$HOME/.skyops:$PATH"' >> "$shell_rc"
    fi
done

# Clean up
rm -rf "$TEMP_DIR"

# Test installation
log_info "Testing installation..."
if command -v skyops >/dev/null 2>&1; then
    INSTALLED_VERSION=$(skyops --version 2>/dev/null || echo "unknown")
    log_info "✅ SkyOps successfully installed! Version: $INSTALLED_VERSION"
    log_info "You can now use 'skyops' command from anywhere"
    log_info ""
    log_info "To get started, run: skyops --help"
else
    log_warn "Installation completed but 'skyops' command not found in PATH."
    log_info "Please restart your shell or run: source ~/.bashrc"
    log_info "Then try: skyops --help"
fi

log_info ""
log_info "🎉 Installation complete!"
