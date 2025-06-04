//go:build unix

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	pidFilePath = "/tmp/skyops-node.pid"
	logFilePath = "/tmp/skyops-node.log"
)

// startDaemon starts the agent as a daemon process on Unix systems
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

	// Start the process in background
	cmd := exec.Command(execPath, "start", "--no-daemon")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start daemon: %v", err)
	}

	// Write PID file
	if err := writePidFile(cmd.Process.Pid); err != nil {
		logFile.Close()
		cmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %v", err)
	}

	logFile.Close()
	fmt.Printf("✅ Daemon started with PID %d\n", cmd.Process.Pid)
	fmt.Printf("📝 Logs: %s\n", logFilePath)
	return nil
}

// stopDaemon stops the daemon process on Unix systems
func stopDaemon() error {
	pid, err := readPidFile()
	if err != nil {
		return fmt.Errorf("daemon is not running or PID file not found")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		removePidFile()
		return fmt.Errorf("failed to find process with PID %d", pid)
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead, try to clean up
		removePidFile()
		return fmt.Errorf("failed to terminate process: %v", err)
	}

	// Wait a bit and check if process is still alive
	// If it is, send SIGKILL
	for i := 0; i < 30; i++ { // Wait up to 3 seconds
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process is dead
			removePidFile()
			fmt.Println("✅ Daemon stopped successfully")
			return nil
		}
		// Wait 100ms
		cmd := exec.Command("sleep", "0.1")
		cmd.Run()
	}

	// Force kill
	process.Signal(syscall.SIGKILL)
	removePidFile()
	fmt.Println("✅ Daemon force stopped")
	return nil
}

// isDaemonRunning checks if the daemon is currently running
func isDaemonRunning() bool {
	return isAlreadyRunning()
}

// isAlreadyRunning checks if a daemon process is already running
func isAlreadyRunning() bool {
	pid, err := readPidFile()
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		removePidFile()
		return false
	}

	// Check if process is still alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		removePidFile()
		return false
	}

	return true
}

// readPidFile reads the PID from the PID file
func readPidFile() (int, error) {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// writePidFile writes the PID to the PID file
func writePidFile(pid int) error {
	pidDir := filepath.Dir(pidFilePath)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(pidFilePath, []byte(strconv.Itoa(pid)), 0644)
}

// removePidFile removes the PID file
func removePidFile() {
	os.Remove(pidFilePath)
}

// redirectOutput redirects stdout and stderr to log file for daemon mode
func redirectOutput() error {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// For cross-platform compatibility, we'll handle redirection differently
	// The actual redirection is handled in the startDaemon function via cmd.Stdout/Stderr
	// This function is mainly for marking that we're in daemon mode
	logFile.Close()
	return nil
}

// stopSkyOpsProcesses finds and stops running skyops processes on Unix systems
func stopSkyOpsProcesses() int {
	// Get the current executable path for comparison
	currentExecPath, err := os.Executable()
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not determine current executable path: %v\n", err)
		return stopSkyOpsProcessesFallback()
	}

	// Resolve any symlinks to get the real path
	currentExecPath, err = filepath.EvalSymlinks(currentExecPath)
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not resolve executable symlinks: %v\n", err)
		return stopSkyOpsProcessesFallback()
	}

	// Find processes using a more specific pattern
	cmd := exec.Command("pgrep", "-f", "skyops.*start")
	output, err := cmd.Output()
	if err != nil {
		// No processes found
		return 0
	}

	pids := strings.Fields(string(output))
	stoppedCount := 0

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == os.Getpid() {
			continue // Skip invalid PIDs and our own process
		}

		// First, check if the executable path matches our current binary
		execPath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			// Process might have died or we don't have permission
			continue
		}

		// Resolve symlinks for comparison
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			continue
		}

		// Only proceed if the executable is the same as ours
		if execPath != currentExecPath {
			continue
		}

		// Double-check by reading the command line
		cmdLineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}

		// cmdline is null-separated, convert to space-separated for easier parsing
		cmdLine := strings.ReplaceAll(string(cmdLineBytes), "\x00", " ")
		cmdLine = strings.TrimSpace(cmdLine)

		// Verify this is actually a skyops start command
		if !isValidSkyOpsStartCommand(cmdLine) {
			continue
		}

		fmt.Printf("🔄 Stopping skyops agent process %d...\n", pid)

		// Try graceful shutdown first (SIGTERM)
		if err := syscall.Kill(pid, syscall.SIGTERM); err == nil {
			// Wait up to 5 seconds for graceful shutdown
			gracefulShutdown := false
			for i := 0; i < 50; i++ { // 50 * 100ms = 5 seconds
				if err := syscall.Kill(pid, 0); err != nil {
					// Process is gone, graceful shutdown worked
					fmt.Printf("✅ Process %d stopped gracefully\n", pid)
					gracefulShutdown = true
					stoppedCount++
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			if !gracefulShutdown {
				// Force kill if graceful didn't work
				fmt.Printf("⚡ Force stopping process %d...\n", pid)
				if err := syscall.Kill(pid, syscall.SIGKILL); err == nil {
					stoppedCount++
					fmt.Printf("✅ Process %d force stopped\n", pid)
				}
			}
		}
	}

	return stoppedCount
}

// stopSkyOpsProcessesFallback provides a fallback method when executable path comparison fails
func stopSkyOpsProcessesFallback() int {
	fmt.Println("🔄 Using fallback process detection method...")
	
	cmd := exec.Command("pgrep", "-f", "skyops.*start")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	pids := strings.Fields(string(output))
	stoppedCount := 0

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == os.Getpid() {
			continue
		}

		// Read the command line
		cmdLineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}

		cmdLine := strings.ReplaceAll(string(cmdLineBytes), "\x00", " ")
		cmdLine = strings.TrimSpace(cmdLine)

		// Only stop if this looks like a valid skyops start command
		if isValidSkyOpsStartCommand(cmdLine) {
			fmt.Printf("🔄 Stopping process %d...\n", pid)
			if err := syscall.Kill(pid, syscall.SIGTERM); err == nil {
				time.Sleep(2 * time.Second)
				if err := syscall.Kill(pid, 0); err != nil {
					fmt.Printf("✅ Process %d stopped gracefully\n", pid)
					stoppedCount++
				} else {
					syscall.Kill(pid, syscall.SIGKILL)
					stoppedCount++
					fmt.Printf("✅ Process %d force stopped\n", pid)
				}
			}
		}
	}

	return stoppedCount
}

// isValidSkyOpsStartCommand checks if a command line represents a valid skyops start command
func isValidSkyOpsStartCommand(cmdLine string) bool {
	// Must contain "skyops" and "start"
	if !strings.Contains(cmdLine, "skyops") || !strings.Contains(cmdLine, "start") {
		return false
	}

	// Exclude SSH connections and other network-related processes
	excludePatterns := []string{
		"ssh", "@", "connect", "scp", "rsync", "sftp",
		"git", "clone", "pull", "push", "fetch",
		"curl", "wget", "http", "https",
	}

	cmdLineLower := strings.ToLower(cmdLine)
	for _, pattern := range excludePatterns {
		if strings.Contains(cmdLineLower, pattern) {
			return false
		}
	}

	// Should contain "skyops start" as a sequence
	return strings.Contains(cmdLine, "skyops start") || 
		   strings.Contains(cmdLine, "skyops\x00start") // null-separated version
}

// isAgentRunning checks if a skyops agent process is currently running on Unix systems
func isAgentRunning() bool {
	// Get the current executable path for comparison
	currentExecPath, err := os.Executable()
	if err != nil {
		// Fallback to less precise method if we can't get executable path
		return isAgentRunningFallback()
	}

	// Resolve any symlinks to get the real path
	currentExecPath, err = filepath.EvalSymlinks(currentExecPath)
	if err != nil {
		return isAgentRunningFallback()
	}

	// Find processes that might be skyops agents
	cmd := exec.Command("pgrep", "-f", "skyops.*start")
	output, err := cmd.Output()
	if err != nil {
		return false // No matching processes found
	}

	pids := strings.Fields(string(output))
	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == os.Getpid() {
			continue // Skip invalid PIDs and our own process
		}

		// Check if the executable path matches our current binary
		execPath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			// Process might have died or we don't have permission, skip
			continue
		}

		// Resolve symlinks for comparison
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			continue
		}

		// If executable paths match, verify it's running a start command
		if execPath == currentExecPath {
			// Read the command line to confirm it's a start command
			cmdLineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
			if err != nil {
				continue
			}

			// cmdline is null-separated, convert for easier parsing
			cmdLine := strings.ReplaceAll(string(cmdLineBytes), "\x00", " ")
			cmdLine = strings.TrimSpace(cmdLine)

			// Verify this is a valid skyops start command
			if isValidSkyOpsStartCommand(cmdLine) {
				return true
			}
		}
	}

	return false
}

// isAgentRunningFallback provides a fallback method for checking running agents
func isAgentRunningFallback() bool {
	// Use pgrep with more specific pattern
	cmd := exec.Command("pgrep", "-f", "skyops.*start")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	pids := strings.Fields(string(output))
	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == os.Getpid() {
			continue
		}
		
		// Read the command line of the process
		cmdLineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		
		// cmdline is null-separated, convert for easier parsing
		cmdLine := strings.ReplaceAll(string(cmdLineBytes), "\x00", " ")
		cmdLine = strings.TrimSpace(cmdLine)
		
		// Use the same validation logic as the main function
		if isValidSkyOpsStartCommand(cmdLine) {
			return true
		}
	}
	return false
}
