package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

//go:embed .env
var embeddedEnvFile embed.FS

// Global API client instance
var globalAPIClient *APIClient

// Global agent instance
var globalAgent *Agent

// Global config instance
var globalConfig *Config

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level string `json:"level"`
	File  string `json:"file"`
}

// Config represents the agent configuration
type Config struct {
	AgentID            string                 `json:"agent_id"`
	WalletAddress      string                 `json:"wallet_address"`
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
	lastHeartbeatLog  time.Time
}

// NewAgent creates a new SkyOps agent instance
func NewAgent() (*Agent, error) {
	config, err := getOrLoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	logger := setupLogging(config)
	monitor := NewSystemMonitor()
	
	// Create API client with the new interface
	apiClient := NewAPIClient(config.BackendURL)
	apiClient.SetLogger(logger)
	
	// Copy authentication token from global client if available
	if globalAPIClient != nil && globalAPIClient.GetAuthToken() != "" {
		apiClient.SetAuthToken(globalAPIClient.GetAuthToken())
	} else {
		// Try to load saved token
		apiClient.LoadSavedToken()
	}
	
	// Set wallet address if available in config
	if config.WalletAddress != "" {
		apiClient.SetWalletAddress(config.WalletAddress)
	}

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

// loadConfig loads configuration from environment variables
func loadConfig() (*Config, error) {
	// First try to load embedded .env file
	if envData, err := embeddedEnvFile.ReadFile(".env"); err == nil {
		envMap, err := godotenv.Unmarshal(string(envData))
		if err == nil {
			// Set environment variables from embedded file if not already set
			for key, value := range envMap {
				if os.Getenv(key) == "" {
					os.Setenv(key, value)
				}
			}
		}
	}
	
	// Also try to load local .env file if it exists (for development)
	_ = godotenv.Load()

	config := &Config{}

	// Read from environment variables
	config.BackendURL = os.Getenv("BACKEND_URL")
	if config.BackendURL == "" {
		return nil, fmt.Errorf("BACKEND_URL environment variable is required")
	}
	
	config.APIKey = os.Getenv("API_KEY")

	if v := os.Getenv("HEARTBEAT_INTERVAL"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			config.HeartbeatInterval = i
		}
	}
	if v := os.Getenv("MAX_PRICE_PER_HOUR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			config.MaxPricePerHour = f
		}
	}
	if v := os.Getenv("AUTO_ACCEPT_JOBS"); v != "" {
		config.AutoAcceptJobs = (v == "true" || v == "1")
	}
	if v := os.Getenv("GPU_WHITELIST"); v != "" {
		// Comma-separated list
		config.GPUWhitelist = strings.Split(v, ",")
	}

	config.Logging.Level = os.Getenv("LOG_LEVEL")
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	config.Logging.File = os.Getenv("LOG_FILE")
	if config.Logging.File == "" {
		config.Logging.File = "agent.log"
	}

	// Generate dynamic agent ID if not set or if it's a placeholder
	if config.AgentID == "" {
		config.AgentID = generateDynamicAgentID()
		// fmt.Printf("🆔 %s\n", config.AgentID)
	}

	// Set default heartbeat interval if not set
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30
	}

	return config, nil
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
	// Get system information for registration
	systemInfo := a.monitor.GetSystemInfo()
	gpuStats := a.monitor.GetGPUStats()

	// Always auto-detect location
	location := "Unknown"
	if detectedLocation, err := a.detectLocation(); err == nil {
		location = detectedLocation
		a.logger.WithField("location", location).Info("Auto-detected location")
	} else {
		a.logger.WithError(err).Warning("Could not auto-detect location")
	}

	// Log to the screen if no GPU found
	if len(gpuStats) == 0 {
		fmt.Println("⚠️  No GPU found on this node")
		a.logger.Warning("No GPU found on this node")
	}

	// Create registration request using unified types
	regRequest := AgentRegisterRequest{
		Hostname:       systemInfo.Hostname,
		Location:       location,
		SystemInfo:     systemInfo,
		AutoAcceptJobs: &a.config.AutoAcceptJobs,
	}

	// Convert to map for the legacy API client
	nodeInfo := map[string]interface{}{
		"hostname":         regRequest.Hostname,
		"location":         regRequest.Location,
		"auto_accept_jobs": regRequest.AutoAcceptJobs,
		"system_info":      regRequest.SystemInfo,
	}

	// Register and get the actual agent ID assigned by server
	_, err := a.apiClient.RegisterNode(nodeInfo)
	if err != nil {
		return err
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
		IP       string `json:"ip"`
		City     string `json:"city"`
		Region   string `json:"region"`
		Country  string `json:"country"`
		Loc      string `json:"loc"`
		Org      string `json:"org"`
		Timezone string `json:"timezone"`
		Readme   string `json:"readme"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	// Return the full object as JSON string
	fullLocation := struct {
		IP       string `json:"ip"`
		City     string `json:"city"`
		Region   string `json:"region"`
		Country  string `json:"country"`
		Loc      string `json:"loc"`
		Org      string `json:"org"`
		Timezone string `json:"timezone"`
		Readme   string `json:"readme"`
	}{
		IP:       data.IP,
		City:     data.City,
		Region:   data.Region,
		Country:  data.Country,
		Loc:      data.Loc,
		Org:      data.Org,
		Timezone: data.Timezone,
		Readme:   data.Readme,
	}

	locationJSON, err := json.Marshal(fullLocation)
	if err != nil {
		return "", err
	}

	return string(locationJSON), nil
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
		a.logger.Debug("Job check completed - no pending jobs")
		fmt.Printf("📋 [%s] No pending jobs - monitoring...\n", now.Format("15:04:05"))
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

// initializeGlobalAPIClient initializes the global API client
func initializeGlobalAPIClient() error {
	config, err := getOrLoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	globalAPIClient = NewAPIClient(config.BackendURL)
	// Use a minimal logger for API operations  
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	globalAPIClient.SetLogger(logger)

	// If authenticated, get and set wallet address
	if globalAPIClient.ValidateToken() {
		if userInfo, err := globalAPIClient.GetUserInfo(); err == nil {
			if walletAddress, ok := userInfo["wallet"].(string); ok {
				globalAPIClient.SetWalletAddress(walletAddress)
			}
		}
	}

	return nil
}

// getOrCreateAgent gets the global agent instance or creates it if it doesn't exist
func getOrCreateAgent(verbose bool) (*Agent, error) {
	if globalAgent != nil {
		return globalAgent, nil
	}
	
	agent, err := NewAgent()
	if err != nil {
		return nil, err
	}
	
	// Disable verbose logging unless requested
	if !verbose {
		agent.logger.SetOutput(io.Discard)
		agent.apiClient.SetLogger(logrus.New())
		agent.apiClient.logger.SetOutput(io.Discard)
	}
	
	globalAgent = agent
	return globalAgent, nil
}

// getOrLoadConfig gets the global config instance or loads it if it doesn't exist
func getOrLoadConfig() (*Config, error) {
	if globalConfig != nil {
		return globalConfig, nil
	}
	
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}
	
	globalConfig = config
	return globalConfig, nil
}

// loadMinimalConfig loads only essential config for lightweight commands
func loadMinimalConfig() (*Config, error) {
	config := &Config{}
	
	// Only load essential environment variables
	config.BackendURL = os.Getenv("BACKEND_URL")
	if config.BackendURL == "" {
		config.BackendURL = "https://app.skyopslabs.ai" // Default value
	}
	
	config.Logging.Level = os.Getenv("LOG_LEVEL")
	if config.Logging.Level == "" {
		config.Logging.Level = "error" // Minimal logging for fast commands
	}
	
	return config, nil
}

// Note: No cleanup function needed as it would reset agent stats in database

func main() {
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Fast path for simple commands - no initialization at all
	switch command {
	case "help", "--help", "-h":
		showHelp()
		return
	case "version", "--version", "-v":
		fmt.Println("SkyOps GPU Agent v1.0.0")
		return
	case "login":
		// Ultra-fast token check without any initialization
		homeDir, _ := os.UserHomeDir()
		tokenPath := filepath.Join(homeDir, ".skyops", "token")
		if data, err := os.ReadFile(tokenPath); err == nil {
			token := strings.TrimSpace(string(data))
			if token != "" {
				fmt.Println("✅ Already logged in!")
				return
			}
		}
		// Fall through to full login flow
	}

	// Only initialize API client for commands that need it
	needsAPI := map[string]bool{
		"login":    true,
		"register": true,
		"start":    true,
		"status":   true,
		"stats":    true,
		"stop":     true,
		"ping":     true,
	}

	if needsAPI[command] {
		if err := initializeGlobalAPIClient(); err != nil {
			// For commands that don't need API access, we can continue
			// Commands that need API will handle the error gracefully
		}
	}

	switch command {
	case "login":
		handleLoginCommandFullFlow()
	case "register":
		handleRegisterCommand(args)
	case "start":
		handleStartCommand(args)
	case "status":
		handleStatusCommand()
	case "stats":
		handleStatsCommand(args)
	case "stop":
		handleStopCommand(args)
	case "ping":
		handlePingCommand(args)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showHelp()
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println(`
		/$$                                              
		| $$                                              
/$$$$$$$| $$   /$$ /$$   /$$  /$$$$$$   /$$$$$$   /$$$$$$$
/$$_____/| $$  /$$/| $$  | $$ /$$__  $$ /$$__  $$ /$$_____/
|  $$$$$$ | $$$$$$/ | $$  | $$| $$  \ $$| $$  \ $$|  $$$$$$ 
\____  $$| $$_  $$ | $$  | $$| $$  | $$| $$  | $$ \____  $$
/$$$$$$$/| $$ \  $$|  $$$$$$$|  $$$$$$/| $$$$$$$/ /$$$$$$$/
|_______/ |__/  \__/ \____  $$ \______/ | $$____/ |_______/ 
					/$$  | $$          | $$                
					|  $$$$$$/          | $$                
					\______/           |__/                

SkyOps GPU Provider - Join the decentralized GPU network

Usage:
  skyops [command] [flags]

Available Commands:
  login       Authenticate with the SkyOps network
  register    Register this node with the SkyOps network
  start       Start the agent daemon
  status      Check agent status
  stats       Show detailed agent statistics and information
  stop        Stop the agent daemon
  ping        Test heartbeat connection to SkyOps network
  version     Show version information
  help        Show this help message

Examples:
  skyops login           # Authenticate with the SkyOps network
  skyops register        # Register this node (must be authenticated)
  skyops start           # Start the agent in foreground
  skyops start --daemon  # Start the agent as background daemon
  skyops status          # Check agent status
  skyops stop            # Stop the agent daemon/process
  skyops ping            # Test heartbeat connection

Flags:
  --daemon           Run as background daemon (for start command)
  --verbose, -v      Show detailed output (for ping command)
  --help             Show help information

Daemon Mode:
  Use 'skyops start --daemon' to run the agent in the background.
  The daemon will continue running even after you close the terminal.
  Use 'skyops stop' to stop the daemon process.
  Use 'skyops status' to check if the daemon is running.`)
}

func handleLoginCommandFullFlow() {
	// Ultra-fast check: just check if token file exists, no initialization at all
	homeDir, _ := os.UserHomeDir()
	tokenPath := filepath.Join(homeDir, ".skyops", "token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			fmt.Println("✅ Already logged in!")
			return
		}
	}

	// Only proceed with full authentication flow if no token exists
	fmt.Println("🔐 Starting authentication flow...")
	
	// Now do full initialization
	if globalAPIClient == nil {
		if err := initializeGlobalAPIClient(); err != nil {
			fmt.Printf("❌ Failed to initialize API client: %v\n", err)
			os.Exit(1)
		}
	}

	// Authentication logic
	if err := globalAPIClient.Authenticate(); err != nil {
		fmt.Println("❌ Authentication failed")
		os.Exit(1)
	}
	
	// Get and store wallet address after successful authentication
	if userInfo, err := globalAPIClient.GetUserInfo(); err == nil {
		if walletAddress, ok := userInfo["wallet"].(string); ok {
			globalAPIClient.SetWalletAddress(walletAddress)
		}
	}
	
	fmt.Println("✅ Authentication successful!")

	fmt.Println("🔐 Starting SkyOps authentication flow...")
	if err := globalAPIClient.Authenticate(); err != nil {
		fmt.Println("❌ Authentication failed")
		os.Exit(1)
	}
	
	// Get and store wallet address after successful authentication
	if userInfo, err := globalAPIClient.GetUserInfo(); err == nil {
		if walletAddress, ok := userInfo["wallet"].(string); ok {
			globalAPIClient.SetWalletAddress(walletAddress)
		}
	}
	
	fmt.Println("✅ Authentication successful!")
}

func handleRegisterCommand(args []string) {
	// Parse flags - keeping only relevant ones
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println("Register this node with the SkyOps network")
			fmt.Println("Usage: skyops register")
			return
		}
	}

	fmt.Println("🚀 SkyOps GPU Node Registration")
	fmt.Println(strings.Repeat("=", 50))

	// Ensure global API client is initialized
	if globalAPIClient == nil {
		if err := initializeGlobalAPIClient(); err != nil {
			fmt.Printf("❌ Failed to initialize API client: %v\n", err)
			os.Exit(1)
		}
	}

	// Check for wallet address before proceeding
	if globalAPIClient.wallet == "" {
		fmt.Println("❌ No wallet address found. Please run 'skyops login' to authenticate before registering.")
		os.Exit(1)
	}

	// Check if agent already exists
	if globalAPIClient.ValidateToken() {
		agentExists, err := globalAPIClient.CheckAgentExists()
		if err != nil {
			fmt.Printf("❌ Failed to check agent status: %v\n", err)
			os.Exit(1)
		}
		
		if agentExists {
			fmt.Println("✅ agent_id already on the network")
			return
		}
	} else {
		fmt.Println("❌ Not authenticated. Please run 'skyops login' to authenticate before registering.")
		os.Exit(1)
	}

	// For actual registration, we still need the full agent (for system monitoring)
	agent, err := getOrCreateAgent(false)
	if err != nil {
		fmt.Printf("❌ Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}

	// Proceed with registration
	fmt.Printf("📝 Registering node with SkyOps network...\n")
	if err := agent.registerNode(); err != nil {
		fmt.Printf("❌ Node registration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Node registration completed successfully!")
	fmt.Println("💡 You can now start the agent with: skyops start")
}

func handleStartCommand(args []string) {
	var daemon bool = false
	var noDaemon bool = false

	// Parse flags
	for _, arg := range args {
		if arg == "--daemon" {
			daemon = true
		} else if arg == "--no-daemon" {
			noDaemon = true
		} else if arg == "--help" || arg == "-h" {
			fmt.Println("Start the SkyOps GPU agent")
			fmt.Println("Usage: skyops start [--daemon|--no-daemon]")
			fmt.Println("  --daemon      Run as background daemon")
			fmt.Println("  --no-daemon   Run in foreground (used internally)")
			return
		}
	}

	// If daemon mode is requested and this is not the no-daemon subprocess
	if daemon && !noDaemon {
		fmt.Println("� Starting daemon...")
		if err := startDaemon(); err != nil {
			fmt.Printf("❌ Failed to start daemon: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// If running as daemon subprocess, redirect output
	if noDaemon {
		if err := redirectOutput(); err != nil {
			fmt.Printf("❌ Failed to redirect output: %v\n", err)
			// Continue anyway, this is not critical
		}
	}

	fmt.Println("�🚀 Starting SkyOps GPU Agent...")

	// Check if required environment variables are set
	if os.Getenv("BACKEND_URL") == "" {
		fmt.Println("❌ BACKEND_URL environment variable not set")
		fmt.Println("Please set required environment variables before starting the agent")
		os.Exit(1)
	}

	agent, err := getOrCreateAgent(false)
	if err != nil {
		fmt.Printf("❌ Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}

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

	fmt.Println("✅ Agent started successfully")
	fmt.Println("📊 Sending heartbeats to network...")
	fmt.Println("Press Ctrl+C to stop")

	// Start the agent
	if err := agent.Start(); err != nil {
		fmt.Printf("❌ Agent error: %v\n", err)
		os.Exit(1)
	}
}

// isAgentRunning checks if the skyops agent is currently running (started with 'skyops start')
func handleStatusCommand() {
	fmt.Println("📊 SkyOps Agent Status")
	fmt.Println(strings.Repeat("=", 30))

	config, err := getOrLoadConfig()
	if err != nil {
		fmt.Println("❌ Status: Not configured - missing environment variables")
		fmt.Println("💡 Set BACKEND_URL and other environment variables")
		fmt.Println("💡 Run 'skyops register' to set up your node")
		return
	}

	// Ensure global API client is initialized
	if globalAPIClient == nil {
		if err := initializeGlobalAPIClient(); err != nil {
			fmt.Println("❌ Status: Failed to initialize API client")
			return
		}
	}
	
	// Check if agent is currently running (daemon or regular process)
	daemonRunning := isDaemonRunning()
	agentRunning := isAgentRunning()
	
	if globalAPIClient.ValidateToken() {
		fmt.Printf("✅ Status: Configured and Authenticated (Agent ID: %s)\n", config.AgentID)
		fmt.Println("🔑 Authentication: Valid")
		
		if daemonRunning {
			fmt.Println("🚀 Agent: Running as Daemon")
			fmt.Println("📊 Network Status: Online - Sending heartbeats")
			fmt.Println("💡 Agent is actively providing GPU resources")
		} else if agentRunning {
			fmt.Println("🚀 Agent: Currently Running")
			fmt.Println("📊 Network Status: Online - Sending heartbeats")
			fmt.Println("💡 Agent is actively providing GPU resources")
		} else {
			fmt.Println("⏸️  Agent: Not Running")
			fmt.Println("📊 Network Status: Offline")
			fmt.Println("💡 Run 'skyops start' or 'skyops start --daemon' to begin providing GPU resources")
		}
	} else {
		savedToken := globalAPIClient.LoadSavedToken()
		if savedToken != "" {
			fmt.Println("⚠️  Status: Configured but Token Invalid")
			fmt.Println("🔑 Authentication: Token expired/invalid")
			if daemonRunning || agentRunning {
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

	// Parse flags
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Println(`🛑 SkyOps Stop Command

Usage:
  skyops stop [flags]

Description:
  Stop any running SkyOps agent processes

Flags:
  --help, -h          Show this help message

Examples:
  skyops stop         # Stop running agent
  
This command will:
  - Try to stop daemon process first
  - Find running skyops processes
  - Send graceful shutdown signals
  - Clean up any leftover processes`)
			return
		}
	}

	// First try to stop daemon if it's running
	if isDaemonRunning() {
		fmt.Println("🔧 Stopping daemon process...")
		if err := stopDaemon(); err != nil {
			fmt.Printf("⚠️  Failed to stop daemon: %v\n", err)
			fmt.Println("🔍 Falling back to process search...")
		} else {
			return // Daemon stopped successfully
		}
	}

	// Find and stop skyops processes
	stopped := stopSkyOpsProcesses()
	
	if stopped > 0 {
		fmt.Printf("✅ Stopped %d SkyOps process(es)\n", stopped)
	} else {
		fmt.Println("ℹ️  No running SkyOps processes found")
	}
}

// stopSkyOpsProcesses finds and stops running skyops processes
func handlePingCommand(args []string) {
	var verbose bool = false

	// Parse flags
	for _, arg := range args {
		if arg == "--verbose" || arg == "-v" {
			verbose = true
		} else if arg == "--help" || arg == "-h" {
			fmt.Println(`📡 SkyOps Ping Command

Usage:
  skyops ping [flags]

Description:
  Test the heartbeat connection to the SkyOps network

Flags:
  --verbose, -v       Show detailed output and logs
  --help, -h          Show this help message

Examples:
  skyops ping         # Test heartbeat with clean output
  skyops ping -v      # Test with verbose output

Environment Variables:
  BACKEND_URL         # Required: SkyOps backend URL
  API_KEY            # Optional: API key for authentication`)
			return
		}
	}

	// Check if required environment variables are set
	if os.Getenv("BACKEND_URL") == "" {
		fmt.Println("❌ BACKEND_URL environment variable not set")
		fmt.Println("Please set required environment variables before running ping")
		return
	}



	// Ensure global API client is initialized
	// if globalAPIClient == nil {
	// 	if err := initializeGlobalAPIClient(); err != nil {
	// 		fmt.Printf("❌ Failed to initialize API client: %v\n", err)
	// 		return
	// 	}
	// }

	if verbose {
		fmt.Println("🏓 Testing SkyOps Network Connection...")
		fmt.Println(strings.Repeat("=", 40))
		fmt.Println("🔐 Testing authentication...")
	} else {
		fmt.Print("📡 Testing connection... ")
	}

	// Test authentication
	if !globalAPIClient.ValidateToken() {
		if verbose {
			fmt.Println("❌ Authentication failed")
			fmt.Println("💡 Please run 'skyops login' to authenticate")
		} else {
			fmt.Println("❌ Authentication failed. Run 'skyops register' first.")
		}
		return
	}
	
	if verbose {
		fmt.Println("✅ Authentication successful")
		fmt.Println("📊 Collecting system information...")
	}

	// Now create full agent for system monitoring
	agent, err := getOrCreateAgent(verbose)
	if err != nil {
		fmt.Printf("❌ Failed to load agent config: %v\n", err)
		return
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

func handleStatsCommand(args []string) {
	var verbose bool = false

	// Parse flags
	for _, arg := range args {
		if arg == "--verbose" || arg == "-v" {
			verbose = true
		} else if arg == "--help" || arg == "-h" {
			fmt.Println(`📊 SkyOps Agent Statistics

Usage:
  skyops stats [flags]

Description:
  Display detailed information about the registered agent including system specs,
  network status, reputation, and earnings.

Flags:
  --verbose, -v       Show additional technical details
  --help, -h          Show this help message


Examples:
  skyops stats        # Show basic agent statistics
  skyops stats -v     # Show detailed statistics with technical info`)
			return
		}
	}

	fmt.Println("📊 SkyOps Node Statistics")
	fmt.Println(strings.Repeat("=", 50))




	// Ensure global API client is initialized
	if globalAPIClient == nil {
		if err := initializeGlobalAPIClient(); err != nil {
			fmt.Printf("❌ Failed to initialize API client: %v\n", err)
			return
		}
	}

	// Check authentication
	if !globalAPIClient.ValidateToken() {
		fmt.Println("❌ Not authenticated")
		fmt.Println("💡 Run 'skyops login' to authenticate first")
		return
	}

	// Get agent information from the server
	agentExists, err := globalAPIClient.CheckAgentExists()
	if err != nil {
		fmt.Printf("❌ Failed to check agent status: %v\n", err)
		return
	}

	if !agentExists {
		fmt.Println("❌ Agent not found on the network")
		fmt.Println("💡 Run 'skyops register' to register your agent")
		return
	}

	// Get detailed agent info using the wallet address
	walletAddress := globalAPIClient.GetWalletAddress()
	if walletAddress == "" {
		// Try to get wallet address from user info
		if userInfo, err := globalAPIClient.GetUserInfo(); err == nil {
			if wallet, ok := userInfo["wallet"].(string); ok {
				walletAddress = wallet
				globalAPIClient.SetWalletAddress(walletAddress)
			}
		}
	}

	if walletAddress == "" {
		fmt.Println("❌ Could not retrieve wallet address")
		return
	}

	// Get agent details from the backend
	resp, err := globalAPIClient.makeRequest("GET", fmt.Sprintf("/api/v1/agents/%s", walletAddress), nil)
	if err != nil {
		fmt.Printf("❌ Failed to get agent details: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Failed to get agent details (status %d): %s\n", resp.StatusCode, string(body))
		return
	}

	var agentInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agentInfo); err != nil {
		fmt.Printf("❌ Failed to parse agent details: %v\n", err)
		return
	}

	// Display agent information in a nicely formatted way
	displayAgentStats(agentInfo, verbose)
}

func displayAgentStats(agentInfo map[string]interface{}, verbose bool) {
	// Basic Information
	fmt.Println("\n🤖 Node Information")
	fmt.Println(strings.Repeat("-", 30))
	
	agentID, _ := agentInfo["agent_id"].(string)
	var status string
	if isAgentRunning() {
		status = "online"
	} else {
		status = "offline"
	}
	
	fmt.Printf("Agent ID:     %s\n", agentID)
	fmt.Printf("Status:       %s\n", getStatusEmoji(status)+status)
	
	// Location
	if location, ok := agentInfo["location"].(string); ok && location != "" {
		fmt.Printf("Location:     %s\n", location)
	}

	// Timestamps
	if createdAt, ok := agentInfo["created_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, createdAt); err == nil {
			fmt.Printf("Created:      %s\n", parsed.Format("2006-01-02 15:04:05 UTC"))
		}
	}
	
	if lastSeen, ok := agentInfo["last_seen"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, lastSeen); err == nil {
			duration := time.Since(parsed)
			fmt.Printf("Last Seen:    %s (%s ago)\n", parsed.Format("2006-01-02 15:04:05 UTC"), formatDuration(duration))
		}
	}

	// Reputation and Performance
	fmt.Println("\n📈 Performance Metrics")
	fmt.Println(strings.Repeat("-", 30))
	
	if reputation, ok := agentInfo["reputation_score"].(float64); ok {
		fmt.Printf("Reputation:   %.2f/5.0 %s\n", reputation, getReputationEmoji(reputation))
	}
	
	if jobsCompleted, ok := agentInfo["total_jobs_completed"].(float64); ok {
		fmt.Printf("Jobs Done:    %.0f\n", jobsCompleted)
	}

	// System Information
	fmt.Println("\n💻 System Specifications")
	fmt.Println(strings.Repeat("-", 30))
	
	// Check for direct agent-level hardware specs first (new format)
	if cpuCount, ok := agentInfo["cpu_count"].(float64); ok && cpuCount > 0 {
		fmt.Printf("CPU Cores:    %.0f\n", cpuCount)
	} else if systemInfo, ok := agentInfo["system_info"].(map[string]interface{}); ok {
		if cpuCount, ok := systemInfo["cpu_count"].(float64); ok {
			fmt.Printf("CPU Cores:    %.0f\n", cpuCount)
		}
	}
	
	if ramTotal, ok := agentInfo["ram_total"].(float64); ok && ramTotal > 0 {
		fmt.Printf("Total RAM:    %.1f GB\n", ramTotal)
		if ramFree, ok := agentInfo["ram_free"].(float64); ok {
			ramUsed := ramTotal - ramFree
			usagePercent := (ramUsed / ramTotal) * 100
			fmt.Printf("RAM Used:     %.1f GB (%.1f%%)\n", ramUsed, usagePercent)
			fmt.Printf("RAM Free:     %.1f GB\n", ramFree)
		}
	} else if systemInfo, ok := agentInfo["system_info"].(map[string]interface{}); ok {
		if memTotal, ok := systemInfo["memory_total"].(float64); ok {
			fmt.Printf("Total RAM:    %.1f GB\n", memTotal)
		}
	}
	
	if diskTotal, ok := agentInfo["disk_total"].(float64); ok && diskTotal > 0 {
		fmt.Printf("Total Disk:   %.0f GB\n", diskTotal)
		if diskFree, ok := agentInfo["disk_free"].(float64); ok {
			diskUsed := diskTotal - diskFree
			usagePercent := (diskUsed / diskTotal) * 100
			fmt.Printf("Disk Used:    %.0f GB (%.1f%%)\n", diskUsed, usagePercent)
			fmt.Printf("Disk Free:    %.0f GB\n", diskFree)
		}
	} else if systemInfo, ok := agentInfo["system_info"].(map[string]interface{}); ok {
		if diskTotal, ok := systemInfo["disk_total"].(float64); ok {
			fmt.Printf("Total Disk:   %.0f GB\n", diskTotal)
		}
	}

	// GPU Information - check agent level first, then system_info
	if gpuName, ok := agentInfo["gpu_name"].(string); ok && gpuName != "" {
		fmt.Println("\n🎮 GPU Information")
		fmt.Println(strings.Repeat("-", 30))
		fmt.Printf("GPU Model:    %s\n", gpuName)
		
		if gpuRamTotal, ok := agentInfo["gpu_ram_total"].(float64); ok && gpuRamTotal > 0 {
			fmt.Printf("GPU Memory:   %.1f GB\n", gpuRamTotal)
			if gpuRamFree, ok := agentInfo["gpu_ram_free"].(float64); ok {
				gpuRamUsed := gpuRamTotal - gpuRamFree
				usagePercent := (gpuRamUsed / gpuRamTotal) * 100
				fmt.Printf("GPU Used:     %.1f GB (%.1f%%)\n", gpuRamUsed, usagePercent)
				fmt.Printf("GPU Free:     %.1f GB\n", gpuRamFree)
			}
		}
	}
	


	// Pricing Information
	if pricing, ok := agentInfo["pricing"].(map[string]interface{}); ok && len(pricing) > 0 {
		fmt.Println("\n💰 Pricing")
		fmt.Println(strings.Repeat("-", 30))
		
		for resource, price := range pricing {
			if priceVal, ok := price.(float64); ok {
				resourceName := strings.ToUpper(string(resource[0])) + resource[1:]
				fmt.Printf("%-12s: $%.4f/hour\n", resourceName, priceVal)
			}
		}
	}

	// Tags
	if tags, ok := agentInfo["tags"].([]interface{}); ok && len(tags) > 0 {
		fmt.Println("\n🏷️  Tags")
		fmt.Println(strings.Repeat("-", 30))
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				fmt.Printf("• %s\n", tagStr)
			}
		}
	}

	// Verbose information
	if verbose {
		fmt.Println("\n🔧 Technical Details")
		fmt.Println(strings.Repeat("-", 30))
		
		// Show agent-level hardware details first
		if ramTotal, ok := agentInfo["ram_total"].(float64); ok && ramTotal > 0 {
			// Calculate RAM usage if we have the total from agent level
			fmt.Printf("RAM Total:    %.1f GB\n", ramTotal)
			
			// Try to get current usage from system_info if available
			if systemInfo, ok := agentInfo["system_info"].(map[string]interface{}); ok {
				if memUsed, ok := systemInfo["memory_used"].(float64); ok {
					fmt.Printf("RAM Used:     %.1f GB\n", memUsed)
				}
				if memAvail, ok := systemInfo["memory_available"].(float64); ok {
					fmt.Printf("RAM Free:     %.1f GB\n", memAvail)
				}
			}
		}
		
		// Show disk details
		if diskTotal, ok := agentInfo["disk_total"].(float64); ok && diskTotal > 0 {
			if diskFree, ok := agentInfo["disk_free"].(float64); ok {
				diskUsed := diskTotal - diskFree
				fmt.Printf("Disk Used:    %.0f GB\n", diskUsed)
				fmt.Printf("Disk Free:    %.0f GB\n", diskFree)
			}
		}
		
		// Show GPU memory details
		if gpuRamTotal, ok := agentInfo["gpu_ram_total"].(float64); ok && gpuRamTotal > 0 {
			if gpuRamFree, ok := agentInfo["gpu_ram_free"].(float64); ok {
				gpuRamUsed := gpuRamTotal - gpuRamFree
				fmt.Printf("GPU RAM Used: %.1f GB\n", gpuRamUsed)
				fmt.Printf("GPU RAM Free: %.1f GB\n", gpuRamFree)
			}
		}
		
		// Fallback to system_info for additional details
		if systemInfo, ok := agentInfo["system_info"].(map[string]interface{}); ok {
			if cpuUsage, ok := systemInfo["cpu_usage"].(float64); ok {
				fmt.Printf("CPU Usage:    %.1f%%\n", cpuUsage)
			}
			
			// Only show these if we don't have agent-level equivalents
			if _, hasAgentRAM := agentInfo["ram_total"]; !hasAgentRAM {
				if memUsed, ok := systemInfo["memory_used"].(float64); ok {
					fmt.Printf("RAM Used:     %.1f GB\n", memUsed)
				}
				
				if memAvail, ok := systemInfo["memory_available"].(float64); ok {
					fmt.Printf("RAM Free:     %.1f GB\n", memAvail)
				}
			}
			
			if _, hasAgentDisk := agentInfo["disk_total"]; !hasAgentDisk {
				if diskUsed, ok := systemInfo["disk_used"].(float64); ok {
					fmt.Printf("Disk Used:    %.0f GB\n", diskUsed)
				}
				
				if diskFree, ok := systemInfo["disk_free"].(float64); ok {
					fmt.Printf("Disk Free:    %.0f GB\n", diskFree)
				}
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
}

func getStatusEmoji(status string) string {
	switch strings.ToLower(status) {
	case "online":
		return "🟢 "
	case "offline":
		return "🔴 "
	case "busy":
		return "🟡 "
	default:
		return "⚪ "
	}
}

func getReputationEmoji(score float64) string {
	if score >= 4.5 {
		return "⭐⭐⭐⭐⭐"
	} else if score >= 3.5 {
		return "⭐⭐⭐⭐"
	} else if score >= 2.5 {
		return "⭐⭐⭐"
	} else if score >= 1.5 {
		return "⭐⭐"
	} else if score >= 0.5 {
		return "⭐"
	}
	return "☆"
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	} else {
		return fmt.Sprintf("%.1fd", d.Hours()/24)
	}
}
