package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// APIClient handles communication with the SkyOps backend
type APIClient struct {
	baseURL           string
	wallet            string
	logger            *logrus.Logger
	httpClient        *http.Client
	authToken         string
	expressServerURL  string
	clientURL         string
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		logger:           logrus.New(),
		expressServerURL: "https://app.skyopslabs.ai",
		clientURL:        "https://app.skyopslabs.ai",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}


// SetLogger sets the logger for the API client
func (c *APIClient) SetLogger(logger *logrus.Logger) {
	c.logger = logger
}

// getTokenFilePath returns the path for storing authentication token
func (c *APIClient) getTokenFilePath() string {
	homeDir, _ := os.UserHomeDir()
	skyopsDir := filepath.Join(homeDir, ".skyops")
	os.MkdirAll(skyopsDir, 0700)
	return filepath.Join(skyopsDir, "token")
}

// SaveToken saves authentication token to file
func (c *APIClient) SaveToken(token string) error {
	tokenPath := c.getTokenFilePath()
	err := os.WriteFile(tokenPath, []byte(token), 0600)
	if err != nil {
		c.logger.WithError(err).Warning("Could not save authentication token")
		return err
	}
	c.logger.Debug("Authentication token saved")
	return nil
}

// LoadSavedToken loads authentication token from file
func (c *APIClient) LoadSavedToken() string {
	tokenPath := c.getTokenFilePath()
	if data, err := os.ReadFile(tokenPath); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			c.authToken = token
			c.logger.Debug("Authentication token loaded from file")
			return token
		}
	}
	return ""
}

// RemoveToken removes saved authentication token
func (c *APIClient) RemoveToken() {
	tokenPath := c.getTokenFilePath()
	os.Remove(tokenPath)
	c.authToken = ""
	c.logger.Debug("Authentication token removed")
}

// ValidateToken validates the current token with Express server
func (c *APIClient) ValidateToken() bool {
	if c.authToken == "" {
		c.authToken = c.LoadSavedToken()
	}
	
	if c.authToken == "" {
		return false
	}

	req, err := http.NewRequest("GET", c.expressServerURL+"/api/users", nil)
	if err != nil {
		return false
	}
	
	req.Header.Set("x-auth-token", c.authToken)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Debug("Token validation error")
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == 200
}

// findAvailablePort finds an available port for the callback server
func (c *APIClient) findAvailablePort() int {
	for port := 8080; port < 8090; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 8080 // fallback
}

// openBrowser opens a URL in the default browser
func (c *APIClient) openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// Authenticate authenticates with the backend using browser-based flow
func (c *APIClient) Authenticate() error {
	// Check if we have a saved token first
	savedToken := c.LoadSavedToken()
	if savedToken != "" {
		c.logger.Info("Found saved authentication token, validating...")
		if c.ValidateToken() {
			c.logger.Info("✅ Saved token is valid!")
			
			return nil
		} else {
			c.logger.Info("Saved token is invalid, need to re-authenticate")
			c.RemoveToken()
		}
	}

	c.logger.Info("Starting automatic browser authentication...")

	// Find available port for callback
	callbackPort := c.findAvailablePort()
	callbackURL := fmt.Sprintf("http://localhost:%d/callback", callbackPort)

	// Channel to receive the token
	tokenChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// Start callback server
	server := &http.Server{Addr: fmt.Sprintf(":%d", callbackPort)}
	
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		errorParam := r.URL.Query().Get("error")
		
		if token != "" {
			tokenChan <- token
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(200)
			w.Write([]byte(`
				<html>
				<body>
					<h1>✅ Authentication Successful!</h1>
					<p>You can now close this window and return to your terminal.</p>
					<script>window.close();</script>
				</body>
				</html>
			`))
		} else if errorParam != "" {
			errorChan <- fmt.Errorf("authentication error: %s", errorParam)
			w.WriteHeader(400)
			w.Write([]byte(`
				<html>
				<body>
					<h1>❌ Authentication Failed</h1>
					<p>Error: ` + errorParam + `</p>
				</body>
				</html>
			`))
		}
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errorChan <- err
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Build authentication URL
	authURL := fmt.Sprintf("%s/auth/cli?callback=%s", c.clientURL, callbackURL)
	
	fmt.Printf("🌐 Opening browser for authentication...\n")
	fmt.Printf("📍 If browser doesn't open, visit: %s\n", authURL)
	
	if err := c.openBrowser(authURL); err != nil {
		c.logger.WithError(err).Warning("Could not open browser automatically")
	}

	fmt.Println("⏳ Waiting for authentication...")

	// Wait for token or error with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case token := <-tokenChan:
		server.Shutdown(context.Background())
		c.authToken = token
		c.SaveToken(token)
		fmt.Println("✅ Authentication successful!")
		return nil
	case err := <-errorChan:
		server.Shutdown(context.Background())
		return fmt.Errorf("authentication failed: %v", err)
	case <-ctx.Done():
		server.Shutdown(context.Background())
		return fmt.Errorf("authentication timeout after 5 minutes")
	}
}

// makeRequest makes an authenticated HTTP request to the API
func (c *APIClient) makeRequest(method, endpoint string, payload interface{}) (*http.Response, error) {
	url := c.baseURL + endpoint

	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return nil, fmt.Errorf("failed to encode payload: %v", err)
		}
	}

	req, err := http.NewRequest(method, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	
	req.Header.Set("Accept", "application/json")
	
	if c.authToken != "" {
		req.Header.Set("x-auth-token", c.authToken)
	}

	c.logger.WithFields(logrus.Fields{
		"method":   method,
		"endpoint": endpoint,
		"url":      url,
	}).Debug("Making API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}

	return resp, nil
}

// RegisterNode registers this node with the SkyOps network
func (c *APIClient) RegisterNode(nodeInfo map[string]interface{}) (string, error) {
	resp, err := c.makeRequest("POST", "/api/v1/agents/register", nodeInfo)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("node registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get the actual agent ID and message
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse registration response: %v", err)
	}

	actualAgentID, ok := response["agent_id"].(string)
	if !ok {
		return "", fmt.Errorf("agent_id not found in registration response")
	}

	if message, ok := response["message"].(string); ok {
		c.logger.Info(message)
	}

	c.logger.WithField("agent_id", actualAgentID).Info("Successfully registered node with SkyOps network")
	return actualAgentID, nil
}

func (c *APIClient) CheckAgentExists() (bool, error) {
	// First ensure we have the wallet address
	if c.wallet == "" {
		userInfo, err := c.GetUserInfo()
		if err != nil {
			return false, fmt.Errorf("failed to get user info: %v", err)
		}
		
		walletAddress, ok := userInfo["wallet"].(string)
		if !ok {
			return false, fmt.Errorf("wallet not found in user info")
		}
		
		c.wallet = walletAddress
	}
	
	// Use the wallet address as the agent_id to check existence
	// This works because the backend /agents/{agent_id} endpoint can find by wallet_address
	resp, err := c.makeRequest("GET", fmt.Sprintf("/api/v1/agents/%s", c.wallet), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	
	// If we get a 200 OK, the agent exists
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	
	// If we get a 404, the agent doesn't exist
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	
	// For other status codes, consider it an error
	body, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("failed to check agent existence with status %d: %s", resp.StatusCode, string(body))
}

// SendHeartbeat sends heartbeat with current system status
func (c *APIClient) SendHeartbeat(stats map[string]interface{}) (map[string]interface{}, error) {
	// Debug: Log the payload being sent
	if payloadBytes, err := json.MarshalIndent(stats, "", "  "); err == nil {
		c.logger.WithField("payload", string(payloadBytes)).Debug("Sending heartbeat payload")
	}
	
	resp, err := c.makeRequest("POST", "/api/v1/agents/heartbeat", stats)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read response body for better error information
		body, _ := io.ReadAll(resp.Body)
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"response_body": string(body),
		}).Error("Heartbeat failed")
		return nil, fmt.Errorf("heartbeat failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		c.logger.WithError(err).Warning("Failed to decode heartbeat response")
		return map[string]interface{}{}, nil
	}

	c.logger.Debug("Heartbeat sent successfully")
	return response, nil
}

// GetPendingJobs retrieves pending jobs from the backend
func (c *APIClient) GetPendingJobs() ([]map[string]interface{}, error) {
	resp, err := c.makeRequest("GET", "/api/v1/jobs/list", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get pending jobs with status %d", resp.StatusCode)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode jobs response: %v", err)
	}

	// Extract the jobs array from the response object
	jobs := make([]map[string]interface{}, 0)
	if jobsData, ok := response["jobs"].([]interface{}); ok {
		for _, job := range jobsData {
			if jobMap, ok := job.(map[string]interface{}); ok {
				jobs = append(jobs, jobMap)
			}
		}
	}

	return jobs, nil
}

// AcceptJob accepts a job offer
func (c *APIClient) AcceptJob(jobID string) error {
	endpoint := fmt.Sprintf("/api/v1/jobs/%s/accept", jobID)
	resp, err := c.makeRequest("POST", endpoint, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to accept job with status %d", resp.StatusCode)
	}

	c.logger.WithField("job_id", jobID).Info("Job accepted successfully")
	return nil
}

// RejectJob rejects a job offer
func (c *APIClient) RejectJob(jobID, reason string) error {
	payload := map[string]interface{}{"reason": reason}
	endpoint := fmt.Sprintf("/api/v1/jobs/%s/reject", jobID)
	
	resp, err := c.makeRequest("POST", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to reject job with status %d", resp.StatusCode)
	}

	c.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"reason": reason,
	}).Info("Job rejected successfully")
	return nil
}

// UpdateJobStatus updates the status of a job
func (c *APIClient) UpdateJobStatus(jobID, status string, result map[string]interface{}) error {
	payload := map[string]interface{}{
		"status": status,
	}
	if result != nil {
		payload["result"] = result
	}

	endpoint := fmt.Sprintf("/api/v1/jobs/%s/status", jobID)
	resp, err := c.makeRequest("PUT", endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update job status with status %d", resp.StatusCode)
	}

	c.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"status": status,
	}).Info("Job status updated successfully")
	return nil
}

// GetUserInfo gets the current user information including wallet address
func (c *APIClient) GetUserInfo() (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", c.expressServerURL+"/api/users", nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("x-auth-token", c.authToken)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info with status %d: %s", resp.StatusCode, string(body))
	}
	
	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %v", err)
	}
	// fmt.Printf("users: %v\n", userInfo)
	return userInfo, nil
}

// SetWalletAddress sets the wallet address for this client
func (c *APIClient) SetWalletAddress(walletAddress string) {
	c.wallet = walletAddress
}

// GetWalletAddress returns the current wallet address
func (c *APIClient) GetWalletAddress() string {
	return c.wallet
}

// GetAuthToken returns the current authentication token
func (c *APIClient) GetAuthToken() string {
	return c.authToken
}

// SetAuthToken sets the authentication token
func (c *APIClient) SetAuthToken(token string) {
	c.authToken = token
}
