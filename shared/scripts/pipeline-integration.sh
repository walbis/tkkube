#!/bin/bash

# Pipeline Integration Script for Backup-to-GitOps Automation
# This script coordinates between the backup tool and GitOps generator

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SCRIPT_DIR}/../config"
SHARED_CONFIG="${CONFIG_DIR}/shared-config.yaml"
ENV_BRIDGE="${SCRIPT_DIR}/env-bridge.sh"

# Default paths - can be overridden
BACKUP_BINARY="${CONFIG_DIR}/../../backup/build/backup"
GITOPS_BINARY="${CONFIG_DIR}/../../kOTN/minio-to-git"
BACKUP_CONFIG="${CONFIG_DIR}/backup.env"
GITOPS_CONFIG="${CONFIG_DIR}/gitops.env"

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

# Help function
show_help() {
    cat << EOF
Pipeline Integration Script for Backup-to-GitOps Automation

USAGE:
    $0 [COMMAND] [OPTIONS]

COMMANDS:
    run             Run complete backup-to-gitops pipeline
    backup-only     Run backup tool only
    gitops-only     Run GitOps generation only
    validate        Validate pipeline configuration
    status          Show pipeline status
    init            Initialize pipeline configuration
    clean           Clean up pipeline artifacts
    help            Show this help message

OPTIONS:
    --config FILE       Shared configuration file (default: shared-config.yaml)
    --backup-bin PATH   Backup tool binary path
    --gitops-bin PATH   GitOps tool binary path
    --mode MODE         Pipeline mode (sequential/parallel)
    --wait-timeout SEC  Maximum wait time for backup completion (default: 300)
    --verbose           Verbose output
    --dry-run           Show what would be done without executing
    --no-cleanup        Skip cleanup after execution
    --continue-on-error Continue pipeline on non-critical errors

EXAMPLES:
    # Run complete pipeline
    $0 run

    # Run with custom configuration
    $0 run --config custom-config.yaml --verbose

    # Run backup only
    $0 backup-only --wait-timeout 600

    # Validate configuration
    $0 validate

    # Initialize new pipeline
    $0 init --config myproject-config.yaml

ENVIRONMENT VARIABLES:
    PIPELINE_MODE           Pipeline execution mode (sequential/parallel)
    AUTO_GITOPS            Auto-trigger GitOps after backup (true/false)
    WAIT_FOR_BACKUP        Wait for backup completion (true/false)
    MAX_WAIT_TIME          Maximum wait time in seconds
    CONTINUE_ON_ERROR      Continue on non-critical errors (true/false)
    BACKUP_BINARY_PATH     Path to backup tool binary
    GITOPS_BINARY_PATH     Path to GitOps tool binary

EOF
}

# Validate pipeline prerequisites
validate_prerequisites() {
    local verbose="${1:-false}"
    local errors=()

    log_info "Validating pipeline prerequisites..."

    # Check shared configuration
    if [[ ! -f "$SHARED_CONFIG" ]]; then
        errors+=("Shared configuration file not found: $SHARED_CONFIG")
    fi

    # Check environment bridge script
    if [[ ! -x "$ENV_BRIDGE" ]]; then
        errors+=("Environment bridge script not executable: $ENV_BRIDGE")
    fi

    # Check backup tool
    if [[ ! -f "$BACKUP_BINARY" && ! -f "$(command -v backup 2>/dev/null || echo '')" ]]; then
        errors+=("Backup tool binary not found: $BACKUP_BINARY")
    fi

    # Check GitOps tool
    if [[ ! -f "$GITOPS_BINARY" && ! -f "$(command -v minio-to-git 2>/dev/null || echo '')" ]]; then
        errors+=("GitOps tool binary not found: $GITOPS_BINARY")
    fi

    # Check required environment variables
    local required_vars=("MINIO_ENDPOINT" "MINIO_ACCESS_KEY" "MINIO_SECRET_KEY" "MINIO_BUCKET")
    for var in "${required_vars[@]}"; do
        if [[ -z "${!var:-}" ]]; then
            errors+=("Required environment variable not set: $var")
        fi
    done

    if [[ ${#errors[@]} -gt 0 ]]; then
        log_error "Validation failed with ${#errors[@]} error(s):"
        for error in "${errors[@]}"; do
            echo "  - $error"
        done
        return 1
    fi

    if [[ "$verbose" == "true" ]]; then
        log_info "✓ Shared configuration found"
        log_info "✓ Environment bridge script ready"
        log_info "✓ Backup tool available"
        log_info "✓ GitOps tool available"
        log_info "✓ Required environment variables set"
    fi

    log_success "Pipeline prerequisites validated"
    return 0
}

# Initialize pipeline configuration
init_pipeline() {
    local config_file="${1:-$SHARED_CONFIG}"
    local verbose="${2:-false}"

    log_info "Initializing pipeline configuration..."

    # Create config directory
    mkdir -p "$CONFIG_DIR"

    # Generate environment bridge files
    "$ENV_BRIDGE" generate --config "$config_file" --verbose="$verbose"
    "$ENV_BRIDGE" sync --verbose="$verbose"

    # Create pipeline status file
    cat > "${CONFIG_DIR}/pipeline-status.json" << EOF
{
  "initialized": "$(date -Iseconds)",
  "config_file": "$config_file",
  "last_run": null,
  "last_backup": null,
  "last_gitops": null,
  "status": "ready"
}
EOF

    log_success "Pipeline initialized successfully"
}

# Run backup tool
run_backup() {
    local wait_for_completion="${1:-true}"
    local timeout="${2:-300}"
    local verbose="${3:-false}"

    log_info "Starting backup process..."

    # Source backup environment
    if [[ -f "$BACKUP_CONFIG" ]]; then
        source "$BACKUP_CONFIG"
    fi

    # Determine backup binary
    local backup_cmd
    if [[ -f "$BACKUP_BINARY" ]]; then
        backup_cmd="$BACKUP_BINARY"
    elif command -v backup &> /dev/null; then
        backup_cmd="backup"
    else
        log_error "Backup tool not found"
        return 1
    fi

    # Run backup
    local backup_start=$(date +%s)
    if [[ "$verbose" == "true" ]]; then
        log_info "Executing: $backup_cmd"
    fi

    if "$backup_cmd"; then
        local backup_end=$(date +%s)
        local backup_duration=$((backup_end - backup_start))
        
        log_success "Backup completed in ${backup_duration}s"
        
        # Update status
        update_pipeline_status "backup" "completed" "$backup_start" "$backup_end"
        
        return 0
    else
        log_error "Backup failed"
        update_pipeline_status "backup" "failed" "$backup_start" "$(date +%s)"
        return 1
    fi
}

# Run GitOps generator
run_gitops() {
    local verbose="${1:-false}"

    log_info "Starting GitOps generation..."

    # Source GitOps environment
    if [[ -f "$GITOPS_CONFIG" ]]; then
        source "$GITOPS_CONFIG"
    fi

    # Determine GitOps binary
    local gitops_cmd
    if [[ -f "$GITOPS_BINARY" ]]; then
        gitops_cmd="$GITOPS_BINARY"
    elif command -v minio-to-git &> /dev/null; then
        gitops_cmd="minio-to-git"
    else
        log_error "GitOps tool not found"
        return 1
    fi

    # Run GitOps generation
    local gitops_start=$(date +%s)
    if [[ "$verbose" == "true" ]]; then
        log_info "Executing: $gitops_cmd"
    fi

    if "$gitops_cmd"; then
        local gitops_end=$(date +%s)
        local gitops_duration=$((gitops_end - gitops_start))
        
        log_success "GitOps generation completed in ${gitops_duration}s"
        
        # Update status
        update_pipeline_status "gitops" "completed" "$gitops_start" "$gitops_end"
        
        return 0
    else
        log_error "GitOps generation failed"
        update_pipeline_status "gitops" "failed" "$gitops_start" "$(date +%s)"
        return 1
    fi
}

# Update pipeline status
update_pipeline_status() {
    local component="$1"
    local status="$2"
    local start_time="$3"
    local end_time="$4"
    
    local status_file="${CONFIG_DIR}/pipeline-status.json"
    
    if [[ -f "$status_file" ]]; then
        # Use jq if available, otherwise use sed
        if command -v jq &> /dev/null; then
            local temp_file=$(mktemp)
            jq --arg comp "$component" --arg stat "$status" --arg start "$start_time" --arg end "$end_time" \
               '.last_run = now | .["last_" + $comp] = {status: $stat, start: ($start | tonumber), end: ($end | tonumber), duration: (($end | tonumber) - ($start | tonumber))}' \
               "$status_file" > "$temp_file" && mv "$temp_file" "$status_file"
        else
            log_info "Status updated: $component -> $status"
        fi
    fi
}

# Show pipeline status
show_status() {
    local status_file="${CONFIG_DIR}/pipeline-status.json"
    
    if [[ -f "$status_file" ]]; then
        if command -v jq &> /dev/null; then
            echo "Pipeline Status:"
            jq . "$status_file"
        else
            echo "Pipeline Status:"
            cat "$status_file"
        fi
    else
        log_warning "No pipeline status found. Run 'init' first."
    fi
}

# Run complete pipeline
run_pipeline() {
    local mode="${1:-sequential}"
    local wait_timeout="${2:-300}"
    local continue_on_error="${3:-false}"
    local verbose="${4:-false}"

    log_info "Running complete backup-to-GitOps pipeline in $mode mode..."

    local pipeline_start=$(date +%s)
    update_pipeline_status "pipeline" "running" "$pipeline_start" "$pipeline_start"

    case "$mode" in
        "sequential")
            # Run backup first, then GitOps
            if run_backup true "$wait_timeout" "$verbose"; then
                if run_gitops "$verbose"; then
                    local pipeline_end=$(date +%s)
                    local total_duration=$((pipeline_end - pipeline_start))
                    log_success "Pipeline completed successfully in ${total_duration}s"
                    update_pipeline_status "pipeline" "completed" "$pipeline_start" "$pipeline_end"
                    return 0
                else
                    if [[ "$continue_on_error" == "true" ]]; then
                        log_warning "GitOps failed but continuing due to continue-on-error"
                        return 0
                    else
                        return 1
                    fi
                fi
            else
                if [[ "$continue_on_error" == "true" ]]; then
                    log_warning "Backup failed but continuing due to continue-on-error"
                    run_gitops "$verbose"
                    return 0
                else
                    return 1
                fi
            fi
            ;;
        "parallel")
            # Run backup and GitOps in parallel (GitOps waits for backup data)
            log_warning "Parallel mode not fully implemented - falling back to sequential"
            run_pipeline "sequential" "$wait_timeout" "$continue_on_error" "$verbose"
            ;;
        *)
            log_error "Unknown pipeline mode: $mode"
            return 1
            ;;
    esac
}

# Clean up pipeline artifacts
cleanup_pipeline() {
    local verbose="${1:-false}"

    log_info "Cleaning up pipeline artifacts..."

    # Remove temporary environment files
    for file in "$BACKUP_CONFIG" "$GITOPS_CONFIG" "${CONFIG_DIR}/pipeline.env"; do
        if [[ -f "$file" ]]; then
            rm -f "$file"
            if [[ "$verbose" == "true" ]]; then
                log_info "Removed: $file"
            fi
        fi
    done

    # Clean up status file
    local status_file="${CONFIG_DIR}/pipeline-status.json"
    if [[ -f "$status_file" ]]; then
        rm -f "$status_file"
        if [[ "$verbose" == "true" ]]; then
            log_info "Removed: $status_file"
        fi
    fi

    log_success "Pipeline cleanup completed"
}

# Main function
main() {
    local command="${1:-help}"
    local config_file="$SHARED_CONFIG"
    local backup_binary="$BACKUP_BINARY"
    local gitops_binary="$GITOPS_BINARY"
    local mode="${PIPELINE_MODE:-sequential}"
    local wait_timeout="${MAX_WAIT_TIME:-300}"
    local verbose=false
    local dry_run=false
    local continue_on_error="${CONTINUE_ON_ERROR:-false}"
    local cleanup=true

    # Parse arguments
    shift || true
    while [[ $# -gt 0 ]]; do
        case $1 in
            --config)
                config_file="$2"
                shift 2
                ;;
            --backup-bin)
                backup_binary="$2"
                shift 2
                ;;
            --gitops-bin)
                gitops_binary="$2"
                shift 2
                ;;
            --mode)
                mode="$2"
                shift 2
                ;;
            --wait-timeout)
                wait_timeout="$2"
                shift 2
                ;;
            --verbose)
                verbose=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            --continue-on-error)
                continue_on_error=true
                shift
                ;;
            --no-cleanup)
                cleanup=false
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    # Update global variables
    SHARED_CONFIG="$config_file"
    BACKUP_BINARY="$backup_binary"
    GITOPS_BINARY="$gitops_binary"

    # Execute command
    case "$command" in
        run)
            if [[ "$dry_run" == "true" ]]; then
                log_info "Would run complete pipeline with mode=$mode, timeout=$wait_timeout"
                validate_prerequisites "$verbose"
            else
                validate_prerequisites "$verbose"
                init_pipeline "$config_file" "$verbose"
                if run_pipeline "$mode" "$wait_timeout" "$continue_on_error" "$verbose"; then
                    if [[ "$cleanup" == "true" ]]; then
                        cleanup_pipeline "$verbose"
                    fi
                else
                    log_error "Pipeline execution failed"
                    exit 1
                fi
            fi
            ;;
        backup-only)
            if [[ "$dry_run" == "true" ]]; then
                log_info "Would run backup tool with timeout=$wait_timeout"
            else
                validate_prerequisites "$verbose"
                init_pipeline "$config_file" "$verbose"
                run_backup true "$wait_timeout" "$verbose"
            fi
            ;;
        gitops-only)
            if [[ "$dry_run" == "true" ]]; then
                log_info "Would run GitOps generation"
            else
                validate_prerequisites "$verbose"
                init_pipeline "$config_file" "$verbose"
                run_gitops "$verbose"
            fi
            ;;
        validate)
            validate_prerequisites "$verbose"
            ;;
        status)
            show_status
            ;;
        init)
            init_pipeline "$config_file" "$verbose"
            ;;
        clean)
            cleanup_pipeline "$verbose"
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "Unknown command: $command"
            show_help
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"