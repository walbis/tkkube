# Circuit Breaker Resilience System

This package provides a comprehensive circuit breaker implementation for the backup/restore system, designed to enhance resilience and prevent cascading failures when external services become unavailable.

## Features

- **Complete Circuit Breaker Implementation**: CLOSED, OPEN, and HALF_OPEN states with configurable thresholds
- **Service-Specific Circuit Breakers**: Dedicated circuit breakers for MinIO, HTTP, Git, Kubernetes, Security, and other services
- **Comprehensive Monitoring**: Built-in metrics, health monitoring, and observability
- **Integration-Ready**: Pre-built integrations for HTTP clients, MinIO operations, and Git operations
- **Production-Ready**: Thread-safe, high-performance implementation with extensive testing

## Quick Start

### 1. Basic Circuit Breaker Usage

```go
import "shared-config/resilience"

// Create circuit breaker manager
manager := resilience.NewCircuitBreakerManager(sharedConfig, monitoring)

// Use circuit breaker for operations
ctx := context.Background()
err := manager.WrapMinIOOperation(ctx, func() error {
    // Your MinIO operation here
    return minioClient.BucketExists(ctx, "my-bucket")
})

if resilience.IsCircuitBreakerError(err) {
    // Circuit breaker rejected the request
    log.Printf("Service unavailable: %v", err)
    // Implement fallback logic
}
```

### 2. HTTP Client with Circuit Breaker

```go
import (
    "shared-config/http"
    "shared-config/resilience"
)

// Create resilient HTTP client
httpClient := http.NewResilientHTTPClientFromSharedConfig(
    sharedConfig,
    circuitBreakerManager,
    monitoring,
    "backup_service",
    "backup_tool",
)

// Make HTTP requests with circuit breaker protection
resp, err := httpClient.Get(ctx, "http://backup-service/api/health")
if err != nil {
    if resilience.IsCircuitBreakerError(err) {
        // Handle circuit breaker rejection
        log.Printf("Backup service circuit breaker is open")
    }
}
```

### 3. MinIO Client with Circuit Breaker

```go
import "shared-config/storage"

// Create resilient MinIO client
minioClient, err := storage.NewResilientMinIOClientFromSharedConfig(
    sharedConfig,
    circuitBreakerManager,
    monitoring,
)

// All MinIO operations are automatically protected
uploadInfo, err := minioClient.PutObject(ctx, "bucket", "object", reader, size, opts)
if resilience.IsCircuitBreakerError(err) {
    // Handle circuit breaker rejection
    log.Printf("MinIO service unavailable")
}
```

### 4. Git Operations with Circuit Breaker

```go
import "shared-config/gitops"

// Create resilient Git client
gitClient := gitops.NewResilientGitClientFromSharedConfig(
    sharedConfig,
    circuitBreakerManager,
    monitoring,
    "/tmp/gitops",
)

// Git operations with circuit breaker protection
result, err := gitClient.Clone(ctx, repoURL, localPath)
if resilience.IsCircuitBreakerError(err) {
    // Handle circuit breaker rejection
    log.Printf("Git service unavailable")
}
```

## Configuration

### Shared Configuration

Circuit breaker settings are configured through the shared configuration:

```yaml
retries:
  circuit_breaker_threshold: 5      # Failures before opening circuit
  circuit_breaker_timeout: 60s      # Time to keep circuit open
  circuit_breaker_recovery_time: 300s # Time before attempting recovery
```

### Environment Variables

```bash
# Circuit breaker configuration
CIRCUIT_BREAKER_THRESHOLD=5
CIRCUIT_BREAKER_TIMEOUT=60s
CIRCUIT_BREAKER_RECOVERY_TIME=300s
```

### Service-Specific Configuration

Each service type has optimized defaults:

```go
// MinIO operations - more tolerant of failures
config.FailureThreshold = 10
config.Timeout = 2 * time.Minute
config.RecoveryTime = 5 * time.Minute

// HTTP operations - fast recovery
config.FailureThreshold = 5
config.Timeout = 30 * time.Second
config.RecoveryTime = 2 * time.Minute

// Git operations - slower but more resilient
config.FailureThreshold = 3
config.Timeout = 5 * time.Minute
config.RecoveryTime = 10 * time.Minute
```

## Monitoring and Observability

### Health Status

```go
// Get overall system health
healthStatus := manager.GetHealthStatus()
fmt.Printf("System Health: %.1f%%", healthStatus["overall_health"].(float64))

// Get service-specific health
for serviceName, health := range healthStatus["services"].(map[string]interface{}) {
    serviceHealth := health.(map[string]interface{})
    fmt.Printf("%s: %s (%.1f%% success rate)",
        serviceName,
        serviceHealth["state"].(string),
        serviceHealth["success_rate"].(float64))
}
```

### Metrics

```go
// Get detailed metrics for all circuit breakers
allMetrics := manager.GetAllMetrics()

for serviceName, metrics := range allMetrics {
    fmt.Printf("%s: %d total, %d successful, %d failed, %d rejected",
        serviceName,
        metrics.TotalRequests,
        metrics.SuccessfulReqs,
        metrics.FailedReqs,
        metrics.RejectedReqs)
}
```

### Circuit Breaker States

```go
// List all circuit breaker states
circuitBreakers := manager.ListCircuitBreakers()
for name, state := range circuitBreakers {
    fmt.Printf("%s: %s", name, state.String())
}
```

### Observability System

```go
// Create observability system
observabilityConfig := resilience.DefaultObservabilityConfig()
observer := resilience.NewCircuitBreakerObserver(observabilityConfig, monitoring)

// Register circuit breakers
for name := range manager.ListCircuitBreakers() {
    cb := manager.GetCircuitBreaker(name)
    observer.RegisterCircuitBreaker(name, cb)
}

// Start monitoring
observer.StartMonitoring(ctx)

// Get system health report
healthReport := observer.GetSystemHealth()
fmt.Printf("Overall Status: %s", healthReport.OverallStatus)

// Get recent events
events := observer.GetEventHistory(10)
for _, event := range events {
    fmt.Printf("[%s] %s: %s", event.Type, event.CircuitBreaker, event.Message)
}

// Get active alerts
alerts := observer.GetAlerts()
for _, alert := range alerts {
    fmt.Printf("[%s] %s: %s", alert.Severity, alert.CircuitBreaker, alert.Message)
}
```

## Circuit Breaker States

### CLOSED (Normal Operation)
- All requests are allowed through
- Failure count is tracked
- Transitions to OPEN when failure threshold is exceeded

### OPEN (Service Unavailable)
- All requests are immediately rejected
- Circuit breaker returns `CircuitBreakerError`
- Transitions to HALF_OPEN after recovery time

### HALF_OPEN (Testing Recovery)
- Limited number of requests allowed through
- Successful requests transition back to CLOSED
- Any failure immediately transitions back to OPEN

## Error Handling

### Circuit Breaker Errors

```go
err := manager.WrapMinIOOperation(ctx, operation)
if resilience.IsCircuitBreakerError(err) {
    // Circuit breaker is open, implement fallback
    return handleServiceUnavailable()
}
if err != nil {
    // Other error, handle normally
    return handleOperationError(err)
}
```

### Graceful Degradation

```go
func performBackupWithFallback(ctx context.Context) error {
    // Try primary backup method
    err := manager.WrapMinIOOperation(ctx, func() error {
        return uploadToPrimaryStorage(ctx)
    })
    
    if resilience.IsCircuitBreakerError(err) {
        // Fallback to secondary storage
        log.Println("Primary storage unavailable, using fallback")
        return uploadToSecondaryStorage(ctx)
    }
    
    return err
}
```

## Management Operations

### Manual Control

```go
// Reset a circuit breaker
err := manager.ResetCircuitBreaker("minio")

// Reset all circuit breakers
manager.ResetAllCircuitBreakers()

// Force open a circuit breaker (for maintenance)
err := manager.ForceOpenCircuitBreaker("git")
```

### Custom Circuit Breakers

```go
// Create custom circuit breaker
customConfig := resilience.DefaultCircuitBreakerConfig("custom-service")
customConfig.FailureThreshold = 3
customConfig.Timeout = 2 * time.Minute

cb := manager.CreateServiceCircuitBreaker("custom-service", customConfig)

// Use custom circuit breaker
err := manager.ExecuteWithCircuitBreaker(ctx, "custom-service", func() error {
    return customServiceOperation()
})
```

## Testing

### Unit Tests

```bash
# Run circuit breaker tests
go test ./shared/resilience/...

# Run with coverage
go test -cover ./shared/resilience/...

# Run benchmarks
go test -bench=. ./shared/resilience/...
```

### Integration Tests

```bash
# Run integration example
go run ./shared/examples/circuit_breaker_integration_example.go
```

## Best Practices

### 1. Service-Specific Configuration
- Use different thresholds for different service types
- Consider the criticality and expected reliability of each service
- Account for normal operational patterns

### 2. Monitoring and Alerting
- Monitor circuit breaker state changes
- Alert on multiple circuit breakers opening simultaneously
- Track success rates and failure patterns

### 3. Graceful Degradation
- Always implement fallback mechanisms
- Design for circuit breaker failures
- Consider partial functionality when services are unavailable

### 4. Testing Circuit Breaker Behavior
- Test failure scenarios regularly
- Verify fallback mechanisms work correctly
- Practice circuit breaker recovery procedures

### 5. Operations
- Include circuit breaker status in health checks
- Provide manual override capabilities
- Document escalation procedures for circuit breaker events

## Integration with Backup/Restore System

The circuit breaker system is fully integrated with the backup/restore system:

### Backup Operations
- **MinIO Storage**: Bucket operations, object uploads/downloads
- **HTTP Clients**: Backup tool communication, webhook notifications
- **Git Operations**: GitOps repository synchronization
- **Kubernetes API**: Resource discovery and management

### Restore Operations
- **API Endpoints**: Restore service communication
- **Validation Services**: Security and integrity checks
- **Storage Operations**: Backup retrieval and restoration

### Cross-Component Resilience
- **Independent Failures**: Circuit breakers isolate service failures
- **Graceful Degradation**: System continues operating with reduced functionality
- **Automatic Recovery**: Services automatically recover when available

## Performance

The circuit breaker implementation is designed for high performance:

- **Lock-free Operations**: Uses atomic operations for state management
- **Minimal Overhead**: <1μs overhead per operation in CLOSED state
- **Concurrent-Safe**: Thread-safe for high-concurrency environments
- **Memory Efficient**: Minimal memory footprint per circuit breaker

## Troubleshooting

### Circuit Breaker Not Opening
- Check failure threshold configuration
- Verify error types are being counted as failures
- Check if operations are actually going through circuit breaker

### Circuit Breaker Not Closing
- Verify recovery time has elapsed
- Check that recovery operations are actually succeeding
- Ensure success threshold is being met in HALF_OPEN state

### High False Positives
- Adjust failure threshold for service characteristics
- Review timeout settings
- Consider different thresholds for different error types

### Monitoring Issues
- Verify monitoring system is configured correctly
- Check metric collection and retention settings
- Ensure observability system is started

For more examples and detailed usage, see the integration example in `/shared/examples/circuit_breaker_integration_example.go`.