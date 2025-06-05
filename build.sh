#!/bin/bash

# SkyOps GPU Agent Go Build Script
# Builds cross-platform binaries for Linux, macOS, and Windows

set -e

PROJECT_NAME="skyops"
VERSION=${VERSION:-"1.0.0"}
BUILD_DIR="build"
PLATFORMS=("linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64" "windows/amd64")

echo "Building SkyOps GPU Agent v${VERSION}"

# Clean previous builds
rm -rf ${BUILD_DIR}
mkdir -p ${BUILD_DIR}

# Get dependencies
echo "Installing dependencies..."
go mod tidy
go mod download

# Build for each platform
for platform in "${PLATFORMS[@]}"; do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}
    
    output_name=${PROJECT_NAME}
    if [ $GOOS = "windows" ]; then
        output_name+='.exe'
    fi
    
    output_path="${BUILD_DIR}/${PROJECT_NAME}-${GOOS}-${GOARCH}/${output_name}"
    
    echo "Building for ${GOOS}/${GOARCH}..."
    mkdir -p "$(dirname ${output_path})"
    
    env GOOS=$GOOS GOARCH=$GOARCH go build \
        -ldflags="-s -w -X main.Version=${VERSION}" \
        -o ${output_path} .

    
    # Create archive
    cd ${BUILD_DIR}
    if [ $GOOS = "windows" ]; then
        zip -r "${PROJECT_NAME}-${GOOS}-${GOARCH}.zip" "${PROJECT_NAME}-${GOOS}-${GOARCH}/"
    else
        tar -czf "${PROJECT_NAME}-${GOOS}-${GOARCH}.tar.gz" "${PROJECT_NAME}-${GOOS}-${GOARCH}/"
    fi
    cd ..
    
    echo "✓ Built ${GOOS}/${GOARCH}"
done

echo ""
echo "Build complete! Binaries available in ${BUILD_DIR}/"
ls -la ${BUILD_DIR}/

echo ""
echo "Cross-platform binaries:"
for platform in "${PLATFORMS[@]}"; do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}
    
    if [ $GOOS = "windows" ]; then
        echo "  - ${GOOS}/${GOARCH}: ${BUILD_DIR}/${PROJECT_NAME}-${GOOS}-${GOARCH}.zip"
    else
        echo "  - ${GOOS}/${GOARCH}: ${BUILD_DIR}/${PROJECT_NAME}-${GOOS}-${GOARCH}.tar.gz"
    fi
done
