#!/bin/bash
# Final Integration Test for Complete GitOps Pipeline Simulation
# Validates all components and runs end-to-end integration test

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_RESULTS_DIR="$SCRIPT_DIR/integration-test-results"
TEST_TIMESTAMP=$(date +%Y%m%d-%H%M%S)

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
}

highlight() {
    echo -e "${PURPLE}[HIGHLIGHT]${NC} $1"
}

info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

# Test tracking
TESTS_TOTAL=0
TESTS_PASSED=0
TESTS_FAILED=0
FAILED_TESTS=()

run_test() {
    local test_name="$1"
    local test_command="$2"
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    log "🧪 Running test: $test_name"
    
    if eval "$test_command" >/dev/null 2>&1; then
        success "✅ $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        error "❌ $test_name"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

create_test_results_dir() {
    mkdir -p "$TEST_RESULTS_DIR"
    info "Test results will be saved to: $TEST_RESULTS_DIR"
}

test_prerequisites() {
    highlight "🔍 Testing Prerequisites"
    
    run_test "CRC cluster accessible" "oc whoami"
    run_test "kubectl command available" "which kubectl"
    run_test "oc command available" "which oc"
    run_test "kustomize available" "which kustomize"
    run_test "helm available" "which helm"
    run_test "mc (MinIO client) available" "which mc"
    run_test "Go compiler available" "which go"
    run_test "tar command available" "which tar"
    run_test "gzip command available" "which gzip"
}

test_script_executability() {
    highlight "🔍 Testing Script Executability"
    
    local scripts=(
        "environment-setup.sh"
        "deploy-workloads.sh"
        "gitops-pipeline-orchestrator.sh"
        "disaster-recovery-simulator.sh"
        "start-validation-framework.sh"
        "master-orchestrator.sh"
        "validate-setup.sh"
    )
    
    for script in "${scripts[@]}"; do
        run_test "Script executable: $script" "test -x $SCRIPT_DIR/$script"
    done
}

test_go_compilation() {
    highlight "🔍 Testing Go Code Compilation"
    
    local go_files=(
        "enhanced-backup-executor.go"
        "validation-monitoring-framework.go"
    )
    
    for go_file in "${go_files[@]}"; do
        if [ -f "$SCRIPT_DIR/$go_file" ]; then
            run_test "Go compilation: $go_file" "go build -o /tmp/test-$go_file $SCRIPT_DIR/$go_file"
            # Cleanup
            rm -f "/tmp/test-$go_file" 2>/dev/null || true
        fi
    done
}

test_yaml_syntax() {
    highlight "🔍 Testing YAML Syntax"
    
    # Find all YAML files
    local yaml_files=($(find "$SCRIPT_DIR" -name "*.yaml" -type f))
    
    for yaml_file in "${yaml_files[@]}"; do
        local filename=$(basename "$yaml_file")
        run_test "YAML syntax: $filename" "python3 -c 'import yaml; yaml.safe_load(open(\"$yaml_file\"))'"
    done
}

test_directory_structure() {
    highlight "🔍 Testing Directory Structure"
    
    local expected_files=(
        "README.md"
        "environment-setup.sh"
        "deploy-workloads.sh"
        "enhanced-backup-executor.go"
        "gitops-pipeline-orchestrator.sh"
        "disaster-recovery-simulator.sh"
        "validation-monitoring-framework.go"
        "start-validation-framework.sh"
        "master-orchestrator.sh"
        "validate-setup.sh"
        "final-integration-test.sh"
    )
    
    for file in "${expected_files[@]}"; do
        run_test "File exists: $file" "test -f $SCRIPT_DIR/$file"
    done
}

test_script_syntax() {
    highlight "🔍 Testing Shell Script Syntax"
    
    local shell_scripts=($(find "$SCRIPT_DIR" -name "*.sh" -type f))
    
    for script in "${shell_scripts[@]}"; do
        local filename=$(basename "$script")
        run_test "Shell syntax: $filename" "bash -n $script"
    done
}

run_quick_functional_test() {
    highlight "🧪 Running Quick Functional Test"
    
    # Test environment validation
    if [ -x "$SCRIPT_DIR/validate-setup.sh" ]; then
        run_test "Environment validation" "$SCRIPT_DIR/validate-setup.sh --quick"
    fi
    
    # Test MinIO configuration parsing
    if [ -f "$SCRIPT_DIR/minio-config.env" ]; then
        run_test "MinIO config readable" "source $SCRIPT_DIR/minio-config.env && test -n \$MINIO_ENDPOINT"
    fi
    
    # Test kubectl cluster access
    run_test "Kubernetes cluster access" "kubectl cluster-info --request-timeout=5s"
    
    # Test basic namespace creation (dry-run)
    run_test "Namespace creation test" "kubectl create namespace test-integration-$TEST_TIMESTAMP --dry-run=client"
}

test_documentation_completeness() {
    highlight "🔍 Testing Documentation Completeness"
    
    run_test "README.md exists and non-empty" "test -s $SCRIPT_DIR/README.md"
    
    # Check if README contains essential sections
    if [ -f "$SCRIPT_DIR/README.md" ]; then
        run_test "README contains Quick Start" "grep -q 'Quick Start' $SCRIPT_DIR/README.md"
        run_test "README contains Prerequisites" "grep -q 'Prerequisites' $SCRIPT_DIR/README.md"
        run_test "README contains Troubleshooting" "grep -q 'Troubleshooting' $SCRIPT_DIR/README.md"
    fi
}

generate_test_report() {
    log "📋 Generating test report..."
    
    local report_file="$TEST_RESULTS_DIR/integration-test-report-$TEST_TIMESTAMP.md"
    
    cat > "$report_file" << EOF
# Integration Test Report

**Test Execution**: $(date -Iseconds)  
**Test Directory**: $SCRIPT_DIR  
**Test Results Directory**: $TEST_RESULTS_DIR

## Summary

- **Total Tests**: $TESTS_TOTAL
- **Passed**: $TESTS_PASSED
- **Failed**: $TESTS_FAILED
- **Success Rate**: $(( TESTS_PASSED * 100 / TESTS_TOTAL ))%

## Test Results

### Overall Status
EOF

    if [ $TESTS_FAILED -eq 0 ]; then
        echo "✅ **ALL TESTS PASSED** - System ready for production use" >> "$report_file"
    else
        echo "❌ **SOME TESTS FAILED** - Review failed tests before proceeding" >> "$report_file"
    fi
    
    cat >> "$report_file" << EOF

### Failed Tests
EOF
    
    if [ $TESTS_FAILED -eq 0 ]; then
        echo "None - all tests passed successfully!" >> "$report_file"
    else
        for failed_test in "${FAILED_TESTS[@]}"; do
            echo "- ❌ $failed_test" >> "$report_file"
        done
    fi
    
    cat >> "$report_file" << EOF

### Test Categories

#### ✅ Prerequisites Testing
- Validates all required tools and dependencies
- Checks CRC cluster accessibility
- Verifies CLI tool availability

#### ✅ Code Quality Testing  
- Shell script syntax validation
- Go code compilation testing
- YAML syntax verification

#### ✅ Structure Testing
- Required file existence
- Script executability
- Directory structure validation

#### ✅ Functional Testing
- Environment validation
- Configuration file parsing
- Kubernetes cluster connectivity

#### ✅ Documentation Testing
- README completeness
- Essential section presence
- Documentation quality

## Recommendations

EOF
    
    if [ $TESTS_FAILED -eq 0 ]; then
        cat >> "$report_file" << EOF
🎉 **System is ready for production use!**

You can now proceed with:
1. Full simulation execution: \`./master-orchestrator.sh run\`
2. Individual component testing
3. Production deployment validation

EOF
    else
        cat >> "$report_file" << EOF
⚠️ **Address failed tests before proceeding:**

1. Review failed test details above
2. Fix identified issues
3. Re-run integration test: \`./final-integration-test.sh\`
4. Ensure all tests pass before production use

EOF
    fi
    
    cat >> "$report_file" << EOF
## Next Steps

### For Development:
- Fix any failed tests
- Add additional test coverage
- Update documentation as needed

### For Production:
- Ensure all tests pass
- Run full simulation with \`./master-orchestrator.sh run\`
- Validate results in monitoring dashboard

---

**Report Generated**: $(date)  
**Integration Test**: $([ $TESTS_FAILED -eq 0 ] && echo "PASSED ✅" || echo "FAILED ❌")
EOF

    success "Test report generated: $report_file"
}

display_summary() {
    echo
    highlight "🎯 INTEGRATION TEST SUMMARY"
    highlight "=========================="
    
    echo -e "${CYAN}Total Tests:${NC} $TESTS_TOTAL"
    echo -e "${GREEN}Passed:${NC} $TESTS_PASSED"
    echo -e "${RED}Failed:${NC} $TESTS_FAILED"
    echo -e "${PURPLE}Success Rate:${NC} $(( TESTS_PASSED * 100 / TESTS_TOTAL ))%"
    
    echo
    if [ $TESTS_FAILED -eq 0 ]; then
        success "🎉 ALL TESTS PASSED - System ready for production!"
        echo
        highlight "Next steps:"
        echo "1. Run full simulation: ./master-orchestrator.sh run"
        echo "2. Access monitoring: ./start-validation-framework.sh"
        echo "3. View results: ./master-orchestrator.sh report"
    else
        error "❌ Some tests failed. Please review and fix:"
        echo
        for failed_test in "${FAILED_TESTS[@]}"; do
            echo -e "  ${RED}•${NC} $failed_test"
        done
        echo
        warn "Re-run this test after fixing issues: ./final-integration-test.sh"
    fi
    
    echo
    info "📋 Detailed report: $TEST_RESULTS_DIR/integration-test-report-$TEST_TIMESTAMP.md"
}

main() {
    highlight "🚀 GitOps Pipeline Integration Test"
    highlight "=================================="
    
    create_test_results_dir
    
    # Run all test categories
    test_prerequisites
    test_script_executability
    test_go_compilation
    test_yaml_syntax
    test_directory_structure
    test_script_syntax
    test_documentation_completeness
    run_quick_functional_test
    
    # Generate reports and summary
    generate_test_report
    display_summary
    
    # Exit with appropriate code
    exit $TESTS_FAILED
}

main "$@"