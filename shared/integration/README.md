# Integration Bridge Documentation

The Integration Bridge connects the Go backup tool, Python GitOps generator, and shared configuration components into a unified platform for Kubernetes backup and disaster recovery.

## Overview

The Integration Bridge solves the critical gap between three well-built but isolated components:
- **Go Backup Tool**: Kubernetes resource backup to MinIO
- **Python GitOps Generator**: Backup transformation to GitOps manifests
- **Shared Configuration**: Common configuration and monitoring

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Backup Tool   │    │ Integration     │    │ GitOps Generator│
│   (Go)          │◄──►│ Bridge          │◄──►│ (Python)        │
│                 │    │                 │    │                 │
│ - K8s API       │    │ - Event Bus     │    │ - YAML Gen      │
│ - MinIO Upload  │    │ - Webhooks      │    │ - Git Commit    │
│ - Scheduling    │    │ - Monitoring    │    │ - Transformation│
└─────────────────┘    │ - Health Checks │    └─────────────────┘
                       │ - Config Mgmt   │
                       └─────────────────┘
                                │
                       ┌─────────────────┐
                       │ Shared Config   │
                       │ & Monitoring    │
                       └─────────────────┘
```

## Key Features

### ✅ **Unified Configuration**
- Single YAML configuration for all components
- Environment variable overrides
- Configuration validation and defaults
- Component-specific config generation

### ✅ **Event-Driven Integration**
- Backup completion → GitOps generation trigger
- Cross-language communication (Go ↔ Python)
- Webhook-based events with fallback methods
- Parallel and sequential execution support

### ✅ **Comprehensive Monitoring**
- Health checks across all components
- End-to-end integration metrics
- Performance monitoring and alerting
- Distributed observability

### ✅ **Robust Communication**
- HTTP webhooks with retry logic
- Component registration and discovery
- Circuit breaker patterns
- Graceful degradation

## Quick Start

### 1. Configuration

Create `config.yaml`:

```yaml
schema_version: "2.1.0"
description: "Production backup and GitOps integration"

# Storage (MinIO)
storage:
  endpoint: "${MINIO_ENDPOINT}"
  access_key: "${MINIO_ACCESS_KEY}"
  secret_key: "${MINIO_SECRET_KEY}"
  bucket: "k8s-backups"
  use_ssl: true

# Kubernetes Cluster
cluster:
  name: "${CLUSTER_NAME}"
  kubeconfig_path: "${KUBECONFIG}"

# GitOps Repository
gitops:
  repository: "${GITOPS_REPO}"
  branch: "main"
  path: "clusters/${CLUSTER_NAME}"

# Integration Bridge
integration:
  enabled: true
  webhook_port: 8080
  communication:
    method: "webhook"
    endpoints:
      backup_tool: "http://backup-tool:8080"
      gitops_generator: "http://gitops-generator:8081"
      integration_bridge: "http://integration-bridge:8080"
  triggers:
    auto_trigger: true
    delay_after_backup: "30s"
```

### 2. Start Integration Bridge

```go
package main

import (
    "context"
    "log"
    
    "shared-config/integration"
)

func main() {
    // Load configuration
    configManager := integration.NewConfigManager("config.yaml")
    config, err := configManager.LoadConfig()
    if err != nil {
        log.Fatal("Failed to load config:", err)
    }

    // Create and start bridge
    bridge, err := integration.NewIntegrationBridge(config)
    if err != nil {
        log.Fatal("Failed to create bridge:", err)
    }

    ctx := context.Background()
    if err := bridge.Start(ctx); err != nil {
        log.Fatal("Failed to start bridge:", err)
    }

    log.Println("Integration bridge running on port", config.Integration.WebhookPort)
    
    // Keep running
    select {}
}
```

### 3. Component Integration

#### Backup Tool Integration

```go
// Register with bridge
client := integration.NewBackupClient(config, monitoringClient)
err := client.RegisterWithBridge(ctx, bridgeURL, "1.0.0")

// Notify completion
backupEvent := &integration.BackupCompletionEvent{
    BackupID:      backupID,
    ClusterName:   clusterName,
    Success:       true,
    MinIOPath:     backupPath,
    ResourceCount: resourceCount,
    Size:          sizeBytes,
}
err = client.NotifyCompletion(ctx, bridgeURL, backupEvent)
```

#### GitOps Generator Integration

```python
import asyncio
from integration.gitops_client import GitOpsClient, process_backup_completion

async def main():
    config = load_config("config.yaml")
    
    # Register with bridge
    async with GitOpsClient(config) as client:
        await client.register_with_bridge("2.1.0")
        
        # Process backup events (called by bridge)
        backup_event = {
            "backup_id": "backup-123",
            "cluster_name": "production",
            "minio_path": "production/2024/01/15/backup-123"
        }
        
        status = await process_backup_completion(config, backup_event)
        print(f"GitOps generation completed: {status.status}")

if __name__ == "__main__":
    asyncio.run(main())
```

## API Reference

### Integration Bridge Endpoints

#### Health Check
```
GET /health
```
Returns bridge and component health status.

#### Component Registration
```
POST /register/backup
POST /register/gitops
```
Register backup tool or GitOps generator with the bridge.

#### Webhook Events
```
POST /webhooks/backup/completed
POST /webhooks/gitops/generate
POST /webhooks/gitops/completed
```
Process backup completion, GitOps generation requests, and completion notifications.

#### Status
```
GET /status
```
Get current status of all registered components.

### Configuration API

```go
// Load and validate configuration
configManager := integration.NewConfigManager("config.yaml")
config, err := configManager.LoadConfig()

// Get component-specific configs
backupConfig := configManager.CreateBackupToolConfig()
gitopsConfig := configManager.CreateGitOpsConfig()
bridgeConfig := configManager.CreateIntegrationBridgeConfig()
```

### Monitoring API

```go
// Get integrated metrics
metrics := bridge.GetIntegratedMetrics()
fmt.Printf("Success Rate: %.2f%%\n", 
    float64(metrics.TotalSuccesses) / float64(metrics.TotalRequests) * 100)

// Get component health
health, err := bridge.GetComponentHealth("backup-tool")
fmt.Printf("Backup Tool Status: %s\n", health.HealthStatus)

// Get overall health
overall := bridge.GetOverallHealth()
fmt.Printf("System Health: %s (%d/%d healthy)\n", 
    overall.OverallStatus, overall.HealthyComponents, overall.TotalComponents)
```

## Deployment

### Docker Compose

```yaml
version: '3.8'
services:
  integration-bridge:
    build: ./integration-bridge
    ports:
      - "8080:8080"
    environment:
      - MINIO_ENDPOINT=minio:9000
      - CLUSTER_NAME=production
      - GITOPS_REPO=https://github.com/org/gitops
    volumes:
      - ./config.yaml:/app/config.yaml
    depends_on:
      - minio

  backup-tool:
    build: ./backup
    environment:
      - INTEGRATION_BRIDGE_ENDPOINT=http://integration-bridge:8080
    volumes:
      - ~/.kube:/root/.kube:ro

  gitops-generator:
    build: ./kOTN
    environment:
      - INTEGRATION_BRIDGE_ENDPOINT=http://integration-bridge:8080
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: integration-bridge
spec:
  replicas: 1
  selector:
    matchLabels:
      app: integration-bridge
  template:
    metadata:
      labels:
        app: integration-bridge
    spec:
      containers:
      - name: bridge
        image: integration-bridge:latest
        ports:
        - containerPort: 8080
        env:
        - name: MINIO_ENDPOINT
          value: "minio.storage.svc.cluster.local:9000"
        - name: CLUSTER_NAME
          value: "production"
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
      volumes:
      - name: config
        configMap:
          name: integration-config
---
apiVersion: v1
kind: Service
metadata:
  name: integration-bridge
spec:
  selector:
    app: integration-bridge
  ports:
  - port: 8080
    targetPort: 8080
```

## Monitoring and Observability

### Metrics

The integration bridge exposes comprehensive metrics:

```
# Request metrics
integration_total_requests{component="backup-tool"} 150
integration_total_errors{component="backup-tool"} 2
integration_success_rate_percent 98.67

# Health metrics
integration_healthy_components 3
integration_total_components 3

# Flow metrics
integration_flow_total_requests 45
integration_flow_successful 43
integration_backup_to_gitops_latency_seconds 45.2
```

### Health Checks

```bash
# Check overall health
curl http://localhost:8080/health

# Check component status
curl http://localhost:8080/status

# Response format
{
  "success": true,
  "message": "All components healthy",
  "data": {
    "overall_status": "healthy",
    "healthy_components": 3,
    "total_components": 3,
    "components": {
      "backup-tool": {
        "status": "healthy",
        "version": "1.0.0",
        "last_check": "2024-01-15T10:30:00Z"
      }
    }
  }
}
```

### Alerting

Configure alerts for key metrics:

```yaml
# Prometheus AlertManager
groups:
- name: integration-bridge
  rules:
  - alert: IntegrationBridgeDown
    expr: up{job="integration-bridge"} == 0
    for: 1m
    annotations:
      summary: "Integration bridge is down"
      
  - alert: HighIntegrationFailureRate
    expr: rate(integration_flow_failed[5m]) > 0.1
    for: 2m
    annotations:
      summary: "High integration failure rate"
      
  - alert: ComponentUnhealthy
    expr: integration_unhealthy_components > 0
    for: 1m
    annotations:
      summary: "{{ $value }} components are unhealthy"
```

## Troubleshooting

### Common Issues

#### 1. Component Registration Failures
```bash
# Check bridge connectivity
curl -X POST http://localhost:8080/register/backup \
  -H "Content-Type: application/json" \
  -d '{"endpoint": "http://backup-tool:8080", "version": "1.0.0"}'

# Response should be:
{"success": true, "message": "Backup tool registered successfully"}
```

#### 2. Backup → GitOps Flow Not Triggering
```bash
# Check event bus subscriptions
curl http://localhost:8080/status

# Verify webhook endpoint accessibility
curl -X POST http://localhost:8080/webhooks/backup/completed \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-1",
    "type": "backup_completed", 
    "source": "backup-tool",
    "data": {
      "backup_id": "test",
      "cluster_name": "test",
      "success": true,
      "minio_path": "test/path"
    }
  }'
```

#### 3. Configuration Issues
```bash
# Validate configuration
go run main.go --validate-config

# Check environment variables
env | grep -E "(MINIO|CLUSTER|GITOPS)"

# Test configuration loading
go run -c 'cm := integration.NewConfigManager(); cm.LoadConfig()'
```

### Debug Mode

Enable debug logging:

```yaml
observability:
  logging:
    level: "debug"
    format: "json"
```

```bash
# View integration events
docker logs integration-bridge | grep "integration_event"

# Monitor webhook calls
docker logs integration-bridge | grep "webhook_request"
```

## Best Practices

### Configuration Management
- Use environment variables for secrets
- Validate configuration on startup
- Implement configuration hot-reloading for non-sensitive changes
- Keep component-specific configs in sync

### Reliability
- Implement circuit breakers for external calls
- Use exponential backoff for retries
- Monitor and alert on integration flow health
- Test disaster recovery scenarios

### Security
- Use TLS for all component communication
- Implement authentication for webhook endpoints
- Rotate secrets regularly
- Validate all incoming webhook data

### Performance
- Monitor backup → GitOps latency
- Implement batch processing for multiple backups
- Use connection pooling for HTTP clients
- Optimize event bus throughput

## Development

### Building

```bash
# Build integration bridge
cd shared/integration
go build -o bridge ./cmd/bridge

# Run tests
go test ./...

# Run integration tests
go test -tags=integration ./...
```

### Testing

```bash
# Unit tests
go test ./integration/

# Integration tests with Docker
docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit

# Load testing
go test -bench=. ./integration/
```

### Contributing

1. Follow the existing code structure
2. Add tests for new functionality
3. Update documentation
4. Ensure backward compatibility
5. Test with all three components

## License

This integration bridge is part of the backup-to-GitOps platform and follows the same licensing as the parent project.