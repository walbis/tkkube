package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
)

// Test configuration
var testConfig = &sharedconfig.SharedConfig{
	SchemaVersion: "2.1.0",
	Description:   "Test configuration for integration bridge",
	Storage: sharedconfig.StorageConfig{
		Type:      "minio",
		Endpoint:  "localhost:9000",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Bucket:    "test-bucket",
		UseSSL:    false,
	},
	Cluster: sharedconfig.ClusterConfig{
		Name:   "test-cluster",
		Domain: "cluster.local",
	},
	GitOps: sharedconfig.GitOpsConfig{
		Repository: "https://github.com/test/repo",
		Branch:     "main",
		Path:       "clusters/test",
	},
	Integration: sharedconfig.IntegrationConfig{
		Enabled:     true,
		WebhookPort: 8080,
		Bridge: sharedconfig.BridgeConfig{
			Enabled:         true,
			HealthInterval:  30 * time.Second,
			EventBufferSize: 1000,
			MaxConcurrency:  10,
		},
		Communication: sharedconfig.CommunicationConfig{
			Method: "webhook",
			Endpoints: sharedconfig.EndpointsConfig{
				BackupTool:        "http://localhost:8080",
				GitOpsGenerator:   "http://localhost:8081",
				IntegrationBridge: "http://localhost:8082",
			},
			Retry: sharedconfig.RetryConfig{
				MaxAttempts:  3,
				InitialDelay: 1 * time.Second,
				MaxDelay:     30 * time.Second,
				Multiplier:   2.0,
			},
		},
		Triggers: sharedconfig.TriggerIntegrationConfig{
			AutoTrigger:       true,
			DelayAfterBackup:  30 * time.Second,
			ParallelExecution: false,
			FallbackMethods:   []string{"webhook", "process"},
		},
	},
}

// TestConfigManager tests the configuration manager
func TestConfigManager(t *testing.T) {
	t.Run("NewConfigManager", func(t *testing.T) {
		cm := NewConfigManager()
		if cm == nil {
			t.Fatal("Expected config manager to be created")
		}
		if cm.loader == nil {
			t.Fatal("Expected config loader to be initialized")
		}
	})

	t.Run("InitializeDefaults", func(t *testing.T) {
		cm := NewConfigManager()
		cm.config = &sharedconfig.SharedConfig{}
		
		err := cm.initializeIntegrationDefaults()
		if err != nil {
			t.Fatalf("Failed to initialize defaults: %v", err)
		}

		if cm.config.Integration.WebhookPort != 8080 {
			t.Errorf("Expected webhook port 8080, got %d", cm.config.Integration.WebhookPort)
		}
		if !cm.config.Integration.Bridge.Enabled {
			t.Error("Expected bridge to be enabled by default")
		}
		if cm.config.Integration.Communication.Method != "webhook" {
			t.Errorf("Expected webhook method, got %s", cm.config.Integration.Communication.Method)
		}
	})

	t.Run("ValidateConfiguration", func(t *testing.T) {
		cm := NewConfigManager()
		cm.config = testConfig

		err := cm.ValidateConfiguration()
		if err != nil {
			t.Fatalf("Configuration validation failed: %v", err)
		}
	})

	t.Run("CreateBackupToolConfig", func(t *testing.T) {
		cm := NewConfigManager()
		cm.config = testConfig

		backupConfig := cm.CreateBackupToolConfig()
		if backupConfig == nil {
			t.Fatal("Expected backup config to be created")
		}

		if _, ok := backupConfig["cluster"]; !ok {
			t.Error("Expected cluster config in backup tool config")
		}
		if _, ok := backupConfig["storage"]; !ok {
			t.Error("Expected storage config in backup tool config")
		}
	})

	t.Run("CreateGitOpsConfig", func(t *testing.T) {
		cm := NewConfigManager()
		cm.config = testConfig

		gitopsConfig := cm.CreateGitOpsConfig()
		if gitopsConfig == nil {
			t.Fatal("Expected GitOps config to be created")
		}

		if _, ok := gitopsConfig["gitops"]; !ok {
			t.Error("Expected GitOps config in GitOps tool config")
		}
		if _, ok := gitopsConfig["storage"]; !ok {
			t.Error("Expected storage config in GitOps tool config")
		}
	})
}

// TestIntegrationBridge tests the integration bridge
func TestIntegrationBridge(t *testing.T) {
	t.Run("NewIntegrationBridge", func(t *testing.T) {
		bridge, err := NewIntegrationBridge(testConfig)
		if err != nil {
			t.Fatalf("Failed to create integration bridge: %v", err)
		}
		defer bridge.Stop()

		if bridge.config != testConfig {
			t.Error("Expected config to be set")
		}
		if bridge.monitoringSystem == nil {
			t.Error("Expected monitoring system to be initialized")
		}
		if bridge.eventBus == nil {
			t.Error("Expected event bus to be initialized")
		}
	})

	t.Run("StartStop", func(t *testing.T) {
		bridge, err := NewIntegrationBridge(testConfig)
		if err != nil {
			t.Fatalf("Failed to create integration bridge: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Start bridge
		err = bridge.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start bridge: %v", err)
		}

		if !bridge.running {
			t.Error("Expected bridge to be running")
		}

		// Stop bridge
		err = bridge.Stop()
		if err != nil {
			t.Fatalf("Failed to stop bridge: %v", err)
		}

		if bridge.running {
			t.Error("Expected bridge to be stopped")
		}
	})

	t.Run("ComponentRegistration", func(t *testing.T) {
		bridge, err := NewIntegrationBridge(testConfig)
		if err != nil {
			t.Fatalf("Failed to create integration bridge: %v", err)
		}
		defer bridge.Stop()

		// Register backup tool
		err = bridge.RegisterBackupTool("http://localhost:8080", "1.0.0")
		if err != nil {
			t.Fatalf("Failed to register backup tool: %v", err)
		}

		status := bridge.GetComponentStatus()
		if backupStatus, ok := status["backup"]; ok {
			if backupStatus.Name != "backup-tool" {
				t.Errorf("Expected backup tool name, got %s", backupStatus.Name)
			}
			if backupStatus.Version != "1.0.0" {
				t.Errorf("Expected version 1.0.0, got %s", backupStatus.Version)
			}
		} else {
			t.Error("Expected backup component to be registered")
		}

		// Register GitOps tool
		err = bridge.RegisterGitOpsTool("http://localhost:8081", "2.1.0")
		if err != nil {
			t.Fatalf("Failed to register GitOps tool: %v", err)
		}

		status = bridge.GetComponentStatus()
		if gitopsStatus, ok := status["gitops"]; ok {
			if gitopsStatus.Name != "gitops-generator" {
				t.Errorf("Expected gitops generator name, got %s", gitopsStatus.Name)
			}
			if gitopsStatus.Version != "2.1.0" {
				t.Errorf("Expected version 2.1.0, got %s", gitopsStatus.Version)
			}
		} else {
			t.Error("Expected GitOps component to be registered")
		}
	})
}

// TestEventBus tests the event bus functionality
func TestEventBus(t *testing.T) {
	t.Run("SubscribePublish", func(t *testing.T) {
		eventBus := NewEventBus()
		eventReceived := make(chan bool, 1)

		// Subscribe to events
		eventBus.Subscribe("test_event", func(ctx context.Context, event *IntegrationEvent) error {
			if event.Type == "test_event" && event.Source == "test" {
				eventReceived <- true
			}
			return nil
		})

		// Publish event
		event := &IntegrationEvent{
			ID:        "test-1",
			Type:      "test_event",
			Source:    "test",
			Target:    "test",
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"message": "test"},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := eventBus.Publish(ctx, event)
		if err != nil {
			t.Fatalf("Failed to publish event: %v", err)
		}

		// Wait for event
		select {
		case <-eventReceived:
			// Success
		case <-time.After(2 * time.Second):
			t.Error("Event was not received within timeout")
		}
	})

	t.Run("GetSubscriberCount", func(t *testing.T) {
		eventBus := NewEventBus()

		count := eventBus.GetSubscriberCount("nonexistent")
		if count != 0 {
			t.Errorf("Expected 0 subscribers, got %d", count)
		}

		eventBus.Subscribe("test_event", func(ctx context.Context, event *IntegrationEvent) error {
			return nil
		})

		count = eventBus.GetSubscriberCount("test_event")
		if count != 1 {
			t.Errorf("Expected 1 subscriber, got %d", count)
		}
	})
}

// TestWebhookHandler tests webhook handling
func TestWebhookHandler(t *testing.T) {
	bridge, err := NewIntegrationBridge(testConfig)
	if err != nil {
		t.Fatalf("Failed to create integration bridge: %v", err)
	}
	defer bridge.Stop()

	// Create test server
	handler := bridge.webhookHandler
	server := httptest.NewServer(handler.mux)
	defer server.Close()

	t.Run("HealthEndpoint", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/health")
		if err != nil {
			t.Fatalf("Failed to call health endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response WebhookResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Error("Expected health check to succeed")
		}
	})

	t.Run("BackupCompletedWebhook", func(t *testing.T) {
		backupEvent := BackupCompletionEvent{
			BackupID:      "test-backup-1",
			ClusterName:   "test-cluster",
			Timestamp:     time.Now(),
			ResourceCount: 10,
			Size:          1024,
			Success:       true,
			MinIOPath:     "test-cluster/2024/01/01/backup-1",
		}

		webhookRequest := map[string]interface{}{
			"id":        "webhook-1",
			"type":      "backup_completed",
			"source":    "backup-tool",
			"timestamp": time.Now(),
			"data":      backupEvent,
		}

		requestBody, err := json.Marshal(webhookRequest)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		resp, err := http.Post(
			server.URL+"/webhooks/backup/completed",
			"application/json",
			strings.NewReader(string(requestBody)),
		)
		if err != nil {
			t.Fatalf("Failed to call webhook: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response WebhookResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Errorf("Expected webhook to succeed, got: %s", response.Message)
		}
	})

	t.Run("ComponentRegistration", func(t *testing.T) {
		registrationData := map[string]string{
			"endpoint": "http://localhost:8080",
			"version":  "1.0.0",
		}

		requestBody, err := json.Marshal(registrationData)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		resp, err := http.Post(
			server.URL+"/register/backup",
			"application/json",
			strings.NewReader(string(requestBody)),
		)
		if err != nil {
			t.Fatalf("Failed to call registration endpoint: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var response WebhookResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !response.Success {
			t.Errorf("Expected registration to succeed, got: %s", response.Message)
		}
	})
}

// TestBackupToGitOpsFlow tests the complete backup-to-GitOps integration flow
func TestBackupToGitOpsFlow(t *testing.T) {
	bridge, err := NewIntegrationBridge(testConfig)
	if err != nil {
		t.Fatalf("Failed to create integration bridge: %v", err)
	}
	defer bridge.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start bridge
	err = bridge.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	// Test backup completion event
	backupEvent := &BackupCompletionEvent{
		BackupID:      "integration-test-backup",
		ClusterName:   "test-cluster",
		Timestamp:     time.Now(),
		ResourceCount: 15,
		Size:          2048,
		Success:       true,
		MinIOPath:     "test-cluster/2024/01/01/integration-test-backup",
	}

	// Trigger GitOps generation
	err = bridge.TriggerGitOpsGeneration(ctx, backupEvent)
	if err != nil {
		t.Fatalf("Failed to trigger GitOps generation: %v", err)
	}

	// Verify event was published
	eventTypes := bridge.eventBus.GetEventTypes()
	found := false
	for _, eventType := range eventTypes {
		if eventType == "gitops_generation_requested" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected gitops_generation_requested event to be published")
	}

	// Check metrics
	metrics := bridge.GetIntegratedMetrics()
	if metrics == nil {
		t.Error("Expected integrated metrics to be available")
	}

	// Verify component status
	status := bridge.GetComponentStatus()
	if bridgeStatus, ok := status["integration-bridge"]; ok {
		if bridgeStatus.Status != "healthy" {
			t.Errorf("Expected bridge status to be healthy, got %s", bridgeStatus.Status)
		}
	} else {
		t.Error("Expected bridge status to be available")
	}
}

// TestMonitoringIntegration tests the monitoring integration
func TestMonitoringIntegration(t *testing.T) {
	bridge, err := NewIntegrationBridge(testConfig)
	if err != nil {
		t.Fatalf("Failed to create integration bridge: %v", err)
	}
	defer bridge.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = bridge.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	t.Run("GetIntegratedMetrics", func(t *testing.T) {
		metrics := bridge.GetIntegratedMetrics()
		if metrics == nil {
			t.Fatal("Expected integrated metrics to be available")
		}

		if metrics.ComponentBreakdown == nil {
			t.Error("Expected component breakdown to be available")
		}
	})

	t.Run("RecordIntegrationEvent", func(t *testing.T) {
		// Record a test event
		bridge.RecordIntegrationEvent("test_flow", 5*time.Second, true)

		// Give time for metrics to be recorded
		time.Sleep(100 * time.Millisecond)

		// Verify the event was recorded (would need access to metrics collector)
		metrics := bridge.GetIntegratedMetrics()
		if metrics == nil {
			t.Error("Expected metrics to be available after recording event")
		}
	})

	t.Run("GetOverallHealth", func(t *testing.T) {
		health := bridge.GetOverallHealth()
		if health.OverallStatus == "" {
			t.Error("Expected overall health status to be set")
		}

		if health.TotalComponents < 0 {
			t.Error("Expected total components to be non-negative")
		}
	})
}

// TestErrorHandling tests error handling scenarios
func TestErrorHandling(t *testing.T) {
	t.Run("InvalidConfiguration", func(t *testing.T) {
		invalidConfig := &sharedconfig.SharedConfig{}
		_, err := NewIntegrationBridge(invalidConfig)
		if err == nil {
			t.Error("Expected error with invalid configuration")
		}
	})

	t.Run("FailedBackupEvent", func(t *testing.T) {
		bridge, err := NewIntegrationBridge(testConfig)
		if err != nil {
			t.Fatalf("Failed to create integration bridge: %v", err)
		}
		defer bridge.Stop()

		ctx := context.Background()

		// Test failed backup event
		failedBackupEvent := &BackupCompletionEvent{
			BackupID:      "failed-backup",
			ClusterName:   "test-cluster",
			Timestamp:     time.Now(),
			ResourceCount: 0,
			Size:          0,
			Success:       false,
			ErrorMessage:  "Simulated backup failure",
			MinIOPath:     "",
		}

		// Should not trigger GitOps generation for failed backup
		err = bridge.TriggerGitOpsGeneration(ctx, failedBackupEvent)
		if err != nil {
			t.Errorf("Expected no error for failed backup handling, got: %v", err)
		}
	})
}

// BenchmarkIntegrationBridge benchmarks the integration bridge performance
func BenchmarkIntegrationBridge(b *testing.B) {
	bridge, err := NewIntegrationBridge(testConfig)
	if err != nil {
		b.Fatalf("Failed to create integration bridge: %v", err)
	}
	defer bridge.Stop()

	ctx := context.Background()
	bridge.Start(ctx)

	backupEvent := &BackupCompletionEvent{
		BackupID:      "benchmark-backup",
		ClusterName:   "test-cluster",
		Timestamp:     time.Now(),
		ResourceCount: 10,
		Size:          1024,
		Success:       true,
		MinIOPath:     "test-cluster/benchmark",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bridge.TriggerGitOpsGeneration(ctx, backupEvent)
		}
	})
}

// TestIntegrationExample demonstrates complete integration usage
func TestIntegrationExample(t *testing.T) {
	// This test demonstrates how to use the integration bridge in practice
	
	// 1. Load configuration
	configManager := NewConfigManager("config_integration.yaml")
	config, err := configManager.LoadConfig()
	if err != nil {
		// Use test config if file not found
		config = testConfig
	}

	// 2. Create and start integration bridge
	bridge, err := NewIntegrationBridge(config)
	if err != nil {
		t.Fatalf("Failed to create integration bridge: %v", err)
	}
	defer bridge.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = bridge.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	// 3. Register components (normally done by the components themselves)
	err = bridge.RegisterBackupTool("http://backup-tool:8080", "1.0.0")
	if err != nil {
		t.Fatalf("Failed to register backup tool: %v", err)
	}

	err = bridge.RegisterGitOpsTool("http://gitops-generator:8081", "2.1.0")
	if err != nil {
		t.Fatalf("Failed to register GitOps tool: %v", err)
	}

	// 4. Simulate backup completion
	backupEvent := &BackupCompletionEvent{
		BackupID:      "example-backup-123",
		ClusterName:   "production-cluster",
		Timestamp:     time.Now(),
		ResourceCount: 50,
		Size:          10485760, // 10MB
		Success:       true,
		MinIOPath:     "production-cluster/2024/01/15/backup-123",
	}

	// 5. Trigger GitOps generation
	err = bridge.TriggerGitOpsGeneration(ctx, backupEvent)
	if err != nil {
		t.Fatalf("Failed to trigger GitOps generation: %v", err)
	}

	// 6. Check system health
	health := bridge.GetOverallHealth()
	fmt.Printf("System Health: %s\n", health.OverallStatus)
	fmt.Printf("Healthy Components: %d/%d\n", health.HealthyComponents, health.TotalComponents)

	// 7. Get integrated metrics
	metrics := bridge.GetIntegratedMetrics()
	fmt.Printf("Total Integration Requests: %d\n", metrics.IntegrationFlow.TotalIntegrationRequests)
	fmt.Printf("Successful Integrations: %d\n", metrics.IntegrationFlow.SuccessfulIntegrations)

	// 8. Verify everything worked
	if health.OverallStatus != "healthy" && health.OverallStatus != "unknown" {
		t.Errorf("Expected system to be healthy, got: %s", health.OverallStatus)
	}

	fmt.Println("Integration bridge example completed successfully!")
}