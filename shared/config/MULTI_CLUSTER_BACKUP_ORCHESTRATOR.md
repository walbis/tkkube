# Multi-Cluster Backup Orchestrator

## Overview

The Multi-Cluster Backup Orchestrator is a comprehensive solution for coordinating backup operations across multiple Kubernetes clusters with enhanced authentication, parallel execution, and sophisticated error handling capabilities.

## Features

### 🔐 Enhanced Authentication
- **Multiple Auth Methods**: Token-based, service account, OIDC, and exec authentication
- **Secure TLS**: Certificate authority validation, client certificates, custom CA bundles
- **Environment Variables**: Secure credential management with `${VAR}` expansion
- **Backward Compatibility**: Legacy token support maintained

### ⚡ Parallel Execution
- **Sequential Mode**: Execute backups one cluster at a time with failure threshold control
- **Parallel Mode**: Execute multiple cluster backups simultaneously with concurrency limits
- **Priority Scheduling**: Backup high-priority clusters first
- **Load Balancing**: Distribute workload across available resources

### 🔄 Advanced Coordination
- **Circuit Breakers**: Prevent cascading failures with automatic recovery
- **Retry Policies**: Configurable retry behavior with exponential backoff
- **Health Monitoring**: Continuous cluster health checking
- **Workflow Management**: Complex backup workflows with checkpoints

### 📊 Monitoring & Observability
- **Comprehensive Metrics**: Execution statistics, performance metrics, health indicators
- **Event Bus**: Event-driven architecture with extensible handlers
- **Structured Logging**: JSON-formatted logs with correlation IDs
- **Real-time Status**: Live execution monitoring and progress tracking

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                Multi-Cluster Backup Orchestrator        │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌───────────────────────────────┐  │
│  │ Advanced        │  │ Base Multi-Cluster            │  │
│  │ Orchestrator    │─▶│ Backup Orchestrator           │  │
│  │                 │  │                               │  │
│  │ • Priority      │  │ • Cluster Management          │  │
│  │ • Load Balance  │  │ • Authentication               │  │
│  │ • Circuit Break │  │ • Parallel/Sequential Exec    │  │
│  │ • Workflows     │  │ • Health Monitoring           │  │
│  └─────────────────┘  └───────────────────────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌───────────────────────────────┐  │
│  │ Multi-Cluster   │  │ Enhanced Authentication       │  │
│  │ Manager         │─▶│ Manager                       │  │
│  │                 │  │                               │  │
│  │ • Cluster Conn  │  │ • Token Auth                  │  │
│  │ • Health Checks │  │ • Service Account Auth        │  │
│  │ • Execution     │  │ • OIDC Auth                   │  │
│  │ • Coordination  │  │ • TLS Configuration           │  │
│  └─────────────────┘  └───────────────────────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────┐  │
│  │ Cluster Backup  │  │ Storage         │  │ K8s API  │  │
│  │ Executors       │─▶│ Backends        │  │ Clients  │  │
│  │                 │  │                 │  │          │  │
│  │ • Per-cluster   │  │ • MinIO/S3      │  │ • Auth   │  │
│  │ • Config        │  │ • Compression   │  │ • REST   │  │
│  │ • Execution     │  │ • Encryption    │  │ • Dynamic│  │
│  └─────────────────┘  └─────────────────┘  └──────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Basic Usage

```go
package main

import (
    "context"
    "log"
    sharedconfig "shared-config/config"
)

func main() {
    // Load configuration
    loader := sharedconfig.NewConfigLoader("backup-config.yaml")
    config, err := loader.Load()
    if err != nil {
        log.Fatal(err)
    }

    // Create orchestrator
    orchestrator, err := sharedconfig.NewMultiClusterBackupOrchestrator(&config.MultiCluster)
    if err != nil {
        log.Fatal(err)
    }
    defer orchestrator.Shutdown(context.Background())

    // Execute backup
    result, err := orchestrator.ExecuteBackup()
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Backup completed: %d successful, %d failed clusters", 
        result.SuccessfulClusters, result.FailedClusters)
}
```

### 2. Advanced Usage with Priority Scheduling

```go
// Create advanced orchestrator with priority scheduling
advancedOrchestrator, err := sharedconfig.NewAdvancedBackupOrchestrator(&config.MultiCluster)
if err != nil {
    log.Fatal(err)
}
defer advancedOrchestrator.Shutdown(context.Background())

// Execute with advanced features
result, err := advancedOrchestrator.ExecuteAdvancedBackup()
if err != nil {
    log.Printf("Backup completed with warnings: %v", err)
}

// Get comprehensive status
status := advancedOrchestrator.GetAdvancedStatus()
log.Printf("System health: %v", status["system_health"])
```

## Configuration

### Multi-Cluster Configuration

```yaml
multi_cluster:
  enabled: true
  mode: "parallel"  # or "sequential"
  default_cluster: "prod-cluster"
  
  clusters:
    - name: "prod-cluster"
      endpoint: "https://api.prod.k8s.company.com:6443"
      auth:
        method: "service_account"
        service_account:
          token_path: "/var/run/secrets/kubernetes.io/serviceaccount/token"
          ca_cert_path: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
      tls:
        insecure: false
        ca_bundle: "/etc/ssl/certs/prod-ca.crt"
      storage:
        type: "s3"
        endpoint: "s3.us-east-1.amazonaws.com"
        access_key: "${PROD_S3_ACCESS_KEY}"
        secret_key: "${PROD_S3_SECRET_KEY}"
        bucket: "prod-cluster-backups"
        use_ssl: true
        region: "us-east-1"
  
  coordination:
    timeout: 1800  # 30 minutes
    retry_attempts: 3
    failure_threshold: 2
    health_check_interval: "60s"
  
  scheduling:
    strategy: "priority"
    max_concurrent_clusters: 3
    cluster_priorities:
      - cluster: "prod-cluster"
        priority: 1
      - cluster: "staging-cluster"
        priority: 2
```

### Authentication Methods

#### 1. Token Authentication
```yaml
auth:
  method: "token"
  token:
    value: "${CLUSTER_TOKEN}"
    type: "bearer"
    refresh_threshold: 300
```

#### 2. Service Account Authentication
```yaml
auth:
  method: "service_account"
  service_account:
    token_path: "/var/run/secrets/kubernetes.io/serviceaccount/token"
    ca_cert_path: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
```

#### 3. OIDC Authentication
```yaml
auth:
  method: "oidc"
  oidc:
    issuer_url: "https://oidc.company.com"
    client_id: "kubernetes-backup"
    client_secret: "${OIDC_CLIENT_SECRET}"
    id_token: "${OIDC_ID_TOKEN}"
    refresh_token: "${OIDC_REFRESH_TOKEN}"
```

#### 4. Exec Authentication
```yaml
auth:
  method: "exec"
  exec:
    command: "/usr/local/bin/get-k8s-token"
    args:
      - "--cluster=prod"
      - "--format=token"
    env:
      - "CLUSTER_NAME=prod"
```

### TLS Configuration

```yaml
tls:
  insecure: false
  ca_bundle: "/etc/ssl/certs/ca-certificates.crt"  # CA bundle file
  ca_data: "${CA_CERT_DATA}"                       # Base64 encoded CA cert
  cert_file: "/etc/ssl/client.crt"                 # Client certificate file
  key_file: "/etc/ssl/client.key"                  # Client key file
  cert_data: "${CLIENT_CERT_DATA}"                 # Base64 encoded client cert
  key_data: "${CLIENT_KEY_DATA}"                   # Base64 encoded client key
  server_name: "kubernetes.company.com"            # SNI server name
```

## API Reference

### MultiClusterBackupOrchestrator

#### Methods

```go
// NewMultiClusterBackupOrchestrator creates a new orchestrator
func NewMultiClusterBackupOrchestrator(config *MultiClusterConfig) (*MultiClusterBackupOrchestrator, error)

// ExecuteBackup starts the multi-cluster backup process
func (mbo *MultiClusterBackupOrchestrator) ExecuteBackup() (*MultiClusterBackupResult, error)

// GetExecutorStatus returns the current status of all backup executors
func (mbo *MultiClusterBackupOrchestrator) GetExecutorStatus() map[string]interface{}

// GetOrchestratorStats returns orchestrator statistics
func (mbo *MultiClusterBackupOrchestrator) GetOrchestratorStats() map[string]interface{}

// Shutdown gracefully shuts down the orchestrator
func (mbo *MultiClusterBackupOrchestrator) Shutdown(ctx context.Context) error
```

### AdvancedBackupOrchestrator

#### Methods

```go
// NewAdvancedBackupOrchestrator creates a new advanced orchestrator
func NewAdvancedBackupOrchestrator(config *MultiClusterConfig) (*AdvancedBackupOrchestrator, error)

// ExecuteAdvancedBackup executes backup with advanced coordination features
func (abo *AdvancedBackupOrchestrator) ExecuteAdvancedBackup() (*MultiClusterBackupResult, error)

// GetAdvancedStatus returns comprehensive status information
func (abo *AdvancedBackupOrchestrator) GetAdvancedStatus() map[string]interface{}
```

### Data Structures

#### MultiClusterBackupResult
```go
type MultiClusterBackupResult struct {
    TotalClusters       int
    SuccessfulClusters  int
    FailedClusters      int
    TotalDuration       time.Duration
    ClusterResults      map[string]*ClusterBackupResult
    OverallStatus       BackupStatus
    ExecutionMode       string
    StartTime           time.Time
    EndTime             time.Time
}
```

#### ClusterBackupResult
```go
type ClusterBackupResult struct {
    ClusterName         string
    StartTime          time.Time
    EndTime            time.Time
    Duration           time.Duration
    Status             BackupStatus
    NamespacesBackedUp int
    ResourcesBackedUp  int
    TotalDataSize      int64
    CompressedSize     int64
    Errors             []error
    Warnings           []string
    StorageLocation    string
    BackupID           string
}
```

## Execution Modes

### Sequential Mode
- Executes backups one cluster at a time
- Respects priority ordering when priorities are configured
- Stops execution if failure threshold is reached
- Better for resource-constrained environments
- Easier debugging and monitoring

```yaml
multi_cluster:
  mode: "sequential"
  coordination:
    failure_threshold: 1  # Stop after 1 failure
```

### Parallel Mode
- Executes multiple cluster backups simultaneously
- Controlled by `max_concurrent_clusters` setting
- Uses goroutines with semaphore-based concurrency control
- Better performance for multiple clusters
- Requires more system resources

```yaml
multi_cluster:
  mode: "parallel"
  scheduling:
    max_concurrent_clusters: 3  # Max 3 simultaneous backups
```

## Priority Scheduling

Configure cluster priorities to ensure critical clusters are backed up first:

```yaml
scheduling:
  strategy: "priority"
  cluster_priorities:
    - cluster: "prod-cluster"
      priority: 1      # Highest priority (backup first)
    - cluster: "staging-cluster"
      priority: 2      # Second priority
    - cluster: "dev-cluster"
      priority: 3      # Lowest priority
```

**Priority Rules:**
- Lower numbers = higher priority
- Clusters without explicit priority get priority 99 (lowest)
- In parallel mode, priority affects batch ordering
- In sequential mode, priority determines execution order

## Error Handling & Recovery

### Circuit Breaker Pattern
Prevents cascading failures by temporarily disabling operations to failing clusters:

```yaml
orchestration:
  circuit_breaker:
    enabled: true
    failure_threshold: 5      # Open after 5 failures
    recovery_timeout: "5m"    # Try half-open after 5 minutes
    half_open_max_calls: 3    # Max calls in half-open state
```

**States:**
- **Closed**: Normal operation
- **Open**: All requests fail fast
- **Half-Open**: Limited requests to test recovery

### Retry Policy
Configurable retry behavior for transient failures:

```yaml
orchestration:
  retry_policy:
    max_attempts: 3
    base_delay: "10s"
    max_delay: "5m"
    backoff_multiplier: 2.0
    jitter_enabled: true
```

### Health Monitoring
Continuous monitoring of cluster health:

```yaml
orchestration:
  health_monitoring:
    enabled: true
    check_interval: "30s"
    timeout: "10s"
    failure_threshold: 3
```

## Monitoring & Observability

### Metrics
The orchestrator exposes various metrics for monitoring:

- **Execution Metrics**: Total executions, success/failure rates, duration
- **Cluster Metrics**: Per-cluster success rates, health status, response times
- **System Metrics**: Memory usage, CPU usage, active connections
- **Performance Metrics**: Throughput, batch processing efficiency

### Logging
Structured JSON logging with correlation IDs:

```go
// Example log entry
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "message": "backup_execution_started",
  "cluster": "prod-cluster",
  "execution_id": "exec-1705308600",
  "correlation_id": "corr-12345"
}
```

### Events
Event-driven architecture for integration:

```go
// Subscribe to events
orchestrator.EventBus.Subscribe("backup.execution.completed", func(event *Event) error {
    // Handle backup completion
    return nil
})
```

**Available Events:**
- `backup.execution.started`
- `backup.execution.completed`
- `backup.cluster.started`
- `backup.cluster.completed`
- `backup.cluster.failed`
- `health.check.failed`
- `circuit.breaker.opened`

## Performance Optimization

### Resource Management
```yaml
performance:
  limits:
    max_concurrent_operations: 6
    memory_limit: "8Gi"
    cpu_limit: "4"
    max_backup_size: "10Gi"
  
  optimization:
    batch_processing: true
    compression: true
    caching: true
    cache_ttl: 900
```

### HTTP Client Tuning
```yaml
performance:
  http:
    max_idle_conns: 200
    max_conns_per_host: 100
    request_timeout: "300s"
    keep_alive_timeout: "30s"
```

## Security

### Credential Management
- Environment variable expansion: `${VAR}`
- Support for external secret managers
- No credentials stored in plain text
- Automatic token refresh

### Network Security
- TLS verification with custom CA bundles
- Client certificate authentication
- Configurable timeouts and connection limits
- SNI support for multi-domain certificates

### Validation
- Comprehensive input validation
- Secret scanning prevention
- Schema validation
- Runtime security checks

## Production Deployment

### Docker Deployment
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o multi-cluster-backup-orchestrator

FROM alpine:3.18
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/multi-cluster-backup-orchestrator .
COPY --from=builder /app/config.yaml .
CMD ["./multi-cluster-backup-orchestrator"]
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backup-orchestrator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: backup-orchestrator
  template:
    metadata:
      labels:
        app: backup-orchestrator
    spec:
      serviceAccountName: backup-orchestrator
      containers:
      - name: orchestrator
        image: backup-orchestrator:latest
        env:
        - name: CONFIG_PATH
          value: "/config/backup-config.yaml"
        volumeMounts:
        - name: config
          mountPath: /config
        - name: ca-certs
          mountPath: /etc/ssl/certs
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "8Gi"
            cpu: "4"
      volumes:
      - name: config
        configMap:
          name: backup-orchestrator-config
      - name: ca-certs
        secret:
          secretName: cluster-ca-certs
```

### Service Account & RBAC
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: backup-orchestrator
  namespace: backup-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: backup-orchestrator
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: backup-orchestrator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: backup-orchestrator
subjects:
- kind: ServiceAccount
  name: backup-orchestrator
  namespace: backup-system
```

## Testing

### Unit Tests
```bash
go test ./shared/config -v -run TestMultiClusterBackupOrchestrator
```

### Integration Tests
```bash
go test ./shared/config -v -run TestMultiClusterBackupExecution
```

### Benchmark Tests
```bash
go test ./shared/config -bench=BenchmarkMultiClusterBackupExecution -benchmem
```

## Troubleshooting

### Common Issues

#### Authentication Failures
```
Error: failed to create REST config for cluster prod: invalid authentication
```

**Solutions:**
1. Verify token/credentials are correct
2. Check token expiration
3. Validate TLS configuration
4. Ensure proper RBAC permissions

#### Connection Timeouts
```
Error: context deadline exceeded
```

**Solutions:**
1. Increase timeout values in configuration
2. Check network connectivity
3. Verify DNS resolution
4. Monitor cluster health

#### Resource Exhaustion
```
Error: too many open files
```

**Solutions:**
1. Increase system file descriptor limits
2. Reduce `max_concurrent_clusters`
3. Enable connection pooling
4. Monitor memory usage

### Debug Mode
Enable debug logging for detailed troubleshooting:

```yaml
observability:
  logging:
    level: "debug"
    format: "json"
```

### Health Checks
Monitor orchestrator health:

```bash
# Get orchestrator status
curl http://localhost:8080/metrics

# Check individual cluster health  
curl http://localhost:8080/health/clusters

# Get execution history
curl http://localhost:8080/executions/history
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For support and questions:
- Create an issue on GitHub
- Check the troubleshooting section
- Review the examples and documentation

---

**Note**: This orchestrator builds upon the existing backup infrastructure and enhanced authentication system to provide a comprehensive multi-cluster backup solution suitable for production environments.