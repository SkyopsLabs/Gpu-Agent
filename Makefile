# SkyOps Agent Makefile

.PHONY: dev build cross-build clean install test e2e help

# Development build - quick local build and install
dev:
	@echo "🔨 Building for development..."
	@go build -o skyops .
	@mkdir -p ~/tools/skyops
	@cp skyops ~/tools/skyops/
	@echo "✅ Installed to ~/tools/skyops/skyops"
	@~/tools/skyops/skyops version

# Standard build
build:
	@echo "🔨 Building skyops..."
	@go build -o skyops .
	@echo "✅ Build complete: ./skyops"

# Cross-platform build
cross-build:
	@echo "🔨 Building for all platforms..."
	@./build.sh

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	@rm -f skyops
	@rm -rf build/
	@rm -f *.log
	@echo "✅ Clean complete"

# Install to tools directory
install: build
	@echo "📦 Installing to ~/tools/skyops..."
	@mkdir -p ~/tools/skyops
	@cp skyops ~/tools/skyops/
	@echo "✅ Installed to ~/tools/skyops/skyops"

# Run tests
test:
	@echo "🧪 Running tests..."
	@go test ./...

# End-to-end testing - starts all services and tests complete system
e2e: dev
	@echo "🚀 Starting complete end-to-end test..."
	@./e2e-complete.sh

# Show help
help:
	@echo "SkyOps Agent Build Commands:"
	@echo ""
	@echo "  make dev         - Quick build and install for development"
	@echo "  make build       - Build binary locally"
	@echo "  make cross-build - Build for all platforms (Linux, macOS, Windows)"
	@echo "  make install     - Build and install to ~/tools/skyops"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make test        - Run tests"
	@echo "  make e2e         - Full end-to-end test (Express + FastAPI + Client + Agent)"
	@echo "  make help        - Show this help"
	@echo ""
	@echo "Quick development workflow:"
	@echo "  make dev         - Builds and installs in one command"
	@echo "  make e2e         - Complete system test"

# Default target
.DEFAULT_GOAL := dev
