package main

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemMonitor handles system and GPU monitoring
type SystemMonitor struct {
}

// NewSystemMonitor creates a new system monitor
func NewSystemMonitor() *SystemMonitor {
	return &SystemMonitor{}
}

// GetSystemInfo returns basic system information
func (m *SystemMonitor) GetSystemInfo() SystemInfo {
	hostInfo, _ := host.Info()

	return SystemInfo{
		Hostname:        hostInfo.Hostname,
		Platform:        hostInfo.Platform,
		PlatformFamily:  hostInfo.PlatformFamily,
		PlatformVersion: hostInfo.PlatformVersion,
		KernelVersion:   hostInfo.KernelVersion,
		Architecture:    runtime.GOARCH,
		CPUCount:        runtime.NumCPU(),
		BootTime:        time.Unix(int64(hostInfo.BootTime), 0).Format(time.RFC3339),
		Uptime:          hostInfo.Uptime,
	}
}

// GetCPUStats returns CPU statistics
func (m *SystemMonitor) GetCPUStats() CPUStats {
	// Get CPU percentage over 1 second
	percentages, _ := cpu.Percent(time.Second, false)

	// Get CPU info
	cpuInfo, _ := cpu.Info()

	var totalUsage float64
	if len(percentages) > 0 {
		totalUsage = percentages[0]
	}

	result := CPUStats{
		UsagePercent: totalUsage,
		CoreCount:    runtime.NumCPU(),
	}

	if len(cpuInfo) > 0 {
		result.ModelName = cpuInfo[0].ModelName
		result.Family = cpuInfo[0].Family
		result.Mhz = cpuInfo[0].Mhz
		result.CacheSize = cpuInfo[0].CacheSize
	}

	return result
}

// GetMemoryStats returns memory statistics
func (m *SystemMonitor) GetMemoryStats() MemoryStats {
	memInfo, _ := mem.VirtualMemory()
	swapInfo, _ := mem.SwapMemory()

	return MemoryStats{
		Total:             memInfo.Total,
		Available:         memInfo.Available,
		Used:              memInfo.Used,
		Free:              memInfo.Free,
		UsagePercent:      memInfo.UsedPercent,
		SwapTotal:         swapInfo.Total,
		SwapUsed:          swapInfo.Used,
		SwapFree:          swapInfo.Free,
		SwapUsagePercent:  swapInfo.UsedPercent,
	}
}

// GetDiskStats returns disk statistics
func (m *SystemMonitor) GetDiskStats() DiskStats {
	diskInfo, _ := disk.Usage("/")

	return DiskStats{
		Total:        diskInfo.Total,
		Used:         diskInfo.Used,
		Free:         diskInfo.Free,
		UsagePercent: diskInfo.UsedPercent,
		Path:         diskInfo.Path,
		Fstype:       diskInfo.Fstype,
	}
}

// GetGPUStats returns GPU statistics
func (m *SystemMonitor) GetGPUStats() []GPUInfo {
	// Try to detect NVIDIA GPUs using nvidia-smi
	gpus := []GPUInfo{}

	cmd := exec.Command("nvidia-smi", "--query-gpu=index,name,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,fan.speed", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return gpus
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 10 {
			continue // Unexpected output
		}
		gpu := GPUInfo{}
		// Parse fields safely
		gpu.Index = parseInt(fields[0])
		gpu.Name = strings.TrimSpace(fields[1])
		gpu.MemoryTotal = parseUint64(fields[2]) * 1024 * 1024 // MB to bytes
		gpu.MemoryUsed = parseUint64(fields[3]) * 1024 * 1024
		gpu.MemoryFree = parseUint64(fields[4]) * 1024 * 1024
		gpu.MemoryUsagePercent = percentFromFields(fields[3], fields[2])
		gpu.GPUUtilization = parseUint32(fields[5])
		gpu.MemoryUtilization = parseUint32(fields[6])
		gpu.Temperature = parseUint32(fields[7])
		gpu.PowerUsage = parseFloat64(fields[8])
		gpu.FanSpeed = parseUint32(fields[9])
		gpus = append(gpus, gpu)
	}
	return gpus
}


// Helper functions for parsing nvidia-smi output
func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}
func parseUint32(s string) uint32 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	return uint32(v)
}
func parseFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
func percentFromFields(used, total string) float64 {
	u := parseFloat64(used)
	t := parseFloat64(total)
	if t == 0 {
		return 0
	}
	return (u / t) * 100
}

// GetCompleteStats returns complete system statistics
func (m *SystemMonitor) GetCompleteStats() AgentMonitoringData {
	return AgentMonitoringData{
		SystemInfo:  m.GetSystemInfo(),
		CPUStats:    m.GetCPUStats(),
		MemoryStats: m.GetMemoryStats(),
		DiskStats:   m.GetDiskStats(),
		GPUStats:    m.GetGPUStats(),
		Timestamp:   time.Now().Unix(),
	}
}
