package main

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemMonitor handles system and GPU monitoring
type SystemMonitor struct {
	nvmlInitialized bool
}

// NewSystemMonitor creates a new system monitor
func NewSystemMonitor() *SystemMonitor {
	monitor := &SystemMonitor{}

	// Try to initialize NVML for NVIDIA GPU monitoring
	// This may fail on systems without NVIDIA drivers or GPUs
	defer func() {
		if r := recover(); r != nil {
			// Handle panic from NVML initialization gracefully
			monitor.nvmlInitialized = false
		}
	}()

	if ret := nvml.Init(); ret == nvml.SUCCESS {
		monitor.nvmlInitialized = true
	} else {
		monitor.nvmlInitialized = false
	}

	return monitor
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
	if !m.nvmlInitialized {
		return m.getFallbackGPUStats()
	}

	deviceCount, ret := nvml.DeviceGetCount()

	if ret != nvml.SUCCESS {
		return m.getFallbackGPUStats()
	}

	gpus := make([]GPUInfo, 0, deviceCount)

	for i := 0; i < deviceCount; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)

		if ret != nvml.SUCCESS {
			continue
		}

		gpu := m.getGPUInfo(device, i)
		gpus = append(gpus, gpu)
	}

	return gpus
}

// getGPUInfo gets detailed information for a specific GPU
func (m *SystemMonitor) getGPUInfo(device nvml.Device, index int) GPUInfo {
	gpu := GPUInfo{
		Index: index,
	}

	// Get GPU name
	if name, ret := device.GetName(); ret == nvml.SUCCESS {
		gpu.Name = name
	}

	// Get memory info
	if memInfo, ret := device.GetMemoryInfo(); ret == nvml.SUCCESS {
		gpu.MemoryTotal = memInfo.Total
		gpu.MemoryUsed = memInfo.Used
		gpu.MemoryFree = memInfo.Free
		gpu.MemoryUsagePercent = float64(memInfo.Used) / float64(memInfo.Total) * 100
	}

	// Get utilization
	if utilization, ret := device.GetUtilizationRates(); ret == nvml.SUCCESS {
		gpu.GPUUtilization = utilization.Gpu
		gpu.MemoryUtilization = utilization.Memory
	}

	// Get temperature
	if temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
		gpu.Temperature = temp
	}

	// Get power usage
	if power, ret := device.GetPowerUsage(); ret == nvml.SUCCESS {
		gpu.PowerUsage = float64(power) / 1000.0 // Convert mW to W
	}

	// Get fan speed
	if fanSpeed, ret := device.GetFanSpeed(); ret == nvml.SUCCESS {
		gpu.FanSpeed = fanSpeed
	}

	return gpu
}

// getFallbackGPUStats attempts to get GPU information using system commands
// when NVML is not available (e.g., driver version mismatch)
func (m *SystemMonitor) getFallbackGPUStats() []GPUInfo {
	// Try to detect NVIDIA GPUs using lspci
	gpus := []GPUInfo{}

	// Look for NVIDIA GPUs in lspci output
	cmd := exec.Command("lspci")
	output, err := cmd.Output()
	if err != nil {
		return gpus
	}

	lines := strings.Split(string(output), "\n")
	gpuIndex := 0

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "nvidia") &&
			strings.Contains(strings.ToLower(line), "vga") {

			// Extract GPU name from lspci output
			parts := strings.Split(line, ": ")
			gpuName := "Unknown NVIDIA GPU"
			if len(parts) > 1 {
				// Remove revision info if present
				name := parts[1]
				if revIndex := strings.Index(name, " (rev "); revIndex != -1 {
					name = name[:revIndex]
				}
				gpuName = name
			}

			gpu := GPUInfo{
				Index:                gpuIndex,
				Name:                 gpuName,
				MemoryTotal:          0, // Can't get memory info without NVML
				MemoryUsed:           0,
				MemoryFree:           0,
				MemoryUsagePercent:   0.0,
				GPUUtilization:       0, // Can't get utilization without NVML
				MemoryUtilization:    0,
				Temperature:          0, // Can't get temperature without NVML
				PowerUsage:           0.0,
				FanSpeed:             0,
			}

			gpus = append(gpus, gpu)
			gpuIndex++
		}
	}

	return gpus
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

// Cleanup cleans up the monitor resources
func (m *SystemMonitor) Cleanup() {
	if m.nvmlInitialized {
		nvml.Shutdown()
	}
}
