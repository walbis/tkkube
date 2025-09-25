# Enhanced Multi-Cluster Validation System

## Overview

The Enhanced Multi-Cluster Validation System provides comprehensive validation for multi-cluster backup configurations, including token validation, cluster connectivity checks, and real-time health monitoring. This system ensures that your multi-cluster backup orchestrator is properly configured and operational before attempting backup operations.

## Features

### 🔐 **Token Validation**
- **Bearer Token Validation**: Format checking, length validation, environment variable detection
- **Service Account Token Validation**: File existence, permissions, format verification
- **OIDC Token Validation**: JWT structure, issuer URL validation, client configuration
- **Exec Authentication Validation**: Command existence, path validation, execution checks

### 🌐 **Connectivity Verification**
- **Network Connectivity**: Basic TCP connectivity tests to cluster endpoints
- **TLS Validation**: Certificate chain verification, custom CA validation
- **Kubernetes API Testing**: Authentication verification with real API calls
- **Storage Backend Testing**: MinIO/S3 endpoint connectivity verification

### 📊 **Live Health Monitoring**
- **Real-time Validation**: Periodic validation of all configured clusters
- **Health Check Service**: HTTP API for health status and metrics
- **Event-driven Architecture**: Configurable event handlers for validation events
- **Performance Metrics**: Detailed timing and success rate tracking

### 🔧 **Configuration Validation**
- **Schema Validation**: Complete configuration structure validation
- **Cross-cluster Validation**: Detect conflicts and inconsistencies
- **Security Validation**: Demo token detection, insecure endpoint warnings
- **Production Readiness**: Additional validation for production environments

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Enhanced Validation System                 │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌───────────────────────────────┐  │
│  │ Live Validation │  │ Enhanced Multi-Cluster        │  │
│  │ Service         │──│ Validator                     │  │
│  │                 │  │                               │  │
│  │ • HTTP API      │  │ • Token Validation            │  │
│  │ • Health Checks │  │ • Connectivity Checks         │  │
│  │ • Event Bus     │  │ • Config Validation           │  │
│  │ • Metrics       │  │ • Caching & Performance       │  │
│  └─────────────────┘  └───────────────────────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌───────────────────────────────┐  │
│  │ Base Multi-     │  │ Cluster Auth Manager          │  │
│  │ Cluster         │──│                               │  │
│  │ Validator       │  │ • Auth Method Validation      │  │
│  │                 │  │ • TLS Configuration           │  │
│  │ • Schema Check  │  │ • REST Config Creation        │  │
│  │ • Basic Rules   │  │ • Security Validation         │  │
│  └─────────────────┘  └───────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Quick Start

### Basic Validation

```go
package main

import (
    "log"
    sharedconfig "shared-config/config"
)

func main() {
    // Load configuration
    loader := sharedconfig.NewConfigLoader("multi-cluster-config.yaml")
    config, err := loader.Load()
    if err != nil {
        log.Fatal(err)
    }

    // Create enhanced validator
    validator := sharedconfig.NewEnhancedMultiClusterValidator(nil)

    // Perform comprehensive validation
    result := validator.ValidateMultiClusterConfigurationWithLiveChecks(&config.MultiCluster)
    
    if !result.OverallValid {
        log.Printf("Validation failed with %d errors:", len(result.GlobalErrors))
        for _, err := range result.GlobalErrors {
            log.Printf("  - %s: %s", err.Field, err.Message)
        }
        
        for clusterName, clusterResult := range result.ClusterResults {
            if !clusterResult.Valid {
                log.Printf("Cluster %s errors:", clusterName)
                for _, err := range clusterResult.Errors {
                    log.Printf("    - %s: %s", err.Field, err.Message)
                }
            }
        }
    } else {
        log.Printf("✅ All validation checks passed!")
        log.Printf("Validated %d clusters in %v", len(result.ClusterResults), result.TotalValidationTime)
    }
}
```

### Live Validation Service

```go
package main

import (
    "log"
    "time"
    sharedconfig "shared-config/config"
)

func main() {
    // Load configuration
    loader := sharedconfig.NewConfigLoader("multi-cluster-config.yaml")
    config, err := loader.Load()
    if err != nil {
        log.Fatal(err)
    }

    // Configure live validation service
    serviceConfig := &sharedconfig.LiveValidationServiceConfig{
        Enabled:               true,
        ValidationInterval:    5 * time.Minute,  // Validate every 5 minutes
        HealthCheckInterval:   30 * time.Second, // Health checks every 30s
        HTTPServerPort:        8090,
        ValidationOptions: &sharedconfig.EnhancedValidationOptions{
            EnableConnectivityChecks: true,
            EnableTokenValidation:    true,
            ValidationTimeout:        30 * time.Second,
        },
    }

    // Create and start live validation service
    service := sharedconfig.NewLiveValidationService(&config.MultiCluster, serviceConfig)
    
    if err := service.Start(); err != nil {
        log.Fatal("Failed to start validation service:", err)
    }
    defer service.Stop()

    log.Println("Live validation service started on http://localhost:8090")
    
    // Register event handlers
    service.RegisterEventHandler(sharedconfig.EventValidationFailed, func(event *sharedconfig.ValidationEvent) error {
        log.Printf("⚠️ Validation failed: %+v", event.Data)
        return nil
    })

    service.RegisterEventHandler(sharedconfig.EventClusterUnreachable, func(event *sharedconfig.ValidationEvent) error {
        log.Printf("🚨 Cluster unreachable: %s", event.ClusterName)
        return nil
    })

    // Keep service running
    select {}
}
```

## Validation Options

### EnhancedValidationOptions

```go
type EnhancedValidationOptions struct {
    EnableConnectivityChecks bool          // Enable network and API connectivity tests
    EnableTokenValidation    bool          // Enable token format and validity checks
    EnableLiveValidation     bool          // Enable real-time validation monitoring
    ValidationTimeout        time.Duration // Timeout for individual validation operations
    MaxConcurrentChecks      int           // Maximum concurrent cluster validations
    CacheTimeout             time.Duration // How long to cache validation results
    SkipTLSVerification      bool          // Skip TLS verification (testing only)
}

// Default options
options := &EnhancedValidationOptions{
    EnableConnectivityChecks: true,
    EnableTokenValidation:    true,
    EnableLiveValidation:     false,
    ValidationTimeout:        30 * time.Second,
    MaxConcurrentChecks:      5,
    CacheTimeout:             5 * time.Minute,
    SkipTLSVerification:      false,
}
```

### LiveValidationServiceConfig

```go
type LiveValidationServiceConfig struct {
    Enabled               bool
    ValidationInterval    time.Duration
    HealthCheckInterval   time.Duration
    HTTPServerPort        int
    MaxHistoryEntries     int
    EnableEventHandlers   bool
    ValidationOptions     *EnhancedValidationOptions
}

// Production configuration
config := &LiveValidationServiceConfig{
    Enabled:               true,
    ValidationInterval:    10 * time.Minute,  // Validate every 10 minutes
    HealthCheckInterval:   1 * time.Minute,   // Health checks every minute
    HTTPServerPort:        8090,
    MaxHistoryEntries:     100,               // Keep last 100 validation results
    EnableEventHandlers:   true,
}
```

## Validation Types

### 1. Token Validation

#### Bearer Token Validation
```go
// Validates bearer tokens for format, length, and content
auth:
  method: "token"
  token:
    value: "your-cluster-token-here"
    type: "bearer"
    refresh_threshold: 300
```

**Validation Checks:**
- ✅ Token length (minimum 10 characters)
- ✅ Environment variable expansion detection
- ✅ Demo token pattern detection
- ✅ JWT structure validation (if applicable)
- ✅ Authentication test (if connectivity enabled)

#### Service Account Token Validation
```go
auth:
  method: "service_account"
  service_account:
    token_path: "/var/run/secrets/kubernetes.io/serviceaccount/token"
    ca_cert_path: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
```

**Validation Checks:**
- ✅ Token file existence and readability
- ✅ Token file content validation
- ✅ CA certificate file validation
- ✅ File permissions check

#### OIDC Token Validation
```go
auth:
  method: "oidc"
  oidc:
    issuer_url: "https://oidc.company.com"
    client_id: "kubernetes-cluster"
    client_secret: "${OIDC_CLIENT_SECRET}"
    id_token: "${OIDC_ID_TOKEN}"
```

**Validation Checks:**
- ✅ Issuer URL format validation
- ✅ Required field presence
- ✅ JWT structure validation
- ✅ Client configuration validation

#### Exec Authentication Validation
```go
auth:
  method: "exec"
  exec:
    command: "/usr/local/bin/get-k8s-token"
    args: ["--cluster", "prod"]
    env: ["KUBECONFIG=/etc/kubeconfig"]
```

**Validation Checks:**
- ✅ Command existence and executability
- ✅ Path resolution validation
- ✅ Environment variable validation

### 2. Connectivity Validation

#### Network Connectivity
```go
// Tests basic TCP connectivity to cluster endpoints
endpoint: "https://api.prod.k8s.company.com:6443"
```

**Validation Checks:**
- ✅ DNS resolution
- ✅ TCP connection establishment
- ✅ Response time measurement
- ✅ Network timeout handling

#### TLS Validation
```go
tls:
  insecure: false
  ca_bundle: "/etc/ssl/certs/ca-certificates.crt"
  cert_data: "${CLIENT_CERT_DATA}"
  key_data: "${CLIENT_KEY_DATA}"
```

**Validation Checks:**
- ✅ Certificate chain validation
- ✅ Custom CA certificate verification
- ✅ Client certificate validation
- ✅ TLS handshake testing

#### Kubernetes API Validation
```go
// Tests actual Kubernetes API connectivity with authentication
```

**Validation Checks:**
- ✅ API server reachability
- ✅ Authentication success
- ✅ Basic API operation (list namespaces)
- ✅ Server version detection

### 3. Configuration Validation

#### Schema Validation
```go
// Validates complete configuration structure
```

**Validation Checks:**
- ✅ Required field presence
- ✅ Data type validation
- ✅ Value range checking
- ✅ Format validation (URLs, durations, etc.)

#### Security Validation
```go
// Detects security issues and misconfigurations
```

**Validation Checks:**
- ✅ Demo/test token detection
- ✅ Insecure endpoint warnings
- ✅ Weak authentication methods
- ✅ Production readiness assessment

#### Cross-Cluster Validation
```go
// Validates consistency across multiple clusters
```

**Validation Checks:**
- ✅ Duplicate endpoint detection
- ✅ Storage bucket conflicts
- ✅ Priority configuration validation
- ✅ Load distribution analysis

## HTTP API Reference

The Live Validation Service provides a REST API for monitoring and controlling validation:

### Endpoints

#### GET /health
Returns overall health status and cluster health information.

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "overall_healthy": true,
  "cluster_health": {
    "prod-cluster": {
      "cluster_name": "prod-cluster",
      "healthy": true,
      "last_validated": "2024-01-15T10:29:30Z",
      "response_time": "120ms",
      "error_count": 0,
      "consecutive_errors": 0,
      "availability": 0.99
    }
  },
  "service_health": {
    "healthy": true,
    "uptime": "24h30m",
    "validations_performed": 288,
    "error_rate": 0.02
  }
}
```

#### GET /validation
Returns the latest complete validation result.

```json
{
  "overall_valid": true,
  "validation_time": "2024-01-15T10:30:00Z",
  "total_validation_time": "2.5s",
  "cluster_results": {
    "prod-cluster": {
      "cluster_name": "prod-cluster",
      "valid": true,
      "connectivity_status": {
        "reachable": true,
        "response_time": "120ms",
        "tls_valid": true,
        "api_server_version": "v1.28.4",
        "authentication_valid": true
      },
      "token_validation": {
        "valid": true,
        "token_type": "bearer",
        "validation_method": "token"
      }
    }
  },
  "summary": {
    "total_clusters": 3,
    "valid_clusters": 3,
    "invalid_clusters": 0,
    "total_errors": 0,
    "total_warnings": 1
  }
}
```

#### GET /validation/status
Returns a summary of the latest validation status.

```json
{
  "overall_valid": true,
  "validation_time": "2024-01-15T10:30:00Z",
  "total_clusters": 3,
  "valid_clusters": 3,
  "invalid_clusters": 0,
  "summary": {
    "total_clusters": 3,
    "valid_clusters": 3,
    "invalid_clusters": 0,
    "total_errors": 0,
    "total_warnings": 1
  }
}
```

#### GET /validation/history
Returns historical validation results.

```json
[
  {
    "timestamp": "2024-01-15T10:30:00Z",
    "duration": "2.5s",
    "result": {
      "overall_valid": true,
      "cluster_results": {...}
    }
  }
]
```

#### POST /validation/trigger
Triggers an immediate validation check.

```json
{
  "status": "triggered",
  "message": "Validation triggered successfully"
}
```

#### GET /clusters
Returns health information for all clusters.

```json
{
  "prod-cluster": {
    "cluster_name": "prod-cluster",
    "healthy": true,
    "last_validated": "2024-01-15T10:29:30Z",
    "response_time": "120ms",
    "availability": 0.99
  },
  "staging-cluster": {
    "cluster_name": "staging-cluster",
    "healthy": true,
    "last_validated": "2024-01-15T10:29:35Z",
    "response_time": "85ms",
    "availability": 0.98
  }
}
```

#### GET /clusters/{cluster-name}
Returns detailed validation information for a specific cluster.

```json
{
  "cluster_name": "prod-cluster",
  "valid": true,
  "validated_at": "2024-01-15T10:29:30Z",
  "connectivity_status": {
    "reachable": true,
    "response_time": "120ms",
    "tls_valid": true,
    "api_server_version": "v1.28.4",
    "authentication_valid": true
  },
  "token_validation": {
    "valid": true,
    "token_type": "bearer",
    "validation_method": "token"
  },
  "performance_metrics": {
    "total_validation_time": "1.2s",
    "connectivity_check_time": "120ms",
    "token_validation_time": "50ms"
  }
}
```

#### GET /metrics
Returns performance and operational metrics.

```json
{
  "total_validations": 288,
  "successful_validations": 282,
  "failed_validations": 6,
  "average_validation_time": "2.1s",
  "last_validation_time": "2024-01-15T10:30:00Z",
  "cluster_metrics": {
    "prod-cluster": {
      "cluster_name": "prod-cluster",
      "successful_validations": 96,
      "failed_validations": 2,
      "average_response_time": "120ms",
      "last_successful_check": "2024-01-15T10:29:30Z",
      "consecutive_failures": 0
    }
  }
}
```

## Event System

### Event Types

```go
const (
    EventValidationStarted     ValidationEventType = "validation_started"
    EventValidationCompleted   ValidationEventType = "validation_completed"
    EventValidationFailed      ValidationEventType = "validation_failed"
    EventClusterUnreachable    ValidationEventType = "cluster_unreachable"
    EventClusterReconnected    ValidationEventType = "cluster_reconnected"
    EventTokenExpired          ValidationEventType = "token_expired"
    EventConfigurationChanged  ValidationEventType = "configuration_changed"
    EventHealthCheckFailed     ValidationEventType = "health_check_failed"
)
```

### Event Handler Registration

```go
service.RegisterEventHandler(EventClusterUnreachable, func(event *ValidationEvent) error {
    // Send alert to monitoring system
    alert := Alert{
        Severity:    "critical",
        Summary:     fmt.Sprintf("Cluster %s is unreachable", event.ClusterName),
        Description: fmt.Sprintf("Cluster validation failed: %+v", event.Data),
        Timestamp:   event.Timestamp,
    }
    
    return alertManager.Send(alert)
})

service.RegisterEventHandler(EventTokenExpired, func(event *ValidationEvent) error {
    // Trigger token renewal process
    return tokenManager.RenewToken(event.ClusterName)
})

service.RegisterEventHandler(EventValidationFailed, func(event *ValidationEvent) error {
    // Log detailed validation failure
    log.Printf("Validation failed: %+v", event.Data)
    return nil
})
```

## Integration Examples

### Integration with Backup Orchestrator

```go
package main

import (
    "log"
    "time"
    sharedconfig "shared-config/config"
)

func main() {
    // Load configuration
    loader := sharedconfig.NewConfigLoader("config.yaml")
    config, err := loader.Load()
    if err != nil {
        log.Fatal(err)
    }

    // Validate configuration before starting backup orchestrator
    validator := sharedconfig.NewEnhancedMultiClusterValidator(nil)
    result := validator.ValidateMultiClusterConfigurationWithLiveChecks(&config.MultiCluster)
    
    if !result.OverallValid {
        log.Fatal("Configuration validation failed - cannot start backup orchestrator")
    }

    // Create backup orchestrator
    orchestrator, err := sharedconfig.NewMultiClusterBackupOrchestrator(&config.MultiCluster)
    if err != nil {
        log.Fatal(err)
    }

    // Start live validation service
    validationService := sharedconfig.NewLiveValidationService(&config.MultiCluster, &sharedconfig.LiveValidationServiceConfig{
        ValidationInterval: 5 * time.Minute,
    })

    if err := validationService.Start(); err != nil {
        log.Fatal("Failed to start validation service:", err)
    }

    // Register event handler to pause backups if validation fails
    validationService.RegisterEventHandler(sharedconfig.EventValidationFailed, func(event *sharedconfig.ValidationEvent) error {
        log.Println("⚠️ Pausing backup operations due to validation failure")
        // Implement pause logic here
        return nil
    })

    // Start backup orchestrator
    log.Println("Starting backup orchestrator with live validation monitoring")
    // Run backup orchestrator...
}
```

### Integration with Monitoring Systems

```go
// Prometheus metrics integration
service.RegisterEventHandler(sharedconfig.EventValidationCompleted, func(event *sharedconfig.ValidationEvent) error {
    summary, ok := event.Data.(*sharedconfig.ValidationSummary)
    if !ok {
        return nil
    }
    
    // Update Prometheus metrics
    validationSuccessMetric.Set(float64(summary.ValidClusters))
    validationFailureMetric.Set(float64(summary.InvalidClusters))
    validationErrorsMetric.Set(float64(summary.TotalErrors))
    
    return nil
})

// Slack integration
service.RegisterEventHandler(sharedconfig.EventClusterUnreachable, func(event *sharedconfig.ValidationEvent) error {
    message := SlackMessage{
        Channel: "#backup-alerts",
        Text:    fmt.Sprintf("🚨 Cluster %s is unreachable", event.ClusterName),
        Color:   "danger",
    }
    
    return slackClient.Send(message)
})
```

## Best Practices

### Production Configuration

```yaml
# Production-ready validation configuration
multi_cluster:
  enabled: true
  clusters:
    - name: "prod-cluster"
      endpoint: "https://api.prod.k8s.company.com:6443"  # HTTPS required
      auth:
        method: "service_account"  # Preferred for production
        service_account:
          token_path: "/var/run/secrets/kubernetes.io/serviceaccount/token"
          ca_cert_path: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
      tls:
        insecure: false  # Never use insecure in production
        ca_bundle: "/etc/ssl/certs/ca-certificates.crt"

# Live validation service configuration
validation_service:
  enabled: true
  validation_interval: "10m"     # Reasonable interval for production
  health_check_interval: "1m"   # Frequent health checks
  http_server_port: 8090
  validation_options:
    enable_connectivity_checks: true
    enable_token_validation: true
    validation_timeout: "30s"
    max_concurrent_checks: 3     # Limit concurrent operations
```

### Security Guidelines

1. **Token Security**
   - Never use demo or test tokens in production
   - Use environment variables for sensitive values
   - Implement token rotation policies
   - Monitor token expiration

2. **Network Security**
   - Always use HTTPS endpoints
   - Validate TLS certificates
   - Use custom CA bundles when required
   - Implement network timeouts

3. **Access Control**
   - Use service accounts with minimal permissions
   - Implement RBAC for validation service
   - Secure validation service endpoints
   - Monitor access logs

### Performance Optimization

1. **Validation Frequency**
   - Use appropriate validation intervals (5-15 minutes)
   - Cache validation results when possible
   - Limit concurrent validations
   - Monitor validation duration

2. **Resource Management**
   - Set reasonable timeouts
   - Limit concurrent operations
   - Monitor memory and CPU usage
   - Clean up resources properly

3. **Error Handling**
   - Implement exponential backoff for retries
   - Use circuit breakers for failing clusters
   - Log errors with appropriate detail
   - Provide clear error messages

## Troubleshooting

### Common Issues

#### Validation Timeout Errors
```
Error: context deadline exceeded during validation
```

**Solutions:**
- Increase `ValidationTimeout` in options
- Check network connectivity to cluster endpoints
- Verify DNS resolution
- Monitor system resource usage

#### Token Validation Failures
```
Error: Token authentication test failed
```

**Solutions:**
- Verify token format and content
- Check token expiration
- Validate RBAC permissions
- Test authentication manually with kubectl

#### TLS Certificate Issues
```
Error: TLS validation failed
```

**Solutions:**
- Verify certificate chain
- Check CA bundle configuration
- Validate certificate expiration
- Test TLS connection manually

#### High Memory Usage
```
Warning: Validation service using excessive memory
```

**Solutions:**
- Reduce `MaxHistoryEntries`
- Decrease `CacheTimeout`
- Limit `MaxConcurrentChecks`
- Monitor goroutine leaks

### Debug Mode

Enable debug logging for detailed troubleshooting:

```go
validator := sharedconfig.NewEnhancedMultiClusterValidator(&sharedconfig.EnhancedValidationOptions{
    ValidationTimeout: 60 * time.Second,  // Longer timeout for debugging
})

// Enable detailed logging
log.SetLevel(log.DebugLevel)
```

### Health Checks

Monitor validation service health:

```bash
# Check overall health
curl http://localhost:8090/health

# Check specific cluster
curl http://localhost:8090/clusters/prod-cluster

# Trigger immediate validation
curl -X POST http://localhost:8090/validation/trigger

# Get performance metrics
curl http://localhost:8090/metrics
```

## Testing

### Unit Tests
```bash
go test ./shared/config -v -run TestEnhancedMultiClusterValidator
```

### Integration Tests
```bash
go test ./shared/config -v -run TestConnectivityChecks
```

### Benchmark Tests
```bash
go test ./shared/config -bench=BenchmarkTokenValidation -benchmem
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add comprehensive tests
4. Ensure all tests pass
5. Update documentation
6. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

---

**Note**: This validation system is designed to work seamlessly with the multi-cluster backup orchestrator, providing confidence that your backup operations will succeed by validating configuration and connectivity before execution.