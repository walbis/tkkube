package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shared-config/integration"
	"shared-config/monitoring"
)

// CompleteIntegrationExample demonstrates the full integration bridge usage
func main() {
	fmt.Println("🚀 Starting Complete Integration Bridge Example")
	fmt.Println("=" * 60)

	// 1. Load Configuration
	fmt.Println("📋 Step 1: Loading Configuration")
	configManager := integration.NewConfigManager("../config_integration.yaml")
	config, err := configManager.LoadConfig()
	if err != nil {
		log.Printf("⚠️  Failed to load config file, using defaults: %v", err)
		config = createDefaultConfig()
	}

	fmt.Printf("✅ Configuration loaded: %s\n", config.Description)
	fmt.Printf("   - Storage: %s\n", config.Storage.Endpoint)
	fmt.Printf("   - Cluster: %s\n", config.Cluster.Name)
	fmt.Printf("   - GitOps: %s\n", config.GitOps.Repository)
	fmt.Printf("   - Bridge Port: %d\n", config.Integration.WebhookPort)

	// 2. Create Integration Bridge
	fmt.Println("\n🌉 Step 2: Creating Integration Bridge")
	bridge, err := integration.NewIntegrationBridge(config)
	if err != nil {
		log.Fatalf("❌ Failed to create integration bridge: %v", err)
	}

	fmt.Println("✅ Integration bridge created successfully")

	// 3. Start the Bridge
	fmt.Println("\n🚀 Step 3: Starting Integration Bridge")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := bridge.Start(ctx); err != nil {
		log.Fatalf("❌ Failed to start bridge: %v", err)
	}

	fmt.Printf("✅ Bridge started on port %d\n", config.Integration.WebhookPort)
	defer func() {
		fmt.Println("\n🛑 Stopping Integration Bridge")
		bridge.Stop()
		fmt.Println("✅ Bridge stopped successfully")
	}()

	// 4. Register Components
	fmt.Println("\n📝 Step 4: Registering Components")
	
	// Register backup tool
	err = bridge.RegisterBackupTool("http://backup-tool:8080", "1.0.0")
	if err != nil {
		log.Printf("⚠️  Failed to register backup tool: %v", err)
	} else {
		fmt.Println("✅ Backup tool registered")
	}

	// Register GitOps generator
	err = bridge.RegisterGitOpsTool("http://gitops-generator:8081", "2.1.0")
	if err != nil {
		log.Printf("⚠️  Failed to register GitOps generator: %v", err)
	} else {
		fmt.Println("✅ GitOps generator registered")
	}

	// 5. Display Component Status
	fmt.Println("\n📊 Step 5: Component Status")
	displayComponentStatus(bridge)

	// 6. Simulate Backup Completion Events
	fmt.Println("\n💾 Step 6: Simulating Backup Operations")
	go simulateBackupOperations(ctx, bridge)

	// 7. Monitor System Health
	fmt.Println("\n❤️  Step 7: Monitoring System Health")
	go monitorSystemHealth(ctx, bridge)

	// 8. Display Metrics
	fmt.Println("\n📈 Step 8: Monitoring Metrics")
	go displayMetricsPeriodically(ctx, bridge)

	// 9. Wait for shutdown signal
	fmt.Println("\n⏳ Integration bridge is running...")
	fmt.Println("   📡 Webhook endpoints available:")
	fmt.Printf("   - Health: http://localhost:%d/health\n", config.Integration.WebhookPort)
	fmt.Printf("   - Status: http://localhost:%d/status\n", config.Integration.WebhookPort)
	fmt.Printf("   - Backup webhook: http://localhost:%d/webhooks/backup/completed\n", config.Integration.WebhookPort)
	fmt.Println("\n💡 Try the following commands:")
	fmt.Printf("   curl http://localhost:%d/health\n", config.Integration.WebhookPort)
	fmt.Printf("   curl http://localhost:%d/status\n", config.Integration.WebhookPort)
	fmt.Println("\n🛑 Press Ctrl+C to stop")

	// Wait for interrupt signal
	waitForShutdown()

	fmt.Println("\n🎉 Complete Integration Example finished successfully!")
}

// createDefaultConfig creates a default configuration for demo purposes
func createDefaultConfig() *sharedconfig.SharedConfig {
	return &sharedconfig.SharedConfig{
		SchemaVersion: "2.1.0",
		Description:   "Integration Bridge Demo Configuration",
		Storage: sharedconfig.StorageConfig{
			Type:      "minio",
			Endpoint:  "localhost:9000",
			AccessKey: "demo-access-key",
			SecretKey: "demo-secret-key",
			Bucket:    "demo-backups",
			UseSSL:    false,
		},
		Cluster: sharedconfig.ClusterConfig{
			Name:   "demo-cluster",
			Domain: "cluster.local",
		},
		GitOps: sharedconfig.GitOpsConfig{
			Repository: "https://github.com/demo/gitops-repo",
			Branch:     "main",
			Path:       "clusters/demo",
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
					BackupTool:        "http://backup-tool:8080",
					GitOpsGenerator:   "http://gitops-generator:8081",
					IntegrationBridge: "http://localhost:8080",
				},
			},
			Triggers: sharedconfig.TriggerIntegrationConfig{
				AutoTrigger:      true,
				DelayAfterBackup: 30 * time.Second,
			},
		},
	}
}

// displayComponentStatus shows the current status of all components
func displayComponentStatus(bridge *integration.IntegrationBridge) {
	status := bridge.GetComponentStatus()

	fmt.Printf("   Total Components: %d\n", len(status))
	for name, componentStatus := range status {
		if componentStatus.Name != "" {
			icon := getStatusIcon(componentStatus.Status)
			fmt.Printf("   %s %s: %s (v%s)\n", icon, name, componentStatus.Status, componentStatus.Version)
			if endpoint, ok := componentStatus.Metadata["endpoint"].(string); ok {
				fmt.Printf("     └─ Endpoint: %s\n", endpoint)
			}
		}
	}
}

// getStatusIcon returns an emoji icon for the component status
func getStatusIcon(status string) string {
	switch status {
	case "healthy":
		return "✅"
	case "degraded":
		return "⚠️"
	case "unhealthy":
		return "❌"
	default:
		return "❓"
	}
}

// simulateBackupOperations simulates backup completion events
func simulateBackupOperations(ctx context.Context, bridge *integration.IntegrationBridge) {
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	backupCounter := 1

	// Simulate first backup immediately
	simulateBackup(ctx, bridge, backupCounter)
	backupCounter++

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			simulateBackup(ctx, bridge, backupCounter)
			backupCounter++
		}
	}
}

// simulateBackup simulates a single backup completion event
func simulateBackup(ctx context.Context, bridge *integration.IntegrationBridge, counter int) {
	// Randomly simulate success/failure (90% success rate)
	success := (counter%10) != 0
	
	backupEvent := &integration.BackupCompletionEvent{
		BackupID:      fmt.Sprintf("demo-backup-%03d", counter),
		ClusterName:   "demo-cluster",
		Timestamp:     time.Now(),
		ResourceCount: 25 + (counter * 3), // Simulate growing cluster
		Size:          int64(1024 * 1024 * (5 + counter)), // ~5MB+ per backup
		Success:       success,
		MinIOPath:     fmt.Sprintf("demo-cluster/2024/01/%02d/backup-%03d", counter%30+1, counter),
	}

	if !success {
		backupEvent.ErrorMessage = "Simulated backup failure for demo"
		backupEvent.ResourceCount = 0
		backupEvent.Size = 0
		backupEvent.MinIOPath = ""
	}

	fmt.Printf("\n💾 Simulated Backup Event #%d:\n", counter)
	fmt.Printf("   🆔 Backup ID: %s\n", backupEvent.BackupID)
	fmt.Printf("   🎯 Cluster: %s\n", backupEvent.ClusterName)
	fmt.Printf("   📊 Resources: %d\n", backupEvent.ResourceCount)
	fmt.Printf("   📏 Size: %.1f MB\n", float64(backupEvent.Size)/(1024*1024))
	
	if success {
		fmt.Printf("   ✅ Status: Success\n")
		fmt.Printf("   📁 Path: %s\n", backupEvent.MinIOPath)
	} else {
		fmt.Printf("   ❌ Status: Failed (%s)\n", backupEvent.ErrorMessage)
	}

	// Trigger GitOps generation
	err := bridge.TriggerGitOpsGeneration(ctx, backupEvent)
	if err != nil {
		fmt.Printf("   ⚠️  Failed to trigger GitOps: %v\n", err)
	} else if success {
		fmt.Printf("   🚀 GitOps generation triggered\n")
	} else {
		fmt.Printf("   ⏭️  GitOps generation skipped (backup failed)\n")
	}
}

// monitorSystemHealth monitors and displays system health
func monitorSystemHealth(ctx context.Context, bridge *integration.IntegrationBridge) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Check health immediately
	checkSystemHealth(bridge)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkSystemHealth(bridge)
		}
	}
}

// checkSystemHealth checks and displays current system health
func checkSystemHealth(bridge *integration.IntegrationBridge) {
	health := bridge.GetOverallHealth()
	
	fmt.Printf("\n❤️  System Health Check:\n")
	fmt.Printf("   Overall Status: %s %s\n", getStatusIcon(health.OverallStatus), health.OverallStatus)
	fmt.Printf("   Components: %d healthy, %d degraded, %d unhealthy (total: %d)\n", 
		health.HealthyComponents, 
		health.DegradedComponents, 
		health.UnhealthyComponents, 
		health.TotalComponents)

	if health.UnhealthyComponents > 0 {
		fmt.Printf("   ⚠️  Warning: %d components are unhealthy\n", health.UnhealthyComponents)
	}
}

// displayMetricsPeriodically displays integration metrics
func displayMetricsPeriodically(ctx context.Context, bridge *integration.IntegrationBridge) {
	ticker := time.NewTicker(90 * time.Second)
	defer ticker.Stop()

	// Display metrics immediately after a delay
	time.Sleep(30 * time.Second)
	displayMetrics(bridge)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			displayMetrics(bridge)
		}
	}
}

// displayMetrics shows current integration metrics
func displayMetrics(bridge *integration.IntegrationBridge) {
	metrics := bridge.GetIntegratedMetrics()
	
	fmt.Printf("\n📈 Integration Metrics:\n")
	fmt.Printf("   Total Requests: %d\n", metrics.TotalRequests)
	fmt.Printf("   Successful: %d\n", metrics.TotalSuccesses)
	fmt.Printf("   Failed: %d\n", metrics.TotalErrors)
	
	if metrics.TotalRequests > 0 {
		successRate := float64(metrics.TotalSuccesses) / float64(metrics.TotalRequests) * 100
		fmt.Printf("   Success Rate: %.1f%%\n", successRate)
	}
	
	fmt.Printf("   Average Latency: %v\n", metrics.AverageLatency)

	// Integration flow metrics
	flow := metrics.IntegrationFlow
	fmt.Printf("\n🔄 Integration Flow:\n")
	fmt.Printf("   Total Integrations: %d\n", flow.TotalIntegrationRequests)
	fmt.Printf("   Successful: %d\n", flow.SuccessfulIntegrations)
	fmt.Printf("   Failed: %d\n", flow.FailedIntegrations)
	fmt.Printf("   Avg Flow Duration: %v\n", flow.AverageFlowDuration)
	fmt.Printf("   Backup→GitOps Latency: %v\n", flow.BackupToGitOpsLatency)

	// Component breakdown
	if len(metrics.ComponentBreakdown) > 0 {
		fmt.Printf("\n🔧 Component Breakdown:\n")
		for componentName, componentMetrics := range metrics.ComponentBreakdown {
			if componentMetrics.ComponentName != "" {
				fmt.Printf("   %s:\n", componentName)
				fmt.Printf("     Requests: %d\n", componentMetrics.RequestCount)
				fmt.Printf("     Errors: %d\n", componentMetrics.ErrorCount)
				fmt.Printf("     Avg Latency: %v\n", componentMetrics.AverageLatency)
				fmt.Printf("     Status: %s %s\n", getStatusIcon(componentMetrics.HealthStatus), componentMetrics.HealthStatus)
			}
		}
	}
}

// waitForShutdown waits for interrupt signal
func waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\n🛑 Shutdown signal received")
}

// Helper function to repeat strings (for visual formatting)
func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}