#!/bin/bash

# Build Windows MSI installer for SkyOps CLI
# Requires: WiX Toolset v3.x (Windows) or wixl (Linux)

set -e

VERSION=${1:-"1.0.0"}
ARCH=${2:-"amd64"}

echo "🔨 Building Windows MSI installer..."
echo "Version: $VERSION"
echo "Architecture: $ARCH"

# Create build directory
BUILD_DIR="build/windows"
mkdir -p "$BUILD_DIR"

# Clean previous builds
rm -f "$BUILD_DIR"/*.msi "$BUILD_DIR"/*.wixobj "$BUILD_DIR"/*.wixpdb

# Check if we're on Windows (for native WiX) or Linux (for wixl)
if command -v candle.exe >/dev/null 2>&1 && command -v light.exe >/dev/null 2>&1; then
    echo "📦 Using WiX Toolset (Windows)"
    
    # Build the Windows executable first
    echo "🔨 Building Windows executable..."
    GOOS=windows GOARCH=$ARCH go build -o "$BUILD_DIR/skyops.exe" .
    
    # Copy WiX source files to build directory
    cp windows/skyops.wxs "$BUILD_DIR/"
    cp windows/License.rtf "$BUILD_DIR/"
    cp windows/add-to-path.bat "$BUILD_DIR/"
    
    # Change to build directory
    cd "$BUILD_DIR"
    
    # Compile WiX source
    echo "📝 Compiling WiX source..."
    candle.exe -arch x64 -dVersion="$VERSION" skyops.wxs
    
    # Link and create MSI
    echo "🔗 Creating MSI package..."
    light.exe -ext WixUIExtension -out "skyops_${VERSION}_windows_${ARCH}.msi" skyops.wixobj
    
    echo "✅ MSI created: build/windows/skyops_${VERSION}_windows_${ARCH}.msi"
    
elif command -v wixl >/dev/null 2>&1; then
    echo "📦 Using wixl (Linux)"
    
    # Build the Windows executable first
    echo "🔨 Building Windows executable..."
    GOOS=windows GOARCH=$ARCH go build -o "$BUILD_DIR/skyops.exe" .
    
    # Copy WiX source files to build directory
    cp windows/skyops.wxs "$BUILD_DIR/"
    cp windows/License.rtf "$BUILD_DIR/"
    cp windows/add-to-path.bat "$BUILD_DIR/"
    
    # Change to build directory
    cd "$BUILD_DIR"
    
    # Create MSI using wixl
    echo "🔗 Creating MSI package with wixl..."
    wixl -v skyops.wxs -o "skyops_${VERSION}_windows_${ARCH}.msi"
    
    echo "✅ MSI created: build/windows/skyops_${VERSION}_windows_${ARCH}.msi"
    
else
    echo "❌ Error: WiX Toolset not found!"
    echo ""
    echo "Please install one of the following:"
    echo ""
    echo "Windows:"
    echo "  - Download and install WiX Toolset v3.x from https://wixtoolset.org/"
    echo "  - Add WiX bin directory to your PATH"
    echo ""
    echo "Linux (Ubuntu/Debian):"
    echo "  - sudo apt install wixl"
    echo ""
    echo "Linux (RHEL/CentOS/Fedora):"
    echo "  - sudo dnf install msitools"
    echo "  # or"
    echo "  - sudo yum install msitools"
    echo ""
    echo "macOS:"
    echo "  - brew install msitools"
    exit 1
fi

# Move MSI to releases directory
mkdir -p "../../releases"
if [ -f "skyops_${VERSION}_windows_${ARCH}.msi" ]; then
    cp "skyops_${VERSION}_windows_${ARCH}.msi" "../../releases/"
    echo "📦 MSI copied to releases/ directory"
fi

# Return to original directory
cd - >/dev/null

echo ""
echo "🎉 Windows MSI build complete!"
echo "📁 Output: releases/skyops_${VERSION}_windows_${ARCH}.msi"
echo ""
echo "To test the installer:"
echo "  1. Copy the .msi file to a Windows machine"
echo "  2. Run: msiexec /i skyops_${VERSION}_windows_${ARCH}.msi"
echo "  3. Or double-click the .msi file to install"
