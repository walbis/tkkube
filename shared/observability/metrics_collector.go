package observability

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"shared-config/monitoring"
)

// MetricsCollector provides comprehensive metrics collection with configurable granularity
type MetricsCollector struct {
	config      *MetricsConfig
	registry    *MetricsRegistry
	aggregators map[string]*MetricsAggregator
	exporters   []MetricsExporter
	
	// Granularity controls
	granularity MetricsGranularity
	dimensions  []MetricDimension
	
	// Performance metrics
	counters    map[string]*atomic.Int64
	gauges      map[string]*atomic.Value
	histograms  map[string]*Histogram
	summaries   map[string]*Summary
	
	// Internal state
	running     bool
	stopChan    chan struct{}
	mu          sync.RWMutex
}

// MetricsConfig defines configuration for metrics collection
type MetricsConfig struct {
	// Collection settings
	CollectionInterval time.Duration `json:"collection_interval"`
	RetentionPeriod    time.Duration `json:"retention_period"`
	MaxMetricsBuffer   int           `json:"max_metrics_buffer"`
	
	// Granularity settings
	DefaultGranularity MetricsGranularity `json:"default_granularity"`
	EnabledDimensions  []string           `json:"enabled_dimensions"`
	
	// Aggregation settings
	AggregationWindows []time.Duration `json:"aggregation_windows"`
	PercentileLevels   []float64       `json:"percentile_levels"`
	
	// Export settings
	ExportInterval     time.Duration `json:"export_interval"`
	ExportBatchSize    int           `json:"export_batch_size"`
	EnablePrometheus   bool          `json:"enable_prometheus"`
	EnableOpenTelemetry bool         `json:"enable_opentelemetry"`
	EnableStatsD       bool          `json:"enable_statsd"`
}

// MetricsGranularity defines the level of detail for metrics collection
type MetricsGranularity string

const (
	GranularityLow      MetricsGranularity = "low"      // Basic metrics only
	GranularityMedium   MetricsGranularity = "medium"   // Standard metrics with some detail
	GranularityHigh     MetricsGranularity = "high"     // Detailed metrics with all dimensions
	GranularityFull     MetricsGranularity = "full"     // Complete metrics with trace-level detail
	GranularityAdaptive MetricsGranularity = "adaptive" // Automatically adjust based on load
)

// MetricDimension represents a dimension for metrics categorization
type MetricDimension struct {
	Name        string                 `json:"name"`
	Values      []string               `json:"values"`
	Cardinality int                    `json:"cardinality"`
	Enabled     bool                   `json:"enabled"`
	Tags        map[string]string      `json:"tags"`
}

// MetricPoint represents a single metric measurement
type MetricPoint struct {
	Name       string                 `json:"name"`
	Type       MetricType             `json:"type"`
	Value      float64                `json:"value"`
	Timestamp  time.Time              `json:"timestamp"`
	Dimensions map[string]string      `json:"dimensions"`
	Tags       map[string]string      `json:"tags"`
	Unit       string                 `json:"unit"`
}

// MetricType defines the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
	MetricTypeTimer     MetricType = "timer"
)

// NewMetricsCollector creates a new metrics collector with advanced features
func NewMetricsCollector(config *MetricsConfig) *MetricsCollector {
	mc := &MetricsCollector{
		config:      config,
		registry:    NewMetricsRegistry(),
		aggregators: make(map[string]*MetricsAggregator),
		exporters:   []MetricsExporter{},
		granularity: config.DefaultGranularity,
		dimensions:  []MetricDimension{},
		counters:    make(map[string]*atomic.Int64),
		gauges:      make(map[string]*atomic.Value),
		histograms:  make(map[string]*Histogram),
		summaries:   make(map[string]*Summary),
		stopChan:    make(chan struct{}),
	}
	
	// Initialize default dimensions
	mc.initializeDefaultDimensions()
	
	// Initialize exporters based on configuration
	mc.initializeExporters()
	
	// Create aggregators for different windows
	for _, window := range config.AggregationWindows {
		mc.aggregators[window.String()] = NewMetricsAggregatorWithWindow(window)
	}
	
	return mc
}

// initializeDefaultDimensions sets up standard metric dimensions
func (mc *MetricsCollector) initializeDefaultDimensions() {
	mc.dimensions = []MetricDimension{
		{
			Name:        "component",
			Cardinality: 50,
			Enabled:     true,
			Tags:        map[string]string{"type": "system"},
		},
		{
			Name:        "operation",
			Cardinality: 100,
			Enabled:     true,
			Tags:        map[string]string{"type": "operational"},
		},
		{
			Name:        "namespace",
			Cardinality: 20,
			Enabled:     true,
			Tags:        map[string]string{"type": "kubernetes"},
		},
		{
			Name:        "cluster",
			Cardinality: 5,
			Enabled:     true,
			Tags:        map[string]string{"type": "infrastructure"},
		},
		{
			Name:        "status",
			Values:      []string{"success", "failure", "partial", "timeout", "cancelled"},
			Cardinality: 5,
			Enabled:     true,
			Tags:        map[string]string{"type": "result"},
		},
		{
			Name:        "error_type",
			Cardinality: 25,
			Enabled:     mc.granularity == GranularityHigh || mc.granularity == GranularityFull,
			Tags:        map[string]string{"type": "error"},
		},
		{
			Name:        "resource_type",
			Cardinality: 30,
			Enabled:     true,
			Tags:        map[string]string{"type": "kubernetes"},
		},
		{
			Name:        "phase",
			Values:      []string{"planning", "validation", "execution", "verification", "cleanup"},
			Cardinality: 5,
			Enabled:     mc.granularity != GranularityLow,
			Tags:        map[string]string{"type": "lifecycle"},
		},
	}
}

// initializeExporters sets up metric exporters based on configuration
func (mc *MetricsCollector) initializeExporters() {
	if mc.config.EnablePrometheus {
		mc.exporters = append(mc.exporters, NewPrometheusExporter())
	}
	
	if mc.config.EnableOpenTelemetry {
		mc.exporters = append(mc.exporters, NewOpenTelemetryExporter())
	}
	
	if mc.config.EnableStatsD {
		mc.exporters = append(mc.exporters, NewStatsDExporter())
	}
}

// Start begins metric collection
func (mc *MetricsCollector) Start(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if mc.running {
		return fmt.Errorf("metrics collector already running")
	}
	
	mc.running = true
	
	// Start collection goroutine
	go mc.runCollection(ctx)
	
	// Start aggregation goroutines
	for name, aggregator := range mc.aggregators {
		go mc.runAggregation(ctx, name, aggregator)
	}
	
	// Start export goroutine
	go mc.runExport(ctx)
	
	return nil
}

// Stop halts metric collection
func (mc *MetricsCollector) Stop() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if !mc.running {
		return nil
	}
	
	close(mc.stopChan)
	mc.running = false
	
	// Flush any remaining metrics
	mc.flush()
	
	return nil
}

// SetGranularity adjusts the level of metric detail
func (mc *MetricsCollector) SetGranularity(granularity MetricsGranularity) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	mc.granularity = granularity
	
	// Update dimension enablement based on granularity
	switch granularity {
	case GranularityLow:
		// Only essential dimensions
		for i := range mc.dimensions {
			mc.dimensions[i].Enabled = mc.dimensions[i].Name == "component" || 
				mc.dimensions[i].Name == "status"
		}
	case GranularityMedium:
		// Standard dimensions
		for i := range mc.dimensions {
			mc.dimensions[i].Enabled = mc.dimensions[i].Name != "error_type"
		}
	case GranularityHigh, GranularityFull:
		// All dimensions enabled
		for i := range mc.dimensions {
			mc.dimensions[i].Enabled = true
		}
	case GranularityAdaptive:
		// Will be adjusted dynamically based on load
		mc.enableAdaptiveGranularity()
	}
}

// RecordCounter increments a counter metric
func (mc *MetricsCollector) RecordCounter(name string, value int64, dimensions map[string]string) {
	key := mc.generateMetricKey(name, dimensions)
	
	counter, exists := mc.counters[key]
	if !exists {
		counter = &atomic.Int64{}
		mc.counters[key] = counter
		mc.registry.RegisterMetric(name, MetricTypeCounter, dimensions)
	}
	
	counter.Add(value)
	
	// Record in aggregators
	for _, agg := range mc.aggregators {
		agg.RecordValue(name, float64(value), dimensions)
	}
}

// RecordGauge sets a gauge metric value
func (mc *MetricsCollector) RecordGauge(name string, value float64, dimensions map[string]string) {
	key := mc.generateMetricKey(name, dimensions)
	
	gauge, exists := mc.gauges[key]
	if !exists {
		gauge = &atomic.Value{}
		mc.gauges[key] = gauge
		mc.registry.RegisterMetric(name, MetricTypeGauge, dimensions)
	}
	
	gauge.Store(value)
	
	// Record in aggregators
	for _, agg := range mc.aggregators {
		agg.RecordValue(name, value, dimensions)
	}
}

// RecordHistogram records a value in a histogram
func (mc *MetricsCollector) RecordHistogram(name string, value float64, dimensions map[string]string) {
	key := mc.generateMetricKey(name, dimensions)
	
	histogram, exists := mc.histograms[key]
	if !exists {
		histogram = NewHistogram(mc.config.PercentileLevels)
		mc.histograms[key] = histogram
		mc.registry.RegisterMetric(name, MetricTypeHistogram, dimensions)
	}
	
	histogram.Observe(value)
	
	// Record in aggregators
	for _, agg := range mc.aggregators {
		agg.RecordValue(name, value, dimensions)
	}
}

// RecordTimer records a timing measurement
func (mc *MetricsCollector) RecordTimer(name string, duration time.Duration, dimensions map[string]string) {
	mc.RecordHistogram(name+"_duration_ms", float64(duration.Milliseconds()), dimensions)
}

// RecordSummary records a value in a summary
func (mc *MetricsCollector) RecordSummary(name string, value float64, dimensions map[string]string) {
	key := mc.generateMetricKey(name, dimensions)
	
	summary, exists := mc.summaries[key]
	if !exists {
		summary = NewSummary(mc.config.PercentileLevels)
		mc.summaries[key] = summary
		mc.registry.RegisterMetric(name, MetricTypeSummary, dimensions)
	}
	
	summary.Observe(value)
}

// generateMetricKey creates a unique key for a metric
func (mc *MetricsCollector) generateMetricKey(name string, dimensions map[string]string) string {
	// Filter dimensions based on current granularity
	filteredDims := mc.filterDimensions(dimensions)
	
	key := name
	for dim, value := range filteredDims {
		key += fmt.Sprintf("_%s:%s", dim, value)
	}
	
	return key
}

// filterDimensions filters dimensions based on granularity settings
func (mc *MetricsCollector) filterDimensions(dimensions map[string]string) map[string]string {
	filtered := make(map[string]string)
	
	for _, dim := range mc.dimensions {
		if dim.Enabled {
			if value, exists := dimensions[dim.Name]; exists {
				filtered[dim.Name] = value
			}
		}
	}
	
	return filtered
}

// enableAdaptiveGranularity sets up adaptive granularity adjustment
func (mc *MetricsCollector) enableAdaptiveGranularity() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				mc.adjustGranularityBasedOnLoad()
			case <-mc.stopChan:
				return
			}
		}
	}()
}

// adjustGranularityBasedOnLoad dynamically adjusts granularity based on system load
func (mc *MetricsCollector) adjustGranularityBasedOnLoad() {
	// Get current metrics load
	metricCount := mc.registry.GetMetricCount()
	bufferUsage := float64(len(mc.counters)+len(mc.gauges)) / float64(mc.config.MaxMetricsBuffer)
	
	// Adjust granularity based on load
	if bufferUsage > 0.8 || metricCount > 10000 {
		// High load - reduce granularity
		mc.SetGranularity(GranularityLow)
	} else if bufferUsage > 0.5 || metricCount > 5000 {
		// Medium load - use medium granularity
		mc.SetGranularity(GranularityMedium)
	} else {
		// Low load - can use high granularity
		mc.SetGranularity(GranularityHigh)
	}
}

// runCollection handles periodic metric collection
func (mc *MetricsCollector) runCollection(ctx context.Context) {
	ticker := time.NewTicker(mc.config.CollectionInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopChan:
			return
		case <-ticker.C:
			mc.collectMetrics()
		}
	}
}

// collectMetrics gathers current metric values
func (mc *MetricsCollector) collectMetrics() {
	timestamp := time.Now()
	
	// Collect counter values
	for key, counter := range mc.counters {
		value := counter.Load()
		mc.recordMetricPoint(key, MetricTypeCounter, float64(value), timestamp)
	}
	
	// Collect gauge values
	for key, gauge := range mc.gauges {
		if value, ok := gauge.Load().(float64); ok {
			mc.recordMetricPoint(key, MetricTypeGauge, value, timestamp)
		}
	}
	
	// Collect histogram snapshots
	for key, histogram := range mc.histograms {
		snapshot := histogram.Snapshot()
		mc.recordHistogramSnapshot(key, snapshot, timestamp)
	}
	
	// Collect summary snapshots
	for key, summary := range mc.summaries {
		snapshot := summary.Snapshot()
		mc.recordSummarySnapshot(key, snapshot, timestamp)
	}
}

// recordMetricPoint records a single metric point
func (mc *MetricsCollector) recordMetricPoint(key string, metricType MetricType, value float64, timestamp time.Time) {
	point := MetricPoint{
		Name:      key,
		Type:      metricType,
		Value:     value,
		Timestamp: timestamp,
	}
	
	// Store in registry
	mc.registry.AddMetricPoint(point)
}

// recordHistogramSnapshot records histogram statistics
func (mc *MetricsCollector) recordHistogramSnapshot(key string, snapshot *HistogramSnapshot, timestamp time.Time) {
	// Record percentiles
	for percentile, value := range snapshot.Percentiles {
		mc.recordMetricPoint(
			fmt.Sprintf("%s_p%d", key, int(percentile*100)),
			MetricTypeGauge,
			value,
			timestamp,
		)
	}
	
	// Record mean and count
	mc.recordMetricPoint(key+"_mean", MetricTypeGauge, snapshot.Mean, timestamp)
	mc.recordMetricPoint(key+"_count", MetricTypeCounter, float64(snapshot.Count), timestamp)
}

// recordSummarySnapshot records summary statistics
func (mc *MetricsCollector) recordSummarySnapshot(key string, snapshot *SummarySnapshot, timestamp time.Time) {
	// Record quantiles
	for quantile, value := range snapshot.Quantiles {
		mc.recordMetricPoint(
			fmt.Sprintf("%s_q%d", key, int(quantile*100)),
			MetricTypeGauge,
			value,
			timestamp,
		)
	}
	
	// Record sum and count
	mc.recordMetricPoint(key+"_sum", MetricTypeGauge, snapshot.Sum, timestamp)
	mc.recordMetricPoint(key+"_count", MetricTypeCounter, float64(snapshot.Count), timestamp)
}

// runAggregation handles metric aggregation for a specific window
func (mc *MetricsCollector) runAggregation(ctx context.Context, name string, aggregator *MetricsAggregator) {
	ticker := time.NewTicker(aggregator.window)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopChan:
			return
		case <-ticker.C:
			aggregator.Aggregate()
		}
	}
}

// runExport handles periodic metric export
func (mc *MetricsCollector) runExport(ctx context.Context) {
	ticker := time.NewTicker(mc.config.ExportInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopChan:
			return
		case <-ticker.C:
			mc.exportMetrics()
		}
	}
}

// exportMetrics exports metrics to configured exporters
func (mc *MetricsCollector) exportMetrics() {
	// Get metrics to export
	metrics := mc.registry.GetMetricsForExport(mc.config.ExportBatchSize)
	
	// Export to each configured exporter
	for _, exporter := range mc.exporters {
		if err := exporter.Export(metrics); err != nil {
			// Log export error but continue with other exporters
			fmt.Printf("Failed to export metrics: %v\n", err)
		}
	}
}

// flush exports any remaining metrics
func (mc *MetricsCollector) flush() {
	// Collect final metrics
	mc.collectMetrics()
	
	// Export all remaining metrics
	metrics := mc.registry.GetAllMetrics()
	for _, exporter := range mc.exporters {
		exporter.Export(metrics)
	}
}

// GetMetrics returns current metrics snapshot
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	metrics := make(map[string]interface{})
	
	// Add counter values
	for key, counter := range mc.counters {
		metrics[key] = counter.Load()
	}
	
	// Add gauge values
	for key, gauge := range mc.gauges {
		if value, ok := gauge.Load().(float64); ok {
			metrics[key] = value
		}
	}
	
	// Add histogram statistics
	for key, histogram := range mc.histograms {
		snapshot := histogram.Snapshot()
		metrics[key] = map[string]interface{}{
			"count":       snapshot.Count,
			"mean":        snapshot.Mean,
			"percentiles": snapshot.Percentiles,
		}
	}
	
	// Add summary statistics
	for key, summary := range mc.summaries {
		snapshot := summary.Snapshot()
		metrics[key] = map[string]interface{}{
			"count":     snapshot.Count,
			"sum":       snapshot.Sum,
			"quantiles": snapshot.Quantiles,
		}
	}
	
	// Add metadata
	metrics["_metadata"] = map[string]interface{}{
		"granularity":   mc.granularity,
		"metric_count":  mc.registry.GetMetricCount(),
		"buffer_usage":  float64(len(mc.counters)+len(mc.gauges)) / float64(mc.config.MaxMetricsBuffer),
		"dimensions":    mc.getEnabledDimensions(),
	}
	
	return metrics
}

// getEnabledDimensions returns list of currently enabled dimensions
func (mc *MetricsCollector) getEnabledDimensions() []string {
	enabled := []string{}
	for _, dim := range mc.dimensions {
		if dim.Enabled {
			enabled = append(enabled, dim.Name)
		}
	}
	return enabled
}

// GetCollector implements monitoring.MetricsCollector interface
func (mc *MetricsCollector) GetCollector() monitoring.MetricsCollector {
	// Return a wrapper that implements the monitoring.MetricsCollector interface
	return &metricsCollectorWrapper{mc: mc}
}

// metricsCollectorWrapper wraps MetricsCollector to implement monitoring.MetricsCollector
type metricsCollectorWrapper struct {
	mc *MetricsCollector
}

// CollectMetrics implements monitoring.MetricsCollector
func (w *metricsCollectorWrapper) CollectMetrics() map[string]interface{} {
	return w.mc.GetMetrics()
}

// RecordDuration implements monitoring.MetricsCollector
func (w *metricsCollectorWrapper) RecordDuration(name string, duration time.Duration) {
	w.mc.RecordTimer(name, duration, nil)
}

// RecordCount implements monitoring.MetricsCollector
func (w *metricsCollectorWrapper) RecordCount(name string, count int) {
	w.mc.RecordCounter(name, int64(count), nil)
}

// RecordValue implements monitoring.MetricsCollector
func (w *metricsCollectorWrapper) RecordValue(name string, value float64) {
	w.mc.RecordGauge(name, value, nil)
}