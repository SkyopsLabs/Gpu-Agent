# SkyOps GPU Agent - Go Implementation

A cross-platform GPU agent written in Go for easier distribution and deployment.

## Quick Start

### Development Build (Recommended for development)
```bash
# Quick build and install - one command does it all
make dev

# Or use the shell script
./dev-build.sh
```

### Other Build Options
```bash
# Just build locally
make build

# Build and install to ~/tools/skyops
make install

# Cross-platform build for all platforms
make cross-build

# Clean build artifacts
make clean

# Run tests
make test

# Show all available commands
make help
```

### Manual Build
```bash
# Standard Go build
go build -o skyops .

# Copy to tools directory
mkdir -p ~/tools/skyops
cp skyops ~/tools/skyops/
```

## Usage

Once built and installed, you can use the agent:

```bash
# Show version
skyops version

# Register with network
skyops register

# Start agent
skyops start

# Check status
skyops status

# Stop agent
skyops stop

# Show help
skyops help
```

## Project Structure

- `main.go` - Main CLI interface and agent logic
- `api_client.go` - API client for SkyOps backend communication
- `monitor.go` - System and GPU monitoring
- `build.sh` - Cross-platform build script
- `dev-build.sh` - Quick development build script
- `Makefile` - Build automation

## Development Workflow

1. Make your changes
2. Run `make dev` to build and install
3. Test with `skyops [command]`
4. Repeat

## Cross-Platform Building

The project supports building for multiple platforms:
- Linux (amd64, arm64)
- macOS (amd64, arm64) 
- Windows (amd64)

Use `make cross-build` to build for all platforms.

## Dependencies

- Go 1.19+
- NVIDIA drivers (for GPU monitoring)
- System monitoring libraries (gopsutil)
