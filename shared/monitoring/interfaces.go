package monitoring

import (
	"context"
	"time"
)

// MetricsCollector defines the interface for collecting metrics across all components
type MetricsCollector interface {
	// Counter metrics
	IncCounter(name string, labels map[string]string, value float64)
	
	// Gauge metrics
	SetGauge(name string, labels map[string]string, value float64)
	
	// Histogram metrics
	RecordHistogram(name string, labels map[string]string, value float64)
	
	// Timing metrics
	RecordDuration(name string, labels map[string]string, duration time.Duration)
	
	// Custom metrics
	RecordMetric(metric Metric) error
	
	// Metric retrieval
	GetMetrics() []Metric
	ResetMetrics()
}

// HealthMonitor defines the interface for component health monitoring
type HealthMonitor interface {
	// Component health
	RegisterHealthCheck(name string, check HealthCheck)
	GetHealthStatus(component string) HealthStatus
	GetOverallHealth() OverallHealthStatus
	
	// Dependency health
	CheckDependency(name string, endpoint string) DependencyHealth
	GetDependencyStatus() map[string]DependencyHealth
	
	// Health check management
	StartHealthChecks(ctx context.Context) error
	StopHealthChecks() error
}

// EventPublisher defines the interface for publishing system events
type EventPublisher interface {
	// System events
	PublishSystemEvent(event SystemEvent) error
	
	// Business events
	PublishBusinessEvent(event BusinessEvent) error
	
	// Error events
	PublishErrorEvent(event ErrorEvent) error
	
	// Custom events
	PublishEvent(event Event) error
	
	// Event management
	SetExporter(exporter EventExporter) error
	Close() error
}

// Logger defines the standard logging interface for all components
type Logger interface {
	Debug(message string, fields map[string]interface{})
	Info(message string, fields map[string]interface{})
	Warn(message string, fields map[string]interface{})
	Error(message string, fields map[string]interface{})
	Fatal(message string, fields map[string]interface{})
	
	// Context-aware logging
	WithContext(ctx context.Context) Logger
	WithFields(fields map[string]interface{}) Logger
}

// MonitoringHub defines the central coordination interface for all monitoring
type MonitoringHub interface {
	// Component registration
	RegisterComponent(name string, component MonitoredComponent) error
	UnregisterComponent(name string) error
	
	// Metrics aggregation
	GetAggregatedMetrics() AggregatedMetrics
	
	// Health aggregation
	GetSystemHealth() SystemHealth
	
	// Event coordination
	BroadcastEvent(event Event) error
	
	// Configuration
	Configure(config MonitoringConfig) error
	Start(ctx context.Context) error
	Stop() error
}

// MonitoredComponent defines the interface that components must implement for monitoring
type MonitoredComponent interface {
	// Component identification
	GetComponentName() string
	GetComponentVersion() string
	
	// Metrics
	GetMetrics() map[string]interface{}
	ResetMetrics()
	
	// Health
	HealthCheck(ctx context.Context) HealthStatus
	GetDependencies() []string
	
	// Lifecycle hooks
	OnStart(ctx context.Context) error
	OnStop(ctx context.Context) error
}

// Core data types

// Metric represents a single metric measurement
type Metric struct {
	Name      string                 `json:"name"`
	Type      MetricType             `json:"type"`
	Value     float64                `json:"value"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// MetricType defines the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// HealthStatus represents the health of a component
type HealthStatus struct {
	Status      HealthStatusType       `json:"status"`
	Message     string                 `json:"message,omitempty"`
	LastCheck   time.Time              `json:"last_check"`
	CheckCount  int64                  `json:"check_count"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
	Duration    time.Duration          `json:"duration"`
}

// HealthStatusType defines health status levels
type HealthStatusType string

const (
	HealthStatusHealthy   HealthStatusType = "healthy"
	HealthStatusDegraded  HealthStatusType = "degraded"
	HealthStatusUnhealthy HealthStatusType = "unhealthy"
	HealthStatusUnknown   HealthStatusType = "unknown"
)

// OverallHealthStatus represents the overall system health
type OverallHealthStatus struct {
	Status      HealthStatusType           `json:"status"`
	Components  map[string]HealthStatus    `json:"components"`
	Dependencies map[string]DependencyHealth `json:"dependencies"`
	Summary     HealthSummary              `json:"summary"`
	Timestamp   time.Time                  `json:"timestamp"`
}

// HealthSummary provides a summary of health metrics
type HealthSummary struct {
	TotalComponents    int `json:"total_components"`
	HealthyComponents  int `json:"healthy_components"`
	DegradedComponents int `json:"degraded_components"`
	UnhealthyComponents int `json:"unhealthy_components"`
	TotalDependencies  int `json:"total_dependencies"`
	HealthyDependencies int `json:"healthy_dependencies"`
}

// DependencyHealth represents the health of an external dependency
type DependencyHealth struct {
	Name             string        `json:"name"`
	Status           HealthStatusType `json:"status"`
	ResponseTime     time.Duration `json:"response_time"`
	LastSuccess      time.Time     `json:"last_success"`
	LastFailure      time.Time     `json:"last_failure"`
	ConsecutiveFails int           `json:"consecutive_fails"`
	ErrorMessage     string        `json:"error_message,omitempty"`
	Endpoint         string        `json:"endpoint,omitempty"`
}

// HealthCheck defines a health check function
type HealthCheck func(ctx context.Context) HealthStatus

// Event represents a system event
type Event interface {
	GetID() string
	GetType() string
	GetTimestamp() time.Time
	GetMetadata() map[string]interface{}
}

// SystemEvent represents a system-level event
type SystemEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        string                 `json:"type"`
	Component   string                 `json:"component"`
	Action      string                 `json:"action"`
	Status      string                 `json:"status"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
	SpanID      string                 `json:"span_id,omitempty"`
}

func (e SystemEvent) GetID() string                            { return e.ID }
func (e SystemEvent) GetType() string                          { return e.Type }
func (e SystemEvent) GetTimestamp() time.Time                  { return e.Timestamp }
func (e SystemEvent) GetMetadata() map[string]interface{}      { return e.Metadata }

// BusinessEvent represents a business-level event
type BusinessEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        string                 `json:"type"`
	BusinessID  string                 `json:"business_id"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Metrics     map[string]float64     `json:"metrics,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (e BusinessEvent) GetID() string                         { return e.ID }
func (e BusinessEvent) GetType() string                       { return e.Type }
func (e BusinessEvent) GetTimestamp() time.Time               { return e.Timestamp }
func (e BusinessEvent) GetMetadata() map[string]interface{}   { return e.Metadata }

// ErrorEvent represents an error event
type ErrorEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Component   string                 `json:"component"`
	Operation   string                 `json:"operation"`
	Error       error                  `json:"error"`
	Severity    string                 `json:"severity"`
	Recoverable bool                   `json:"recoverable"`
	Context     map[string]interface{} `json:"context,omitempty"`
	StackTrace  string                 `json:"stack_trace,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
}

func (e ErrorEvent) GetID() string                            { return e.ID }
func (e ErrorEvent) GetType() string                          { return "error" }
func (e ErrorEvent) GetTimestamp() time.Time                  { return e.Timestamp }
func (e ErrorEvent) GetMetadata() map[string]interface{}      { return e.Context }

// AggregatedMetrics represents system-wide aggregated metrics
type AggregatedMetrics struct {
	Timestamp          time.Time                  `json:"timestamp"`
	ComponentMetrics   map[string][]Metric        `json:"component_metrics"`
	SystemMetrics      []Metric                   `json:"system_metrics"`
	PerformanceMetrics PerformanceMetrics         `json:"performance_metrics"`
	ErrorMetrics       ErrorMetrics               `json:"error_metrics"`
}

// PerformanceMetrics represents system performance metrics
type PerformanceMetrics struct {
	TotalRequests     int64         `json:"total_requests"`
	SuccessfulReqs    int64         `json:"successful_requests"`
	FailedReqs        int64         `json:"failed_requests"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
	Throughput        float64       `json:"throughput_rps"`
	ErrorRate         float64       `json:"error_rate"`
	P95ResponseTime   time.Duration `json:"p95_response_time"`
	P99ResponseTime   time.Duration `json:"p99_response_time"`
}

// ErrorMetrics represents system error metrics
type ErrorMetrics struct {
	TotalErrors        int64                  `json:"total_errors"`
	ErrorsByComponent  map[string]int64       `json:"errors_by_component"`
	ErrorsBySeverity   map[string]int64       `json:"errors_by_severity"`
	RecoverableErrors  int64                  `json:"recoverable_errors"`
	CriticalErrors     int64                  `json:"critical_errors"`
	ErrorTrends        []ErrorTrendPoint      `json:"error_trends"`
}

// ErrorTrendPoint represents a point in error trend analysis
type ErrorTrendPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	ErrorCount  int64     `json:"error_count"`
	Component   string    `json:"component"`
	ErrorType   string    `json:"error_type"`
}

// SystemHealth represents overall system health
type SystemHealth struct {
	Status             HealthStatusType       `json:"status"`
	OverallHealth      OverallHealthStatus    `json:"overall_health"`
	ComponentHealth    map[string]HealthStatus `json:"component_health"`
	DependencyHealth   map[string]DependencyHealth `json:"dependency_health"`
	SystemMetrics      SystemHealthMetrics    `json:"system_metrics"`
	Timestamp          time.Time              `json:"timestamp"`
}

// SystemHealthMetrics provides system-level health metrics
type SystemHealthMetrics struct {
	Uptime             time.Duration `json:"uptime"`
	MemoryUsage        float64       `json:"memory_usage_percent"`
	CPUUsage           float64       `json:"cpu_usage_percent"`
	DiskUsage          float64       `json:"disk_usage_percent"`
	NetworkLatency     time.Duration `json:"network_latency"`
	ActiveConnections  int64         `json:"active_connections"`
	GoroutineCount     int64         `json:"goroutine_count"`
}

// MonitoringConfig defines configuration for the monitoring system
type MonitoringConfig struct {
	// Collection settings
	MetricsEnabled     bool          `yaml:"metrics_enabled"`
	HealthEnabled      bool          `yaml:"health_enabled"`
	EventsEnabled      bool          `yaml:"events_enabled"`
	
	// Collection intervals
	MetricsInterval    time.Duration `yaml:"metrics_interval"`
	HealthInterval     time.Duration `yaml:"health_interval"`
	
	// Storage settings
	MetricsRetention   time.Duration `yaml:"metrics_retention"`
	EventsRetention    time.Duration `yaml:"events_retention"`
	
	// Export settings
	ExportEnabled      bool          `yaml:"export_enabled"`
	ExportInterval     time.Duration `yaml:"export_interval"`
	ExportEndpoint     string        `yaml:"export_endpoint"`
	
	// Alerting settings
	AlertingEnabled    bool          `yaml:"alerting_enabled"`
	AlertThresholds    map[string]float64 `yaml:"alert_thresholds"`
	
	// Performance settings
	MaxMetricsBuffer   int           `yaml:"max_metrics_buffer"`
	MaxEventsBuffer    int           `yaml:"max_events_buffer"`
}

// EventExporter defines the interface for exporting events
type EventExporter interface {
	ExportEvents(ctx context.Context, events []Event) error
	Configure(config ExporterConfig) error
	Close() error
}

// ExporterConfig defines configuration for exporters
type ExporterConfig struct {
	Type     string                 `yaml:"type"`
	Endpoint string                 `yaml:"endpoint"`
	Settings map[string]interface{} `yaml:"settings"`
}