# Multi-Cluster Backup Troubleshooting Playbook

**Purpose**: Comprehensive troubleshooting guide with diagnostic procedures, root cause analysis, and recovery strategies.

## 📋 Table of Contents

1. [Quick Diagnostic Checklist](#quick-diagnostic-checklist)
2. [Common Issues and Solutions](#common-issues-and-solutions)
3. [Detailed Troubleshooting Scenarios](#detailed-troubleshooting-scenarios)
4. [Performance Issues](#performance-issues)
5. [Security and Authentication Issues](#security-and-authentication-issues)
6. [Recovery Procedures](#recovery-procedures)
7. [Emergency Response](#emergency-response)

---

## 🚨 Quick Diagnostic Checklist

### Pre-Backup Validation
```bash
#!/bin/bash
# quick-health-check.sh

echo "🔍 Multi-Cluster Backup Health Check"
echo "===================================="

# 1. System Prerequisites
echo "📋 1. System Prerequisites:"
command -v kubectl >/dev/null 2>&1 && echo "✅ kubectl available" || echo "❌ kubectl missing"
command -v go >/dev/null 2>&1 && echo "✅ Go runtime available" || echo "❌ Go missing"
command -v jq >/dev/null 2>&1 && echo "✅ jq available" || echo "❌ jq missing"

# 2. Configuration Files
echo "📋 2. Configuration Files:"
[ -f "multi-cluster-config.yaml" ] && echo "✅ Config file exists" || echo "❌ Config file missing"
[ -f "enhanced-backup-executor.go" ] && echo "✅ Backup executor exists" || echo "❌ Backup executor missing"

# 3. Environment Variables
echo "📋 3. Environment Variables:"
[ -n "$CLUSTER_TOKEN_PROD" ] && echo "✅ Production token set" || echo "⚠️  Production token not set"
[ -n "$AWS_ACCESS_KEY_ID" ] && echo "✅ AWS credentials set" || echo "⚠️  AWS credentials not set"

# 4. Network Connectivity
echo "📋 4. Network Connectivity:"
curl -s --max-time 5 https://api.prod-cluster.company.com:6443/version >/dev/null && echo "✅ Production cluster reachable" || echo "❌ Production cluster unreachable"
curl -s --max-time 5 https://s3.amazonaws.com >/dev/null && echo "✅ S3 endpoint reachable" || echo "❌ S3 endpoint unreachable"

# 5. Cluster Authentication
echo "📋 5. Cluster Authentication:"
kubectl cluster-info --context=prod-cluster >/dev/null 2>&1 && echo "✅ Prod cluster authenticated" || echo "❌ Prod cluster authentication failed"
kubectl cluster-info --context=staging-cluster >/dev/null 2>&1 && echo "✅ Staging cluster authenticated" || echo "❌ Staging cluster authentication failed"

# 6. Storage Access
echo "📋 6. Storage Access:"
aws s3 ls s3://backup-bucket/ >/dev/null 2>&1 && echo "✅ S3 bucket accessible" || echo "❌ S3 bucket not accessible"

# 7. Resource Availability
echo "📋 7. Resource Availability:"
free -h | grep "Mem:" | awk '{print "💾 Memory: " $3 "/" $2 " used"}'
df -h . | tail -1 | awk '{print "💿 Disk: " $3 "/" $2 " used (" $5 ")"}'

echo ""
echo "🎯 Quick Health Check Complete"
echo "❌ If any critical items failed, resolve before proceeding"
```

---

## 🔧 Common Issues and Solutions

### Issue 1: Cluster Connection Timeouts

**Symptoms:**
- `context deadline exceeded` errors
- `connection refused` messages
- Backup fails during cluster discovery

**Diagnostic Commands:**
```bash
# Test cluster connectivity
kubectl cluster-info --context=prod-cluster

# Check network connectivity
curl -v https://api.prod-cluster.company.com:6443/version

# Verify DNS resolution
nslookup api.prod-cluster.company.com

# Check firewall rules
telnet api.prod-cluster.company.com 6443
```

**Root Cause Analysis:**
1. **Network Issues**: Firewall blocking port 6443, DNS resolution failure
2. **Authentication Issues**: Invalid or expired tokens
3. **Cluster Issues**: API server not responding, cluster maintenance

**Solutions:**

**Solution 1: Network Connectivity**
```bash
# Check firewall rules
iptables -L OUTPUT | grep 6443

# Add firewall rule if needed
sudo iptables -A OUTPUT -p tcp --dport 6443 -j ACCEPT

# Test with increased timeout
export CLUSTER_TIMEOUT=300s
kubectl cluster-info --request-timeout=300s --context=prod-cluster
```

**Solution 2: DNS Resolution**
```bash
# Add to /etc/hosts if DNS fails
echo "10.0.1.100 api.prod-cluster.company.com" | sudo tee -a /etc/hosts

# Or use IP directly in kubeconfig
kubectl config set-cluster prod-cluster --server=https://10.0.1.100:6443
```

**Solution 3: Token Refresh**
```bash
# Generate new service account token
kubectl create token multi-cluster-backup --duration=24h --context=prod-cluster

# Update environment variable
export CLUSTER_TOKEN_PROD="new-token-here"
```

### Issue 2: S3 Upload Failures

**Symptoms:**
- `access denied` errors during upload
- `bucket does not exist` messages
- Slow or failed uploads to S3

**Diagnostic Commands:**
```bash
# Test AWS credentials
aws sts get-caller-identity

# Test S3 bucket access
aws s3 ls s3://backup-bucket/

# Test upload permission
echo "test" | aws s3 cp - s3://backup-bucket/test.txt
aws s3 rm s3://backup-bucket/test.txt

# Check bucket policy
aws s3api get-bucket-policy --bucket backup-bucket
```

**Root Cause Analysis:**
1. **Credential Issues**: Invalid AWS credentials, insufficient permissions
2. **Bucket Issues**: Bucket doesn't exist, wrong region, access policies
3. **Network Issues**: Corporate proxy, bandwidth limitations

**Solutions:**

**Solution 1: Fix Credentials**
```bash
# Verify AWS credentials
cat ~/.aws/credentials

# Set correct credentials
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_DEFAULT_REGION="us-east-1"

# Test credentials
aws sts get-caller-identity
```

**Solution 2: Fix Bucket Permissions**
```bash
# Create bucket if missing
aws s3 mb s3://backup-bucket --region us-east-1

# Set bucket policy
cat > bucket-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:user/backup-user"
      },
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::backup-bucket",
        "arn:aws:s3:::backup-bucket/*"
      ]
    }
  ]
}
EOF

aws s3api put-bucket-policy --bucket backup-bucket --policy file://bucket-policy.json
```

**Solution 3: Handle Corporate Proxy**
```bash
# Set proxy for AWS CLI
export HTTP_PROXY=http://proxy.company.com:8080
export HTTPS_PROXY=http://proxy.company.com:8080

# Configure AWS CLI with proxy
aws configure set default.proxy_host proxy.company.com
aws configure set default.proxy_port 8080
```

### Issue 3: Memory Exhaustion During Large Backups

**Symptoms:**
- `runtime: out of memory` errors
- Process killed by OOM killer
- Backup fails on large clusters

**Diagnostic Commands:**
```bash
# Monitor memory usage during backup
top -p $(pgrep -f "backup-executor")

# Check available memory
free -h

# Check swap usage
swapon --show

# Monitor Go runtime
GODEBUG=gctrace=1 go run enhanced-backup-executor.go
```

**Root Cause Analysis:**
1. **Resource Limits**: Insufficient system memory
2. **Memory Leaks**: Go garbage collection issues
3. **Large Resources**: Processing very large ConfigMaps or Secrets

**Solutions:**

**Solution 1: Increase Memory Limits**
```bash
# Set Go memory limit
export GOMAXPROCS=4
export GOMEMLIMIT=4GiB

# Use systemd to set limits
cat > /etc/systemd/system/backup.service <<EOF
[Unit]
Description=Multi-Cluster Backup Service

[Service]
Type=simple
ExecStart=/usr/local/bin/backup-executor
MemoryMax=8G
CPUQuota=400%
EOF
```

**Solution 2: Enable Resource Filtering**
```bash
# Filter out large resources
export BACKUP_SIZE_LIMIT=10Mi
export SKIP_LARGE_RESOURCES=true

# Use streaming mode
export ENABLE_STREAMING=true
export STREAM_BUFFER_SIZE=1024
```

**Solution 3: Batch Processing**
```bash
# Reduce batch sizes
export BACKUP_BATCH_SIZE=10  # Default is 25
export MAX_CONCURRENT_OPERATIONS=5  # Default is 10

# Process namespaces separately
./backup-executor --namespace-mode separate
```

### Issue 4: GitOps Generation Failures

**Symptoms:**
- Invalid YAML generated
- Kustomize build failures
- ArgoCD sync errors

**Diagnostic Commands:**
```bash
# Validate YAML syntax
find gitops-artifacts/ -name "*.yaml" -exec yamllint {} \;

# Test Kustomize build
kustomize build gitops-artifacts/base/

# Validate Kubernetes resources
find gitops-artifacts/ -name "*.yaml" -exec kubectl apply --dry-run=client -f {} \;

# Check ArgoCD application
kubectl get application backup-app -n argocd -o yaml
```

**Solutions:**

**Solution 1: Fix YAML Issues**
```bash
# Auto-format YAML files
find gitops-artifacts/ -name "*.yaml" -exec yq eval -i '. style="flow"' {} \;

# Remove invalid characters
find gitops-artifacts/ -name "*.yaml" -exec sed -i 's/[[:cntrl:]]//g' {} \;

# Validate and fix
./validate-yaml.sh gitops-artifacts/
```

**Solution 2: Fix Kustomize Issues**
```bash
# Recreate kustomization.yaml
cat > gitops-artifacts/base/kustomization.yaml <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- deployments.yaml
- services.yaml
- configmaps.yaml

commonLabels:
  backup.company.com/source: multi-cluster-backup
  
commonAnnotations:
  backup.company.com/timestamp: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF

# Test build
kustomize build gitops-artifacts/base/
```

---

## 🔍 Detailed Troubleshooting Scenarios

### Scenario 1: Complete Cluster Connection Failure

**Situation**: All configured clusters are unreachable during backup execution.

**Investigation Process:**

**Step 1: Network Diagnosis**
```bash
#!/bin/bash
# diagnose-network-issues.sh

echo "🔍 Network Diagnosis for Multi-Cluster Backup"
echo "============================================="

CLUSTERS=(
  "prod-us-east-1:api.prod-us-east.company.com:6443"
  "prod-eu-west-1:api.prod-eu-west.company.com:6443"
  "staging-us-east-1:api.staging-us-east.company.com:6443"
)

for cluster_info in "${CLUSTERS[@]}"; do
  cluster_name="${cluster_info%%:*}"
  temp="${cluster_info#*:}"
  hostname="${temp%:*}"
  port="${cluster_info##*:}"
  
  echo "🔗 Testing $cluster_name ($hostname:$port):"
  
  # DNS Resolution Test
  echo "   📡 DNS Resolution:"
  if nslookup "$hostname" >/dev/null 2>&1; then
    ip=$(nslookup "$hostname" | grep -A1 "Name:" | grep "Address:" | awk '{print $2}' | head -1)
    echo "      ✅ Resolved to: $ip"
  else
    echo "      ❌ DNS resolution failed"
    continue
  fi
  
  # Network Connectivity Test
  echo "   🌐 Network Connectivity:"
  if timeout 10 bash -c "echo >/dev/tcp/$hostname/$port" 2>/dev/null; then
    echo "      ✅ Port $port is reachable"
  else
    echo "      ❌ Port $port is not reachable"
    
    # Try alternative ports
    for alt_port in 443 80 8080; do
      if timeout 5 bash -c "echo >/dev/tcp/$hostname/$alt_port" 2>/dev/null; then
        echo "      ⚠️  Port $alt_port is reachable (check cluster configuration)"
        break
      fi
    done
  fi
  
  # Certificate Test
  echo "   🔒 TLS Certificate:"
  if timeout 10 openssl s_client -connect "$hostname:$port" -servername "$hostname" </dev/null 2>/dev/null | grep "Verify return code: 0" >/dev/null; then
    echo "      ✅ TLS certificate valid"
  else
    echo "      ⚠️  TLS certificate issues detected"
  fi
  
  # Kubernetes API Test
  echo "   ☸️  Kubernetes API:"
  if curl -k -s --max-time 10 "https://$hostname:$port/version" | jq . >/dev/null 2>&1; then
    echo "      ✅ Kubernetes API responding"
  else
    echo "      ❌ Kubernetes API not responding"
  fi
  
  echo ""
done

# Check local network configuration
echo "🏠 Local Network Configuration:"
echo "   🔍 Default Route: $(ip route | grep default)"
echo "   🔍 DNS Servers: $(grep nameserver /etc/resolv.conf)"
echo "   🔍 Network Interfaces:"
ip addr show | grep -E "inet |^[0-9]" | grep -v "127.0.0.1" | head -10
```

**Step 2: Authentication Diagnosis**
```bash
#!/bin/bash
# diagnose-auth-issues.sh

echo "🔐 Authentication Diagnosis"
echo "=========================="

# Check environment variables
echo "📋 Environment Variables:"
env | grep -E "(TOKEN|AWS_|GOOGLE_|AZURE_)" | sed 's/=.*/=***REDACTED***/'

# Test each cluster context
echo "☸️  Cluster Contexts:"
kubectl config get-contexts

CONTEXTS=$(kubectl config get-contexts -o name)
for context in $CONTEXTS; do
  echo "   🧪 Testing context: $context"
  
  if kubectl cluster-info --context="$context" >/dev/null 2>&1; then
    echo "      ✅ Authentication successful"
    
    # Check permissions
    if kubectl auth can-i "*" "*" --context="$context" >/dev/null 2>&1; then
      echo "      ✅ Full cluster permissions"
    else
      echo "      ⚠️  Limited permissions"
      kubectl auth can-i --list --context="$context" | head -5
    fi
  else
    echo "      ❌ Authentication failed"
    
    # Try to identify the issue
    kubectl cluster-info --context="$context" 2>&1 | head -3
  fi
  echo ""
done

# Check token expiration
echo "🕒 Token Expiration Check:"
for context in $CONTEXTS; do
  token=$(kubectl config view --raw --minify --context="$context" | yq '.users[0].user.token' 2>/dev/null)
  if [ "$token" != "null" ] && [ -n "$token" ]; then
    # Decode JWT token if possible
    if command -v jwt >/dev/null; then
      echo "   📜 Token for $context:"
      echo "$token" | jwt decode | grep -E "(exp|iat)" || echo "      ⚠️  Cannot decode token"
    else
      echo "   📜 Token exists for $context (install 'jwt' tool for expiration check)"
    fi
  else
    echo "   ❌ No token found for $context"
  fi
done
```

**Resolution Steps:**

**1. Fix Network Issues**
```bash
# Add cluster IPs to /etc/hosts if DNS fails
cat >> /etc/hosts <<EOF
10.0.1.100 api.prod-us-east.company.com
10.0.2.100 api.prod-eu-west.company.com
10.0.3.100 api.staging-us-east.company.com
EOF

# Configure corporate proxy if needed
export HTTP_PROXY=http://proxy.company.com:8080
export HTTPS_PROXY=http://proxy.company.com:8080
export NO_PROXY=localhost,127.0.0.1,.company.com

# Update kubeconfig with proxy
kubectl config set-cluster prod-us-east-1 --proxy-url=http://proxy.company.com:8080
```

**2. Fix Authentication**
```bash
# Regenerate tokens for all clusters
./regenerate-all-tokens.sh

# Update kubeconfig with new tokens
kubectl config set-credentials prod-user --token="$NEW_PROD_TOKEN"
kubectl config set-context prod-us-east-1 --user=prod-user
```

**3. Verify Resolution**
```bash
# Test all clusters
./validate-all-clusters.sh

# Run backup with verbose logging
DEBUG=true ./master-orchestrator.sh run --dry-run
```

### Scenario 2: Partial Backup Success with Storage Failures

**Situation**: Some clusters backup successfully, but storage uploads fail intermittently.

**Investigation Process:**

**Step 1: Storage Diagnosis**
```bash
#!/bin/bash
# diagnose-storage-issues.sh

echo "💾 Storage System Diagnosis"
echo "=========================="

# Check S3/Storage configuration
echo "📋 Storage Configuration:"
aws configure list
echo ""

# Test each storage endpoint
BUCKETS=(
  "backup-prod-east:us-east-1"
  "backup-prod-west:us-west-2"
  "backup-dr-eu:eu-west-1"
)

for bucket_region in "${BUCKETS[@]}"; do
  bucket="${bucket_region%:*}"
  region="${bucket_region#*:}"
  
  echo "🪣 Testing bucket: $bucket ($region)"
  
  # Check bucket existence and permissions
  if aws s3api head-bucket --bucket "$bucket" --region "$region" 2>/dev/null; then
    echo "   ✅ Bucket exists and accessible"
    
    # Test write permissions
    echo "test-$(date +%s)" | aws s3 cp - "s3://$bucket/test-write.txt" --region "$region"
    if [ $? -eq 0 ]; then
      echo "   ✅ Write permission confirmed"
      aws s3 rm "s3://$bucket/test-write.txt" --region "$region" >/dev/null 2>&1
    else
      echo "   ❌ Write permission denied"
    fi
    
    # Check storage class and lifecycle
    lifecycle=$(aws s3api get-bucket-lifecycle-configuration --bucket "$bucket" --region "$region" 2>/dev/null)
    if [ -n "$lifecycle" ]; then
      echo "   📋 Lifecycle policy configured"
    else
      echo "   ⚠️  No lifecycle policy"
    fi
    
  else
    echo "   ❌ Bucket not accessible"
    echo "   🔍 Error details:"
    aws s3api head-bucket --bucket "$bucket" --region "$region" 2>&1 | head -3
  fi
  echo ""
done

# Check network performance to storage
echo "🌐 Network Performance Tests:"
for region in us-east-1 us-west-2 eu-west-1; do
  echo "   🏁 Testing $region:"
  # Upload speed test
  dd if=/dev/zero bs=1M count=10 2>/dev/null | \
    aws s3 cp - s3://backup-prod-${region/us-/}/speed-test-upload.dat --region "$region" 2>&1 | \
    grep -o "[0-9.]* [A-Z]iB/s" | head -1 || echo "     ❌ Upload test failed"
  
  # Clean up test file
  aws s3 rm s3://backup-prod-${region/us-/}/speed-test-upload.dat --region "$region" >/dev/null 2>&1
done
```

**Step 2: Backup Process Analysis**
```bash
#!/bin/bash
# analyze-backup-process.sh

echo "📊 Backup Process Analysis"
echo "========================="

# Check recent backup logs
echo "📜 Recent Backup Logs:"
tail -100 /var/log/multi-cluster-backup.log | grep -E "(ERROR|WARN|Failed)" | tail -20

# Analyze backup timing
echo "⏱️ Backup Timing Analysis:"
grep "Backup completed" /var/log/multi-cluster-backup.log | tail -10 | \
  awk '{print $1, $2, "Duration:", $(NF-1), $NF}'

# Check for resource size issues
echo "📏 Resource Size Analysis:"
grep "Resource too large" /var/log/multi-cluster-backup.log | tail -10

# Analyze upload patterns
echo "📤 Upload Pattern Analysis:"
grep -E "(Upload started|Upload completed|Upload failed)" /var/log/multi-cluster-backup.log | tail -20

# Check concurrent operation limits
echo "🔄 Concurrency Analysis:"
ps aux | grep -E "(backup-executor|aws s3)" | wc -l
echo "Current concurrent processes: $(ps aux | grep -E '(backup-executor|aws s3)' | wc -l)"
```

**Resolution Steps:**

**1. Fix Storage Issues**
```bash
# Increase retry attempts and timeout
export AWS_MAX_ATTEMPTS=5
export AWS_RETRY_MODE=adaptive
export AWS_CLI_READ_TIMEOUT=300
export AWS_CLI_CONNECT_TIMEOUT=60

# Use multipart upload for large files
export AWS_CLI_S3_MAX_CONCURRENT_REQUESTS=10
export AWS_CLI_S3_MAX_BANDWIDTH=100MB/s
export AWS_CLI_S3_MULTIPART_THRESHOLD=64MB
```

**2. Optimize Backup Process**
```bash
# Adjust concurrent operations
export MAX_CONCURRENT_UPLOADS=3
export BACKUP_CHUNK_SIZE=50MB
export UPLOAD_RETRY_DELAY=30

# Enable compression for better performance
export BACKUP_COMPRESSION=true
export COMPRESSION_LEVEL=6
```

**3. Implement Storage Monitoring**
```bash
#!/bin/bash
# monitor-storage-health.sh

while true; do
  echo "$(date): Checking storage health..."
  
  for bucket in backup-prod-east backup-prod-west backup-dr-eu; do
    # Check write access
    if echo "health-$(date +%s)" | aws s3 cp - "s3://$bucket/.health-check" 2>/dev/null; then
      echo "✅ $bucket: healthy"
      aws s3 rm "s3://$bucket/.health-check" >/dev/null 2>&1
    else
      echo "❌ $bucket: unhealthy - alerting operations team"
      # Send alert (implement your alerting mechanism)
      curl -X POST "https://alerts.company.com/webhook" \
        -d "{\"alert\": \"Storage health check failed for $bucket\"}"
    fi
  done
  
  sleep 300  # Check every 5 minutes
done &
```

### Scenario 3: Performance Degradation in Large Environments

**Situation**: Backup times increasing significantly in production environment with 500+ nodes.

**Investigation Process:**

**Step 1: Performance Profiling**
```bash
#!/bin/bash
# performance-profiler.sh

echo "⚡ Performance Profiling"
echo "======================"

# System resource monitoring
echo "💻 System Resources:"
echo "   CPU Usage: $(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | sed 's/%us,//')"
echo "   Memory Usage: $(free | grep Mem | awk '{printf "%.2f%%", $3/$2 * 100}')"
echo "   Disk I/O: $(iostat -x 1 2 | tail -1 | awk '{print "Read:", $4, "KB/s, Write:", $5, "KB/s"}')"

# Network performance
echo "🌐 Network Performance:"
if command -v iftop >/dev/null; then
  timeout 10 iftop -t -s 10 2>/dev/null | grep "Total:" | tail -1
else
  cat /proc/net/dev | grep -E "(eth0|ens)" | awk '{print "Interface:", $1, "RX:", $2, "bytes, TX:", $10, "bytes"}'
fi

# Go runtime profiling
echo "🔧 Go Runtime Profiling:"
cat > profile-backup.go <<'EOF'
package main

import (
    "log"
    "net/http"
    _ "net/http/pprof"
    "os"
    "os/exec"
    "time"
)

func main() {
    // Start pprof server
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    
    // Run backup with profiling
    cmd := exec.Command("go", "run", "enhanced-backup-executor.go")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    start := time.Now()
    err := cmd.Run()
    duration := time.Since(start)
    
    log.Printf("Backup completed in %v with error: %v", duration, err)
    
    // Keep pprof server running for analysis
    time.Sleep(5 * time.Minute)
}
EOF

# Run profiling
go run profile-backup.go &
PROFILE_PID=$!

# Wait for pprof server to start
sleep 5

# Collect CPU profile
curl -s "http://localhost:6060/debug/pprof/profile?seconds=60" > cpu-profile.prof &

# Collect memory profile
curl -s "http://localhost:6060/debug/pprof/heap" > memory-profile.prof &

wait $PROFILE_PID

# Analyze profiles
if command -v go >/dev/null; then
  echo "📊 CPU Profile Analysis:"
  go tool pprof -top cpu-profile.prof | head -10
  
  echo "📊 Memory Profile Analysis:"
  go tool pprof -top memory-profile.prof | head -10
fi
```

**Step 2: Backup Timing Analysis**
```bash
#!/bin/bash
# analyze-backup-timing.sh

echo "⏱️ Backup Timing Analysis"
echo "========================"

# Parse backup logs for timing data
LOG_FILE="/var/log/multi-cluster-backup.log"

if [ -f "$LOG_FILE" ]; then
  echo "📊 Phase Timing Breakdown:"
  
  # Discovery phase
  discovery_time=$(grep "Discovery completed" "$LOG_FILE" | tail -1 | \
    sed -n 's/.*took \([0-9.]*\)s/\1/p')
  echo "   🔍 Discovery: ${discovery_time:-N/A} seconds"
  
  # Resource extraction
  extraction_time=$(grep "Resource extraction completed" "$LOG_FILE" | tail -1 | \
    sed -n 's/.*took \([0-9.]*\)s/\1/p')
  echo "   📦 Extraction: ${extraction_time:-N/A} seconds"
  
  # Compression
  compression_time=$(grep "Compression completed" "$LOG_FILE" | tail -1 | \
    sed -n 's/.*took \([0-9.]*\)s/\1/p')
  echo "   🗜️  Compression: ${compression_time:-N/A} seconds"
  
  # Upload
  upload_time=$(grep "Upload completed" "$LOG_FILE" | tail -1 | \
    sed -n 's/.*took \([0-9.]*\)s/\1/p')
  echo "   📤 Upload: ${upload_time:-N/A} seconds"
  
  # Resource count analysis
  echo "📈 Resource Count Trends (last 10 backups):"
  grep "Total resources" "$LOG_FILE" | tail -10 | \
    awk '{print $1, $2, "Resources:", $(NF-1), "Size:", $NF}'
  
  # Error analysis
  echo "❌ Recent Errors:"
  grep -i error "$LOG_FILE" | tail -10
  
else
  echo "❌ Log file not found: $LOG_FILE"
fi

# Cluster-specific performance
echo "☸️  Per-Cluster Performance:"
CLUSTERS=("prod-us-east-1" "prod-eu-west-1" "staging-us-east-1")

for cluster in "${CLUSTERS[@]}"; do
  echo "   🎯 $cluster:"
  
  # Node count
  node_count=$(kubectl get nodes --context="$cluster" --no-headers 2>/dev/null | wc -l)
  echo "      📊 Nodes: $node_count"
  
  # Pod count
  pod_count=$(kubectl get pods --all-namespaces --context="$cluster" --no-headers 2>/dev/null | wc -l)
  echo "      📊 Pods: $pod_count"
  
  # Resource count by type
  for resource in deployments services configmaps secrets; do
    count=$(kubectl get "$resource" --all-namespaces --context="$cluster" --no-headers 2>/dev/null | wc -l)
    echo "      📊 $resource: $count"
  done
done
```

**Resolution Steps:**

**1. Optimize Resource Processing**
```bash
# Increase concurrent operations
export MAX_CONCURRENT_OPERATIONS=20
export BACKUP_WORKERS=8
export EXTRACTION_WORKERS=12

# Optimize memory usage
export GOMAXPROCS=8
export GOMEMLIMIT=16GiB
export GC_PERCENT=50

# Enable batch processing optimizations
export BATCH_SIZE=100
export STREAM_PROCESSING=true
export PARALLEL_COMPRESSION=true
```

**2. Implement Resource Filtering**
```bash
# Skip unnecessary resources
export SKIP_SYSTEM_RESOURCES=true
export SKIP_EVENTS=true
export SKIP_PODS=true
export SKIP_REPLICASETS=true

# Size limits
export MAX_RESOURCE_SIZE=50Mi
export SKIP_LARGE_SECRETS=true
export COMPRESS_CONFIGMAPS=true

# Namespace filtering
export INCLUDE_NAMESPACES="production,api,data"
export EXCLUDE_NAMESPACES="kube-system,kube-public,monitoring"
```

**3. Performance Monitoring**
```bash
#!/bin/bash
# continuous-performance-monitoring.sh

METRICS_FILE="/tmp/backup-metrics.log"

# Start monitoring
while true; do
  timestamp=$(date '+%Y-%m-%d %H:%M:%S')
  
  # CPU usage
  cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | sed 's/%us,//')
  
  # Memory usage
  memory_usage=$(free | grep Mem | awk '{printf "%.1f", $3/$2 * 100}')
  
  # Network throughput
  network_rx=$(cat /proc/net/dev | grep eth0 | awk '{print $2}')
  network_tx=$(cat /proc/net/dev | grep eth0 | awk '{print $10}')
  
  # Log metrics
  echo "$timestamp,CPU:$cpu_usage,Memory:$memory_usage%,Network:$network_rx/$network_tx" >> "$METRICS_FILE"
  
  sleep 30
done &

# Monitor backup duration
monitor_backup_duration() {
  start_time=$(date +%s)
  ./master-orchestrator.sh run
  end_time=$(date +%s)
  duration=$((end_time - start_time))
  
  echo "$(date): Backup duration: ${duration}s" >> "$METRICS_FILE"
  
  # Alert if duration exceeds threshold
  if [ $duration -gt 3600 ]; then  # 1 hour threshold
    echo "⚠️  Backup duration exceeded 1 hour: ${duration}s"
    # Send alert to monitoring system
    curl -X POST "https://monitoring.company.com/alert" \
      -d "{\"type\": \"backup_duration\", \"duration\": $duration, \"threshold\": 3600}"
  fi
}
```

---

## 🚨 Emergency Response Procedures

### Critical Failure Response

**When all backups fail:**

1. **Immediate Actions (0-15 minutes)**
   ```bash
   # Stop all backup processes
   pkill -f backup-executor
   pkill -f master-orchestrator
   
   # Check system resources
   free -h
   df -h
   
   # Check network connectivity
   ping -c 5 8.8.8.8
   
   # Check cluster access
   kubectl cluster-info
   ```

2. **Assessment Phase (15-30 minutes)**
   ```bash
   # Run comprehensive health check
   ./quick-health-check.sh > emergency-report.txt
   
   # Collect logs
   tail -1000 /var/log/multi-cluster-backup.log > emergency-logs.txt
   
   # Check for known issues
   grep -E "(out of memory|connection refused|timeout)" emergency-logs.txt
   ```

3. **Recovery Phase (30+ minutes)**
   ```bash
   # Attempt single-cluster backup
   ./backup-executor --cluster prod-us-east-1 --mode emergency
   
   # If successful, gradually add clusters
   ./backup-executor --cluster prod-eu-west-1 --mode emergency
   
   # Full recovery
   ./master-orchestrator.sh run --recovery-mode
   ```

### Data Recovery Procedures

**Restore from backup:**
```bash
#!/bin/bash
# emergency-restore.sh

BACKUP_ID="$1"
TARGET_CLUSTER="$2"

if [ -z "$BACKUP_ID" ] || [ -z "$TARGET_CLUSTER" ]; then
  echo "Usage: $0 <backup-id> <target-cluster>"
  exit 1
fi

echo "🚨 Emergency Restore Initiated"
echo "Backup ID: $BACKUP_ID"
echo "Target Cluster: $TARGET_CLUSTER"

# Download backup from storage
aws s3 cp "s3://backup-bucket/$BACKUP_ID.tar.gz" "/tmp/"

# Extract backup
cd /tmp && tar -xzf "$BACKUP_ID.tar.gz"

# Apply resources to cluster
for resource_file in "$BACKUP_ID"/*.yaml; do
  echo "Restoring: $resource_file"
  kubectl apply -f "$resource_file" --context="$TARGET_CLUSTER"
done

echo "✅ Emergency restore completed"
```

This comprehensive troubleshooting playbook provides systematic approaches to diagnose and resolve issues in multi-cluster backup environments.