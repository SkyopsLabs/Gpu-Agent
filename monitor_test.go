package main

import (
	"fmt"
	"testing"
)

// func TestNewSystemMonitor(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	if monitor == nil {
// 		t.Fatal("Expected non-nil SystemMonitor")
// 	}
// }

// func TestGetSystemInfo(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	sysInfo := monitor.GetSystemInfo()
// 	if sysInfo.Hostname == "" {
// 		t.Error("Expected Hostname to be set")
// 	}
// 	if sysInfo.CPUCount <= 0 {
// 		t.Error("Expected CPUCount > 0")
// 	}
// }

// func TestGetCPUStats(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	cpuStats := monitor.GetCPUStats()
// 	if cpuStats.CoreCount <= 0 {
// 		t.Error("Expected CoreCount > 0")
// 	}
// }

// func TestGetMemoryStats(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	memStats := monitor.GetMemoryStats()
// 	if memStats.Total == 0 {
// 		t.Error("Expected Total memory > 0")
// 	}
// }

// func TestGetDiskStats(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	diskStats := monitor.GetDiskStats()
// 	if diskStats.Total == 0 {
// 		t.Error("Expected Total disk > 0")
// 	}
// }

// func TestGetGPUStats(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	gpus := monitor.GetGPUStats()
// 	// No assertion on count, but should not panic or error
// 	for _, gpu := range gpus {
// 		if gpu.Name == "" {
// 			t.Error("Expected GPU Name to be set")
// 		}
// 	}
// }

func TestGetGPUStats_Print(t *testing.T) {
	monitor := NewSystemMonitor()
	gpus := monitor.GetGPUStats()
	fmt.Println("GetGPUStats output:")
	for _, gpu := range gpus {
		fmt.Printf("GPU %d: %+v\n", gpu.Index, gpu)
	}
}



// func TestGetCompleteStats(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	stats := monitor.GetCompleteStats()
// 	if stats.SystemInfo.Hostname == "" {
// 		t.Error("Expected SystemInfo.Hostname to be set")
// 	}
// 	if stats.CPUStats.CoreCount <= 0 {
// 		t.Error("Expected CPUStats.CoreCount > 0")
// 	}
// }

// func TestCleanup(t *testing.T) {
// 	monitor := NewSystemMonitor()
// 	monitor.Cleanup()
// 	// No assertion, just ensure no panic
// }
