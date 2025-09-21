package monitoring

import (
	"sync"
	"time"
)

// DefaultMetricsCollector provides a thread-safe implementation of MetricsCollector
type DefaultMetricsCollector struct {
	metrics []Metric
	mu      sync.RWMutex
	config  *MonitoringConfig
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config *MonitoringConfig) *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		metrics: make([]Metric, 0),
		config:  config,
	}
}

// IncCounter increments a counter metric
func (mc *DefaultMetricsCollector) IncCounter(name string, labels map[string]string, value float64) {
	metric := Metric{
		Name:      name,
		Type:      MetricTypeCounter,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
	mc.addMetric(metric)
}

// SetGauge sets a gauge metric value
func (mc *DefaultMetricsCollector) SetGauge(name string, labels map[string]string, value float64) {
	metric := Metric{
		Name:      name,
		Type:      MetricTypeGauge,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
	mc.addMetric(metric)
}

// RecordHistogram records a histogram metric
func (mc *DefaultMetricsCollector) RecordHistogram(name string, labels map[string]string, value float64) {
	metric := Metric{
		Name:      name,
		Type:      MetricTypeHistogram,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
	mc.addMetric(metric)
}

// RecordDuration records a timing metric
func (mc *DefaultMetricsCollector) RecordDuration(name string, labels map[string]string, duration time.Duration) {
	metric := Metric{
		Name:      name,
		Type:      MetricTypeHistogram,
		Value:     float64(duration.Milliseconds()),
		Labels:    labels,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"unit": "milliseconds",
		},
	}
	mc.addMetric(metric)
}

// RecordMetric records a custom metric
func (mc *DefaultMetricsCollector) RecordMetric(metric Metric) error {
	if metric.Timestamp.IsZero() {
		metric.Timestamp = time.Now()
	}
	mc.addMetric(metric)
	return nil
}

// GetMetrics returns all collected metrics
func (mc *DefaultMetricsCollector) GetMetrics() []Metric {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	// Return a copy to prevent concurrent modification
	metrics := make([]Metric, len(mc.metrics))
	copy(metrics, mc.metrics)
	return metrics
}

// ResetMetrics clears all collected metrics
func (mc *DefaultMetricsCollector) ResetMetrics() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.metrics = make([]Metric, 0)
}

// addMetric adds a metric to the collection with buffer management
func (mc *DefaultMetricsCollector) addMetric(metric Metric) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.metrics = append(mc.metrics, metric)
	
	// Apply buffer size limit if configured
	if mc.config != nil && mc.config.MaxMetricsBuffer > 0 {
		if len(mc.metrics) > mc.config.MaxMetricsBuffer {
			// Remove oldest metrics to maintain buffer size
			excess := len(mc.metrics) - mc.config.MaxMetricsBuffer
			mc.metrics = mc.metrics[excess:]
		}
	}
}

// MetricsAggregator provides metric aggregation functionality
type MetricsAggregator struct {
	collectors map[string]MetricsCollector
	mu         sync.RWMutex
}

// NewMetricsAggregator creates a new metrics aggregator
func NewMetricsAggregator() *MetricsAggregator {
	return &MetricsAggregator{
		collectors: make(map[string]MetricsCollector),
	}
}

// RegisterCollector registers a metrics collector for a component
func (ma *MetricsAggregator) RegisterCollector(component string, collector MetricsCollector) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.collectors[component] = collector
}

// UnregisterCollector removes a metrics collector
func (ma *MetricsAggregator) UnregisterCollector(component string) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	delete(ma.collectors, component)
}

// GetAggregatedMetrics returns aggregated metrics from all collectors
func (ma *MetricsAggregator) GetAggregatedMetrics() AggregatedMetrics {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	
	componentMetrics := make(map[string][]Metric)
	var allMetrics []Metric
	
	for component, collector := range ma.collectors {
		metrics := collector.GetMetrics()
		componentMetrics[component] = metrics
		allMetrics = append(allMetrics, metrics...)
	}
	
	return AggregatedMetrics{
		Timestamp:          time.Now(),
		ComponentMetrics:   componentMetrics,
		SystemMetrics:      allMetrics,
		PerformanceMetrics: ma.calculatePerformanceMetrics(allMetrics),
		ErrorMetrics:       ma.calculateErrorMetrics(allMetrics),
	}
}

// calculatePerformanceMetrics calculates performance metrics from raw metrics
func (ma *MetricsAggregator) calculatePerformanceMetrics(metrics []Metric) PerformanceMetrics {
	var totalRequests, successfulReqs, failedReqs int64
	var totalResponseTime time.Duration
	var responseTimes []time.Duration
	
	for _, metric := range metrics {
		switch metric.Name {
		case "http_requests_total":
			totalRequests += int64(metric.Value)
			if status, ok := metric.Labels["status"]; ok {
				if status[0] == '2' || status[0] == '3' { // 2xx, 3xx are successful
					successfulReqs += int64(metric.Value)
				} else {
					failedReqs += int64(metric.Value)
				}
			}
		case "http_request_duration_seconds", "request_duration_ms":
			duration := time.Duration(metric.Value) * time.Millisecond
			totalResponseTime += duration
			responseTimes = append(responseTimes, duration)
		}
	}
	
	var avgResponseTime time.Duration
	if len(responseTimes) > 0 {
		avgResponseTime = totalResponseTime / time.Duration(len(responseTimes))
	}
	
	var errorRate float64
	if totalRequests > 0 {
		errorRate = float64(failedReqs) / float64(totalRequests) * 100
	}
	
	var throughput float64
	if totalRequests > 0 {
		throughput = float64(totalRequests) / 60.0 // Assuming 1-minute window
	}
	
	// Calculate percentiles
	p95, p99 := calculatePercentiles(responseTimes)
	
	return PerformanceMetrics{
		TotalRequests:   totalRequests,
		SuccessfulReqs:  successfulReqs,
		FailedReqs:      failedReqs,
		AvgResponseTime: avgResponseTime,
		Throughput:      throughput,
		ErrorRate:       errorRate,
		P95ResponseTime: p95,
		P99ResponseTime: p99,
	}
}

// calculateErrorMetrics calculates error metrics from raw metrics  
func (ma *MetricsAggregator) calculateErrorMetrics(metrics []Metric) ErrorMetrics {
	var totalErrors int64
	errorsByComponent := make(map[string]int64)
	errorsBySeverity := make(map[string]int64)
	var recoverableErrors, criticalErrors int64
	
	for _, metric := range metrics {
		if metric.Name == "errors_total" {
			totalErrors += int64(metric.Value)
			
			if component, ok := metric.Labels["component"]; ok {
				errorsByComponent[component] += int64(metric.Value)
			}
			
			if severity, ok := metric.Labels["severity"]; ok {
				errorsBySeverity[severity] += int64(metric.Value)
				
				switch severity {
				case "critical", "fatal":
					criticalErrors += int64(metric.Value)
				case "warning", "error":
					recoverableErrors += int64(metric.Value)
				}
			}
		}
	}
	
	return ErrorMetrics{
		TotalErrors:       totalErrors,
		ErrorsByComponent: errorsByComponent,
		ErrorsBySeverity:  errorsBySeverity,
		RecoverableErrors: recoverableErrors,
		CriticalErrors:    criticalErrors,
		ErrorTrends:       []ErrorTrendPoint{}, // Could be implemented with historical data
	}
}

// calculatePercentiles calculates P95 and P99 response times
func calculatePercentiles(times []time.Duration) (p95, p99 time.Duration) {
	if len(times) == 0 {
		return 0, 0
	}
	
	// Sort times (simplified implementation)
	sorted := make([]time.Duration, len(times))
	copy(sorted, times)
	
	// Simple bubble sort for demonstration
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}
	
	p95Index := int(float64(len(sorted)) * 0.95)
	p99Index := int(float64(len(sorted)) * 0.99)
	
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	if p99Index >= len(sorted) {
		p99Index = len(sorted) - 1
	}
	
	return sorted[p95Index], sorted[p99Index]
}

// Standard metric names for consistency across components
const (
	// Configuration metrics
	ConfigLoadDuration     = "config_load_duration_seconds"
	ConfigValidationErrors = "config_validation_errors_total"
	ConfigReloads         = "config_reloads_total"
	
	// Security metrics
	AuthenticationAttempts = "auth_attempts_total"
	AuthenticationFailures = "auth_failures_total"
	VulnerabilitiesFound  = "vulnerabilities_found_total"
	SecretAccessCount     = "secret_access_total"
	
	// HTTP metrics
	HTTPRequestDuration     = "http_request_duration_seconds"
	HTTPRequestsTotal       = "http_requests_total"
	HTTPErrorsTotal         = "http_errors_total"
	HTTPConnectionPoolSize  = "http_connection_pool_size"
	HTTPCircuitBreakerState = "http_circuit_breaker_state"
	
	// Trigger metrics
	TriggerExecutionDuration = "trigger_execution_duration_seconds"
	TriggerSuccessTotal      = "trigger_success_total"
	TriggerFailuresTotal     = "trigger_failures_total"
	TriggerRetryTotal        = "trigger_retry_attempts_total"
	
	// Business metrics
	BackupsCompleted      = "backups_completed_total"
	BackupDuration        = "backup_duration_seconds"
	BackupSizeBytes       = "backup_size_bytes"
	BackupResourcesCount  = "backup_resources_total"
	GitOpsGenerations     = "gitops_generations_total"
	GitOpsCommits         = "gitops_commits_total"
	GitOpsSyncDuration    = "gitops_sync_duration_seconds"
	PipelineExecutions    = "pipeline_executions_total"
	PipelineSuccessRate   = "pipeline_success_rate"
	PipelineEndToEndDuration = "pipeline_e2e_duration_seconds"
	
	// Health metrics
	ComponentHealth         = "component_health_status"
	ComponentUptime         = "component_uptime_seconds"
	ComponentLastRestart    = "component_last_restart_timestamp"
	DependencyHealthMetric  = "dependency_health_status"
	DependencyResponseTime  = "dependency_response_time_seconds"
	DependencyAvailability  = "dependency_availability_ratio"
	
	// Resource usage
	ResourceMemoryUsage = "resource_memory_usage_bytes"
	ResourceCPUUsage    = "resource_cpu_usage_ratio"
	ResourceDiskUsage   = "resource_disk_usage_bytes"
)