# Monitoring System Integration Guide

This guide explains how to integrate and use the new monitoring system with the shared configuration framework.

## Overview

The monitoring system provides comprehensive observability for the backup and GitOps tools through:

- **Metrics Collection**: Performance metrics, error rates, business metrics
- **Health Monitoring**: Component health checks and dependency monitoring  
- **Event Publishing**: System events, business events, and error events
- **Centralized Hub**: Coordinated monitoring across all components

## Quick Start

### 1. Basic Integration

```go
package main

import (
    "context"
    "log"
    
    "shared-config/monitoring"
)

func main() {
    ctx := context.Background()
    
    // Initialize monitoring with auto-detection
    initializer := monitoring.NewMonitoringInitializer()
    system, err := initializer.StartWithAutoSetup(ctx)
    if err != nil {
        log.Fatalf("Failed to start monitoring: %v", err)
    }
    defer initializer.Cleanup()
    
    // Your application code here
    log.Println("Application running with monitoring enabled")
    
    // View system health
    health := system.GetSystemHealth()
    log.Printf("System health: %s", health.Status)
}
```

### 2. Integration with Existing Components

```go
// HTTP Client Integration
import "shared-config/http"

func setupHTTPClient() {
    logger := monitoring.NewLogger("my_app")
    
    // Create monitored HTTP client
    config := http.DefaultHTTPClientConfig()
    client := http.NewMonitoredHTTPClient(config, "api", logger)
    
    // Use the client normally - monitoring happens automatically
    resp, err := client.Get(ctx, "https://api.example.com/health")
}

// Config Validation Integration  
import "shared-config/config"

func validateConfig(cfg *config.SharedConfig) {
    logger := monitoring.NewLogger("config")
    
    // Create monitored validator
    validator := config.NewMonitoredConfigValidator(cfg, logger)
    
    // Validation with automatic monitoring
    result := validator.Validate()
    if !result.Valid {
        log.Printf("Config validation failed: %d errors", len(result.Errors))
    }
}

// Trigger System Integration
import "shared-config/triggers"

func setupTriggers(cfg *config.SharedConfig) {
    logger := monitoring.NewLogger("triggers")
    
    // Create monitored trigger
    trigger := triggers.NewMonitoredAutoTrigger(cfg, logger)
    
    // Trigger with monitoring
    event := &triggers.BackupCompletionEvent{
        BackupID:    "backup-123",
        ClusterName: "prod-cluster",
        // ... other fields
    }
    
    result, err := trigger.TriggerGitOpsGeneration(ctx, event)
}
```

## Advanced Usage

### Custom Component Monitoring

To add monitoring to your own components, implement the `MonitoredComponent` interface:

```go
type MyComponent struct {
    metricsCollector monitoring.MetricsCollector
    logger          monitoring.Logger
    // ... your component fields
}

func NewMyComponent(logger monitoring.Logger) *MyComponent {
    config := &monitoring.MonitoringConfig{
        MetricsEnabled: true,
        EventsEnabled:  true,
    }
    
    return &MyComponent{
        metricsCollector: monitoring.NewMetricsCollector(config),
        logger:          logger.WithFields(map[string]interface{}{"component": "my_component"}),
    }
}

// Implement MonitoredComponent interface
func (mc *MyComponent) GetComponentName() string {
    return "my_component"
}

func (mc *MyComponent) GetComponentVersion() string {
    return "1.0.0"
}

func (mc *MyComponent) GetMetrics() map[string]interface{} {
    // Return your component's metrics
    return map[string]interface{}{
        "operations_total": 42,
        "errors_total":     0,
    }
}

func (mc *MyComponent) HealthCheck(ctx context.Context) monitoring.HealthStatus {
    // Implement your health check logic
    return monitoring.HealthStatus{
        Status:  monitoring.HealthStatusHealthy,
        Message: "Component operating normally",
    }
}

func (mc *MyComponent) GetDependencies() []string {
    return []string{"database", "external_api"}
}

func (mc *MyComponent) OnStart(ctx context.Context) error {
    mc.logger.Info("component_starting", nil)
    return nil
}

func (mc *MyComponent) OnStop(ctx context.Context) error {
    mc.logger.Info("component_stopping", nil)
    return nil
}

// Register with monitoring system
func registerComponent(system *monitoring.MonitoringSystem) {
    logger := monitoring.NewLogger("my_app")
    component := NewMyComponent(logger)
    
    hub := system.GetMonitoringHub()
    err := hub.RegisterComponent("my_component", component)
    if err != nil {
        log.Printf("Failed to register component: %v", err)
    }
}
```

### Custom Metrics

```go
func recordCustomMetrics(collector monitoring.MetricsCollector) {
    // Counter metrics
    labels := map[string]string{"operation": "process_backup"}
    collector.IncCounter("operations_total", labels, 1)
    
    // Gauge metrics
    collector.SetGauge("active_connections", nil, 25)
    
    // Duration metrics
    start := time.Now()
    // ... do work
    duration := time.Since(start)
    collector.RecordDuration("operation_duration", labels, duration)
    
    // Custom metrics
    metric := monitoring.Metric{
        Name:      "custom_business_metric",
        Type:      monitoring.MetricTypeGauge,
        Value:     123.45,
        Labels:    map[string]string{"region": "us-east-1"},
        Timestamp: time.Now(),
    }
    collector.RecordMetric(metric)
}
```

### Custom Events

```go
func publishCustomEvents(publisher monitoring.EventPublisher) {
    // System event
    systemEvent := monitoring.SystemEvent{
        ID:        "evt_123",
        Timestamp: time.Now(),
        Type:      "custom_operation",
        Component: "my_component",
        Action:    "process_data",
        Status:    "success",
        Duration:  time.Second * 2,
    }
    publisher.PublishSystemEvent(systemEvent)
    
    // Business event
    businessEvent := monitoring.BusinessEvent{
        ID:          "biz_456",
        Timestamp:   time.Now(),
        Type:        "data_processed",
        BusinessID:  "batch_789",
        Description: "Successfully processed data batch",
        Impact:      "positive",
        Metrics:     map[string]float64{"records_processed": 1000},
    }
    publisher.PublishBusinessEvent(businessEvent)
    
    // Error event
    err := fmt.Errorf("something went wrong")
    errorEvent := monitoring.CreateErrorEvent("my_component", "process_data", err, "error")
    publisher.PublishErrorEvent(errorEvent)
}
```

### Health Checks

```go
// HTTP endpoint health check
func setupHTTPHealthCheck(monitor monitoring.HealthMonitor) {
    healthCheck := monitoring.HTTPHealthCheck("https://api.example.com/health")
    monitor.RegisterHealthCheck("external_api", healthCheck)
}

// Custom health check
func setupCustomHealthCheck(monitor monitoring.HealthMonitor) {
    healthCheck := func(ctx context.Context) monitoring.HealthStatus {
        // Your custom health check logic
        if isComponentHealthy() {
            return monitoring.HealthStatus{
                Status:  monitoring.HealthStatusHealthy,
                Message: "All systems operational",
            }
        }
        return monitoring.HealthStatus{
            Status:  monitoring.HealthStatusUnhealthy,
            Message: "Component experiencing issues",
        }
    }
    
    monitor.RegisterHealthCheck("my_component", healthCheck)
}

// Dependency monitoring
func setupDependencyMonitoring(monitor monitoring.HealthMonitor) {
    // Register HTTP dependency
    if hm, ok := monitor.(*monitoring.DefaultHealthMonitor); ok {
        hm.RegisterDependency("database", "tcp://db.example.com:5432", "database")
        hm.RegisterDependency("redis", "tcp://redis.example.com:6379", "cache")
    }
}
```

## Configuration

### Monitoring Configuration

```yaml
# monitoring section in shared-config.yaml (conceptual)
monitoring:
  enabled: true
  metrics:
    enabled: true
    interval: 30s
    retention: 24h
    buffer_size: 10000
  health:
    enabled: true
    interval: 60s
    dependency_timeout: 15s
  events:
    enabled: true
    retention: 168h  # 7 days
    buffer_size: 5000
  export:
    enabled: true
    interval: 5m
    endpoint: "http://metrics.example.com/webhook"
  alerting:
    enabled: false
    thresholds:
      error_rate: 0.05
      response_time_p95: 5000
```

### Environment Variables

The monitoring system respects these environment variables:

```bash
# Logging level
LOG_LEVEL=info

# Monitoring settings
MONITORING_ENABLED=true
METRICS_INTERVAL=30s
HEALTH_INTERVAL=60s

# Export settings  
EXPORT_ENDPOINT=http://monitoring.example.com/webhook
EXPORT_ENABLED=true

# Buffer sizes
MAX_METRICS_BUFFER=10000
MAX_EVENTS_BUFFER=5000
```

## Metrics Reference

### Standard Metrics

**Configuration Metrics:**
- `config_load_duration_seconds` - Time to load configuration
- `config_validation_errors_total` - Number of validation errors
- `config_reloads_total` - Configuration reload count

**HTTP Metrics:**
- `http_requests_total` - Total HTTP requests (labeled by method, status)
- `http_request_duration_seconds` - Request duration (labeled by method)
- `http_errors_total` - HTTP errors (labeled by method, error_type)
- `http_connection_pool_size` - Active connections in pool

**Trigger Metrics:**
- `trigger_execution_duration_seconds` - Trigger execution time
- `trigger_success_total` - Successful triggers (labeled by method)
- `trigger_failures_total` - Failed triggers (labeled by method)
- `trigger_retry_attempts_total` - Retry attempts

**Business Metrics:**
- `backups_completed_total` - Completed backups
- `backup_duration_seconds` - Backup execution time
- `gitops_generations_total` - GitOps generations triggered
- `pipeline_executions_total` - Pipeline executions

### Custom Labels

All metrics support custom labels for filtering and aggregation:

```go
labels := map[string]string{
    "cluster":     "prod-east",
    "namespace":   "default", 
    "operation":   "backup",
    "environment": "production",
}
collector.IncCounter("operations_total", labels, 1)
```

## Event Types

### System Events

- `component_registered` - Component registered with monitoring
- `component_started` - Component started
- `component_stopped` - Component stopped
- `config_loaded` - Configuration loaded
- `http_request_start` - HTTP request initiated
- `http_request_complete` - HTTP request completed
- `trigger_gitops_start` - GitOps trigger started

### Business Events

- `backup_completed` - Backup operation completed
- `gitops_generation_completed` - GitOps manifests generated
- `pipeline_executed` - Pipeline execution finished

### Error Events

- `config_validation_error` - Configuration validation failed
- `http_request_error` - HTTP request failed
- `trigger_execution_error` - Trigger execution failed

## Best Practices

### 1. Component Integration

```go
// Always integrate monitoring when creating components
func NewMyService(config *Config) *MyService {
    logger := monitoring.NewLogger("my_service")
    
    return &MyService{
        logger: logger,
        metrics: monitoring.NewMetricsCollector(&monitoring.MonitoringConfig{}),
        // ... other fields
    }
}
```

### 2. Error Handling

```go
func (s *MyService) ProcessData(data []byte) error {
    start := time.Now()
    
    err := s.doProcessing(data)
    duration := time.Since(start)
    
    labels := map[string]string{"operation": "process_data"}
    
    if err != nil {
        s.logger.Error("processing_failed", map[string]interface{}{
            "error": err.Error(),
            "duration_ms": duration.Milliseconds(),
        })
        s.metrics.IncCounter("errors_total", labels, 1)
        return err
    }
    
    s.logger.Info("processing_success", map[string]interface{}{
        "duration_ms": duration.Milliseconds(),
    })
    s.metrics.IncCounter("operations_total", labels, 1)
    s.metrics.RecordDuration("operation_duration", labels, duration)
    
    return nil
}
```

### 3. Health Check Implementation

```go
func (s *MyService) HealthCheck(ctx context.Context) monitoring.HealthStatus {
    // Check multiple aspects of component health
    
    // 1. Check dependencies
    if !s.isDatabaseConnected() {
        return monitoring.HealthStatus{
            Status:  monitoring.HealthStatusUnhealthy,
            Message: "Database connection lost",
        }
    }
    
    // 2. Check recent error rates
    errorRate := s.getRecentErrorRate()
    if errorRate > 0.1 {
        return monitoring.HealthStatus{
            Status:  monitoring.HealthStatusDegraded,
            Message: fmt.Sprintf("High error rate: %.1f%%", errorRate*100),
        }
    }
    
    // 3. Check resource usage
    if s.getMemoryUsage() > 0.9 {
        return monitoring.HealthStatus{
            Status:  monitoring.HealthStatusDegraded,
            Message: "High memory usage",
        }
    }
    
    return monitoring.HealthStatus{
        Status:  monitoring.HealthStatusHealthy,
        Message: "All systems operational",
    }
}
```

### 4. Metric Naming

Follow consistent naming conventions:

```go
// Good
collector.IncCounter("http_requests_total", labels, 1)
collector.RecordDuration("http_request_duration_seconds", labels, duration)
collector.SetGauge("active_connections_current", labels, count)

// Avoid
collector.IncCounter("requests", labels, 1)  // Not descriptive
collector.IncCounter("HttpRequestsTotal", labels, 1)  // Wrong case
collector.RecordDuration("request_time", labels, duration)  // Missing units
```

## Troubleshooting

### Common Issues

1. **Metrics not appearing**: Check that the component is registered with the monitoring hub
2. **High memory usage**: Reduce buffer sizes in monitoring configuration
3. **Missing health checks**: Ensure components implement health check methods
4. **Event export failures**: Verify export endpoint configuration and connectivity

### Debug Logging

Enable debug logging to troubleshoot monitoring issues:

```go
logger := monitoring.NewLogger("debug")
// or
logger = monitoring.NewStructuredLogger("debug")

// Check monitoring system status
system := getMonitoringSystem()
health := system.GetSystemHealth()
logger.Info("monitoring_system_health", map[string]interface{}{
    "status": health.Status,
    "components": len(health.ComponentHealth),
})
```

### Performance Considerations

- Adjust buffer sizes based on load: higher concurrency = larger buffers
- Use appropriate metric intervals: more frequent = higher overhead
- Consider export frequency: balance between real-time updates and performance
- Monitor the monitoring system itself for resource usage

## Integration Examples

See the test files and example implementations in:

- `/shared/http/monitored_client.go` - HTTP client monitoring
- `/shared/config/monitored_validator.go` - Config validation monitoring  
- `/shared/triggers/monitored_trigger.go` - Trigger system monitoring
- `/shared/monitoring/config.go` - System initialization examples