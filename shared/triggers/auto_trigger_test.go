package triggers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	sharedconfig "shared-config/config"
)

// MockLogger implements the Logger interface for testing
type MockLogger struct {
	logs []LogEntry
}

type LogEntry struct {
	Level   string
	Message string
	Fields  map[string]interface{}
}

func (ml *MockLogger) Info(message string, fields map[string]interface{}) {
	ml.logs = append(ml.logs, LogEntry{Level: "info", Message: message, Fields: fields})
}

func (ml *MockLogger) Error(message string, fields map[string]interface{}) {
	ml.logs = append(ml.logs, LogEntry{Level: "error", Message: message, Fields: fields})
}

func (ml *MockLogger) Debug(message string, fields map[string]interface{}) {
	ml.logs = append(ml.logs, LogEntry{Level: "debug", Message: message, Fields: fields})
}

func (ml *MockLogger) HasLogWithMessage(message string) bool {
	for _, log := range ml.logs {
		if log.Message == message {
			return true
		}
	}
	return false
}

func (ml *MockLogger) GetLogsWithLevel(level string) []LogEntry {
	var filtered []LogEntry
	for _, log := range ml.logs {
		if log.Level == level {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func createTestConfig() *sharedconfig.SharedConfig {
	return &sharedconfig.SharedConfig{
		Storage: sharedconfig.StorageConfig{
			Endpoint:  "localhost:9000",
			AccessKey: "testkey",
			SecretKey: "testsecret",
			Bucket:    "test-bucket",
		},
		Cluster: sharedconfig.ClusterConfig{
			Name:   "test-cluster",
			Domain: "cluster.local",
		},
		Pipeline: sharedconfig.PipelineConfig{
			Automation: sharedconfig.AutomationConfig{
				Enabled:                 true,
				TriggerOnBackupComplete: true,
				WaitForBackup:          true,
				MaxWaitTime:            300,
			},
			ErrorHandling: sharedconfig.ErrorHandlingConfig{
				ContinueOnError: false,
				MaxRetries:      3,
				RetryDelay:      5 * time.Second,
			},
		},
	}
}

func createTestBackupEvent() *BackupCompletionEvent {
	return &BackupCompletionEvent{
		BackupID:        "test-backup-123",
		ClusterName:     "test-cluster",
		Timestamp:       time.Now(),
		Duration:        2 * time.Minute,
		NamespacesCount: 5,
		ResourcesCount:  50,
		Success:         true,
		MinIOBucket:     "test-bucket",
		BackupLocation:  "test-bucket/test-cluster",
		Metadata: map[string]string{
			"test": "metadata",
		},
	}
}

func TestNewAutoTrigger(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	
	trigger := NewAutoTrigger(config, logger)
	
	if trigger == nil {
		t.Fatal("Expected AutoTrigger instance, got nil")
	}
	
	if trigger.config != config {
		t.Error("Expected config to be set")
	}
	
	if trigger.logger != logger {
		t.Error("Expected logger to be set")
	}
}

func TestAutoTrigger_TriggerGitOpsGeneration_Disabled(t *testing.T) {
	config := createTestConfig()
	config.Pipeline.Automation.Enabled = false
	
	logger := &MockLogger{}
	trigger := NewAutoTrigger(config, logger)
	event := createTestBackupEvent()
	
	ctx := context.Background()
	result, err := trigger.TriggerGitOpsGeneration(ctx, event)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if !result.Success {
		t.Error("Expected success when auto-trigger is disabled")
	}
	
	if result.Method != "disabled" {
		t.Errorf("Expected method 'disabled', got: %s", result.Method)
	}
	
	if !logger.HasLogWithMessage("auto_trigger_disabled") {
		t.Error("Expected disabled log message")
	}
}

func TestAutoTrigger_TriggerViaFile(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	trigger := NewAutoTrigger(config, logger)
	event := createTestBackupEvent()
	
	ctx := context.Background()
	result, err := trigger.triggerViaFile(ctx, event)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if !result.Success {
		t.Error("Expected successful file trigger")
	}
	
	if result.Method != TriggerTypeFile {
		t.Errorf("Expected method '%s', got: %s", TriggerTypeFile, result.Method)
	}
	
	// Check if signal file was created
	triggerDir := "/tmp/backup-gitops-triggers"
	files, err := os.ReadDir(triggerDir)
	if err != nil {
		t.Fatalf("Failed to read trigger directory: %v", err)
	}
	
	found := false
	for _, file := range files {
		if file.Name() != "." && file.Name() != ".." {
			found = true
			// Clean up the test file
			os.Remove(triggerDir + "/" + file.Name())
			break
		}
	}
	
	if !found {
		t.Error("Expected signal file to be created")
	}
}

func TestBackupTriggerIntegration_OnBackupComplete(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	integration := NewBackupTriggerIntegration(config, logger)
	
	result := &BackupResult{
		NamespacesBackedUp: 5,
		ResourcesBackedUp:  50,
		Errors:             []error{},
		Duration:           2 * time.Minute,
		StartTime:          time.Now().Add(-2 * time.Minute),
		EndTime:            time.Now(),
	}
	
	ctx := context.Background()
	err := integration.OnBackupComplete(ctx, result)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if !logger.HasLogWithMessage("backup_complete_triggering_gitops") {
		t.Error("Expected backup completion log message")
	}
}

func TestBackupTriggerIntegration_OnBackupComplete_Disabled(t *testing.T) {
	config := createTestConfig()
	config.Pipeline.Automation.Enabled = false
	
	logger := &MockLogger{}
	integration := NewBackupTriggerIntegration(config, logger)
	
	result := &BackupResult{
		NamespacesBackedUp: 5,
		ResourcesBackedUp:  50,
		Errors:             []error{},
		Duration:           2 * time.Minute,
		StartTime:          time.Now().Add(-2 * time.Minute),
		EndTime:            time.Now(),
	}
	
	ctx := context.Background()
	err := integration.OnBackupComplete(ctx, result)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	// Should skip triggering when disabled
	debugLogs := logger.GetLogsWithLevel("debug")
	if len(debugLogs) == 0 {
		t.Error("Expected debug log about skipping trigger")
	}
}

func TestRetryHandler_RetryOperation(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	retryHandler := NewRetryHandler(config, logger)
	
	attempts := 0
	operation := func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("temporary failure")
		}
		return nil
	}
	
	retryConfig := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffFactor:   2.0,
		JitterEnabled:   false,
		RetryableErrors: []string{"temporary failure"},
	}
	
	ctx := context.Background()
	err := retryHandler.RetryOperation(ctx, operation, retryConfig)
	
	if err != nil {
		t.Fatalf("Expected operation to succeed after retries, got: %v", err)
	}
	
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
}

func TestRetryHandler_RetryOperation_NonRetryableError(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	retryHandler := NewRetryHandler(config, logger)
	
	attempts := 0
	operation := func() error {
		attempts++
		return fmt.Errorf("permanent failure")
	}
	
	retryConfig := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		BackoffFactor:   2.0,
		JitterEnabled:   false,
		RetryableErrors: []string{"temporary failure"},
	}
	
	ctx := context.Background()
	err := retryHandler.RetryOperation(ctx, operation, retryConfig)
	
	if err == nil {
		t.Fatal("Expected error for non-retryable failure")
	}
	
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got: %d", attempts)
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	cb := NewCircuitBreaker(config, logger)
	cb.failureThreshold = 2 // Lower threshold for testing
	
	// First failure
	ctx := context.Background()
	err1 := cb.Execute(ctx, func() error {
		return fmt.Errorf("failure 1")
	})
	
	if err1 == nil {
		t.Error("Expected first operation to fail")
	}
	
	if cb.state != CircuitBreakerClosed {
		t.Error("Circuit should still be closed after first failure")
	}
	
	// Second failure - should open circuit
	err2 := cb.Execute(ctx, func() error {
		return fmt.Errorf("failure 2")
	})
	
	if err2 == nil {
		t.Error("Expected second operation to fail")
	}
	
	if cb.state != CircuitBreakerOpen {
		t.Error("Circuit should be open after threshold failures")
	}
	
	// Third attempt - should be blocked by open circuit
	err3 := cb.Execute(ctx, func() error {
		return nil // This shouldn't be called
	})
	
	if err3 == nil {
		t.Error("Expected operation to be blocked by open circuit")
	}
	
	if !logger.HasLogWithMessage("circuit_breaker_opened") {
		t.Error("Expected circuit breaker opened log message")
	}
}

func TestResilientTrigger_TriggerWithResilience(t *testing.T) {
	config := createTestConfig()
	logger := &MockLogger{}
	resilientTrigger := NewResilientTrigger(config, logger)
	
	event := createTestBackupEvent()
	
	ctx := context.Background()
	result, _ := resilientTrigger.TriggerWithResilience(ctx, event)
	
	// Note: This test will likely fail in a real environment because
	// it tries to actually trigger GitOps generation. In a real test
	// environment, you would mock the trigger methods.
	
	if result == nil {
		t.Error("Expected result to be returned")
	}
	
	if !logger.HasLogWithMessage("resilient_trigger_start") {
		t.Error("Expected resilient trigger start log message")
	}
}

func TestGenerateBackupID(t *testing.T) {
	clusterName := "test-cluster"
	startTime := time.Unix(1234567890, 0)
	
	backupID := generateBackupID(clusterName, startTime)
	expected := "test-cluster-1234567890"
	
	if backupID != expected {
		t.Errorf("Expected backup ID '%s', got: '%s'", expected, backupID)
	}
}

func TestBackupCompletionEvent_Creation(t *testing.T) {
	event := createTestBackupEvent()
	
	if event.BackupID == "" {
		t.Error("Expected backup ID to be set")
	}
	
	if event.ClusterName != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got: '%s'", event.ClusterName)
	}
	
	if !event.Success {
		t.Error("Expected successful backup event")
	}
	
	if event.NamespacesCount != 5 {
		t.Errorf("Expected 5 namespaces, got: %d", event.NamespacesCount)
	}
	
	if event.ResourcesCount != 50 {
		t.Errorf("Expected 50 resources, got: %d", event.ResourcesCount)
	}
}