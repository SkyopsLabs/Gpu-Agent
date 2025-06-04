package main

// Unified types for SkyOps GPU Agent
// These should match the backend Python schemas and frontend TypeScript types

import "time"

// AgentStatusType represents the status of an agent
type AgentStatusType string

const (
	StatusOffline     AgentStatusType = "OFFLINE"
	StatusOnline      AgentStatusType = "ONLINE"
	StatusBusy        AgentStatusType = "BUSY"
	StatusMaintenance AgentStatusType = "MAINTENANCE"
	StatusError       AgentStatusType = "ERROR"
)

// AgentAvailability represents the availability of an agent
type AgentAvailability string

const (
	AvailabilityAvailable   AgentAvailability = "available"
	AvailabilityBusy        AgentAvailability = "busy"
	AvailabilityOffline     AgentAvailability = "offline"
	AvailabilityMaintenance AgentAvailability = "maintenance"
)

// Location represents geographic location information
type Location struct {
	Country   string   `json:"country"`
	Region    string   `json:"region"`
	City      string   `json:"city"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// SystemInfo represents system information from monitoring
type SystemInfo struct {
	Hostname        string `json:"hostname"`
	Platform        string `json:"platform"`
	PlatformFamily  string `json:"platform_family"`
	PlatformVersion string `json:"platform_version"`
	KernelVersion   string `json:"kernel_version"`
	Architecture    string `json:"architecture"`
	CPUCount        int    `json:"cpu_count"`
	BootTime        string `json:"boot_time"`
	Uptime          uint64 `json:"uptime"`
}

// CPUStats represents CPU statistics
type CPUStats struct {
	UsagePercent float64 `json:"usage_percent"`
	CoreCount    int     `json:"core_count"`
	ModelName    string  `json:"model_name,omitempty"`
	Family       string  `json:"family,omitempty"`
	Mhz          float64 `json:"mhz,omitempty"`
	CacheSize    int32   `json:"cache_size,omitempty"`
}

// MemoryStats represents memory statistics
type MemoryStats struct {
	Total             uint64  `json:"total"`
	Available         uint64  `json:"available"`
	Used              uint64  `json:"used"`
	Free              uint64  `json:"free"`
	UsagePercent      float64 `json:"usage_percent"`
	SwapTotal         uint64  `json:"swap_total"`
	SwapUsed          uint64  `json:"swap_used"`
	SwapFree          uint64  `json:"swap_free"`
	SwapUsagePercent  float64 `json:"swap_usage_percent"`
}

// DiskStats represents disk statistics
type DiskStats struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usage_percent"`
	Path         string  `json:"path"`
	Fstype       string  `json:"fstype"`
}

// GPUInfo represents detailed GPU information
type GPUInfo struct {
	Index               int     `json:"index"`
	Name                string  `json:"name"`
	MemoryTotal         uint64  `json:"memory_total"`
	MemoryUsed          uint64  `json:"memory_used"`
	MemoryFree          uint64  `json:"memory_free"`
	MemoryUsagePercent  float64 `json:"memory_usage_percent"`
	GPUUtilization      uint32  `json:"gpu_utilization"`
	MemoryUtilization   uint32  `json:"memory_utilization"`
	Temperature         uint32  `json:"temperature"`
	PowerUsage          float64 `json:"power_usage"`
	FanSpeed            uint32  `json:"fan_speed"`
}

// GPUSummary represents a simplified GPU summary
type GPUSummary struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	MemoryTotal uint64 `json:"memory_total"`
}

// GPUCapabilities represents GPU capabilities summary
type GPUCapabilities struct {
	GPUCount           int           `json:"gpu_count"`
	PrimaryGPUName     string        `json:"primary_gpu_name"`
	TotalMemory        uint64        `json:"total_memory"`
	FreeMemory         uint64        `json:"free_memory"`
	GPUUtilization     *uint32       `json:"gpu_utilization,omitempty"`
	MemoryUtilization  *uint32       `json:"memory_utilization,omitempty"`
	Temperature        *uint32       `json:"temperature,omitempty"`
	PowerUsage         *float64      `json:"power_usage,omitempty"`
	AllGPUs            []GPUSummary  `json:"all_gpus"`
}

// AgentPricing represents pricing information
type AgentPricing struct {
	PricePerHour            float64 `json:"price_per_hour"`
	Currency                string  `json:"currency"`
	MinimumDurationMinutes  *int    `json:"minimum_duration_minutes,omitempty"`
	MaximumDurationMinutes  *int    `json:"maximum_duration_minutes,omitempty"`
}

// AgentMetrics represents agent reputation and performance metrics
type AgentMetrics struct {
	ReputationScore     float64 `json:"reputation_score"`
	TotalJobsCompleted  int     `json:"total_jobs_completed"`
	SuccessRate         float64 `json:"success_rate"`
	AverageResponseTime float64 `json:"average_response_time"`
	UptimePercentage    float64 `json:"uptime_percentage"`
}

// AgentRegisterRequest represents the agent registration request
type AgentRegisterRequest struct {
	AgentID         *string          `json:"agent_id,omitempty"`
	Hostname        string           `json:"hostname"`
	Location        string           `json:"location"`
	SystemInfo      SystemInfo       `json:"system_info"`
	// GPUCapabilities GPUCapabilities  `json:"gpu_capabilities"`
	AutoAcceptJobs  *bool            `json:"auto_accept_jobs,omitempty"`
}

// AgentRegisterResponse represents the agent registration response
type AgentRegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	AgentID string `json:"agent_id"`
	Token   string `json:"token"`
}

// AgentHeartbeatRequest represents the heartbeat request (simplified flat structure)
type AgentHeartbeatRequest struct {
	Timestamp    int64   `json:"timestamp"`
	CPUCount     int     `json:"cpu_count"`
	RAMTotal     float64 `json:"ram_total"`
	RAMFree      float64 `json:"ram_free"`
	DiskTotal    int     `json:"disk_total"`
	DiskFree     int     `json:"disk_free"`
	GPUName      string  `json:"gpu_name"`
	GPURamTotal  float64 `json:"gpu_ram_total"`
	GPURamFree   float64 `json:"gpu_ram_free"`
}

// AgentHeartbeatResponse represents the heartbeat response
type AgentHeartbeatResponse struct {
	Success              bool                      `json:"success"`
	Message              string                    `json:"message"`
	NextHeartbeatSeconds int                       `json:"next_heartbeat_seconds"`
	Commands             []AgentCommand            `json:"commands,omitempty"`
}

// AgentCommand represents a command from the backend
type AgentCommand struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// AgentMonitoringData represents complete monitoring data
type AgentMonitoringData struct {
	AgentID     string        `json:"agent_id"`
	Timestamp   int64         `json:"timestamp"`
	SystemInfo  SystemInfo    `json:"system_info"`
	CPUStats    CPUStats      `json:"cpu_stats"`
	MemoryStats MemoryStats   `json:"memory_stats"`
	DiskStats   DiskStats     `json:"disk_stats"`
	GPUStats    []GPUInfo     `json:"gpu_stats"`
}

// AgentInfo represents complete agent information
type AgentInfo struct {
	AgentID         string           `json:"agent_id"`
	Name            string           `json:"name"`
	Hostname        string           `json:"hostname"`
	Status          AgentStatusType  `json:"status"`
	IsOnline        bool             `json:"is_online"`
	LastSeen        time.Time        `json:"last_seen"`
	CreatedAt       time.Time        `json:"created_at"`
	Location        Location         `json:"location"`
	SystemInfo      SystemInfo       `json:"system_info"`
	GPUCapabilities GPUCapabilities  `json:"gpu_capabilities"`
	Pricing         AgentPricing     `json:"pricing"`
	Metrics         AgentMetrics     `json:"metrics"`
	Tags            []string         `json:"tags"`
}

// PublicAgent represents a simplified agent view for public APIs
type PublicAgent struct {
	ID           string            `json:"id"`
	Provider     string            `json:"provider"`
	Location     string            `json:"location"`
	GPUModel     string            `json:"gpu_model"`
	VramGB       int               `json:"vram_gb"`
	Cores        int               `json:"cores"`
	PricePerHour float64           `json:"price_per_hour"`
	Availability AgentAvailability `json:"availability"`
	Status       AgentStatusType   `json:"status"`
	LastSeen     string            `json:"last_seen"`
	Reputation   float64           `json:"reputation"`
	JobsCompleted int              `json:"jobs_completed"`
	Specs        PublicAgentSpecs  `json:"specs"`
}

// PublicAgentSpecs represents simplified specs for public view
type PublicAgentSpecs struct {
	MemoryGB  int    `json:"memory_gb"`
	StorageGB int    `json:"storage_gb"`
	CPUModel  string `json:"cpu_model"`
	Hostname  string `json:"hostname"`
}

// NetworkStats represents network-wide statistics
type NetworkStats struct {
	TotalNodes           int     `json:"total_nodes"`
	OnlineNodes          int     `json:"online_nodes"`
	TotalGPUs            int     `json:"total_gpus"`
	TotalMemoryGB        float64 `json:"total_memory_gb"`
	AveragePricePerHour  float64 `json:"average_price_per_hour"`
	NetworkUtilization   float64 `json:"network_utilization"`
	LastUpdated          string  `json:"last_updated"`
}

// AgentListResponse represents a paginated list of agents
type AgentListResponse struct {
	Agents     []AgentInfo `json:"agents"`
	TotalCount int         `json:"total_count"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
}

// AgentAuthResponse represents authentication response
type AgentAuthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	AgentID     string `json:"agent_id"`
}
