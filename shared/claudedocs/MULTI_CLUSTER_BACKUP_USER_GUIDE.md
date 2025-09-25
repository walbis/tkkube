# Multi-Cluster Backup System - Complete User Guide

**Version**: 1.0  
**Date**: 2025-09-25  
**Compatibility**: Kubernetes 1.24+, OpenShift 4.x  

## 📖 Table of Contents

1. [Introduction](#introduction)
2. [Prerequisites](#prerequisites)  
3. [Installation & Setup](#installation--setup)
4. [Configuration](#configuration)
5. [Basic Operations](#basic-operations)
6. [Advanced Usage](#advanced-usage)
7. [Monitoring & Troubleshooting](#monitoring--troubleshooting)
8. [Security Best Practices](#security-best-practices)
9. [Production Deployment](#production-deployment)
10. [Reference](#reference)

---

## 📚 Introduction

The Multi-Cluster Backup System is an enterprise-grade solution for backing up Kubernetes resources across multiple clusters simultaneously. It provides:

- **Multi-Cluster Support**: Backup multiple Kubernetes clusters in parallel or sequential mode
- **S3-Compatible Storage**: Support for AWS S3, MinIO, and other S3-compatible storage
- **GitOps Integration**: Automatic conversion of backups to GitOps artifacts
- **Disaster Recovery**: Built-in disaster recovery simulation and validation
- **Enterprise Security**: RBAC, encryption, and compliance features
- **Real-time Monitoring**: Comprehensive monitoring with Prometheus metrics

### 🎯 Use Cases

- **Enterprise Multi-Environment**: Backup dev, staging, and production clusters
- **Multi-Region Deployments**: Backup clusters across different geographical regions
- **Hybrid Cloud**: Support for on-premises and cloud clusters
- **Disaster Recovery**: Automated backup and restore for business continuity
- **Compliance**: Automated backup with audit trails for regulatory compliance

---

## ⚙️ Prerequisites

### System Requirements

- **Kubernetes Access**: kubectl configured for all target clusters
- **Go Runtime**: Go 1.19+ for backup executor
- **Storage Access**: S3-compatible storage (AWS S3, MinIO, etc.)
- **Git Access**: Git repository for GitOps artifacts
- **Network Connectivity**: HTTPS access to all cluster APIs

### Cluster Requirements

Each target cluster must have:
- **API Access**: Valid kubeconfig with cluster-admin or read permissions
- **Network Access**: Clusters must be reachable from backup system
- **Storage**: Sufficient storage for temporary backup files
- **RBAC**: Service account with required permissions

### Resource Requirements

- **CPU**: 2+ cores for sequential, 4+ cores for parallel execution  
- **Memory**: 4GB minimum, 8GB recommended for large clusters
- **Storage**: 50GB+ for temporary files and cached backups
- **Network**: Stable connection with sufficient bandwidth

---

## 🚀 Installation & Setup

### Step 1: Download and Setup

```bash
# Clone the repository
git clone <repository-url>
cd shared/production-simulation

# Verify all components are present
ls -la *.sh *.go *.yaml

# Make scripts executable
chmod +x *.sh
```

### Step 2: Configure Environment

Create environment configuration file:

```bash
# Copy example configuration
cp /home/tkkaray/inceleme/shared/config/multi-cluster-example.yaml my-config.yaml

# Edit configuration for your environment
vi my-config.yaml
```

### Step 3: Test Cluster Connectivity

```bash
# Verify kubectl access to all clusters
kubectl cluster-info --context=prod-us-east-1
kubectl cluster-info --context=prod-eu-west-1
kubectl cluster-info --context=staging-us-east-1
```

### Step 4: Setup Storage

#### For MinIO (Development/Testing)
```bash
# Deploy MinIO using the environment setup
./environment-setup.sh

# Verify MinIO deployment
kubectl get pods -n minio-system
```

#### For AWS S3 (Production)
```bash
# Configure AWS credentials
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_DEFAULT_REGION="us-east-1"
```

### Step 5: Initial Validation

```bash
# Run setup validation
./validate-setup.sh

# Expected output: All checks should pass ✅
```

---

## 🔧 Configuration

### Basic Multi-Cluster Configuration

Edit `my-config.yaml` to define your clusters:

```yaml
multi_cluster:
  enabled: true
  mode: "parallel"  # or "sequential"
  
  clusters:
    - name: "production-us-east"
      endpoint: "https://api.prod-us-east.company.com:6443"
      token: "${PROD_US_EAST_TOKEN}"
      storage:
        type: "s3"
        bucket: "prod-us-east-backups"
        region: "us-east-1"
    
    - name: "production-eu-west"
      endpoint: "https://api.prod-eu-west.company.com:6443"
      token: "${PROD_EU_WEST_TOKEN}"
      storage:
        type: "s3"
        bucket: "prod-eu-west-backups"
        region: "eu-west-1"
```

### Authentication Setup

#### Using Kubernetes Service Account Tokens
```bash
# Create service account in each cluster
kubectl create serviceaccount backup-sa -n default

# Create cluster role binding
kubectl create clusterrolebinding backup-sa-binding \
  --clusterrole=cluster-admin \
  --serviceaccount=default:backup-sa

# Get token
kubectl get secret $(kubectl get sa backup-sa -o jsonpath='{.secrets[0].name}') \
  -o jsonpath='{.data.token}' | base64 -d
```

#### Using Environment Variables
```bash
# Set cluster tokens
export PROD_US_EAST_TOKEN="eyJhbGciOiJSUzI1NiIs..."
export PROD_EU_WEST_TOKEN="eyJhbGciOiJSUzI1NiIs..."
export STAGING_US_EAST_TOKEN="eyJhbGciOiJSUzI1NiIs..."

# Set storage credentials
export PROD_US_EAST_ACCESS_KEY="AKIA..."
export PROD_US_EAST_SECRET_KEY="..."
```

### Storage Configuration

#### S3-Compatible Storage Settings
```yaml
storage:
  type: "s3"  # or "minio"
  endpoint: "s3.amazonaws.com"  # or your MinIO endpoint
  access_key: "${AWS_ACCESS_KEY_ID}"
  secret_key: "${AWS_SECRET_ACCESS_KEY}"
  bucket: "my-cluster-backups"
  use_ssl: true
  region: "us-east-1"
```

### Backup Filtering

Configure what to backup from each cluster:

```yaml
backup:
  filtering:
    mode: "whitelist"
    resources:
      include:
        - deployments
        - services
        - configmaps
        - secrets
        - persistentvolumeclaims
        - statefulsets
        - ingresses
      exclude:
        - events
        - pods
        - replicasets
    namespaces:
      exclude:
        - kube-system
        - kube-public
        - kube-node-lease
```

---

## 💼 Basic Operations

### Running Your First Multi-Cluster Backup

#### Option 1: Complete Automated Backup

```bash
# Run complete backup and GitOps pipeline
./master-orchestrator.sh run

# This will:
# 1. Validate cluster connectivity
# 2. Execute backup on all configured clusters
# 3. Upload backups to S3 storage
# 4. Generate GitOps artifacts
# 5. Create monitoring reports
```

#### Option 2: Step-by-Step Execution

```bash
# Step 1: Setup environment
./environment-setup.sh

# Step 2: Deploy test workloads (optional)
./deploy-workloads.sh

# Step 3: Execute multi-cluster backup
go run enhanced-backup-executor.go

# Step 4: Generate GitOps artifacts
./gitops-pipeline-orchestrator.sh

# Step 5: Start monitoring
./start-validation-framework.sh start
```

### Checking Backup Status

```bash
# Check orchestration status
./master-orchestrator.sh status

# View backup results
./master-orchestrator.sh report

# Check cluster health
kubectl get nodes --context=prod-us-east-1
kubectl get nodes --context=prod-eu-west-1
```

### Viewing Backup Contents

```bash
# List backups in storage
aws s3 ls s3://prod-us-east-backups/

# Download specific backup
aws s3 cp s3://prod-us-east-backups/backup-2025-09-25-123456.tar.gz ./

# Extract and examine backup
tar -xzf backup-2025-09-25-123456.tar.gz
ls -la backup-contents/
```

---

## 🔧 Advanced Usage

### Parallel vs Sequential Execution

#### Parallel Mode (Recommended for Performance)
```yaml
multi_cluster:
  mode: "parallel"
  scheduling:
    max_concurrent_clusters: 3
```

```bash
# Execute parallel backup
CONFIG_MODE=parallel ./master-orchestrator.sh run
```

**Pros**: Faster execution, efficient resource usage  
**Cons**: Higher resource consumption, complex error handling

#### Sequential Mode (Recommended for Reliability)
```yaml
multi_cluster:
  mode: "sequential"
  coordination:
    failure_threshold: 1  # Stop on first failure
```

```bash
# Execute sequential backup
CONFIG_MODE=sequential ./master-orchestrator.sh run
```

**Pros**: Predictable resource usage, easier error isolation  
**Cons**: Slower execution, single point of failure

### Custom Backup Scheduling

#### Cron-based Scheduling
```bash
# Add to crontab for daily 2 AM backup
0 2 * * * /path/to/master-orchestrator.sh run

# Weekly full backup on Sundays
0 1 * * 0 /path/to/master-orchestrator.sh run --full-backup
```

#### Kubernetes CronJob
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: multi-cluster-backup
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup-executor
            image: multi-cluster-backup:latest
            command: ["./master-orchestrator.sh", "run"]
            envFrom:
            - secretRef:
                name: backup-credentials
          restartPolicy: OnFailure
```

### Selective Cluster Backup

#### Backup Specific Clusters
```bash
# Backup only production clusters
CLUSTER_FILTER="prod-*" ./master-orchestrator.sh run

# Backup single cluster
CLUSTERS="prod-us-east-1" ./master-orchestrator.sh run
```

#### Environment-Based Backup
```bash
# Production only
ENVIRONMENT=production ./master-orchestrator.sh run

# Staging and development
ENVIRONMENT="staging,development" ./master-orchestrator.sh run
```

### Disaster Recovery Testing

#### Simulate Disaster Scenarios
```bash
# Run all disaster scenarios
./disaster-recovery-simulator.sh

# Run specific scenario
./disaster-recovery-simulator.sh --scenario node-failure

# Custom disaster test
./disaster-recovery-simulator.sh --custom \
  --delete-namespace app-namespace \
  --cluster prod-us-east-1
```

#### Restore Testing
```bash
# Test restore capability
./master-orchestrator.sh restore \
  --backup-id backup-2025-09-25-123456 \
  --target-cluster staging-us-east-1 \
  --dry-run
```

---

## 📊 Monitoring & Troubleshooting

### Real-time Monitoring

#### Start Validation Framework
```bash
# Start monitoring service
./start-validation-framework.sh start

# Check framework status
./start-validation-framework.sh status

# View monitoring endpoints
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

#### Prometheus Integration
```bash
# Add Prometheus scrape config
- job_name: 'multi-cluster-backup'
  static_configs:
  - targets: ['localhost:8080']
  scrape_interval: 30s
  metrics_path: '/metrics'
```

### Log Analysis

#### Centralized Logging
```bash
# View orchestration logs
tail -f orchestration-*.log

# Search for errors across all logs
grep -r "ERROR" phase*.log

# View specific cluster backup logs
grep "prod-us-east-1" phase3-backup.log
```

#### JSON Structured Logs
```bash
# Parse JSON logs with jq
cat orchestration-report-*.json | jq '.phases[].status'

# Find failed operations
cat orchestration-report-*.json | jq '.phases[] | select(.status != "success")'
```

### Common Issues and Solutions

#### Issue 1: Cluster Connection Timeout
```bash
# Symptom
ERROR: Failed to connect to cluster prod-us-east-1: context deadline exceeded

# Diagnosis
kubectl cluster-info --context=prod-us-east-1

# Solution 1: Check network connectivity
curl -k https://api.prod-us-east-1.company.com:6443/version

# Solution 2: Increase timeout
export CLUSTER_TIMEOUT=300s
```

#### Issue 2: S3 Upload Failures
```bash
# Symptom
ERROR: Failed to upload backup to S3: access denied

# Diagnosis
aws s3 ls s3://prod-us-east-backups/

# Solution 1: Check credentials
aws sts get-caller-identity

# Solution 2: Verify bucket permissions
aws s3api get-bucket-policy --bucket prod-us-east-backups
```

#### Issue 3: Out of Memory During Large Backups
```bash
# Symptom
ERROR: runtime: out of memory

# Solution 1: Increase memory limits
export GOMAXPROCS=4
export BACKUP_BATCH_SIZE=10

# Solution 2: Enable resource filtering
export BACKUP_SIZE_LIMIT=10Mi
export SKIP_LARGE_RESOURCES=true
```

#### Issue 4: GitOps Generation Failures
```bash
# Symptom
ERROR: Failed to generate Kustomize manifests

# Diagnosis
kustomize build ./gitops-artifacts/base/

# Solution
# Fix YAML syntax issues
yamllint ./gitops-artifacts/base/*.yaml
```

### Performance Tuning

#### Optimize for Speed
```bash
# Enable parallel processing
export MAX_CONCURRENT_CLUSTERS=5
export BACKUP_COMPRESSION=false
export VALIDATION_PARALLEL=true
```

#### Optimize for Large Clusters
```bash
# Batch processing
export BACKUP_BATCH_SIZE=50
export MEMORY_LIMIT=8Gi
export ENABLE_STREAMING=true
```

#### Optimize for Network Constraints
```bash
# Reduce network usage
export BACKUP_COMPRESSION=true
export INCREMENTAL_BACKUP=true
export DELTA_SYNC=true
```

---

## 🔒 Security Best Practices

### Authentication & Authorization

#### Service Account Setup
```bash
# Create dedicated backup service account
kubectl create serviceaccount backup-service-account

# Create custom cluster role with minimal permissions
kubectl create clusterrole backup-reader \
  --verb=get,list \
  --resource=deployments,services,configmaps,secrets,persistentvolumeclaims

# Bind role to service account
kubectl create clusterrolebinding backup-service-binding \
  --clusterrole=backup-reader \
  --serviceaccount=default:backup-service-account
```

#### Token Management
```bash
# Use short-lived tokens
export TOKEN_REFRESH_INTERVAL=3600  # 1 hour

# Store tokens securely
echo "${CLUSTER_TOKEN}" | base64 > .cluster-token.enc
chmod 600 .cluster-token.enc

# Rotate tokens regularly
./rotate-cluster-tokens.sh --interval weekly
```

### Data Protection

#### Encryption at Rest
```yaml
backup:
  security:
    encryption:
      enabled: true
      algorithm: "AES-256-GCM"
      key_source: "env"  # or "vault", "kms"
```

#### Encryption in Transit
```yaml
multi_cluster:
  clusters:
    - name: "production"
      endpoint: "https://api.prod.company.com:6443"  # Always HTTPS
      tls_verify: true
      ca_bundle: "/path/to/ca-bundle.pem"
```

#### Secret Scanning
```bash
# Enable secret scanning before backup
export SCAN_FOR_SECRETS=true
export SECRET_PATTERNS="password,token,key,secret"

# Exclude secrets from backup
export EXCLUDE_SECRET_VALUES=true
```

### Network Security

#### Network Policies
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: backup-executor-policy
spec:
  podSelector:
    matchLabels:
      app: backup-executor
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
  - to: []  # Allow API server access
    ports:
    - protocol: TCP
      port: 6443
```

#### Firewall Rules
```bash
# Allow backup executor to cluster APIs
iptables -A OUTPUT -p tcp --dport 6443 -j ACCEPT

# Allow S3 access
iptables -A OUTPUT -p tcp --dport 443 -j ACCEPT
```

### Compliance and Auditing

#### Audit Logging
```yaml
observability:
  audit:
    enabled: true
    log_format: "json"
    include_request_body: false
    events:
      - "backup_started"
      - "backup_completed"
      - "backup_failed"
      - "cluster_accessed"
```

#### Compliance Reporting
```bash
# Generate compliance report
./master-orchestrator.sh compliance-report

# Check for compliance violations
./master-orchestrator.sh check-compliance \
  --standard SOC2 \
  --output compliance-report.json
```

---

## 🏗️ Production Deployment

### High Availability Setup

#### Multi-Instance Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: multi-cluster-backup
spec:
  replicas: 3
  selector:
    matchLabels:
      app: multi-cluster-backup
  template:
    metadata:
      labels:
        app: multi-cluster-backup
    spec:
      containers:
      - name: backup-executor
        image: multi-cluster-backup:1.0
        resources:
          requests:
            memory: "2Gi"
            cpu: "1000m"
          limits:
            memory: "4Gi"
            cpu: "2000m"
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: multi-cluster-backup
            topologyKey: kubernetes.io/hostname
```

#### Leader Election
```go
// Enable leader election for backup coordination
config := &MultiClusterConfig{
    LeaderElection: &LeaderElectionConfig{
        Enabled:       true,
        LockName:      "multi-cluster-backup-lock",
        LockNamespace: "backup-system",
        LeaseDuration: 30 * time.Second,
    },
}
```

### Monitoring and Alerting

#### Prometheus Alerts
```yaml
groups:
- name: multi-cluster-backup
  rules:
  - alert: BackupFailed
    expr: backup_status{status="failed"} > 0
    for: 0m
    annotations:
      summary: "Multi-cluster backup failed"
      description: "Backup failed for cluster {{ $labels.cluster }}"
  
  - alert: BackupDuration
    expr: backup_duration_seconds > 3600
    for: 0m
    annotations:
      summary: "Backup taking too long"
      description: "Backup duration exceeds 1 hour for cluster {{ $labels.cluster }}"
```

#### Grafana Dashboard
```json
{
  "dashboard": {
    "title": "Multi-Cluster Backup Dashboard",
    "panels": [
      {
        "title": "Backup Success Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(backup_success_total[5m]) / rate(backup_attempts_total[5m]) * 100"
          }
        ]
      },
      {
        "title": "Backup Duration",
        "type": "graph",
        "targets": [
          {
            "expr": "backup_duration_seconds"
          }
        ]
      }
    ]
  }
}
```

### Disaster Recovery

#### Backup Strategy
```bash
# Primary backup (daily)
0 2 * * * /usr/local/bin/master-orchestrator.sh run

# Secondary backup to different region (weekly)
0 3 * * 0 /usr/local/bin/master-orchestrator.sh run \
  --storage-region eu-west-1 \
  --backup-type weekly

# DR site backup (monthly)
0 4 1 * * /usr/local/bin/master-orchestrator.sh run \
  --target-storage dr-backup-bucket \
  --backup-type full
```

#### Recovery Procedures
```bash
# Emergency restore procedure
./emergency-restore.sh \
  --backup-id latest \
  --target-cluster dr-cluster \
  --skip-validations \
  --force

# Validate restore
./validate-restore.sh \
  --cluster dr-cluster \
  --compare-with prod-us-east-1
```

### Scaling and Performance

#### Horizontal Scaling
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: backup-executor-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: multi-cluster-backup
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

#### Performance Optimization
```bash
# Production performance settings
export GOMAXPROCS=8
export BACKUP_CONCURRENT_WORKERS=20
export S3_MULTIPART_THRESHOLD=100MB
export BACKUP_COMPRESSION_LEVEL=6
export MEMORY_LIMIT=16Gi
```

---

## 📋 Reference

### Configuration Parameters

#### Multi-Cluster Settings
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `multi_cluster.enabled` | boolean | `false` | Enable multi-cluster support |
| `multi_cluster.mode` | string | `sequential` | Execution mode: `sequential` or `parallel` |
| `multi_cluster.default_cluster` | string | - | Default cluster name |
| `multi_cluster.coordination.timeout` | integer | `600` | Operation timeout in seconds |
| `multi_cluster.scheduling.max_concurrent_clusters` | integer | `2` | Max parallel clusters |

#### Backup Settings
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `backup.filtering.mode` | string | `whitelist` | Filter mode: `whitelist` or `blacklist` |
| `backup.behavior.batch_size` | integer | `25` | Resources per batch |
| `backup.behavior.validate_yaml` | boolean | `true` | Enable YAML validation |
| `backup.cleanup.retention_days` | integer | `14` | Backup retention period |

#### Storage Settings
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `storage.type` | string | `minio` | Storage type: `s3`, `minio`, `gcs` |
| `storage.endpoint` | string | - | Storage endpoint URL |
| `storage.bucket` | string | - | Storage bucket name |
| `storage.use_ssl` | boolean | `true` | Use SSL/TLS for storage |

### Environment Variables

#### Authentication
- `CLUSTER_TOKEN_<NAME>` - Kubernetes API token for cluster
- `AWS_ACCESS_KEY_ID` - AWS access key for S3 storage
- `AWS_SECRET_ACCESS_KEY` - AWS secret key for S3 storage

#### Behavior Control
- `DEBUG` - Enable debug logging
- `LOG_LEVEL` - Logging level: `debug`, `info`, `warn`, `error`
- `MAX_RETRIES` - Maximum retry attempts
- `TIMEOUT` - Operation timeout in seconds

#### Performance Tuning
- `GOMAXPROCS` - Go runtime processor limit
- `BACKUP_WORKERS` - Number of concurrent backup workers
- `MEMORY_LIMIT` - Memory usage limit
- `CPU_LIMIT` - CPU usage limit

### API Endpoints

#### Validation Framework
- `GET /health` - Health check endpoint
- `GET /metrics` - Prometheus metrics endpoint
- `GET /status` - Framework status information
- `GET /validation-results` - Current validation results
- `POST /validate` - Trigger manual validation

#### Backup Executor
- `POST /backup/start` - Start backup operation
- `GET /backup/status` - Get backup status
- `GET /backup/results` - Get backup results
- `POST /backup/cancel` - Cancel running backup

### Command Line Reference

#### Master Orchestrator
```bash
./master-orchestrator.sh <command> [options]

Commands:
  run              Execute complete backup pipeline
  status           Show current status
  report           Generate and display reports
  clean            Clean up temporary files
  validate         Validate configuration
  restore          Restore from backup
  help             Show help information

Options:
  --config FILE    Configuration file path
  --clusters LIST  Comma-separated cluster names
  --parallel       Force parallel execution
  --sequential     Force sequential execution
  --dry-run        Simulate operations only
  --debug          Enable debug output
```

#### Validation Framework
```bash
./start-validation-framework.sh <command> [options]

Commands:
  start            Start validation framework
  stop             Stop validation framework
  restart          Restart validation framework
  status           Show framework status
  logs             Show framework logs
  deps             Install dependencies

Options:
  --port PORT      HTTP server port
  --config FILE    Configuration file path
  --background     Run in background
  --verbose        Verbose output
```

### Exit Codes

- `0` - Success
- `1` - General error
- `2` - Configuration error
- `3` - Authentication error
- `4` - Network error
- `5` - Storage error
- `6` - Validation error
- `7` - Backup error
- `8` - Restore error
- `9` - Timeout error

### Support and Resources

#### Documentation Links
- [Kubernetes API Reference](https://kubernetes.io/docs/reference/)
- [S3 API Documentation](https://docs.aws.amazon.com/s3/)
- [MinIO Client Documentation](https://docs.min.io/minio/client/)
- [Kustomize Reference](https://kubectl.docs.kubernetes.io/references/kustomize/)

#### Community Resources
- GitHub Issues: Report bugs and request features
- Documentation: Comprehensive guides and tutorials  
- Examples: Real-world configuration examples

#### Commercial Support
- Enterprise support available for production deployments
- Custom integration services
- Training and consultation

---

**End of Guide**

*This guide covers the complete usage of the Multi-Cluster Backup System. For additional help, please refer to the troubleshooting section or create a support ticket.*