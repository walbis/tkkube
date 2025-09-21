#!/bin/bash

# Integration Test Runner Script
# This script orchestrates the complete integration test suite execution

set -euo pipefail

# Script configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../" && pwd)"
readonly TEST_DIR="${PROJECT_ROOT}/tests"
readonly DOCKER_DIR="${TEST_DIR}/docker"
readonly REPORTS_DIR="${PROJECT_ROOT}/test-reports"
readonly ARTIFACTS_DIR="${PROJECT_ROOT}/test-artifacts"

# Default configuration
TEST_SUITE="all"
ENVIRONMENT="docker"
CLEANUP_AFTER="true"
VERBOSE="false"
PARALLEL="true"
COVERAGE="true"
PERFORMANCE_TESTS="false"
TIMEOUT="30m"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Function definitions
usage() {
    cat << EOF
Integration Test Runner

Usage: $0 [OPTIONS]

OPTIONS:
    -s, --suite SUITE       Test suite to run (all, api, workflows, performance) [default: all]
    -e, --environment ENV   Test environment (docker, local, ci) [default: docker]
    -c, --no-cleanup        Don't cleanup after tests
    -v, --verbose           Enable verbose output
    -p, --performance       Include performance tests
    -t, --timeout DURATION Test timeout [default: 30m]
    --no-parallel          Disable parallel test execution
    --no-coverage           Disable coverage collection
    -h, --help              Show this help message

EXAMPLES:
    $0                                          # Run all tests in Docker
    $0 -s api -e local                         # Run API tests locally
    $0 -s performance -p -t 60m               # Run performance tests with 60m timeout
    $0 -v --no-cleanup                        # Verbose output, keep containers running

EOF
}

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] ✓${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] ⚠${NC} $*"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ✗${NC} $*"
}

cleanup() {
    if [[ "${CLEANUP_AFTER}" == "true" ]]; then
        log "Cleaning up test environment..."
        if [[ "${ENVIRONMENT}" == "docker" ]]; then
            docker-compose -f "${DOCKER_DIR}/docker-compose.test.yml" down --volumes --remove-orphans || true
        fi
        log_success "Cleanup completed"
    else
        log_warning "Skipping cleanup (--no-cleanup specified)"
    fi
}

setup_directories() {
    log "Setting up test directories..."
    mkdir -p "${REPORTS_DIR}" "${ARTIFACTS_DIR}"
    chmod 755 "${REPORTS_DIR}" "${ARTIFACTS_DIR}"
    log_success "Directories created: ${REPORTS_DIR}, ${ARTIFACTS_DIR}"
}

check_dependencies() {
    log "Checking dependencies..."
    
    local dependencies=()
    
    if [[ "${ENVIRONMENT}" == "docker" ]]; then
        dependencies+=("docker" "docker-compose")
    elif [[ "${ENVIRONMENT}" == "local" ]]; then
        dependencies+=("go" "kubectl")
    fi
    
    for dep in "${dependencies[@]}"; do
        if ! command -v "${dep}" &> /dev/null; then
            log_error "Required dependency '${dep}' is not installed"
            exit 1
        fi
    done
    
    log_success "All dependencies are available"
}

setup_docker_environment() {
    log "Setting up Docker test environment..."
    
    cd "${DOCKER_DIR}"
    
    # Check if containers are already running
    if docker-compose -f docker-compose.test.yml ps | grep -q "Up"; then
        log_warning "Test containers are already running. Stopping them first..."
        docker-compose -f docker-compose.test.yml down --volumes
    fi
    
    # Start test environment
    log "Starting test containers..."
    docker-compose -f docker-compose.test.yml up -d --build
    
    # Wait for services to be ready
    log "Waiting for services to be ready..."
    local max_wait=120
    local wait_time=0
    
    while [[ ${wait_time} -lt ${max_wait} ]]; do
        if docker-compose -f docker-compose.test.yml ps | grep -q "unhealthy"; then
            log_warning "Some services are still starting... (${wait_time}s/${max_wait}s)"
            sleep 5
            wait_time=$((wait_time + 5))
        else
            break
        fi
    done
    
    if [[ ${wait_time} -ge ${max_wait} ]]; then
        log_error "Services failed to start within ${max_wait} seconds"
        docker-compose -f docker-compose.test.yml logs
        exit 1
    fi
    
    log_success "Docker environment is ready"
}

setup_local_environment() {
    log "Setting up local test environment..."
    
    # Check if required services are running
    local services=("minio" "redis" "postgres")
    
    for service in "${services[@]}"; do
        if ! pgrep -f "${service}" > /dev/null; then
            log_warning "Service '${service}' is not running locally"
        fi
    done
    
    # Set local environment variables
    export MINIO_ENDPOINT="localhost:9000"
    export MINIO_ACCESS_KEY="testuser"
    export MINIO_SECRET_KEY="testpassword123"
    export REDIS_ENDPOINT="localhost:6379"
    export POSTGRES_ENDPOINT="localhost:5432"
    export TEST_ENV="local"
    
    log_success "Local environment configured"
}

build_test_binaries() {
    if [[ "${ENVIRONMENT}" == "local" ]]; then
        log "Building test binaries..."
        cd "${PROJECT_ROOT}"
        
        go build -race -o "${ARTIFACTS_DIR}/test-runner" ./tests/cmd/runner
        
        log_success "Test binaries built"
    fi
}

run_test_suite() {
    local suite="$1"
    
    log "Running test suite: ${suite}"
    
    cd "${PROJECT_ROOT}"
    
    # Prepare test command
    local test_cmd="go test"
    local test_flags=()
    
    # Add common flags
    test_flags+=("-timeout" "${TIMEOUT}")
    
    if [[ "${VERBOSE}" == "true" ]]; then
        test_flags+=("-v")
    fi
    
    if [[ "${PARALLEL}" == "true" ]]; then
        test_flags+=("-race")
    fi
    
    if [[ "${COVERAGE}" == "true" ]]; then
        test_flags+=("-cover" "-coverprofile=${REPORTS_DIR}/coverage-${suite}.out")
    fi
    
    # Set test packages based on suite
    local test_packages=""
    case "${suite}" in
        "api")
            test_packages="./tests/integration/api/..."
            ;;
        "workflows")
            test_packages="./tests/integration/workflows/..."
            ;;
        "components")
            test_packages="./tests/integration/components/..."
            ;;
        "performance")
            test_packages="./tests/integration/performance/..."
            test_flags+=("-bench=." "-benchmem")
            ;;
        "security")
            test_packages="./tests/integration/security/..."
            ;;
        "chaos")
            test_packages="./tests/integration/chaos/..."
            ;;
        "all")
            test_packages="./tests/integration/..."
            if [[ "${PERFORMANCE_TESTS}" == "true" ]]; then
                test_flags+=("-bench=." "-benchmem")
            fi
            ;;
        *)
            log_error "Unknown test suite: ${suite}"
            exit 1
            ;;
    esac
    
    # Execute tests
    local test_output="${REPORTS_DIR}/test-${suite}-$(date +%Y%m%d-%H%M%S).log"
    
    if [[ "${ENVIRONMENT}" == "docker" ]]; then
        # Run tests in Docker container
        docker-compose -f "${DOCKER_DIR}/docker-compose.test.yml" exec -T test-runner \
            ${test_cmd} "${test_flags[@]}" ${test_packages} 2>&1 | tee "${test_output}"
    else
        # Run tests locally
        ${test_cmd} "${test_flags[@]}" ${test_packages} 2>&1 | tee "${test_output}"
    fi
    
    local exit_code=${PIPESTATUS[0]}
    
    if [[ ${exit_code} -eq 0 ]]; then
        log_success "Test suite '${suite}' completed successfully"
    else
        log_error "Test suite '${suite}' failed with exit code ${exit_code}"
        return ${exit_code}
    fi
}

generate_reports() {
    log "Generating test reports..."
    
    cd "${PROJECT_ROOT}"
    
    # Generate coverage report if coverage was enabled
    if [[ "${COVERAGE}" == "true" ]]; then
        local coverage_files=($(find "${REPORTS_DIR}" -name "coverage-*.out" 2>/dev/null))
        
        if [[ ${#coverage_files[@]} -gt 0 ]]; then
            # Merge coverage files
            echo "mode: atomic" > "${REPORTS_DIR}/coverage-merged.out"
            for file in "${coverage_files[@]}"; do
                grep -v "mode: atomic" "${file}" >> "${REPORTS_DIR}/coverage-merged.out" 2>/dev/null || true
            done
            
            # Generate HTML report
            go tool cover -html="${REPORTS_DIR}/coverage-merged.out" -o "${REPORTS_DIR}/coverage.html"
            
            # Generate summary
            local coverage_percent=$(go tool cover -func="${REPORTS_DIR}/coverage-merged.out" | grep "total:" | awk '{print $3}')
            
            log_success "Coverage report generated: ${coverage_percent} total coverage"
            log "Coverage report available at: ${REPORTS_DIR}/coverage.html"
        fi
    fi
    
    # Generate test summary
    cat > "${REPORTS_DIR}/test-summary.json" << EOF
{
    "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
    "suite": "${TEST_SUITE}",
    "environment": "${ENVIRONMENT}",
    "timeout": "${TIMEOUT}",
    "coverage_enabled": ${COVERAGE},
    "performance_tests": ${PERFORMANCE_TESTS},
    "reports_directory": "${REPORTS_DIR}",
    "artifacts_directory": "${ARTIFACTS_DIR}"
}
EOF
    
    log_success "Test reports generated in: ${REPORTS_DIR}"
}

collect_artifacts() {
    log "Collecting test artifacts..."
    
    if [[ "${ENVIRONMENT}" == "docker" ]]; then
        # Copy artifacts from Docker containers
        docker-compose -f "${DOCKER_DIR}/docker-compose.test.yml" exec -T test-runner \
            sh -c "cp -r /app/test-artifacts/* ${ARTIFACTS_DIR}/ 2>/dev/null || true"
    fi
    
    # Collect system information
    cat > "${ARTIFACTS_DIR}/system-info.txt" << EOF
Test Run Information
===================
Date: $(date)
Environment: ${ENVIRONMENT}
Suite: ${TEST_SUITE}
Timeout: ${TIMEOUT}
Coverage: ${COVERAGE}
Performance Tests: ${PERFORMANCE_TESTS}
Parallel: ${PARALLEL}

System Information
==================
OS: $(uname -a)
Go Version: $(go version)
Docker Version: $(docker --version 2>/dev/null || echo "Not available")
Docker Compose Version: $(docker-compose --version 2>/dev/null || echo "Not available")

Environment Variables
====================
$(env | grep -E "^(TEST_|MINIO_|K8S_|REDIS_|POSTGRES_)" | sort)
EOF
    
    log_success "Artifacts collected in: ${ARTIFACTS_DIR}"
}

main() {
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -s|--suite)
                TEST_SUITE="$2"
                shift 2
                ;;
            -e|--environment)
                ENVIRONMENT="$2"
                shift 2
                ;;
            -c|--no-cleanup)
                CLEANUP_AFTER="false"
                shift
                ;;
            -v|--verbose)
                VERBOSE="true"
                shift
                ;;
            -p|--performance)
                PERFORMANCE_TESTS="true"
                shift
                ;;
            -t|--timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            --no-parallel)
                PARALLEL="false"
                shift
                ;;
            --no-coverage)
                COVERAGE="false"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
    
    # Set up signal handlers
    trap cleanup EXIT
    trap 'log_error "Script interrupted"; exit 130' INT TERM
    
    log "Starting integration test suite execution"
    log "Suite: ${TEST_SUITE}, Environment: ${ENVIRONMENT}, Timeout: ${TIMEOUT}"
    
    # Execute test pipeline
    check_dependencies
    setup_directories
    
    if [[ "${ENVIRONMENT}" == "docker" ]]; then
        setup_docker_environment
    elif [[ "${ENVIRONMENT}" == "local" ]]; then
        setup_local_environment
        build_test_binaries
    fi
    
    # Run tests
    local overall_exit_code=0
    
    if [[ "${TEST_SUITE}" == "all" ]]; then
        local suites=("api" "workflows" "components")
        
        if [[ "${PERFORMANCE_TESTS}" == "true" ]]; then
            suites+=("performance")
        fi
        
        for suite in "${suites[@]}"; do
            if ! run_test_suite "${suite}"; then
                overall_exit_code=1
                log_error "Test suite '${suite}' failed"
                # Continue with other suites
            fi
        done
    else
        if ! run_test_suite "${TEST_SUITE}"; then
            overall_exit_code=1
        fi
    fi
    
    # Generate reports and collect artifacts
    generate_reports
    collect_artifacts
    
    # Final status
    if [[ ${overall_exit_code} -eq 0 ]]; then
        log_success "All integration tests completed successfully!"
        log "Reports available at: ${REPORTS_DIR}"
        log "Artifacts available at: ${ARTIFACTS_DIR}"
    else
        log_error "Some integration tests failed. Check the reports for details."
        exit ${overall_exit_code}
    fi
}

# Execute main function
main "$@"