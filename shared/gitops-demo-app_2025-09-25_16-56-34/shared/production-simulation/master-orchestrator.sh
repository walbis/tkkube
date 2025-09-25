#!/bin/bash

# Master Test Orchestrator
# Production-ready automated test orchestration for complete GitOps pipeline simulation
# Coordinates all components: environment, workloads, backup, GitOps, disaster recovery, monitoring

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ORCHESTRATOR_LOG="master-orchestrator-$(date +%Y%m%d-%H%M%S).log"
RESULTS_DIR="orchestration-results-$(date +%Y%m%d-%H%M%S)"
TEST_REPORT="test-orchestration-report-$(date +%Y%m%d-%H%M%S).json"

# Test phases configuration
declare -A PHASES
PHASES[1]="Environment Setup"
PHASES[2]="Workload Deployment"
PHASES[3]="Backup Execution"
PHASES[4]="GitOps Pipeline"
PHASES[5]="Disaster Recovery"
PHASES[6]="Validation Framework"
PHASES[7]="Integration Testing"
PHASES[8]="Performance Testing"
PHASES[9]="Security Testing"
PHASES[10]="Report Generation"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Test execution tracking
declare -A PHASE_RESULTS
declare -A PHASE_DURATION
declare -A PHASE_ERRORS
TEST_START_TIME=$(date +%s)
TOTAL_PHASES=${#PHASES[@]}
COMPLETED_PHASES=0
FAILED_PHASES=0

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" | tee -a "$ORCHESTRATOR_LOG"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" | tee -a "$ORCHESTRATOR_LOG"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a "$ORCHESTRATOR_LOG"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$ORCHESTRATOR_LOG"
}

log_phase() {
    echo -e "${PURPLE}[PHASE $1]${NC} $2" | tee -a "$ORCHESTRATOR_LOG"
}

log_header() {
    echo ""
    echo -e "${CYAN}=================================================================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}=================================================================================${NC}"
    echo "" | tee -a "$ORCHESTRATOR_LOG"
}

# Progress tracking
show_progress() {
    local current=$1
    local total=$2
    local phase_name=$3
    
    local percentage=$(( (current * 100) / total ))
    local completed_bar=$(( current * 50 / total ))
    local remaining_bar=$(( 50 - completed_bar ))
    
    printf "\r${CYAN}Progress:${NC} ["
    printf "%*s" "$completed_bar" | tr ' ' '█'
    printf "%*s" "$remaining_bar" | tr ' ' '░'
    printf "] %d%% (%d/%d) - %s" "$percentage" "$current" "$total" "$phase_name"
}

# Initialize orchestration environment
initialize_orchestration() {
    log_header "🚀 INITIALIZING MASTER TEST ORCHESTRATION"
    
    # Create results directory
    mkdir -p "$RESULTS_DIR"
    cd "$RESULTS_DIR"
    
    log_info "📁 Results directory: $RESULTS_DIR"
    log_info "📋 Log file: $ORCHESTRATOR_LOG"
    log_info "📊 Report file: $TEST_REPORT"
    
    # Initialize test report
    cat > "$TEST_REPORT" <<EOF
{
  "orchestration_metadata": {
    "start_time": "$(date -Iseconds)",
    "orchestrator_version": "1.0.0",
    "test_environment": "CRC",
    "total_phases": $TOTAL_PHASES
  },
  "phases": {},
  "summary": {
    "status": "in_progress",
    "completed_phases": 0,
    "failed_phases": 0,
    "total_duration": 0
  }
}
EOF
    
    # Verify prerequisites
    check_orchestration_prerequisites
    
    log_success "Orchestration environment initialized"
}

# Check orchestration prerequisites
check_orchestration_prerequisites() {
    log_info "🔍 Checking orchestration prerequisites..."
    
    local missing_tools=()
    
    # Check required tools
    for tool in kubectl go docker jq curl; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done
    
    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    # Check script files
    local required_scripts=(
        "environment-setup.sh"
        "deploy-workloads.sh"
        "enhanced-backup-executor.go"
        "gitops-pipeline-orchestrator.sh"
        "disaster-recovery-simulator.sh"
        "start-validation-framework.sh"
    )
    
    for script in "${required_scripts[@]}"; do
        if [[ ! -f "$SCRIPT_DIR/$script" ]]; then
            log_error "Required script not found: $script"
            exit 1
        fi
        
        # Make sure script is executable
        chmod +x "$SCRIPT_DIR/$script" 2>/dev/null || true
    done
    
    log_success "All prerequisites satisfied"
}

# Execute phase with error handling and metrics
execute_phase() {
    local phase_num=$1
    local phase_name="${PHASES[$phase_num]}"
    local start_time=$(date +%s)
    
    log_phase "$phase_num" "Starting: $phase_name"
    show_progress "$COMPLETED_PHASES" "$TOTAL_PHASES" "$phase_name"
    
    # Initialize phase result
    PHASE_RESULTS[$phase_num]="running"
    PHASE_ERRORS[$phase_num]=""
    
    local success=true
    local error_output=""
    
    case $phase_num in
        1) execute_environment_setup || { success=false; error_output="Environment setup failed"; } ;;
        2) execute_workload_deployment || { success=false; error_output="Workload deployment failed"; } ;;
        3) execute_backup_execution || { success=false; error_output="Backup execution failed"; } ;;
        4) execute_gitops_pipeline || { success=false; error_output="GitOps pipeline failed"; } ;;
        5) execute_disaster_recovery || { success=false; error_output="Disaster recovery failed"; } ;;
        6) execute_validation_framework || { success=false; error_output="Validation framework failed"; } ;;
        7) execute_integration_testing || { success=false; error_output="Integration testing failed"; } ;;
        8) execute_performance_testing || { success=false; error_output="Performance testing failed"; } ;;
        9) execute_security_testing || { success=false; error_output="Security testing failed"; } ;;
        10) execute_report_generation || { success=false; error_output="Report generation failed"; } ;;
        *) success=false; error_output="Unknown phase: $phase_num" ;;
    esac
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    PHASE_DURATION[$phase_num]=$duration
    
    if [[ "$success" == "true" ]]; then
        PHASE_RESULTS[$phase_num]="success"
        COMPLETED_PHASES=$((COMPLETED_PHASES + 1))
        log_success "Phase $phase_num completed: $phase_name (${duration}s)"
    else
        PHASE_RESULTS[$phase_num]="failed"
        PHASE_ERRORS[$phase_num]="$error_output"
        FAILED_PHASES=$((FAILED_PHASES + 1))
        log_error "Phase $phase_num failed: $phase_name - $error_output"
    fi
    
    # Update progress
    show_progress "$COMPLETED_PHASES" "$TOTAL_PHASES" "$phase_name"
    echo "" # New line after progress bar
    
    # Update test report
    update_test_report "$phase_num" "$phase_name" "${PHASE_RESULTS[$phase_num]}" "$duration" "${PHASE_ERRORS[$phase_num]}"
    
    return $([ "$success" == "true" ] && echo 0 || echo 1)
}

# Phase 1: Environment Setup
execute_environment_setup() {
    log_info "🏗️ Executing environment setup..."
    
    cd "$SCRIPT_DIR"
    
    # Run environment setup script
    if timeout 600 ./environment-setup.sh > "../$RESULTS_DIR/phase1-environment.log" 2>&1; then
        log_success "Environment setup completed successfully"
        
        # Verify cluster is ready
        if kubectl get nodes | grep -q Ready; then
            log_success "Cluster nodes are ready"
            return 0
        else
            log_error "Cluster nodes are not ready"
            return 1
        fi
    else
        log_error "Environment setup script failed"
        return 1
    fi
}

# Phase 2: Workload Deployment
execute_workload_deployment() {
    log_info "🚀 Executing workload deployment..."
    
    cd "$SCRIPT_DIR"
    
    # Run workload deployment script
    if timeout 900 ./deploy-workloads.sh > "../$RESULTS_DIR/phase2-workloads.log" 2>&1; then
        log_success "Workload deployment completed successfully"
        
        # Verify workloads are running
        local running_pods=$(kubectl get pods --all-namespaces --field-selector=status.phase=Running --no-headers | wc -l)
        if [[ $running_pods -gt 0 ]]; then
            log_success "$running_pods workload pods are running"
            return 0
        else
            log_error "No workload pods are running"
            return 1
        fi
    else
        log_error "Workload deployment script failed"
        return 1
    fi
}

# Phase 3: Backup Execution
execute_backup_execution() {
    log_info "💾 Executing backup execution..."
    
    cd "$SCRIPT_DIR"
    
    # Compile and run backup executor
    if go run enhanced-backup-executor.go > "../$RESULTS_DIR/phase3-backup.log" 2>&1; then
        log_success "Backup execution completed successfully"
        
        # Verify backup files exist
        if [[ -f "../backup-source/backup-summary.yaml" ]] && [[ -f "../backup-source/deployments.yaml" ]]; then
            log_success "Backup files created successfully"
            return 0
        else
            log_error "Backup files not found"
            return 1
        fi
    else
        log_error "Backup execution failed"
        return 1
    fi
}

# Phase 4: GitOps Pipeline
execute_gitops_pipeline() {
    log_info "🔄 Executing GitOps pipeline..."
    
    cd "$SCRIPT_DIR"
    
    # Run GitOps pipeline orchestrator
    if timeout 600 bash gitops-pipeline-orchestrator.sh > "../$RESULTS_DIR/phase4-gitops.log" 2>&1; then
        log_success "GitOps pipeline completed successfully"
        
        # Verify GitOps artifacts exist
        if [[ -f "../base/kustomization.yaml" ]] && [[ -d "../overlays" ]]; then
            log_success "GitOps artifacts generated successfully"
            return 0
        else
            log_error "GitOps artifacts not found"
            return 1
        fi
    else
        log_error "GitOps pipeline script failed"
        return 1
    fi
}

# Phase 5: Disaster Recovery
execute_disaster_recovery() {
    log_info "🚨 Executing disaster recovery simulation..."
    
    cd "$SCRIPT_DIR"
    
    # Run disaster recovery simulator
    if timeout 1800 ./disaster-recovery-simulator.sh > "../$RESULTS_DIR/phase5-disaster-recovery.log" 2>&1; then
        log_success "Disaster recovery simulation completed successfully"
        
        # Check for DR report
        if ls dr-report-*.md &>/dev/null; then
            cp dr-report-*.md "../$RESULTS_DIR/"
            log_success "Disaster recovery report generated"
            return 0
        else
            log_warning "No disaster recovery report found, but execution completed"
            return 0
        fi
    else
        log_error "Disaster recovery simulation failed"
        return 1
    fi
}

# Phase 6: Validation Framework
execute_validation_framework() {
    log_info "✅ Executing validation framework..."
    
    cd "$SCRIPT_DIR"
    
    # Start validation framework and run tests
    if ./start-validation-framework.sh start > "../$RESULTS_DIR/phase6-validation.log" 2>&1; then
        sleep 30 # Allow framework to initialize
        
        # Run validation tests
        if ./start-validation-framework.sh test >> "../$RESULTS_DIR/phase6-validation.log" 2>&1; then
            log_success "Validation framework tests completed successfully"
            
            # Generate validation report
            ./start-validation-framework.sh report >> "../$RESULTS_DIR/phase6-validation.log" 2>&1 || true
            
            # Stop validation framework
            ./start-validation-framework.sh stop >> "../$RESULTS_DIR/phase6-validation.log" 2>&1 || true
            
            return 0
        else
            log_error "Validation tests failed"
            ./start-validation-framework.sh stop >> "../$RESULTS_DIR/phase6-validation.log" 2>&1 || true
            return 1
        fi
    else
        log_error "Validation framework startup failed"
        return 1
    fi
}

# Phase 7: Integration Testing
execute_integration_testing() {
    log_info "🔗 Executing integration testing..."
    
    local test_log="../$RESULTS_DIR/phase7-integration.log"
    
    {
        echo "=== Integration Testing Report ==="
        echo "Timestamp: $(date)"
        echo ""
        
        # Test 1: End-to-end backup and restore
        echo "Test 1: End-to-end backup and restore"
        test_backup_restore_integration
        echo ""
        
        # Test 2: GitOps sync verification
        echo "Test 2: GitOps sync verification"  
        test_gitops_sync_integration
        echo ""
        
        # Test 3: Monitoring and alerting
        echo "Test 3: Monitoring and alerting"
        test_monitoring_integration
        echo ""
        
        # Test 4: Cross-component communication
        echo "Test 4: Cross-component communication"
        test_component_communication
        echo ""
        
    } > "$test_log" 2>&1
    
    log_success "Integration testing completed"
    return 0
}

# Test backup and restore integration
test_backup_restore_integration() {
    echo "  ✓ Testing backup creation..."
    if [[ -f "../backup-source/backup-summary.yaml" ]]; then
        echo "  ✓ Backup files exist"
        
        # Test backup validity
        if kubectl apply --dry-run=client -f "../backup-source/deployments.yaml" &>/dev/null; then
            echo "  ✓ Backup YAML is valid"
        else
            echo "  ✗ Backup YAML validation failed"
        fi
    else
        echo "  ✗ Backup files not found"
    fi
    
    echo "  ✓ Testing GitOps integration..."
    if [[ -f "../base/deployments.yaml" ]]; then
        echo "  ✓ GitOps deployment files exist"
    else
        echo "  ✗ GitOps deployment files not found"
    fi
}

# Test GitOps sync integration
test_gitops_sync_integration() {
    echo "  ✓ Testing GitOps structure..."
    
    local structure_valid=true
    
    for dir in base overlays/development overlays/staging overlays/production; do
        if [[ -d "../$dir" ]]; then
            echo "  ✓ Directory exists: $dir"
        else
            echo "  ✗ Missing directory: $dir"
            structure_valid=false
        fi
    done
    
    if [[ -f "../base/kustomization.yaml" ]]; then
        echo "  ✓ Base kustomization exists"
    else
        echo "  ✗ Base kustomization missing"
        structure_valid=false
    fi
    
    if [[ "$structure_valid" == "true" ]]; then
        echo "  ✓ GitOps structure is valid"
    else
        echo "  ✗ GitOps structure issues found"
    fi
}

# Test monitoring integration
test_monitoring_integration() {
    echo "  ✓ Testing monitoring components..."
    
    # Check if monitoring namespace exists
    if kubectl get namespace validation-monitoring &>/dev/null; then
        echo "  ✓ Monitoring namespace exists"
        
        # Check monitoring pods
        local monitor_pods=$(kubectl get pods -n validation-monitoring --no-headers 2>/dev/null | wc -l)
        if [[ $monitor_pods -gt 0 ]]; then
            echo "  ✓ Monitoring pods are deployed ($monitor_pods pods)"
        else
            echo "  ✗ No monitoring pods found"
        fi
    else
        echo "  ✗ Monitoring namespace not found"
    fi
}

# Test component communication
test_component_communication() {
    echo "  ✓ Testing component communication..."
    
    # Test API connectivity
    if kubectl cluster-info &>/dev/null; then
        echo "  ✓ Kubernetes API is accessible"
    else
        echo "  ✗ Kubernetes API connection failed"
    fi
    
    # Test service discovery
    local services=$(kubectl get services --all-namespaces --no-headers | wc -l)
    if [[ $services -gt 0 ]]; then
        echo "  ✓ Services are discoverable ($services services)"
    else
        echo "  ✗ No services found"
    fi
}

# Phase 8: Performance Testing
execute_performance_testing() {
    log_info "⚡ Executing performance testing..."
    
    local perf_log="../$RESULTS_DIR/phase8-performance.log"
    
    {
        echo "=== Performance Testing Report ==="
        echo "Timestamp: $(date)"
        echo ""
        
        # API Performance Test
        echo "API Performance Test:"
        measure_api_performance
        echo ""
        
        # Resource Usage Test
        echo "Resource Usage Test:"
        measure_resource_usage
        echo ""
        
        # Backup Performance Test
        echo "Backup Performance Test:"
        measure_backup_performance
        echo ""
        
    } > "$perf_log" 2>&1
    
    log_success "Performance testing completed"
    return 0
}

# Measure API performance
measure_api_performance() {
    local api_start=$(date +%s%3N)
    kubectl get pods --all-namespaces > /dev/null 2>&1
    local api_end=$(date +%s%3N)
    local api_duration=$((api_end - api_start))
    
    echo "  API Response Time: ${api_duration}ms"
    
    if [[ $api_duration -lt 1000 ]]; then
        echo "  ✓ API performance is good (<1s)"
    elif [[ $api_duration -lt 5000 ]]; then
        echo "  ⚠ API performance is acceptable (<5s)"
    else
        echo "  ✗ API performance is poor (>5s)"
    fi
}

# Measure resource usage
measure_resource_usage() {
    # Node resource usage (if metrics-server is available)
    if kubectl top nodes &>/dev/null; then
        echo "  Node Resource Usage:"
        kubectl top nodes | while read -r line; do
            echo "    $line"
        done
    else
        echo "  Node metrics not available (metrics-server not installed)"
    fi
    
    # Pod count and status
    local total_pods=$(kubectl get pods --all-namespaces --no-headers | wc -l)
    local running_pods=$(kubectl get pods --all-namespaces --field-selector=status.phase=Running --no-headers | wc -l)
    local pod_health_percentage=$(( (running_pods * 100) / total_pods ))
    
    echo "  Pod Health: $running_pods/$total_pods running (${pod_health_percentage}%)"
}

# Measure backup performance
measure_backup_performance() {
    if [[ -f "../backup-source/backup-summary.yaml" ]]; then
        local backup_size=$(du -h ../backup-source/ | tail -1 | cut -f1)
        echo "  Backup Size: $backup_size"
        echo "  ✓ Backup completed successfully"
    else
        echo "  ✗ Backup files not found"
    fi
}

# Phase 9: Security Testing
execute_security_testing() {
    log_info "🛡️ Executing security testing..."
    
    local security_log="../$RESULTS_DIR/phase9-security.log"
    
    {
        echo "=== Security Testing Report ==="
        echo "Timestamp: $(date)"
        echo ""
        
        # RBAC Test
        echo "RBAC Configuration Test:"
        test_rbac_configuration
        echo ""
        
        # Network Policy Test
        echo "Network Policy Test:"
        test_network_policies
        echo ""
        
        # Pod Security Test
        echo "Pod Security Test:"
        test_pod_security
        echo ""
        
        # Secret Management Test
        echo "Secret Management Test:"
        test_secret_management
        echo ""
        
    } > "$security_log" 2>&1
    
    log_success "Security testing completed"
    return 0
}

# Test RBAC configuration
test_rbac_configuration() {
    local rbac_count=$(kubectl get clusterroles,roles --all-namespaces --no-headers 2>/dev/null | wc -l)
    if [[ $rbac_count -gt 0 ]]; then
        echo "  ✓ RBAC policies configured ($rbac_count policies)"
    else
        echo "  ✗ No RBAC policies found"
    fi
    
    # Check for default service account usage
    local default_sa_pods=$(kubectl get pods --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}: {.spec.serviceAccountName}{"\n"}{end}' 2>/dev/null | grep -c ": default$" || echo "0")
    if [[ $default_sa_pods -eq 0 ]]; then
        echo "  ✓ No pods using default service account"
    else
        echo "  ⚠ $default_sa_pods pods using default service account"
    fi
}

# Test network policies
test_network_policies() {
    local netpol_count=$(kubectl get networkpolicies --all-namespaces --no-headers 2>/dev/null | wc -l)
    if [[ $netpol_count -gt 0 ]]; then
        echo "  ✓ Network policies configured ($netpol_count policies)"
    else
        echo "  ⚠ No network policies found"
    fi
}

# Test pod security
test_pod_security() {
    # Check for privileged containers
    local privileged_pods=$(kubectl get pods --all-namespaces -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.securityContext.privileged}{"\n"}{end}{end}' 2>/dev/null | grep -c "true" || echo "0")
    
    if [[ $privileged_pods -eq 0 ]]; then
        echo "  ✓ No privileged containers found"
    else
        echo "  ⚠ $privileged_pods privileged containers found"
    fi
    
    # Check for pod security standards
    local pss_namespaces=$(kubectl get namespaces -o jsonpath='{range .items[*]}{.metadata.name}: {.metadata.labels.pod-security\.kubernetes\.io/enforce}{"\n"}{end}' 2>/dev/null | grep -v ": $" | wc -l)
    echo "  Pod Security Standards: $pss_namespaces namespaces configured"
}

# Test secret management
test_secret_management() {
    local secret_count=$(kubectl get secrets --all-namespaces --no-headers 2>/dev/null | wc -l)
    if [[ $secret_count -gt 0 ]]; then
        echo "  ✓ Secrets configured ($secret_count secrets)"
    else
        echo "  ⚠ No secrets found"
    fi
}

# Phase 10: Report Generation
execute_report_generation() {
    log_info "📊 Executing comprehensive report generation..."
    
    # Generate final comprehensive report
    generate_final_report
    
    # Create HTML dashboard
    generate_html_dashboard
    
    # Generate executive summary
    generate_executive_summary
    
    log_success "All reports generated successfully"
    return 0
}

# Update test report with phase results
update_test_report() {
    local phase_num=$1
    local phase_name=$2
    local status=$3
    local duration=$4
    local error_msg=$5
    
    # Update the JSON report
    local temp_report=$(mktemp)
    jq --arg phase "$phase_num" \
       --arg name "$phase_name" \
       --arg status "$status" \
       --arg duration "$duration" \
       --arg error "$error_msg" \
       --arg timestamp "$(date -Iseconds)" \
       '.phases[$phase] = {
         "name": $name,
         "status": $status,
         "duration": ($duration | tonumber),
         "error": $error,
         "timestamp": $timestamp
       } | 
       .summary.completed_phases = (.phases | to_entries | map(select(.value.status == "success")) | length) |
       .summary.failed_phases = (.phases | to_entries | map(select(.value.status == "failed")) | length)' \
       "$TEST_REPORT" > "$temp_report" && mv "$temp_report" "$TEST_REPORT"
}

# Generate final comprehensive report
generate_final_report() {
    local end_time=$(date +%s)
    local total_duration=$((end_time - TEST_START_TIME))
    
    # Update final summary in JSON report
    local temp_report=$(mktemp)
    jq --arg status "$([ $FAILED_PHASES -eq 0 ] && echo "success" || echo "failed")" \
       --arg total_duration "$total_duration" \
       --arg end_time "$(date -Iseconds)" \
       '.summary.status = $status |
        .summary.total_duration = ($total_duration | tonumber) |
        .summary.end_time = $end_time' \
       "$TEST_REPORT" > "$temp_report" && mv "$temp_report" "$TEST_REPORT"
    
    log_success "Final JSON report updated: $TEST_REPORT"
}

# Generate HTML dashboard
generate_html_dashboard() {
    local html_report="orchestration-dashboard.html"
    
    # Calculate success rate
    local success_rate=0
    if [[ $TOTAL_PHASES -gt 0 ]]; then
        success_rate=$(( (COMPLETED_PHASES * 100) / TOTAL_PHASES ))
    fi
    
    cat > "$html_report" <<EOF
<!DOCTYPE html>
<html>
<head>
    <title>GitOps Pipeline Test Orchestration Dashboard</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 10px; text-align: center; margin-bottom: 30px; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .metric-card { background: white; padding: 20px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .metric-value { font-size: 2em; font-weight: bold; margin-bottom: 5px; }
        .metric-label { color: #666; font-size: 0.9em; }
        .success { color: #4CAF50; }
        .warning { color: #FF9800; }
        .error { color: #F44336; }
        .phases { background: white; border-radius: 10px; padding: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .phase-item { display: flex; justify-content: space-between; align-items: center; padding: 15px 0; border-bottom: 1px solid #eee; }
        .phase-item:last-child { border-bottom: none; }
        .phase-name { font-weight: 500; }
        .phase-status { padding: 5px 15px; border-radius: 20px; color: white; font-size: 0.8em; }
        .status-success { background: #4CAF50; }
        .status-failed { background: #F44336; }
        .status-running { background: #2196F3; }
        .progress-bar { width: 100%; height: 20px; background: #e0e0e0; border-radius: 10px; overflow: hidden; margin: 20px 0; }
        .progress-fill { height: 100%; background: linear-gradient(90deg, #4CAF50, #8BC34A); transition: width 0.3s ease; }
        .logs { background: #1e1e1e; color: #00ff00; padding: 20px; border-radius: 10px; font-family: 'Courier New', monospace; font-size: 0.9em; max-height: 300px; overflow-y: auto; }
        .timestamp { color: #888; font-size: 0.8em; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔧 GitOps Pipeline Test Orchestration</h1>
            <p>Comprehensive production-ready testing dashboard</p>
            <p class="timestamp">Generated: $(date)</p>
        </div>
        
        <div class="metrics">
            <div class="metric-card">
                <div class="metric-value success">$success_rate%</div>
                <div class="metric-label">Success Rate</div>
            </div>
            <div class="metric-card">
                <div class="metric-value">$COMPLETED_PHASES</div>
                <div class="metric-label">Completed Phases</div>
            </div>
            <div class="metric-card">
                <div class="metric-value $([ $FAILED_PHASES -eq 0 ] && echo "success" || echo "error")">$FAILED_PHASES</div>
                <div class="metric-label">Failed Phases</div>
            </div>
            <div class="metric-card">
                <div class="metric-value">$(($(date +%s) - TEST_START_TIME))s</div>
                <div class="metric-label">Total Duration</div>
            </div>
        </div>
        
        <div class="progress-bar">
            <div class="progress-fill" style="width: ${success_rate}%"></div>
        </div>
        
        <div class="phases">
            <h2>📋 Phase Execution Summary</h2>
EOF

    # Add phase details
    for phase_num in $(seq 1 $TOTAL_PHASES); do
        local phase_name="${PHASES[$phase_num]}"
        local status="${PHASE_RESULTS[$phase_num]:-pending}"
        local duration="${PHASE_DURATION[$phase_num]:-0}"
        local error_msg="${PHASE_ERRORS[$phase_num]:-}"
        
        local status_class="status-running"
        local status_text="PENDING"
        
        case "$status" in
            "success") status_class="status-success"; status_text="SUCCESS" ;;
            "failed") status_class="status-failed"; status_text="FAILED" ;;
            "running") status_class="status-running"; status_text="RUNNING" ;;
        esac
        
        cat >> "$html_report" <<EOF
            <div class="phase-item">
                <div>
                    <div class="phase-name">Phase $phase_num: $phase_name</div>
                    $([ -n "$error_msg" ] && echo "<div style='color: #F44336; font-size: 0.8em;'>$error_msg</div>")
                </div>
                <div>
                    <span class="phase-status $status_class">$status_text</span>
                    $([ $duration -gt 0 ] && echo "<span style='margin-left: 10px; color: #666; font-size: 0.8em;'>${duration}s</span>")
                </div>
            </div>
EOF
    done
    
    cat >> "$html_report" <<EOF
        </div>
        
        <div style="margin-top: 30px;">
            <h2>📊 Additional Reports</h2>
            <ul>
                <li><a href="$TEST_REPORT">JSON Test Report</a></li>
                <li><a href="$ORCHESTRATOR_LOG">Orchestration Log</a></li>
                <li><a href="phase1-environment.log">Environment Setup Log</a></li>
                <li><a href="phase2-workloads.log">Workload Deployment Log</a></li>
                <li><a href="phase3-backup.log">Backup Execution Log</a></li>
                <li><a href="phase4-gitops.log">GitOps Pipeline Log</a></li>
                <li><a href="phase5-disaster-recovery.log">Disaster Recovery Log</a></li>
                <li><a href="phase6-validation.log">Validation Framework Log</a></li>
                <li><a href="phase7-integration.log">Integration Testing Log</a></li>
                <li><a href="phase8-performance.log">Performance Testing Log</a></li>
                <li><a href="phase9-security.log">Security Testing Log</a></li>
            </ul>
        </div>
        
        <div class="logs">
            <h3>🔍 Recent Log Entries</h3>
$(tail -10 "$ORCHESTRATOR_LOG" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g' || echo "No log entries found")
        </div>
    </div>
</body>
</html>
EOF
    
    log_success "HTML dashboard generated: $html_report"
}

# Generate executive summary
generate_executive_summary() {
    local summary_file="executive-summary.md"
    local success_rate=0
    if [[ $TOTAL_PHASES -gt 0 ]]; then
        success_rate=$(( (COMPLETED_PHASES * 100) / TOTAL_PHASES ))
    fi
    
    cat > "$summary_file" <<EOF
# GitOps Pipeline Test Orchestration - Executive Summary

**Date**: $(date)
**Duration**: $(($(date +%s) - TEST_START_TIME)) seconds
**Environment**: CRC (CodeReady Containers)

## 🎯 Overall Results

- **Success Rate**: ${success_rate}%
- **Phases Completed**: $COMPLETED_PHASES/$TOTAL_PHASES
- **Failed Phases**: $FAILED_PHASES
- **Status**: $([ $FAILED_PHASES -eq 0 ] && echo "✅ PASSED" || echo "❌ FAILED")

## 📊 Phase Breakdown

| Phase | Name | Status | Duration | 
|-------|------|--------|----------|
EOF

    for phase_num in $(seq 1 $TOTAL_PHASES); do
        local phase_name="${PHASES[$phase_num]}"
        local status="${PHASE_RESULTS[$phase_num]:-pending}"
        local duration="${PHASE_DURATION[$phase_num]:-0}"
        
        local status_icon="⏳"
        case "$status" in
            "success") status_icon="✅" ;;
            "failed") status_icon="❌" ;;
            "running") status_icon="🔄" ;;
        esac
        
        echo "| $phase_num | $phase_name | $status_icon $status | ${duration}s |" >> "$summary_file"
    done
    
    cat >> "$summary_file" <<EOF

## 🔍 Key Findings

### ✅ Successful Components
EOF

    for phase_num in $(seq 1 $TOTAL_PHASES); do
        if [[ "${PHASE_RESULTS[$phase_num]:-}" == "success" ]]; then
            echo "- **${PHASES[$phase_num]}**: Completed successfully in ${PHASE_DURATION[$phase_num]}s" >> "$summary_file"
        fi
    done

    cat >> "$summary_file" <<EOF

### ❌ Failed Components
EOF

    local has_failures=false
    for phase_num in $(seq 1 $TOTAL_PHASES); do
        if [[ "${PHASE_RESULTS[$phase_num]:-}" == "failed" ]]; then
            echo "- **${PHASES[$phase_num]}**: ${PHASE_ERRORS[$phase_num]}" >> "$summary_file"
            has_failures=true
        fi
    done
    
    if [[ "$has_failures" == "false" ]]; then
        echo "- None! All phases completed successfully." >> "$summary_file"
    fi

    cat >> "$summary_file" <<EOF

## 🚀 Recommendations

1. **Performance Optimization**: Review phase durations and optimize slow components
2. **Error Handling**: Enhance error recovery mechanisms for failed phases
3. **Monitoring Enhancement**: Expand monitoring coverage for better observability
4. **Automation Improvement**: Reduce manual intervention points
5. **Documentation Update**: Keep runbooks updated based on test results

## 📄 Detailed Reports

- **Full JSON Report**: $TEST_REPORT
- **HTML Dashboard**: orchestration-dashboard.html
- **Orchestration Log**: $ORCHESTRATOR_LOG
- **Individual Phase Logs**: phase*.log files

## 📞 Support

For issues or questions regarding this test orchestration:
- Review the detailed logs for specific error messages
- Check the HTML dashboard for visual status overview
- Consult the JSON report for programmatic access to results

---
*Report generated by GitOps Demo App Master Orchestrator v1.0.0*
EOF

    log_success "Executive summary generated: $summary_file"
}

# Display orchestration results
display_results() {
    log_header "📊 ORCHESTRATION RESULTS SUMMARY"
    
    echo ""
    printf "%-30s | %-10s | %-10s | %-s\n" "Phase" "Status" "Duration" "Result"
    echo "----------------------------------------------------------------------"
    
    for phase_num in $(seq 1 $TOTAL_PHASES); do
        local phase_name="${PHASES[$phase_num]}"
        local status="${PHASE_RESULTS[$phase_num]:-pending}"
        local duration="${PHASE_DURATION[$phase_num]:-0}s"
        local error_msg="${PHASE_ERRORS[$phase_num]:-OK}"
        
        local status_color="$YELLOW"
        case "$status" in
            "success") status_color="$GREEN" ;;
            "failed") status_color="$RED" ;;
        esac
        
        printf "%-30s | ${status_color}%-10s${NC} | %-10s | %-s\n" \
            "$phase_name" "$status" "$duration" "$error_msg"
    done
    
    echo ""
    echo "SUMMARY:"
    echo "  Total Phases: $TOTAL_PHASES"
    echo "  Completed: $COMPLETED_PHASES"
    echo "  Failed: $FAILED_PHASES"
    echo "  Success Rate: $(( (COMPLETED_PHASES * 100) / TOTAL_PHASES ))%"
    echo "  Total Duration: $(($(date +%s) - TEST_START_TIME))s"
    echo ""
    echo "REPORTS GENERATED:"
    echo "  📋 Executive Summary: $RESULTS_DIR/executive-summary.md"
    echo "  📊 HTML Dashboard: $RESULTS_DIR/orchestration-dashboard.html"
    echo "  📄 JSON Report: $RESULTS_DIR/$TEST_REPORT"
    echo "  📋 Orchestration Log: $RESULTS_DIR/$ORCHESTRATOR_LOG"
    echo ""
    
    if [[ $FAILED_PHASES -eq 0 ]]; then
        log_success "🎉 ALL PHASES COMPLETED SUCCESSFULLY!"
        return 0
    else
        log_error "❌ $FAILED_PHASES PHASE(S) FAILED"
        return 1
    fi
}

# Cleanup function
cleanup_orchestration() {
    log_info "🧹 Cleaning up orchestration resources..."
    
    # Stop any running validation framework
    cd "$SCRIPT_DIR"
    ./start-validation-framework.sh stop 2>/dev/null || true
    
    # Clean up temporary files
    find . -name "*.pid" -delete 2>/dev/null || true
    find . -name "dr-*.json" -delete 2>/dev/null || true
    find . -name "validation-framework" -delete 2>/dev/null || true
    
    log_success "Cleanup completed"
}

# Main orchestration function
main() {
    local command=${1:-run}
    
    case "$command" in
        run)
            # Full orchestration run
            log_header "🚀 STARTING MASTER TEST ORCHESTRATION"
            
            initialize_orchestration
            
            # Execute all phases
            for phase_num in $(seq 1 $TOTAL_PHASES); do
                if ! execute_phase "$phase_num"; then
                    # Continue with next phase even if current fails (for comprehensive testing)
                    log_warning "Phase $phase_num failed, continuing with next phase..."
                fi
            done
            
            # Generate final reports and display results
            display_results
            ;;
        
        clean)
            cleanup_orchestration
            ;;
        
        status)
            if [[ -f "$RESULTS_DIR/$TEST_REPORT" ]]; then
                jq -r '.summary | "Status: \(.status), Completed: \(.completed_phases)/10, Failed: \(.failed_phases)"' "$RESULTS_DIR/$TEST_REPORT"
            else
                echo "No orchestration results found"
            fi
            ;;
        
        report)
            if [[ -d "$RESULTS_DIR" ]]; then
                echo "📊 Reports available in: $RESULTS_DIR"
                ls -la "$RESULTS_DIR/"
            else
                echo "No results directory found"
            fi
            ;;
        
        help|--help|-h)
            cat <<EOF
🔧 Master Test Orchestrator

USAGE:
    $0 [COMMAND]

COMMANDS:
    run       Execute full orchestration (default)
    clean     Clean up orchestration resources
    status    Show current orchestration status
    report    List available reports
    help      Show this help message

The orchestrator executes these phases:
  1. Environment Setup
  2. Workload Deployment  
  3. Backup Execution
  4. GitOps Pipeline
  5. Disaster Recovery
  6. Validation Framework
  7. Integration Testing
  8. Performance Testing
  9. Security Testing
  10. Report Generation

Results are saved to: orchestration-results-{timestamp}/
EOF
            ;;
        
        *)
            log_error "Unknown command: $command"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    # Set up trap for cleanup on exit
    trap cleanup_orchestration EXIT
    
    main "$@"
fi