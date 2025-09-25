#!/bin/bash

# Validation and Monitoring Framework Starter
# Production-ready validation framework with comprehensive monitoring
# Part of the GitOps Demo Pipeline Integration

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${CONFIG_FILE:-$SCRIPT_DIR/validation-config.yaml}"
LOG_FILE="${LOG_FILE:-validation-framework-$(date +%Y%m%d-%H%M%S).log}"
PID_FILE="${PID_FILE:-validation-framework.pid}"
METRICS_PORT="${METRICS_PORT:-8080}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
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

log_header() {
    echo -e "${PURPLE}[FRAMEWORK]${NC} $1" | tee -a "$LOG_FILE"
}

# Check prerequisites
check_prerequisites() {
    log_info "🔍 Checking validation framework prerequisites..."
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is required but not installed"
        exit 1
    fi
    
    # Check if go is available
    if ! command -v go &> /dev/null; then
        log_error "Go is required but not installed"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    # Check if config file exists
    if [[ ! -f "$CONFIG_FILE" ]]; then
        log_warning "Config file not found: $CONFIG_FILE, using defaults"
    fi
    
    # Check if port is available
    if netstat -tuln 2>/dev/null | grep -q ":$METRICS_PORT "; then
        log_warning "Port $METRICS_PORT is already in use"
        METRICS_PORT=$((METRICS_PORT + 1))
        log_info "Using alternative port: $METRICS_PORT"
    fi
    
    log_success "Prerequisites check completed"
}

# Install Go dependencies
install_dependencies() {
    log_info "📦 Installing Go dependencies..."
    
    cd "$SCRIPT_DIR"
    
    # Initialize go module if not exists
    if [[ ! -f "go.mod" ]]; then
        log_info "Initializing Go module..."
        go mod init validation-monitoring-framework
    fi
    
    # Install required dependencies
    log_info "Installing Kubernetes client dependencies..."
    go get k8s.io/api/core/v1@latest
    go get k8s.io/apimachinery/pkg/apis/meta/v1@latest
    go get k8s.io/client-go/kubernetes@latest
    go get k8s.io/client-go/tools/clientcmd@latest
    go get k8s.io/metrics/pkg/client/clientset/versioned@latest
    go get gopkg.in/yaml.v2@latest
    
    # Tidy up dependencies
    go mod tidy
    
    log_success "Dependencies installed successfully"
}

# Build the validation framework
build_framework() {
    log_info "🏗️ Building validation framework..."
    
    cd "$SCRIPT_DIR"
    
    # Build the Go binary
    if go build -o validation-framework validation-monitoring-framework.go; then
        log_success "Validation framework built successfully"
    else
        log_error "Failed to build validation framework"
        exit 1
    fi
    
    # Make it executable
    chmod +x validation-framework
}

# Setup monitoring dashboard
setup_monitoring_dashboard() {
    log_info "📊 Setting up monitoring dashboard..."
    
    # Create monitoring namespace
    kubectl create namespace validation-monitoring --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy simple monitoring dashboard
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: monitoring-dashboard
  namespace: validation-monitoring
data:
  index.html: |
    <!DOCTYPE html>
    <html>
    <head>
        <title>Validation Framework Dashboard</title>
        <meta http-equiv="refresh" content="30">
        <style>
            body { font-family: Arial, sans-serif; margin: 20px; }
            .header { background: #2196F3; color: white; padding: 15px; border-radius: 5px; }
            .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin: 20px 0; }
            .metric-card { border: 1px solid #ddd; padding: 15px; border-radius: 5px; background: #f9f9f9; }
            .metric-value { font-size: 24px; font-weight: bold; color: #2196F3; }
            .status-healthy { color: #4CAF50; }
            .status-warning { color: #FF9800; }
            .status-error { color: #F44336; }
        </style>
    </head>
    <body>
        <div class="header">
            <h1>🔍 Validation & Monitoring Framework</h1>
            <p>Real-time validation status and metrics</p>
        </div>
        
        <div class="metrics">
            <div class="metric-card">
                <h3>Framework Status</h3>
                <div class="metric-value status-healthy">ACTIVE</div>
                <p>Last updated: <span id="timestamp"></span></p>
            </div>
            
            <div class="metric-card">
                <h3>Validation Results</h3>
                <div class="metric-value">-</div>
                <p>Recent validation checks</p>
            </div>
            
            <div class="metric-card">
                <h3>Cluster Health</h3>
                <div class="metric-value status-healthy">HEALTHY</div>
                <p>Kubernetes cluster status</p>
            </div>
            
            <div class="metric-card">
                <h3>GitOps Status</h3>
                <div class="metric-value status-healthy">SYNCED</div>
                <p>GitOps synchronization status</p>
            </div>
        </div>
        
        <div>
            <h2>Quick Links</h2>
            <ul>
                <li><a href="/health">Health Check</a></li>
                <li><a href="/metrics">Prometheus Metrics</a></li>
                <li><a href="/validation-results">Validation Results</a></li>
                <li><a href="/status">Framework Status</a></li>
            </ul>
        </div>
        
        <script>
            document.getElementById('timestamp').textContent = new Date().toLocaleString();
        </script>
    </body>
    </html>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: monitoring-dashboard
  namespace: validation-monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: monitoring-dashboard
  template:
    metadata:
      labels:
        app: monitoring-dashboard
    spec:
      containers:
      - name: dashboard
        image: nginx:alpine
        ports:
        - containerPort: 80
        volumeMounts:
        - name: dashboard-content
          mountPath: /usr/share/nginx/html
      volumes:
      - name: dashboard-content
        configMap:
          name: monitoring-dashboard
---
apiVersion: v1
kind: Service
metadata:
  name: monitoring-dashboard-service
  namespace: validation-monitoring
spec:
  selector:
    app: monitoring-dashboard
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
EOF

    log_success "Monitoring dashboard deployed"
}

# Start the validation framework
start_framework() {
    log_header "🚀 Starting Validation and Monitoring Framework"
    
    # Check if already running
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        log_warning "Framework is already running (PID: $(cat "$PID_FILE"))"
        return 0
    fi
    
    # Start the framework in background
    log_info "Starting validation framework on port $METRICS_PORT..."
    
    # Set environment variables
    export METRICS_PORT="$METRICS_PORT"
    export CONFIG_FILE="$CONFIG_FILE"
    
    # Start the framework
    nohup ./validation-framework "$CONFIG_FILE" > "$LOG_FILE" 2>&1 &
    local framework_pid=$!
    
    echo "$framework_pid" > "$PID_FILE"
    
    # Wait a moment and check if it's still running
    sleep 3
    if kill -0 "$framework_pid" 2>/dev/null; then
        log_success "Validation framework started successfully (PID: $framework_pid)"
        log_info "📊 Metrics available at: http://localhost:$METRICS_PORT/metrics"
        log_info "🏥 Health check at: http://localhost:$METRICS_PORT/health"
        log_info "📈 Status at: http://localhost:$METRICS_PORT/status"
    else
        log_error "Failed to start validation framework"
        rm -f "$PID_FILE"
        exit 1
    fi
}

# Stop the validation framework
stop_framework() {
    log_info "🛑 Stopping validation framework..."
    
    if [[ -f "$PID_FILE" ]]; then
        local pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
            
            # Wait for graceful shutdown
            local count=0
            while kill -0 "$pid" 2>/dev/null && [ $count -lt 30 ]; do
                sleep 1
                count=$((count + 1))
            done
            
            # Force kill if still running
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid"
                log_warning "Framework force-killed"
            else
                log_success "Framework stopped gracefully"
            fi
        else
            log_warning "Framework was not running"
        fi
        rm -f "$PID_FILE"
    else
        log_warning "PID file not found, framework may not be running"
    fi
}

# Check framework status
check_status() {
    log_info "📊 Checking validation framework status..."
    
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        local pid=$(cat "$PID_FILE")
        log_success "Framework is running (PID: $pid)"
        
        # Check if metrics endpoint is responding
        if curl -s "http://localhost:$METRICS_PORT/health" > /dev/null 2>&1; then
            log_success "Metrics endpoint is responding"
        else
            log_warning "Metrics endpoint is not responding"
        fi
        
        # Show recent validation results
        if curl -s "http://localhost:$METRICS_PORT/validation-results?since=1h" | jq -r '.count // "unknown"' 2>/dev/null; then
            local count=$(curl -s "http://localhost:$METRICS_PORT/validation-results?since=1h" | jq -r '.count // 0' 2>/dev/null)
            log_info "Recent validations (last hour): $count"
        fi
        
    else
        log_warning "Framework is not running"
        return 1
    fi
}

# Show framework logs
show_logs() {
    log_info "📋 Showing validation framework logs..."
    
    if [[ -f "$LOG_FILE" ]]; then
        echo "=== Last 50 lines of $LOG_FILE ==="
        tail -n 50 "$LOG_FILE"
    else
        log_warning "Log file not found: $LOG_FILE"
    fi
}

# Run health check
run_health_check() {
    log_info "🏥 Running health check..."
    
    # Check if framework is running
    if ! check_status > /dev/null 2>&1; then
        log_error "Framework is not running"
        return 1
    fi
    
    # Check health endpoint
    if response=$(curl -s "http://localhost:$METRICS_PORT/health"); then
        echo "$response" | jq . 2>/dev/null || echo "$response"
        
        # Check if healthy
        if echo "$response" | jq -e '.status == "healthy"' > /dev/null 2>&1; then
            log_success "Framework health check passed"
            return 0
        else
            log_warning "Framework health check shows issues"
            return 1
        fi
    else
        log_error "Health endpoint is not responding"
        return 1
    fi
}

# Run validation tests
run_validation_tests() {
    log_info "🔍 Running validation tests..."
    
    # Check if framework is running
    if ! check_status > /dev/null 2>&1; then
        log_error "Framework is not running, starting it first..."
        start_framework
        sleep 10
    fi
    
    # Get validation results
    if response=$(curl -s "http://localhost:$METRICS_PORT/validation-results?since=5m"); then
        echo "=== Recent Validation Results ==="
        echo "$response" | jq -r '.results[] | "\(.timestamp) | \(.name) | \(.status) | \(.message)"' 2>/dev/null || echo "$response"
        
        # Count failures
        local failures=$(echo "$response" | jq -r '.results[] | select(.status == "failed") | .name' 2>/dev/null | wc -l)
        if [[ $failures -gt 0 ]]; then
            log_warning "Found $failures validation failures"
            return 1
        else
            log_success "All validations passed"
            return 0
        fi
    else
        log_error "Failed to get validation results"
        return 1
    fi
}

# Generate comprehensive report
generate_report() {
    local report_file="validation-framework-report-$(date +%Y%m%d-%H%M%S).json"
    log_info "📊 Generating comprehensive report: $report_file"
    
    # Check if framework is running
    if ! check_status > /dev/null 2>&1; then
        log_error "Framework is not running"
        return 1
    fi
    
    # Collect all data
    local health_data=$(curl -s "http://localhost:$METRICS_PORT/health" || echo '{}')
    local status_data=$(curl -s "http://localhost:$METRICS_PORT/status" || echo '{}')
    local validation_data=$(curl -s "http://localhost:$METRICS_PORT/validation-results" || echo '{}')
    local metrics_data=$(curl -s "http://localhost:$METRICS_PORT/metrics" || echo '')
    
    # Create comprehensive report
    cat > "$report_file" <<EOF
{
  "report_metadata": {
    "timestamp": "$(date -Iseconds)",
    "framework_version": "1.0.0",
    "cluster_name": "$(kubectl config current-context)",
    "report_type": "comprehensive"
  },
  "health_status": $health_data,
  "framework_status": $status_data,
  "validation_results": $validation_data,
  "metrics_summary": {
    "prometheus_metrics": "$(echo "$metrics_data" | wc -l) lines"
  }
}
EOF
    
    log_success "Report generated: $report_file"
    
    # Generate human-readable summary
    local summary_file="validation-summary-$(date +%Y%m%d-%H%M%S).md"
    cat > "$summary_file" <<EOF
# Validation Framework Report

**Generated**: $(date)
**Cluster**: $(kubectl config current-context)

## Health Status
$(echo "$health_data" | jq -r '.status // "unknown"')

## Recent Validations
$(echo "$validation_data" | jq -r '.results[] | "- \(.name): \(.status)"' 2>/dev/null | head -10)

## Metrics Endpoint
- Health: http://localhost:$METRICS_PORT/health
- Metrics: http://localhost:$METRICS_PORT/metrics  
- Status: http://localhost:$METRICS_PORT/status

## Log File
$LOG_FILE
EOF
    
    log_success "Summary generated: $summary_file"
}

# Cleanup function
cleanup() {
    log_info "🧹 Cleaning up validation framework..."
    
    # Stop framework if running
    stop_framework
    
    # Clean up temporary files
    rm -f validation-framework
    rm -f "$PID_FILE"
    
    # Optionally clean up monitoring resources
    if [[ "${CLEANUP_MONITORING:-false}" == "true" ]]; then
        kubectl delete namespace validation-monitoring --ignore-not-found=true --wait=false
    fi
    
    log_success "Cleanup completed"
}

# Display usage information
usage() {
    cat <<EOF
🔍 Validation and Monitoring Framework

USAGE:
    $0 [COMMAND] [OPTIONS]

COMMANDS:
    start           Start the validation framework
    stop            Stop the validation framework  
    restart         Restart the validation framework
    status          Check framework status
    logs            Show framework logs
    health          Run health check
    test            Run validation tests
    report          Generate comprehensive report
    build           Build the framework binary
    deps            Install Go dependencies
    cleanup         Clean up framework resources

OPTIONS:
    -c, --config    Configuration file path (default: validation-config.yaml)
    -p, --port      Metrics port (default: 8080)
    -l, --log       Log file path (default: validation-framework-{timestamp}.log)

EXAMPLES:
    $0 start                    # Start with default settings
    $0 start -p 9090           # Start on port 9090
    $0 test                    # Run validation tests
    $0 report                  # Generate comprehensive report
    $0 cleanup                 # Clean up all resources

ENVIRONMENT VARIABLES:
    CONFIG_FILE                 Configuration file path
    METRICS_PORT               Port for metrics endpoint
    LOG_FILE                   Log file path
    CLEANUP_MONITORING         Clean up monitoring resources on cleanup

More info: https://github.com/your-repo/gitops-demo-app
EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            -p|--port)
                METRICS_PORT="$2"
                shift 2
                ;;
            -l|--log)
                LOG_FILE="$2"
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                break
                ;;
        esac
    done
}

# Main function
main() {
    local command=${1:-help}
    shift || true
    
    # Parse additional arguments
    parse_args "$@"
    
    # Setup trap for cleanup on exit
    trap cleanup EXIT
    
    case "$command" in
        start)
            check_prerequisites
            install_dependencies
            build_framework
            setup_monitoring_dashboard
            start_framework
            ;;
        stop)
            stop_framework
            ;;
        restart)
            stop_framework
            sleep 2
            start_framework
            ;;
        status)
            check_status
            ;;
        logs)
            show_logs
            ;;
        health)
            run_health_check
            ;;
        test)
            run_validation_tests
            ;;
        report)
            generate_report
            ;;
        build)
            install_dependencies
            build_framework
            ;;
        deps)
            install_dependencies
            ;;
        cleanup)
            cleanup
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: $command"
            usage
            exit 1
            ;;
    esac
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi