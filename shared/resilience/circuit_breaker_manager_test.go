package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	sharedconfig "shared-config/config"
)

func TestCircuitBreakerManagerInitialization(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := &sharedconfig.SharedConfig{
		Retries: sharedconfig.RetryConfig{
			CircuitBreakerThreshold:    5,
			CircuitBreakerTimeout:      60 * time.Second,
			CircuitBreakerRecoveryTime: 300 * time.Second,
		},
	}
	
	manager := NewCircuitBreakerManager(config, monitoring)
	
	// Test that service circuit breakers are initialized
	services := []ServiceType{
		ServiceMinIO,
		ServiceHTTP,
		ServiceGit,
		ServiceKubernetes,
		ServiceSecurity,
		ServiceBackup,
		ServiceGitOps,
		ServiceWebhook,
	}
	
	for _, service := range services {
		cb := manager.GetServiceCircuitBreaker(service)
		if cb == nil {
			t.Errorf("Expected circuit breaker for service %s to be initialized", service)
		}
		
		if cb.GetState() != StateClosed {
			t.Errorf("Expected initial state to be CLOSED for service %s, got %v", service, cb.GetState())
		}
	}
}

func TestCircuitBreakerManagerServiceOperations(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	// Test successful operation
	err := manager.WrapMinIOOperation(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful MinIO operation, got error: %v", err)
	}
	
	// Test failed operation
	err = manager.WrapHTTPOperation(ctx, func() error {
		return errors.New("HTTP failure")
	})
	if err == nil {
		t.Errorf("Expected HTTP operation to fail")
	}
	
	// Test Git operation
	err = manager.WrapGitOperation(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful Git operation, got error: %v", err)
	}
}

func TestCircuitBreakerManagerFailureScenarios(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := &sharedconfig.SharedConfig{
		Retries: sharedconfig.RetryConfig{
			CircuitBreakerThreshold:    3,
			CircuitBreakerTimeout:      50 * time.Millisecond,
			CircuitBreakerRecoveryTime: 100 * time.Millisecond,
		},
	}
	
	manager := NewCircuitBreakerManager(config, monitoring)
	ctx := context.Background()
	
	// Trigger circuit breaker opening for MinIO service
	for i := 0; i < 3; i++ {
		err := manager.WrapMinIOOperation(ctx, func() error {
			return errors.New("MinIO failure")
		})
		if err == nil {
			t.Errorf("Expected MinIO operation %d to fail", i+1)
		}
	}
	
	// Verify circuit breaker is open
	cb := manager.GetServiceCircuitBreaker(ServiceMinIO)
	if cb.GetState() != StateOpen {
		t.Errorf("Expected MinIO circuit breaker to be OPEN, got %v", cb.GetState())
	}
	
	// Test that requests are now rejected
	err := manager.WrapMinIOOperation(ctx, func() error {
		return nil
	})
	if !IsCircuitBreakerError(err) {
		t.Errorf("Expected circuit breaker error for MinIO, got: %v", err)
	}
	
	// Other services should still work
	err = manager.WrapHTTPOperation(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected HTTP operation to succeed when MinIO circuit is open, got: %v", err)
	}
}

func TestCircuitBreakerManagerRecovery(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := &sharedconfig.SharedConfig{
		Retries: sharedconfig.RetryConfig{
			CircuitBreakerThreshold:    2,
			CircuitBreakerTimeout:      50 * time.Millisecond,
			CircuitBreakerRecoveryTime: 100 * time.Millisecond,
		},
	}
	
	manager := NewCircuitBreakerManager(config, monitoring)
	ctx := context.Background()
	
	// Trigger circuit breaker opening
	for i := 0; i < 2; i++ {
		manager.WrapGitOperation(ctx, func() error {
			return errors.New("Git failure")
		})
	}
	
	cb := manager.GetServiceCircuitBreaker(ServiceGit)
	if cb.GetState() != StateOpen {
		t.Errorf("Expected Git circuit breaker to be OPEN, got %v", cb.GetState())
	}
	
	// Wait for recovery time
	time.Sleep(150 * time.Millisecond)
	
	// Should allow operation now (transitions to HALF_OPEN then CLOSED)
	err := manager.WrapGitOperation(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful Git operation after recovery, got: %v", err)
	}
	
	if cb.GetState() != StateClosed {
		t.Errorf("Expected Git circuit breaker to be CLOSED after successful operation, got %v", cb.GetState())
	}
}

func TestCircuitBreakerManagerMetrics(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	// Perform various operations
	manager.WrapMinIOOperation(ctx, func() error { return nil })
	manager.WrapHTTPOperation(ctx, func() error { return errors.New("failure") })
	manager.WrapGitOperation(ctx, func() error { return nil })
	
	// Get all metrics
	allMetrics := manager.GetAllMetrics()
	
	if len(allMetrics) == 0 {
		t.Error("Expected circuit breaker metrics to be available")
	}
	
	// Check MinIO metrics
	if minioMetrics, exists := allMetrics["minio"]; exists {
		if minioMetrics.SuccessfulReqs != 1 {
			t.Errorf("Expected 1 successful MinIO request, got %d", minioMetrics.SuccessfulReqs)
		}
	} else {
		t.Error("Expected MinIO metrics to be available")
	}
	
	// Check HTTP metrics
	if httpMetrics, exists := allMetrics["http"]; exists {
		if httpMetrics.FailedReqs != 1 {
			t.Errorf("Expected 1 failed HTTP request, got %d", httpMetrics.FailedReqs)
		}
	} else {
		t.Error("Expected HTTP metrics to be available")
	}
}

func TestCircuitBreakerManagerHealthStatus(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := &sharedconfig.SharedConfig{
		Retries: sharedconfig.RetryConfig{
			CircuitBreakerThreshold: 2,
		},
	}
	
	manager := NewCircuitBreakerManager(config, monitoring)
	ctx := context.Background()
	
	// Initially all should be healthy
	health := manager.GetHealthStatus()
	if health["overall_health"].(float64) != 100.0 {
		t.Errorf("Expected 100%% health initially, got %v", health["overall_health"])
	}
	
	// Trigger failure for one service
	for i := 0; i < 2; i++ {
		manager.WrapMinIOOperation(ctx, func() error {
			return errors.New("MinIO failure")
		})
	}
	
	// Health should be degraded
	health = manager.GetHealthStatus()
	overallHealth := health["overall_health"].(float64)
	if overallHealth == 100.0 {
		t.Error("Expected health to be degraded after circuit breaker opens")
	}
	
	// Check service-specific health
	services := health["services"].(map[string]interface{})
	if minioHealth, exists := services["minio"]; exists {
		serviceHealth := minioHealth.(map[string]interface{})
		if serviceHealth["healthy"].(bool) {
			t.Error("Expected MinIO service to be unhealthy")
		}
	} else {
		t.Error("Expected MinIO service health to be reported")
	}
}

func TestCircuitBreakerManagerListCircuitBreakers(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	
	circuitBreakers := manager.ListCircuitBreakers()
	
	expectedServices := []string{"minio", "http", "git", "kubernetes", "security", "backup", "gitops", "webhook"}
	
	if len(circuitBreakers) != len(expectedServices) {
		t.Errorf("Expected %d circuit breakers, got %d", len(expectedServices), len(circuitBreakers))
	}
	
	for _, service := range expectedServices {
		if state, exists := circuitBreakers[service]; exists {
			if state != StateClosed {
				t.Errorf("Expected %s circuit breaker to be CLOSED initially, got %v", service, state)
			}
		} else {
			t.Errorf("Expected %s circuit breaker to be listed", service)
		}
	}
}

func TestCircuitBreakerManagerReset(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := &sharedconfig.SharedConfig{
		Retries: sharedconfig.RetryConfig{
			CircuitBreakerThreshold: 1,
		},
	}
	
	manager := NewCircuitBreakerManager(config, monitoring)
	ctx := context.Background()
	
	// Trigger circuit breaker opening
	manager.WrapMinIOOperation(ctx, func() error {
		return errors.New("MinIO failure")
	})
	
	cb := manager.GetServiceCircuitBreaker(ServiceMinIO)
	if cb.GetState() != StateOpen {
		t.Errorf("Expected MinIO circuit breaker to be OPEN, got %v", cb.GetState())
	}
	
	// Reset specific circuit breaker
	err := manager.ResetCircuitBreaker("minio")
	if err != nil {
		t.Errorf("Expected successful reset, got error: %v", err)
	}
	
	if cb.GetState() != StateClosed {
		t.Errorf("Expected MinIO circuit breaker to be CLOSED after reset, got %v", cb.GetState())
	}
	
	// Test reset all
	manager.WrapHTTPOperation(ctx, func() error {
		return errors.New("HTTP failure")
	})
	
	manager.ResetAllCircuitBreakers()
	
	httpCb := manager.GetServiceCircuitBreaker(ServiceHTTP)
	if httpCb.GetState() != StateClosed {
		t.Errorf("Expected HTTP circuit breaker to be CLOSED after reset all, got %v", httpCb.GetState())
	}
}

func TestCircuitBreakerManagerForceOpen(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	// Force open a circuit breaker
	err := manager.ForceOpenCircuitBreaker("git")
	if err != nil {
		t.Errorf("Expected successful force open, got error: %v", err)
	}
	
	cb := manager.GetServiceCircuitBreaker(ServiceGit)
	if cb.GetState() != StateOpen {
		t.Errorf("Expected Git circuit breaker to be OPEN after force open, got %v", cb.GetState())
	}
	
	// Should reject operations
	err = manager.WrapGitOperation(ctx, func() error {
		return nil
	})
	if !IsCircuitBreakerError(err) {
		t.Errorf("Expected circuit breaker error after force open, got: %v", err)
	}
}

func TestCircuitBreakerManagerCustomService(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	// Create custom circuit breaker
	customConfig := DefaultCircuitBreakerConfig("custom-service")
	customConfig.FailureThreshold = 2
	
	cb := manager.CreateServiceCircuitBreaker("custom-service", customConfig)
	if cb == nil {
		t.Error("Expected custom circuit breaker to be created")
	}
	
	// Test custom service operation
	err := manager.ExecuteWithCircuitBreaker(ctx, "custom-service", func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful custom service operation, got error: %v", err)
	}
	
	// Verify it's in the list
	circuitBreakers := manager.ListCircuitBreakers()
	if _, exists := circuitBreakers["custom-service"]; !exists {
		t.Error("Expected custom service to be in circuit breaker list")
	}
}

func TestCircuitBreakerManagerMonitoring(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	// Start monitoring in the background
	monitoringCtx, cancel := context.WithCancel(ctx)
	go manager.StartMonitoring(monitoringCtx, 10*time.Millisecond)
	defer cancel()
	
	// Perform some operations
	manager.WrapMinIOOperation(ctx, func() error { return nil })
	manager.WrapHTTPOperation(ctx, func() error { return errors.New("failure") })
	
	// Wait for monitoring to collect metrics
	time.Sleep(50 * time.Millisecond)
	
	// Check that aggregate metrics were recorded
	healthGauge := monitoring.GetGauge("circuit_breaker_health_percentage:manager=circuit_breaker")
	if healthGauge == 0 {
		t.Error("Expected health percentage metric to be recorded")
	}
	
	closedCount := monitoring.GetGauge("circuit_breaker_closed_count:manager=circuit_breaker")
	if closedCount == 0 {
		t.Error("Expected closed count metric to be recorded")
	}
}

func TestCircuitBreakerManagerErrorHandling(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	
	// Test non-existent circuit breaker reset
	err := manager.ResetCircuitBreaker("non-existent")
	if err == nil {
		t.Error("Expected error when resetting non-existent circuit breaker")
	}
	
	// Test non-existent circuit breaker force open
	err = manager.ForceOpenCircuitBreaker("non-existent")
	if err == nil {
		t.Error("Expected error when force opening non-existent circuit breaker")
	}
}

// Benchmark tests for circuit breaker manager

func BenchmarkCircuitBreakerManagerSuccessfulOperations(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			manager.WrapMinIOOperation(ctx, func() error {
				return nil
			})
		}
	})
}

func BenchmarkCircuitBreakerManagerMixedServices(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	ctx := context.Background()
	
	operations := []func() error{
		func() error { return manager.WrapMinIOOperation(ctx, func() error { return nil }) },
		func() error { return manager.WrapHTTPOperation(ctx, func() error { return nil }) },
		func() error { return manager.WrapGitOperation(ctx, func() error { return nil }) },
		func() error { return manager.WrapKubernetesOperation(ctx, func() error { return nil }) },
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			operations[i%len(operations)]()
			i++
		}
	})
}

func BenchmarkCircuitBreakerManagerGetMetrics(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetAllMetrics()
	}
}

func BenchmarkCircuitBreakerManagerHealthStatus(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	manager := NewCircuitBreakerManager(nil, monitoring)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetHealthStatus()
	}
}