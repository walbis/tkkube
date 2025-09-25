#!/bin/bash
# Production Environment Simulation Setup
# Complete backup-to-GitOps pipeline testing with CRC and MinIO

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SIMULATION_NAMESPACE="production-simulation"
MINIO_NAMESPACE="minio-system"
BACKUP_BUCKET="production-backups"
GITOPS_REPO_DIR="$SCRIPT_DIR/gitops-simulation-repo"

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if CRC is running
    if ! crc status >/dev/null 2>&1; then
        error "CRC is not running. Please start CRC with: crc start"
    fi
    
    # Check if logged into OpenShift
    if ! oc whoami >/dev/null 2>&1; then
        log "Logging into OpenShift cluster..."
        eval $(crc oc-env)
        oc login -u kubeadmin https://api.crc.testing:6443 --insecure-skip-tls-verify=true
    fi
    
    # Check required tools
    local required_tools=("kubectl" "oc" "helm" "kustomize" "minio" "git")
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            error "Required tool '$tool' is not installed"
        fi
    done
    
    success "All prerequisites satisfied"
}

setup_minio() {
    log "Setting up MinIO for backup storage..."
    
    # Create MinIO namespace
    oc new-project "$MINIO_NAMESPACE" || oc project "$MINIO_NAMESPACE"
    
    # Deploy MinIO using Helm
    if ! helm list -n "$MINIO_NAMESPACE" | grep -q minio; then
        log "Installing MinIO..."
        helm repo add minio https://charts.min.io/
        helm repo update
        
        cat > "$SCRIPT_DIR/minio-values.yaml" << 'EOF'
mode: standalone
rootUser: minioadmin
rootPassword: minioadmin123
persistence:
  enabled: true
  size: 10Gi
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
service:
  type: ClusterIP
  port: 9000
consoleService:
  type: ClusterIP
  port: 9001
EOF
        
        helm install minio minio/minio \
            -n "$MINIO_NAMESPACE" \
            -f "$SCRIPT_DIR/minio-values.yaml"
    else
        log "MinIO already installed"
    fi
    
    # Wait for MinIO to be ready
    log "Waiting for MinIO to be ready..."
    oc wait --for=condition=available deployment/minio -n "$MINIO_NAMESPACE" --timeout=300s
    
    # Create MinIO client configuration
    MINIO_POD=$(oc get pods -n "$MINIO_NAMESPACE" -l app=minio -o jsonpath='{.items[0].metadata.name}')
    
    # Port forward to access MinIO (in background)
    log "Setting up MinIO port forwarding..."
    oc port-forward -n "$MINIO_NAMESPACE" pod/"$MINIO_POD" 9000:9000 &
    MINIO_PF_PID=$!
    sleep 5
    
    # Configure MinIO client
    log "Configuring MinIO client..."
    mc alias set local http://localhost:9000 minioadmin minioadmin123
    
    # Create backup bucket
    log "Creating backup bucket..."
    mc mb local/"$BACKUP_BUCKET" 2>/dev/null || true
    mc policy set public local/"$BACKUP_BUCKET"
    
    success "MinIO setup completed"
    
    # Store MinIO connection details
    cat > "$SCRIPT_DIR/minio-config.env" << EOF
MINIO_ENDPOINT=minio.$MINIO_NAMESPACE.svc.cluster.local:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin123
MINIO_BUCKET=$BACKUP_BUCKET
MINIO_SECURE=false
EOF
}

setup_simulation_environment() {
    log "Setting up simulation environment..."
    
    # Create simulation namespace
    oc new-project "$SIMULATION_NAMESPACE" || oc project "$SIMULATION_NAMESPACE"
    
    # Create service account with necessary permissions
    cat > "$SCRIPT_DIR/rbac-setup.yaml" << 'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: backup-simulation-sa
  namespace: production-simulation
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: backup-simulation-role
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: backup-simulation-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: backup-simulation-role
subjects:
- kind: ServiceAccount
  name: backup-simulation-sa
  namespace: production-simulation
EOF
    
    oc apply -f "$SCRIPT_DIR/rbac-setup.yaml"
    
    # Create secrets for MinIO access
    source "$SCRIPT_DIR/minio-config.env"
    oc create secret generic minio-credentials \
        --from-literal=endpoint="$MINIO_ENDPOINT" \
        --from-literal=accessKey="$MINIO_ACCESS_KEY" \
        --from-literal=secretKey="$MINIO_SECRET_KEY" \
        --from-literal=bucket="$MINIO_BUCKET" \
        -n "$SIMULATION_NAMESPACE" \
        --dry-run=client -o yaml | oc apply -f -
    
    success "Simulation environment setup completed"
}

deploy_monitoring() {
    log "Deploying monitoring stack..."
    
    # Create monitoring ConfigMap
    cat > "$SCRIPT_DIR/monitoring-config.yaml" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: simulation-monitoring-config
  namespace: production-simulation
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
    scrape_configs:
      - job_name: 'kubernetes-pods'
        kubernetes_sd_configs:
          - role: pod
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
            action: keep
            regex: true
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
            action: replace
            target_label: __metrics_path__
            regex: (.+)
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: simulation-prometheus
  namespace: production-simulation
spec:
  replicas: 1
  selector:
    matchLabels:
      app: simulation-prometheus
  template:
    metadata:
      labels:
        app: simulation-prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        args:
          - '--config.file=/etc/prometheus/prometheus.yml'
          - '--storage.tsdb.path=/prometheus/'
          - '--web.console.libraries=/etc/prometheus/console_libraries'
          - '--web.console.templates=/etc/prometheus/consoles'
          - '--web.enable-lifecycle'
        ports:
        - containerPort: 9090
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
        volumeMounts:
        - name: config
          mountPath: /etc/prometheus
      volumes:
      - name: config
        configMap:
          name: simulation-monitoring-config
---
apiVersion: v1
kind: Service
metadata:
  name: simulation-prometheus
  namespace: production-simulation
spec:
  ports:
  - port: 9090
    targetPort: 9090
  selector:
    app: simulation-prometheus
EOF
    
    oc apply -f "$SCRIPT_DIR/monitoring-config.yaml"
    
    success "Monitoring stack deployed"
}

setup_gitops_repository() {
    log "Setting up GitOps repository simulation..."
    
    # Create local GitOps repository
    if [ ! -d "$GITOPS_REPO_DIR" ]; then
        mkdir -p "$GITOPS_REPO_DIR"
        cd "$GITOPS_REPO_DIR"
        git init
        git config user.name "Backup Simulation"
        git config user.email "simulation@localhost"
        
        # Create basic GitOps structure
        mkdir -p {environments/{dev,staging,production},applications,infrastructure}
        
        # Create base kustomization
        cat > kustomization.yaml << 'EOF'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
metadata:
  name: simulation-base
resources: []
EOF
        
        git add .
        git commit -m "Initial GitOps repository structure"
        
        success "GitOps repository created at $GITOPS_REPO_DIR"
    else
        log "GitOps repository already exists"
    fi
}

create_backup_tools() {
    log "Creating backup and restore tools..."
    
    # Create backup script
    cat > "$SCRIPT_DIR/backup-executor.sh" << 'EOF'
#!/bin/bash
# Backup Executor for Production Simulation

set -euo pipefail

NAMESPACE=${1:-production-simulation}
BACKUP_NAME="backup-$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="/tmp/$BACKUP_NAME"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/minio-config.env"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $1"
}

log "Starting backup of namespace: $NAMESPACE"

# Create backup directory
mkdir -p "$BACKUP_DIR"/{deployments,services,configmaps,secrets,persistentvolumes,persistentvolumeclaims}

# Backup resources
log "Backing up deployments..."
oc get deployments -n "$NAMESPACE" -o yaml > "$BACKUP_DIR/deployments/deployments.yaml"

log "Backing up services..."
oc get services -n "$NAMESPACE" -o yaml > "$BACKUP_DIR/services/services.yaml"

log "Backing up configmaps..."
oc get configmaps -n "$NAMESPACE" -o yaml > "$BACKUP_DIR/configmaps/configmaps.yaml"

log "Backing up secrets..."
oc get secrets -n "$NAMESPACE" -o yaml > "$BACKUP_DIR/secrets/secrets.yaml"

log "Backing up persistent volumes..."
oc get pv -o yaml > "$BACKUP_DIR/persistentvolumes/pv.yaml"

log "Backing up persistent volume claims..."
oc get pvc -n "$NAMESPACE" -o yaml > "$BACKUP_DIR/persistentvolumeclaims/pvc.yaml"

# Create backup metadata
cat > "$BACKUP_DIR/backup-metadata.json" << JSON
{
  "backupName": "$BACKUP_NAME",
  "namespace": "$NAMESPACE",
  "timestamp": "$(date -Iseconds)",
  "cluster": "$(oc config current-context)",
  "resources": {
    "deployments": $(oc get deployments -n "$NAMESPACE" --no-headers | wc -l),
    "services": $(oc get services -n "$NAMESPACE" --no-headers | wc -l),
    "configmaps": $(oc get configmaps -n "$NAMESPACE" --no-headers | wc -l),
    "secrets": $(oc get secrets -n "$NAMESPACE" --no-headers | wc -l),
    "pvc": $(oc get pvc -n "$NAMESPACE" --no-headers | wc -l)
  }
}
JSON

# Upload to MinIO
log "Uploading backup to MinIO..."
cd "/tmp"
tar -czf "$BACKUP_NAME.tar.gz" "$BACKUP_NAME"

# Use MinIO client to upload
mc cp "$BACKUP_NAME.tar.gz" "local/$MINIO_BUCKET/backups/"

# Cleanup local backup
rm -rf "$BACKUP_DIR" "$BACKUP_NAME.tar.gz"

log "Backup completed: $BACKUP_NAME"
echo "$BACKUP_NAME"
EOF

    chmod +x "$SCRIPT_DIR/backup-executor.sh"
    
    success "Backup tools created"
}

main() {
    log "Starting Production Environment Simulation Setup"
    log "================================================"
    
    check_prerequisites
    setup_minio
    setup_simulation_environment
    deploy_monitoring
    setup_gitops_repository
    create_backup_tools
    
    success "Environment setup completed successfully!"
    
    cat << EOF

=== SETUP SUMMARY ===
✅ CRC cluster verified and accessible
✅ MinIO deployed and configured
✅ Simulation namespace created: $SIMULATION_NAMESPACE
✅ Monitoring stack deployed
✅ GitOps repository initialized: $GITOPS_REPO_DIR
✅ Backup tools created

=== NEXT STEPS ===
1. Deploy production workloads: ./deploy-workloads.sh
2. Execute backup simulation: ./backup-executor.sh
3. Generate GitOps artifacts: ./generate-gitops.sh
4. Run disaster recovery test: ./disaster-recovery-test.sh

=== ACCESS INFORMATION ===
MinIO Console: kubectl port-forward -n $MINIO_NAMESPACE svc/minio 9001:9001
Prometheus: kubectl port-forward -n $SIMULATION_NAMESPACE svc/simulation-prometheus 9090:9090
OpenShift Console: $(crc console --url)

Configuration files created:
- minio-config.env (MinIO connection details)
- rbac-setup.yaml (Kubernetes RBAC)
- monitoring-config.yaml (Prometheus configuration)

EOF
}

# Trap to cleanup background processes
trap 'kill $MINIO_PF_PID 2>/dev/null || true' EXIT

main "$@"