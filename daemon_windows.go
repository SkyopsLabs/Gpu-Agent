//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	pidFilePath = "skyops-node.pid"
	logFilePath = "skyops-node.log"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procCreateProcess  = kernel32.NewProc("CreateProcessW")
	procOpenProcess    = kernel32.NewProc("OpenProcess")
	procTerminateProcess = kernel32.NewProc("TerminateProcess")
	procCloseHandle    = kernel32.NewProc("CloseHandle")
)

// startDaemon starts the agent as a daemon process on Windows
func startDaemon() error {
	// Check if already running
	if isAlreadyRunning() {
		return fmt.Errorf("daemon is already running")
	}

	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	// Create log file
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %v", err)
	}
	defer logFile.Close()

	// Start the process in background using Windows-specific method
	cmd := exec.Command(execPath, "start", "--no-daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	// Redirect output to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %v", err)
	}

	// Write PID file
	if err := writePidFile(cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %v", err)
	}

	fmt.Printf("✅ Daemon started with PID %d\n", cmd.Process.Pid)
	fmt.Printf("📝 Logs: %s\n", logFilePath)
	return nil
}

// stopDaemon stops the daemon process on Windows
func stopDaemon() error {
	pid, err := readPidFile()
	if err != nil {
		return fmt.Errorf("daemon is not running or PID file not found")
	}

	// Open process handle
	handle, _, _ := procOpenProcess.Call(uintptr(syscall.PROCESS_TERMINATE), 0, uintptr(pid))
	if handle == 0 {
		removePidFile()
		return fmt.Errorf("failed to open process with PID %d", pid)
	}
	defer procCloseHandle.Call(handle)

	// Terminate process
	ret, _, _ := procTerminateProcess.Call(handle, 0)
	if ret == 0 {
		return fmt.Errorf("failed to terminate process with PID %d", pid)
	}

	removePidFile()
	fmt.Println("✅ Daemon stopped successfully")
	return nil
}

// isDaemonRunning checks if the daemon is currently running
func isDaemonRunning() bool {
	return isAlreadyRunning()
}

// isAlreadyRunning checks if a daemon process is already running on Windows
func isAlreadyRunning() bool {
	pid, err := readPidFile()
	if err != nil {
		return false
	}

	// Try to open the process
	handle, _, _ := procOpenProcess.Call(uintptr(syscall.PROCESS_QUERY_INFORMATION), 0, uintptr(pid))
	if handle == 0 {
		removePidFile()
		return false
	}
	defer procCloseHandle.Call(handle)

	return true
}

// Alternative method using tasklist command
func isProcessRunning(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), strconv.Itoa(pid))
}

// readPidFile reads the PID from the PID file
func readPidFile() (int, error) {
	// Try current directory first, then temp directory
	paths := []string{pidFilePath, filepath.Join(os.TempDir(), pidFilePath)}
	
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}

		return pid, nil
	}

	return 0, fmt.Errorf("PID file not found")
}

// writePidFile writes the PID to the PID file
func writePidFile(pid int) error {
	// Write to current directory
	return os.WriteFile(pidFilePath, []byte(strconv.Itoa(pid)), 0644)
}

// removePidFile removes the PID file
func removePidFile() {
	os.Remove(pidFilePath)
	os.Remove(filepath.Join(os.TempDir(), pidFilePath))
}

// redirectOutput redirects stdout and stderr to log file for daemon mode
func redirectOutput() error {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// On Windows, we'll handle this differently
	// The redirection is handled in the startDaemon function
	logFile.Close()
	return nil
}

// stopSkyOpsProcesses finds and stops running skyops processes on Windows
func stopSkyOpsProcesses() int {
	// Use tasklist to find skyops processes
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	stoppedCount := 0

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "skyops") {
			// Parse CSV line to get process name and PID
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				pidStr := strings.Trim(parts[1], `"`)
				if pid, err := strconv.Atoi(pidStr); err == nil && pid != os.Getpid() {
					fmt.Printf("🔄 Stopping process %d...\n", pid)
					
					// Use taskkill to terminate the process
					killCmd := exec.Command("taskkill", "/PID", pidStr, "/F")
					if err := killCmd.Run(); err == nil {
						fmt.Printf("✅ Process %d stopped\n", pid)
						stoppedCount++
					}
				}
			}
		}
	}

	return stoppedCount
}

// isAgentRunning checks if a skyops agent process is currently running on Windows
func isAgentRunning() bool {
	// Use tasklist to find skyops processes
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "skyops") {
			// Parse CSV line to get process name and PID
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				pidStr := strings.Trim(parts[1], `"`)
				if pid, err := strconv.Atoi(pidStr); err == nil && pid != os.Getpid() {
					// Found a skyops process that's not our own
					return true
				}
			}
		}
	}
	return false
}
