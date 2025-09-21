#!/bin/bash

# Webhook Examples for Integration Bridge
# This script demonstrates how to interact with the integration bridge webhooks

set -e

BRIDGE_URL="${BRIDGE_URL:-http://localhost:8080}"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "🌉 Integration Bridge Webhook Examples"
echo "======================================="
echo "Bridge URL: $BRIDGE_URL"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    case $1 in
        "success") echo -e "${GREEN}✅ $2${NC}" ;;
        "error") echo -e "${RED}❌ $2${NC}" ;;
        "warning") echo -e "${YELLOW}⚠️  $2${NC}" ;;
        "info") echo -e "${BLUE}ℹ️  $2${NC}" ;;
    esac
}

# Function to make HTTP request and show response
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    echo ""
    print_status "info" "$description"
    echo "📡 $method $BRIDGE_URL$endpoint"
    
    if [ -n "$data" ]; then
        echo "📄 Request data:"
        echo "$data" | jq '.' 2>/dev/null || echo "$data"
        echo ""
        
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" \
            -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$BRIDGE_URL$endpoint")
    else
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" \
            -X "$method" \
            "$BRIDGE_URL$endpoint")
    fi
    
    # Extract HTTP status and body
    http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    response_body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    echo "📥 Response (HTTP $http_code):"
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_status "success" "Request successful"
        echo "$response_body" | jq '.' 2>/dev/null || echo "$response_body"
    else
        print_status "error" "Request failed"
        echo "$response_body" | jq '.' 2>/dev/null || echo "$response_body"
    fi
    
    echo ""
    echo "─────────────────────────────────────────────────────────────"
}

# 1. Health Check
make_request "GET" "/health" "" "1. Health Check"

# 2. System Status
make_request "GET" "/status" "" "2. System Status"

# 3. Register Backup Tool
backup_registration='{
    "endpoint": "http://backup-tool:8080",
    "version": "1.0.0"
}'
make_request "POST" "/register/backup" "$backup_registration" "3. Register Backup Tool"

# 4. Register GitOps Generator
gitops_registration='{
    "endpoint": "http://gitops-generator:8081", 
    "version": "2.1.0"
}'
make_request "POST" "/register/gitops" "$gitops_registration" "4. Register GitOps Generator"

# 5. Successful Backup Completion
successful_backup='{
    "id": "webhook-example-1",
    "type": "backup_completed",
    "source": "backup-tool",
    "timestamp": "'$TIMESTAMP'",
    "data": {
        "backup_id": "example-backup-001",
        "cluster_name": "production-cluster",
        "timestamp": "'$TIMESTAMP'",
        "resource_count": 45,
        "size": 10485760,
        "success": true,
        "minio_path": "production-cluster/2024/01/15/example-backup-001"
    }
}'
make_request "POST" "/webhooks/backup/completed" "$successful_backup" "5. Successful Backup Completion"

# 6. Failed Backup Completion
failed_backup='{
    "id": "webhook-example-2", 
    "type": "backup_completed",
    "source": "backup-tool",
    "timestamp": "'$TIMESTAMP'",
    "data": {
        "backup_id": "example-backup-002",
        "cluster_name": "production-cluster", 
        "timestamp": "'$TIMESTAMP'",
        "resource_count": 0,
        "size": 0,
        "success": false,
        "error_message": "Failed to connect to Kubernetes API",
        "minio_path": ""
    }
}'
make_request "POST" "/webhooks/backup/completed" "$failed_backup" "6. Failed Backup Completion"

# 7. GitOps Generation Request
gitops_request='{
    "id": "webhook-example-3",
    "type": "gitops_generation_requested", 
    "source": "integration-bridge",
    "timestamp": "'$TIMESTAMP'",
    "data": {
        "request_id": "gitops-example-backup-001-'$(date +%s)'",
        "backup_id": "example-backup-001",
        "cluster_name": "production-cluster",
        "source_path": "production-cluster/2024/01/15/example-backup-001",
        "target_repo": "https://github.com/company/gitops-repo",
        "target_branch": "main",
        "configuration": {
            "namespace_per_directory": true,
            "include_metadata": true,
            "remove_status": true
        }
    }
}'
make_request "POST" "/webhooks/gitops/generate" "$gitops_request" "7. GitOps Generation Request"

# 8. GitOps Generation Completion
gitops_completion='{
    "id": "webhook-example-4",
    "type": "gitops_completed", 
    "source": "gitops-generator", 
    "timestamp": "'$TIMESTAMP'",
    "data": {
        "request_id": "gitops-example-backup-001-'$(date +%s)'",
        "status": "completed",
        "files_generated": 12,
        "files_committed": 12,
        "git_commit_hash": "abc123def456",
        "duration_seconds": 45.2
    }
}' 
make_request "POST" "/webhooks/gitops/completed" "$gitops_completion" "8. GitOps Generation Completion"

# 9. Final Status Check
make_request "GET" "/status" "" "9. Final Status Check"

echo ""
print_status "success" "All webhook examples completed!"
echo ""
echo "🔗 Useful endpoints to explore:"
echo "   - Health: $BRIDGE_URL/health"
echo "   - Status: $BRIDGE_URL/status" 
echo "   - Metrics: $BRIDGE_URL/metrics (if Prometheus enabled)"
echo ""
echo "📚 For more examples, see:"
echo "   - Integration tests: ../integration_test.go"
echo "   - Complete example: ./complete_integration_example.go"
echo "   - Documentation: ../README.md"
echo ""
echo "🐳 To run with Docker:"
echo "   docker-compose up -d"
echo "   ./webhook_examples.sh"
echo ""