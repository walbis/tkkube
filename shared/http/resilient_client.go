package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
	"shared-config/resilience"
)

// ResilientHTTPClient wraps HTTP client with circuit breaker and advanced resilience patterns
type ResilientHTTPClient struct {
	client                  *HTTPClient
	circuitBreakerManager   *resilience.CircuitBreakerManager
	config                  *HTTPClientConfig
	monitoring              monitoring.MetricsCollector
	serviceName             string
	mu                      sync.RWMutex
}

// NewResilientHTTPClient creates a new resilient HTTP client
func NewResilientHTTPClient(
	config *HTTPClientConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
	serviceName string,
) *ResilientHTTPClient {
	if config == nil {
		config = DefaultHTTPClientConfig()
	}
	
	if serviceName == "" {
		serviceName = "http_client"
	}
	
	client := NewHTTPClient(config)
	
	return &ResilientHTTPClient{
		client:                client,
		circuitBreakerManager: circuitBreakerManager,
		config:                config,
		monitoring:            monitoring,
		serviceName:           serviceName,
	}
}

// NewResilientHTTPClientFromSharedConfig creates resilient client from shared configuration
func NewResilientHTTPClientFromSharedConfig(
	sharedConfig *sharedconfig.SharedConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
	serviceName string,
	profile string,
) *ResilientHTTPClient {
	// Create HTTP client config
	config := DefaultHTTPClientConfig()
	
	// Apply shared configuration
	if sharedConfig != nil {
		// Apply timeout configuration
		config.RequestTimeout = sharedConfig.Timeouts.HTTPReadTimeout
		config.DialTimeout = 10 * time.Second // Default if not specified
		
		// Apply retry configuration
		config.MaxRetries = sharedConfig.Retries.MaxRetries
		config.RetryDelay = sharedConfig.Retries.BaseRetryDelay
		config.BackoffFactor = sharedConfig.Retries.RetryMultiplier
		
		// Apply circuit breaker configuration
		config.CircuitBreakerEnabled = true
		config.FailureThreshold = sharedConfig.Retries.CircuitBreakerThreshold
		config.CircuitBreakerTimeout = sharedConfig.Retries.CircuitBreakerTimeout
		config.CircuitBreakerResetTime = sharedConfig.Retries.CircuitBreakerRecoveryTime
		
		// Apply performance settings
		if sharedConfig.Performance.Optimization.Compression {
			config.CompressionEnabled = true
		}
		
		// Apply security settings
		if !sharedConfig.Security.Network.VerifySSL {
			config.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		}
	}
	
	// Apply profile-specific overrides
	switch profile {
	case "backup_tool":
		config.RequestTimeout = 5 * time.Minute // Backup operations can be slow
		config.MaxRetries = 5
		serviceName = "backup_client"
	case "gitops":
		config.RequestTimeout = 2 * time.Minute
		config.MaxRetries = 3
		serviceName = "gitops_client"
	case "webhook":
		config.RequestTimeout = 30 * time.Second
		config.MaxRetries = 2
		serviceName = "webhook_client"
	case "security":
		config.RequestTimeout = 15 * time.Second
		config.MaxRetries = 2
		serviceName = "security_client"
	}
	
	return NewResilientHTTPClient(config, circuitBreakerManager, monitoring, serviceName)
}

// Do performs HTTP request with circuit breaker protection
func (rc *ResilientHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return rc.doWithCircuitBreaker(ctx, req)
}

// Get performs HTTP GET request with resilience patterns
func (rc *ResilientHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %v", err)
	}
	return rc.Do(ctx, req)
}

// Post performs HTTP POST request with resilience patterns
func (rc *ResilientHTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return rc.Do(ctx, req)
}

// PostJSON performs HTTP POST request with JSON payload and resilience patterns
func (rc *ResilientHTTPClient) PostJSON(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}
	
	return rc.Post(ctx, url, "application/json", strings.NewReader(string(jsonData)))
}

// Put performs HTTP PUT request with resilience patterns
func (rc *ResilientHTTPClient) Put(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create PUT request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return rc.Do(ctx, req)
}

// Delete performs HTTP DELETE request with resilience patterns
func (rc *ResilientHTTPClient) Delete(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DELETE request: %v", err)
	}
	return rc.Do(ctx, req)
}

// ExecuteWithRetryAndCircuitBreaker executes a custom HTTP operation with full resilience
func (rc *ResilientHTTPClient) ExecuteWithRetryAndCircuitBreaker(ctx context.Context, operation func() (*http.Response, error)) (*http.Response, error) {
	var response *http.Response
	var err error
	
	cbError := rc.circuitBreakerManager.ExecuteWithCircuitBreaker(ctx, rc.serviceName, func() error {
		response, err = operation()
		return err
	})
	
	if cbError != nil {
		// Circuit breaker rejected the request
		if resilience.IsCircuitBreakerError(cbError) {
			rc.recordMetric("http_circuit_breaker_rejections", 1)
			return nil, cbError
		}
		return nil, cbError
	}
	
	return response, err
}

// GetCircuitBreakerState returns the current circuit breaker state
func (rc *ResilientHTTPClient) GetCircuitBreakerState() resilience.CircuitBreakerState {
	cb := rc.circuitBreakerManager.GetCircuitBreaker(rc.serviceName)
	return cb.GetState()
}

// GetMetrics returns HTTP client metrics including circuit breaker metrics
func (rc *ResilientHTTPClient) GetMetrics() map[string]interface{} {
	httpMetrics := rc.client.GetMetrics()
	cb := rc.circuitBreakerManager.GetCircuitBreaker(rc.serviceName)
	cbMetrics := cb.GetMetrics()
	
	return map[string]interface{}{
		"http_client": map[string]interface{}{
			"total_requests":         httpMetrics.TotalRequests,
			"successful_requests":    httpMetrics.SuccessfulReqs,
			"failed_requests":        httpMetrics.FailedReqs,
			"avg_response_time_ms":   httpMetrics.AvgResponseTime.Milliseconds(),
			"timeout_errors":         httpMetrics.TimeoutErrors,
			"connection_errors":      httpMetrics.ConnectionErrors,
			"retry_attempts":         httpMetrics.RetryAttempts,
		},
		"circuit_breaker": map[string]interface{}{
			"state":                cbMetrics.State.String(),
			"total_requests":       cbMetrics.TotalRequests,
			"successful_requests":  cbMetrics.SuccessfulReqs,
			"failed_requests":      cbMetrics.FailedReqs,
			"rejected_requests":    cbMetrics.RejectedReqs,
			"failure_streak":       cbMetrics.FailureStreak,
			"success_streak":       cbMetrics.SuccessStreak,
			"state_changes":        cbMetrics.StateChanges,
			"last_failure":         cbMetrics.LastFailureTime,
			"last_success":         cbMetrics.LastSuccessTime,
			"last_state_change":    cbMetrics.LastStateChange,
		},
		"service_name": rc.serviceName,
		"timestamp":    time.Now(),
	}
}

// ResetCircuitBreaker resets the circuit breaker for this client
func (rc *ResilientHTTPClient) ResetCircuitBreaker() error {
	return rc.circuitBreakerManager.ResetCircuitBreaker(rc.serviceName)
}

// ForceOpenCircuitBreaker forces the circuit breaker to open state
func (rc *ResilientHTTPClient) ForceOpenCircuitBreaker() error {
	return rc.circuitBreakerManager.ForceOpenCircuitBreaker(rc.serviceName)
}

// IsHealthy returns true if the circuit breaker is not in OPEN state
func (rc *ResilientHTTPClient) IsHealthy() bool {
	return rc.GetCircuitBreakerState() != resilience.StateOpen
}

// GetHealthStatus returns detailed health status
func (rc *ResilientHTTPClient) GetHealthStatus() map[string]interface{} {
	state := rc.GetCircuitBreakerState()
	metrics := rc.GetMetrics()
	
	cbMetrics := metrics["circuit_breaker"].(map[string]interface{})
	httpMetrics := metrics["http_client"].(map[string]interface{})
	
	successRate := float64(0)
	if totalReqs := cbMetrics["total_requests"].(int64); totalReqs > 0 {
		successRate = float64(cbMetrics["successful_requests"].(int64)) / float64(totalReqs) * 100
	}
	
	return map[string]interface{}{
		"service":       rc.serviceName,
		"healthy":       state != resilience.StateOpen,
		"state":         state.String(),
		"success_rate":  successRate,
		"error_rate":    100 - successRate,
		"total_requests": cbMetrics["total_requests"],
		"recent_failures": cbMetrics["failure_streak"],
		"avg_response_time_ms": httpMetrics["avg_response_time_ms"],
		"last_failure":  cbMetrics["last_failure"],
		"timestamp":     time.Now(),
	}
}

// Close cleanly shuts down the HTTP client
func (rc *ResilientHTTPClient) Close() error {
	return rc.client.Close()
}

// Private methods

func (rc *ResilientHTTPClient) doWithCircuitBreaker(ctx context.Context, req *http.Request) (*http.Response, error) {
	var response *http.Response
	var err error
	
	// Wrap the HTTP request in circuit breaker
	cbError := rc.circuitBreakerManager.ExecuteWithCircuitBreaker(ctx, rc.serviceName, func() error {
		response, err = rc.client.Do(ctx, req)
		
		// Evaluate if this should be considered a failure for circuit breaker
		if err != nil {
			return err
		}
		
		// Consider HTTP errors as failures for circuit breaker
		if response != nil && rc.isCircuitBreakerFailureStatusCode(response.StatusCode) {
			return fmt.Errorf("HTTP %d: %s", response.StatusCode, response.Status)
		}
		
		return nil
	})
	
	if cbError != nil {
		// Check if this is a circuit breaker rejection
		if resilience.IsCircuitBreakerError(cbError) {
			rc.recordMetric("http_circuit_breaker_rejections", 1)
			return nil, cbError
		}
		
		// This is an actual HTTP error that occurred during execution
		return response, cbError
	}
	
	// Success case
	rc.recordMetric("http_requests_total", 1)
	return response, nil
}

func (rc *ResilientHTTPClient) isCircuitBreakerFailureStatusCode(statusCode int) bool {
	// Consider 5xx errors and specific 4xx errors as circuit breaker failures
	if statusCode >= 500 {
		return true
	}
	
	// Specific 4xx errors that indicate service issues
	failureCodes := []int{408, 429} // Request Timeout, Too Many Requests
	for _, code := range failureCodes {
		if statusCode == code {
			return true
		}
	}
	
	return false
}

func (rc *ResilientHTTPClient) recordMetric(metricName string, value float64) {
	if rc.monitoring == nil {
		return
	}
	
	labels := map[string]string{
		"service": rc.serviceName,
		"client":  "resilient_http",
	}
	rc.monitoring.IncCounter(metricName, labels, value)
}

// HTTPClientPool manages multiple resilient HTTP clients for different services
type HTTPClientPool struct {
	clients               map[string]*ResilientHTTPClient
	circuitBreakerManager *resilience.CircuitBreakerManager
	sharedConfig          *sharedconfig.SharedConfig
	monitoring            monitoring.MetricsCollector
	mu                    sync.RWMutex
}

// NewHTTPClientPool creates a new pool of resilient HTTP clients
func NewHTTPClientPool(
	sharedConfig *sharedconfig.SharedConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
) *HTTPClientPool {
	return &HTTPClientPool{
		clients:               make(map[string]*ResilientHTTPClient),
		circuitBreakerManager: circuitBreakerManager,
		sharedConfig:          sharedConfig,
		monitoring:            monitoring,
	}
}

// GetClient returns a resilient HTTP client for a specific service
func (pool *HTTPClientPool) GetClient(serviceName, profile string) *ResilientHTTPClient {
	pool.mu.RLock()
	if client, exists := pool.clients[serviceName]; exists {
		pool.mu.RUnlock()
		return client
	}
	pool.mu.RUnlock()
	
	// Create new client
	pool.mu.Lock()
	defer pool.mu.Unlock()
	
	// Double-check after acquiring write lock
	if client, exists := pool.clients[serviceName]; exists {
		return client
	}
	
	client := NewResilientHTTPClientFromSharedConfig(
		pool.sharedConfig,
		pool.circuitBreakerManager,
		pool.monitoring,
		serviceName,
		profile,
	)
	
	pool.clients[serviceName] = client
	return client
}

// GetAllClients returns all HTTP clients in the pool
func (pool *HTTPClientPool) GetAllClients() map[string]*ResilientHTTPClient {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	
	result := make(map[string]*ResilientHTTPClient)
	for name, client := range pool.clients {
		result[name] = client
	}
	
	return result
}

// GetPoolHealthStatus returns health status for all clients in the pool
func (pool *HTTPClientPool) GetPoolHealthStatus() map[string]interface{} {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	
	status := make(map[string]interface{})
	healthyCount := 0
	totalCount := len(pool.clients)
	
	clients := make(map[string]interface{})
	for name, client := range pool.clients {
		clientHealth := client.GetHealthStatus()
		clients[name] = clientHealth
		
		if clientHealth["healthy"].(bool) {
			healthyCount++
		}
	}
	
	status["overall_health"] = float64(healthyCount) / float64(totalCount) * 100
	status["healthy_clients"] = healthyCount
	status["total_clients"] = totalCount
	status["clients"] = clients
	status["timestamp"] = time.Now()
	
	return status
}

// CloseAll closes all HTTP clients in the pool
func (pool *HTTPClientPool) CloseAll() error {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	
	var lastError error
	for _, client := range pool.clients {
		if err := client.Close(); err != nil {
			lastError = err
		}
	}
	
	return lastError
}