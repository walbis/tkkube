# Security Framework Implementation Guide

## Overview

This guide provides step-by-step instructions for implementing the comprehensive security framework across all components of the Kubernetes backup and GitOps system. The implementation follows a defense-in-depth approach with authentication, authorization, encryption, input validation, and comprehensive monitoring.

## Quick Start - Production Deployment

### 1. Environment Setup

```bash
# Set up environment variables
export CLUSTER_NAME="production"
export MINIO_ENDPOINT="https://minio.production.local:9000"
export MINIO_ACCESS_KEY="$(vault kv get -field=access_key secret/minio/production)"
export MINIO_SECRET_KEY="$(vault kv get -field=secret_key secret/minio/production)"
export GITOPS_REPO_URL="https://github.com/org/gitops-production"
export WEBHOOK_API_KEY="$(vault kv get -field=api_key secret/webhook/production)"

# TLS certificates
export TLS_CERT_PATH="/etc/ssl/certs/backup-gitops.crt"
export TLS_KEY_PATH="/etc/ssl/private/backup-gitops.key"
export TLS_CA_PATH="/etc/ssl/certs/ca.crt"

# Vault configuration
export VAULT_ADDR="https://vault.production.local:8200"
export VAULT_TOKEN="$(cat /etc/vault/token)"
```

### 2. Deploy Secure Configuration

```bash
# Copy the secure configuration template
cp config/secure-config.yaml config/production-config.yaml

# Validate configuration security
go run security/integration_example.go --validate-config production-config.yaml

# Deploy configuration
kubectl create configmap backup-gitops-config \
  --from-file=config=production-config.yaml \
  --namespace=backup-gitops
```

### 3. Deploy Components with Security

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: secure-integration-bridge
  namespace: backup-gitops
spec:
  replicas: 1
  selector:
    matchLabels:
      app: secure-integration-bridge
  template:
    metadata:
      labels:
        app: secure-integration-bridge
    spec:
      serviceAccountName: backup-gitops
      containers:
      - name: bridge
        image: secure-integration-bridge:2.0.0
        ports:
        - containerPort: 8443
        env:
        - name: CONFIG_PATH
          value: /etc/config/production-config.yaml
        - name: WEBHOOK_API_KEY
          valueFrom:
            secretKeyRef:
              name: webhook-secrets
              key: api-key
        volumeMounts:
        - name: config
          mountPath: /etc/config
        - name: tls-certs
          mountPath: /etc/ssl/certs
        - name: vault-token
          mountPath: /etc/vault
        livenessProbe:
          httpGet:
            path: /health
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: backup-gitops-config
      - name: tls-certs
        secret:
          secretName: tls-certificates
      - name: vault-token
        secret:
          secretName: vault-token
```

## Component Integration

### 1. Go Backup Tool Integration

#### Enhanced HTTP Client

```go
package main

import (
    "context"
    "log"
    
    "shared-config/config"
    "shared-config/security"
    "shared-config/integration"
)

func main() {
    // Load secure configuration
    configLoader := config.NewConfigLoader("config/production-config.yaml")
    sharedConfig, err := configLoader.Load()
    if err != nil {
        log.Fatal("Failed to load configuration:", err)
    }

    // Initialize security manager
    securityConfig := security.DefaultSecurityConfig()
    securityManager, err := security.NewSecurityManager(securityConfig, logger)
    if err != nil {
        log.Fatal("Failed to initialize security:", err)
    }

    // Create secure integration bridge client
    secureClient := integration.NewSecureBackupClient(sharedConfig, securityManager)
    
    // Register with bridge securely
    ctx := context.Background()
    if err := secureClient.RegisterWithBridge(ctx, "https://integration-bridge:8443", "1.0.0"); err != nil {
        log.Fatal("Failed to register with bridge:", err)
    }

    // Perform backup with security validation
    backupResult, err := performSecureBackup(ctx, secureClient)
    if err != nil {
        log.Fatal("Backup failed:", err)
    }

    // Notify completion securely
    if err := secureClient.NotifyCompletion(ctx, backupResult); err != nil {
        log.Error("Failed to notify completion:", err)
    }
}

func performSecureBackup(ctx context.Context, client *integration.SecureBackupClient) (*BackupResult, error) {
    // Implementation with security validation
    return nil, nil
}
```

#### Secure Webhook Notifications

```go
// Enhanced backup completion notification
func (bc *BackupClient) NotifySecureCompletion(ctx context.Context, event *BackupCompletionEvent) error {
    // Create secure webhook request
    webhookRequest := &security.WebhookRequest{
        RequestID: fmt.Sprintf("backup-%s", event.BackupID),
        Method:    "POST",
        Endpoint:  "/webhooks/backup/completed",
        Headers: map[string]string{
            "Content-Type": "application/json",
            "X-API-Key":    bc.config.Security.APIKey,
            "User-Agent":   "BackupTool/1.0",
        },
        Body: marshalBackupEvent(event),
    }

    // Process through security manager
    response, err := bc.securityManager.SecureWebhookRequest(ctx, webhookRequest)
    if err != nil {
        return fmt.Errorf("secure webhook failed: %v", err)
    }

    log.Printf("Backup completion notified securely: %s", response.RequestID)
    return nil
}
```

### 2. Python GitOps Generator Integration

#### Secure Flask Application

```python
#!/usr/bin/env python3
"""
Secure GitOps Generator with Security Framework Integration
"""

from flask import Flask, request, jsonify
import asyncio
from security.python_security import (
    PythonSecurityManager, create_security_config_from_dict,
    SecurityError, AuthenticationError, AuthorizationError
)
from integration.secure_gitops_client import SecureGitOpsClient

app = Flask(__name__)

# Load configuration
with open('config/production-config.yaml', 'r') as f:
    config = yaml.safe_load(f)

# Initialize security
security_config = create_security_config_from_dict(config)
security_manager = PythonSecurityManager(security_config)

@app.before_request
async def security_middleware():
    """Apply security middleware to all requests"""
    try:
        # Extract request information
        headers = dict(request.headers)
        client_ip = request.remote_addr
        method = request.method
        endpoint = request.endpoint
        
        # Get request body
        body = request.get_data() if request.is_json else None
        
        # Authenticate request
        auth_context = await security_manager.authenticate_request(headers, client_ip)
        
        # Validate request
        validated_request = await security_manager.validate_request(
            method, endpoint, headers, body, client_ip
        )
        
        # Store security context
        request.auth_context = auth_context
        request.validated_request = validated_request
        
    except SecurityError as e:
        return jsonify({
            "error": e.message,
            "error_type": e.error_type,
            "timestamp": datetime.utcnow().isoformat()
        }), e.status_code

@app.route('/api/gitops/generate', methods=['POST'])
async def generate_gitops():
    """Generate GitOps manifests securely"""
    try:
        # Check authorization
        security_manager.authorize_request(request.auth_context, "gitops_generate")
        
        # Process request
        gitops_request = request.get_json()
        
        # Create secure GitOps client
        async with SecureGitOpsClient(config) as client:
            # Process the generation request
            result = await client.start_secure_gitops_generation(gitops_request)
            
            return jsonify({
                "success": True,
                "request_id": result.request_id,
                "status": result.status,
                "security_context": {
                    "authenticated": True,
                    "client_id": request.auth_context.client_id
                }
            })
            
    except SecurityError as e:
        return jsonify({
            "error": e.message,
            "error_type": e.error_type
        }), e.status_code

@app.route('/health', methods=['GET'])
def health_check():
    """Secure health check endpoint"""
    return jsonify({
        "status": "healthy",
        "security": {
            "authentication_enabled": True,
            "tls_enabled": security_config.tls_enabled,
            "audit_enabled": security_config.audit_enabled
        },
        "timestamp": datetime.utcnow().isoformat()
    })

if __name__ == '__main__':
    # Run with TLS
    app.run(
        host='0.0.0.0',
        port=8443,
        ssl_context=(
            security_config.tls_cert_path,
            security_config.tls_key_path
        ) if security_config.tls_enabled else None
    )
```

### 3. Integration Bridge Deployment

#### Secure Bridge Implementation

```go
package main

import (
    "context"
    "log"
    "shared-config/integration"
    "shared-config/config"
)

func main() {
    // Load configuration
    configLoader := config.NewConfigLoader("config/production-config.yaml")
    sharedConfig, err := configLoader.Load()
    if err != nil {
        log.Fatal("Failed to load config:", err)
    }

    // Create secure integration bridge
    bridge, err := integration.NewSecureIntegrationBridge(sharedConfig)
    if err != nil {
        log.Fatal("Failed to create secure bridge:", err)
    }

    // Start the bridge
    ctx := context.Background()
    if err := bridge.Start(ctx); err != nil {
        log.Fatal("Failed to start secure bridge:", err)
    }

    log.Println("Secure integration bridge running on port 8443")
    
    // Graceful shutdown
    select {}
}
```

## Security Monitoring Integration

### 1. Metrics Collection

```go
// Enhanced security metrics
func (sib *SecureIntegrationBridge) collectSecurityMetrics() {
    metrics := sib.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
    
    // Authentication metrics
    metrics.SetGauge("security_authentication_success_rate", 
        map[string]string{"component": "integration-bridge"},
        sib.calculateAuthSuccessRate())
    
    // TLS certificate expiry
    metrics.SetGauge("security_certificate_days_until_expiry",
        map[string]string{"component": "integration-bridge"},
        sib.getCertificateExpiry().Sub(time.Now()).Hours()/24)
    
    // Vulnerability scan results
    if scanner := sib.securityManager.GetVulnerabilityScanner(); scanner != nil {
        latestScan := scanner.GetLatestScan()
        if latestScan != nil {
            metrics.SetGauge("security_vulnerabilities_critical",
                map[string]string{"component": "integration-bridge"},
                float64(latestScan.Summary.CriticalCount))
        }
    }
    
    // Security score
    metrics.SetGauge("security_score",
        map[string]string{"component": "integration-bridge"},
        sib.calculateSecurityScore())
}
```

### 2. Alerting Rules

```yaml
# prometheus-alerts.yaml
groups:
- name: security
  rules:
  - alert: SecurityVulnerabilityDetected
    expr: security_vulnerabilities_critical > 0
    for: 0m
    labels:
      severity: critical
    annotations:
      summary: "Critical security vulnerabilities detected"
      description: "{{ $value }} critical vulnerabilities found in security scan"

  - alert: CertificateExpiringSoon
    expr: security_certificate_days_until_expiry < 7
    for: 0m
    labels:
      severity: warning
    annotations:
      summary: "TLS certificate expiring soon"
      description: "Certificate expires in {{ $value }} days"

  - alert: AuthenticationFailureSpike
    expr: rate(security_authentication_failures_total[5m]) > 10
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High authentication failure rate"
      description: "Authentication failure rate is {{ $value }} per second"

  - alert: SecurityScoreLow
    expr: security_score < 80
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Security score below threshold"
      description: "Security score is {{ $value }}, below 80 threshold"
```

### 3. Grafana Dashboard

```json
{
  "dashboard": {
    "title": "Security Monitoring Dashboard",
    "panels": [
      {
        "title": "Security Score",
        "type": "gauge",
        "targets": [
          {
            "expr": "security_score",
            "legendFormat": "Security Score"
          }
        ],
        "fieldConfig": {
          "min": 0,
          "max": 100,
          "thresholds": [
            {"color": "red", "value": 0},
            {"color": "yellow", "value": 70},
            {"color": "green", "value": 90}
          ]
        }
      },
      {
        "title": "Authentication Success Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "security_authentication_success_rate * 100",
            "legendFormat": "Success Rate %"
          }
        ]
      },
      {
        "title": "Certificate Expiry",
        "type": "stat",
        "targets": [
          {
            "expr": "security_certificate_days_until_expiry",
            "legendFormat": "Days Until Expiry"
          }
        ]
      },
      {
        "title": "Security Events",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(security_events_total[5m])",
            "legendFormat": "{{event_type}}"
          }
        ]
      }
    ]
  }
}
```

## Testing and Validation

### 1. Security Testing

```bash
#!/bin/bash
# security-test.sh

echo "Running security validation tests..."

# Test 1: Configuration security scan
echo "1. Testing configuration security..."
go run security/integration_example.go --validate-config config/production-config.yaml

# Test 2: Webhook authentication
echo "2. Testing webhook authentication..."
curl -X POST https://integration-bridge:8443/webhooks/backup/completed \
  -H "Content-Type: application/json" \
  -H "X-API-Key: invalid-key" \
  -d '{"backup_id": "test"}' \
  --insecure || echo "✓ Authentication working - rejected invalid key"

curl -X POST https://integration-bridge:8443/webhooks/backup/completed \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $WEBHOOK_API_KEY" \
  -d '{"backup_id": "test", "cluster_name": "test", "success": true, "minio_path": "test/path"}' \
  --insecure && echo "✓ Authentication working - accepted valid key"

# Test 3: TLS configuration
echo "3. Testing TLS configuration..."
openssl s_client -connect integration-bridge:8443 -verify_return_error -brief

# Test 4: Input validation
echo "4. Testing input validation..."
curl -X POST https://integration-bridge:8443/webhooks/backup/completed \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $WEBHOOK_API_KEY" \
  -d '{"backup_id": "<script>alert(1)</script>"}' \
  --insecure || echo "✓ Input validation working - rejected XSS attempt"

# Test 5: Rate limiting
echo "5. Testing rate limiting..."
for i in {1..110}; do
  curl -X GET https://integration-bridge:8443/health --insecure &
done
wait
echo "✓ Rate limiting test completed"

echo "Security validation tests completed"
```

### 2. Penetration Testing

```python
#!/usr/bin/env python3
"""
Security penetration testing script
"""

import asyncio
import aiohttp
import json
import ssl
from datetime import datetime

class SecurityPenTest:
    def __init__(self, base_url, api_key):
        self.base_url = base_url
        self.api_key = api_key
        self.results = []
    
    async def run_all_tests(self):
        """Run comprehensive penetration tests"""
        print("Starting security penetration tests...")
        
        # Test authentication bypass
        await self.test_auth_bypass()
        
        # Test input validation bypass
        await self.test_input_validation()
        
        # Test rate limiting
        await self.test_rate_limiting()
        
        # Test TLS configuration
        await self.test_tls_security()
        
        # Generate report
        self.generate_report()
    
    async def test_auth_bypass(self):
        """Test authentication bypass attempts"""
        test_cases = [
            {"headers": {}, "expected": 401},
            {"headers": {"X-API-Key": "invalid"}, "expected": 401},
            {"headers": {"Authorization": "Bearer invalid"}, "expected": 401},
            {"headers": {"X-API-Key": self.api_key}, "expected": 200},
        ]
        
        for case in test_cases:
            async with aiohttp.ClientSession() as session:
                try:
                    async with session.get(
                        f"{self.base_url}/health",
                        headers=case["headers"],
                        ssl=False
                    ) as response:
                        if response.status == case["expected"]:
                            self.results.append({
                                "test": "auth_bypass",
                                "case": str(case["headers"]),
                                "status": "PASS",
                                "message": f"Expected {case['expected']}, got {response.status}"
                            })
                        else:
                            self.results.append({
                                "test": "auth_bypass",
                                "case": str(case["headers"]),
                                "status": "FAIL",
                                "message": f"Expected {case['expected']}, got {response.status}"
                            })
                except Exception as e:
                    self.results.append({
                        "test": "auth_bypass",
                        "case": str(case["headers"]),
                        "status": "ERROR",
                        "message": str(e)
                    })
    
    async def test_input_validation(self):
        """Test input validation bypass attempts"""
        malicious_payloads = [
            '{"backup_id": "<script>alert(1)</script>"}',
            '{"backup_id": "../../etc/passwd"}',
            '{"backup_id": "test\' OR 1=1--"}',
            '{"backup_id": "' + 'A' * 10000 + '"}',
        ]
        
        for payload in malicious_payloads:
            async with aiohttp.ClientSession() as session:
                try:
                    async with session.post(
                        f"{self.base_url}/webhooks/backup/completed",
                        headers={"X-API-Key": self.api_key, "Content-Type": "application/json"},
                        data=payload,
                        ssl=False
                    ) as response:
                        if response.status == 400:  # Expected validation error
                            self.results.append({
                                "test": "input_validation",
                                "payload": payload[:50] + "...",
                                "status": "PASS",
                                "message": "Malicious payload rejected"
                            })
                        else:
                            self.results.append({
                                "test": "input_validation",
                                "payload": payload[:50] + "...",
                                "status": "FAIL",
                                "message": f"Payload accepted with status {response.status}"
                            })
                except Exception as e:
                    self.results.append({
                        "test": "input_validation",
                        "payload": payload[:50] + "...",
                        "status": "ERROR",
                        "message": str(e)
                    })
    
    def generate_report(self):
        """Generate penetration test report"""
        print("\n" + "="*60)
        print("SECURITY PENETRATION TEST REPORT")
        print("="*60)
        print(f"Test Date: {datetime.now().isoformat()}")
        print(f"Target: {self.base_url}")
        print()
        
        passed = len([r for r in self.results if r["status"] == "PASS"])
        failed = len([r for r in self.results if r["status"] == "FAIL"])
        errors = len([r for r in self.results if r["status"] == "ERROR"])
        
        print(f"Total Tests: {len(self.results)}")
        print(f"Passed: {passed}")
        print(f"Failed: {failed}")
        print(f"Errors: {errors}")
        print()
        
        for result in self.results:
            status_icon = "✓" if result["status"] == "PASS" else "✗" if result["status"] == "FAIL" else "⚠"
            print(f"{status_icon} {result['test']}: {result['message']}")
        
        print("\n" + "="*60)

async def main():
    pen_test = SecurityPenTest(
        "https://integration-bridge:8443",
        "your-api-key-here"
    )
    await pen_test.run_all_tests()

if __name__ == "__main__":
    asyncio.run(main())
```

## Troubleshooting Guide

### Common Security Issues

#### 1. Authentication Failures

```bash
# Check API key configuration
kubectl get secret webhook-secrets -o yaml | base64 -d

# Verify webhook authentication
curl -v -X POST https://integration-bridge:8443/webhooks/backup/completed \
  -H "X-API-Key: $WEBHOOK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"test": "data"}'

# Check audit logs
kubectl logs deployment/secure-integration-bridge | grep "authentication"
```

#### 2. TLS Certificate Issues

```bash
# Check certificate validity
openssl x509 -in /etc/ssl/certs/backup-gitops.crt -text -noout

# Verify certificate chain
openssl verify -CAfile /etc/ssl/certs/ca.crt /etc/ssl/certs/backup-gitops.crt

# Test TLS connection
openssl s_client -connect integration-bridge:8443 -servername integration-bridge
```

#### 3. Rate Limiting Issues

```bash
# Check rate limiting configuration
kubectl get configmap backup-gitops-config -o yaml | grep -A 10 rate_limit

# Monitor rate limiting metrics
curl https://integration-bridge:9090/metrics | grep rate_limit

# Adjust rate limits if needed
kubectl patch configmap backup-gitops-config --patch '{"data":{"rate_limit_requests":"200"}}'
```

### Security Monitoring

#### 1. Check Security Metrics

```bash
# Security score
curl -s https://integration-bridge:9090/metrics | grep security_score

# Authentication success rate
curl -s https://integration-bridge:9090/metrics | grep authentication_success_rate

# Certificate expiry
curl -s https://integration-bridge:9090/metrics | grep certificate_days_until_expiry
```

#### 2. View Audit Logs

```bash
# Component audit logs
kubectl logs deployment/secure-integration-bridge | grep "audit"

# Security events
kubectl logs deployment/secure-integration-bridge | grep "security_event"

# Failed authentication attempts
kubectl logs deployment/secure-integration-bridge | grep "auth_failed"
```

## Maintenance and Updates

### 1. Certificate Rotation

```bash
#!/bin/bash
# rotate-certificates.sh

echo "Rotating TLS certificates..."

# Generate new certificate
openssl req -new -x509 -days 365 -key /etc/ssl/private/backup-gitops.key \
  -out /etc/ssl/certs/backup-gitops.crt \
  -subj "/CN=integration-bridge/O=backup-gitops"

# Update Kubernetes secret
kubectl create secret tls tls-certificates \
  --cert=/etc/ssl/certs/backup-gitops.crt \
  --key=/etc/ssl/private/backup-gitops.key \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart deployments to pick up new certificates
kubectl rollout restart deployment/secure-integration-bridge
kubectl rollout restart deployment/backup-tool
kubectl rollout restart deployment/gitops-generator

echo "Certificate rotation completed"
```

### 2. Security Updates

```bash
#!/bin/bash
# security-update.sh

echo "Applying security updates..."

# Update security configuration
kubectl patch configmap backup-gitops-config --patch-file security-update.yaml

# Update security policies
kubectl apply -f security-policies.yaml

# Run security scan
kubectl create job security-scan --from=cronjob/vulnerability-scanner

# Verify security score
curl -s https://integration-bridge:9090/metrics | grep security_score

echo "Security updates completed"
```

## Conclusion

This implementation guide provides a comprehensive security framework that:

- ✅ **Authenticates and authorizes** all webhook endpoints and inter-component communication
- ✅ **Encrypts communication** using TLS/mTLS with proper certificate management
- ✅ **Validates and sanitizes** all input data to prevent injection attacks
- ✅ **Implements rate limiting** to prevent DoS attacks
- ✅ **Provides comprehensive audit logging** for security monitoring
- ✅ **Integrates with monitoring** for real-time security metrics and alerting
- ✅ **Supports both Go and Python** components with consistent security patterns
- ✅ **Follows security best practices** including defense in depth and zero trust principles

The framework achieves enterprise-grade security while maintaining system performance and ease of deployment. Regular security testing and monitoring ensure continued protection against evolving threats.