package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level string `json:"level"`
	File  string `json:"file"`
}

// Config represents the agent configuration
type Config struct {
	AgentID            string                 `json:"agent_id"`
	BackendURL         string                 `json:"backend_url"`
	APIKey             string                 `json:"api_key"`
	HeartbeatInterval  int                    `json:"heartbeat_interval"`
	MaxPricePerHour    float64                `json:"max_price_per_hour"`
	AutoAcceptJobs     bool                   `json:"auto_accept_jobs"`
	GPUWhitelist       []string               `json:"gpu_whitelist"`
	Logging            LoggingConfig          `json:"logging"`
	ProviderInfo       map[string]interface{} `json:"provider_info"`
}

// Job represents a compute job
type Job struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	PricePerHour     float64                `json:"price_per_hour"`
	Requirements     map[string]interface{} `json:"requirements"`
	EstimatedDuration int                   `json:"estimated_duration"`
}

// Agent represents the main SkyOps GPU agent
type Agent struct {
	config            *Config
	logger            *logrus.Logger
	monitor           *SystemMonitor
	apiClient         *APIClient
	running           bool
	ctx               context.Context
	cancel            context.CancelFunc
	lastJobCheckLog   time.Time
	lastHeartbeatLog  time.Time
}

// NewAgent creates a new SkyOps agent instance
func NewAgent(configPath string) (*Agent, error) {
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	logger := setupLogging(config)
	monitor := NewSystemMonitor()
	
	// Create API client with the new interface
	apiClient := NewAPIClient(config.BackendURL, config.AgentID)
	apiClient.SetLogger(logger)

	ctx, cancel := context.WithCancel(context.Background())

	agent := &Agent{
		config:    config,
		logger:    logger,
		monitor:   monitor,
		apiClient: apiClient,
		running:   false,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Setup signal handlers for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-signalChan
		logger.Info("Received shutdown signal, initiating graceful shutdown...")
		agent.cancel()
	}()

	logger.WithField("agent_id", config.AgentID).Info("SkyOps GPU Agent initialized")
	return agent, nil
}

// loadConfig loads configuration from JSON file
func loadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configPath = filepath.Join(homeDir, ".skyops", "config.json")
	}

	   data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %v", err)
	}

	// Set defaults
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.File == "" {
		config.Logging.File = "agent.log"
	}
	
	// Generate dynamic agent ID if not set or if it's a placeholder
	if config.AgentID == "" || config.AgentID == "my-gpu-node" {
		// We'll let the server generate a unique agent ID based on hostname
		// For now, set a temporary ID that will be replaced by server response
		config.AgentID = generateDynamicAgentID()
		fmt.Printf("🆔 Will request unique agent ID based on hostname: %s\n", config.AgentID)
	}

	return &config, nil
}

// setupLogging configures the logger
func setupLogging(config *Config) *logrus.Logger {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(config.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Set up file logging with default path
	logFile := config.Logging.File
	if logFile == "" || !filepath.IsAbs(logFile) {
		homeDir, _ := os.UserHomeDir()
		logsDir := filepath.Join(homeDir, ".skyops", "logs")
		os.MkdirAll(logsDir, 0755)
		if logFile == "" {
			logFile = filepath.Join(logsDir, "agent.log")
		} else {
			// Make relative path absolute within logs directory
			logFile = filepath.Join(logsDir, logFile)
		}
	}

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// Silently fail and just use discard for cleaner CLI
		logger.SetOutput(io.Discard)
	} else {
		// Only log to file for cleaner CLI output
		logger.SetOutput(file)
	}

	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	return logger
}

// Start begins the agent's main loop
func (a *Agent) Start() error {
	a.running = true
	a.logger.Info("Starting SkyOps GPU Agent...")

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Register node
	if err := a.registerNode(); err != nil {
		return fmt.Errorf("node registration failed: %v", err)
	}

	// Start heartbeat ticker
	heartbeatTicker := time.NewTicker(time.Duration(a.config.HeartbeatInterval) * time.Second)
	defer heartbeatTicker.Stop()

	// Start job checker ticker
	jobTicker := time.NewTicker(10 * time.Second)
	defer jobTicker.Stop()

	a.logger.Info("Agent started successfully")

	// Main event loop
	for a.running {
		select {
		case <-heartbeatTicker.C:
			go a.sendHeartbeat()

		// case <-jobTicker.C:
		// 	go a.checkForJobs()

		case sig := <-sigChan:
			a.logger.WithField("signal", sig).Info("Received shutdown signal")
			a.Stop()

		case <-a.ctx.Done():
			a.logger.Info("Context cancelled, shutting down")
			a.running = false
		}
	}

	a.logger.Info("Agent stopped")
	return nil
}

// Stop gracefully stops the agent
func (a *Agent) Stop() {
	a.logger.Info("Stopping agent...")
	a.running = false
	a.cancel()
}

// authenticate with the SkyOps backend
func (a *Agent) authenticate() error {
	a.logger.Info("Authenticating with SkyOps backend...")
	return a.apiClient.Authenticate()
}

// registerNode registers this node with the SkyOps network
func (a *Agent) registerNode() error {
	a.logger.WithField("agent_id", a.config.AgentID).Info("Registering node with SkyOps network...")

	// Get system information for registration
	systemInfo := a.monitor.GetSystemInfo()
	gpuStats := a.monitor.GetGPUStats()

	// Auto-detect location if requested
	location := "Unknown"
	if autoDetect, ok := a.config.ProviderInfo["auto_detect_location"].(bool); ok && autoDetect {
		if detectedLocation, err := a.detectLocation(); err == nil {
			location = detectedLocation
			a.logger.WithField("location", location).Info("Auto-detected location")
		} else {
			a.logger.WithError(err).Warning("Could not auto-detect location")
		}
	} else if configLocation, ok := a.config.ProviderInfo["location"].(string); ok {
		location = configLocation
	}

	// Convert GPU stats to capabilities summary
	gpuCapabilities := GPUCapabilities{
		GPUCount: len(gpuStats),
		AllGPUs:  make([]GPUSummary, 0, len(gpuStats)),
	}
	
	if len(gpuStats) > 0 {
		firstGPU := gpuStats[0]
		gpuCapabilities.PrimaryGPUName = firstGPU.Name
		gpuCapabilities.TotalMemory = firstGPU.MemoryTotal
		gpuCapabilities.FreeMemory = firstGPU.MemoryFree
		
		if firstGPU.GPUUtilization > 0 {
			gpuCapabilities.GPUUtilization = &firstGPU.GPUUtilization
		}
		if firstGPU.MemoryUtilization > 0 {
			gpuCapabilities.MemoryUtilization = &firstGPU.MemoryUtilization
		}
		if firstGPU.Temperature > 0 {
			gpuCapabilities.Temperature = &firstGPU.Temperature
		}
		if firstGPU.PowerUsage > 0 {
			gpuCapabilities.PowerUsage = &firstGPU.PowerUsage
		}
		
		// Add summary of all GPUs
		for _, gpu := range gpuStats {
			summary := GPUSummary{
				Index:       gpu.Index,
				Name:        gpu.Name,
				MemoryTotal: gpu.MemoryTotal,
			}
			gpuCapabilities.AllGPUs = append(gpuCapabilities.AllGPUs, summary)
		}
	}

	// Create registration request using unified types
	regRequest := AgentRegisterRequest{
		Hostname:        systemInfo.Hostname,
		Location:        location,
		SystemInfo:      systemInfo,
		GPUCapabilities: gpuCapabilities,
		AutoAcceptJobs:  &a.config.AutoAcceptJobs,
	}
	
	if a.config.AgentID != "" {
		regRequest.AgentID = &a.config.AgentID
	}

	// Convert to map for the legacy API client
	nodeInfo := map[string]interface{}{
		"agent_id":          regRequest.AgentID,
		"hostname":          regRequest.Hostname,
		"location":          regRequest.Location,
		"gpu_capabilities":  regRequest.GPUCapabilities,
		"auto_accept_jobs":  regRequest.AutoAcceptJobs,
		"system_info":       regRequest.SystemInfo,
	}

	// Register and get the actual agent ID assigned by server
	actualAgentID, err := a.apiClient.RegisterNode(nodeInfo)
	if err != nil {
		return err
	}

	// Update our config with the actual agent ID
	if actualAgentID != a.config.AgentID {
		a.logger.WithFields(logrus.Fields{
			"old_agent_id": a.config.AgentID,
			"new_agent_id": actualAgentID,
		}).Info("Agent ID updated by server")
		
		a.config.AgentID = actualAgentID
		a.apiClient.UpdateAgentID(actualAgentID)
		
		// Save the updated config
		if err := a.saveConfig(); err != nil {
			a.logger.WithError(err).Warning("Failed to save updated config with new agent ID")
		}
	}

	return nil
}

// detectLocation detects the current location via IP geolocation
func (a *Agent) detectLocation() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data struct {
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s, %s, %s", data.City, data.Region, data.Country), nil
}

// sendHeartbeat sends a heartbeat with current system status
func (a *Agent) sendHeartbeat() {
	stats := a.monitor.GetCompleteStats()

	// Format the heartbeat payload to match FastAPI schema
	heartbeatPayload := a.formatHeartbeatPayload(stats)

	response, err := a.apiClient.SendHeartbeat(heartbeatPayload)
	if err != nil {
		a.logger.WithError(err).Warn("Heartbeat failed")
		fmt.Printf("❌ Heartbeat failed: %v\n", err)
		return
	}

	// Only log heartbeat success periodically to avoid spam
	now := time.Now()
	if a.lastHeartbeatLog.IsZero() || now.Sub(a.lastHeartbeatLog) >= 2*time.Minute {
		a.logger.Info("Heartbeat sent successfully")
		fmt.Printf("✅ [%s] Agent online - heartbeat OK\n", now.Format("15:04:05"))
		a.lastHeartbeatLog = now
	}

	// Process any commands from the backend
	if commands, ok := response["commands"].([]interface{}); ok {
		commandMaps := make([]map[string]interface{}, len(commands))
		for i, cmd := range commands {
			if cmdMap, ok := cmd.(map[string]interface{}); ok {
				commandMaps[i] = cmdMap
			}
		}
		a.processBackendCommands(commandMaps)
	}
}

// formatHeartbeatPayload formats the monitor stats into the simplified flat structure expected by the FastAPI server
func (a *Agent) formatHeartbeatPayload(stats AgentMonitoringData) map[string]interface{} {
	// Initialize with defaults
	payload := map[string]interface{}{
		"agent_id":       a.config.AgentID,
		"timestamp":      stats.Timestamp,
		"cpu_count":      stats.SystemInfo.CPUCount,
		"cpu_total":      100.0,
		"ram_total":      0.0,
		"disk_total":     0,
		"disk_free":      0,
		"gpu_name":       "",
		"gpu_ram_total":  0.0,
		"gpu_ram_free":   0.0,
	}

	// RAM stats - rounded to 1 decimal place
	ramTotalGB := float64(stats.MemoryStats.Total) / (1024 * 1024 * 1024)
	payload["ram_total"] = float64(int(ramTotalGB*10+0.5)) / 10 // Round to 1 decimal
	
	ramFreeGB := float64(stats.MemoryStats.Available) / (1024 * 1024 * 1024)
	payload["ram_free"] = float64(int(ramFreeGB*10+0.5)) / 10 // Round to 1 decimal

	// Disk stats - rounded to nearest whole number
	diskTotalGB := float64(stats.DiskStats.Total) / (1024 * 1024 * 1024)
	payload["disk_total"] = int(diskTotalGB + 0.5) // Round to whole number
	
	diskFreeGB := float64(stats.DiskStats.Free) / (1024 * 1024 * 1024)
	payload["disk_free"] = int(diskFreeGB + 0.5) // Round to whole number

	// GPU info - use first GPU if available
	if len(stats.GPUStats) > 0 {
		firstGPU := stats.GPUStats[0]
		payload["gpu_name"] = firstGPU.Name
		
		gpuRamTotalGB := float64(firstGPU.MemoryTotal) / (1024 * 1024 * 1024)
		payload["gpu_ram_total"] = float64(int(gpuRamTotalGB*10+0.5)) / 10 // Round to 1 decimal
		
		gpuRamFreeGB := float64(firstGPU.MemoryFree) / (1024 * 1024 * 1024)
		payload["gpu_ram_free"] = float64(int(gpuRamFreeGB*10+0.5)) / 10 // Round to 1 decimal
	}

	return payload
}

// checkForJobs checks for pending jobs and handles them
func (a *Agent) checkForJobs() {
	jobs, err := a.apiClient.GetPendingJobs()
	if err != nil {
		a.logger.WithError(err).Error("Failed to check for jobs")
		fmt.Printf("❌ Job check failed: %v\n", err)
		return
	}

	if len(jobs) == 0 {
		// Only log job checking success every 5 minutes to avoid spam
		now := time.Now()
		if a.lastJobCheckLog.IsZero() || now.Sub(a.lastJobCheckLog) >= 5*time.Minute {
			a.logger.Debug("Job check completed - no pending jobs")
			fmt.Printf("📋 [%s] No pending jobs - monitoring...\n", now.Format("15:04:05"))
			a.lastJobCheckLog = now
		}
		return
	}

	timestamp := time.Now().Format("15:04:05")
	a.logger.WithField("count", len(jobs)).Info("Found pending jobs")
	fmt.Printf("📋 [%s] Found %d pending jobs - processing...\n", timestamp, len(jobs))

	for _, job := range jobs {
		go a.handleJobOffer(job)
	}
}

// handleJobOffer handles a job offer
func (a *Agent) handleJobOffer(job map[string]interface{}) {
	jobID, ok := job["id"].(string)
	if !ok {
		a.logger.Error("Job missing ID field")
		return
	}

	logger := a.logger.WithField("job_id", jobID)
	logger.Info("Processing job offer")

	// Auto-accept logic based on configuration
	if a.config.AutoAcceptJobs {
		if err := a.apiClient.AcceptJob(jobID); err != nil {
			logger.WithError(err).Error("Failed to accept job")
		} else {
			logger.Info("Job accepted automatically")
		}
	} else {
		logger.Info("Job requires manual acceptance")
	}
}

// executeJob executes a compute job
func (a *Agent) executeJob(job Job) error {
	logger := a.logger.WithField("job_id", job.ID)
	logger.Info("Starting job execution")

	// Update job status to running
	if err := a.apiClient.UpdateJobStatus(job.ID, "running", nil); err != nil {
		return fmt.Errorf("failed to update job status: %v", err)
	}

	// Simulate job execution
	logger.WithField("duration", job.EstimatedDuration).Info("Executing job...")
	time.Sleep(time.Duration(job.EstimatedDuration) * time.Second)

	// Update job status to completed
	result := map[string]interface{}{
		"execution_time": job.EstimatedDuration,
		"result":        "Job completed successfully",
	}

	if err := a.apiClient.UpdateJobStatus(job.ID, "completed", result); err != nil {
		return fmt.Errorf("failed to update completion status: %v", err)
	}

	logger.Info("Job completed successfully")
	return nil
}

// processBackendCommands processes commands received from the backend
func (a *Agent) processBackendCommands(commands []map[string]interface{}) {
	for _, command := range commands {
		commandType, ok := command["type"].(string)
		if !ok {
			a.logger.Warning("Received command without type")
			continue
		}

		switch commandType {
		case "shutdown":
			a.logger.Info("Received shutdown command from backend")
			a.cancel()
		case "update_config":
			a.logger.Info("Received config update command")
			if config, ok := command["config"].(map[string]interface{}); ok {
				a.updateConfig(config)
			}
		case "restart_monitoring":
			a.logger.Info("Restarting monitoring subsystem")
			a.monitor = NewSystemMonitor()
		default:
			a.logger.WithField("type", commandType).Warning("Unknown command type")
		}
	}
}

// updateConfig updates agent configuration
func (a *Agent) updateConfig(newConfig map[string]interface{}) {
	if maxPrice, ok := newConfig["max_price_per_hour"].(float64); ok {
		a.config.MaxPricePerHour = maxPrice
	}
	
	if autoAccept, ok := newConfig["auto_accept_jobs"].(bool); ok {
		a.config.AutoAcceptJobs = autoAccept
	}
	
	if interval, ok := newConfig["heartbeat_interval"].(float64); ok {
		a.config.HeartbeatInterval = int(interval)
	}
	
	a.logger.Info("Configuration updated successfully")
}

// cleanup performs cleanup before shutdown
func (a *Agent) cleanup() {
	a.logger.Info("Cleaning up agent resources...")
	
	// Send final status update with simplified flat schema
	finalHeartbeat := map[string]interface{}{
		"agent_id":       a.config.AgentID,
		"timestamp":      time.Now().Unix(),
		"cpu_count":      0,
		"ram_total":      0.0,
		"ram_free":       0.0,
		"disk_total":     0,
		"disk_free":      0,
		"gpu_name":       "",
		"gpu_ram_total":  0.0,
		"gpu_ram_free":   0.0,
	}
	a.apiClient.SendHeartbeat(finalHeartbeat)
	
	a.logger.Info("Agent cleanup completed")
}

// generateDynamicAgentID generates an agent ID based on hostname
func generateDynamicAgentID() string {
	hostname, err := os.Hostname()
	if err != nil {
		// Fallback to a generic name if hostname fails
		hostname = "unknown"
	}
	// Clean hostname to make it a valid agent ID
	hostname = strings.ReplaceAll(hostname, " ", "_")
	hostname = strings.ReplaceAll(hostname, ".", "_")
	return fmt.Sprintf("skyops_node_%s", hostname)
}

func main() {
	if len(os.Args) < 2 {
		// Default behavior - start agent
		handleStartCommand([]string{"start"})
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "login":
		handleLoginCommand(args)
	case "register":
		handleRegisterCommand(args)
	case "start":
		handleStartCommand(args)
	case "status":
		handleStatusCommand(args)
	case "stop":
		handleStopCommand(args)
	case "ping":
		handlePingCommand(args)
	case "version", "--version", "-v":
		fmt.Println("SkyOps GPU Agent v1.0.0")
	case "help", "--help", "-h":
		showHelp()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showHelp()
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println(`SkyOps GPU Provider Agent - Join the decentralized GPU network

Usage:
  skyops [command] [flags]

Available Commands:
  login       Authenticate with the SkyOps network
  register    Register this node with the SkyOps network
  start       Start the agent daemon
  status      Check agent status
  stop        Stop the agent daemon
  ping        Test heartbeat connection to SkyOps network
  version     Show version information
  help        Show this help message

Examples:
  skyops login        # Authenticate with the SkyOps network
  skyops register     # Register this node (must be authenticated)
  skyops start        # Start the agent daemon
  skyops status       # Check agent status
  skyops stop         # Stop the agent daemon
  skyops ping         # Test heartbeat connection

Flags:
  --config string     Path to configuration file (default: ~/.skyops/config.json)
  --daemon           Run as background daemon (for start command)
  --help             Show help information`)
}

func handleLoginCommand(args []string) {
		
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".skyops", "config.json")
	for i, arg := range args {
		if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
		}
	}

	agent, err := NewAgent(configPath)
	if err != nil {
		fmt.Printf("❌ Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}
	defer agent.cleanup()

	// Check if already logged in
	if agent.apiClient.ValidateToken() {
		fmt.Println("✅ Already logged in!")
		return
	}

	fmt.Println("🔐 Starting SkyOps authentication flow...")
	if err := agent.apiClient.Authenticate(); err != nil {
		fmt.Println("❌ Authentication failed")
		os.Exit(1)
	}
	fmt.Println("✅ Authentication successful!")
}

func handleRegisterCommand(args []string) {
	homeDir, _ := os.UserHomeDir()
	var configPath string = filepath.Join(homeDir, ".skyops", "config.json")
	// Parse flags
	for i, arg := range args {
		if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
		}
	}

	fmt.Println("🚀 SkyOps GPU Node Registration")
	fmt.Println(strings.Repeat("=", 50))

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("❌ Configuration file '%s' not found\n", configPath)
		homeDir, _ := os.UserHomeDir()
		defaultConfig := filepath.Join(homeDir, ".skyops", "config.json")
		fmt.Printf("💡 Create the config file or specify a different one with --config\n")
		fmt.Printf("📄 Default location: %s\n", defaultConfig)
		os.Exit(1)
	}

	agent, err := NewAgent(configPath)
	if err != nil {
		fmt.Printf("❌ Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}
	defer agent.cleanup()

	// Disable verbose logging for cleaner output
	agent.logger.SetOutput(io.Discard)
	agent.apiClient.SetLogger(logrus.New())
	agent.apiClient.logger.SetOutput(io.Discard)

	// Only check authentication, do not authenticate here
	if !agent.apiClient.ValidateToken() {
		fmt.Println("❌ Not authenticated. Please run 'skyops login' to authenticate before registering.")
		os.Exit(1)
	}

	// Step 1: Registration
	initialAgentID := agent.config.AgentID
	fmt.Printf("📝 Registering node with SkyOps network (Initial Agent ID: %s)...\n", initialAgentID)
	if err := agent.registerNode(); err != nil {
		fmt.Printf("❌ Node registration failed: %v\n", err)
		os.Exit(1)
	}

	// Show final agent ID (may have been updated by server)
	if agent.config.AgentID != initialAgentID {
		fmt.Printf("✅ Node registration completed successfully with Agent ID: %s\n", agent.config.AgentID)
	} else {
		fmt.Println("✅ Node registration completed successfully!")
	}
	fmt.Println("💡 You can now start the agent with: skyops start")
}

func handleStartCommand(args []string) {
	homeDir, _ := os.UserHomeDir()
	var configPath string = filepath.Join(homeDir, ".skyops", "config.json")
	var daemon bool = false

	// Parse flags
	for i, arg := range args {
		if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
		} else if arg == "--daemon" {
			daemon = true
		}
	}

	fmt.Println("🚀 Starting SkyOps GPU Agent...")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("❌ Configuration file '%s' not found\n", configPath)
		fmt.Println("Please run 'skyops register' first to set up your node")
		os.Exit(1)
	}

	agent, err := NewAgent(configPath)
	if err != nil {
		fmt.Printf("❌ Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}
	defer agent.cleanup()

	// Disable verbose logging for cleaner output
	agent.logger.SetOutput(io.Discard)
	agent.apiClient.SetLogger(logrus.New())
	agent.apiClient.logger.SetOutput(io.Discard)

	// Check authentication
	fmt.Println("🔐 Checking authentication...")
	if !agent.apiClient.ValidateToken() {
		fmt.Println("🔐 Authentication required...")
		if err := agent.apiClient.Authenticate(); err != nil {
			fmt.Println("❌ Authentication failed. Please run 'skyops register' first")
			os.Exit(1)
		}
	} else {
		fmt.Println("✅ Using saved authentication token")
	}

	if daemon {
		fmt.Println("🔧 Starting as daemon...")
		// TODO: Implement proper daemon mode
	}

	fmt.Println("✅ Agent started successfully")
	fmt.Println("📊 Sending heartbeats to network...")
	fmt.Println("Press Ctrl+C to stop")

	// Start the agent
	if err := agent.Start(); err != nil {
		fmt.Printf("❌ Agent error: %v\n", err)
		os.Exit(1)
	}
}

// isAgentRunning checks if the skyops agent is currently running
func isAgentRunning() bool {
	// Simple process check - look for skyops processes
	cmd := exec.Command("pgrep", "-f", "skyops.*start")
	err := cmd.Run()
	return err == nil
}

func handleStatusCommand(args []string) {
	fmt.Println("📊 SkyOps Agent Status")
	fmt.Println(strings.Repeat("=", 30))

	// Check if config exists
	homeDir, _ := os.UserHomeDir()
	defaultConfig := filepath.Join(homeDir, ".skyops", "config.json")
	if _, err := os.Stat(defaultConfig); os.IsNotExist(err) {
		fmt.Println("❌ Status: Not configured")
		fmt.Println("💡 Run 'skyops register' to set up your node")
		return
	}

	// Load config to get agent info
	   configData, err := os.ReadFile(defaultConfig)
	if err != nil {
		fmt.Println("❌ Status: Configuration file corrupted")
		return
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		fmt.Println("❌ Status: Configuration file corrupted")
		return
	}

	// Create temporary client to check auth status
	client := NewAPIClient("http://localhost:8000", config.AgentID)
	
	// Check if agent is currently running
	agentRunning := isAgentRunning()
	
	if client.ValidateToken() {
		fmt.Printf("✅ Status: Configured and Authenticated (Agent ID: %s)\n", config.AgentID)
		fmt.Println("🔑 Authentication: Valid")
		
		if agentRunning {
			fmt.Println("🚀 Agent: Currently Running")
			fmt.Println("📊 Network Status: Online - Sending heartbeats")
			fmt.Println("💡 Agent is actively providing GPU resources")
		} else {
			fmt.Println("⏸️  Agent: Not Running")
			fmt.Println("📊 Network Status: Offline")
			fmt.Println("💡 Run 'skyops start' to begin providing GPU resources")
		}
	} else {
		savedToken := client.LoadSavedToken()
		if savedToken != "" {
			fmt.Println("⚠️  Status: Configured but Token Invalid")
			fmt.Println("🔑 Authentication: Token expired/invalid")
			if agentRunning {
				fmt.Println("⚠️  Agent: Running but authentication failed")
			}
			fmt.Println("💡 Run 'skyops register' to re-authenticate")
		} else {
			fmt.Println("⚠️  Status: Configured but Not Authenticated")
			fmt.Println("🔑 Authentication: No saved token")
			if agentRunning {
				fmt.Println("⚠️  Agent: Running but not authenticated")
			}
			fmt.Println("💡 Run 'skyops register' to authenticate")
		}
	}
}

func handleStopCommand(args []string) {
	fmt.Println("🛑 Stopping SkyOps Agent...")
	
	// TODO: Add logic to gracefully stop running agent
	// This could involve PID files, signal handling, etc.
	fmt.Println("✅ Agent stopped")
}

// saveConfig saves the current configuration to the config file
func (a *Agent) saveConfig() error {
	// Determine config file path
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".skyops", "config.json")

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Marshal config to JSON
	configData, err := json.MarshalIndent(a.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	a.logger.WithField("config_path", configPath).Debug("Configuration saved")
	return nil
}

func handlePingCommand(args []string) {
	homeDir, _ := os.UserHomeDir()
	var configPath string = filepath.Join(homeDir, ".skyops", "config.json")
	var verbose bool = false

	// Parse flags
	for i, arg := range args {
		if arg == "--config" && i+1 < len(args) {
			configPath = args[i+1]
		} else if arg == "--verbose" || arg == "-v" {
			verbose = true
		} else if arg == "--help" || arg == "-h" {
			fmt.Println(`📡 SkyOps Ping Command

Usage:
  skyops ping [flags]

Description:
  Test the heartbeat connection to the SkyOps network

Flags:
  --config string     Path to configuration file (default: ~/.skyops/config.json)
  --verbose, -v       Show detailed output and logs
  --help, -h          Show this help message

Examples:
  skyops ping         # Test heartbeat with clean output
  skyops ping -v      # Test with verbose output
  skyops ping --config /path/to/config.json  # Test with custom config`)
			return
		}
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("❌ Configuration file not found. Please run 'skyops register' first.")
		return
	}

	// Load config and create agent
	agent, err := NewAgent(configPath)
	if err != nil {
		fmt.Printf("❌ Failed to load agent config: %v\n", err)
		return
	}
	// Note: No defer cleanup() for ping command since it's just a test

	// Disable logging output unless verbose mode
	if !verbose {
		agent.logger.SetOutput(io.Discard)
		agent.apiClient.SetLogger(logrus.New())
		agent.apiClient.logger.SetOutput(io.Discard)
	}

	if verbose {
		fmt.Println("🏓 Testing SkyOps Network Connection...")
		fmt.Println(strings.Repeat("=", 40))
		fmt.Println("🔐 Testing authentication...")
	} else {
		fmt.Print("📡 Testing connection... ")
	}

	// Test authentication
	if !agent.apiClient.ValidateToken() {
		if verbose {
			fmt.Println("❌ Authentication failed")
			fmt.Println("💡 Please run 'skyops register' to authenticate")
		} else {
			fmt.Println("❌ Authentication failed. Run 'skyops register' first.")
		}
		return
	}
	
	if verbose {
		fmt.Println("✅ Authentication successful")
		fmt.Println("📊 Collecting system information...")
	}

	// Get system stats and format heartbeat
	stats := agent.monitor.GetCompleteStats()
	heartbeatPayload := agent.formatHeartbeatPayload(stats)
	
	if verbose {
		fmt.Println("💓 Sending test heartbeat...")
		if payloadJSON, err := json.MarshalIndent(heartbeatPayload, "", "  "); err == nil {
			fmt.Printf("Heartbeat payload:\n%s\n", string(payloadJSON))
		}
	}

	// Send heartbeat
	response, err := agent.apiClient.SendHeartbeat(heartbeatPayload)
	if err != nil {
		if verbose {
			fmt.Printf("❌ Heartbeat failed: %v\n", err)
			fmt.Println("💡 Check your network connection and server status")
		} else {
			fmt.Printf("❌ Failed: %v\n", err)
		}
		return
	}

	if verbose {
		fmt.Println("✅ Heartbeat successful!")
		if response != nil {
			if responseJSON, err := json.MarshalIndent(response, "", "  "); err == nil {
				fmt.Printf("Server response:\n%s\n", string(responseJSON))
			}
		}
		fmt.Println(strings.Repeat("=", 40))
		fmt.Printf("🎉 Connection test completed successfully!\n")
		fmt.Printf("📍 Agent ID: %s\n", agent.config.AgentID)
		if nextHeartbeat, ok := response["next_heartbeat_seconds"].(float64); ok {
			fmt.Printf("⏱️  Next heartbeat in: %.0f seconds\n", nextHeartbeat)
		}
	} else {
		fmt.Println("✅ Success!")
	}
}
