# Backup Service

OpenShift/Kubernetes cluster backup service that exports resources to MinIO object storage with structured logging and automatic cleanup.

## Configuration

### Environment Variables (from Secrets)
```bash
# Required
CLUSTER_NAME=my-openshift-cluster
MINIO_ENDPOINT=192.168.1.4:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin123
MINIO_BUCKET=openshift-cluster-backups-3

# Optional
CLUSTER_DOMAIN=cluster.local          # default: cluster.local
MINIO_USE_SSL=false                   # default: true
BATCH_SIZE=50                         # default: 50
RETRY_ATTEMPTS=3                      # default: 3
RETRY_DELAY=5s                        # default: 5s
ENABLE_CLEANUP=true                   # default: true
RETENTION_DAYS=7                      # default: 7
CLEANUP_ON_STARTUP=false              # default: false
LOG_LEVEL=info                        # default: info
POD_NAMESPACE=cluster-backup          # auto-detected
```

### ConfigMap (backup-config)
```yaml
# Filtering mode: whitelist, blacklist, hybrid
filtering-mode: "whitelist"

# Resources to include (whitelist/hybrid mode)
include-resources: |
  deployments
  services
  configmaps
  persistentvolumeclaims
  routes
  buildconfigs
  imagestreams
  deploymentconfigs

# Resources to exclude (blacklist/hybrid mode) 
exclude-resources: |
  events
  nodes
  endpoints
  pods
  replicasets
  secrets

# Namespaces to include (empty = use exclude list)
include-namespaces: |
  berkay-test
  cluster-backup

# Namespaces to exclude (when include-namespaces empty)
exclude-namespaces: |
  openshift
  openshift-*
  kube-*
  default

# OpenShift CRDs to include
include-crds: |
  routes.route.openshift.io
  buildconfigs.build.openshift.io
  imagestreams.image.openshift.io
  deploymentconfigs.apps.openshift.io
  templates.template.openshift.io

# Advanced filtering
label-selector: ""                    # e.g. "app=production"
annotation-selector: ""               # e.g. "backup=enabled"
max-resource-size: "10Mi"
skip-invalid-resources: "true"
validate-yaml: "true"
include-status: "false"
include-managed-fields: "false"
follow-owner-references: "false"

# OpenShift detection
openshift-mode: "auto-detect"         # enabled, disabled, auto-detect
include-openshift-resources: "true"

# Cleanup settings
enable-cleanup: "true"
retention-days: "7"
cleanup-on-startup: "false"

# Performance
batch-size: "50"
retry-attempts: "3"
retry-delay: "5s"
log-level: "info"
```

## How Config is Read

### 1. Main Config (loadConfig)
- **Source**: Environment variables via `getSecretValue()`
- **Priority**: Secret env vars → defaults
- **Location**: main.go:184-235

### 2. Backup Config (loadBackupConfig)
- **Source**: ConfigMap `backup-config` in pod namespace
- **Fallback**: Default configuration if ConfigMap not found
- **Location**: main.go:237-261

### 3. Config Parsing (parseBackupConfig)
- **Method**: String parsing with comma/newline separation
- **Function**: `parseCommaSeparated()` - main.go:362-377
- **Booleans**: String comparison with "true"
- **Location**: main.go:263-328

## Storage Structure

MinIO objects stored as:
```
clusterbackup/{cluster-name}/{namespace}/{resource-type}/{resource-name}.yaml
```

Example:
```
clusterbackup/my-openshift-cluster/berkay-test/deployments/nginx.yaml
clusterbackup/my-openshift-cluster/cluster-backup/services/backup-service.yaml
```

## Build & Run

```bash
# Build
go build -o backup main.go

# Run locally (requires kubectl config)
export CLUSTER_NAME=test
export MINIO_ENDPOINT=localhost:9000
./backup

# Health check
./backup --health-check

# Docker build
docker build -t backup:latest .
```

## Features

- **OpenShift Auto-Detection**: Detects OpenShift via route.openshift.io API
- **Flexible Filtering**: 3 modes (whitelist/blacklist/hybrid) with namespace/resource filters
- **Resource Cleanup**: Removes volatile fields (uid, resourceVersion, etc.)
- **Structured Logging**: JSON logs with operation tracking
- **Prometheus Metrics**: Exposed on :8080/metrics
- **Automatic Cleanup**: Retention-based cleanup with configurable schedule
- **Error Resilience**: Retry logic and graceful error handling

## Monitoring

**Key Metrics:**
- `cluster_backup_duration_seconds`: Backup operation duration
- `cluster_backup_resources_total`: Total resources backed up
- `cluster_backup_errors_total`: Total backup errors
- `cluster_backup_namespaces_total`: Namespaces backed up count
- `cluster_backup_last_success_timestamp`: Last successful backup time

**Log Operations:**
- `startup`, `config_loaded`, `backup_start`
- `openshift_detected`, `minio_ready`
- `api_discovery_complete`, `namespace_discovery_complete`
- `namespace_backup_start`, `resource_type_summary`
- `cleanup_start`, `cleanup_complete`, `backup_complete`