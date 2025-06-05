# SkyOps Agent Makefile

.PHONY: dev build cross-build clean install test e2e package-build package-release package-install msi-build msi-release build-all-packages version version-set release transfer-releases help

# Version management targets
version:
	@./version-manager.sh --get

version-set:
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ VERSION required. Usage: make version-set VERSION=1.0.1"; \
		exit 1; \
	fi
	@./version-manager.sh --set $(VERSION)

# Build and transfer releases to skyops-cli-repo
release:
	@if [ -n "$(VERSION)" ]; then \
		./version-manager.sh --release $(VERSION); \
	else \
		./version-manager.sh --release; \
	fi

# Transfer existing releases to skyops-cli-repo without rebuilding
transfer-releases:
	@echo "📦 Transferring releases to skyops-cli-repo..."
	@mkdir -p skyops-cli-repo/releases/
	@transferred=0; \
	if [ -d "releases" ]; then \
		cp releases/*.deb skyops-cli-repo/releases/ 2>/dev/null && transferred=1 || true; \
		cp releases/*.msi skyops-cli-repo/releases/ 2>/dev/null && transferred=1 || true; \
	fi; \
	if [ -d "releases/download" ]; then \
		cp releases/download/*.tar.gz skyops-cli-repo/releases/ 2>/dev/null && transferred=1 || true; \
		cp releases/download/*.zip skyops-cli-repo/releases/ 2>/dev/null && transferred=1 || true; \
	fi; \
	if [ -d "build" ]; then \
		cp build/*.tar.gz skyops-cli-repo/releases/ 2>/dev/null && transferred=1 || true; \
		cp build/*.zip skyops-cli-repo/releases/ 2>/dev/null && transferred=1 || true; \
	fi; \
	if [ $$transferred -eq 1 ]; then \
		echo "✅ Release files transferred:"; \
		ls -la skyops-cli-repo/releases/; \
	else \
		echo "❌ No release files found. Run 'make build-all-packages' first"; \
		exit 1; \
	fi

.PHONY: dev build cross-build clean install test e2e package-build package-release package-install msi-build msi-release build-all-packages version version-set release transfer-releases help

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

# Build Debian package
package-build:
	@echo "📦 Building Debian package..."
	@dpkg-buildpackage -us -uc -b
	@mkdir -p releases/
	@mv ../skyops_*.deb releases/ 2>/dev/null || true
	@echo "✅ Debian package built and moved to releases/"

# Build and prepare for release distribution
package-release:
	@echo "🚀 Preparing release package..."
	@dpkg-buildpackage -us -uc -b
	@mkdir -p releases/
	@mv ../skyops_*.deb releases/ 2>/dev/null || true
	@echo ""
	@echo "✅ Release prepared in releases/ directory"
	@echo "📁 Contents:"
	@ls -la releases/
	@echo ""
	@echo "🔗 Next steps:"
	@echo "  1. Upload releases/skyops_*.deb to GitHub releases"
	@echo "  2. Update install.sh with correct GitHub URL"
	@echo "  3. Test installation: sudo dpkg -i releases/skyops_*.deb"

# Install built package locally
package-install:
	@echo "📥 Installing local package..."
	@if [ -f releases/skyops_*.deb ]; then \
		echo "Installing package from releases/"; \
		sudo dpkg -i releases/skyops_*.deb; \
		sudo apt-get install -f; \
	else \
		echo "❌ No DEB package found in releases/"; \
		echo "Run 'make package-build' first"; \
		exit 1; \
	fi

# Build Windows MSI installer
msi-build:
	@echo "🏢 Building Windows MSI installer..."
	@./build-msi.sh

# Build MSI with version (usage: make msi-release VERSION=1.0.0)
msi-release:
	@VERSION=$${VERSION:-1.0.0}; \
	echo "🚀 Building Windows MSI installer v$$VERSION..."; \
	./build-msi.sh $$VERSION
	@echo ""
	@echo "✅ MSI installer ready for release"
	@echo "📁 Contents:"
	@ls -la releases/*.msi 2>/dev/null || echo "No MSI files found"
	@echo ""
	@echo "🔗 Next steps:"
	@echo "  1. Test the MSI installer on a Windows machine"
	@echo "  2. Upload releases/skyops_*.msi to GitHub releases"
	@echo "  3. Update documentation with installation instructions"

# Build all packages (DEB + MSI + cross-platform binaries) for release
build-all-packages:
	@VERSION=$${VERSION:-1.0.0}; \
	echo "🎯 Building all packages for release v$$VERSION..."; \
	echo ""; \
	echo "🔨 Building cross-platform binaries..."; \
	VERSION=$$VERSION make cross-build; \
	echo ""; \
	echo "📁 Backing up cross-platform binaries..."; \
	mkdir -p releases/download/; \
	if [ -d "build" ]; then \
		cp build/*.tar.gz releases/download/ 2>/dev/null || true; \
		cp build/*.zip releases/download/ 2>/dev/null || true; \
	fi; \
	echo ""; \
	echo "📦 Building Debian package..."; \
	make package-release; \
	echo ""; \
	echo "🏢 Building Windows MSI installer..."; \
	make msi-release VERSION=$$VERSION; \
	echo ""; \
	echo "📁 Transferring all artifacts to skyops-cli-repo/releases..."; \
	mkdir -p skyops-cli-repo/releases/; \
	if [ -d "releases/download" ]; then \
		cp releases/download/*.tar.gz skyops-cli-repo/releases/ 2>/dev/null || true; \
		cp releases/download/*.zip skyops-cli-repo/releases/ 2>/dev/null || true; \
	fi; \
	if [ -d "releases" ]; then \
		cp releases/*.deb skyops-cli-repo/releases/ 2>/dev/null || true; \
		cp releases/*.msi skyops-cli-repo/releases/ 2>/dev/null || true; \
	fi; \
	echo ""; \
	echo "✅ All packages built and transferred successfully!"; \
	echo "📁 skyops-cli-repo/releases contents:"; \
	ls -la skyops-cli-repo/releases/ 2>/dev/null || echo "No release files found"

# Show help
help:
	@echo "SkyOps Agent Build Commands:"
	@echo ""
	@echo "  make dev              - Quick build and install for development"
	@echo "  make build            - Build binary locally"
	@echo "  make cross-build      - Build for all platforms (Linux, macOS, Windows)"
	@echo "  make install          - Build and install to ~/tools/skyops"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make test             - Run tests"
	@echo "  make e2e              - Full end-to-end test (Express + FastAPI + Client + Agent)"
	@echo ""
	@echo "Package Management:"
	@echo "  make package-build    - Build Debian (.deb) package"
	@echo "  make package-release  - Build and prepare for distribution"
	@echo "  make package-install  - Install built package locally"
	@echo ""
	@echo "Windows Installer:"
	@echo "  make msi-build        - Build Windows MSI installer"
	@echo "  make msi-release      - Build MSI with version (VERSION=1.0.0)"
	@echo ""
	@echo "Release Management:"
	@echo "  make build-all-packages - Build DEB, MSI, and cross-platform binaries (VERSION=1.0.0)"
	@echo "  make release          - Build and transfer to skyops-cli-repo (VERSION=1.0.1)"
	@echo "  make transfer-releases- Transfer existing releases to skyops-cli-repo"
	@echo ""
	@echo "Version Management:"
	@echo "  make version          - Show current version"
	@echo "  make version-set      - Set new version (VERSION=1.0.1)"
	@echo "  ./version-manager.sh  - Advanced version management script"
	@echo ""
	@echo "  make help             - Show this help"
	@echo ""
	@echo "Quick development workflow:"
	@echo "  make dev              - Builds and installs in one command"
	@echo "  make e2e              - Complete system test"
	@echo ""
	@echo "Release workflow:"
	@echo "  make version-set VERSION=1.0.1  - Update version across all files"
	@echo "  make release VERSION=1.0.1      - Build and transfer to skyops-cli-repo"
	@echo "  OR: ./version-manager.sh 1.0.1  - One-command version update and release"

# Default target
.DEFAULT_GOAL := dev
