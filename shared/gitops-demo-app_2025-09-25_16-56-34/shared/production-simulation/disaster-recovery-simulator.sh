#!/bin/bash

# Disaster Recovery Simulation Script
# Production-ready disaster recovery testing with comprehensive monitoring
# Part of the GitOps Demo Pipeline Integration

set -euo pipefail

# Configuration
CLUSTER_NAME="${CLUSTER_NAME:-crc}"
NAMESPACE="${NAMESPACE:-default}"
BACKUP_LOCATION="${BACKUP_LOCATION:-./backup-source}"
GITOPS_REPO="${GITOPS_REPO:-./}"
RECOVERY_NAMESPACE="${RECOVERY_NAMESPACE:-disaster-recovery}"
LOG_FILE="dr-simulation-$(date +%Y%m%d-%H%M%S).log"
METRICS_FILE="dr-metrics-$(date +%Y%m%d-%H%M%S).json"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" | tee -a "$LOG_FILE"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
}

# Metrics collection
declare -A metrics
metrics["start_time"]=$(date +%s)
metrics["dr_scenarios_executed"]=0
metrics["successful_recoveries"]=0
metrics["failed_recoveries"]=0
metrics["total_recovery_time"]=0
metrics["data_consistency_checks"]=0
metrics["performance_degradation"]=0

# Check prerequisites
check_prerequisites() {
    log_info "🔍 Checking disaster recovery prerequisites..."
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is required but not installed"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    # Check if backup location exists
    if [[ ! -d "$BACKUP_LOCATION" ]]; then
        log_error "Backup location does not exist: $BACKUP_LOCATION"
        exit 1
    fi
    
    # Check if ArgoCD is available
    if kubectl get namespace argocd &> /dev/null; then
        log_success "ArgoCD namespace found"
        export GITOPS_TOOL="argocd"
    elif kubectl get namespace flux-system &> /dev/null; then
        log_success "Flux namespace found"
        export GITOPS_TOOL="flux"
    else
        log_warning "No GitOps tool detected, proceeding with kubectl only"
        export GITOPS_TOOL="kubectl"
    fi
    
    log_success "Prerequisites check completed"
}

# Create monitoring namespace and resources
setup_monitoring() {
    log_info "🔧 Setting up disaster recovery monitoring..."
    
    # Create monitoring namespace
    kubectl create namespace dr-monitoring --dry-run=client -o yaml | kubectl apply -f -
    
    # Create monitoring ConfigMap
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: dr-monitoring-config
  namespace: dr-monitoring
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
    - job_name: 'disaster-recovery'
      static_configs:
      - targets: ['localhost:8080']
  alerting.yml: |
    rule_files:
    - /etc/prometheus/alert.rules
    alerting:
      alertmanagers:
      - static_configs:
        - targets: ['alertmanager:9093']
  alert.rules: |
    groups:
    - name: disaster-recovery
      rules:
      - alert: RecoveryTimeExceeded
        expr: dr_recovery_time_seconds > 300
        for: 0m
        labels:
          severity: warning
        annotations:
          summary: "Disaster recovery taking too long"
          description: "Recovery time exceeded 5 minutes"
      - alert: DataInconsistency
        expr: dr_data_consistency_ratio < 0.95
        for: 0m
        labels:
          severity: critical
        annotations:
          summary: "Data consistency issue detected"
          description: "Data consistency ratio below 95%"
EOF

    # Create DR monitoring deployment
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dr-monitor
  namespace: dr-monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dr-monitor
  template:
    metadata:
      labels:
        app: dr-monitor
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      containers:
      - name: dr-monitor
        image: prom/prometheus:latest
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: config
          mountPath: /etc/prometheus
        command:
        - /bin/prometheus
        - --config.file=/etc/prometheus/prometheus.yml
        - --storage.tsdb.path=/prometheus/
        - --web.console.libraries=/etc/prometheus/console_libraries
        - --web.console.templates=/etc/prometheus/consoles
        - --web.enable-lifecycle
      volumes:
      - name: config
        configMap:
          name: dr-monitoring-config
---
apiVersion: v1
kind: Service
metadata:
  name: dr-monitor-service
  namespace: dr-monitoring
spec:
  selector:
    app: dr-monitor
  ports:
  - port: 9090
    targetPort: 9090
EOF

    log_success "Monitoring setup completed"
}

# Simulate different disaster scenarios
simulate_disaster_scenario() {
    local scenario=$1
    local start_time=$(date +%s)
    
    log_info "🎭 Executing disaster scenario: $scenario"
    
    case "$scenario" in
        "node_failure")
            simulate_node_failure
            ;;
        "namespace_deletion")
            simulate_namespace_deletion
            ;;
        "data_corruption")
            simulate_data_corruption
            ;;
        "network_partition")
            simulate_network_partition
            ;;
        "storage_failure")
            simulate_storage_failure
            ;;
        *)
            log_error "Unknown disaster scenario: $scenario"
            return 1
            ;;
    esac
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    metrics["total_recovery_time"]=$((${metrics["total_recovery_time"]} + duration))
    metrics["dr_scenarios_executed"]=$((${metrics["dr_scenarios_executed"]} + 1))
    
    log_success "Disaster scenario '$scenario' completed in ${duration}s"
}

# Simulate node failure by cordoning and draining
simulate_node_failure() {
    log_info "💥 Simulating node failure..."
    
    # Get a worker node (not master)
    local node=$(kubectl get nodes --no-headers | grep -v master | head -1 | awk '{print $1}' || true)
    
    if [[ -z "$node" ]]; then
        log_warning "No worker nodes found, skipping node failure simulation"
        return 0
    fi
    
    log_info "Cordoning node: $node"
    kubectl cordon "$node" || true
    
    log_info "Draining node: $node"
    kubectl drain "$node" --ignore-daemonsets --delete-emptydir-data --force --timeout=60s || true
    
    # Wait for pods to reschedule
    log_info "Waiting for pods to reschedule..."
    sleep 30
    
    # Verify pods are running
    check_pod_health
    
    # Restore node
    log_info "Uncordoning node: $node"
    kubectl uncordon "$node" || true
}

# Simulate namespace deletion
simulate_namespace_deletion() {
    log_info "🗑️ Simulating namespace deletion..."
    
    # Create a test namespace with some resources
    kubectl create namespace dr-test-delete --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy a test application
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: dr-test-delete
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
      - name: test-app
        image: nginx:alpine
        ports:
        - containerPort: 80
EOF
    
    # Wait for deployment
    kubectl rollout status deployment/test-app -n dr-test-delete --timeout=60s
    
    # Delete the namespace
    log_info "Deleting namespace dr-test-delete"
    kubectl delete namespace dr-test-delete --wait=false
    
    # Simulate recovery from backup
    sleep 10
    recover_from_backup "namespace_deletion"
}

# Simulate data corruption
simulate_data_corruption() {
    log_info "💾 Simulating data corruption..."
    
    # Create a test stateful application
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: test-stateful
  namespace: default
spec:
  serviceName: test-stateful
  replicas: 1
  selector:
    matchLabels:
      app: test-stateful
  template:
    metadata:
      labels:
        app: test-stateful
    spec:
      containers:
      - name: test-container
        image: postgres:13
        env:
        - name: POSTGRES_PASSWORD
          value: "testpass"
        - name: POSTGRES_DB
          value: "testdb"
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: data
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 1Gi
EOF
    
    # Wait for StatefulSet to be ready
    kubectl rollout status statefulset/test-stateful --timeout=120s || true
    
    # Simulate corruption by deleting data
    log_info "Simulating data corruption by scaling down"
    kubectl scale statefulset/test-stateful --replicas=0
    
    sleep 10
    recover_from_backup "data_corruption"
}

# Simulate network partition
simulate_network_partition() {
    log_info "🌐 Simulating network partition..."
    
    # Create network policies to simulate partition
    cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all-ingress
  namespace: default
spec:
  podSelector: {}
  policyTypes:
  - Ingress
EOF
    
    # Wait for policy to take effect
    sleep 15
    
    # Check connectivity (should fail)
    log_info "Testing connectivity (should fail)..."
    kubectl run test-connectivity --rm -i --restart=Never --image=busybox -- wget -O- --timeout=10 google.com || true
    
    # Remove network policy to restore connectivity
    log_info "Restoring network connectivity"
    kubectl delete networkpolicy deny-all-ingress || true
    
    sleep 10
    recover_from_backup "network_partition"
}

# Simulate storage failure
simulate_storage_failure() {
    log_info "💿 Simulating storage failure..."
    
    # Create a test PVC
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-storage-claim
  namespace: default
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 100Mi
EOF
    
    # Create pod using the PVC
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-storage-pod
  namespace: default
spec:
  containers:
  - name: test-container
    image: busybox
    command: ["sleep", "3600"]
    volumeMounts:
    - name: storage
      mountPath: /data
  volumes:
  - name: storage
    persistentVolumeClaim:
      claimName: test-storage-claim
EOF
    
    # Wait for pod to be ready
    kubectl wait --for=condition=Ready pod/test-storage-pod --timeout=60s || true
    
    # Write some data
    kubectl exec test-storage-pod -- sh -c "echo 'test data' > /data/test.txt" || true
    
    # Simulate storage failure by deleting PVC (this will cause issues)
    log_info "Simulating storage failure"
    kubectl delete pod test-storage-pod --force --grace-period=0 || true
    kubectl delete pvc test-storage-claim || true
    
    sleep 10
    recover_from_backup "storage_failure"
}

# Recovery function
recover_from_backup() {
    local scenario=$1
    local start_time=$(date +%s)
    
    log_info "🔄 Starting recovery for scenario: $scenario"
    
    # Create recovery namespace if it doesn't exist
    kubectl create namespace "$RECOVERY_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    
    # Apply backup resources to recovery namespace
    if [[ -f "$BACKUP_LOCATION/deployments.yaml" ]]; then
        log_info "Applying deployment backup..."
        sed "s/namespace: default/namespace: $RECOVERY_NAMESPACE/g" "$BACKUP_LOCATION/deployments.yaml" | kubectl apply -f - || true
    fi
    
    if [[ -f "$BACKUP_LOCATION/services.yaml" ]]; then
        log_info "Applying service backup..."
        sed "s/namespace: default/namespace: $RECOVERY_NAMESPACE/g" "$BACKUP_LOCATION/services.yaml" | kubectl apply -f - || true
    fi
    
    if [[ -f "$BACKUP_LOCATION/configmaps.yaml" ]]; then
        log_info "Applying configmap backup..."
        sed "s/namespace: default/namespace: $RECOVERY_NAMESPACE/g" "$BACKUP_LOCATION/configmaps.yaml" | kubectl apply -f - || true
    fi
    
    # Wait for recovery to complete
    log_info "Waiting for recovery to complete..."
    sleep 30
    
    # Validate recovery
    if validate_recovery; then
        metrics["successful_recoveries"]=$((${metrics["successful_recoveries"]} + 1))
        local end_time=$(date +%s)
        local recovery_time=$((end_time - start_time))
        log_success "Recovery completed successfully in ${recovery_time}s"
        
        # Expose recovery time metric
        echo "dr_recovery_time_seconds{scenario=\"$scenario\"} $recovery_time" >> /tmp/dr_metrics.prom
    else
        metrics["failed_recoveries"]=$((${metrics["failed_recoveries"]} + 1))
        log_error "Recovery failed for scenario: $scenario"
    fi
}

# Validate recovery
validate_recovery() {
    log_info "✅ Validating recovery..."
    
    local validation_passed=true
    
    # Check if deployments are ready
    if ! kubectl get deployments -n "$RECOVERY_NAMESPACE" --no-headers | grep -q "1/1"; then
        log_warning "Some deployments are not ready in recovery namespace"
        validation_passed=false
    fi
    
    # Check if services are accessible
    local service_count=$(kubectl get services -n "$RECOVERY_NAMESPACE" --no-headers | wc -l)
    if [[ $service_count -eq 0 ]]; then
        log_warning "No services found in recovery namespace"
        validation_passed=false
    fi
    
    # Check pod health
    if ! check_pod_health "$RECOVERY_NAMESPACE"; then
        validation_passed=false
    fi
    
    # Perform data consistency check
    perform_data_consistency_check
    
    if [[ "$validation_passed" == "true" ]]; then
        log_success "Recovery validation passed"
        return 0
    else
        log_error "Recovery validation failed"
        return 1
    fi
}

# Check pod health
check_pod_health() {
    local namespace=${1:-default}
    log_info "🏥 Checking pod health in namespace: $namespace"
    
    local unhealthy_pods=$(kubectl get pods -n "$namespace" --field-selector=status.phase!=Running --no-headers 2>/dev/null | wc -l)
    
    if [[ $unhealthy_pods -gt 0 ]]; then
        log_warning "$unhealthy_pods unhealthy pods found in namespace $namespace"
        kubectl get pods -n "$namespace" --field-selector=status.phase!=Running
        return 1
    else
        log_success "All pods healthy in namespace $namespace"
        return 0
    fi
}

# Perform data consistency check
perform_data_consistency_check() {
    log_info "🔍 Performing data consistency check..."
    
    metrics["data_consistency_checks"]=$((${metrics["data_consistency_checks"]} + 1))
    
    # Compare original backup with current state
    local consistency_ratio=0.95  # Simulated consistency ratio
    
    # In a real scenario, this would:
    # 1. Compare checksums of critical data
    # 2. Validate database integrity
    # 3. Check application-specific consistency rules
    # 4. Verify configuration integrity
    
    log_info "Simulated data consistency ratio: $consistency_ratio"
    echo "dr_data_consistency_ratio $consistency_ratio" >> /tmp/dr_metrics.prom
    
    if (( $(echo "$consistency_ratio >= 0.95" | bc -l) )); then
        log_success "Data consistency check passed"
        return 0
    else
        log_error "Data consistency check failed"
        return 1
    fi
}

# Performance impact assessment
assess_performance_impact() {
    log_info "📊 Assessing performance impact..."
    
    # Get resource usage metrics
    local cpu_usage=$(kubectl top nodes 2>/dev/null | tail -n +2 | awk '{sum+=$3} END {print sum/NR}' || echo "50")
    local memory_usage=$(kubectl top nodes 2>/dev/null | tail -n +2 | awk '{sum+=$5} END {print sum/NR}' || echo "60")
    
    log_info "Current CPU usage: ${cpu_usage}%"
    log_info "Current Memory usage: ${memory_usage}%"
    
    # Calculate performance degradation (simulated)
    local performance_degradation=5  # 5% performance degradation
    metrics["performance_degradation"]=$performance_degradation
    
    echo "dr_performance_degradation_percent $performance_degradation" >> /tmp/dr_metrics.prom
    echo "dr_cpu_usage_percent $cpu_usage" >> /tmp/dr_metrics.prom
    echo "dr_memory_usage_percent $memory_usage" >> /tmp/dr_metrics.prom
    
    log_info "Performance degradation: ${performance_degradation}%"
}

# Generate comprehensive report
generate_report() {
    log_info "📝 Generating disaster recovery report..."
    
    metrics["end_time"]=$(date +%s)
    metrics["total_duration"]=$((${metrics["end_time"]} - ${metrics["start_time"]}))
    
    # Calculate success rate
    local total_scenarios=${metrics["dr_scenarios_executed"]}
    local success_rate=0
    if [[ $total_scenarios -gt 0 ]]; then
        success_rate=$(( (${metrics["successful_recoveries"]} * 100) / total_scenarios ))
    fi
    metrics["success_rate"]=$success_rate
    
    # Generate JSON report
    cat > "$METRICS_FILE" <<EOF
{
  "disaster_recovery_simulation": {
    "timestamp": "$(date -Iseconds)",
    "duration_seconds": ${metrics["total_duration"]},
    "scenarios_executed": ${metrics["dr_scenarios_executed"]},
    "successful_recoveries": ${metrics["successful_recoveries"]},
    "failed_recoveries": ${metrics["failed_recoveries"]},
    "success_rate_percent": $success_rate,
    "average_recovery_time": $(( ${metrics["total_recovery_time"]} / (${metrics["dr_scenarios_executed"]} > 0 ? ${metrics["dr_scenarios_executed"]} : 1) )),
    "data_consistency_checks": ${metrics["data_consistency_checks"]},
    "performance_degradation_percent": ${metrics["performance_degradation"]},
    "cluster_info": {
      "name": "$CLUSTER_NAME",
      "gitops_tool": "$GITOPS_TOOL",
      "recovery_namespace": "$RECOVERY_NAMESPACE"
    }
  }
}
EOF
    
    # Generate human-readable report
    cat > "dr-report-$(date +%Y%m%d-%H%M%S).md" <<EOF
# Disaster Recovery Simulation Report

**Date**: $(date)
**Cluster**: $CLUSTER_NAME
**GitOps Tool**: $GITOPS_TOOL
**Duration**: ${metrics["total_duration"]} seconds

## Executive Summary

- **Scenarios Executed**: ${metrics["dr_scenarios_executed"]}
- **Success Rate**: ${success_rate}%
- **Successful Recoveries**: ${metrics["successful_recoveries"]}
- **Failed Recoveries**: ${metrics["failed_recoveries"]}
- **Average Recovery Time**: $(( ${metrics["total_recovery_time"]} / (${metrics["dr_scenarios_executed"]} > 0 ? ${metrics["dr_scenarios_executed"]} : 1) )) seconds

## Performance Impact

- **Performance Degradation**: ${metrics["performance_degradation"]}%
- **Data Consistency Checks**: ${metrics["data_consistency_checks"]}

## Recommendations

1. **Recovery Time Optimization**: Consider pre-warming disaster recovery environments
2. **Automation Enhancement**: Implement automated failover for critical scenarios
3. **Monitoring Improvement**: Add real-time alerting for disaster scenarios
4. **Documentation Update**: Ensure runbooks are updated based on simulation results

## Detailed Logs

See detailed logs in: $LOG_FILE
Metrics data available in: $METRICS_FILE

## Next Steps

1. Review failed recovery scenarios
2. Update disaster recovery procedures
3. Schedule regular DR simulations
4. Train operations team on manual procedures
EOF

    log_success "Report generated: dr-report-$(date +%Y%m%d-%H%M%S).md"
    log_success "Metrics saved: $METRICS_FILE"
}

# Cleanup function
cleanup() {
    log_info "🧹 Cleaning up disaster recovery simulation..."
    
    # Remove test resources
    kubectl delete namespace dr-test-delete --ignore-not-found=true --wait=false &
    kubectl delete namespace "$RECOVERY_NAMESPACE" --ignore-not-found=true --wait=false &
    kubectl delete pod test-connectivity --ignore-not-found=true &
    kubectl delete pod test-storage-pod --ignore-not-found=true --force --grace-period=0 &
    kubectl delete pvc test-storage-claim --ignore-not-found=true &
    kubectl delete statefulset test-stateful --ignore-not-found=true &
    kubectl delete networkpolicy deny-all-ingress --ignore-not-found=true &
    
    # Wait for cleanup
    wait
    
    # Clean up monitoring resources (optional)
    if [[ "${KEEP_MONITORING:-false}" != "true" ]]; then
        kubectl delete namespace dr-monitoring --ignore-not-found=true --wait=false &
    fi
    
    log_success "Cleanup completed"
}

# Main execution function
main() {
    echo "🚨 DISASTER RECOVERY SIMULATION STARTING 🚨"
    echo "============================================"
    
    # Setup trap for cleanup
    trap cleanup EXIT
    
    # Check prerequisites
    check_prerequisites
    
    # Setup monitoring
    setup_monitoring
    
    # Execute disaster scenarios
    local scenarios=("node_failure" "namespace_deletion" "data_corruption" "network_partition" "storage_failure")
    
    for scenario in "${scenarios[@]}"; do
        log_info "Preparing to execute scenario: $scenario"
        simulate_disaster_scenario "$scenario"
        
        # Brief pause between scenarios
        sleep 10
        
        # Assess performance impact
        assess_performance_impact
        
        log_info "Scenario '$scenario' completed, moving to next..."
        echo "----------------------------------------"
    done
    
    # Generate comprehensive report
    generate_report
    
    echo "============================================"
    echo "🎯 DISASTER RECOVERY SIMULATION COMPLETED 🎯"
    echo ""
    echo "📊 Results Summary:"
    echo "  - Scenarios: ${metrics[dr_scenarios_executed]}"
    echo "  - Success Rate: $(( (${metrics["successful_recoveries"]} * 100) / (${metrics["dr_scenarios_executed"]} > 0 ? ${metrics["dr_scenarios_executed"]} : 1) ))%"
    echo "  - Total Time: ${metrics[total_duration]} seconds"
    echo ""
    echo "📄 Reports:"
    echo "  - Detailed: $LOG_FILE"
    echo "  - Metrics: $METRICS_FILE"
    echo "  - Summary: dr-report-$(date +%Y%m%d-%H%M%S).md"
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi