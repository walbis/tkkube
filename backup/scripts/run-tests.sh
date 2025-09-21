#!/bin/bash

# Test runner script for the backup tool
# This script provides a convenient way to run all tests with proper setup

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
MINIO_CONTAINER_NAME="test-minio-$$"
MINIO_PORT="9000"
COVERAGE_THRESHOLD=80

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    if docker ps -q --filter "name=$MINIO_CONTAINER_NAME" | grep -q .; then
        docker stop "$MINIO_CONTAINER_NAME" >/dev/null 2>&1 || true
        docker rm "$MINIO_CONTAINER_NAME" >/dev/null 2>&1 || true
    fi
}

# Set up cleanup trap
trap cleanup EXIT

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.21 or later."
        exit 1
    fi
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker for integration tests."
        exit 1
    fi
    
    # Check Go version
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    required_version="1.21"
    if ! printf '%s\n' "$required_version" "$go_version" | sort -V -C; then
        log_error "Go version $go_version is too old. Please upgrade to $required_version or later."
        exit 1
    fi
    
    log_success "Dependencies check passed"
}

# Download Go modules
download_modules() {
    log_info "Downloading Go modules..."
    cd "$PROJECT_ROOT"
    go mod download
    go mod verify
    log_success "Go modules downloaded and verified"
}

# Run unit tests
run_unit_tests() {
    log_info "Running unit tests..."
    cd "$PROJECT_ROOT"
    
    local coverage_file="coverage.out"
    local packages=(
        "./internal/config"
        "./internal/logging"
        "./internal/backup"
        "./internal/metrics"
    )
    
    # Run tests with coverage
    if go test -v -race -coverprofile="$coverage_file" -covermode=atomic "${packages[@]}"; then
        log_success "Unit tests passed"
        
        # Generate coverage report
        local coverage_percent
        coverage_percent=$(go tool cover -func="$coverage_file" | grep total | awk '{print $3}' | sed 's/%//')
        
        log_info "Test coverage: ${coverage_percent}%"
        
        if (( $(echo "$coverage_percent >= $COVERAGE_THRESHOLD" | bc -l) )); then
            log_success "Coverage threshold met (${coverage_percent}% >= ${COVERAGE_THRESHOLD}%)"
        else
            log_warning "Coverage below threshold (${coverage_percent}% < ${COVERAGE_THRESHOLD}%)"
        fi
        
        # Generate HTML coverage report
        go tool cover -html="$coverage_file" -o coverage.html
        log_info "Coverage report generated: coverage.html"
        
        return 0
    else
        log_error "Unit tests failed"
        return 1
    fi
}

# Start MinIO container for integration tests
start_minio() {
    log_info "Starting MinIO container for integration tests..."
    
    # Check if container is already running
    if docker ps -q --filter "name=$MINIO_CONTAINER_NAME" | grep -q .; then
        log_info "MinIO container already running"
        return 0
    fi
    
    # Start MinIO container
    docker run -d \
        --name "$MINIO_CONTAINER_NAME" \
        -p "$MINIO_PORT:9000" \
        -e MINIO_ROOT_USER=minioadmin \
        -e MINIO_ROOT_PASSWORD=minioadmin123 \
        minio/minio:latest server /data \
        > /dev/null
    
    # Wait for MinIO to be ready
    log_info "Waiting for MinIO to be ready..."
    local max_attempts=30
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        if curl -f "http://localhost:$MINIO_PORT/minio/health/live" >/dev/null 2>&1; then
            log_success "MinIO is ready"
            return 0
        fi
        
        attempt=$((attempt + 1))
        sleep 2
    done
    
    log_error "MinIO failed to start within timeout"
    return 1
}

# Run integration tests
run_integration_tests() {
    log_info "Running integration tests..."
    cd "$PROJECT_ROOT"
    
    export MINIO_ENDPOINT="localhost:$MINIO_PORT"
    export MINIO_ACCESS_KEY="minioadmin"
    export MINIO_SECRET_KEY="minioadmin123"
    
    if go test -v -timeout 10m ./tests/integration/...; then
        log_success "Integration tests passed"
        return 0
    else
        log_error "Integration tests failed"
        return 1
    fi
}

# Run linter
run_linter() {
    log_info "Running linter..."
    cd "$PROJECT_ROOT"
    
    if command -v golangci-lint &> /dev/null; then
        if golangci-lint run --timeout=5m; then
            log_success "Linter checks passed"
            return 0
        else
            log_error "Linter checks failed"
            return 1
        fi
    else
        log_warning "golangci-lint not found, skipping linter checks"
        log_info "Install golangci-lint: https://golangci-lint.run/usage/install/"
        return 0
    fi
}

# Run security scanner
run_security_scan() {
    log_info "Running security scanner..."
    cd "$PROJECT_ROOT"
    
    if command -v gosec &> /dev/null; then
        if gosec -fmt json -out security-report.json -stdout ./...; then
            log_success "Security scan passed"
            return 0
        else
            log_error "Security scan failed"
            return 1
        fi
    else
        log_warning "gosec not found, skipping security scan"
        log_info "Install gosec: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"
        return 0
    fi
}

# Build binary
build_binary() {
    log_info "Building binary..."
    cd "$PROJECT_ROOT"
    
    mkdir -p build
    if CGO_ENABLED=0 go build -a -installsuffix cgo -o build/backup ./cmd/backup; then
        log_success "Binary built successfully: build/backup"
        return 0
    else
        log_error "Binary build failed"
        return 1
    fi
}

# Run benchmark tests
run_benchmarks() {
    log_info "Running benchmark tests..."
    cd "$PROJECT_ROOT"
    
    local benchmark_packages=(
        "./internal/config"
        "./internal/logging"
        "./internal/backup"
    )
    
    if go test -bench=. -benchmem -run=^$$ "${benchmark_packages[@]}"; then
        log_success "Benchmark tests completed"
        return 0
    else
        log_error "Benchmark tests failed"
        return 1
    fi
}

# Print test summary
print_summary() {
    local unit_result=$1
    local integration_result=$2
    local lint_result=$3
    local security_result=$4
    local build_result=$5
    
    echo
    log_info "=== TEST SUMMARY ==="
    
    [ $unit_result -eq 0 ] && echo -e "  Unit Tests:        ${GREEN}PASS${NC}" || echo -e "  Unit Tests:        ${RED}FAIL${NC}"
    [ $integration_result -eq 0 ] && echo -e "  Integration Tests: ${GREEN}PASS${NC}" || echo -e "  Integration Tests: ${RED}FAIL${NC}"
    [ $lint_result -eq 0 ] && echo -e "  Linter:           ${GREEN}PASS${NC}" || echo -e "  Linter:           ${RED}FAIL${NC}"
    [ $security_result -eq 0 ] && echo -e "  Security Scan:    ${GREEN}PASS${NC}" || echo -e "  Security Scan:    ${RED}FAIL${NC}"
    [ $build_result -eq 0 ] && echo -e "  Build:            ${GREEN}PASS${NC}" || echo -e "  Build:            ${RED}FAIL${NC}"
    
    echo
    
    local total_failures=$((unit_result + integration_result + lint_result + security_result + build_result))
    if [ $total_failures -eq 0 ]; then
        log_success "All tests passed! 🎉"
        return 0
    else
        log_error "Some tests failed. Check the output above for details."
        return 1
    fi
}

# Main function
main() {
    local run_unit=true
    local run_integration=true
    local run_lint=true
    local run_security=true
    local run_build=true
    local run_benchmarks=false
    local fast_mode=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --unit-only)
                run_integration=false
                run_lint=false
                run_security=false
                run_build=false
                shift
                ;;
            --integration-only)
                run_unit=false
                run_lint=false
                run_security=false
                run_build=false
                shift
                ;;
            --fast)
                fast_mode=true
                run_integration=false
                run_security=false
                shift
                ;;
            --benchmarks)
                run_benchmarks=true
                shift
                ;;
            --help)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --unit-only       Run only unit tests"
                echo "  --integration-only Run only integration tests"
                echo "  --fast            Run unit tests, linter, and build only"
                echo "  --benchmarks      Include benchmark tests"
                echo "  --help            Show this help message"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    log_info "Starting test suite..."
    
    # Check dependencies
    check_dependencies
    
    # Download modules
    download_modules
    
    # Initialize result variables
    local unit_result=0
    local integration_result=0
    local lint_result=0
    local security_result=0
    local build_result=0
    
    # Run tests
    if [ "$run_unit" = true ]; then
        run_unit_tests || unit_result=1
    fi
    
    if [ "$run_integration" = true ]; then
        start_minio || integration_result=1
        if [ $integration_result -eq 0 ]; then
            run_integration_tests || integration_result=1
        fi
    fi
    
    if [ "$run_lint" = true ]; then
        run_linter || lint_result=1
    fi
    
    if [ "$run_security" = true ]; then
        run_security_scan || security_result=1
    fi
    
    if [ "$run_build" = true ]; then
        build_binary || build_result=1
    fi
    
    if [ "$run_benchmarks" = true ]; then
        run_benchmarks
    fi
    
    # Print summary
    print_summary $unit_result $integration_result $lint_result $security_result $build_result
}

# Run main function with all arguments
main "$@"