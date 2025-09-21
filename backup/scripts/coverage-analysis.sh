#!/bin/bash

# Comprehensive Test Coverage Analysis and Reporting System
# For Kubernetes Backup and Restore System

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
COVERAGE_DIR="$PROJECT_ROOT/coverage"
REPORTS_DIR="$COVERAGE_DIR/reports"
TRENDS_DIR="$COVERAGE_DIR/trends"

# Coverage thresholds
GLOBAL_THRESHOLD=80
CRITICAL_PATH_THRESHOLD=90
MODULE_THRESHOLDS=(
    "internal/backup:85"
    "internal/config:75"
    "internal/logging:70"
    "internal/resilience:85"
    "internal/metrics:75"
    "internal/orchestrator:85"
    "internal/cleanup:80"
    "internal/priority:75"
    "internal/cluster:75"
    "internal/server:75"
)

# Critical paths requiring high coverage
CRITICAL_PATHS=(
    "internal/backup"
    "internal/resilience"
    "internal/orchestrator"
)

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

log_critical() {
    echo -e "${PURPLE}[CRITICAL]${NC} $1"
}

log_trend() {
    echo -e "${CYAN}[TREND]${NC} $1"
}

# Setup directories
setup_directories() {
    log_info "Setting up coverage directories..."
    mkdir -p "$COVERAGE_DIR"
    mkdir -p "$REPORTS_DIR"
    mkdir -p "$TRENDS_DIR"
    log_success "Coverage directories created"
}

# Get all Go packages
get_go_packages() {
    cd "$PROJECT_ROOT"
    go list ./... | grep -v vendor | sort
}

# Generate comprehensive coverage profile
generate_coverage_profile() {
    local output_file="$1"
    log_info "Generating comprehensive coverage profile..."
    
    cd "$PROJECT_ROOT"
    
    # Get all packages
    local packages
    packages=$(get_go_packages)
    
    # Run tests with coverage for all packages
    local coverage_profiles=()
    local temp_dir
    temp_dir=$(mktemp -d)
    
    local package_count=0
    for package in $packages; do
        package_count=$((package_count + 1))
        local package_name
        package_name=$(basename "$package")
        local profile_file="$temp_dir/coverage_${package_count}_${package_name}.out"
        
        log_info "Testing package: $package"
        if go test -coverprofile="$profile_file" -covermode=atomic "$package" 2>/dev/null; then
            if [ -f "$profile_file" ] && [ -s "$profile_file" ]; then
                coverage_profiles+=("$profile_file")
            fi
        else
            log_warning "Failed to generate coverage for package: $package"
        fi
    done
    
    # Merge coverage profiles
    if [ ${#coverage_profiles[@]} -gt 0 ]; then
        echo "mode: atomic" > "$output_file"
        for profile in "${coverage_profiles[@]}"; do
            tail -n +2 "$profile" >> "$output_file" 2>/dev/null || true
        done
        log_success "Coverage profile generated: $output_file"
    else
        log_error "No coverage profiles generated"
        return 1
    fi
    
    # Cleanup
    rm -rf "$temp_dir"
}

# Analyze coverage by module
analyze_module_coverage() {
    local coverage_file="$1"
    local report_file="$2"
    
    log_info "Analyzing coverage by module..."
    
    {
        echo "# Module Coverage Analysis"
        echo "Generated: $(date)"
        echo
        echo "## Module Coverage Summary"
        echo
        echo "| Module | Coverage | Threshold | Status | Lines Covered | Total Lines |"
        echo "|--------|----------|-----------|--------|---------------|-------------|"
    } > "$report_file"
    
    local total_violations=0
    local critical_violations=0
    
    for threshold_spec in "${MODULE_THRESHOLDS[@]}"; do
        local module="${threshold_spec%:*}"
        local threshold="${threshold_spec#*:}"
        
        # Get coverage for this module
        local module_coverage
        module_coverage=$(go tool cover -func="$coverage_file" | grep "^$module" | \
                         awk '{sum+=$3; count++} END {if(count>0) printf "%.1f", sum/count; else print "0.0"}' | \
                         sed 's/%//')
        
        # Get line counts
        local lines_info
        lines_info=$(go tool cover -func="$coverage_file" | grep "^$module" | \
                    awk '{covered+=$2; total+=$3} END {printf "%d %d", covered, total}')
        local covered_lines total_lines
        read -r covered_lines total_lines <<< "$lines_info"
        
        # Determine status
        local status
        local status_color
        if (( $(echo "$module_coverage >= $threshold" | bc -l) )); then
            status="✅ PASS"
            status_color="${GREEN}PASS${NC}"
        else
            status="❌ FAIL"
            status_color="${RED}FAIL${NC}"
            total_violations=$((total_violations + 1))
            
            # Check if this is a critical path
            for critical_path in "${CRITICAL_PATHS[@]}"; do
                if [[ "$module" == "$critical_path" ]]; then
                    critical_violations=$((critical_violations + 1))
                    break
                fi
            done
        fi
        
        echo "| $module | ${module_coverage}% | ${threshold}% | $status | $covered_lines | $total_lines |" >> "$report_file"
        echo -e "  Module: ${CYAN}$module${NC} - Coverage: ${module_coverage}% - Status: $status_color"
    done
    
    {
        echo
        echo "## Coverage Violations"
        echo
        echo "- Total modules below threshold: $total_violations"
        echo "- Critical path violations: $critical_violations"
        echo
    } >> "$report_file"
    
    if [ $critical_violations -gt 0 ]; then
        log_critical "$critical_violations critical path modules below threshold!"
        return 2
    elif [ $total_violations -gt 0 ]; then
        log_warning "$total_violations modules below threshold"
        return 1
    else
        log_success "All modules meet coverage thresholds"
        return 0
    fi
}

# Generate detailed HTML coverage report
generate_html_report() {
    local coverage_file="$1"
    local html_file="$2"
    
    log_info "Generating HTML coverage report..."
    go tool cover -html="$coverage_file" -o "$html_file"
    log_success "HTML report generated: $html_file"
}

# Generate JSON coverage report for CI/CD integration
generate_json_report() {
    local coverage_file="$1"
    local json_file="$2"
    
    log_info "Generating JSON coverage report..."
    
    local timestamp
    timestamp=$(date -Iseconds)
    
    local total_coverage
    total_coverage=$(go tool cover -func="$coverage_file" | grep total | awk '{print $3}' | sed 's/%//')
    
    # Get function-level coverage
    local functions_json
    functions_json=$(go tool cover -func="$coverage_file" | grep -v total | \
                    awk '{printf "{\"file\":\"%s\",\"function\":\"%s\",\"coverage\":%s},", $1, $2, $3}' | \
                    sed 's/%//g' | sed 's/,$//')
    
    cat > "$json_file" <<EOF
{
  "timestamp": "$timestamp",
  "overall_coverage": $total_coverage,
  "global_threshold": $GLOBAL_THRESHOLD,
  "critical_threshold": $CRITICAL_PATH_THRESHOLD,
  "meets_global_threshold": $([ $(echo "$total_coverage >= $GLOBAL_THRESHOLD" | bc -l) -eq 1 ] && echo "true" || echo "false"),
  "modules": [
EOF

    local first=true
    for threshold_spec in "${MODULE_THRESHOLDS[@]}"; do
        local module="${threshold_spec%:*}"
        local threshold="${threshold_spec#*:}"
        
        local module_coverage
        module_coverage=$(go tool cover -func="$coverage_file" | grep "^$module" | \
                         awk '{sum+=$3; count++} END {if(count>0) printf "%.1f", sum/count; else print "0.0"}' | \
                         sed 's/%//')
        
        local is_critical=false
        for critical_path in "${CRITICAL_PATHS[@]}"; do
            if [[ "$module" == "$critical_path" ]]; then
                is_critical=true
                break
            fi
        done
        
        if [ "$first" = true ]; then
            first=false
        else
            echo "," >> "$json_file"
        fi
        
        cat >> "$json_file" <<EOF
    {
      "name": "$module",
      "coverage": $module_coverage,
      "threshold": $threshold,
      "is_critical": $is_critical,
      "meets_threshold": $([ $(echo "$module_coverage >= $threshold" | bc -l) -eq 1 ] && echo "true" || echo "false")
    }EOF
    done
    
    cat >> "$json_file" <<EOF

  ],
  "functions": [$functions_json]
}
EOF
    
    log_success "JSON report generated: $json_file"
}

# Track coverage trends
track_coverage_trends() {
    local current_coverage="$1"
    local trend_file="$TRENDS_DIR/coverage_trends.csv"
    
    log_info "Tracking coverage trends..."
    
    # Create header if file doesn't exist
    if [ ! -f "$trend_file" ]; then
        echo "timestamp,coverage,commit_hash" > "$trend_file"
    fi
    
    # Get current commit hash if in git repo
    local commit_hash="unknown"
    if git rev-parse --git-dir > /dev/null 2>&1; then
        commit_hash=$(git rev-parse --short HEAD)
    fi
    
    # Add current data point
    local timestamp
    timestamp=$(date -Iseconds)
    echo "$timestamp,$current_coverage,$commit_hash" >> "$trend_file"
    
    # Analyze trend (last 10 data points)
    local trend_analysis
    trend_analysis=$(tail -n 11 "$trend_file" | tail -n +2 | \
                    awk -F',' 'NR==1{first=$2} END{last=$2; if(last>first) print "IMPROVING"; else if(last<first) print "DECLINING"; else print "STABLE"}')
    
    log_trend "Coverage trend: $trend_analysis"
    
    # Generate trend report
    local trend_report="$REPORTS_DIR/coverage_trends.md"
    {
        echo "# Coverage Trends Report"
        echo "Generated: $(date)"
        echo
        echo "## Current Status"
        echo "- Current Coverage: ${current_coverage}%"
        echo "- Trend: $trend_analysis"
        echo "- Commit: $commit_hash"
        echo
        echo "## Historical Data (Last 10 Runs)"
        echo
        echo "| Timestamp | Coverage | Commit | Change |"
        echo "|-----------|----------|--------|--------|"
        
        tail -n 11 "$trend_file" | tail -n +2 | \
        awk -F',' 'NR==1{prev=$2} {if(NR>1){change=$2-prev; printf "| %s | %s%% | %s | %+.1f%% |\n", $1, $2, $3, change} prev=$2}'
    } > "$trend_report"
    
    log_success "Trend report generated: $trend_report"
}

# Identify uncovered critical paths
identify_uncovered_critical_paths() {
    local coverage_file="$1"
    local output_file="$2"
    
    log_info "Identifying uncovered critical code paths..."
    
    {
        echo "# Uncovered Critical Paths Analysis"
        echo "Generated: $(date)"
        echo
        echo "## Critical Functions with Low Coverage"
        echo
    } > "$output_file"
    
    # Find functions in critical paths with coverage below critical threshold
    for critical_path in "${CRITICAL_PATHS[@]}"; do
        echo "### Module: $critical_path" >> "$output_file"
        echo >> "$output_file"
        
        local uncovered_functions
        uncovered_functions=$(go tool cover -func="$coverage_file" | \
                             grep "^$critical_path" | \
                             awk -v threshold="$CRITICAL_PATH_THRESHOLD" \
                             '$3+0 < threshold {printf "- **%s::%s** - Coverage: %s (Target: %d%%)\n", $1, $2, $3, threshold}')
        
        if [ -n "$uncovered_functions" ]; then
            echo "$uncovered_functions" >> "$output_file"
            log_warning "Found uncovered functions in critical path: $critical_path"
        else
            echo "- ✅ All functions meet critical coverage threshold" >> "$output_file"
        fi
        echo >> "$output_file"
    done
    
    # Add recommendations
    {
        echo "## Recommendations"
        echo
        echo "### Immediate Actions"
        echo "1. **Prioritize Critical Paths**: Focus testing efforts on modules marked as critical"
        echo "2. **Add Edge Case Tests**: Identify and test boundary conditions and error scenarios"
        echo "3. **Integration Test Coverage**: Ensure integration tests cover critical interaction paths"
        echo
        echo "### Testing Strategy"
        echo "1. **Unit Tests**: Achieve ${CRITICAL_PATH_THRESHOLD}% coverage for critical modules"
        echo "2. **Integration Tests**: Cover end-to-end scenarios and error recovery"
        echo "3. **Error Path Testing**: Test failure modes and recovery mechanisms"
        echo "4. **Performance Tests**: Include performance regression testing"
        echo
        echo "### Coverage Improvement Plan"
        echo "1. **Phase 1**: Address critical path violations"
        echo "2. **Phase 2**: Improve overall module coverage"
        echo "3. **Phase 3**: Maintain trend monitoring and regression prevention"
    } >> "$output_file"
    
    log_success "Critical paths analysis generated: $output_file"
}

# Generate quality gates report
generate_quality_gates_report() {
    local coverage_file="$1"
    local output_file="$2"
    
    log_info "Generating quality gates report..."
    
    local total_coverage
    total_coverage=$(go tool cover -func="$coverage_file" | grep total | awk '{print $3}' | sed 's/%//')
    
    # Check global threshold
    local global_status
    if (( $(echo "$total_coverage >= $GLOBAL_THRESHOLD" | bc -l) )); then
        global_status="✅ PASS"
    else
        global_status="❌ FAIL"
    fi
    
    # Check critical paths
    local critical_status="✅ PASS"
    local critical_failures=0
    for critical_path in "${CRITICAL_PATHS[@]}"; do
        local path_coverage
        path_coverage=$(go tool cover -func="$coverage_file" | grep "^$critical_path" | \
                       awk '{sum+=$3; count++} END {if(count>0) printf "%.1f", sum/count; else print "0.0"}' | \
                       sed 's/%//')
        
        if ! (( $(echo "$path_coverage >= $CRITICAL_PATH_THRESHOLD" | bc -l) )); then
            critical_status="❌ FAIL"
            critical_failures=$((critical_failures + 1))
        fi
    done
    
    {
        echo "# Quality Gates Report"
        echo "Generated: $(date)"
        echo
        echo "## Gate Status Summary"
        echo
        echo "| Gate | Status | Actual | Threshold | Result |"
        echo "|------|--------|--------|-----------|--------|"
        echo "| Global Coverage | $global_status | ${total_coverage}% | ${GLOBAL_THRESHOLD}% | $([ "$global_status" = "✅ PASS" ] && echo "PASS" || echo "FAIL") |"
        echo "| Critical Paths | $critical_status | $([ $critical_failures -eq 0 ] && echo "All Pass" || echo "$critical_failures Failures") | ${CRITICAL_PATH_THRESHOLD}% | $([ "$critical_status" = "✅ PASS" ] && echo "PASS" || echo "FAIL") |"
        echo
        echo "## Gate Definitions"
        echo
        echo "### Global Coverage Gate"
        echo "- **Requirement**: Overall test coverage must be ≥ ${GLOBAL_THRESHOLD}%"
        echo "- **Scope**: All Go packages in the project"
        echo "- **Purpose**: Ensure minimum quality standard across codebase"
        echo
        echo "### Critical Path Coverage Gate"
        echo "- **Requirement**: Critical modules must have ≥ ${CRITICAL_PATH_THRESHOLD}% coverage"
        echo "- **Critical Modules**: $(printf '%s, ' "${CRITICAL_PATHS[@]}" | sed 's/, $//')"
        echo "- **Purpose**: Ensure high confidence in mission-critical functionality"
        echo
        echo "## CI/CD Integration"
        echo
        echo "### Build Pipeline Requirements"
        echo "- All quality gates must pass for successful build"
        echo "- Coverage reports must be generated and archived"
        echo "- Trend analysis should be performed for every commit"
        echo
        echo "### Failure Actions"
        echo "- **Global Gate Failure**: Block deployment, require coverage improvement"
        echo "- **Critical Gate Failure**: Block deployment, require immediate attention"
        echo "- **Trend Decline**: Warning notification, schedule coverage review"
    } > "$output_file"
    
    log_success "Quality gates report generated: $output_file"
    
    # Return exit code based on gate status
    if [ "$global_status" = "❌ FAIL" ] || [ "$critical_status" = "❌ FAIL" ]; then
        return 1
    else
        return 0
    fi
}

# Generate CI/CD integration script
generate_ci_integration() {
    local ci_script="$PROJECT_ROOT/scripts/ci-coverage-check.sh"
    
    log_info "Generating CI/CD integration script..."
    
    cat > "$ci_script" <<'EOF'
#!/bin/bash

# CI/CD Coverage Integration Script
# This script should be called from your CI/CD pipeline

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Run coverage analysis
bash "$SCRIPT_DIR/coverage-analysis.sh" --ci-mode

# Check exit code
if [ $? -eq 0 ]; then
    echo "✅ All coverage quality gates passed"
    exit 0
elif [ $? -eq 1 ]; then
    echo "❌ Coverage quality gates failed"
    echo "📊 Check coverage reports in coverage/reports/ directory"
    exit 1
elif [ $? -eq 2 ]; then
    echo "🚨 CRITICAL: Critical path coverage violations detected"
    echo "📊 Check coverage reports in coverage/reports/ directory"
    exit 2
else
    echo "❌ Coverage analysis failed"
    exit 3
fi
EOF

    chmod +x "$ci_script"
    log_success "CI/CD integration script generated: $ci_script"
}

# Main coverage analysis function
run_coverage_analysis() {
    local ci_mode=false
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --ci-mode)
                ci_mode=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    setup_directories
    
    local timestamp
    timestamp=$(date +"%Y%m%d_%H%M%S")
    
    local coverage_file="$COVERAGE_DIR/coverage_${timestamp}.out"
    local html_report="$REPORTS_DIR/coverage_${timestamp}.html"
    local json_report="$REPORTS_DIR/coverage_${timestamp}.json"
    local module_report="$REPORTS_DIR/module_analysis_${timestamp}.md"
    local critical_report="$REPORTS_DIR/critical_paths_${timestamp}.md"
    local gates_report="$REPORTS_DIR/quality_gates_${timestamp}.md"
    
    log_info "Starting comprehensive coverage analysis..."
    
    # Generate coverage profile
    if ! generate_coverage_profile "$coverage_file"; then
        log_error "Failed to generate coverage profile"
        exit 3
    fi
    
    # Get total coverage
    local total_coverage
    total_coverage=$(go tool cover -func="$coverage_file" | grep total | awk '{print $3}' | sed 's/%//')
    
    log_info "Overall coverage: ${total_coverage}%"
    
    # Generate reports
    analyze_module_coverage "$coverage_file" "$module_report"
    local module_status=$?
    
    generate_html_report "$coverage_file" "$html_report"
    generate_json_report "$coverage_file" "$json_report"
    identify_uncovered_critical_paths "$coverage_file" "$critical_report"
    generate_quality_gates_report "$coverage_file" "$gates_report"
    local gates_status=$?
    
    # Track trends (not in CI mode to avoid noise)
    if [ "$ci_mode" = false ]; then
        track_coverage_trends "$total_coverage"
    fi
    
    # Create latest symlinks
    ln -sf "$(basename "$html_report")" "$REPORTS_DIR/latest.html"
    ln -sf "$(basename "$json_report")" "$REPORTS_DIR/latest.json"
    ln -sf "$(basename "$gates_report")" "$REPORTS_DIR/latest_quality_gates.md"
    
    log_success "Coverage analysis completed"
    log_info "Reports generated in: $REPORTS_DIR"
    log_info "View HTML report: file://$html_report"
    
    # Return appropriate exit code
    if [ $gates_status -ne 0 ]; then
        if [ $module_status -eq 2 ]; then
            return 2  # Critical violations
        else
            return 1  # Regular violations
        fi
    else
        return 0  # All gates passed
    fi
}

# Help function
show_help() {
    cat <<EOF
Coverage Analysis and Reporting System

USAGE:
    $0 [OPTIONS]

OPTIONS:
    --ci-mode       Run in CI/CD mode (skip trend tracking)
    --help          Show this help message

EXAMPLES:
    # Run full coverage analysis
    $0

    # Run in CI/CD mode
    $0 --ci-mode

REPORTS:
    HTML Report:     coverage/reports/latest.html
    JSON Report:     coverage/reports/latest.json
    Quality Gates:   coverage/reports/latest_quality_gates.md
    Trends:          coverage/trends/coverage_trends.csv

EXIT CODES:
    0 - All quality gates passed
    1 - Quality gates failed
    2 - Critical path violations
    3 - Analysis failed
EOF
}

# Main execution
main() {
    if [[ $# -gt 0 && "$1" == "--help" ]]; then
        show_help
        exit 0
    fi
    
    run_coverage_analysis "$@"
}

# Generate CI integration on first run
if [ ! -f "$PROJECT_ROOT/scripts/ci-coverage-check.sh" ]; then
    generate_ci_integration
fi

main "$@"