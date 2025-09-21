package examples

import (
	"context"
	"fmt"
	"log"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/http"
	"shared-config/monitoring"
	"shared-config/resilience"
	"shared-config/storage"
	"shared-config/gitops"
)

// CircuitBreakerIntegrationExample demonstrates how to integrate circuit breakers
// across the entire backup/restore system
type CircuitBreakerIntegrationExample struct {
	config                *sharedconfig.SharedConfig
	circuitBreakerManager *resilience.CircuitBreakerManager
	observer              *resilience.CircuitBreakerObserver
	monitoring            monitoring.MetricsCollector
	
	// Service clients with circuit breaker protection
	httpClientPool        *http.HTTPClientPool
	minioClient          *storage.ResilientMinIOClient
	gitClient            *gitops.ResilientGitClient
}

// NewCircuitBreakerIntegrationExample creates a new integration example
func NewCircuitBreakerIntegrationExample() (*CircuitBreakerIntegrationExample, error) {
	// Load shared configuration
	configLoader := sharedconfig.NewConfigLoader(
		"./shared-config.yaml",
		"./config/shared-config.yaml",
	)
	
	config, err := configLoader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}
	
	// Create monitoring collector (mock for example)
	monitoring := &MockMetricsCollector{
		counters:  make(map[string]float64),
		gauges:    make(map[string]float64),
		durations: make(map[string]time.Duration),
	}
	
	// Create circuit breaker manager
	circuitBreakerManager := resilience.NewCircuitBreakerManager(config, monitoring)
	
	// Create observability system
	observabilityConfig := resilience.DefaultObservabilityConfig()
	observer := resilience.NewCircuitBreakerObserver(observabilityConfig, monitoring)
	
	// Register all circuit breakers with observer
	for name, cb := range circuitBreakerManager.ListCircuitBreakers() {
		circuitBreaker := circuitBreakerManager.GetCircuitBreaker(name)
		observer.RegisterCircuitBreaker(name, circuitBreaker)
	}
	
	// Create service clients with circuit breaker protection
	httpClientPool := http.NewHTTPClientPool(config, circuitBreakerManager, monitoring)
	
	minioClient, err := storage.NewResilientMinIOClientFromSharedConfig(
		config, circuitBreakerManager, monitoring)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}
	
	gitClient := gitops.NewResilientGitClientFromSharedConfig(
		config, circuitBreakerManager, monitoring, "/tmp/gitops")
	
	return &CircuitBreakerIntegrationExample{
		config:                config,
		circuitBreakerManager: circuitBreakerManager,
		observer:              observer,
		monitoring:            monitoring,
		httpClientPool:        httpClientPool,
		minioClient:          minioClient,
		gitClient:            gitClient,
	}, nil
}

// RunExample demonstrates the circuit breaker system in action
func (example *CircuitBreakerIntegrationExample) RunExample(ctx context.Context) error {
	log.Println("Starting Circuit Breaker Integration Example")
	
	// Start monitoring
	example.observer.StartMonitoring(ctx)
	
	// Start circuit breaker manager monitoring
	go example.circuitBreakerManager.StartMonitoring(ctx, 30*time.Second)
	
	// Demonstrate normal operations
	if err := example.demonstrateNormalOperations(ctx); err != nil {
		return fmt.Errorf("normal operations demo failed: %v", err)
	}
	
	// Demonstrate failure scenarios
	if err := example.demonstrateFailureScenarios(ctx); err != nil {
		return fmt.Errorf("failure scenarios demo failed: %v", err)
	}
	
	// Demonstrate recovery scenarios
	if err := example.demonstrateRecoveryScenarios(ctx); err != nil {
		return fmt.Errorf("recovery scenarios demo failed: %v", err)
	}
	
	// Show metrics and health status
	example.showMetricsAndHealth()
	
	log.Println("Circuit Breaker Integration Example completed successfully")
	return nil
}

// demonstrateNormalOperations shows circuit breakers in normal operation
func (example *CircuitBreakerIntegrationExample) demonstrateNormalOperations(ctx context.Context) error {
	log.Println("=== Demonstrating Normal Operations ===")
	
	// HTTP operations
	log.Println("Testing HTTP operations...")
	backupClient := example.httpClientPool.GetClient("backup_tool", "backup_tool")
	
	// Simulate successful backup API calls
	for i := 0; i < 5; i++ {
		_, err := backupClient.Get(ctx, "http://backup-service/api/health")
		if err != nil && !isExpectedError(err) {
			log.Printf("HTTP operation %d failed (expected for demo): %v", i+1, err)
		} else {
			log.Printf("HTTP operation %d succeeded", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// MinIO operations
	log.Println("Testing MinIO operations...")
	
	// Test bucket operations
	bucketName := "demo-bucket"
	exists, err := example.minioClient.BucketExists(ctx, bucketName)
	if err != nil && !isExpectedError(err) {
		log.Printf("MinIO bucket check failed (expected for demo): %v", err)
	} else {
		log.Printf("MinIO bucket exists check: %v", exists)
	}
	
	// Git operations
	log.Println("Testing Git operations...")
	
	// Test Git health check
	err = example.gitClient.HealthCheck(ctx)
	if err != nil && !isExpectedError(err) {
		log.Printf("Git health check failed (expected for demo): %v", err)
	} else {
		log.Println("Git health check succeeded")
	}
	
	log.Println("Normal operations demonstration completed")
	return nil
}

// demonstrateFailureScenarios shows circuit breakers handling failures
func (example *CircuitBreakerIntegrationExample) demonstrateFailureScenarios(ctx context.Context) error {
	log.Println("=== Demonstrating Failure Scenarios ===")
	
	// Simulate MinIO failures to trigger circuit breaker
	log.Println("Simulating MinIO failures...")
	
	for i := 0; i < 6; i++ { // Exceed failure threshold
		err := example.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
			return fmt.Errorf("simulated MinIO failure %d", i+1)
		})
		
		if resilience.IsCircuitBreakerError(err) {
			log.Printf("MinIO operation %d rejected by circuit breaker", i+1)
		} else {
			log.Printf("MinIO operation %d failed: %v", i+1, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	
	// Check circuit breaker state
	minioState := example.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceMinIO).GetState()
	log.Printf("MinIO circuit breaker state: %s", minioState.String())
	
	// Simulate HTTP failures
	log.Println("Simulating HTTP failures...")
	
	httpClient := example.httpClientPool.GetClient("webhook", "webhook")
	for i := 0; i < 4; i++ { // Trigger failures but not enough to open circuit
		err := httpClient.ExecuteWithRetryAndCircuitBreaker(ctx, func() (*http.Response, error) {
			return nil, fmt.Errorf("simulated HTTP failure %d", i+1)
		})
		
		if err != nil {
			log.Printf("HTTP operation %d failed: %v", i+1, err)
		}
		time.Sleep(150 * time.Millisecond)
	}
	
	log.Println("Failure scenarios demonstration completed")
	return nil
}

// demonstrateRecoveryScenarios shows circuit breaker recovery
func (example *CircuitBreakerIntegrationExample) demonstrateRecoveryScenarios(ctx context.Context) error {
	log.Println("=== Demonstrating Recovery Scenarios ===")
	
	// Check current MinIO circuit breaker state
	minioCB := example.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceMinIO)
	currentState := minioCB.GetState()
	log.Printf("Current MinIO circuit breaker state: %s", currentState.String())
	
	if currentState == resilience.StateOpen {
		log.Println("Waiting for recovery period...")
		time.Sleep(6 * time.Second) // Wait for recovery time
		
		// Attempt recovery with successful operations
		log.Println("Attempting recovery with successful operations...")
		
		for i := 0; i < 3; i++ {
			err := example.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
				log.Printf("Successful MinIO operation %d during recovery", i+1)
				return nil // Simulate successful operation
			})
			
			if err != nil {
				log.Printf("Recovery operation %d failed: %v", i+1, err)
			} else {
				log.Printf("Recovery operation %d succeeded", i+1)
			}
			
			state := minioCB.GetState()
			log.Printf("Circuit breaker state after operation %d: %s", i+1, state.String())
			
			time.Sleep(500 * time.Millisecond)
		}
	}
	
	// Demonstrate manual reset
	log.Println("Demonstrating manual circuit breaker reset...")
	
	// Force open a circuit breaker for demonstration
	err := example.circuitBreakerManager.ForceOpenCircuitBreaker("git")
	if err != nil {
		return fmt.Errorf("failed to force open Git circuit breaker: %v", err)
	}
	
	gitState := example.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceGit).GetState()
	log.Printf("Git circuit breaker forced to: %s", gitState.String())
	
	// Reset the circuit breaker
	err = example.circuitBreakerManager.ResetCircuitBreaker("git")
	if err != nil {
		return fmt.Errorf("failed to reset Git circuit breaker: %v", err)
	}
	
	gitState = example.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceGit).GetState()
	log.Printf("Git circuit breaker after reset: %s", gitState.String())
	
	log.Println("Recovery scenarios demonstration completed")
	return nil
}

// showMetricsAndHealth displays current metrics and health status
func (example *CircuitBreakerIntegrationExample) showMetricsAndHealth() {
	log.Println("=== Current System Status ===")
	
	// Show overall health
	healthStatus := example.circuitBreakerManager.GetHealthStatus()
	log.Printf("Overall System Health: %.1f%% (%d/%d services healthy)",
		healthStatus["overall_health"].(float64),
		healthStatus["healthy_services"].(int),
		healthStatus["total_services"].(int))
	
	// Show circuit breaker states
	circuitBreakers := example.circuitBreakerManager.ListCircuitBreakers()
	log.Println("Circuit Breaker States:")
	for name, state := range circuitBreakers {
		log.Printf("  %s: %s", name, state.String())
	}
	
	// Show detailed metrics for critical services
	log.Println("Detailed Metrics:")
	allMetrics := example.circuitBreakerManager.GetAllMetrics()
	
	criticalServices := []string{"minio", "http", "git"}
	for _, service := range criticalServices {
		if metrics, exists := allMetrics[service]; exists {
			log.Printf("  %s: %d total, %d successful, %d failed, %d rejected",
				service,
				metrics.TotalRequests,
				metrics.SuccessfulReqs,
				metrics.FailedReqs,
				metrics.RejectedReqs)
		}
	}
	
	// Show recent events
	log.Println("Recent Events:")
	events := example.observer.GetEventHistory(5)
	for _, event := range events {
		log.Printf("  [%s] %s: %s",
			event.Timestamp.Format("15:04:05"),
			event.CircuitBreaker,
			event.Message)
	}
	
	// Show active alerts
	alerts := example.observer.GetAlerts()
	if len(alerts) > 0 {
		log.Println("Active Alerts:")
		for _, alert := range alerts {
			log.Printf("  [%s] %s: %s",
				alert.Severity,
				alert.CircuitBreaker,
				alert.Message)
		}
	} else {
		log.Println("No active alerts")
	}
}

// BackupWorkflowExample demonstrates circuit breakers in a real backup workflow
func (example *CircuitBreakerIntegrationExample) BackupWorkflowExample(ctx context.Context) error {
	log.Println("=== Backup Workflow with Circuit Breaker Protection ===")
	
	// Step 1: Check MinIO connectivity
	log.Println("Step 1: Checking MinIO connectivity...")
	err := example.minioClient.HealthCheck(ctx)
	if err != nil {
		if resilience.IsCircuitBreakerError(err) {
			log.Printf("MinIO circuit breaker is open, skipping backup: %v", err)
			return nil // Graceful degradation
		}
		return fmt.Errorf("MinIO health check failed: %v", err)
	}
	log.Println("MinIO is healthy")
	
	// Step 2: Fetch backup metadata from backup service
	log.Println("Step 2: Fetching backup metadata...")
	backupClient := example.httpClientPool.GetClient("backup_service", "backup_tool")
	
	_, err = backupClient.Get(ctx, "http://backup-service/api/backups/latest")
	if err != nil {
		if resilience.IsCircuitBreakerError(err) {
			log.Printf("Backup service circuit breaker is open, using cached metadata: %v", err)
			// Continue with cached or default metadata
		} else {
			log.Printf("Failed to fetch backup metadata: %v", err)
		}
	} else {
		log.Println("Backup metadata fetched successfully")
	}
	
	// Step 3: Clone/update GitOps repository
	log.Println("Step 3: Updating GitOps repository...")
	repoPath := "/tmp/demo-gitops"
	
	_, err = example.gitClient.EnsureRepository(ctx, 
		"https://github.com/example/gitops-repo.git", 
		repoPath, 
		"main")
	if err != nil {
		if resilience.IsCircuitBreakerError(err) {
			log.Printf("Git circuit breaker is open, skipping GitOps update: %v", err)
			// Continue without GitOps update
		} else {
			log.Printf("Failed to update GitOps repository: %v", err)
		}
	} else {
		log.Println("GitOps repository updated successfully")
	}
	
	// Step 4: Perform backup upload to MinIO
	log.Println("Step 4: Uploading backup to MinIO...")
	err = example.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		// Simulate backup upload
		log.Println("Uploading backup data to MinIO...")
		time.Sleep(500 * time.Millisecond) // Simulate upload time
		return nil
	})
	
	if err != nil {
		if resilience.IsCircuitBreakerError(err) {
			log.Printf("MinIO circuit breaker rejected upload: %v", err)
			return fmt.Errorf("backup upload failed due to circuit breaker")
		}
		return fmt.Errorf("backup upload failed: %v", err)
	}
	log.Println("Backup uploaded successfully")
	
	// Step 5: Update GitOps with backup completion
	log.Println("Step 5: Updating GitOps with backup completion...")
	err = example.gitClient.CommitAndPush(ctx, repoPath, 
		"Update backup status: completed at "+time.Now().Format(time.RFC3339), 
		"main")
	
	if err != nil {
		if resilience.IsCircuitBreakerError(err) {
			log.Printf("Git circuit breaker prevented GitOps update: %v", err)
			// Continue - backup is complete even without GitOps update
		} else {
			log.Printf("Failed to update GitOps: %v", err)
		}
	} else {
		log.Println("GitOps updated with backup completion")
	}
	
	log.Println("Backup workflow completed with circuit breaker protection")
	return nil
}

// Mock implementation for demonstration
type MockMetricsCollector struct {
	counters  map[string]float64
	gauges    map[string]float64
	durations map[string]time.Duration
}

func (m *MockMetricsCollector) IncCounter(name string, labels map[string]string, value float64) {
	// Implementation for demo
}

func (m *MockMetricsCollector) SetGauge(name string, labels map[string]string, value float64) {
	// Implementation for demo
}

func (m *MockMetricsCollector) RecordDuration(name string, labels map[string]string, duration time.Duration) {
	// Implementation for demo
}

// Helper function to determine if an error is expected in demo
func isExpectedError(err error) bool {
	// In a real implementation, this would check for specific error types
	// that are expected in the demo environment
	return false
}

// RunIntegrationExample runs the complete integration example
func RunIntegrationExample() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	example, err := NewCircuitBreakerIntegrationExample()
	if err != nil {
		return fmt.Errorf("failed to create integration example: %v", err)
	}
	
	// Run the main example
	if err := example.RunExample(ctx); err != nil {
		return err
	}
	
	// Run the backup workflow example
	if err := example.BackupWorkflowExample(ctx); err != nil {
		return err
	}
	
	return nil
}