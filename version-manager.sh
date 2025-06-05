#!/bin/bash

# SkyOps Version Management Script
# This script manages version consistency across all build artifacts

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default version if not provided
DEFAULT_VERSION="1.0.0"

# Function to display help
show_help() {
    echo "SkyOps Version Management"
    echo ""
    echo "Usage: $0 [OPTIONS] [VERSION]"
    echo ""
    echo "Options:"
    echo "  --get                    Get current version"
    echo "  --set VERSION            Set new version across all files"
    echo "  --build                  Build packages with current version"
    echo "  --release                Build and transfer to skyops-cli-repo"
    echo "  --help                   Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 --get                 # Show current version"
    echo "  $0 --set 1.0.1           # Update to version 1.0.1"
    echo "  $0 --build               # Build with current version"
    echo "  $0 --release 1.0.1       # Set version and release"
    echo ""
}

# Function to get current version from main.go
get_current_version() {
    grep -o 'SkyOps GPU Agent v[0-9]\+\.[0-9]\+\.[0-9]\+' main.go | sed 's/SkyOps GPU Agent v//' || echo "$DEFAULT_VERSION"
}

# Function to update version in main.go
update_main_version() {
    local new_version="$1"
    echo -e "${BLUE}📝 Updating version in main.go to $new_version${NC}"
    sed -i "s/SkyOps GPU Agent v[0-9]\+\.[0-9]\+\.[0-9]\+/SkyOps GPU Agent v$new_version/g" main.go
}

# Function to update debian changelog
update_debian_changelog() {
    local new_version="$1"
    local date_str=$(date -R)
    
    echo -e "${BLUE}📝 Updating debian/changelog to $new_version${NC}"
    
    # Create new changelog entry
    cat > debian/changelog.new << EOF
skyops ($new_version-1) stable; urgency=medium

  * Version $new_version release
  * Updated command-line interface and features
  * Enhanced GPU monitoring capabilities
  * Improved system integration and packaging

 -- SkyOps Team <support@skyopslabs.ai>  $date_str

EOF
    
    # Append existing changelog if it exists
    if [ -f debian/changelog ]; then
        cat debian/changelog >> debian/changelog.new
    fi
    
    mv debian/changelog.new debian/changelog
}

# Function to update Windows MSI version
update_msi_version() {
    local new_version="$1"
    echo -e "${BLUE}📝 Updating windows/skyops.wxs to $new_version${NC}"
    sed -i "s/Version=\"[0-9]\+\.[0-9]\+\.[0-9]\+\"/Version=\"$new_version\"/g" windows/skyops.wxs
}

# Function to update build-msi.sh default version
update_build_msi_version() {
    local new_version="$1"
    echo -e "${BLUE}📝 Updating build-msi.sh default version to $new_version${NC}"
    sed -i "s/VERSION=\${1:-[0-9]\+\.[0-9]\+\.[0-9]\+}/VERSION=\${1:-$new_version}/g" build-msi.sh 2>/dev/null || true
}

# Function to update Makefile default version
update_makefile_version() {
    local new_version="$1"
    echo -e "${BLUE}📝 Updating Makefile default version to $new_version${NC}"
    sed -i "s/VERSION=\$\${VERSION:-[0-9]\+\.[0-9]\+\.[0-9]\+}/VERSION=\$\${VERSION:-$new_version}/g" Makefile
}

# Function to set version across all files
set_version() {
    local new_version="$1"
    
    if [[ ! $new_version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo -e "${RED}❌ Invalid version format. Use semantic versioning (e.g., 1.0.0)${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}🔄 Setting version to $new_version across all files...${NC}"
    
    update_main_version "$new_version"
    update_debian_changelog "$new_version"
    update_msi_version "$new_version"
    update_build_msi_version "$new_version"
    update_makefile_version "$new_version"
    
    echo -e "${GREEN}✅ Version updated to $new_version${NC}"
}

# Function to build packages
build_packages() {
    local version=$(get_current_version)
    echo -e "${GREEN}🔨 Building packages for version $version...${NC}"
    make build-all-packages VERSION="$version"
}

# Function to transfer releases to skyops-cli-repo
transfer_releases() {
    local version=$(get_current_version)
    local cli_repo_path="skyops-cli-repo"
    
    echo -e "${GREEN}📦 Transferring releases to $cli_repo_path...${NC}"
    
    # Create releases directory in cli repo if it doesn't exist
    mkdir -p "$cli_repo_path/releases"
    
    # Copy release files
    if [ -d "releases" ]; then
        echo -e "${BLUE}📁 Copying release files...${NC}"
        cp releases/*.deb "$cli_repo_path/releases/" 2>/dev/null || true
        cp releases/*.msi "$cli_repo_path/releases/" 2>/dev/null || true
        
        # Copy cross-platform binaries from backup locations
        if [ -d "releases/cross-platform" ]; then
            cp releases/cross-platform/*.tar.gz "$cli_repo_path/releases/" 2>/dev/null || true
            cp releases/cross-platform/*.zip "$cli_repo_path/releases/" 2>/dev/null || true
        fi
        if [ -d "releases/download" ]; then
            cp releases/download/*.tar.gz "$cli_repo_path/releases/" 2>/dev/null || true
            cp releases/download/*.zip "$cli_repo_path/releases/" 2>/dev/null || true
        fi
        
        echo -e "${GREEN}✅ Release files transferred:${NC}"
        ls -la "$cli_repo_path/releases/"
    else
        echo -e "${YELLOW}⚠️  No releases directory found. Run build first.${NC}"
    fi
}

# Function to create/update changelog in cli repo
update_cli_repo_changelog() {
    local version=$(get_current_version)
    local cli_repo_path="skyops-cli-repo"
    local date_str=$(date +%Y-%m-%d)
    
    echo -e "${BLUE}📝 Updating $cli_repo_path/CHANGELOG.md...${NC}"
    
    # Extract commands from main.go for changelog
    local commands=$(grep -A 20 "Available Commands:" main.go | grep -E "^\s+(login|register|start|status|stats|stop|ping|version|help)" | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*#.*//' | sort -u | tr '\n' ' ' || echo "login register start status stats stop ping version help")
    
    # Create updated changelog entry
    cat > "$cli_repo_path/CHANGELOG.md.new" << EOF
# Changelog

All notable changes to SkyOps CLI will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Planning future features and improvements

## [$version] - $date_str

### Added
- Cross-platform GPU monitoring and job management agent
- Real-time NVIDIA GPU monitoring via NVML
- Comprehensive system monitoring (CPU, memory, disk, network)
- Single binary distribution with no external dependencies
- Structured logging with configurable levels
- Daemon mode for background operation
- Professional packaging for Windows MSI and Linux DEB
- Automatic PATH integration on installation

### Commands
- **Authentication**: \`login\` - Authenticate with SkyOps network
- **Registration**: \`register\` - Register node with SkyOps network  
- **Agent Control**: \`start\`, \`stop\`, \`status\` - Manage agent lifecycle
- **Monitoring**: \`stats\`, \`ping\` - Monitor agent performance and connectivity
- **Information**: \`version\`, \`help\` - Get version and usage information

### Features
- **Platforms**: Linux (x64, ARM64), macOS (Intel, Apple Silicon), Windows (x64)
- **GPU Support**: NVIDIA GPU monitoring with temperature, utilization, memory tracking
- **System Metrics**: Real-time CPU, memory, disk, and network monitoring
- **Job Management**: Automatic job acceptance and execution capabilities
- **Configuration**: JSON-based configuration with intelligent defaults
- **Logging**: Structured logging with file output and configurable levels

### Installation
- **Linux**: DEB package with automatic PATH setup and system integration
- **Windows**: MSI installer with Start Menu shortcuts and PATH configuration
- **macOS/Manual**: Binary distributions for flexible installation

### Security
- Secure HTTPS communication with SkyOps backend
- Local configuration management with proper file permissions
- No sensitive data exposure in logs or command output
- Authenticated API communication

[Unreleased]: https://github.com/skyopslabs/skyops-cli/compare/v$version...HEAD
[$version]: https://github.com/skyopslabs/skyops-cli/releases/tag/v$version

EOF
    
    mv "$cli_repo_path/CHANGELOG.md.new" "$cli_repo_path/CHANGELOG.md"
    echo -e "${GREEN}✅ Changelog updated in $cli_repo_path${NC}"
}

# Function to perform full release workflow
full_release() {
    local new_version="$1"
    
    if [ -n "$new_version" ]; then
        set_version "$new_version"
    fi
    
    local version=$(get_current_version)
    echo -e "${GREEN}🚀 Starting full release workflow for version $version...${NC}"
    
    # Build packages
    build_packages
    
    # Update CLI repo changelog
    update_cli_repo_changelog
    
    # Transfer releases
    transfer_releases
    
    echo -e "${GREEN}🎉 Release $version completed successfully!${NC}"
    echo ""
    echo -e "${BLUE}📁 Release artifacts:${NC}"
    ls -la skyops-cli-repo/releases/ 2>/dev/null || echo "No release files found"
    echo ""
    echo -e "${BLUE}📝 Next steps:${NC}"
    echo -e "  1. Review the updated changelog: skyops-cli-repo/CHANGELOG.md"
    echo -e "  2. Commit and push changes to skyops-cli-repo"
    echo -e "  3. Create a GitHub release with the artifacts"
    echo -e "  4. Update installation documentation if needed"
}

# Main script logic
case "${1:-}" in
    --get)
        echo "Current version: $(get_current_version)"
        ;;
    --set)
        if [ -z "$2" ]; then
            echo -e "${RED}❌ Version required. Usage: $0 --set VERSION${NC}"
            exit 1
        fi
        set_version "$2"
        ;;
    --build)
        build_packages
        ;;
    --release)
        full_release "$2"
        ;;
    --help|help|-h)
        show_help
        ;;
    "")
        show_help
        ;;
    *)
        if [[ $1 =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            # If first argument is a version number, do full release
            full_release "$1"
        else
            echo -e "${RED}❌ Unknown option: $1${NC}"
            show_help
            exit 1
        fi
        ;;
esac
