package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sharedconfig "shared-config/config"
)

// TriggerType defines the type of trigger mechanism
type TriggerType string

const (
	TriggerTypeFile    TriggerType = "file"
	TriggerTypeWebhook TriggerType = "webhook"
	TriggerTypeProcess TriggerType = "process"
	TriggerTypeScript  TriggerType = "script"
)

// TriggerResult represents the result of a trigger operation
type TriggerResult struct {
	Success   bool              `json:"success"`
	Timestamp time.Time         `json:"timestamp"`
	Duration  time.Duration     `json:"duration"`
	Method    TriggerType       `json:"method"`
	Output    string            `json:"output,omitempty"`
	Error     string            `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// BackupCompletionEvent represents backup completion information
type BackupCompletionEvent struct {
	BackupID         string            `json:"backup_id"`
	ClusterName      string            `json:"cluster_name"`
	Timestamp        time.Time         `json:"timestamp"`
	Duration         time.Duration     `json:"duration"`
	NamespacesCount  int               `json:"namespaces_count"`
	ResourcesCount   int               `json:"resources_count"`
	Success          bool              `json:"success"`
	Errors           []string          `json:"errors,omitempty"`
	MinIOBucket      string            `json:"minio_bucket"`
	BackupSize       int64             `json:"backup_size,omitempty"`
	BackupLocation   string            `json:"backup_location"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// AutoTrigger handles automatic GitOps generation triggering
type AutoTrigger struct {
	config     *sharedconfig.SharedConfig
	logger     Logger
	httpClient *http.Client
}

// Logger interface for trigger operations
type Logger interface {
	Info(message string, fields map[string]interface{})
	Error(message string, fields map[string]interface{})
	Debug(message string, fields map[string]interface{})
}

// NewAutoTrigger creates a new auto-trigger instance
func NewAutoTrigger(config *sharedconfig.SharedConfig, logger Logger) *AutoTrigger {
	return &AutoTrigger{
		config: config,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TriggerGitOpsGeneration triggers GitOps generation after backup completion
func (at *AutoTrigger) TriggerGitOpsGeneration(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()
	
	at.logger.Info("auto_trigger_start", map[string]interface{}{
		"backup_id":    event.BackupID,
		"cluster":      event.ClusterName,
		"method":       at.config.Pipeline.Automation.TriggerOnBackupComplete,
		"enabled":      at.config.Pipeline.Automation.Enabled,
	})

	if !at.config.Pipeline.Automation.Enabled {
		at.logger.Info("auto_trigger_disabled", map[string]interface{}{
			"backup_id": event.BackupID,
		})
		return &TriggerResult{
			Success:   true,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    "disabled",
			Output:    "Auto-trigger is disabled",
		}, nil
	}

	var result *TriggerResult
	var err error

	// Try different trigger methods in order of preference
	triggerMethods := at.getTriggerMethods()

	for _, method := range triggerMethods {
		at.logger.Debug("trying_trigger_method", map[string]interface{}{
			"method":    string(method),
			"backup_id": event.BackupID,
		})

		switch method {
		case TriggerTypeFile:
			result, err = at.triggerViaFile(ctx, event)
		case TriggerTypeWebhook:
			result, err = at.triggerViaWebhook(ctx, event)
		case TriggerTypeProcess:
			result, err = at.triggerViaProcess(ctx, event)
		case TriggerTypeScript:
			result, err = at.triggerViaScript(ctx, event)
		default:
			continue
		}

		if err == nil && result.Success {
			at.logger.Info("auto_trigger_success", map[string]interface{}{
				"method":       string(method),
				"backup_id":    event.BackupID,
				"duration":     result.Duration,
				"gitops_output": result.Output,
			})
			return result, nil
		}

		at.logger.Error("auto_trigger_method_failed", map[string]interface{}{
			"method":    string(method),
			"backup_id": event.BackupID,
			"error":     err,
		})
	}

	// If all methods failed, return the last error
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    triggerMethods[len(triggerMethods)-1],
			Error:     err.Error(),
		}, err
	}

	return result, nil
}

// getTriggerMethods returns the ordered list of trigger methods to try
func (at *AutoTrigger) getTriggerMethods() []TriggerType {
	methods := []TriggerType{}

	// Add methods based on configuration availability
	if at.config.Pipeline.Notifications.Webhook.URL != "" {
		methods = append(methods, TriggerTypeWebhook)
	}

	// Always add process and file methods as fallbacks
	methods = append(methods, TriggerTypeProcess, TriggerTypeScript, TriggerTypeFile)

	return methods
}

// triggerViaFile creates a signal file that can be monitored by external processes
func (at *AutoTrigger) triggerViaFile(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()

	// Create trigger directory if it doesn't exist
	triggerDir := "/tmp/backup-gitops-triggers"
	if err := os.MkdirAll(triggerDir, 0755); err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeFile,
			Error:     fmt.Sprintf("Failed to create trigger directory: %v", err),
		}, err
	}

	// Create signal file with backup event data
	signalFile := filepath.Join(triggerDir, fmt.Sprintf("backup-complete-%s-%d.json", event.ClusterName, event.Timestamp.Unix()))
	
	eventData, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeFile,
			Error:     fmt.Sprintf("Failed to marshal event data: %v", err),
		}, err
	}

	if err := os.WriteFile(signalFile, eventData, 0644); err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeFile,
			Error:     fmt.Sprintf("Failed to write signal file: %v", err),
		}, err
	}

	return &TriggerResult{
		Success:   true,
		Timestamp: startTime,
		Duration:  time.Since(startTime),
		Method:    TriggerTypeFile,
		Output:    fmt.Sprintf("Created signal file: %s", signalFile),
		Metadata: map[string]string{
			"signal_file": signalFile,
			"trigger_dir": triggerDir,
		},
	}, nil
}

// triggerViaWebhook sends a webhook notification that can trigger GitOps generation
func (at *AutoTrigger) triggerViaWebhook(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()

	webhookURL := at.config.Pipeline.Notifications.Webhook.URL
	if webhookURL == "" {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     "Webhook URL not configured",
		}, fmt.Errorf("webhook URL not configured")
	}

	// Prepare webhook payload
	payload := map[string]interface{}{
		"event_type": "backup_complete",
		"timestamp":  event.Timestamp,
		"backup":     event,
		"trigger":    "auto_gitops",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     fmt.Sprintf("Failed to marshal webhook payload: %v", err),
		}, err
	}

	// Send webhook request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     fmt.Sprintf("Failed to create webhook request: %v", err),
		}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "backup-gitops-auto-trigger/1.0")
	req.Header.Set("X-Event-Type", "backup_complete")

	resp, err := at.httpClient.Do(req)
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     fmt.Sprintf("Webhook request failed: %v", err),
		}, err
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     fmt.Sprintf("Webhook returned status %d: %s", resp.StatusCode, string(responseBody)),
		}, fmt.Errorf("webhook failed with status %d", resp.StatusCode)
	}

	return &TriggerResult{
		Success:   true,
		Timestamp: startTime,
		Duration:  time.Since(startTime),
		Method:    TriggerTypeWebhook,
		Output:    fmt.Sprintf("Webhook sent successfully (status %d): %s", resp.StatusCode, string(responseBody)),
		Metadata: map[string]string{
			"webhook_url":    webhookURL,
			"response_status": fmt.Sprintf("%d", resp.StatusCode),
		},
	}, nil
}

// triggerViaProcess directly spawns the GitOps generation process
func (at *AutoTrigger) triggerViaProcess(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()

	// Find GitOps binary
	gitopsBinary := at.findGitOpsBinary()
	if gitopsBinary == "" {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeProcess,
			Error:     "GitOps binary not found",
		}, fmt.Errorf("GitOps binary not found")
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(at.config.Pipeline.Automation.MaxWaitTime)*time.Second)
	defer cancel()

	// Prepare command arguments
	args := []string{
		"--cluster", event.ClusterName,
		"--bucket", event.MinIOBucket,
	}

	// Add configuration if available
	if configPath := at.findSharedConfigPath(); configPath != "" {
		args = append(args, "--config", configPath)
	}

	// Create and execute command
	cmd := exec.CommandContext(cmdCtx, gitopsBinary, args...)
	
	// Set environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, 
		fmt.Sprintf("MINIO_ENDPOINT=%s", at.config.Storage.Endpoint),
		fmt.Sprintf("MINIO_ACCESS_KEY=%s", at.config.Storage.AccessKey),
		fmt.Sprintf("MINIO_SECRET_KEY=%s", at.config.Storage.SecretKey),
		fmt.Sprintf("MINIO_BUCKET=%s", at.config.Storage.Bucket),
		fmt.Sprintf("CLUSTER_NAME=%s", at.config.Cluster.Name),
		fmt.Sprintf("GIT_REPOSITORY=%s", at.config.GitOps.Repository.URL),
		fmt.Sprintf("BACKUP_TRIGGER_ID=%s", event.BackupID),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeProcess,
			Error:     fmt.Sprintf("GitOps process failed: %v", err),
			Output:    string(output),
		}, err
	}

	return &TriggerResult{
		Success:   true,
		Timestamp: startTime,
		Duration:  time.Since(startTime),
		Method:    TriggerTypeProcess,
		Output:    string(output),
		Metadata: map[string]string{
			"binary_path": gitopsBinary,
			"arguments":   strings.Join(args, " "),
		},
	}, nil
}

// triggerViaScript runs the pipeline integration script
func (at *AutoTrigger) triggerViaScript(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()

	// Find pipeline integration script
	scriptPath := at.findPipelineScript()
	if scriptPath == "" {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeScript,
			Error:     "Pipeline integration script not found",
		}, fmt.Errorf("pipeline integration script not found")
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(at.config.Pipeline.Automation.MaxWaitTime)*time.Second)
	defer cancel()

	// Prepare script arguments
	args := []string{
		"gitops-only",
		"--verbose",
	}

	if configPath := at.findSharedConfigPath(); configPath != "" {
		args = append(args, "--config", configPath)
	}

	// Create and execute command
	cmd := exec.CommandContext(cmdCtx, scriptPath, args...)
	
	// Set environment variables including backup event context
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("BACKUP_TRIGGER_ID=%s", event.BackupID),
		fmt.Sprintf("BACKUP_TIMESTAMP=%d", event.Timestamp.Unix()),
		fmt.Sprintf("BACKUP_CLUSTER=%s", event.ClusterName),
		fmt.Sprintf("BACKUP_SUCCESS=%t", event.Success),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeScript,
			Error:     fmt.Sprintf("Pipeline script failed: %v", err),
			Output:    string(output),
		}, err
	}

	return &TriggerResult{
		Success:   true,
		Timestamp: startTime,
		Duration:  time.Since(startTime),
		Method:    TriggerTypeScript,
		Output:    string(output),
		Metadata: map[string]string{
			"script_path": scriptPath,
			"arguments":   strings.Join(args, " "),
		},
	}, nil
}

// Helper methods for finding binaries and scripts

func (at *AutoTrigger) findGitOpsBinary() string {
	candidates := []string{
		"minio-to-git",
		"./minio-to-git",
		"../kOTN/minio-to-git",
		"/usr/local/bin/minio-to-git",
		"/usr/bin/minio-to-git",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}

	return ""
}

func (at *AutoTrigger) findPipelineScript() string {
	candidates := []string{
		"./scripts/pipeline-integration.sh",
		"../shared/scripts/pipeline-integration.sh",
		"/usr/local/bin/pipeline-integration.sh",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func (at *AutoTrigger) findSharedConfigPath() string {
	candidates := []string{
		"shared-config.yaml",
		"./config/shared-config.yaml",
		"../shared/config/shared-config.yaml",
		"/etc/backup-gitops/config.yaml",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}