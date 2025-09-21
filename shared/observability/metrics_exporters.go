package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// MetricsExporter defines the interface for metric exporters
type MetricsExporter interface {
	Export(metrics []MetricPoint) error
	Configure(config map[string]interface{}) error
	Name() string
	IsHealthy() bool
}

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	endpoint   string
	httpClient *http.Client
	registry   *PrometheusRegistry
	healthy    bool
}

// PrometheusRegistry manages Prometheus metric registrations
type PrometheusRegistry struct {
	metrics map[string]*PrometheusMetric
}

// PrometheusMetric represents a Prometheus metric
type PrometheusMetric struct {
	Name   string
	Type   string
	Help   string
	Labels []string
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter() *PrometheusExporter {
	return &PrometheusExporter{
		endpoint: ":9090/metrics",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		registry: &PrometheusRegistry{
			metrics: make(map[string]*PrometheusMetric),
		},
		healthy: true,
	}
}

// Export exports metrics in Prometheus format
func (e *PrometheusExporter) Export(metrics []MetricPoint) error {
	// Convert metrics to Prometheus format
	output := &bytes.Buffer{}
	
	for _, metric := range metrics {
		e.writeMetric(output, metric)
	}
	
	// Serve metrics or push to gateway
	// For now, we'll just write to a buffer
	// In production, this would expose an HTTP endpoint or push to Pushgateway
	
	return nil
}

// writeMetric writes a metric in Prometheus format
func (e *PrometheusExporter) writeMetric(output *bytes.Buffer, metric MetricPoint) {
	// Format metric name (replace dots with underscores)
	name := strings.ReplaceAll(metric.Name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	
	// Build labels
	labels := e.buildLabels(metric.Dimensions, metric.Tags)
	
	// Write metric based on type
	switch metric.Type {
	case MetricTypeCounter:
		fmt.Fprintf(output, "# TYPE %s counter\n", name)
		fmt.Fprintf(output, "%s%s %f %d\n", name, labels, metric.Value, metric.Timestamp.UnixMilli())
		
	case MetricTypeGauge:
		fmt.Fprintf(output, "# TYPE %s gauge\n", name)
		fmt.Fprintf(output, "%s%s %f %d\n", name, labels, metric.Value, metric.Timestamp.UnixMilli())
		
	case MetricTypeHistogram:
		fmt.Fprintf(output, "# TYPE %s histogram\n", name)
		fmt.Fprintf(output, "%s_sum%s %f %d\n", name, labels, metric.Value, metric.Timestamp.UnixMilli())
		fmt.Fprintf(output, "%s_count%s %f %d\n", name, labels, metric.Value, metric.Timestamp.UnixMilli())
		
	case MetricTypeSummary:
		fmt.Fprintf(output, "# TYPE %s summary\n", name)
		fmt.Fprintf(output, "%s_sum%s %f %d\n", name, labels, metric.Value, metric.Timestamp.UnixMilli())
		fmt.Fprintf(output, "%s_count%s %f %d\n", name, labels, metric.Value, metric.Timestamp.UnixMilli())
	}
}

// buildLabels builds Prometheus labels from dimensions and tags
func (e *PrometheusExporter) buildLabels(dimensions, tags map[string]string) string {
	if len(dimensions) == 0 && len(tags) == 0 {
		return ""
	}
	
	labels := []string{}
	
	// Add dimensions
	for k, v := range dimensions {
		labels = append(labels, fmt.Sprintf(`%s="%s"`, k, v))
	}
	
	// Add tags
	for k, v := range tags {
		labels = append(labels, fmt.Sprintf(`%s="%s"`, k, v))
	}
	
	return "{" + strings.Join(labels, ",") + "}"
}

// Configure configures the exporter
func (e *PrometheusExporter) Configure(config map[string]interface{}) error {
	if endpoint, ok := config["endpoint"].(string); ok {
		e.endpoint = endpoint
	}
	return nil
}

// Name returns the exporter name
func (e *PrometheusExporter) Name() string {
	return "prometheus"
}

// IsHealthy returns the health status
func (e *PrometheusExporter) IsHealthy() bool {
	return e.healthy
}

// OpenTelemetryExporter exports metrics using OpenTelemetry protocol
type OpenTelemetryExporter struct {
	endpoint   string
	httpClient *http.Client
	headers    map[string]string
	healthy    bool
}

// NewOpenTelemetryExporter creates a new OpenTelemetry exporter
func NewOpenTelemetryExporter() *OpenTelemetryExporter {
	return &OpenTelemetryExporter{
		endpoint: "http://localhost:4318/v1/metrics",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		headers: map[string]string{
			"Content-Type": "application/json",
		},
		healthy: true,
	}
}

// Export exports metrics using OpenTelemetry protocol
func (e *OpenTelemetryExporter) Export(metrics []MetricPoint) error {
	// Convert to OTLP format
	otlpMetrics := e.convertToOTLP(metrics)
	
	// Marshal to JSON
	body, err := json.Marshal(otlpMetrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}
	
	// Create request
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		e.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add headers
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}
	
	// Send request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.healthy = false
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		e.healthy = false
		return fmt.Errorf("server returned error: %d", resp.StatusCode)
	}
	
	e.healthy = true
	return nil
}

// convertToOTLP converts metrics to OpenTelemetry format
func (e *OpenTelemetryExporter) convertToOTLP(metrics []MetricPoint) map[string]interface{} {
	// Simplified OTLP format
	resourceMetrics := []map[string]interface{}{}
	
	for _, metric := range metrics {
		dataPoint := map[string]interface{}{
			"timeUnixNano": metric.Timestamp.UnixNano(),
			"value":        metric.Value,
			"attributes":   e.convertAttributes(metric.Dimensions, metric.Tags),
		}
		
		metricData := map[string]interface{}{
			"name": metric.Name,
			"unit": metric.Unit,
		}
		
		// Set metric type specific fields
		switch metric.Type {
		case MetricTypeCounter:
			metricData["sum"] = map[string]interface{}{
				"dataPoints":             []map[string]interface{}{dataPoint},
				"aggregationTemporality": "CUMULATIVE",
				"isMonotonic":            true,
			}
		case MetricTypeGauge:
			metricData["gauge"] = map[string]interface{}{
				"dataPoints": []map[string]interface{}{dataPoint},
			}
		case MetricTypeHistogram:
			metricData["histogram"] = map[string]interface{}{
				"dataPoints":             []map[string]interface{}{dataPoint},
				"aggregationTemporality": "CUMULATIVE",
			}
		}
		
		resourceMetrics = append(resourceMetrics, map[string]interface{}{
			"scopeMetrics": []map[string]interface{}{
				{
					"metrics": []map[string]interface{}{metricData},
				},
			},
		})
	}
	
	return map[string]interface{}{
		"resourceMetrics": resourceMetrics,
	}
}

// convertAttributes converts dimensions and tags to OTLP attributes
func (e *OpenTelemetryExporter) convertAttributes(dimensions, tags map[string]string) []map[string]interface{} {
	attributes := []map[string]interface{}{}
	
	for k, v := range dimensions {
		attributes = append(attributes, map[string]interface{}{
			"key": k,
			"value": map[string]interface{}{
				"stringValue": v,
			},
		})
	}
	
	for k, v := range tags {
		attributes = append(attributes, map[string]interface{}{
			"key": k,
			"value": map[string]interface{}{
				"stringValue": v,
			},
		})
	}
	
	return attributes
}

// Configure configures the exporter
func (e *OpenTelemetryExporter) Configure(config map[string]interface{}) error {
	if endpoint, ok := config["endpoint"].(string); ok {
		e.endpoint = endpoint
	}
	if headers, ok := config["headers"].(map[string]string); ok {
		e.headers = headers
	}
	return nil
}

// Name returns the exporter name
func (e *OpenTelemetryExporter) Name() string {
	return "opentelemetry"
}

// IsHealthy returns the health status
func (e *OpenTelemetryExporter) IsHealthy() bool {
	return e.healthy
}

// StatsDExporter exports metrics to StatsD
type StatsDExporter struct {
	address    string
	prefix     string
	conn       net.Conn
	tags       map[string]string
	healthy    bool
	tagFormat  string // "datadog" or "influxdb"
}

// NewStatsDExporter creates a new StatsD exporter
func NewStatsDExporter() *StatsDExporter {
	return &StatsDExporter{
		address:   "localhost:8125",
		prefix:    "kubernetes.backup.",
		tags:      make(map[string]string),
		tagFormat: "datadog", // Default to Datadog format
		healthy:   true,
	}
}

// Export exports metrics to StatsD
func (e *StatsDExporter) Export(metrics []MetricPoint) error {
	// Connect if not connected
	if e.conn == nil {
		conn, err := net.Dial("udp", e.address)
		if err != nil {
			e.healthy = false
			return fmt.Errorf("failed to connect to StatsD: %w", err)
		}
		e.conn = conn
	}
	
	// Send each metric
	for _, metric := range metrics {
		if err := e.sendMetric(metric); err != nil {
			// Log error but continue with other metrics
			fmt.Printf("Failed to send metric %s: %v\n", metric.Name, err)
		}
	}
	
	e.healthy = true
	return nil
}

// sendMetric sends a single metric to StatsD
func (e *StatsDExporter) sendMetric(metric MetricPoint) error {
	// Build metric name with prefix
	name := e.prefix + metric.Name
	
	// Build tags
	tags := e.buildTags(metric.Dimensions, metric.Tags)
	
	// Format metric based on type
	var metricStr string
	switch metric.Type {
	case MetricTypeCounter:
		metricStr = fmt.Sprintf("%s:%f|c%s", name, metric.Value, tags)
		
	case MetricTypeGauge:
		metricStr = fmt.Sprintf("%s:%f|g%s", name, metric.Value, tags)
		
	case MetricTypeHistogram, MetricTypeTimer:
		// StatsD uses ms for timers
		value := metric.Value
		if metric.Unit == "ms" || metric.Type == MetricTypeTimer {
			metricStr = fmt.Sprintf("%s:%f|ms%s", name, value, tags)
		} else {
			metricStr = fmt.Sprintf("%s:%f|h%s", name, value, tags)
		}
		
	case MetricTypeSummary:
		// StatsD doesn't have native summary, use gauge
		metricStr = fmt.Sprintf("%s:%f|g%s", name, metric.Value, tags)
	}
	
	// Send to StatsD
	_, err := e.conn.Write([]byte(metricStr))
	return err
}

// buildTags builds StatsD tags based on format
func (e *StatsDExporter) buildTags(dimensions, tags map[string]string) string {
	if len(dimensions) == 0 && len(tags) == 0 && len(e.tags) == 0 {
		return ""
	}
	
	allTags := make(map[string]string)
	
	// Merge global tags
	for k, v := range e.tags {
		allTags[k] = v
	}
	
	// Merge dimensions
	for k, v := range dimensions {
		allTags[k] = v
	}
	
	// Merge tags
	for k, v := range tags {
		allTags[k] = v
	}
	
	// Format based on tag format
	tagPairs := []string{}
	for k, v := range allTags {
		if e.tagFormat == "datadog" {
			tagPairs = append(tagPairs, fmt.Sprintf("%s:%s", k, v))
		} else { // influxdb format
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	
	if e.tagFormat == "datadog" {
		return "|#" + strings.Join(tagPairs, ",")
	}
	return "," + strings.Join(tagPairs, ",")
}

// Configure configures the exporter
func (e *StatsDExporter) Configure(config map[string]interface{}) error {
	if address, ok := config["address"].(string); ok {
		e.address = address
	}
	if prefix, ok := config["prefix"].(string); ok {
		e.prefix = prefix
	}
	if tagFormat, ok := config["tag_format"].(string); ok {
		e.tagFormat = tagFormat
	}
	if tags, ok := config["tags"].(map[string]string); ok {
		e.tags = tags
	}
	return nil
}

// Name returns the exporter name
func (e *StatsDExporter) Name() string {
	return "statsd"
}

// IsHealthy returns the health status
func (e *StatsDExporter) IsHealthy() bool {
	return e.healthy
}

// Close closes the connection
func (e *StatsDExporter) Close() error {
	if e.conn != nil {
		return e.conn.Close()
	}
	return nil
}