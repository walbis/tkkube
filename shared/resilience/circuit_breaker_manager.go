package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
)

// ServiceType represents different external services
type ServiceType string

const (
	ServiceMinIO      ServiceType = "minio"
	ServiceHTTP       ServiceType = "http"
	ServiceGit        ServiceType = "git"
	ServiceKubernetes ServiceType = "kubernetes"
	ServiceSecurity   ServiceType = "security"
	ServiceBackup     ServiceType = "backup"
	ServiceGitOps     ServiceType = "gitops"
	ServiceWebhook    ServiceType = "webhook"
)

// CircuitBreakerManager manages multiple circuit breakers for different services
type CircuitBreakerManager struct {
	circuitBreakers map[string]*CircuitBreaker
	config          *sharedconfig.SharedConfig
	monitoring      monitoring.MetricsCollector
	mu              sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(config *sharedconfig.SharedConfig, monitoring monitoring.MetricsCollector) *CircuitBreakerManager {
	manager := &CircuitBreakerManager{
		circuitBreakers: make(map[string]*CircuitBreaker),
		config:          config,
		monitoring:      monitoring,
	}
	
	// Initialize circuit breakers for different services
	manager.initializeServiceCircuitBreakers()
	
	return manager
}

// GetCircuitBreaker returns a circuit breaker for a specific service
func (m *CircuitBreakerManager) GetCircuitBreaker(serviceName string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if cb, exists := m.circuitBreakers[serviceName]; exists {
		return cb
	}
	
	// Create a default circuit breaker if not found
	return m.createCircuitBreaker(serviceName, DefaultCircuitBreakerConfig(serviceName))
}

// GetServiceCircuitBreaker returns a circuit breaker for a service type
func (m *CircuitBreakerManager) GetServiceCircuitBreaker(serviceType ServiceType) *CircuitBreaker {
	return m.GetCircuitBreaker(string(serviceType))
}

// ExecuteWithCircuitBreaker executes an operation with appropriate circuit breaker
func (m *CircuitBreakerManager) ExecuteWithCircuitBreaker(ctx context.Context, serviceName string, operation func() error) error {
	cb := m.GetCircuitBreaker(serviceName)
	return cb.Execute(ctx, operation)
}

// ExecuteWithServiceCircuitBreaker executes an operation with service-specific circuit breaker
func (m *CircuitBreakerManager) ExecuteWithServiceCircuitBreaker(ctx context.Context, serviceType ServiceType, operation func() error) error {
	return m.ExecuteWithCircuitBreaker(ctx, string(serviceType), operation)
}

// CreateServiceCircuitBreaker creates a circuit breaker for a specific service with custom config
func (m *CircuitBreakerManager) CreateServiceCircuitBreaker(serviceName string, config *CircuitBreakerConfig) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if config == nil {
		config = m.getServiceConfig(serviceName)
	}
	
	config.Name = serviceName
	cb := m.createCircuitBreaker(serviceName, config)
	m.circuitBreakers[serviceName] = cb
	
	return cb
}

// ListCircuitBreakers returns all circuit breaker names and their states
func (m *CircuitBreakerManager) ListCircuitBreakers() map[string]CircuitBreakerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]CircuitBreakerState)
	for name, cb := range m.circuitBreakers {
		result[name] = cb.GetState()
	}
	
	return result
}

// GetAllMetrics returns metrics for all circuit breakers
func (m *CircuitBreakerManager) GetAllMetrics() map[string]CircuitBreakerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]CircuitBreakerMetrics)
	for name, cb := range m.circuitBreakers {
		result[name] = cb.GetMetrics()
	}
	
	return result
}

// ResetCircuitBreaker resets a specific circuit breaker
func (m *CircuitBreakerManager) ResetCircuitBreaker(serviceName string) error {
	m.mu.RLock()
	cb, exists := m.circuitBreakers[serviceName]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("circuit breaker not found: %s", serviceName)
	}
	
	cb.Reset()
	return nil
}

// ResetAllCircuitBreakers resets all circuit breakers
func (m *CircuitBreakerManager) ResetAllCircuitBreakers() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, cb := range m.circuitBreakers {
		cb.Reset()
	}
}

// ForceOpenCircuitBreaker forces a circuit breaker to open state
func (m *CircuitBreakerManager) ForceOpenCircuitBreaker(serviceName string) error {
	m.mu.RLock()
	cb, exists := m.circuitBreakers[serviceName]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("circuit breaker not found: %s", serviceName)
	}
	
	cb.ForceOpen()
	return nil
}

// GetHealthStatus returns health status of all circuit breakers
func (m *CircuitBreakerManager) GetHealthStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	status := make(map[string]interface{})
	healthyCount := 0
	totalCount := len(m.circuitBreakers)
	
	services := make(map[string]interface{})
	for name, cb := range m.circuitBreakers {
		metrics := cb.GetMetrics()
		state := cb.GetState()
		
		services[name] = map[string]interface{}{
			"state":           state.String(),
			"healthy":         state != StateOpen,
			"total_requests":  metrics.TotalRequests,
			"success_rate":    m.calculateSuccessRate(metrics),
			"last_failure":    metrics.LastFailureTime,
			"failure_streak":  metrics.FailureStreak,
		}
		
		if state != StateOpen {
			healthyCount++
		}
	}
	
	status["overall_health"] = float64(healthyCount) / float64(totalCount) * 100
	status["healthy_services"] = healthyCount
	status["total_services"] = totalCount
	status["services"] = services
	status["timestamp"] = time.Now()
	
	return status
}

// StartMonitoring starts background monitoring of circuit breakers
func (m *CircuitBreakerManager) StartMonitoring(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.recordAggregateMetrics()
		}
	}
}

// Private methods

func (m *CircuitBreakerManager) initializeServiceCircuitBreakers() {
	// Create circuit breakers for known services
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
		serviceName := string(service)
		config := m.getServiceConfig(serviceName)
		config.Name = serviceName
		m.circuitBreakers[serviceName] = m.createCircuitBreaker(serviceName, config)
	}
}

func (m *CircuitBreakerManager) getServiceConfig(serviceName string) *CircuitBreakerConfig {
	// Get configuration from shared config if available
	if m.config != nil && m.config.Retries.CircuitBreakerThreshold > 0 {
		return ConfigFromShared(serviceName, &m.config.Retries)
	}
	
	// Return service-specific defaults
	config := DefaultCircuitBreakerConfig(serviceName)
	
	// Customize based on service type
	switch ServiceType(serviceName) {
	case ServiceMinIO:
		// MinIO operations might need more tolerance
		config.FailureThreshold = 10
		config.Timeout = 2 * time.Minute
		config.RecoveryTime = 5 * time.Minute
	case ServiceHTTP:
		// HTTP operations are typically fast
		config.FailureThreshold = 5
		config.Timeout = 30 * time.Second
		config.RecoveryTime = 2 * time.Minute
	case ServiceGit:
		// Git operations can be slow
		config.FailureThreshold = 3
		config.Timeout = 5 * time.Minute
		config.RecoveryTime = 10 * time.Minute
	case ServiceKubernetes:
		// K8s API should be reliable
		config.FailureThreshold = 8
		config.Timeout = 1 * time.Minute
		config.RecoveryTime = 3 * time.Minute
	case ServiceSecurity:
		// Security operations are critical
		config.FailureThreshold = 3
		config.Timeout = 30 * time.Second
		config.RecoveryTime = 1 * time.Minute
	case ServiceWebhook:
		// Webhooks can be unreliable
		config.FailureThreshold = 5
		config.Timeout = 1 * time.Minute
		config.RecoveryTime = 2 * time.Minute
	}
	
	return config
}

func (m *CircuitBreakerManager) createCircuitBreaker(serviceName string, config *CircuitBreakerConfig) *CircuitBreaker {
	// Add state change logging
	config.OnStateChange = func(name string, from, to CircuitBreakerState) {
		if m.monitoring != nil {
			labels := map[string]string{
				"service":    name,
				"from_state": from.String(),
				"to_state":   to.String(),
			}
			m.monitoring.IncCounter("circuit_breaker_state_transitions", labels, 1)
		}
		
		// Log significant state changes
		if to == StateOpen {
			fmt.Printf("CIRCUIT BREAKER OPENED: %s (failures exceeded threshold)\n", name)
		} else if to == StateClosed && from == StateHalfOpen {
			fmt.Printf("CIRCUIT BREAKER CLOSED: %s (service recovered)\n", name)
		}
	}
	
	return NewCircuitBreaker(config, m.monitoring)
}

func (m *CircuitBreakerManager) calculateSuccessRate(metrics CircuitBreakerMetrics) float64 {
	if metrics.TotalRequests == 0 {
		return 100.0
	}
	return float64(metrics.SuccessfulReqs) / float64(metrics.TotalRequests) * 100
}

func (m *CircuitBreakerManager) recordAggregateMetrics() {
	if m.monitoring == nil {
		return
	}
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	openCount := 0
	halfOpenCount := 0
	closedCount := 0
	totalRequests := int64(0)
	totalSuccessful := int64(0)
	totalFailed := int64(0)
	totalRejected := int64(0)
	
	for _, cb := range m.circuitBreakers {
		state := cb.GetState()
		metrics := cb.GetMetrics()
		
		switch state {
		case StateOpen:
			openCount++
		case StateHalfOpen:
			halfOpenCount++
		case StateClosed:
			closedCount++
		}
		
		totalRequests += metrics.TotalRequests
		totalSuccessful += metrics.SuccessfulReqs
		totalFailed += metrics.FailedReqs
		totalRejected += metrics.RejectedReqs
	}
	
	// Record aggregate metrics
	labels := map[string]string{"manager": "circuit_breaker"}
	m.monitoring.SetGauge("circuit_breaker_open_count", labels, float64(openCount))
	m.monitoring.SetGauge("circuit_breaker_half_open_count", labels, float64(halfOpenCount))
	m.monitoring.SetGauge("circuit_breaker_closed_count", labels, float64(closedCount))
	m.monitoring.SetGauge("circuit_breaker_total_requests", labels, float64(totalRequests))
	m.monitoring.SetGauge("circuit_breaker_total_successful", labels, float64(totalSuccessful))
	m.monitoring.SetGauge("circuit_breaker_total_failed", labels, float64(totalFailed))
	m.monitoring.SetGauge("circuit_breaker_total_rejected", labels, float64(totalRejected))
	
	// Calculate and record overall health
	totalCircuitBreakers := len(m.circuitBreakers)
	healthyCircuitBreakers := closedCount + halfOpenCount
	healthPercentage := float64(healthyCircuitBreakers) / float64(totalCircuitBreakers) * 100
	m.monitoring.SetGauge("circuit_breaker_health_percentage", labels, healthPercentage)
}

// Convenience methods for common operations

// WrapMinIOOperation wraps MinIO operations with circuit breaker
func (m *CircuitBreakerManager) WrapMinIOOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceMinIO, operation)
}

// WrapHTTPOperation wraps HTTP operations with circuit breaker
func (m *CircuitBreakerManager) WrapHTTPOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceHTTP, operation)
}

// WrapGitOperation wraps Git operations with circuit breaker
func (m *CircuitBreakerManager) WrapGitOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceGit, operation)
}

// WrapKubernetesOperation wraps Kubernetes API operations with circuit breaker
func (m *CircuitBreakerManager) WrapKubernetesOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceKubernetes, operation)
}

// WrapSecurityOperation wraps security validation operations with circuit breaker
func (m *CircuitBreakerManager) WrapSecurityOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceSecurity, operation)
}

// WrapBackupOperation wraps backup tool operations with circuit breaker
func (m *CircuitBreakerManager) WrapBackupOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceBackup, operation)
}

// WrapGitOpsOperation wraps GitOps operations with circuit breaker
func (m *CircuitBreakerManager) WrapGitOpsOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceGitOps, operation)
}

// WrapWebhookOperation wraps webhook operations with circuit breaker
func (m *CircuitBreakerManager) WrapWebhookOperation(ctx context.Context, operation func() error) error {
	return m.ExecuteWithServiceCircuitBreaker(ctx, ServiceWebhook, operation)
}