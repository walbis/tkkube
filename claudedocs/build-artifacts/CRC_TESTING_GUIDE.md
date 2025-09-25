# CRC Testing Guide - Multi-Cluster Backup System

## Overview
This guide provides comprehensive instructions for testing the Multi-Cluster Backup System in a CodeReady Containers (CRC) environment.

## Prerequisites

### 1. CRC Environment
```bash
# Verify CRC is running
crc status

# Ensure sufficient resources
crc config set memory 16384
crc config set cpus 4
crc config set disk-size 100
```

### 2. Required Tools
- `oc` CLI tool (OpenShift Client)
- `kubectl` (Kubernetes CLI)
- `go` v1.24+ (for building components)
- MinIO client (`mc`) for storage testing

## Testing Architecture

### Component Structure
```
┌─────────────────┐    ┌─────────────────┐
│   CRC Cluster   │    │  Backup System  │
│                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ Workloads   │ │◄───┤ │Orchestrator │ │
│ │ - Pods      │ │    │ │             │ │
│ │ - Services  │ │    │ └─────────────┘ │
│ │ - ConfigMaps│ │    │ ┌─────────────┐ │
│ └─────────────┘ │    │ │ Validation  │ │
│                 │    │ │ Service     │ │
│ ┌─────────────┐ │    │ └─────────────┘ │
│ │ Storage     │ │    │ ┌─────────────┐ │
│ │ - PVCs      │ │◄───┤ │ Auth Manager│ │
│ │ - Secrets   │ │    │ │             │ │
│ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘
```

## Phase 1: Environment Setup

### 1.1 Deploy MinIO Storage Backend
```bash
# Create MinIO namespace
oc new-project minio-storage

# Deploy MinIO server
oc apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
  namespace: minio-storage
spec:
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
      - name: minio
        image: minio/minio:latest
        command:
        - /bin/bash
        - -c
        args:
        - minio server /data --console-address :9090
        env:
        - name: MINIO_ROOT_USER
          value: "testuser"
        - name: MINIO_ROOT_PASSWORD
          value: "testpassword123"
        ports:
        - containerPort: 9000
        - containerPort: 9090
        volumeMounts:
        - name: storage
          mountPath: "/data"
      volumes:
      - name: storage
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: minio-service
  namespace: minio-storage
spec:
  selector:
    app: minio
  ports:
    - name: api
      port: 9000
      targetPort: 9000
    - name: console
      port: 9090
      targetPort: 9090
  type: ClusterIP
EOF
```

### 1.2 Create Test Workloads
```bash
# Create test namespace
oc new-project backup-test

# Deploy sample applications
oc apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: backup-test
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: backup-test
spec:
  selector:
    app: test-app
  ports:
    - port: 80
      targetPort: 80
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: backup-test
data:
  app.properties: |
    environment=test
    debug=true
---
apiVersion: v1
kind: Secret
metadata:
  name: test-secret
  namespace: backup-test
data:
  password: dGVzdHBhc3N3b3JkMTIz
EOF
```

## Phase 2: Build and Deploy Backup System

### 2.1 Build Backup System
```bash
cd /home/tkkaray/inceleme

# Build backup orchestrator
cd shared
go build -o ../claudedocs/build-artifacts/backup-orchestrator \
  ./config/multi_cluster_backup_orchestrator.go \
  ./config/multi_cluster_manager.go \
  ./config/loader.go

# Build validation service  
go build -o ../claudedocs/build-artifacts/validation-service \
  ./config/live_validation_service.go \
  ./config/enhanced_multi_cluster_validator.go

# Build main backup tool
cd ../backup
go build -o ../claudedocs/build-artifacts/backup-tool ./cmd/backup/
```

### 2.2 Create Configuration
```bash
# Generate CRC-specific configuration
cat > /home/tkkaray/inceleme/claudedocs/build-artifacts/crc-backup-config.yaml <<EOF
# Multi-Cluster Backup Configuration for CRC Testing
enabled: true
mode: "sequential"  # Use sequential for single cluster testing
default_cluster: "crc-cluster"

coordination:
  timeout_seconds: 300
  retry_attempts: 3
  health_check_interval: 30s

clusters:
  - name: "crc-cluster"
    endpoint: "https://api.crc.testing:6443"
    auth:
      method: "token"
      token:
        value: "${CRC_TOKEN}"  # Set via environment variable
    storage:
      type: "minio"
      minio:
        endpoint: "minio-service.minio-storage.svc.cluster.local:9000"
        access_key: "testuser"
        secret_key: "testpassword123"
        bucket: "cluster-backups"
        use_ssl: false
    backup_scope:
      include_namespaces:
        - "backup-test"
        - "minio-storage"
      exclude_namespaces:
        - "kube-system"
        - "openshift-*"
      resource_types:
        - "deployments"
        - "services"
        - "configmaps"
        - "secrets"
        - "persistentvolumeclaims"

validation:
  enabled: true
  timeout_seconds: 60
  connectivity_checks:
    - network
    - tls
    - api_auth
  token_validation: true
EOF
```

## Phase 3: Testing Scenarios

### 3.1 Connectivity Validation Test
```bash
# Set up environment
export CRC_TOKEN=$(oc whoami -t)

# Test 1: Validate cluster connectivity
echo "=== Testing Cluster Connectivity ==="
cd /home/tkkaray/inceleme/shared
go run ./config/enhanced_multi_cluster_validator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml \
  -validate-only

# Expected: All connectivity checks should pass
```

### 3.2 Authentication Test
```bash
# Test 2: Token authentication validation  
echo "=== Testing Token Authentication ==="
go run ./config/cluster_auth_test.go

# Expected: Bearer token validation should succeed
```

### 3.3 Storage Backend Test
```bash
# Test 3: MinIO storage connectivity
echo "=== Testing Storage Backend ==="

# Install MinIO client in CRC
oc run minio-client --rm -i --tty --image=minio/mc -- bash

# Inside the pod:
mc alias set local http://minio-service.minio-storage.svc.cluster.local:9000 testuser testpassword123
mc mb local/cluster-backups
mc ls local/

# Expected: Bucket creation and listing should succeed
```

### 3.4 Backup Orchestration Test
```bash
# Test 4: Full backup execution
echo "=== Testing Backup Orchestration ==="

# Run backup orchestrator
cd /home/tkkaray/inceleme/shared
export CRC_TOKEN=$(oc whoami -t)
go run ./config/multi_cluster_backup_orchestrator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml \
  -dry-run=false

# Expected outcomes:
# 1. Cluster health check passes
# 2. Namespace discovery finds backup-test, minio-storage
# 3. Resource enumeration finds deployments, services, etc.
# 4. Backup artifacts uploaded to MinIO
# 5. Backup validation confirms integrity
```

### 3.5 Live Validation Service Test
```bash
# Test 5: HTTP API validation service
echo "=== Testing Live Validation Service ==="

# Start validation service
go run ./config/live_validation_service.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml \
  -port 8080 &

# Test API endpoints
curl http://localhost:8080/health
curl http://localhost:8080/validation/status
curl http://localhost:8080/clusters
curl http://localhost:8080/metrics

# Expected: All endpoints return valid JSON responses
```

## Phase 4: Performance Testing

### 4.1 Load Testing
```bash
# Create multiple test namespaces
for i in {1..5}; do
  oc new-project test-ns-$i
  oc apply -n test-ns-$i -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: load-test-$i
spec:
  replicas: 3
  selector:
    matchLabels:
      app: load-test-$i
  template:
    metadata:
      labels:
        app: load-test-$i
    spec:
      containers:
      - name: busybox
        image: busybox
        command: ['sleep', '3600']
EOF
done

# Run performance test
time go run ./config/multi_cluster_backup_orchestrator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml

# Expected: Backup completion under 5 minutes for ~50 resources
```

### 4.2 Concurrent Operations Test
```bash
# Test parallel validation
echo "=== Testing Concurrent Validation ==="
go run ./config/enhanced_multi_cluster_validator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml \
  -parallel=true \
  -concurrent-checks=5

# Expected: Faster validation with concurrent checks
```

## Phase 5: Failure Scenario Testing

### 5.1 Network Failure Simulation
```bash
# Temporarily block MinIO access
oc patch service minio-service -n minio-storage -p '{"spec":{"selector":{"app":"blocked"}}}'

# Run backup - should trigger circuit breaker
go run ./config/multi_cluster_backup_orchestrator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml

# Restore service
oc patch service minio-service -n minio-storage -p '{"spec":{"selector":{"app":"minio"}}}'

# Expected: Circuit breaker activates, graceful failure handling
```

### 5.2 Authentication Failure Test
```bash
# Use invalid token
export CRC_TOKEN="invalid-token"

# Run validation
go run ./config/enhanced_multi_cluster_validator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml

# Expected: Token validation failure with detailed error message
```

## Phase 6: Monitoring and Observability

### 6.1 Metrics Collection
```bash
# Deploy Prometheus (optional)
oc new-project monitoring
oc apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml

# Configure backup system metrics endpoint
# Metrics available at: http://localhost:8081/metrics
```

### 6.2 Log Analysis
```bash
# Enable structured logging
export LOG_LEVEL=DEBUG
export LOG_FORMAT=json

# Run with detailed logging
go run ./config/multi_cluster_backup_orchestrator.go \
  -config ../claudedocs/build-artifacts/crc-backup-config.yaml 2>&1 | jq .

# Expected: Structured JSON logs with timing information
```

## Success Criteria

### ✅ Must Pass
1. **Connectivity**: All cluster connectivity checks pass
2. **Authentication**: Token validation succeeds
3. **Storage**: MinIO backend accessible and functional  
4. **Backup**: At least 1 complete backup/restore cycle
5. **Validation**: API endpoints respond correctly

### ✅ Should Pass
1. **Performance**: Backup completes in <5 minutes for test workload
2. **Resilience**: Circuit breaker activates on failures
3. **Monitoring**: Metrics and logs provide operational visibility
4. **Concurrency**: Parallel operations improve performance

### ⚠️ Known Limitations
1. **Single Cluster**: CRC provides only one cluster for testing
2. **Storage**: EmptyDir volumes don't persist across restarts
3. **Network**: Limited to cluster-internal networking
4. **Resources**: CRC resource constraints may affect performance

## Troubleshooting

### Common Issues
1. **"cluster unreachable"**: Check CRC status and token validity
2. **"storage connection failed"**: Verify MinIO service is running
3. **"permission denied"**: Ensure proper RBAC permissions
4. **"timeout"**: Increase timeout values in configuration

### Debug Commands
```bash
# Check cluster status
crc status
oc cluster-info

# Check backup system logs
oc logs -f deployment/backup-orchestrator

# Check MinIO connectivity
oc port-forward svc/minio-service 9000:9000 -n minio-storage
# Browse to http://localhost:9000

# Check resource discovery
oc api-resources | head -20
```

## Expected Test Results

After successful completion, you should have:

1. **Backup Artifacts**: 
   - MinIO bucket with timestamped backup files
   - Resource manifests in YAML format
   - Metadata and validation checksums

2. **Performance Metrics**:
   - Backup duration: <5 minutes
   - Validation time: <30 seconds  
   - Resource discovery: <10 seconds

3. **Operational Data**:
   - Structured logs with timing information
   - Prometheus metrics (if configured)
   - Health check status history

---
**Testing Guide Version**: 1.0  
**Last Updated**: 2025-09-25  
**Compatibility**: CRC 2.x, OpenShift 4.x, Kubernetes 1.24+