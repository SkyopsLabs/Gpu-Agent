# SkyOps GPU Agent (Go)

A high-performance GPU monitoring and job management agent written in Go for the SkyOps network.

## Features

- **Cross-platform**: Native binaries for Linux, macOS, and Windows
- **GPU Monitoring**: Real-time NVIDIA GPU monitoring via NVML
- **System Monitoring**: CPU, memory, disk, and network statistics
- **Job Management**: Automatic job offer handling and execution
- **Lightweight**: Single binary with no dependencies
- **Logging**: Structured logging with configurable levels

## Quick Start

### Download Pre-built Binaries

Download the appropriate binary for your platform from the releases page:
- Linux (x64): `skyops-linux-amd64.tar.gz`
- Linux (ARM64): `skyops-linux-arm64.tar.gz`  
- macOS (Intel): `skyops-darwin-amd64.tar.gz`
- macOS (Apple Silicon): `skyops-darwin-arm64.tar.gz`
- Windows (x64): `skyops-windows-amd64.zip`

### Configuration

1. Extract the binary and config file
2. Edit `config.json` with your settings:

```json
{
  "agent_id": "skyops_node_${hostname}",
  "backend_url": "https://app.skyopslabs.ai", 
  "heartbeat_interval": 30,
  "max_price_per_hour": 1.50,
  "auto_accept_jobs": false,
  "gpu_whitelist": [],
  "logging_level": "info",
  "logging_file": "gpu-agent.log",
}
```

### Run the Agent

```bash
# Linux/macOS
./skyops-

# Windows  
skyops-.exe

# With custom config
./skyops- --config /path/to/config.json

# Show version
./skyops- -version
```

## Building from Source

### Prerequisites

- Go 1.21 or later
- NVIDIA drivers (for GPU monitoring)
- NVIDIA Management Library (NVML) development files

#### NVIDIA Library Installation

For systems with NVIDIA GPUs, you need to install the NVIDIA Management Library development package:

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install libnvidia-ml-dev
```

**RHEL/CentOS/Fedora:**
```bash
# RHEL/CentOS with EPEL
sudo yum install nvidia-ml-devel
# or for newer versions
sudo dnf install nvidia-ml-devel
```

**Arch Linux:**
```bash
sudo pacman -S nvidia-ml-py
```

#### Troubleshooting NVML Issues

If you encounter segmentation faults or NVML initialization errors:

1. **Verify NVIDIA drivers are installed:**
   ```bash
   nvidia-smi
   ```

2. **Check NVML library is available:**
   ```bash
   ldconfig -p | grep nvidia-ml
   ```

3. **Install development headers:**
   ```bash
   # Ubuntu/Debian
   sudo apt install libnvidia-ml-dev
   ```

4. **Update Go NVML library to latest version:**
   ```bash
   go get github.com/NVIDIA/go-nvml@latest
   go mod tidy
   ```

5
**Note:** Without proper NVIDIA libraries, the agent will still run but GPU monitoring features will be disabled. The Go NVML library version v0.12.4-1 or later is recommended for compatibility with modern NVIDIA drivers.

### Build

```bash
# Clone the repository
git clone <repository-url>
cd repo

# Install dependencies
go mod download

# Build for current platform
go build -o skyops.

# Build for all platforms
./build.sh
```

### Cross-compilation

The build script creates binaries for all supported platforms:

```bash
./build.sh
```

This creates:
- `build/skyops-agent-linux-amd64.tar.gz`
- `build/skyops-agent-linux-arm64.tar.gz`
- `build/skyops-agent-darwin-amd64.tar.gz`
- `build/skyops-agent-darwin-arm64.tar.gz`
- `build/skyops-agent-windows-amd64.zip`

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `agent_id` | Unique identifier for this agent | Required |
| `backend_url` | SkyOps API endpoint | Required |
| `heartbeat_interval` | Seconds between heartbeats | 30 |
| `max_price_per_hour` | Maximum price to accept jobs | 1.50 |
| `auto_accept_jobs` | Automatically accept job offers | false |
| `gpu_whitelist` | List of allowed GPU models | [] (all) |
| `logging_level` | Log level (debug, info, warn, error) | info |
| `logging_file` | Log file path | gpu-agent.log |

## GPU Support

The agent supports NVIDIA GPUs via the NVML library:
- Real-time GPU utilization monitoring
- Memory usage tracking  
- Temperature and power monitoring
- CUDA capability detection
- Driver version reporting




## Troubleshooting

### NVIDIA GPU not detected

1. Ensure NVIDIA drivers are installed
2. Check that `nvidia-smi` works
3. Verify NVML library is accessible