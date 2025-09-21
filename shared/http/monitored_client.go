package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"shared-config/monitoring"
	sharedconfig "shared-config/config"
)

// MonitoredHTTPClient wraps HTTPClient with monitoring capabilities
type MonitoredHTTPClient struct {
	client           *HTTPClient
	metricsCollector monitoring.MetricsCollector
	logger           monitoring.Logger
	eventPublisher   monitoring.EventPublisher
	profile          string
}

// NewMonitoredHTTPClient creates a monitored HTTP client
func NewMonitoredHTTPClient(config *HTTPClientConfig, profile string, logger monitoring.Logger) *MonitoredHTTPClient {
	client := NewHTTPClient(config)
	
	monitoringConfig := &monitoring.MonitoringConfig{
		MetricsEnabled: true,
		EventsEnabled:  true,
	}
	
	return &MonitoredHTTPClient{
		client:           client,
		metricsCollector: monitoring.NewMetricsCollector(monitoringConfig),
		logger:           logger.WithFields(map[string]interface{}{"component": "http_client", "profile": profile}),
		eventPublisher:   monitoring.NewEventPublisher(monitoringConfig, logger),
		profile:          profile,
	}
}

// NewMonitoredHTTPClientFromConfig creates monitored HTTP client from shared configuration
func NewMonitoredHTTPClientFromConfig(sharedConfig *sharedconfig.SharedConfig, profile string, logger monitoring.Logger) *MonitoredHTTPClient {
	client := NewHTTPClientFromConfig(sharedConfig, profile)
	
	monitoringConfig := &monitoring.MonitoringConfig{
		MetricsEnabled: true,
		EventsEnabled:  true,
	}
	
	return &MonitoredHTTPClient{
		client:           client,
		metricsCollector: monitoring.NewMetricsCollector(monitoringConfig),
		logger:           logger.WithFields(map[string]interface{}{"component": "http_client", "profile": profile}),
		eventPublisher:   monitoring.NewEventPublisher(monitoringConfig, logger),
		profile:          profile,
	}
}

// Implement MonitoredComponent interface

func (mc *MonitoredHTTPClient) GetComponentName() string {
	return fmt.Sprintf("http_client_%s", mc.profile)
}

func (mc *MonitoredHTTPClient) GetComponentVersion() string {
	return "1.0.0"
}

func (mc *MonitoredHTTPClient) GetMetrics() map[string]interface{} {
	httpMetrics := mc.client.GetMetrics()
	
	return map[string]interface{}{
		"total_requests":        httpMetrics.TotalRequests,
		"successful_requests":   httpMetrics.SuccessfulReqs,
		"failed_requests":       httpMetrics.FailedReqs,
		"avg_response_time_ms":  httpMetrics.AvgResponseTime.Milliseconds(),
		"timeout_errors":        httpMetrics.TimeoutErrors,
		"connection_errors":     httpMetrics.ConnectionErrors,
		"circuit_breaker_hits":  httpMetrics.CircuitBreakerHits,
		"retry_attempts":        httpMetrics.RetryAttempts,
		"total_bytes_read":      httpMetrics.TotalBytesRead,
		"total_bytes_written":   httpMetrics.TotalBytesWritten,
		"active_connections":    httpMetrics.ActiveConns,
		"idle_connections":      httpMetrics.IdleConns,
		"connection_reuse":      httpMetrics.ConnectionReuse,
	}
}

func (mc *MonitoredHTTPClient) ResetMetrics() {
	mc.client.ResetMetrics()
	mc.metricsCollector.ResetMetrics()
}

func (mc *MonitoredHTTPClient) HealthCheck(ctx context.Context) monitoring.HealthStatus {
	// Perform a simple health check by attempting a HEAD request to a known endpoint
	// In a real implementation, this might ping a health check endpoint
	
	metrics := mc.client.GetMetrics()
	
	// Determine health based on error rates and circuit breaker status
	var status monitoring.HealthStatusType
	var message string
	
	if metrics.TotalRequests == 0 {
		status = monitoring.HealthStatusHealthy
		message = "HTTP client initialized, no requests yet"
	} else {
		errorRate := float64(metrics.FailedReqs) / float64(metrics.TotalRequests)
		
		if errorRate > 0.5 { // More than 50% errors
			status = monitoring.HealthStatusUnhealthy
			message = fmt.Sprintf("High error rate: %.2f%%", errorRate*100)
		} else if errorRate > 0.1 { // More than 10% errors
			status = monitoring.HealthStatusDegraded
			message = fmt.Sprintf("Elevated error rate: %.2f%%", errorRate*100)
		} else {
			status = monitoring.HealthStatusHealthy
			message = fmt.Sprintf("Operating normally, error rate: %.2f%%", errorRate*100)
		}
		
		// Check circuit breaker
		if metrics.CircuitBreakerHits > 0 {
			status = monitoring.HealthStatusDegraded
			message += fmt.Sprintf(", circuit breaker hits: %d", metrics.CircuitBreakerHits)
		}
	}
	
	return monitoring.HealthStatus{
		Status:    status,
		Message:   message,
		LastCheck: time.Now(),
		Metrics: map[string]interface{}{
			"total_requests":       metrics.TotalRequests,
			"error_rate":          float64(metrics.FailedReqs) / max(float64(metrics.TotalRequests), 1),
			"avg_response_time_ms": metrics.AvgResponseTime.Milliseconds(),
			"circuit_breaker_hits": metrics.CircuitBreakerHits,
		},
	}
}

func (mc *MonitoredHTTPClient) GetDependencies() []string {
	// HTTP clients typically depend on network connectivity and target services
	return []string{"network", "dns"}
}

func (mc *MonitoredHTTPClient) OnStart(ctx context.Context) error {
	mc.logger.Info("http_client_starting", map[string]interface{}{
		"profile": mc.profile,
	})
	
	// Publish start event
	event := monitoring.CreateSystemStartEvent(mc.GetComponentName())
	mc.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

func (mc *MonitoredHTTPClient) OnStop(ctx context.Context) error {
	mc.logger.Info("http_client_stopping", map[string]interface{}{
		"profile": mc.profile,
	})
	
	// Close the underlying client
	err := mc.client.Close()
	
	// Publish stop event
	event := monitoring.CreateSystemStopEvent(mc.GetComponentName())
	mc.eventPublisher.PublishSystemEvent(event)
	
	return err
}

// HTTP request methods with monitoring hooks

func (mc *MonitoredHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	start := time.Now()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	
	// Log request start
	mc.logger.Info("http_request_start", map[string]interface{}{
		"request_id": requestID,
		"method":     req.Method,
		"url":        req.URL.String(),
		"profile":    mc.profile,
	})
	
	// Record request metrics
	labels := map[string]string{
		"method":  req.Method,
		"profile": mc.profile,
	}
	mc.metricsCollector.IncCounter(monitoring.HTTPRequestsTotal, labels, 1)
	
	// Publish request start event
	startEvent := monitoring.SystemEvent{
		ID:        fmt.Sprintf("evt_%s_start", requestID),
		Timestamp: time.Now(),
		Type:      "http_request_start",
		Component: mc.GetComponentName(),
		Action:    "http_request",
		Status:    "started",
		Metadata: map[string]interface{}{
			"request_id": requestID,
			"method":     req.Method,
			"url":        req.URL.String(),
		},
	}
	mc.eventPublisher.PublishSystemEvent(startEvent)
	
	// Perform the request
	resp, err := mc.client.Do(ctx, req)
	
	duration := time.Since(start)
	
	// Record response metrics
	if err != nil {
		// Request failed
		mc.logger.Error("http_request_failed", map[string]interface{}{
			"request_id":    requestID,
			"method":        req.Method,
			"url":           req.URL.String(),
			"duration_ms":   duration.Milliseconds(),
			"error":         err.Error(),
			"profile":       mc.profile,
		})
		
		labels["status"] = "error"
		mc.metricsCollector.IncCounter(monitoring.HTTPErrorsTotal, labels, 1)
		mc.metricsCollector.RecordDuration(monitoring.HTTPRequestDuration, labels, duration)
		
		// Publish error event
		errorEvent := monitoring.CreateErrorEvent(mc.GetComponentName(), "http_request", err, "error")
		errorEvent.Context["request_id"] = requestID
		errorEvent.Context["method"] = req.Method
		errorEvent.Context["url"] = req.URL.String()
		errorEvent.Context["duration_ms"] = duration.Milliseconds()
		mc.eventPublisher.PublishErrorEvent(errorEvent)
		
	} else {
		// Request succeeded
		statusCode := resp.StatusCode
		statusClass := fmt.Sprintf("%dxx", statusCode/100)
		
		mc.logger.Info("http_request_complete", map[string]interface{}{
			"request_id":    requestID,
			"method":        req.Method,
			"url":           req.URL.String(),
			"status_code":   statusCode,
			"duration_ms":   duration.Milliseconds(),
			"profile":       mc.profile,
		})
		
		labels["status"] = statusClass
		labels["status_code"] = fmt.Sprintf("%d", statusCode)
		mc.metricsCollector.RecordDuration(monitoring.HTTPRequestDuration, labels, duration)
		
		// Publish completion event
		completeEvent := monitoring.SystemEvent{
			ID:        fmt.Sprintf("evt_%s_complete", requestID),
			Timestamp: time.Now(),
			Type:      "http_request_complete",
			Component: mc.GetComponentName(),
			Action:    "http_request",
			Status:    "completed",
			Duration:  duration,
			Metadata: map[string]interface{}{
				"request_id":  requestID,
				"method":      req.Method,
				"url":         req.URL.String(),
				"status_code": statusCode,
			},
		}
		mc.eventPublisher.PublishSystemEvent(completeEvent)
	}
	
	return resp, err
}

func (mc *MonitoredHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return mc.Do(ctx, req)
}

func (mc *MonitoredHTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return mc.Do(ctx, req)
}

func (mc *MonitoredHTTPClient) PostJSON(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	return mc.client.PostJSON(ctx, url, payload)
}

// Additional monitoring methods

func (mc *MonitoredHTTPClient) GetMetricsCollector() monitoring.MetricsCollector {
	return mc.metricsCollector
}

func (mc *MonitoredHTTPClient) GetEventPublisher() monitoring.EventPublisher {
	return mc.eventPublisher
}

func (mc *MonitoredHTTPClient) GetLogger() monitoring.Logger {
	return mc.logger
}

func (mc *MonitoredHTTPClient) Close() error {
	return mc.client.Close()
}

func (mc *MonitoredHTTPClient) GetConfig() *HTTPClientConfig {
	return mc.client.GetConfig()
}

// MonitoredHTTPClientPool manages a pool of monitored HTTP clients
type MonitoredHTTPClientPool struct {
	pool              *HTTPClientPool
	clients           map[string]*MonitoredHTTPClient
	metricsCollector  monitoring.MetricsCollector
	logger            monitoring.Logger
	config            *sharedconfig.SharedConfig
}

// NewMonitoredHTTPClientPool creates a monitored HTTP client pool
func NewMonitoredHTTPClientPool(config *sharedconfig.SharedConfig, logger monitoring.Logger) *MonitoredHTTPClientPool {
	monitoringConfig := &monitoring.MonitoringConfig{
		MetricsEnabled: true,
		EventsEnabled:  true,
	}
	
	return &MonitoredHTTPClientPool{
		pool:             NewHTTPClientPool(config),
		clients:          make(map[string]*MonitoredHTTPClient),
		metricsCollector: monitoring.NewMetricsCollector(monitoringConfig),
		logger:           logger.WithFields(map[string]interface{}{"component": "http_client_pool"}),
		config:           config,
	}
}

// GetMonitoredClient returns a monitored client for the specified profile
func (mcp *MonitoredHTTPClientPool) GetMonitoredClient(profile string) (*MonitoredHTTPClient, error) {
	if client, exists := mcp.clients[profile]; exists {
		return client, nil
	}
	
	// Create new monitored client
	client := NewMonitoredHTTPClientFromConfig(mcp.config, profile, mcp.logger)
	mcp.clients[profile] = client
	
	mcp.logger.Info("monitored_client_created", map[string]interface{}{
		"profile": profile,
	})
	
	return client, nil
}

// GetAggregatedMetrics returns aggregated metrics from all monitored clients
func (mcp *MonitoredHTTPClientPool) GetAggregatedMetrics() map[string]interface{} {
	aggregated := make(map[string]interface{})
	
	var totalRequests, totalSuccessful, totalFailed int64
	
	for profile, client := range mcp.clients {
		metrics := client.GetMetrics()
		aggregated[profile] = metrics
		
		// Aggregate totals
		if total, ok := metrics["total_requests"].(int64); ok {
			totalRequests += total
		}
		if successful, ok := metrics["successful_requests"].(int64); ok {
			totalSuccessful += successful
		}
		if failed, ok := metrics["failed_requests"].(int64); ok {
			totalFailed += failed
		}
	}
	
	// Calculate overall metrics
	var successRate float64
	if totalRequests > 0 {
		successRate = float64(totalSuccessful) / float64(totalRequests) * 100
	}
	
	aggregated["summary"] = map[string]interface{}{
		"total_clients":      len(mcp.clients),
		"total_requests":     totalRequests,
		"successful_requests": totalSuccessful,
		"failed_requests":    totalFailed,
		"success_rate":       successRate,
	}
	
	return aggregated
}

// HealthCheck performs health checks on all monitored clients
func (mcp *MonitoredHTTPClientPool) HealthCheck(ctx context.Context) monitoring.OverallHealthStatus {
	componentHealth := make(map[string]monitoring.HealthStatus)
	
	var healthyCount, degradedCount, unhealthyCount int
	
	for profile, client := range mcp.clients {
		health := client.HealthCheck(ctx)
		componentHealth[profile] = health
		
		switch health.Status {
		case monitoring.HealthStatusHealthy:
			healthyCount++
		case monitoring.HealthStatusDegraded:
			degradedCount++
		case monitoring.HealthStatusUnhealthy:
			unhealthyCount++
		}
	}
	
	// Determine overall status
	var overallStatus monitoring.HealthStatusType
	if unhealthyCount > 0 {
		overallStatus = monitoring.HealthStatusUnhealthy
	} else if degradedCount > 0 {
		overallStatus = monitoring.HealthStatusDegraded
	} else if healthyCount > 0 {
		overallStatus = monitoring.HealthStatusHealthy
	} else {
		overallStatus = monitoring.HealthStatusUnknown
	}
	
	return monitoring.OverallHealthStatus{
		Status:     overallStatus,
		Components: componentHealth,
		Summary: monitoring.HealthSummary{
			TotalComponents:     len(mcp.clients),
			HealthyComponents:   healthyCount,
			DegradedComponents:  degradedCount,
			UnhealthyComponents: unhealthyCount,
		},
		Timestamp: time.Now(),
	}
}

// Close closes all monitored clients
func (mcp *MonitoredHTTPClientPool) Close() error {
	var errors []error
	
	for profile, client := range mcp.clients {
		if err := client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close client %s: %v", profile, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("errors closing monitored clients: %v", errors)
	}
	
	return nil
}

// Helper function for max calculation
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}