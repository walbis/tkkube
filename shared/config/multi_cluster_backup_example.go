// Package main provides examples for using the multi-cluster backup orchestrator
// To run this example: go run multi_cluster_backup_example.go
// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	sharedconfig "shared-config/config"
)

func main() {
	fmt.Println("🚀 Multi-Cluster Backup Orchestrator Example")
	fmt.Println("============================================")

	// Example 1: Basic Multi-Cluster Backup Orchestrator
	if err := basicOrchestratorExample(); err != nil {
		log.Fatalf("Basic orchestrator example failed: %v", err)
	}

	// Example 2: Advanced Orchestrator with Priority Scheduling
	if err := advancedOrchestratorExample(); err != nil {
		log.Fatalf("Advanced orchestrator example failed: %v", err)
	}

	// Example 3: Orchestrator with Custom Configuration
	if err := customConfigurationExample(); err != nil {
		log.Fatalf("Custom configuration example failed: %v", err)
	}

	// Example 4: Long-running Orchestrator Service
	if err := longRunningServiceExample(); err != nil {
		log.Fatalf("Long-running service example failed: %v", err)
	}

	fmt.Println("✅ All examples completed successfully!")
}

// basicOrchestratorExample demonstrates basic usage of the multi-cluster backup orchestrator
func basicOrchestratorExample() error {
	fmt.Println("\n📋 Example 1: Basic Multi-Cluster Backup Orchestrator")
	fmt.Println("----------------------------------------------------")

	// Load configuration from file
	loader := sharedconfig.NewConfigLoader("multi-cluster-backup-example.yaml")
	config, err := loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("✓ Loaded configuration with %d clusters\n", len(config.MultiCluster.Clusters))

	// Create multi-cluster backup orchestrator
	orchestrator, err := sharedconfig.NewMultiClusterBackupOrchestrator(&config.MultiCluster)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}
	defer orchestrator.Shutdown(context.Background())

	fmt.Printf("✓ Created orchestrator in '%s' mode\n", config.MultiCluster.Mode)

	// Get initial status
	status := orchestrator.GetOrchestratorStats()
	fmt.Printf("✓ Orchestrator configured with %v clusters\n", status["configured_clusters"])

	// Execute backup
	fmt.Println("🔄 Starting backup execution...")
	result, err := orchestrator.ExecuteBackup()
	if err != nil {
		return fmt.Errorf("backup execution failed: %w", err)
	}

	// Display results
	displayBackupResults(result)

	return nil
}

// advancedOrchestratorExample demonstrates the advanced orchestrator with priority scheduling
func advancedOrchestratorExample() error {
	fmt.Println("\n🔧 Example 2: Advanced Orchestrator with Priority Scheduling")
	fmt.Println("----------------------------------------------------------")

	// Create advanced configuration
	config := createAdvancedTestConfiguration()

	// Create advanced backup orchestrator
	advancedOrchestrator, err := sharedconfig.NewAdvancedBackupOrchestrator(config)
	if err != nil {
		return fmt.Errorf("failed to create advanced orchestrator: %w", err)
	}
	defer advancedOrchestrator.Shutdown(context.Background())

	fmt.Printf("✓ Created advanced orchestrator with priority scheduling\n")

	// Get advanced status before execution
	advancedStatus := advancedOrchestrator.GetAdvancedStatus()
	fmt.Printf("✓ System health: %s\n", getSystemHealthStatus(advancedStatus))
	fmt.Printf("✓ Circuit breakers status: %d configured\n", len(getCircuitBreakersStatus(advancedStatus)))

	// Execute advanced backup
	fmt.Println("🔄 Starting advanced backup execution with priority scheduling...")
	result, err := advancedOrchestrator.ExecuteAdvancedBackup()
	if err != nil {
		log.Printf("⚠️  Advanced backup completed with errors: %v", err)
		// Continue to display partial results
	}

	if result != nil {
		displayBackupResults(result)
	}

	// Display final advanced status
	finalStatus := advancedOrchestrator.GetAdvancedStatus()
	fmt.Printf("✓ Final active executions: %v\n", finalStatus["active_executions"])

	return nil
}

// customConfigurationExample demonstrates creating a custom configuration programmatically
func customConfigurationExample() error {
	fmt.Println("\n⚙️  Example 3: Custom Configuration")
	fmt.Println("----------------------------------")

	// Create custom multi-cluster configuration
	config := &sharedconfig.MultiClusterConfig{
		Enabled:        true,
		Mode:           "parallel",
		DefaultCluster: "custom-prod",
		Clusters: []sharedconfig.MultiClusterClusterConfig{
			{
				Name:     "custom-prod",
				Endpoint: "https://api.custom-prod.k8s.example.com:6443",
				Auth: sharedconfig.ClusterAuthConfig{
					Method: "token",
					Token: sharedconfig.TokenAuthConfig{
						Value: os.Getenv("PROD_TOKEN"),
						Type:  "bearer",
					},
				},
				TLS: sharedconfig.ClusterTLSConfig{
					Insecure: false,
					CABundle: "/etc/ssl/certs/prod-ca.crt",
				},
				Storage: sharedconfig.StorageConfig{
					Type:      "s3",
					Endpoint:  "s3.amazonaws.com",
					AccessKey: os.Getenv("S3_ACCESS_KEY"),
					SecretKey: os.Getenv("S3_SECRET_KEY"),
					Bucket:    "custom-prod-backups",
					UseSSL:    true,
					Region:    "us-west-2",
				},
			},
			{
				Name:     "custom-dev",
				Endpoint: "https://api.custom-dev.k8s.example.com:6443",
				Auth: sharedconfig.ClusterAuthConfig{
					Method: "service_account",
					ServiceAccount: sharedconfig.ServiceAccountConfig{
						TokenPath:  "/var/run/secrets/kubernetes.io/serviceaccount/token",
						CACertPath: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
					},
				},
				Storage: sharedconfig.StorageConfig{
					Type:      "minio",
					Endpoint:  "dev-minio.example.com:9000",
					AccessKey: os.Getenv("DEV_MINIO_KEY"),
					SecretKey: os.Getenv("DEV_MINIO_SECRET"),
					Bucket:    "custom-dev-backups",
					UseSSL:    false,
				},
			},
		},
		Coordination: sharedconfig.CoordinationConfig{
			Timeout:             1200, // 20 minutes
			RetryAttempts:       3,
			FailureThreshold:    1,
			HealthCheckInterval: "45s",
		},
		Scheduling: sharedconfig.SchedulingConfig{
			Strategy:              "priority",
			MaxConcurrentClusters: 2,
			ClusterPriorities: []sharedconfig.ClusterPriority{
				{Cluster: "custom-prod", Priority: 1},
				{Cluster: "custom-dev", Priority: 2},
			},
		},
	}

	fmt.Printf("✓ Created custom configuration with %d clusters\n", len(config.Clusters))

	// Validate configuration
	validator := sharedconfig.NewMultiClusterValidator()
	validationResult := validator.ValidateMultiClusterConfig(config)

	if !validationResult.Valid {
		fmt.Printf("⚠️  Configuration validation warnings:\n")
		for _, warning := range validationResult.Warnings {
			fmt.Printf("   - %s\n", warning)
		}
		if len(validationResult.Errors) > 0 {
			return fmt.Errorf("configuration validation failed: %v", validationResult.Errors)
		}
	} else {
		fmt.Println("✓ Configuration validation passed")
	}

	// Create orchestrator with custom config
	orchestrator, err := sharedconfig.NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator with custom config: %w", err)
	}
	defer orchestrator.Shutdown(context.Background())

	// Execute backup with custom configuration
	fmt.Println("🔄 Executing backup with custom configuration...")
	result, err := orchestrator.ExecuteBackup()
	if err != nil {
		return fmt.Errorf("custom configuration backup failed: %w", err)
	}

	displayBackupResults(result)

	return nil
}

// longRunningServiceExample demonstrates running the orchestrator as a long-running service
func longRunningServiceExample() error {
	fmt.Println("\n🔄 Example 4: Long-running Orchestrator Service")
	fmt.Println("----------------------------------------------")

	// This example shows how to run the orchestrator as a service
	// In a real deployment, this would be the main service loop

	config := createBasicTestConfiguration()

	orchestrator, err := sharedconfig.NewMultiClusterBackupOrchestrator(config)
	if err != nil {
		return fmt.Errorf("failed to create service orchestrator: %w", err)
	}
	defer orchestrator.Shutdown(context.Background())

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Simulate periodic backup execution
	backupTicker := time.NewTicker(30 * time.Second) // In real usage, this would be much longer
	defer backupTicker.Stop()

	fmt.Println("✓ Started long-running backup service")
	fmt.Println("   (This example will run for 2 minutes, or until interrupted)")

	executionCount := 0
	maxExecutions := 4 // Limit for demo purposes

	for {
		select {
		case <-sigChan:
			fmt.Println("\n📤 Received shutdown signal")
			cancel()
			return orchestrator.Shutdown(ctx)

		case <-ctx.Done():
			fmt.Println("📤 Context cancelled, shutting down")
			return orchestrator.Shutdown(context.Background())

		case <-backupTicker.C:
			if executionCount >= maxExecutions {
				fmt.Printf("📊 Demo completed after %d backup executions\n", executionCount)
				cancel()
				continue
			}

			executionCount++
			fmt.Printf("\n🔄 Scheduled backup execution #%d\n", executionCount)

			// Execute backup
			result, err := orchestrator.ExecuteBackup()
			if err != nil {
				log.Printf("⚠️  Scheduled backup execution #%d failed: %v", executionCount, err)
				continue
			}

			// Log results
			fmt.Printf("✅ Backup execution #%d completed: %d successful, %d failed clusters\n",
				executionCount, result.SuccessfulClusters, result.FailedClusters)

			// Get current stats
			stats := orchestrator.GetOrchestratorStats()
			fmt.Printf("📊 Service stats: %d total executions, %d successful runs\n",
				stats["total_executions"], stats["successful_runs"])
		}
	}
}

// Helper functions

func createBasicTestConfiguration() *sharedconfig.MultiClusterConfig {
	return &sharedconfig.MultiClusterConfig{
		Enabled:        true,
		Mode:           "sequential",
		DefaultCluster: "test-cluster-1",
		Clusters: []sharedconfig.MultiClusterClusterConfig{
			{
				Name:     "test-cluster-1",
				Endpoint: "https://api.test1.k8s.local:6443",
				Auth: sharedconfig.ClusterAuthConfig{
					Method: "token",
					Token: sharedconfig.TokenAuthConfig{
						Value: "test-token-1",
						Type:  "bearer",
					},
				},
				Storage: sharedconfig.StorageConfig{
					Type:      "minio",
					Endpoint:  "minio1.test.local:9000",
					AccessKey: "test-key-1",
					SecretKey: "test-secret-1",
					Bucket:    "test-backups-1",
				},
			},
			{
				Name:     "test-cluster-2",
				Endpoint: "https://api.test2.k8s.local:6443",
				Auth: sharedconfig.ClusterAuthConfig{
					Method: "token",
					Token: sharedconfig.TokenAuthConfig{
						Value: "test-token-2",
						Type:  "bearer",
					},
				},
				Storage: sharedconfig.StorageConfig{
					Type:      "minio",
					Endpoint:  "minio2.test.local:9000",
					AccessKey: "test-key-2",
					SecretKey: "test-secret-2",
					Bucket:    "test-backups-2",
				},
			},
		},
		Coordination: sharedconfig.CoordinationConfig{
			Timeout:             300,
			RetryAttempts:       2,
			FailureThreshold:    1,
			HealthCheckInterval: "30s",
		},
		Scheduling: sharedconfig.SchedulingConfig{
			Strategy:              "round_robin",
			MaxConcurrentClusters: 1,
		},
	}
}

func createAdvancedTestConfiguration() *sharedconfig.MultiClusterConfig {
	config := createBasicTestConfiguration()
	config.Mode = "parallel"
	config.Scheduling.Strategy = "priority"
	config.Scheduling.MaxConcurrentClusters = 2
	config.Scheduling.ClusterPriorities = []sharedconfig.ClusterPriority{
		{Cluster: "test-cluster-1", Priority: 1},
		{Cluster: "test-cluster-2", Priority: 2},
	}
	return config
}

func displayBackupResults(result *sharedconfig.MultiClusterBackupResult) {
	fmt.Printf("\n📊 Backup Results Summary:\n")
	fmt.Printf("   Overall Status: %s\n", result.OverallStatus)
	fmt.Printf("   Execution Mode: %s\n", result.ExecutionMode)
	fmt.Printf("   Total Duration: %v\n", result.TotalDuration)
	fmt.Printf("   Total Clusters: %d\n", result.TotalClusters)
	fmt.Printf("   Successful: %d\n", result.SuccessfulClusters)
	fmt.Printf("   Failed: %d\n", result.FailedClusters)
	fmt.Printf("   Execution Time: %v to %v\n", 
		result.StartTime.Format("15:04:05"), result.EndTime.Format("15:04:05"))

	fmt.Println("\n📋 Per-Cluster Results:")
	for clusterName, clusterResult := range result.ClusterResults {
		status := "✅"
		if clusterResult.Status != sharedconfig.BackupStatusCompleted {
			status = "❌"
		}
		
		fmt.Printf("   %s %s:\n", status, clusterName)
		fmt.Printf("      Status: %s\n", clusterResult.Status)
		fmt.Printf("      Duration: %v\n", clusterResult.Duration)
		fmt.Printf("      Namespaces: %d\n", clusterResult.NamespacesBackedUp)
		fmt.Printf("      Resources: %d\n", clusterResult.ResourcesBackedUp)
		
		if clusterResult.TotalDataSize > 0 {
			fmt.Printf("      Data Size: %.2f MB\n", float64(clusterResult.TotalDataSize)/1024/1024)
		}
		
		if len(clusterResult.Errors) > 0 {
			fmt.Printf("      Errors: %d\n", len(clusterResult.Errors))
			for _, err := range clusterResult.Errors {
				fmt.Printf("        - %s\n", err.Error())
			}
		}
		
		if len(clusterResult.Warnings) > 0 {
			fmt.Printf("      Warnings: %d\n", len(clusterResult.Warnings))
		}
	}
}

func getSystemHealthStatus(status map[string]interface{}) string {
	if systemHealth, ok := status["system_health"].(map[string]interface{}); ok {
		if overall, ok := systemHealth["Overall"].(string); ok {
			return overall
		}
	}
	return "unknown"
}

func getCircuitBreakersStatus(status map[string]interface{}) map[string]interface{} {
	if circuitBreakers, ok := status["circuit_breakers"].(map[string]interface{}); ok {
		return circuitBreakers
	}
	return make(map[string]interface{})
}

// Environment variable examples for testing
func init() {
	// Set example environment variables if not already set
	envVars := map[string]string{
		"PROD_TOKEN":        "prod-cluster-token-12345",
		"S3_ACCESS_KEY":     "AKIAIOSFODNN7EXAMPLE",
		"S3_SECRET_KEY":     "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"DEV_MINIO_KEY":     "dev-minio-access-key",
		"DEV_MINIO_SECRET":  "dev-minio-secret-key",
	}

	for key, value := range envVars {
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}