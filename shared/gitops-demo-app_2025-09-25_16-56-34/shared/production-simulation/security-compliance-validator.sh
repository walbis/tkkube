#!/bin/bash

# Security Compliance Validator
# Enterprise GitOps Pipeline Security Assessment
# Version: 1.0.0

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${SCRIPT_DIR}/security-logs"
RESULTS_DIR="${SCRIPT_DIR}/security-results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Security scoring
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0
HIGH_SEVERITY_ISSUES=0
MEDIUM_SEVERITY_ISSUES=0
LOW_SEVERITY_ISSUES=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging functions
log_security() {
    echo -e "${BLUE}[SECURITY]${NC} $1" | tee -a "${LOG_DIR}/security_validation_${TIMESTAMP}.log"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1" | tee -a "${LOG_DIR}/security_validation_${TIMESTAMP}.log"
    ((PASSED_CHECKS++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1" | tee -a "${LOG_DIR}/security_validation_${TIMESTAMP}.log"
    ((FAILED_CHECKS++))
    case "${2:-medium}" in
        "high") ((HIGH_SEVERITY_ISSUES++)) ;;
        "medium") ((MEDIUM_SEVERITY_ISSUES++)) ;;
        "low") ((LOW_SEVERITY_ISSUES++)) ;;
    esac
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" | tee -a "${LOG_DIR}/security_validation_${TIMESTAMP}.log"
}

# Initialize security validation
initialize_security_validation() {
    log_security "Initializing security compliance validation..."
    
    mkdir -p "${LOG_DIR}" "${RESULTS_DIR}"
    
    # Create security report structure
    cat > "${RESULTS_DIR}/security_report_${TIMESTAMP}.json" << EOF
{
    "security_validation": {
        "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
        "version": "1.0.0",
        "environment": "production-simulation",
        "overall_status": "in_progress",
        "categories": {
            "rbac": {"status": "pending", "checks": [], "issues": []},
            "network_security": {"status": "pending", "checks": [], "issues": []},
            "pod_security": {"status": "pending", "checks": [], "issues": []},
            "secrets_management": {"status": "pending", "checks": [], "issues": []},
            "container_security": {"status": "pending", "checks": [], "issues": []},
            "compliance": {"status": "pending", "checks": [], "issues": []}
        },
        "summary": {
            "total_checks": 0,
            "passed_checks": 0,
            "failed_checks": 0,
            "high_severity_issues": 0,
            "medium_severity_issues": 0,
            "low_severity_issues": 0
        }
    }
}
EOF
    
    log_security "Security validation environment initialized"
}

# RBAC Security Validation
validate_rbac_security() {
    log_security "Validating RBAC (Role-Based Access Control) security..."
    
    ((TOTAL_CHECKS++))
    # Check for default service account usage
    log_security "Checking for default service account usage..."
    if kubectl get pods --all-namespaces -o jsonpath='{range .items[*]}{.spec.serviceAccountName}{"\n"}{end}' | grep -q "^$"; then
        log_fail "❌ Pods using default service account detected" "medium"
        add_security_issue "rbac" "default_service_account" "medium" "Pods using default service account with excessive permissions"
    else
        log_pass "✅ No pods using default service account"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for cluster-admin role bindings
    log_security "Checking for excessive cluster-admin permissions..."
    cluster_admin_bindings=$(kubectl get clusterrolebindings -o json | jq -r '.items[] | select(.roleRef.name=="cluster-admin") | .metadata.name' | wc -l)
    if [ "$cluster_admin_bindings" -gt 3 ]; then
        log_fail "❌ Excessive cluster-admin role bindings detected ($cluster_admin_bindings)" "high"
        add_security_issue "rbac" "excessive_cluster_admin" "high" "Too many cluster-admin role bindings detected"
    else
        log_pass "✅ Cluster-admin role bindings within acceptable limits"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for proper RBAC policies
    log_security "Validating RBAC policy configuration..."
    if kubectl get roles,rolebindings,clusterroles,clusterrolebindings | grep -q "rbac.authorization.k8s.io"; then
        log_pass "✅ RBAC policies are configured and active"
    else
        log_fail "❌ RBAC policies not properly configured" "high"
        add_security_issue "rbac" "missing_policies" "high" "RBAC authorization policies not found"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for service account token automounting
    log_security "Checking service account token automounting..."
    auto_mount_count=$(kubectl get serviceaccounts --all-namespaces -o json | jq '[.items[] | select(.automountServiceAccountToken != false)] | length')
    if [ "$auto_mount_count" -gt 5 ]; then
        log_warn "⚠️ Service accounts with token automounting detected ($auto_mount_count)"
        add_security_issue "rbac" "token_automounting" "low" "Service accounts allowing token automounting"
    else
        log_pass "✅ Service account token automounting properly configured"
    fi
}

# Network Security Validation
validate_network_security() {
    log_security "Validating network security policies..."
    
    ((TOTAL_CHECKS++))
    # Check for network policies
    log_security "Checking for network policy implementation..."
    network_policies=$(kubectl get networkpolicies --all-namespaces --no-headers | wc -l)
    if [ "$network_policies" -eq 0 ]; then
        log_fail "❌ No network policies found - default allow-all traffic" "medium"
        add_security_issue "network_security" "no_network_policies" "medium" "No network policies implemented, allowing unrestricted pod communication"
    else
        log_pass "✅ Network policies implemented ($network_policies policies found)"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for default deny policies
    log_security "Checking for default deny network policies..."
    default_deny_policies=$(kubectl get networkpolicies --all-namespaces -o json | jq '[.items[] | select(.spec.podSelector == {})] | length')
    if [ "$default_deny_policies" -eq 0 ]; then
        log_warn "⚠️ No default deny network policies found"
        add_security_issue "network_security" "no_default_deny" "low" "Default deny network policies not implemented"
    else
        log_pass "✅ Default deny network policies in place"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for ingress/egress rules
    log_security "Validating ingress and egress traffic controls..."
    if kubectl get networkpolicies --all-namespaces -o json | jq -e '.items[] | select(.spec.ingress or .spec.egress)' > /dev/null; then
        log_pass "✅ Ingress/egress traffic controls configured"
    else
        log_fail "❌ No ingress/egress traffic controls found" "medium"
        add_security_issue "network_security" "no_traffic_controls" "medium" "No specific ingress/egress rules configured"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for services with external access
    log_security "Checking for services with external access..."
    external_services=$(kubectl get services --all-namespaces -o json | jq '[.items[] | select(.spec.type=="LoadBalancer" or .spec.type=="NodePort")] | length')
    if [ "$external_services" -gt 3 ]; then
        log_warn "⚠️ Multiple services with external access detected ($external_services)"
        add_security_issue "network_security" "excessive_external_access" "low" "Multiple services exposed externally"
    else
        log_pass "✅ External service exposure within acceptable limits"
    fi
}

# Pod Security Validation
validate_pod_security() {
    log_security "Validating pod security standards..."
    
    ((TOTAL_CHECKS++))
    # Check for privileged containers
    log_security "Checking for privileged containers..."
    privileged_pods=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[] | select(.securityContext.privileged == true)] | length')
    if [ "$privileged_pods" -gt 0 ]; then
        log_fail "❌ Privileged containers detected ($privileged_pods)" "high"
        add_security_issue "pod_security" "privileged_containers" "high" "Privileged containers running with elevated permissions"
    else
        log_pass "✅ No privileged containers detected"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for containers running as root
    log_security "Checking for containers running as root user..."
    root_containers=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[] | select(.securityContext.runAsUser == 0 or (.securityContext.runAsUser == null and .securityContext.runAsNonRoot != true))] | length')
    if [ "$root_containers" -gt 2 ]; then
        log_fail "❌ Containers running as root detected ($root_containers)" "medium"
        add_security_issue "pod_security" "root_containers" "medium" "Containers running with root user privileges"
    else
        log_pass "✅ Container root user usage within acceptable limits"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for read-only root filesystems
    log_security "Checking for read-only root filesystems..."
    readonly_fs=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[] | select(.securityContext.readOnlyRootFilesystem == true)] | length')
    total_containers=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[]] | length')
    readonly_percentage=$((readonly_fs * 100 / total_containers))
    if [ "$readonly_percentage" -lt 50 ]; then
        log_warn "⚠️ Low percentage of containers with read-only root filesystem ($readonly_percentage%)"
        add_security_issue "pod_security" "writable_root_fs" "low" "Many containers have writable root filesystems"
    else
        log_pass "✅ Good adoption of read-only root filesystems ($readonly_percentage%)"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for security contexts
    log_security "Validating pod security contexts..."
    pods_with_security_context=$(kubectl get pods --all-namespaces -o json | jq '[.items[] | select(.spec.securityContext)] | length')
    total_pods=$(kubectl get pods --all-namespaces --no-headers | wc -l)
    security_context_percentage=$((pods_with_security_context * 100 / total_pods))
    if [ "$security_context_percentage" -lt 80 ]; then
        log_fail "❌ Low security context adoption ($security_context_percentage%)" "medium"
        add_security_issue "pod_security" "missing_security_contexts" "medium" "Many pods lack proper security contexts"
    else
        log_pass "✅ Good security context adoption ($security_context_percentage%)"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for capabilities
    log_security "Checking for dangerous capabilities..."
    dangerous_caps=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[].securityContext.capabilities.add[]? | select(. == "SYS_ADMIN" or . == "NET_ADMIN" or . == "SYS_TIME")] | length')
    if [ "$dangerous_caps" -gt 0 ]; then
        log_fail "❌ Dangerous capabilities detected ($dangerous_caps)" "high"
        add_security_issue "pod_security" "dangerous_capabilities" "high" "Containers with dangerous Linux capabilities"
    else
        log_pass "✅ No dangerous capabilities detected"
    fi
}

# Secrets Management Validation
validate_secrets_management() {
    log_security "Validating secrets management..."
    
    ((TOTAL_CHECKS++))
    # Check for secrets in environment variables
    log_security "Checking for secrets exposed in environment variables..."
    env_secrets=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[].env[]? | select(.name | test("PASSWORD|SECRET|TOKEN|KEY"; "i")) | select(.value)] | length')
    if [ "$env_secrets" -gt 0 ]; then
        log_fail "❌ Secrets exposed in environment variables ($env_secrets)" "high"
        add_security_issue "secrets_management" "env_secrets" "high" "Secrets exposed as plain text environment variables"
    else
        log_pass "✅ No secrets exposed in environment variables"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for proper secret usage
    log_security "Validating Kubernetes secret usage..."
    secrets_count=$(kubectl get secrets --all-namespaces --no-headers | grep -v "kubernetes.io/service-account-token" | wc -l)
    if [ "$secrets_count" -eq 0 ]; then
        log_warn "⚠️ No custom secrets found - may indicate improper secret management"
        add_security_issue "secrets_management" "no_custom_secrets" "low" "No custom Kubernetes secrets found"
    else
        log_pass "✅ Kubernetes secrets properly utilized ($secrets_count secrets found)"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for secret mounting
    log_security "Checking secret mounting practices..."
    volume_mounted_secrets=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.volumes[]? | select(.secret)] | length')
    if [ "$volume_mounted_secrets" -gt 0 ]; then
        log_pass "✅ Secrets properly mounted as volumes ($volume_mounted_secrets)"
    else
        log_warn "⚠️ No volume-mounted secrets found"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for default service account tokens
    log_security "Checking service account token management..."
    default_tokens=$(kubectl get secrets --all-namespaces --no-headers | grep "default-token" | wc -l)
    if [ "$default_tokens" -gt 3 ]; then
        log_warn "⚠️ Multiple default service account tokens present ($default_tokens)"
        add_security_issue "secrets_management" "excessive_default_tokens" "low" "Many default service account tokens present"
    else
        log_pass "✅ Service account tokens properly managed"
    fi
}

# Container Security Validation
validate_container_security() {
    log_security "Validating container security..."
    
    ((TOTAL_CHECKS++))
    # Check for container image tags
    log_security "Checking container image tag practices..."
    latest_tags=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[].image | select(test(":latest$|^[^:]+$"))] | length')
    if [ "$latest_tags" -gt 0 ]; then
        log_fail "❌ Containers using 'latest' or untagged images ($latest_tags)" "medium"
        add_security_issue "container_security" "latest_tags" "medium" "Containers using latest or untagged images"
    else
        log_pass "✅ All containers use specific image tags"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for resource limits
    log_security "Checking container resource limits..."
    containers_with_limits=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[] | select(.resources.limits)] | length')
    total_containers=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[]] | length')
    limits_percentage=$((containers_with_limits * 100 / total_containers))
    if [ "$limits_percentage" -lt 80 ]; then
        log_fail "❌ Low resource limits adoption ($limits_percentage%)" "medium"
        add_security_issue "container_security" "missing_resource_limits" "medium" "Many containers lack resource limits"
    else
        log_pass "✅ Good resource limits adoption ($limits_percentage%)"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for image pull policies
    log_security "Checking image pull policies..."
    always_pull_policy=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.containers[] | select(.imagePullPolicy == "Always")] | length')
    if [ "$always_pull_policy" -lt "$((total_containers / 2))" ]; then
        log_warn "⚠️ Few containers using 'Always' image pull policy"
        add_security_issue "container_security" "pull_policy" "low" "Containers not using 'Always' image pull policy"
    else
        log_pass "✅ Good image pull policy adoption"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for init containers
    log_security "Checking init container security..."
    init_containers=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.initContainers[]?] | length')
    if [ "$init_containers" -gt 0 ]; then
        privileged_init=$(kubectl get pods --all-namespaces -o json | jq '[.items[].spec.initContainers[]? | select(.securityContext.privileged == true)] | length')
        if [ "$privileged_init" -gt 0 ]; then
            log_fail "❌ Privileged init containers detected ($privileged_init)" "high"
            add_security_issue "container_security" "privileged_init" "high" "Init containers running with privileged access"
        else
            log_pass "✅ Init containers properly secured"
        fi
    else
        log_pass "✅ No init containers to validate"
    fi
}

# Compliance Validation
validate_compliance() {
    log_security "Validating compliance standards..."
    
    ((TOTAL_CHECKS++))
    # Check for pod security policies/standards
    log_security "Checking pod security standards compliance..."
    if kubectl get podsecuritypolicies 2>/dev/null | grep -q "NAME" || kubectl get validatingadmissionpolicies 2>/dev/null | grep -q "NAME"; then
        log_pass "✅ Pod security enforcement mechanisms active"
    else
        log_fail "❌ No pod security enforcement found" "medium"
        add_security_issue "compliance" "no_pod_security_enforcement" "medium" "Pod Security Policies or admission controllers not configured"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for audit logging
    log_security "Checking audit logging configuration..."
    if kubectl get events --all-namespaces | grep -q "audit" || [ -f "/var/log/audit/audit.log" ]; then
        log_pass "✅ Audit logging appears to be configured"
    else
        log_warn "⚠️ Audit logging configuration unclear"
        add_security_issue "compliance" "audit_logging" "low" "Audit logging configuration not verified"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for namespace isolation
    log_security "Checking namespace isolation..."
    namespaces_count=$(kubectl get namespaces --no-headers | wc -l)
    if [ "$namespaces_count" -lt 3 ]; then
        log_warn "⚠️ Limited namespace isolation ($namespaces_count namespaces)"
        add_security_issue "compliance" "limited_namespaces" "low" "Limited use of namespaces for isolation"
    else
        log_pass "✅ Good namespace isolation ($namespaces_count namespaces)"
    fi
    
    ((TOTAL_CHECKS++))
    # Check for monitoring and logging
    log_security "Checking monitoring and logging infrastructure..."
    monitoring_pods=$(kubectl get pods --all-namespaces | grep -E "(prometheus|grafana|fluentd|logstash|elasticsearch)" | wc -l)
    if [ "$monitoring_pods" -eq 0 ]; then
        log_warn "⚠️ No dedicated monitoring/logging infrastructure detected"
        add_security_issue "compliance" "no_monitoring" "low" "No monitoring or logging infrastructure detected"
    else
        log_pass "✅ Monitoring/logging infrastructure present ($monitoring_pods components)"
    fi
}

# Add security issue to report
add_security_issue() {
    local category="$1"
    local issue_id="$2"
    local severity="$3"
    local description="$4"
    
    # Add to JSON report
    jq --arg cat "$category" --arg id "$issue_id" --arg sev "$severity" --arg desc "$description" \
       '.security_validation.categories[$cat].issues += [{
            "id": $id,
            "severity": $sev,
            "description": $desc,
            "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
        }]' "${RESULTS_DIR}/security_report_${TIMESTAMP}.json" > tmp.json && \
        mv tmp.json "${RESULTS_DIR}/security_report_${TIMESTAMP}.json"
}

# Generate security report
generate_security_report() {
    log_security "Generating comprehensive security report..."
    
    # Calculate security score
    local security_score=0
    if [ "$TOTAL_CHECKS" -gt 0 ]; then
        security_score=$((PASSED_CHECKS * 100 / TOTAL_CHECKS))
    fi
    
    # Determine overall status
    local overall_status="failed"
    if [ "$HIGH_SEVERITY_ISSUES" -eq 0 ] && [ "$security_score" -ge 80 ]; then
        overall_status="passed"
    elif [ "$HIGH_SEVERITY_ISSUES" -le 1 ] && [ "$security_score" -ge 70 ]; then
        overall_status="warning"
    fi
    
    # Update final report
    jq --arg status "$overall_status" \
       --argjson score "$security_score" \
       --argjson total "$TOTAL_CHECKS" \
       --argjson passed "$PASSED_CHECKS" \
       --argjson failed "$FAILED_CHECKS" \
       --argjson high "$HIGH_SEVERITY_ISSUES" \
       --argjson medium "$MEDIUM_SEVERITY_ISSUES" \
       --argjson low "$LOW_SEVERITY_ISSUES" \
       '.security_validation.overall_status = $status |
        .security_validation.security_score = $score |
        .security_validation.summary = {
            "total_checks": $total,
            "passed_checks": $passed,
            "failed_checks": $failed,
            "high_severity_issues": $high,
            "medium_severity_issues": $medium,
            "low_severity_issues": $low
        } |
        .security_validation.completion_time = "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"' \
        "${RESULTS_DIR}/security_report_${TIMESTAMP}.json" > tmp.json && \
        mv tmp.json "${RESULTS_DIR}/security_report_${TIMESTAMP}.json"
    
    # Generate markdown report
    generate_markdown_security_report "$security_score" "$overall_status"
    
    log_security "Security validation report generated: ${RESULTS_DIR}/security_report_${TIMESTAMP}.json"
    
    # Display summary
    echo
    echo "=========================================="
    echo "SECURITY VALIDATION SUMMARY"
    echo "=========================================="
    echo "Security Score: ${security_score}/100"
    echo "Total Checks: $TOTAL_CHECKS"
    echo "Passed: $PASSED_CHECKS"
    echo "Failed: $FAILED_CHECKS"
    echo "High Severity Issues: $HIGH_SEVERITY_ISSUES"
    echo "Medium Severity Issues: $MEDIUM_SEVERITY_ISSUES"
    echo "Low Severity Issues: $LOW_SEVERITY_ISSUES"
    echo "Overall Status: $overall_status"
    echo "=========================================="
}

# Generate markdown security report
generate_markdown_security_report() {
    local score="$1"
    local status="$2"
    
    cat > "${RESULTS_DIR}/security_assessment_${TIMESTAMP}.md" << EOF
# Security Compliance Assessment Report

**Date**: $(date)  
**Version**: 1.0.0  
**Environment**: Production Simulation  
**Security Score**: ${score}/100  
**Overall Status**: ${status^^}

## Executive Summary

This security assessment validates the GitOps pipeline implementation against enterprise security standards and compliance requirements.

### Security Metrics
- **Total Security Checks**: $TOTAL_CHECKS
- **Passed Checks**: $PASSED_CHECKS
- **Failed Checks**: $FAILED_CHECKS
- **Security Score**: ${score}/100

### Issue Severity Breakdown
- **High Severity**: $HIGH_SEVERITY_ISSUES issues
- **Medium Severity**: $MEDIUM_SEVERITY_ISSUES issues
- **Low Severity**: $LOW_SEVERITY_ISSUES issues

## Security Categories Assessment

### 1. RBAC Security
- Service account configuration validated
- Role-based access controls verified
- Permission escalation risks assessed

### 2. Network Security
- Network policy implementation checked
- Traffic isolation validated
- External access exposure reviewed

### 3. Pod Security
- Container privilege escalation prevention
- Security context configuration
- Runtime security controls

### 4. Secrets Management
- Secret exposure vulnerabilities
- Proper secret mounting practices
- Service account token management

### 5. Container Security
- Image security best practices
- Resource limit enforcement
- Pull policy configuration

### 6. Compliance Standards
- Pod security policy enforcement
- Audit logging configuration
- Namespace isolation practices

## Recommendations

Based on the security assessment, the following recommendations are provided:

1. **Address High Severity Issues**: Immediately resolve all high severity security issues
2. **Implement Missing Controls**: Add missing security controls identified in the assessment
3. **Regular Security Scanning**: Establish regular security validation cycles
4. **Monitoring Enhancement**: Improve security monitoring and alerting capabilities

## Compliance Status

$(if [ "$HIGH_SEVERITY_ISSUES" -eq 0 ] && [ "$score" -ge 80 ]; then
    echo "✅ **COMPLIANT**: System meets enterprise security standards"
elif [ "$HIGH_SEVERITY_ISSUES" -le 1 ] && [ "$score" -ge 70 ]; then
    echo "⚠️ **CONDITIONAL**: System meets basic security requirements with recommendations"
else
    echo "❌ **NON-COMPLIANT**: System requires security improvements before production deployment"
fi)

---

**Assessment Completed By**: Security Compliance Validator  
**Report Generated**: $(date -u +%Y-%m-%dT%H:%M:%SZ)  
**Next Review**: $(date -d "+3 months" +%Y-%m-%d)
EOF
    
    log_security "Markdown security report generated: ${RESULTS_DIR}/security_assessment_${TIMESTAMP}.md"
}

# Main execution function
main() {
    echo "🔒 Starting Security Compliance Validation"
    echo "=========================================="
    
    # Initialize security validation
    initialize_security_validation
    
    # Execute all security validation categories
    validate_rbac_security
    validate_network_security
    validate_pod_security
    validate_secrets_management
    validate_container_security
    validate_compliance
    
    # Generate comprehensive security report
    generate_security_report
    
    # Return appropriate exit code
    if [ "$HIGH_SEVERITY_ISSUES" -eq 0 ] && [ "$PASSED_CHECKS" -ge "$((TOTAL_CHECKS * 80 / 100))" ]; then
        log_security "🎉 Security validation passed! System meets enterprise security standards."
        exit 0
    elif [ "$HIGH_SEVERITY_ISSUES" -le 1 ] && [ "$PASSED_CHECKS" -ge "$((TOTAL_CHECKS * 70 / 100))" ]; then
        log_security "⚠️ Security validation completed with warnings. Review recommendations."
        exit 1
    else
        log_security "💥 Security validation failed! Critical security issues must be addressed."
        exit 2
    fi
}

# Cleanup function
cleanup() {
    log_security "Cleaning up security validation environment..."
}

trap cleanup EXIT

# Execute main function
main "$@"