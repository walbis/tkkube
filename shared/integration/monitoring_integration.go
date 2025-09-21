package integration

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"shared-config/monitoring"
)

// MonitoringIntegration provides unified monitoring across all components
type MonitoringIntegration struct {
	bridge           *IntegrationBridge
	monitoringSystem *monitoring.MonitoringSystem
	collectors       map[string]monitoring.MetricsCollector
	healthChecks     map[string]monitoring.HealthCheck
	
	// Component metrics
	componentMetrics map[string]*ComponentMetrics
	
	// Aggregated metrics
	aggregator *IntegratedMetricsAggregator
	
	mu sync.RWMutex
}

// ComponentMetrics tracks metrics for a specific component
type ComponentMetrics struct {
	ComponentName    string
	RequestCount     int64
	ErrorCount       int64
	SuccessCount     int64
	AverageLatency   time.Duration
	LastRequestTime  time.Time
	HealthStatus     string
	Version          string
}

// IntegratedMetricsAggregator aggregates metrics across all components
type IntegratedMetricsAggregator struct {
	TotalRequests        int64                    `json:"total_requests"`
	TotalErrors          int64                    `json:"total_errors"`
	TotalSuccesses       int64                    `json:"total_successes"`
	AverageLatency       time.Duration            `json:"average_latency"`
	ComponentBreakdown   map[string]*ComponentMetrics `json:"component_breakdown"`
	HealthSummary        HealthSummary            `json:"health_summary"`
	IntegrationFlow      IntegrationFlowMetrics   `json:"integration_flow"`
	SystemPerformance    SystemPerformanceMetrics `json:"system_performance"`
}

// HealthSummary provides overall health status
type HealthSummary struct {
	OverallStatus       string `json:"overall_status"`
	HealthyComponents   int    `json:"healthy_components"`
	DegradedComponents  int    `json:"degraded_components"`
	UnhealthyComponents int    `json:"unhealthy_components"`
	TotalComponents     int    `json:"total_components"`
}

// IntegrationFlowMetrics tracks end-to-end integration performance
type IntegrationFlowMetrics struct {
	BackupToGitOpsLatency    time.Duration `json:"backup_to_gitops_latency"`
	TotalIntegrationRequests int64         `json:"total_integration_requests"`
	SuccessfulIntegrations   int64         `json:"successful_integrations"`
	FailedIntegrations       int64         `json:"failed_integrations"`
	AverageFlowDuration      time.Duration `json:"average_flow_duration"`
}

// SystemPerformanceMetrics tracks system-level performance
type SystemPerformanceMetrics struct {
	MemoryUsage     float64 `json:"memory_usage_percent"`
	CPUUsage        float64 `json:"cpu_usage_percent"`
	DiskUsage       float64 `json:"disk_usage_percent"`
	NetworkIOBytes  int64   `json:"network_io_bytes"`
	ActiveConnections int   `json:"active_connections"`
}

// NewMonitoringIntegration creates a new monitoring integration
func NewMonitoringIntegration(bridge *IntegrationBridge) *MonitoringIntegration {
	return &MonitoringIntegration{
		bridge:           bridge,
		monitoringSystem: bridge.monitoringSystem,
		collectors:       make(map[string]monitoring.MetricsCollector),
		healthChecks:     make(map[string]monitoring.HealthCheck),
		componentMetrics: make(map[string]*ComponentMetrics),
		aggregator:       &IntegratedMetricsAggregator{
			ComponentBreakdown: make(map[string]*ComponentMetrics),
		},
	}
}

// Start initializes monitoring integration
func (mi *MonitoringIntegration) Start(ctx context.Context) error {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	log.Printf("Starting monitoring integration...")

	// Register component health checks
	mi.registerComponentHealthChecks()

	// Register component metrics collectors
	mi.registerComponentMetricsCollectors()

	// Start background aggregation
	go mi.runMetricsAggregation(ctx)

	// Start health monitoring
	go mi.runHealthMonitoring(ctx)

	// Start integration flow monitoring
	go mi.runIntegrationFlowMonitoring(ctx)

	log.Printf("Monitoring integration started successfully")
	return nil
}

// Stop shuts down monitoring integration
func (mi *MonitoringIntegration) Stop() error {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	log.Printf("Stopping monitoring integration...")
	// Cleanup will be handled by context cancellation
	return nil
}

// registerComponentHealthChecks sets up health checks for all components
func (mi *MonitoringIntegration) registerComponentHealthChecks() {
	// Backup tool health check
	mi.healthChecks["backup-tool"] = func(ctx context.Context) monitoring.HealthStatus {
		status := mi.bridge.backupStatus
		if status.Name == "" {
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusUnknown,
				Message: "Backup tool not registered",
			}
		}

		switch status.Status {
		case "healthy":
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusHealthy,
				Message: "Backup tool is operational",
				Metrics: map[string]interface{}{
					"version":    status.Version,
					"last_check": status.LastCheck,
				},
			}
		case "degraded":
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusDegraded,
				Message: "Backup tool is experiencing issues",
				Metrics: map[string]interface{}{
					"version": status.Version,
					"error":   status.Metadata["error"],
				},
			}
		default:
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusUnhealthy,
				Message: "Backup tool is not responding",
			}
		}
	}

	// GitOps generator health check
	mi.healthChecks["gitops-generator"] = func(ctx context.Context) monitoring.HealthStatus {
		status := mi.bridge.gitopsStatus
		if status.Name == "" {
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusUnknown,
				Message: "GitOps generator not registered",
			}
		}

		switch status.Status {
		case "healthy":
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusHealthy,
				Message: "GitOps generator is operational",
				Metrics: map[string]interface{}{
					"version":    status.Version,
					"last_check": status.LastCheck,
				},
			}
		case "degraded":
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusDegraded,
				Message: "GitOps generator is experiencing issues",
				Metrics: map[string]interface{}{
					"version": status.Version,
					"error":   status.Metadata["error"],
				},
			}
		default:
			return monitoring.HealthStatus{
				Status:  monitoring.HealthStatusUnhealthy,
				Message: "GitOps generator is not responding",
			}
		}
	}

	// Register health checks with monitoring system
	for name, check := range mi.healthChecks {
		mi.monitoringSystem.GetMonitoringHub().GetHealthMonitor().RegisterHealthCheck(name, check)
	}
}

// registerComponentMetricsCollectors sets up metrics collection for components
func (mi *MonitoringIntegration) registerComponentMetricsCollectors() {
	// Create collectors for each component
	mi.collectors["backup-tool"] = mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
	mi.collectors["gitops-generator"] = mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
	mi.collectors["integration-bridge"] = mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()

	// Initialize component metrics
	mi.componentMetrics["backup-tool"] = &ComponentMetrics{
		ComponentName: "backup-tool",
		HealthStatus:  "unknown",
	}
	mi.componentMetrics["gitops-generator"] = &ComponentMetrics{
		ComponentName: "gitops-generator",
		HealthStatus:  "unknown",
	}
	mi.componentMetrics["integration-bridge"] = &ComponentMetrics{
		ComponentName: "integration-bridge",
		HealthStatus:  "healthy",
	}
}

// runMetricsAggregation runs background metrics aggregation
func (mi *MonitoringIntegration) runMetricsAggregation(ctx context.Context) {
	interval := 30 * time.Second // default fallback
	if mi.bridge != nil && mi.bridge.config != nil {
		interval = mi.bridge.config.Timeouts.MetricsCollectionInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mi.aggregateMetrics()
		}
	}
}

// runHealthMonitoring runs background health monitoring
func (mi *MonitoringIntegration) runHealthMonitoring(ctx context.Context) {
	interval := 15 * time.Second // default fallback
	if mi.bridge != nil && mi.bridge.config != nil {
		interval = mi.bridge.config.Timeouts.MonitoringInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mi.updateHealthSummary()
		}
	}
}

// runIntegrationFlowMonitoring monitors end-to-end integration flows
func (mi *MonitoringIntegration) runIntegrationFlowMonitoring(ctx context.Context) {
	interval := 60 * time.Second // default fallback
	if mi.bridge != nil && mi.bridge.config != nil {
		interval = mi.bridge.config.Timeouts.MetricsCollectionInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mi.updateIntegrationFlowMetrics()
		}
	}
}

// aggregateMetrics collects and aggregates metrics from all components
func (mi *MonitoringIntegration) aggregateMetrics() {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	// Get aggregated metrics from monitoring system
	_ = mi.monitoringSystem.GetAggregatedMetrics()

	// Update component metrics
	for componentName, metrics := range mi.componentMetrics {
		// Update from component status
		switch componentName {
		case "backup-tool":
			mi.updateComponentMetricsFromStatus(metrics, mi.bridge.backupStatus)
		case "gitops-generator":
			mi.updateComponentMetricsFromStatus(metrics, mi.bridge.gitopsStatus)
		case "integration-bridge":
			mi.updateComponentMetricsFromStatus(metrics, mi.bridge.bridgeStatus)
		}
	}

	// Calculate totals
	var totalRequests, totalErrors, totalSuccesses int64
	var totalLatency time.Duration
	var latencyCount int64

	for _, metrics := range mi.componentMetrics {
		totalRequests += metrics.RequestCount
		totalErrors += metrics.ErrorCount
		totalSuccesses += metrics.SuccessCount
		if metrics.AverageLatency > 0 {
			totalLatency += metrics.AverageLatency
			latencyCount++
		}
	}

	// Update aggregator
	mi.aggregator.TotalRequests = totalRequests
	mi.aggregator.TotalErrors = totalErrors
	mi.aggregator.TotalSuccesses = totalSuccesses
	if latencyCount > 0 {
		mi.aggregator.AverageLatency = totalLatency / time.Duration(latencyCount)
	}

	// Copy component breakdown
	for name, metrics := range mi.componentMetrics {
		mi.aggregator.ComponentBreakdown[name] = metrics
	}

	// Record aggregated metrics
	collector := mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
	collector.SetGauge("integration_total_requests", nil, float64(totalRequests))
	collector.SetGauge("integration_total_errors", nil, float64(totalErrors))
	collector.SetGauge("integration_total_successes", nil, float64(totalSuccesses))
	if totalRequests > 0 {
		successRate := float64(totalSuccesses) / float64(totalRequests) * 100
		collector.SetGauge("integration_success_rate_percent", nil, successRate)
	}
}

// updateComponentMetricsFromStatus updates component metrics from component status
func (mi *MonitoringIntegration) updateComponentMetricsFromStatus(metrics *ComponentMetrics, status ComponentStatus) {
	metrics.HealthStatus = status.Status
	metrics.Version = status.Version
	metrics.LastRequestTime = status.LastCheck

	// Extract metrics from metadata if available
	if status.Metadata != nil {
		if requestCount, ok := status.Metadata["request_count"].(int64); ok {
			metrics.RequestCount = requestCount
		}
		if errorCount, ok := status.Metadata["error_count"].(int64); ok {
			metrics.ErrorCount = errorCount
		}
		if successCount, ok := status.Metadata["success_count"].(int64); ok {
			metrics.SuccessCount = successCount
		}
		if avgLatency, ok := status.Metadata["average_latency"].(time.Duration); ok {
			metrics.AverageLatency = avgLatency
		}
	}
}

// updateHealthSummary updates the overall health summary
func (mi *MonitoringIntegration) updateHealthSummary() {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	var healthy, degraded, unhealthy, total int

	for _, metrics := range mi.componentMetrics {
		total++
		switch metrics.HealthStatus {
		case "healthy":
			healthy++
		case "degraded":
			degraded++
		case "unhealthy":
			unhealthy++
		}
	}

	// Determine overall status
	overallStatus := "healthy"
	if unhealthy > 0 {
		overallStatus = "unhealthy"
	} else if degraded > 0 {
		overallStatus = "degraded"
	}

	mi.aggregator.HealthSummary = HealthSummary{
		OverallStatus:       overallStatus,
		HealthyComponents:   healthy,
		DegradedComponents:  degraded,
		UnhealthyComponents: unhealthy,
		TotalComponents:     total,
	}

	// Record health metrics
	collector := mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
	collector.SetGauge("integration_healthy_components", nil, float64(healthy))
	collector.SetGauge("integration_degraded_components", nil, float64(degraded))
	collector.SetGauge("integration_unhealthy_components", nil, float64(unhealthy))
	collector.SetGauge("integration_total_components", nil, float64(total))
}

// updateIntegrationFlowMetrics updates end-to-end integration flow metrics
func (mi *MonitoringIntegration) updateIntegrationFlowMetrics() {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	// Get integration flow metrics from event bus and collectors
	// This would typically involve analyzing event timestamps and durations

	// For now, we'll use placeholder values that would be calculated from actual events
	// In a real implementation, these would be extracted from component metrics
	
	// Set placeholder values for integration flow metrics
	mi.aggregator.IntegrationFlow.TotalIntegrationRequests = 10  // Would come from actual trigger count
	mi.aggregator.IntegrationFlow.SuccessfulIntegrations = 9     // Would come from successful GitOps generations

	// Calculate failed integrations
	mi.aggregator.IntegrationFlow.FailedIntegrations = 
		mi.aggregator.IntegrationFlow.TotalIntegrationRequests - 
		mi.aggregator.IntegrationFlow.SuccessfulIntegrations

	// Get collector for recording metrics
	collector := mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()

	// Record flow metrics
	collector.SetGauge("integration_flow_total_requests", nil, 
		float64(mi.aggregator.IntegrationFlow.TotalIntegrationRequests))
	collector.SetGauge("integration_flow_successful", nil, 
		float64(mi.aggregator.IntegrationFlow.SuccessfulIntegrations))
	collector.SetGauge("integration_flow_failed", nil, 
		float64(mi.aggregator.IntegrationFlow.FailedIntegrations))
}

// GetAggregatedMetrics returns the current aggregated metrics
func (mi *MonitoringIntegration) GetAggregatedMetrics() *IntegratedMetricsAggregator {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	// Create a deep copy of the aggregator
	result := &IntegratedMetricsAggregator{
		TotalRequests:      mi.aggregator.TotalRequests,
		TotalErrors:        mi.aggregator.TotalErrors,
		TotalSuccesses:     mi.aggregator.TotalSuccesses,
		AverageLatency:     mi.aggregator.AverageLatency,
		ComponentBreakdown: make(map[string]*ComponentMetrics),
		HealthSummary:      mi.aggregator.HealthSummary,
		IntegrationFlow:    mi.aggregator.IntegrationFlow,
		SystemPerformance:  mi.aggregator.SystemPerformance,
	}

	// Copy component breakdown
	for name, metrics := range mi.aggregator.ComponentBreakdown {
		result.ComponentBreakdown[name] = &ComponentMetrics{
			ComponentName:   metrics.ComponentName,
			RequestCount:    metrics.RequestCount,
			ErrorCount:      metrics.ErrorCount,
			SuccessCount:    metrics.SuccessCount,
			AverageLatency:  metrics.AverageLatency,
			LastRequestTime: metrics.LastRequestTime,
			HealthStatus:    metrics.HealthStatus,
			Version:         metrics.Version,
		}
	}

	return result
}

// RecordIntegrationEvent records an integration flow event
func (mi *MonitoringIntegration) RecordIntegrationEvent(eventType string, duration time.Duration, success bool) {
	collector := mi.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
	
	// Record event duration
	collector.RecordDuration(fmt.Sprintf("integration_event_%s_duration", eventType), 
		map[string]string{"success": fmt.Sprintf("%t", success)}, duration)
	
	// Record event count
	status := "failure"
	if success {
		status = "success"
	}
	collector.IncCounter(fmt.Sprintf("integration_event_%s_total", eventType), 
		map[string]string{"status": status}, 1)
}

// GetComponentHealth returns health status for a specific component
func (mi *MonitoringIntegration) GetComponentHealth(componentName string) (*ComponentMetrics, error) {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	metrics, exists := mi.componentMetrics[componentName]
	if !exists {
		return nil, fmt.Errorf("component not found: %s", componentName)
	}

	// Return a copy
	return &ComponentMetrics{
		ComponentName:   metrics.ComponentName,
		RequestCount:    metrics.RequestCount,
		ErrorCount:      metrics.ErrorCount,
		SuccessCount:    metrics.SuccessCount,
		AverageLatency:  metrics.AverageLatency,
		LastRequestTime: metrics.LastRequestTime,
		HealthStatus:    metrics.HealthStatus,
		Version:         metrics.Version,
	}, nil
}

// GetOverallHealth returns the overall integration health status
func (mi *MonitoringIntegration) GetOverallHealth() HealthSummary {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	return mi.aggregator.HealthSummary
}