package http

import (
	"context"
	"fmt"
	"sync"
	"time"

	sharedconfig "shared-config/config"
)

// HTTPClientPool manages a pool of HTTP clients for different use cases
type HTTPClientPool struct {
	clients map[string]*HTTPClient
	config  *sharedconfig.SharedConfig
	mu      sync.RWMutex
}

// HTTPPoolMetrics aggregates metrics from all HTTP clients in the pool
type HTTPPoolMetrics struct {
	TotalClients       int                      `json:"total_clients"`
	PoolMetrics        map[string]HTTPMetrics   `json:"pool_metrics"`
	AggregatedMetrics  HTTPMetrics              `json:"aggregated_metrics"`
	LastUpdated        time.Time                `json:"last_updated"`
}

// ClientProfile defines different HTTP client configurations for specific use cases
type ClientProfile struct {
	Name        string
	Description string
	Config      *HTTPClientConfig
}

// NewHTTPClientPool creates a new HTTP client pool
func NewHTTPClientPool(config *sharedconfig.SharedConfig) *HTTPClientPool {
	return &HTTPClientPool{
		clients: make(map[string]*HTTPClient),
		config:  config,
	}
}

// GetClient returns an HTTP client for the specified profile, creating it if necessary
func (p *HTTPClientPool) GetClient(profile string) (*HTTPClient, error) {
	p.mu.RLock()
	if client, exists := p.clients[profile]; exists {
		p.mu.RUnlock()
		return client, nil
	}
	p.mu.RUnlock()

	// Create new client
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check pattern to avoid race condition
	if client, exists := p.clients[profile]; exists {
		return client, nil
	}

	client := NewHTTPClientFromConfig(p.config, profile)
	p.clients[profile] = client

	return client, nil
}

// GetOrCreateClient gets existing client or creates with custom config
func (p *HTTPClientPool) GetOrCreateClient(name string, config *HTTPClientConfig) *HTTPClient {
	p.mu.RLock()
	if client, exists := p.clients[name]; exists {
		p.mu.RUnlock()
		return client
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check pattern
	if client, exists := p.clients[name]; exists {
		return client
	}

	client := NewHTTPClient(config)
	p.clients[name] = client

	return client
}

// RemoveClient removes a client from the pool and closes it
func (p *HTTPClientPool) RemoveClient(profile string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[profile]; exists {
		delete(p.clients, profile)
		return client.Close()
	}

	return fmt.Errorf("client profile '%s' not found", profile)
}

// GetMetrics returns aggregated metrics from all clients in the pool
func (p *HTTPClientPool) GetMetrics() HTTPPoolMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	poolMetrics := make(map[string]HTTPMetrics)
	aggregated := HTTPMetrics{}

	for profile, client := range p.clients {
		metrics := client.GetMetrics()
		poolMetrics[profile] = metrics

		// Aggregate metrics
		aggregated.TotalRequests += metrics.TotalRequests
		aggregated.SuccessfulReqs += metrics.SuccessfulReqs
		aggregated.FailedReqs += metrics.FailedReqs
		aggregated.TotalResponseTime += metrics.TotalResponseTime
		aggregated.TimeoutErrors += metrics.TimeoutErrors
		aggregated.ConnectionErrors += metrics.ConnectionErrors
		aggregated.CircuitBreakerHits += metrics.CircuitBreakerHits
		aggregated.RetryAttempts += metrics.RetryAttempts
		aggregated.TotalBytesRead += metrics.TotalBytesRead
		aggregated.TotalBytesWritten += metrics.TotalBytesWritten
	}

	// Calculate average response time
	if aggregated.TotalRequests > 0 {
		aggregated.AvgResponseTime = aggregated.TotalResponseTime / time.Duration(aggregated.TotalRequests)
	}

	return HTTPPoolMetrics{
		TotalClients:      len(p.clients),
		PoolMetrics:       poolMetrics,
		AggregatedMetrics: aggregated,
		LastUpdated:       time.Now(),
	}
}

// ResetAllMetrics resets metrics for all clients in the pool
func (p *HTTPClientPool) ResetAllMetrics() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, client := range p.clients {
		client.ResetMetrics()
	}
}

// Close shuts down all clients in the pool
func (p *HTTPClientPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errors []error
	for profile, client := range p.clients {
		if err := client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close client %s: %v", profile, err))
		}
	}

	// Clear the clients map
	p.clients = make(map[string]*HTTPClient)

	if len(errors) > 0 {
		return fmt.Errorf("errors closing clients: %v", errors)
	}

	return nil
}

// ListProfiles returns all available client profiles
func (p *HTTPClientPool) ListProfiles() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	profiles := make([]string, 0, len(p.clients))
	for profile := range p.clients {
		profiles = append(profiles, profile)
	}

	return profiles
}

// GetClientProfiles returns predefined client profiles for common use cases
func GetClientProfiles() []ClientProfile {
	return []ClientProfile{
		{
			Name:        "webhook",
			Description: "Optimized for webhook calls with retry and circuit breaker",
			Config: &HTTPClientConfig{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				MaxConnsPerHost:     20,
				IdleConnTimeout:     60 * time.Second,
				KeepAlive:           30 * time.Second,
				DialTimeout:         5 * time.Second,
				RequestTimeout:      30 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				CompressionEnabled:  true,
				UserAgent:           "shared-config-webhook-client/1.0",
				MaxResponseSize:     5 * 1024 * 1024, // 5MB
				MaxRetries:          3,
				RetryDelay:          1 * time.Second,
				BackoffFactor:       2.0,
				JitterEnabled:       true,
				RetryableErrors: []string{
					"connection refused", "timeout", "network unreachable",
					"service unavailable", "gateway timeout",
				},
				CircuitBreakerEnabled:   true,
				FailureThreshold:        5,
				CircuitBreakerResetTime: 60 * time.Second,
			},
		},
		{
			Name:        "storage",
			Description: "Optimized for MinIO/S3 API calls with aggressive pooling",
			Config: &HTTPClientConfig{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 50,
				MaxConnsPerHost:     100,
				IdleConnTimeout:     120 * time.Second,
				KeepAlive:           60 * time.Second,
				DialTimeout:         10 * time.Second,
				RequestTimeout:      120 * time.Second,
				TLSHandshakeTimeout: 15 * time.Second,
				CompressionEnabled:  false, // S3 handles compression
				UserAgent:           "shared-config-storage-client/1.0",
				MaxResponseSize:     100 * 1024 * 1024, // 100MB for large objects
				MaxRetries:          5,
				RetryDelay:          2 * time.Second,
				BackoffFactor:       2.0,
				JitterEnabled:       true,
				RetryableErrors: []string{
					"connection refused", "timeout", "network unreachable",
					"service unavailable", "gateway timeout", "too many requests",
				},
				CircuitBreakerEnabled:   true,
				FailureThreshold:        10,
				CircuitBreakerResetTime: 120 * time.Second,
			},
		},
		{
			Name:        "api",
			Description: "General purpose API client with balanced settings",
			Config: &HTTPClientConfig{
				MaxIdleConns:        75,
				MaxIdleConnsPerHost: 25,
				MaxConnsPerHost:     50,
				IdleConnTimeout:     90 * time.Second,
				KeepAlive:           30 * time.Second,
				DialTimeout:         10 * time.Second,
				RequestTimeout:      60 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				CompressionEnabled:  true,
				UserAgent:           "shared-config-api-client/1.0",
				MaxResponseSize:     10 * 1024 * 1024, // 10MB
				MaxRetries:          3,
				RetryDelay:          1 * time.Second,
				BackoffFactor:       2.0,
				JitterEnabled:       true,
				RetryableErrors: []string{
					"connection refused", "timeout", "network unreachable",
					"service unavailable", "gateway timeout", "rate limit",
				},
				CircuitBreakerEnabled:   true,
				FailureThreshold:        5,
				CircuitBreakerResetTime: 60 * time.Second,
			},
		},
		{
			Name:        "monitoring",
			Description: "Low-overhead client for monitoring and health checks",
			Config: &HTTPClientConfig{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 5,
				MaxConnsPerHost:     10,
				IdleConnTimeout:     30 * time.Second,
				KeepAlive:           15 * time.Second,
				DialTimeout:         3 * time.Second,
				RequestTimeout:      10 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second,
				CompressionEnabled:  false,
				UserAgent:           "shared-config-monitor-client/1.0",
				MaxResponseSize:     1 * 1024 * 1024, // 1MB
				MaxRetries:          1,
				RetryDelay:          500 * time.Millisecond,
				BackoffFactor:       1.5,
				JitterEnabled:       false,
				RetryableErrors: []string{
					"connection refused", "timeout",
				},
				CircuitBreakerEnabled:   false,
				FailureThreshold:        3,
				CircuitBreakerResetTime: 30 * time.Second,
			},
		},
	}
}

// HTTPClientManager provides a high-level interface for managing HTTP clients
type HTTPClientManager struct {
	pool    *HTTPClientPool
	config  *sharedconfig.SharedConfig
	metrics *HTTPPoolMetrics
	mu      sync.RWMutex
}

// NewHTTPClientManager creates a new HTTP client manager
func NewHTTPClientManager(config *sharedconfig.SharedConfig) *HTTPClientManager {
	pool := NewHTTPClientPool(config)
	
	// Pre-create common clients
	profiles := GetClientProfiles()
	for _, profile := range profiles {
		pool.GetOrCreateClient(profile.Name, profile.Config)
	}

	return &HTTPClientManager{
		pool:   pool,
		config: config,
	}
}

// GetWebhookClient returns a client optimized for webhook calls
func (m *HTTPClientManager) GetWebhookClient() *HTTPClient {
	client, _ := m.pool.GetClient("webhook")
	return client
}

// GetStorageClient returns a client optimized for storage operations
func (m *HTTPClientManager) GetStorageClient() *HTTPClient {
	client, _ := m.pool.GetClient("storage")
	return client
}

// GetAPIClient returns a general-purpose API client
func (m *HTTPClientManager) GetAPIClient() *HTTPClient {
	client, _ := m.pool.GetClient("api")
	return client
}

// GetMonitoringClient returns a low-overhead monitoring client
func (m *HTTPClientManager) GetMonitoringClient() *HTTPClient {
	client, _ := m.pool.GetClient("monitoring")
	return client
}

// GetClient returns a client for the specified profile
func (m *HTTPClientManager) GetClient(profile string) (*HTTPClient, error) {
	return m.pool.GetClient(profile)
}

// CreateCustomClient creates a custom client with specific configuration
func (m *HTTPClientManager) CreateCustomClient(name string, config *HTTPClientConfig) *HTTPClient {
	return m.pool.GetOrCreateClient(name, config)
}

// GetMetrics returns current pool metrics
func (m *HTTPClientManager) GetMetrics() HTTPPoolMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	metrics := m.pool.GetMetrics()
	m.metrics = &metrics
	return metrics
}

// HealthCheck performs health checks on all clients
func (m *HTTPClientManager) HealthCheck(ctx context.Context) map[string]error {
	profiles := m.pool.ListProfiles()
	results := make(map[string]error)

	for _, profile := range profiles {
		client, err := m.pool.GetClient(profile)
		if err != nil {
			results[profile] = err
			continue
		}

		// Simple health check - this could be enhanced to ping actual endpoints
		metrics := client.GetMetrics()
		if metrics.CircuitBreakerHits > 0 && metrics.SuccessfulReqs == 0 {
			results[profile] = fmt.Errorf("client appears unhealthy: no successful requests, circuit breaker hits: %d", metrics.CircuitBreakerHits)
		} else {
			results[profile] = nil // Healthy
		}
	}

	return results
}

// ResetMetrics resets all client metrics
func (m *HTTPClientManager) ResetMetrics() {
	m.pool.ResetAllMetrics()
}

// Close shuts down the manager and all clients
func (m *HTTPClientManager) Close() error {
	return m.pool.Close()
}

// GetConnectionStats returns detailed connection statistics
func (m *HTTPClientManager) GetConnectionStats() map[string]interface{} {
	metrics := m.GetMetrics()
	
	stats := map[string]interface{}{
		"total_clients":      metrics.TotalClients,
		"total_requests":     metrics.AggregatedMetrics.TotalRequests,
		"success_rate":       float64(metrics.AggregatedMetrics.SuccessfulReqs) / float64(metrics.AggregatedMetrics.TotalRequests) * 100,
		"avg_response_time":  metrics.AggregatedMetrics.AvgResponseTime.Milliseconds(),
		"retry_rate":         float64(metrics.AggregatedMetrics.RetryAttempts) / float64(metrics.AggregatedMetrics.TotalRequests) * 100,
		"timeout_rate":       float64(metrics.AggregatedMetrics.TimeoutErrors) / float64(metrics.AggregatedMetrics.TotalRequests) * 100,
		"circuit_breaker_rate": float64(metrics.AggregatedMetrics.CircuitBreakerHits) / float64(metrics.AggregatedMetrics.TotalRequests) * 100,
		"last_updated":       metrics.LastUpdated,
	}

	// Add per-client statistics
	clientStats := make(map[string]map[string]interface{})
	for profile, clientMetrics := range metrics.PoolMetrics {
		clientStats[profile] = map[string]interface{}{
			"requests":           clientMetrics.TotalRequests,
			"success_rate":       float64(clientMetrics.SuccessfulReqs) / float64(clientMetrics.TotalRequests) * 100,
			"avg_response_time":  clientMetrics.AvgResponseTime.Milliseconds(),
			"errors":            clientMetrics.FailedReqs,
			"timeouts":          clientMetrics.TimeoutErrors,
			"connection_errors": clientMetrics.ConnectionErrors,
			"retries":           clientMetrics.RetryAttempts,
			"circuit_breaker_hits": clientMetrics.CircuitBreakerHits,
		}
	}
	stats["clients"] = clientStats

	return stats
}