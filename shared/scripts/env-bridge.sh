#!/bin/bash

# Environment Variable Bridge for Backup-to-GitOps Pipeline
# This script creates a unified environment variable interface
# that both the backup tool and GitOps tool can use

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
ENV_FILE="${CONFIG_DIR}/pipeline.env"

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
Environment Variable Bridge for Backup-to-GitOps Pipeline

USAGE:
    $0 [COMMAND] [OPTIONS]

COMMANDS:
    generate        Generate environment file from shared config
    validate        Validate environment variables
    source          Source environment variables into current shell
    export          Export environment variables for child processes
    sync            Sync environment variables between tools
    template        Generate environment template
    help            Show this help message

OPTIONS:
    --config FILE   Shared configuration file (default: shared-config.yaml)
    --env-file FILE Environment file (default: pipeline.env)
    --verbose       Verbose output
    --dry-run       Show what would be done without executing

EXAMPLES:
    # Generate environment file from shared config
    $0 generate --config shared-config.yaml

    # Validate current environment
    $0 validate

    # Source environment variables
    source <($0 source)

    # Export for use in scripts
    $0 export > .env

    # Sync environment between backup and GitOps tools
    $0 sync --verbose

ENVIRONMENT VARIABLES:
    The following environment variables are supported:

    Storage Configuration:
        MINIO_ENDPOINT       MinIO server endpoint
        MINIO_ACCESS_KEY     MinIO access key
        MINIO_SECRET_KEY     MinIO secret key
        MINIO_BUCKET         MinIO bucket name
        MINIO_USE_SSL        Use SSL for MinIO connection (true/false)
        MINIO_REGION         MinIO region

    Cluster Configuration:
        CLUSTER_NAME         Kubernetes cluster name
        CLUSTER_DOMAIN       Cluster domain
        CLUSTER_TYPE         Cluster type (kubernetes/openshift)

    Git Configuration:
        GIT_REPOSITORY       Git repository URL
        GIT_BRANCH           Git branch (default: main)
        GIT_AUTH_METHOD      Authentication method (ssh/pat/basic)
        GIT_PAT_TOKEN        Personal access token
        GIT_PAT_USERNAME     PAT username
        GIT_USERNAME         Basic auth username
        GIT_PASSWORD         Basic auth password
        GIT_SSH_KEY          SSH private key path
        GIT_SSH_PASSPHRASE   SSH key passphrase

    Backup Configuration:
        BATCH_SIZE           Backup batch size
        RETENTION_DAYS       Backup retention days
        ENABLE_CLEANUP       Enable cleanup (true/false)
        CLEANUP_ON_STARTUP   Cleanup on startup (true/false)
        AUTO_CREATE_BUCKET   Auto-create bucket (true/false)

    Pipeline Configuration:
        PIPELINE_MODE        Pipeline mode (sequential/parallel)
        AUTO_GITOPS          Auto-trigger GitOps (true/false)
        CONTINUE_ON_ERROR    Continue on error (true/false)

    Observability:
        LOG_LEVEL            Log level (debug/info/warning/error)
        LOG_FORMAT           Log format (json/text)
        METRICS_ENABLED      Enable metrics (true/false)
        METRICS_PORT         Metrics port

EOF
}

# Environment variable mappings
declare -A ENV_MAPPINGS=(
    # Storage
    ["MINIO_ENDPOINT"]="storage.endpoint"
    ["MINIO_ACCESS_KEY"]="storage.access_key"
    ["MINIO_SECRET_KEY"]="storage.secret_key"
    ["MINIO_BUCKET"]="storage.bucket"
    ["MINIO_USE_SSL"]="storage.use_ssl"
    ["MINIO_REGION"]="storage.region"
    ["AUTO_CREATE_BUCKET"]="storage.auto_create_bucket"

    # Cluster
    ["CLUSTER_NAME"]="cluster.name"
    ["CLUSTER_DOMAIN"]="cluster.domain"
    ["CLUSTER_TYPE"]="cluster.type"

    # Git
    ["GIT_REPOSITORY"]="gitops.repository.url"
    ["GIT_BRANCH"]="gitops.repository.branch"
    ["GIT_AUTH_METHOD"]="gitops.repository.auth.method"
    ["GIT_PAT_TOKEN"]="gitops.repository.auth.pat.token"
    ["GIT_PAT_USERNAME"]="gitops.repository.auth.pat.username"
    ["GIT_USERNAME"]="gitops.repository.auth.basic.username"
    ["GIT_PASSWORD"]="gitops.repository.auth.basic.password"
    ["GIT_SSH_KEY"]="gitops.repository.auth.ssh.private_key_path"
    ["GIT_SSH_PASSPHRASE"]="gitops.repository.auth.ssh.passphrase"

    # Backup
    ["BATCH_SIZE"]="backup.behavior.batch_size"
    ["RETENTION_DAYS"]="backup.cleanup.retention_days"
    ["ENABLE_CLEANUP"]="backup.cleanup.enabled"
    ["CLEANUP_ON_STARTUP"]="backup.cleanup.cleanup_on_startup"

    # Pipeline
    ["PIPELINE_MODE"]="pipeline.mode"
    ["AUTO_GITOPS"]="pipeline.automation.enabled"
    ["CONTINUE_ON_ERROR"]="pipeline.error_handling.continue_on_error"

    # Observability
    ["LOG_LEVEL"]="observability.logging.level"
    ["LOG_FORMAT"]="observability.logging.format"
    ["METRICS_ENABLED"]="observability.metrics.enabled"
    ["METRICS_PORT"]="observability.metrics.port"
)

# Required environment variables
REQUIRED_VARS=(
    "MINIO_ENDPOINT"
    "MINIO_ACCESS_KEY"
    "MINIO_SECRET_KEY"
    "MINIO_BUCKET"
    "CLUSTER_NAME"
)

# Generate environment file from shared config
generate_env_file() {
    local config_file="${1:-shared-config.yaml}"
    local env_file="${2:-$ENV_FILE}"
    local verbose="${3:-false}"

    log_info "Generating environment file from $config_file"

    # Create config directory if it doesn't exist
    mkdir -p "$(dirname "$env_file")"

    # Check if config file exists
    if [[ ! -f "$config_file" ]]; then
        log_error "Configuration file not found: $config_file"
        return 1
    fi

    # Generate environment file header
    cat > "$env_file" << EOF
# Generated Environment Variables for Backup-to-GitOps Pipeline
# Generated on: $(date)
# Source config: $config_file

EOF

    # Extract values from YAML and create environment variables
    if command -v yq &> /dev/null; then
        for env_var in "${!ENV_MAPPINGS[@]}"; do
            local yaml_path="${ENV_MAPPINGS[$env_var]}"
            local value=$(yq eval ".$yaml_path" "$config_file" 2>/dev/null || echo "")
            
            if [[ "$value" != "null" && "$value" != "" ]]; then
                echo "export $env_var=\"$value\"" >> "$env_file"
                if [[ "$verbose" == "true" ]]; then
                    log_info "Set $env_var=$value"
                fi
            fi
        done
    else
        log_warning "yq not found. Using basic YAML parsing."
        # Basic YAML parsing (limited functionality)
        while IFS= read -r line; do
            if [[ "$line" =~ ^[[:space:]]*endpoint:[[:space:]]*(.+)$ ]]; then
                echo "export MINIO_ENDPOINT=\"${BASH_REMATCH[1]}\"" >> "$env_file"
            elif [[ "$line" =~ ^[[:space:]]*access_key:[[:space:]]*(.+)$ ]]; then
                echo "export MINIO_ACCESS_KEY=\"${BASH_REMATCH[1]}\"" >> "$env_file"
            elif [[ "$line" =~ ^[[:space:]]*secret_key:[[:space:]]*(.+)$ ]]; then
                echo "export MINIO_SECRET_KEY=\"${BASH_REMATCH[1]}\"" >> "$env_file"
            elif [[ "$line" =~ ^[[:space:]]*bucket:[[:space:]]*(.+)$ ]]; then
                echo "export MINIO_BUCKET=\"${BASH_REMATCH[1]}\"" >> "$env_file"
            fi
        done < "$config_file"
    fi

    log_success "Environment file generated: $env_file"
}

# Validate environment variables
validate_env() {
    local verbose="${1:-false}"
    local missing_vars=()
    local warnings=()

    log_info "Validating environment variables..."

    # Check required variables
    for var in "${REQUIRED_VARS[@]}"; do
        if [[ -z "${!var:-}" ]]; then
            missing_vars+=("$var")
        elif [[ "$verbose" == "true" ]]; then
            log_info "✓ $var is set"
        fi
    done

    # Check for warnings
    if [[ -n "${MINIO_SECRET_KEY:-}" && "${#MINIO_SECRET_KEY}" -lt 8 ]]; then
        warnings+=("MINIO_SECRET_KEY should be at least 8 characters")
    fi

    if [[ "${MINIO_USE_SSL:-true}" == "false" && "${MINIO_ENDPOINT:-}" == *"localhost"* ]]; then
        warnings+=("SSL disabled for localhost connection")
    fi

    if [[ -n "${GIT_PAT_TOKEN:-}" && "${#GIT_PAT_TOKEN}" -lt 20 ]]; then
        warnings+=("GIT_PAT_TOKEN seems too short for a valid token")
    fi

    # Report results
    if [[ ${#missing_vars[@]} -gt 0 ]]; then
        log_error "Missing required environment variables:"
        for var in "${missing_vars[@]}"; do
            echo "  - $var"
        done
        return 1
    fi

    if [[ ${#warnings[@]} -gt 0 ]]; then
        log_warning "Environment variable warnings:"
        for warning in "${warnings[@]}"; do
            echo "  - $warning"
        done
    fi

    log_success "Environment validation passed"
    return 0
}

# Source environment variables
source_env() {
    local env_file="${1:-$ENV_FILE}"

    if [[ -f "$env_file" ]]; then
        cat "$env_file"
    else
        log_error "Environment file not found: $env_file"
        return 1
    fi
}

# Export environment variables
export_env() {
    local env_file="${1:-$ENV_FILE}"
    local format="${2:-dotenv}"

    if [[ ! -f "$env_file" ]]; then
        log_error "Environment file not found: $env_file"
        return 1
    fi

    case "$format" in
        "dotenv")
            # Remove 'export ' prefix for .env format
            sed 's/^export //' "$env_file"
            ;;
        "docker")
            # Docker-compose format
            sed 's/^export \([^=]*\)=\(.*\)$/\1=\2/' "$env_file"
            ;;
        "k8s")
            # Kubernetes ConfigMap format
            echo "apiVersion: v1"
            echo "kind: ConfigMap"
            echo "metadata:"
            echo "  name: backup-gitops-config"
            echo "data:"
            sed 's/^export \([^=]*\)="\(.*\)"$/  \1: "\2"/' "$env_file"
            ;;
        *)
            log_error "Unknown export format: $format"
            return 1
            ;;
    esac
}

# Sync environment variables between tools
sync_env() {
    local verbose="${1:-false}"

    log_info "Syncing environment variables between backup and GitOps tools..."

    # Check if both tools are available
    local backup_binary="${CONFIG_DIR}/../../backup/build/backup"
    local gitops_binary="${CONFIG_DIR}/../../kOTN/minio-to-git"

    if [[ ! -f "$backup_binary" && ! -f "$(command -v backup 2>/dev/null)" ]]; then
        log_warning "Backup tool binary not found"
    fi

    if [[ ! -f "$gitops_binary" && ! -f "$(command -v minio-to-git 2>/dev/null)" ]]; then
        log_warning "GitOps tool binary not found"
    fi

    # Create tool-specific environment files
    local backup_env="${CONFIG_DIR}/backup.env"
    local gitops_env="${CONFIG_DIR}/gitops.env"

    # Generate backup tool environment
    cat > "$backup_env" << EOF
# Environment variables for backup tool
export CLUSTER_NAME="${CLUSTER_NAME:-}"
export CLUSTER_DOMAIN="${CLUSTER_DOMAIN:-cluster.local}"
export MINIO_ENDPOINT="${MINIO_ENDPOINT:-}"
export MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-}"
export MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-}"
export MINIO_BUCKET="${MINIO_BUCKET:-}"
export MINIO_USE_SSL="${MINIO_USE_SSL:-true}"
export BATCH_SIZE="${BATCH_SIZE:-50}"
export RETENTION_DAYS="${RETENTION_DAYS:-7}"
export ENABLE_CLEANUP="${ENABLE_CLEANUP:-true}"
export CLEANUP_ON_STARTUP="${CLEANUP_ON_STARTUP:-false}"
export AUTO_CREATE_BUCKET="${AUTO_CREATE_BUCKET:-false}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
EOF

    # Generate GitOps tool environment
    cat > "$gitops_env" << EOF
# Environment variables for GitOps tool
export MINIO_ENDPOINT="${MINIO_ENDPOINT:-}"
export MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-}"
export MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-}"
export MINIO_BUCKET="${MINIO_BUCKET:-}"
export GIT_REPOSITORY="${GIT_REPOSITORY:-}"
export GIT_BRANCH="${GIT_BRANCH:-main}"
export GIT_AUTH_METHOD="${GIT_AUTH_METHOD:-ssh}"
export GIT_PAT_TOKEN="${GIT_PAT_TOKEN:-}"
export GIT_PAT_USERNAME="${GIT_PAT_USERNAME:-}"
export GIT_USERNAME="${GIT_USERNAME:-}"
export GIT_PASSWORD="${GIT_PASSWORD:-}"
export GIT_SSH_KEY="${GIT_SSH_KEY:-~/.ssh/id_rsa}"
export MINIO_TO_GIT_LOG_LEVEL="${LOG_LEVEL:-INFO}"
export MINIO_TO_GIT_LOG_JSON="${LOG_FORMAT:-json}"
EOF

    if [[ "$verbose" == "true" ]]; then
        log_info "Generated backup environment: $backup_env"
        log_info "Generated GitOps environment: $gitops_env"
    fi

    log_success "Environment sync completed"
}

# Generate environment template
generate_template() {
    local template_file="${1:-template.env}"

    log_info "Generating environment template: $template_file"

    cat > "$template_file" << 'EOF'
# Backup-to-GitOps Pipeline Environment Variables Template
# Copy this file to .env and fill in your values

# =============================================================================
# STORAGE CONFIGURATION (Required)
# =============================================================================

# MinIO/S3 Configuration
MINIO_ENDPOINT=localhost:9000              # MinIO server endpoint
MINIO_ACCESS_KEY=minioadmin                # MinIO access key
MINIO_SECRET_KEY=minioadmin123             # MinIO secret key
MINIO_BUCKET=cluster-backups               # Bucket name for backups
MINIO_USE_SSL=false                        # Use SSL/TLS (true/false)
MINIO_REGION=us-east-1                     # MinIO region

# Advanced Storage Options
AUTO_CREATE_BUCKET=false                   # Auto-create bucket if missing

# =============================================================================
# CLUSTER CONFIGURATION (Required)
# =============================================================================

CLUSTER_NAME=my-cluster                    # Kubernetes cluster name
CLUSTER_DOMAIN=cluster.local               # Cluster domain
CLUSTER_TYPE=kubernetes                    # kubernetes or openshift

# =============================================================================
# GIT CONFIGURATION (Required for GitOps)
# =============================================================================

GIT_REPOSITORY=https://github.com/user/repo.git  # Git repository URL
GIT_BRANCH=main                            # Git branch

# Authentication Method (choose one: ssh, pat, basic)
GIT_AUTH_METHOD=ssh

# SSH Authentication
GIT_SSH_KEY=~/.ssh/id_rsa                  # SSH private key path
GIT_SSH_PASSPHRASE=                        # SSH key passphrase

# Personal Access Token Authentication
GIT_PAT_TOKEN=                             # GitHub/GitLab PAT
GIT_PAT_USERNAME=                          # Username for PAT

# Basic Authentication
GIT_USERNAME=                              # Git username
GIT_PASSWORD=                              # Git password

# =============================================================================
# BACKUP CONFIGURATION (Optional)
# =============================================================================

BATCH_SIZE=50                              # Backup batch size
RETENTION_DAYS=7                           # Backup retention days
ENABLE_CLEANUP=true                        # Enable cleanup
CLEANUP_ON_STARTUP=false                   # Cleanup on startup

# Resource Filtering
FILTERING_MODE=whitelist                   # whitelist, blacklist, hybrid
LABEL_SELECTOR=                            # Kubernetes label selector
ANNOTATION_SELECTOR=                       # Kubernetes annotation selector

# =============================================================================
# PIPELINE CONFIGURATION (Optional)
# =============================================================================

PIPELINE_MODE=sequential                   # sequential, parallel
AUTO_GITOPS=true                          # Auto-trigger GitOps after backup
CONTINUE_ON_ERROR=false                   # Continue pipeline on errors

# Notifications
WEBHOOK_URL=                              # Webhook for notifications
SLACK_WEBHOOK_URL=                        # Slack webhook URL
SLACK_CHANNEL=#backup-notifications       # Slack channel

# =============================================================================
# OBSERVABILITY (Optional)
# =============================================================================

LOG_LEVEL=info                            # debug, info, warning, error
LOG_FORMAT=json                           # json, text
METRICS_ENABLED=true                      # Enable Prometheus metrics
METRICS_PORT=8080                         # Metrics port

# Tracing
TRACING_ENABLED=false                     # Enable distributed tracing
TRACING_ENDPOINT=                         # Tracing endpoint
TRACING_SAMPLE_RATE=0.1                   # Trace sampling rate

# =============================================================================
# SECURITY (Optional)
# =============================================================================

VERIFY_SSL=true                           # Verify SSL certificates
STRICT_VALIDATION=true                    # Strict validation mode
SCAN_SECRETS=true                         # Scan for secrets in backups

# =============================================================================
# ENVIRONMENT-SPECIFIC CLUSTERS (Optional)
# =============================================================================

DEV_CLUSTER_URL=https://dev-cluster.example.com
TEST_CLUSTER_URL=https://test-cluster.example.com
PREPROD_CLUSTER_URL=https://preprod-cluster.example.com
PROD_CLUSTER_URL=https://prod-cluster.example.com

EOF

    log_success "Environment template generated: $template_file"
}

# Main function
main() {
    local command="${1:-help}"
    local config_file="shared-config.yaml"
    local env_file="$ENV_FILE"
    local verbose=false
    local dry_run=false

    # Parse arguments
    shift || true
    while [[ $# -gt 0 ]]; do
        case $1 in
            --config)
                config_file="$2"
                shift 2
                ;;
            --env-file)
                env_file="$2"
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
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    # Execute command
    case "$command" in
        generate)
            if [[ "$dry_run" == "true" ]]; then
                log_info "Would generate environment file from $config_file to $env_file"
            else
                generate_env_file "$config_file" "$env_file" "$verbose"
            fi
            ;;
        validate)
            validate_env "$verbose"
            ;;
        source)
            source_env "$env_file"
            ;;
        export)
            export_env "$env_file" "${2:-dotenv}"
            ;;
        sync)
            if [[ "$dry_run" == "true" ]]; then
                log_info "Would sync environment variables between tools"
            else
                sync_env "$verbose"
            fi
            ;;
        template)
            template_file="${2:-template.env}"
            if [[ "$dry_run" == "true" ]]; then
                log_info "Would generate environment template: $template_file"
            else
                generate_template "$template_file"
            fi
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