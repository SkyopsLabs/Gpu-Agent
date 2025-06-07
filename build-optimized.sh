#!/bin/bash

# Build script for optimized SkyOps CLI binary

echo "🚀 Building optimized SkyOps CLI..."

# Set build flags for optimization
BUILD_FLAGS="-ldflags=-s -w"

# Build for current platform
echo "📦 Building for current platform..."
go build -ldflags="-s -w" -o skyops .

# Optional: Build for multiple platforms
if [ "$1" = "all" ]; then
    echo "📦 Building for all platforms..."
    
    # Linux
    GOOS=linux GOARCH=amd64 go build $BUILD_FLAGS -o releases/skyops-linux-amd64 .
    GOOS=linux GOARCH=arm64 go build $BUILD_FLAGS -o releases/skyops-linux-arm64 .
    
    # Windows
    GOOS=windows GOARCH=amd64 go build $BUILD_FLAGS -o releases/skyops-windows-amd64.exe .
    
    # macOS
    GOOS=darwin GOARCH=amd64 go build $BUILD_FLAGS -o releases/skyops-darwin-amd64 .
    GOOS=darwin GOARCH=arm64 go build $BUILD_FLAGS -o releases/skyops-darwin-arm64 .
    
    echo "✅ Cross-platform builds completed!"
fi

echo "✅ Build completed!"
echo "📊 Binary size: $(ls -lh skyops | awk '{print $5}')"
