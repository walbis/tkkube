package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"shared-config/monitoring"
	sharedconfig "shared-config/config"
)

// ObservabilitySystem provides comprehensive observability for the backup/restore system
type ObservabilitySystem struct {
	config      *ObservabilityConfig
	metrics     *MetricsCollector
	tracing     *TracingSystem
	monitoring  *monitoring.MonitoringSystem
	
	// Dashboard and reporting
	dashboard   *ObservabilityDashboard
	reporter    *MetricsReporter
	
	// Internal state
	running     bool
	stopChan    chan struct{}
	mu          sync.RWMutex
}

// ObservabilityConfig defines configuration for the observability system
type ObservabilityConfig struct {
	// Metrics configuration
	Metrics MetricsConfig `json:"metrics"`
	
	// Tracing configuration
	Tracing TracingConfig `json:"tracing"`
	
	// Dashboard configuration
	Dashboard DashboardConfig `json:"dashboard"`
	
	// Reporting configuration
	Reporting ReportingConfig `json:"reporting"`
	
	// Integration settings
	Integration IntegrationConfig `json:"integration"`
}

// DashboardConfig defines dashboard settings
type DashboardConfig struct {
	Enabled         bool          `json:"enabled"`
	UpdateInterval  time.Duration `json:"update_interval"`
	Port            int           `json:"port"`
	MetricsToShow   []string      `json:"metrics_to_show"`
	AlertThresholds map[string]float64 `json:"alert_thresholds"`
}

// ReportingConfig defines reporting settings
type ReportingConfig struct {
	Enabled         bool          `json:"enabled"`
	ReportInterval  time.Duration `json:"report_interval"`
	ReportFormats   []string      `json:"report_formats"` // "json", "csv", "html"
	ReportTargets   []string      `json:"report_targets"` // "file", "email", "webhook"
	IncludeGraphs   bool          `json:"include_graphs"`
}

// IntegrationConfig defines integration settings
type IntegrationConfig struct {
	PrometheusEnabled    bool   `json:"prometheus_enabled"`
	PrometheusEndpoint   string `json:"prometheus_endpoint"`
	JaegerEnabled        bool   `json:"jaeger_enabled"`
	JaegerEndpoint       string `json:"jaeger_endpoint"`
	GrafanaEnabled       bool   `json:"grafana_enabled"`
	GrafanaAPIKey        string `json:"grafana_api_key"`
	AlertManagerEnabled  bool   `json:"alert_manager_enabled"`
	AlertManagerEndpoint string `json:"alert_manager_endpoint"`
}

// NewObservabilitySystem creates a comprehensive observability system
func NewObservabilitySystem(sharedConfig *sharedconfig.SharedConfig, monitoringSystem *monitoring.MonitoringSystem) *ObservabilitySystem {
	config := createObservabilityConfig(sharedConfig)
	
	os := &ObservabilitySystem{
		config:     config,
		metrics:    NewMetricsCollector(&config.Metrics),
		tracing:    NewTracingSystem(&config.Tracing),
		monitoring: monitoringSystem,
		dashboard:  NewObservabilityDashboard(&config.Dashboard),
		reporter:   NewMetricsReporter(&config.Reporting),
		stopChan:   make(chan struct{}),
	}
	
	return os
}

// createObservabilityConfig creates configuration from shared config
func createObservabilityConfig(sharedConfig *sharedconfig.SharedConfig) *ObservabilityConfig {
	config := &ObservabilityConfig{
		Metrics: MetricsConfig{
			CollectionInterval: 10 * time.Second,
			RetentionPeriod:    24 * time.Hour,
			MaxMetricsBuffer:   50000,
			DefaultGranularity: GranularityMedium,
			EnabledDimensions:  []string{"component", "operation", "status", "cluster"},
			AggregationWindows: []time.Duration{
				1 * time.Minute,
				5 * time.Minute,
				15 * time.Minute,
				1 * time.Hour,
			},
			PercentileLevels:    []float64{0.5, 0.75, 0.9, 0.95, 0.99},
			ExportInterval:      30 * time.Second,
			ExportBatchSize:     1000,
			EnablePrometheus:    true,
			EnableOpenTelemetry: true,
			EnableStatsD:        false,
		},
		Tracing: TracingConfig{
			SamplingRate:         0.1,
			SamplingType:         SamplingAdaptive,
			MaxSpansPerTrace:     1000,
			MaxAttributesPerSpan: 50,
			MaxEventsPerSpan:     20,
			MaxLinksPerSpan:      10,
			SpanRetention:        1 * time.Hour,
			ExportInterval:       10 * time.Second,
			ExportBatchSize:      100,
			PropagationFormat:    "w3c",
			ServiceName:          "backup-restore-system",
			ServiceVersion:       "1.0.0",
			Environment:          "production",
		},
		Dashboard: DashboardConfig{
			Enabled:        true,
			UpdateInterval: 5 * time.Second,
			Port:           9091,
			MetricsToShow: []string{
				"restore_operations_total",
				"restore_success_rate",
				"average_restore_duration",
				"backup_size_total",
				"error_rate",
				"system_health_score",
			},
			AlertThresholds: map[string]float64{
				"error_rate":                0.05,
				"restore_failure_rate":      0.1,
				"circuit_breaker_open_count": 3,
				"latency_p95":               5000,
			},
		},
		Reporting: ReportingConfig{
			Enabled:        true,
			ReportInterval: 1 * time.Hour,
			ReportFormats:  []string{"json", "html"},
			ReportTargets:  []string{"file"},
			IncludeGraphs:  true,
		},
		Integration: IntegrationConfig{
			PrometheusEnabled:    true,
			PrometheusEndpoint:   "localhost:9090",
			JaegerEnabled:        true,
			JaegerEndpoint:       "localhost:14268",
			GrafanaEnabled:       false,
			AlertManagerEnabled:  false,
		},
	}
	
	// Adjust based on shared config
	if sharedConfig != nil {
		// Use performance settings
		if sharedConfig.Performance.Limits.MaxConcurrentOperations > 0 {
			config.Metrics.MaxMetricsBuffer = sharedConfig.Performance.Limits.MaxConcurrentOperations * 500
		}
		
		// Adjust granularity based on environment
		if sharedConfig.Pipeline.Automation.Enabled {
			config.Metrics.DefaultGranularity = GranularityHigh
			config.Tracing.SamplingRate = 0.2 // Higher sampling for automation
		}
	}
	
	return config
}

// Start begins the observability system
func (os *ObservabilitySystem) Start(ctx context.Context) error {
	os.mu.Lock()
	defer os.mu.Unlock()
	
	if os.running {
		return fmt.Errorf("observability system already running")
	}
	
	// Start metrics collector
	if err := os.metrics.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}
	
	// Start tracing system
	if err := os.tracing.Start(ctx); err != nil {
		return fmt.Errorf("failed to start tracing system: %w", err)
	}
	
	// Start dashboard if enabled
	if os.config.Dashboard.Enabled {
		go os.dashboard.Start(ctx, os)
	}
	
	// Start reporter if enabled
	if os.config.Reporting.Enabled {
		go os.reporter.Start(ctx, os)
	}
	
	// Start main observability loop
	go os.run(ctx)
	
	os.running = true
	return nil
}

// Stop halts the observability system
func (os *ObservabilitySystem) Stop() error {
	os.mu.Lock()
	defer os.mu.Unlock()
	
	if !os.running {
		return nil
	}
	
	close(os.stopChan)
	
	// Stop components
	os.metrics.Stop()
	os.tracing.Stop()
	os.dashboard.Stop()
	os.reporter.Stop()
	
	os.running = false
	return nil
}

// RecordRestoreOperation records metrics for a restore operation
func (os *ObservabilitySystem) RecordRestoreOperation(
	ctx context.Context,
	operation string,
	cluster string,
	namespace string,
	status string,
	duration time.Duration,
	resourceCount int,
	errorCount int,
) {
	// Start trace span
	ctx, span := os.tracing.StartSpan(ctx, "restore."+operation,
		WithSpanKind(SpanKindServer),
		WithAttributes(map[string]interface{}{
			"cluster":        cluster,
			"namespace":      namespace,
			"status":         status,
			"resource_count": resourceCount,
			"error_count":    errorCount,
		}),
	)
	defer os.tracing.EndSpan(span)
	
	// Record metrics with dimensions
	dimensions := map[string]string{
		"operation": operation,
		"cluster":   cluster,
		"namespace": namespace,
		"status":    status,
	}
	
	// Operation count
	os.metrics.RecordCounter("restore_operations_total", 1, dimensions)
	
	// Duration
	os.metrics.RecordHistogram("restore_operation_duration_ms", float64(duration.Milliseconds()), dimensions)
	
	// Resources
	os.metrics.RecordGauge("restore_resources_processed", float64(resourceCount), dimensions)
	
	// Errors
	if errorCount > 0 {
		os.metrics.RecordCounter("restore_errors_total", int64(errorCount), dimensions)
		os.tracing.SetAttributes(span, map[string]interface{}{
			"error": true,
			"error_count": errorCount,
		})
	}
	
	// Success rate
	if status == "success" {
		os.metrics.RecordCounter("restore_success_total", 1, dimensions)
	} else {
		os.metrics.RecordCounter("restore_failure_total", 1, dimensions)
	}
	
	// Add trace events
	os.tracing.AddEvent(span, "operation_completed", map[string]interface{}{
		"duration_ms": duration.Milliseconds(),
		"status":      status,
	})
}

// RecordBackupMetrics records backup-related metrics
func (os *ObservabilitySystem) RecordBackupMetrics(
	backupID string,
	cluster string,
	size int64,
	duration time.Duration,
	resourceCount int,
	compressionRatio float64,
) {
	dimensions := map[string]string{
		"cluster": cluster,
	}
	
	// Backup size
	os.metrics.RecordGauge("backup_size_bytes", float64(size), dimensions)
	
	// Backup duration
	os.metrics.RecordHistogram("backup_duration_ms", float64(duration.Milliseconds()), dimensions)
	
	// Resource count
	os.metrics.RecordGauge("backup_resource_count", float64(resourceCount), dimensions)
	
	// Compression ratio
	os.metrics.RecordGauge("backup_compression_ratio", compressionRatio, dimensions)
	
	// Total backups
	os.metrics.RecordCounter("backups_total", 1, dimensions)
}

// RecordCircuitBreakerEvent records circuit breaker state changes
func (os *ObservabilitySystem) RecordCircuitBreakerEvent(
	service string,
	state string,
	reason string,
) {
	dimensions := map[string]string{
		"service": service,
		"state":   state,
		"reason":  reason,
	}
	
	// State change counter
	os.metrics.RecordCounter("circuit_breaker_state_changes", 1, dimensions)
	
	// Current state gauge
	stateValue := 0.0
	switch state {
	case "open":
		stateValue = 2.0
	case "half_open":
		stateValue = 1.0
	case "closed":
		stateValue = 0.0
	}
	os.metrics.RecordGauge("circuit_breaker_state", stateValue, dimensions)
	
	// Record event
	if state == "open" {
		os.metrics.RecordCounter("circuit_breaker_trips", 1, dimensions)
	}
}

// RecordSystemHealth records overall system health metrics
func (os *ObservabilitySystem) RecordSystemHealth(
	healthScore float64,
	componentStatuses map[string]string,
	activeOperations int,
	queuedOperations int,
) {
	// Overall health score
	os.metrics.RecordGauge("system_health_score", healthScore, nil)
	
	// Component statuses
	for component, status := range componentStatuses {
		dimensions := map[string]string{
			"component": component,
			"status":    status,
		}
		
		statusValue := 1.0
		if status != "healthy" {
			statusValue = 0.0
		}
		os.metrics.RecordGauge("component_health", statusValue, dimensions)
	}
	
	// Operation metrics
	os.metrics.RecordGauge("operations_active", float64(activeOperations), nil)
	os.metrics.RecordGauge("operations_queued", float64(queuedOperations), nil)
}

// SetMetricsGranularity adjusts the level of metrics detail
func (os *ObservabilitySystem) SetMetricsGranularity(granularity MetricsGranularity) {
	os.metrics.SetGranularity(granularity)
}

// GetCurrentMetrics returns current metrics snapshot
func (os *ObservabilitySystem) GetCurrentMetrics() map[string]interface{} {
	metrics := os.metrics.GetMetrics()
	
	// Add tracing metrics
	tracingMetrics := os.tracing.GetMetrics()
	metrics["tracing"] = map[string]interface{}{
		"spans_created":   tracingMetrics.SpansCreated,
		"spans_completed": tracingMetrics.SpansCompleted,
		"traces_active":   tracingMetrics.TracesActive,
		"error_rate":      tracingMetrics.ErrorRate,
		"average_latency": tracingMetrics.AverageLatency.String(),
	}
	
	// Add monitoring system metrics if available
	if os.monitoring != nil {
		monitoringMetrics := os.monitoring.GetHub().CollectMetrics()
		metrics["monitoring"] = monitoringMetrics
	}
	
	return metrics
}

// GetActiveTraces returns currently active traces
func (os *ObservabilitySystem) GetActiveTraces() []*Trace {
	traces := []*Trace{}
	for _, span := range os.tracing.GetActiveSpans() {
		if trace := os.tracing.GetTrace(span.TraceID); trace != nil {
			traces = append(traces, trace)
		}
	}
	return traces
}

// run is the main observability loop
func (os *ObservabilitySystem) run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-os.stopChan:
			return
		case <-ticker.C:
			os.performHealthCheck()
			os.adjustGranularity()
		}
	}
}

// performHealthCheck checks the health of observability components
func (os *ObservabilitySystem) performHealthCheck() {
	// Check metrics collector health
	metricsHealth := os.metrics.GetMetrics()
	if bufferUsage, ok := metricsHealth["_metadata"].(map[string]interface{})["buffer_usage"].(float64); ok {
		if bufferUsage > 0.9 {
			// High buffer usage - reduce granularity
			os.metrics.SetGranularity(GranularityLow)
		}
	}
	
	// Check tracing system health
	tracingMetrics := os.tracing.GetMetrics()
	if tracingMetrics.TracesActive > 10000 {
		// Too many active traces - adjust sampling
		// This would need to be implemented in the tracing system
	}
}

// adjustGranularity dynamically adjusts metrics granularity based on load
func (os *ObservabilitySystem) adjustGranularity() {
	if os.config.Metrics.DefaultGranularity == GranularityAdaptive {
		// Already handled by the metrics collector
		return
	}
	
	// Could implement additional logic here
}

// ObservabilityDashboard provides a real-time dashboard
type ObservabilityDashboard struct {
	config     *DashboardConfig
	server     *http.Server
	metrics    map[string]interface{}
	mu         sync.RWMutex
}

// NewObservabilityDashboard creates a new dashboard
func NewObservabilityDashboard(config *DashboardConfig) *ObservabilityDashboard {
	return &ObservabilityDashboard{
		config:  config,
		metrics: make(map[string]interface{}),
	}
}

// Start starts the dashboard server
func (d *ObservabilityDashboard) Start(ctx context.Context, os *ObservabilitySystem) {
	// Update metrics periodically
	go func() {
		ticker := time.NewTicker(d.config.UpdateInterval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.updateMetrics(os)
			}
		}
	}()
	
	// Serve dashboard (simplified - would serve actual UI)
	http.HandleFunc("/metrics", d.handleMetrics)
	d.server = &http.Server{
		Addr: fmt.Sprintf(":%d", d.config.Port),
	}
	
	go d.server.ListenAndServe()
}

// Stop stops the dashboard server
func (d *ObservabilityDashboard) Stop() {
	if d.server != nil {
		d.server.Shutdown(context.Background())
	}
}

// updateMetrics updates dashboard metrics
func (d *ObservabilityDashboard) updateMetrics(os *ObservabilitySystem) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.metrics = os.GetCurrentMetrics()
	
	// Check alert thresholds
	for metric, threshold := range d.config.AlertThresholds {
		if value, ok := d.metrics[metric].(float64); ok {
			if value > threshold {
				// Trigger alert
				fmt.Printf("ALERT: %s exceeded threshold: %f > %f\n", metric, value, threshold)
			}
		}
	}
}

// handleMetrics serves metrics endpoint
func (d *ObservabilityDashboard) handleMetrics(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.metrics)
}

// MetricsReporter generates periodic reports
type MetricsReporter struct {
	config   *ReportingConfig
	reports  []Report
	mu       sync.Mutex
}

// Report represents a metrics report
type Report struct {
	Timestamp time.Time              `json:"timestamp"`
	Period    time.Duration          `json:"period"`
	Metrics   map[string]interface{} `json:"metrics"`
	Summary   string                 `json:"summary"`
}

// NewMetricsReporter creates a new reporter
func NewMetricsReporter(config *ReportingConfig) *MetricsReporter {
	return &MetricsReporter{
		config:  config,
		reports: []Report{},
	}
}

// Start starts the reporter
func (r *MetricsReporter) Start(ctx context.Context, os *ObservabilitySystem) {
	ticker := time.NewTicker(r.config.ReportInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.generateReport(os)
		}
	}
}

// Stop stops the reporter
func (r *MetricsReporter) Stop() {
	// Clean shutdown
}

// generateReport generates a metrics report
func (r *MetricsReporter) generateReport(os *ObservabilitySystem) {
	report := Report{
		Timestamp: time.Now(),
		Period:    r.config.ReportInterval,
		Metrics:   os.GetCurrentMetrics(),
		Summary:   r.generateSummary(os.GetCurrentMetrics()),
	}
	
	r.mu.Lock()
	r.reports = append(r.reports, report)
	r.mu.Unlock()
	
	// Export report
	for _, format := range r.config.ReportFormats {
		r.exportReport(report, format)
	}
}

// generateSummary generates a report summary
func (r *MetricsReporter) generateSummary(metrics map[string]interface{}) string {
	// Generate summary based on metrics
	return fmt.Sprintf("System operational. Metrics collected: %d", len(metrics))
}

// exportReport exports a report in the specified format
func (r *MetricsReporter) exportReport(report Report, format string) {
	switch format {
	case "json":
		// Export as JSON
		data, _ := json.MarshalIndent(report, "", "  ")
		// Write to file or send to target
		_ = data
	case "html":
		// Generate HTML report
		// Would include graphs if configured
	case "csv":
		// Export as CSV
	}
}