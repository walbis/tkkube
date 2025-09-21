package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
)

// BackupClient provides interface for integrating with the Go backup tool
type BackupClient struct {
	config        *sharedconfig.SharedConfig
	httpClient    *http.Client
	baseURL       string
	monitoring    monitoring.MetricsCollector
}

// BackupRequest represents a backup operation request
type BackupRequest struct {
	ClusterName   string            `json:"cluster_name"`
	Namespaces    []string          `json:"namespaces,omitempty"`
	Resources     []string          `json:"resources,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	ScheduleTime  *time.Time        `json:"schedule_time,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

// BackupResponse represents backup operation response
type BackupResponse struct {
	BackupID      string                 `json:"backup_id"`
	Status        string                 `json:"status"`
	Message       string                 `json:"message"`
	StartTime     time.Time              `json:"start_time"`
	EstimatedTime *time.Duration         `json:"estimated_time,omitempty"`
	Progress      *BackupProgress        `json:"progress,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// BackupProgress represents backup operation progress
type BackupProgress struct {
	TotalResources     int     `json:"total_resources"`
	ProcessedResources int     `json:"processed_resources"`
	PercentComplete    float64 `json:"percent_complete"`
	CurrentNamespace   string  `json:"current_namespace"`
	CurrentResource    string  `json:"current_resource"`
	BytesProcessed     int64   `json:"bytes_processed"`
	EstimatedSize      int64   `json:"estimated_size"`
}

// BackupStatus represents detailed backup status
type BackupStatus struct {
	BackupID        string                 `json:"backup_id"`
	Status          string                 `json:"status"` // pending, running, completed, failed
	StartTime       time.Time              `json:"start_time"`
	EndTime         *time.Time             `json:"end_time,omitempty"`
	Duration        *time.Duration         `json:"duration,omitempty"`
	ResourceCount   int                    `json:"resource_count"`
	Size            int64                  `json:"size_bytes"`
	MinIOPath       string                 `json:"minio_path"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Progress        *BackupProgress        `json:"progress,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// NewBackupClient creates a new backup client
func NewBackupClient(config *sharedconfig.SharedConfig, monitoring monitoring.MetricsCollector) *BackupClient {
	baseURL := "http://localhost:8080"
	if config.Integration.Enabled && config.Integration.Communication.Endpoints.BackupTool != "" {
		baseURL = config.Integration.Communication.Endpoints.BackupTool
	}

	timeout := 30 * time.Second // default fallback
	if config != nil {
		timeout = config.Timeouts.BackupClientTimeout
	}

	client := &http.Client{
		Timeout: timeout,
	}

	return &BackupClient{
		config:     config,
		httpClient: client,
		baseURL:    baseURL,
		monitoring: monitoring,
	}
}

// RegisterWithBridge registers this backup client with the integration bridge
func (bc *BackupClient) RegisterWithBridge(ctx context.Context, bridgeURL string, version string) error {
	registrationData := map[string]interface{}{
		"endpoint": bc.baseURL,
		"version":  version,
	}

	data, err := json.Marshal(registrationData)
	if err != nil {
		return fmt.Errorf("failed to marshal registration data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", bridgeURL+"/register/backup", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create registration request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register with bridge: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status: %d", resp.StatusCode)
	}

	bc.monitoring.IncCounter("backup_client_registrations", map[string]string{"status": "success"}, 1)
	return nil
}

// StartBackup initiates a new backup operation
func (bc *BackupClient) StartBackup(ctx context.Context, request *BackupRequest) (*BackupResponse, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal backup request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", bc.baseURL+"/api/backup/start", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create backup request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := bc.httpClient.Do(req)
	duration := time.Since(start)

	// Record metrics
	bc.monitoring.RecordDuration("backup_client_request_duration", 
		map[string]string{"operation": "start_backup"}, duration)

	if err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "start_backup", "status": "error"}, 1)
		return nil, fmt.Errorf("failed to start backup: %v", err)
	}
	defer resp.Body.Close()

	var response BackupResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "start_backup", "status": "parse_error"}, 1)
		return nil, fmt.Errorf("failed to parse backup response: %v", err)
	}

	bc.monitoring.IncCounter("backup_client_requests", 
		map[string]string{"operation": "start_backup", "status": "success"}, 1)

	return &response, nil
}

// GetBackupStatus retrieves the status of a backup operation
func (bc *BackupClient) GetBackupStatus(ctx context.Context, backupID string) (*BackupStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", bc.baseURL+"/api/backup/status/"+backupID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %v", err)
	}

	start := time.Now()
	resp, err := bc.httpClient.Do(req)
	duration := time.Since(start)

	bc.monitoring.RecordDuration("backup_client_request_duration", 
		map[string]string{"operation": "get_status"}, duration)

	if err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "get_status", "status": "error"}, 1)
		return nil, fmt.Errorf("failed to get backup status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "get_status", "status": "not_found"}, 1)
		return nil, fmt.Errorf("backup not found: %s", backupID)
	}

	var status BackupStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "get_status", "status": "parse_error"}, 1)
		return nil, fmt.Errorf("failed to parse status response: %v", err)
	}

	bc.monitoring.IncCounter("backup_client_requests", 
		map[string]string{"operation": "get_status", "status": "success"}, 1)

	return &status, nil
}

// ListBackups retrieves a list of backup operations
func (bc *BackupClient) ListBackups(ctx context.Context, limit int, offset int) ([]BackupStatus, error) {
	url := fmt.Sprintf("%s/api/backup/list?limit=%d&offset=%d", bc.baseURL, limit, offset)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %v", err)
	}

	start := time.Now()
	resp, err := bc.httpClient.Do(req)
	duration := time.Since(start)

	bc.monitoring.RecordDuration("backup_client_request_duration", 
		map[string]string{"operation": "list_backups"}, duration)

	if err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "list_backups", "status": "error"}, 1)
		return nil, fmt.Errorf("failed to list backups: %v", err)
	}
	defer resp.Body.Close()

	var backups []BackupStatus
	if err := json.NewDecoder(resp.Body).Decode(&backups); err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "list_backups", "status": "parse_error"}, 1)
		return nil, fmt.Errorf("failed to parse list response: %v", err)
	}

	bc.monitoring.IncCounter("backup_client_requests", 
		map[string]string{"operation": "list_backups", "status": "success"}, 1)

	return backups, nil
}

// WaitForCompletion waits for a backup to complete and returns the final status
func (bc *BackupClient) WaitForCompletion(ctx context.Context, backupID string) (*BackupStatus, error) {
	interval := 5 * time.Second // default fallback
	if bc.config != nil {
		interval = bc.config.Timeouts.BackupPollingInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := bc.GetBackupStatus(ctx, backupID)
			if err != nil {
				return nil, fmt.Errorf("failed to get backup status: %v", err)
			}

			switch status.Status {
			case "completed":
				bc.monitoring.IncCounter("backup_client_completions", 
					map[string]string{"status": "success"}, 1)
				return status, nil
			case "failed":
				bc.monitoring.IncCounter("backup_client_completions", 
					map[string]string{"status": "failure"}, 1)
				return status, fmt.Errorf("backup failed: %s", status.ErrorMessage)
			case "running", "pending":
				// Continue waiting
				continue
			default:
				return status, fmt.Errorf("unknown backup status: %s", status.Status)
			}
		}
	}
}

// CancelBackup cancels a running backup operation
func (bc *BackupClient) CancelBackup(ctx context.Context, backupID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", bc.baseURL+"/api/backup/cancel/"+backupID, nil)
	if err != nil {
		return fmt.Errorf("failed to create cancel request: %v", err)
	}

	start := time.Now()
	resp, err := bc.httpClient.Do(req)
	duration := time.Since(start)

	bc.monitoring.RecordDuration("backup_client_request_duration", 
		map[string]string{"operation": "cancel_backup"}, duration)

	if err != nil {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "cancel_backup", "status": "error"}, 1)
		return fmt.Errorf("failed to cancel backup: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bc.monitoring.IncCounter("backup_client_requests", 
			map[string]string{"operation": "cancel_backup", "status": "failed"}, 1)
		return fmt.Errorf("cancel request failed with status: %d", resp.StatusCode)
	}

	bc.monitoring.IncCounter("backup_client_requests", 
		map[string]string{"operation": "cancel_backup", "status": "success"}, 1)

	return nil
}

// NotifyCompletion sends backup completion notification to integration bridge
func (bc *BackupClient) NotifyCompletion(ctx context.Context, bridgeURL string, event *BackupCompletionEvent) error {
	webhookRequest := map[string]interface{}{
		"id":        fmt.Sprintf("backup-completion-%s", event.BackupID),
		"type":      "backup_completed",
		"source":    "backup-tool",
		"timestamp": time.Now(),
		"data":      event,
	}

	data, err := json.Marshal(webhookRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal completion event: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", bridgeURL+"/webhooks/backup/completed", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create completion request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		bc.monitoring.IncCounter("backup_client_notifications", 
			map[string]string{"type": "completion", "status": "error"}, 1)
		return fmt.Errorf("failed to notify completion: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bc.monitoring.IncCounter("backup_client_notifications", 
			map[string]string{"type": "completion", "status": "failed"}, 1)
		return fmt.Errorf("completion notification failed with status: %d", resp.StatusCode)
	}

	bc.monitoring.IncCounter("backup_client_notifications", 
		map[string]string{"type": "completion", "status": "success"}, 1)

	return nil
}

// GetHealthStatus returns the health status of the backup service
func (bc *BackupClient) GetHealthStatus(ctx context.Context) (*HealthInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", bc.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health request: %v", err)
	}

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get health status: %v", err)
	}
	defer resp.Body.Close()

	var health HealthInfo
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %v", err)
	}

	return &health, nil
}

// HealthInfo represents service health information
type HealthInfo struct {
	Status       string                 `json:"status"`
	Timestamp    time.Time              `json:"timestamp"`
	Version      string                 `json:"version"`
	Uptime       time.Duration          `json:"uptime"`
	Dependencies map[string]string      `json:"dependencies"`
	Metrics      map[string]interface{} `json:"metrics"`
}