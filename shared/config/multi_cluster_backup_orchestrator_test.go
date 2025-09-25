package sharedconfig

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMultiClusterBackupOrchestratorCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      *MultiClusterConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil_config",
			config:      nil,
			expectError: true,
			errorMsg:    "multi-cluster configuration is required",
		},
		{
			name: "disabled_multi_cluster",
			config: &MultiClusterConfig{
				Enabled: false,
			},
			expectError: true,
			errorMsg:    "multi-cluster support is not enabled",
		},
		{
			name: "valid_config",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "test-cluster",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "test-cluster",
						Endpoint: "https://api.test.k8s.local:6443",
						Auth: ClusterAuthConfig{
							Method: "token",
							Token: TokenAuthConfig{
								Value: "test-token-123",
								Type:  "bearer",
							},
						},
						Storage: StorageConfig{
							Type:      "minio",
							Endpoint:  "minio.test.local:9000",
							AccessKey: "testkey",
							SecretKey: "testsecret",
							Bucket:    "test-backups",
						},
					},
				},
				Coordination: CoordinationConfig{
					Timeout:             600,
					RetryAttempts:       3,
					FailureThreshold:    1,
					HealthCheckInterval: "30s",
				},
				Scheduling: SchedulingConfig{
					Strategy:              "round_robin",
					MaxConcurrentClusters: 2,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator, err := NewMultiClusterBackupOrchestrator(tt.config)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if orchestrator == nil {
				t.Error("Expected non-nil orchestrator")
				return
			}

			// Verify orchestrator initialization
			if orchestrator.config != tt.config {
				t.Error("Config not properly set")
			}
			
			if orchestrator.clusterManager == nil {
				t.Error("Cluster manager not initialized")
			}
			
			if orchestrator.backupExecutors == nil {
				t.Error("Backup executors not initialized")
			}
			
			// Clean up
			orchestrator.Shutdown(context.Background())
		})
	}
}

func TestMultiClusterBackupExecution(t *testing.T) {
	// Create test configuration
	config := createTestMultiClusterConfig()
	
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	// Mock cluster health for testing
	for _, executor := range orchestrator.backupExecutors {
		executor.isHealthy = true
	}

	t.Run("sequential_backup_execution", func(t *testing.T) {
		// Set execution mode to sequential
		orchestrator.config.Mode = "sequential"
		
		result, err := orchestrator.ExecuteBackup()
		if err != nil {
			t.Errorf("Sequential backup execution failed: %v", err)
			return
		}
		
		validateBackupResult(t, result, "sequential")
	})

	t.Run("parallel_backup_execution", func(t *testing.T) {
		// Set execution mode to parallel
		orchestrator.config.Mode = "parallel"
		
		result, err := orchestrator.ExecuteBackup()
		if err != nil {
			t.Errorf("Parallel backup execution failed: %v", err)
			return
		}
		
		validateBackupResult(t, result, "parallel")
	})
}

func TestClusterBackupExecutor(t *testing.T) {
	config := createTestMultiClusterConfig()
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	t.Run("executor_initialization", func(t *testing.T) {
		// Verify all executors are initialized
		expectedClusters := []string{"test-cluster-1", "test-cluster-2"}
		
		for _, clusterName := range expectedClusters {
			executor, exists := orchestrator.backupExecutors[clusterName]
			if !exists {
				t.Errorf("Executor for cluster %s not found", clusterName)
				continue
			}
			
			if executor.clusterName != clusterName {
				t.Errorf("Executor cluster name mismatch: expected %s, got %s", 
					clusterName, executor.clusterName)
			}
			
			if !executor.backupConfig.Enabled {
				t.Errorf("Executor for cluster %s is not enabled", clusterName)
			}
		}
	})

	t.Run("executor_backup_config", func(t *testing.T) {
		// Test default backup configuration
		config := orchestrator.getDefaultBackupConfig("test-cluster")
		
		if !config.Enabled {
			t.Error("Default backup config should be enabled")
		}
		
		if len(config.ExcludedNamespaces) == 0 {
			t.Error("Default backup config should have excluded namespaces")
		}
		
		if len(config.BackupResources) == 0 {
			t.Error("Default backup config should have backup resources")
		}
		
		if config.RetentionDays <= 0 {
			t.Error("Default backup config should have positive retention days")
		}
	})
}

func TestBackupResultCalculation(t *testing.T) {
	config := createTestMultiClusterConfig()
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	t.Run("all_successful_results", func(t *testing.T) {
		// Create mock successful results
		clusterResults := make(map[string]*ClusterBackupResult)
		clusterResults["cluster-1"] = &ClusterBackupResult{
			ClusterName:        "cluster-1",
			Status:            BackupStatusCompleted,
			NamespacesBackedUp: 10,
			ResourcesBackedUp:  50,
			Duration:          5 * time.Minute,
		}
		clusterResults["cluster-2"] = &ClusterBackupResult{
			ClusterName:        "cluster-2",
			Status:            BackupStatusCompleted,
			NamespacesBackedUp: 8,
			ResourcesBackedUp:  40,
			Duration:          4 * time.Minute,
		}
		
		startTime := time.Now().Add(-10 * time.Minute)
		endTime := startTime.Add(10 * time.Minute)
		
		result := orchestrator.calculateOverallResult(clusterResults, startTime, endTime)
		
		if result.OverallStatus != BackupStatusCompleted {
			t.Errorf("Expected overall status %s, got %s", 
				BackupStatusCompleted, result.OverallStatus)
		}
		
		if result.SuccessfulClusters != 2 {
			t.Errorf("Expected 2 successful clusters, got %d", result.SuccessfulClusters)
		}
		
		if result.FailedClusters != 0 {
			t.Errorf("Expected 0 failed clusters, got %d", result.FailedClusters)
		}
	})

	t.Run("partial_failure_results", func(t *testing.T) {
		// Create mock partial failure results
		clusterResults := make(map[string]*ClusterBackupResult)
		clusterResults["cluster-1"] = &ClusterBackupResult{
			Status: BackupStatusCompleted,
		}
		clusterResults["cluster-2"] = &ClusterBackupResult{
			Status: BackupStatusFailed,
			Errors: []error{fmt.Errorf("backup failed")},
		}
		
		startTime := time.Now().Add(-10 * time.Minute)
		endTime := startTime.Add(10 * time.Minute)
		
		result := orchestrator.calculateOverallResult(clusterResults, startTime, endTime)
		
		// With failure threshold of 1, this should still be considered successful
		if orchestrator.config.Coordination.FailureThreshold >= 1 {
			if result.OverallStatus != BackupStatusCompleted {
				t.Errorf("Expected overall status %s with failure threshold, got %s", 
					BackupStatusCompleted, result.OverallStatus)
			}
		}
		
		if result.SuccessfulClusters != 1 {
			t.Errorf("Expected 1 successful cluster, got %d", result.SuccessfulClusters)
		}
		
		if result.FailedClusters != 1 {
			t.Errorf("Expected 1 failed cluster, got %d", result.FailedClusters)
		}
	})
}

func TestOrchestratorHealthChecks(t *testing.T) {
	config := createTestMultiClusterConfig()
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	t.Run("healthy_executors_filtering", func(t *testing.T) {
		// Mark one executor as unhealthy
		for _, executor := range orchestrator.backupExecutors {
			executor.isHealthy = false
			break
		}
		
		healthyExecutors := orchestrator.getHealthyExecutors()
		expectedCount := len(orchestrator.backupExecutors) - 1
		
		if len(healthyExecutors) != expectedCount {
			t.Errorf("Expected %d healthy executors, got %d", 
				expectedCount, len(healthyExecutors))
		}
		
		// Verify all returned executors are healthy
		for _, executor := range healthyExecutors {
			if !executor.isHealthy {
				t.Error("Unhealthy executor returned in healthy list")
			}
		}
	})
}

func TestOrchestratorStatistics(t *testing.T) {
	config := createTestMultiClusterConfig()
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	t.Run("executor_status", func(t *testing.T) {
		status := orchestrator.GetExecutorStatus()
		
		if len(status) != len(orchestrator.backupExecutors) {
			t.Errorf("Expected %d executor statuses, got %d", 
				len(orchestrator.backupExecutors), len(status))
		}
		
		for clusterName, executor := range orchestrator.backupExecutors {
			clusterStatus, exists := status[clusterName]
			if !exists {
				t.Errorf("Status for cluster %s not found", clusterName)
				continue
			}
			
			statusMap, ok := clusterStatus.(map[string]interface{})
			if !ok {
				t.Errorf("Invalid status format for cluster %s", clusterName)
				continue
			}
			
			if statusMap["healthy"] != executor.isHealthy {
				t.Errorf("Health status mismatch for cluster %s", clusterName)
			}
			
			if statusMap["enabled"] != executor.backupConfig.Enabled {
				t.Errorf("Enabled status mismatch for cluster %s", clusterName)
			}
		}
	})

	t.Run("orchestrator_stats", func(t *testing.T) {
		stats := orchestrator.GetOrchestratorStats()
		
		expectedFields := []string{
			"uptime_seconds",
			"total_executions",
			"successful_runs",
			"failed_runs",
			"configured_clusters",
			"execution_mode",
			"max_parallel",
			"backup_timeout",
		}
		
		for _, field := range expectedFields {
			if _, exists := stats[field]; !exists {
				t.Errorf("Expected field %s not found in stats", field)
			}
		}
		
		// Verify some specific values
		if stats["configured_clusters"] != len(orchestrator.backupExecutors) {
			t.Error("Configured clusters count mismatch")
		}
		
		if stats["execution_mode"] != orchestrator.config.Mode {
			t.Error("Execution mode mismatch")
		}
	})
}

func TestOrchestratorShutdown(t *testing.T) {
	config := createTestMultiClusterConfig()
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}

	t.Run("graceful_shutdown", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		err := orchestrator.Shutdown(ctx)
		if err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
		
		// Verify context is cancelled
		select {
		case <-orchestrator.orchestratorCtx.Done():
			// Expected
		default:
			t.Error("Orchestrator context not cancelled after shutdown")
		}
	})
}

func TestAdvancedBackupOrchestrator(t *testing.T) {
	config := createTestMultiClusterConfig()
	
	// Add priority configuration for advanced orchestrator
	config.Scheduling.ClusterPriorities = []ClusterPriority{
		{Cluster: "test-cluster-1", Priority: 1},
		{Cluster: "test-cluster-2", Priority: 2},
	}
	
	advancedOrchestrator, err := NewAdvancedBackupOrchestrator(config)
	if err != nil {
		t.Fatalf("Failed to create advanced orchestrator: %v", err)
	}
	defer advancedOrchestrator.Shutdown(context.Background())

	t.Run("advanced_orchestrator_creation", func(t *testing.T) {
		if advancedOrchestrator.baseOrchestrator == nil {
			t.Error("Base orchestrator not initialized")
		}
		
		if advancedOrchestrator.priorityScheduler == nil {
			t.Error("Priority scheduler not initialized")
		}
		
		if advancedOrchestrator.loadBalancer == nil {
			t.Error("Load balancer not initialized")
		}
		
		if len(advancedOrchestrator.circuitBreakers) == 0 {
			t.Error("Circuit breakers not initialized")
		}
		
		// Mock cluster health for testing
		for _, executor := range advancedOrchestrator.baseOrchestrator.backupExecutors {
			executor.isHealthy = true
		}
	})

	t.Run("priority_scheduling", func(t *testing.T) {
		clusters := advancedOrchestrator.selectClustersWithPriority()
		
		if len(clusters) == 0 {
			t.Error("No clusters selected for backup")
			return
		}
		
		// Verify priority ordering (cluster-1 should come before cluster-2)
		if len(clusters) >= 2 {
			if clusters[0] != "test-cluster-1" {
				t.Errorf("Expected first cluster to be test-cluster-1, got %s", clusters[0])
			}
		}
	})

	t.Run("advanced_status", func(t *testing.T) {
		status := advancedOrchestrator.GetAdvancedStatus()
		
		expectedSections := []string{
			"base_orchestrator",
			"active_executions",
			"queue_length",
			"circuit_breakers",
			"system_health",
			"cluster_health",
		}
		
		for _, section := range expectedSections {
			if _, exists := status[section]; !exists {
				t.Errorf("Expected status section %s not found", section)
			}
		}
	})
}

// Helper functions

func createTestMultiClusterConfig() *MultiClusterConfig {
	return &MultiClusterConfig{
		Enabled:        true,
		Mode:           "sequential",
		DefaultCluster: "test-cluster-1",
		Clusters: []MultiClusterClusterConfig{
			{
				Name:     "test-cluster-1",
				Endpoint: "https://api.test-cluster-1.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "test-token-123",
						Type:  "bearer",
					},
				},
				Storage: StorageConfig{
					Type:      "minio",
					Endpoint:  "minio1.test.local:9000",
					AccessKey: "testkey1",
					SecretKey: "testsecret1",
					Bucket:    "test-backups-1",
				},
			},
			{
				Name:     "test-cluster-2",
				Endpoint: "https://api.test-cluster-2.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "test-token-456",
						Type:  "bearer",
					},
				},
				Storage: StorageConfig{
					Type:      "minio",
					Endpoint:  "minio2.test.local:9000",
					AccessKey: "testkey2",
					SecretKey: "testsecret2",
					Bucket:    "test-backups-2",
				},
			},
		},
		Coordination: CoordinationConfig{
			Timeout:             600,
			RetryAttempts:       3,
			FailureThreshold:    1,
			HealthCheckInterval: "30s",
		},
		Scheduling: SchedulingConfig{
			Strategy:              "priority",
			MaxConcurrentClusters: 2,
		},
	}
}

func validateBackupResult(t *testing.T, result *MultiClusterBackupResult, mode string) {
	if result == nil {
		t.Error("Backup result is nil")
		return
	}
	
	if result.ExecutionMode != mode {
		t.Errorf("Expected execution mode %s, got %s", mode, result.ExecutionMode)
	}
	
	if result.TotalClusters <= 0 {
		t.Error("Total clusters should be greater than 0")
	}
	
	if result.TotalDuration <= 0 {
		t.Error("Total duration should be greater than 0")
	}
	
	if result.OverallStatus == "" {
		t.Error("Overall status should not be empty")
	}
	
	if len(result.ClusterResults) == 0 {
		t.Error("Cluster results should not be empty")
	}
	
	// Verify cluster results consistency
	calculatedSuccessful := 0
	calculatedFailed := 0
	
	for clusterName, clusterResult := range result.ClusterResults {
		if clusterResult == nil {
			t.Errorf("Cluster result for %s is nil", clusterName)
			continue
		}
		
		if clusterResult.ClusterName != clusterName {
			t.Errorf("Cluster name mismatch: expected %s, got %s", 
				clusterName, clusterResult.ClusterName)
		}
		
		if clusterResult.Status == BackupStatusCompleted {
			calculatedSuccessful++
		} else {
			calculatedFailed++
		}
	}
	
	if calculatedSuccessful != result.SuccessfulClusters {
		t.Errorf("Successful clusters count mismatch: expected %d, got %d", 
			calculatedSuccessful, result.SuccessfulClusters)
	}
	
	if calculatedFailed != result.FailedClusters {
		t.Errorf("Failed clusters count mismatch: expected %d, got %d", 
			calculatedFailed, result.FailedClusters)
	}
}

// Benchmark tests

func BenchmarkMultiClusterBackupExecution(b *testing.B) {
	config := createTestMultiClusterConfig()
	orchestrator, err := NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		b.Fatalf("Failed to create orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := orchestrator.ExecuteBackup()
		if err != nil {
			b.Errorf("Backup execution failed: %v", err)
		}
	}
}

func BenchmarkAdvancedOrchestratorExecution(b *testing.B) {
	config := createTestMultiClusterConfig()
	config.Scheduling.ClusterPriorities = []ClusterPriority{
		{Cluster: "test-cluster-1", Priority: 1},
		{Cluster: "test-cluster-2", Priority: 2},
	}
	
	orchestrator, err := NewAdvancedBackupOrchestrator(config)
	if err != nil {
		b.Fatalf("Failed to create advanced orchestrator: %v", err)
	}
	defer orchestrator.Shutdown(context.Background())

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := orchestrator.ExecuteAdvancedBackup()
		if err != nil {
			b.Errorf("Advanced backup execution failed: %v", err)
		}
	}
}