#!/bin/bash

# Setup Validation Script
# Validates that all components are properly configured and ready for execution
# Part of the GitOps Demo Pipeline Integration

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Validation results
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0
WARNINGS=0

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    PASSED_CHECKS=$((PASSED_CHECKS + 1))
}

log_warning() {
    echo -e "${YELLOW}[⚠]${NC} $1"
    WARNINGS=$((WARNINGS + 1))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
}

# Check a condition and log result
check() {
    local description="$1"
    local command="$2"
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
    
    if eval "$command" &>/dev/null; then
        log_success "$description"
        return 0
    else
        log_error "$description"
        return 1
    fi
}

# Check with warning (non-critical)
check_warn() {
    local description="$1"
    local command="$2"
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
    
    if eval "$command" &>/dev/null; then
        log_success "$description"
        return 0
    else
        log_warning "$description"
        WARNINGS=$((WARNINGS + 1))
        return 1
    fi
}

echo "🔍 GitOps Production Simulation - Setup Validation"
echo "=================================================="
echo ""

# System Requirements
echo "📋 System Requirements:"
check "kubectl CLI available" "command -v kubectl"
check "Go compiler available" "command -v go"
check "jq JSON processor available" "command -v jq"
check "curl HTTP client available" "command -v curl"
check_warn "Docker available" "command -v docker"
check_warn "CRC available" "command -v crc"

echo ""

# File Structure
echo "📁 File Structure Validation:"
cd "$SCRIPT_DIR"

required_files=(
    "environment-setup.sh"
    "deploy-workloads.sh" 
    "enhanced-backup-executor.go"
    "gitops-pipeline-orchestrator.sh"
    "disaster-recovery-simulator.sh"
    "validation-monitoring-framework.go"
    "start-validation-framework.sh"
    "master-orchestrator.sh"
    "validation-config.yaml"
    "README.md"
)

for file in "${required_files[@]}"; do
    check "File exists: $file" "[[ -f '$file' ]]"
    if [[ "$file" == *.sh ]]; then
        check "Script executable: $file" "[[ -x '$file' ]]"
    fi
done

echo ""

# Directory Structure
echo "📂 Directory Structure:"
cd ..
required_dirs=(
    "production-simulation"
    "backup-source"
    "base"
    "overlays/development"
    "overlays/staging" 
    "overlays/production"
    "argocd"
    "flux"
)

for dir in "${required_dirs[@]}"; do
    check_warn "Directory exists: $dir" "[[ -d '$dir' ]]"
done

echo ""

# Kubernetes Connectivity
echo "☸️  Kubernetes Connectivity:"
check "Cluster connectivity" "kubectl cluster-info"
check_warn "Cluster nodes ready" "kubectl get nodes | grep -q Ready"

if kubectl cluster-info &>/dev/null; then
    local context=$(kubectl config current-context 2>/dev/null || echo "unknown")
    log_info "Current context: $context"
    
    # Check for existing namespaces that might interfere
    if kubectl get namespace minio-system &>/dev/null; then
        log_warning "MinIO namespace already exists"
    fi
    
    if kubectl get namespace argocd &>/dev/null; then
        log_info "ArgoCD namespace already exists (GitOps tool available)"
    fi
    
    if kubectl get namespace flux-system &>/dev/null; then
        log_info "Flux namespace already exists (GitOps tool available)"
    fi
fi

echo ""

# Go Dependencies Check
echo "🔧 Go Dependencies:"
cd "$SCRIPT_DIR"

# Check if go.mod exists or can create one
if [[ ! -f "go.mod" ]]; then
    log_info "Go module not initialized, will be created during setup"
else
    check "Go module exists" "[[ -f 'go.mod' ]]"
    check_warn "Go dependencies downloadable" "go mod download"
fi

# Test Go compilation
if go version &>/dev/null; then
    local go_version=$(go version | awk '{print $3}')
    log_info "Go version: $go_version"
    
    # Test basic compilation
    if echo 'package main; func main() {}' | go run -; then
        log_success "Go compilation test passed"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        log_error "Go compilation test failed"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
fi

echo ""

# Configuration Validation
echo "⚙️  Configuration Validation:"
if [[ -f "validation-config.yaml" ]]; then
    check "Validation config syntax" "python3 -c 'import yaml; yaml.safe_load(open(\"validation-config.yaml\"))'" || \
    check "Validation config syntax (alt)" "ruby -ryaml -e 'YAML.load_file(\"validation-config.yaml\")'" || \
    log_warning "Could not validate YAML syntax (python3/ruby not available)"
else
    log_error "validation-config.yaml not found"
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
fi
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))

echo ""

# Network Connectivity
echo "🌐 Network Connectivity:"
check_warn "Internet connectivity" "curl -s --connect-timeout 5 https://google.com > /dev/null"
check_warn "Docker Hub reachable" "curl -s --connect-timeout 5 https://hub.docker.com > /dev/null"
check_warn "Kubernetes registry reachable" "curl -s --connect-timeout 5 https://registry.k8s.io > /dev/null"

echo ""

# Port Availability
echo "🔌 Port Availability:"
check_ports=(8080 9000 9001)
for port in "${check_ports[@]}"; do
    if ! netstat -tuln 2>/dev/null | grep -q ":$port "; then
        log_success "Port $port available"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        log_warning "Port $port in use"
        WARNINGS=$((WARNINGS + 1))
    fi
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
done

echo ""

# Resource Availability (if possible to check)
echo "💾 System Resources:"
if command -v free &>/dev/null; then
    local mem_total=$(free -m | awk '/^Mem:/{print $2}')
    if [[ $mem_total -ge 4096 ]]; then
        log_success "Sufficient memory available: ${mem_total}MB"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        log_warning "Limited memory available: ${mem_total}MB (recommended: 4GB+)"
        WARNINGS=$((WARNINGS + 1))
    fi
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
fi

if command -v df &>/dev/null; then
    local disk_avail=$(df -m . | tail -1 | awk '{print $4}')
    if [[ $disk_avail -ge 10240 ]]; then
        log_success "Sufficient disk space: ${disk_avail}MB"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        log_warning "Limited disk space: ${disk_avail}MB (recommended: 10GB+)"
        WARNINGS=$((WARNINGS + 1))
    fi
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
fi

echo ""

# Summary
echo "📊 Validation Summary:"
echo "======================"
echo "Total Checks: $TOTAL_CHECKS"
echo "Passed: $PASSED_CHECKS"
echo "Failed: $FAILED_CHECKS"
echo "Warnings: $WARNINGS"

echo ""

if [[ $FAILED_CHECKS -eq 0 ]]; then
    if [[ $WARNINGS -eq 0 ]]; then
        echo -e "${GREEN}🎉 All validations passed! System is ready for production simulation.${NC}"
        echo ""
        echo "Next steps:"
        echo "  ./master-orchestrator.sh run    # Run complete simulation"
        echo "  ./master-orchestrator.sh help   # Show detailed usage"
        exit 0
    else
        echo -e "${YELLOW}⚠️  System is mostly ready with $WARNINGS warnings.${NC}"
        echo ""
        echo "You can proceed, but consider addressing the warnings for optimal results."
        echo ""
        echo "Next steps:"
        echo "  ./master-orchestrator.sh run    # Run complete simulation"
        exit 0
    fi
else
    echo -e "${RED}❌ $FAILED_CHECKS critical issues found. Please address before proceeding.${NC}"
    echo ""
    echo "Common solutions:"
    echo "  - Install missing tools (kubectl, go, jq, curl)"
    echo "  - Start/configure Kubernetes cluster"
    echo "  - Check network connectivity"
    echo "  - Ensure sufficient system resources"
    exit 1
fi