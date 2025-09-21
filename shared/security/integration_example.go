package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"shared-config/config"
	"shared-config/security"
)

// Example integration showing how to use all security components together
func main() {
	// Initialize logger (implement your own)
	logger := &ConsoleLogger{}

	// Load and validate configuration securely
	if err := secureConfigurationExample(logger); err != nil {
		log.Fatal("Configuration security check failed:", err)
	}

	// Start secure web server
	if err := secureWebServerExample(logger); err != nil {
		log.Fatal("Failed to start secure web server:", err)
	}
}

// secureConfigurationExample demonstrates secure configuration loading and validation
func secureConfigurationExample(logger security.Logger) error {
	logger.Info("Starting secure configuration example", nil)

	// Initialize security manager with default configuration
	securityConfig := security.DefaultSecurityConfig()
	securityManager, err := security.NewSecurityManager(securityConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize security manager: %v", err)
	}

	// Load shared configuration
	configLoader := config.NewConfigLoader("config/demo-config.yaml")
	sharedConfig, err := configLoader.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %v", err)
	}

	// Validate configuration for security issues
	validationResult, err := securityManager.ValidateSharedConfig(sharedConfig)
	if err != nil {
		return fmt.Errorf("failed to validate configuration: %v", err)
	}

	// Handle validation results
	if validationResult.OverallStatus == "fail" {
		logger.Error("Configuration validation failed", map[string]interface{}{
			"issues": len(validationResult.Issues),
		})
		
		// Log each security issue
		for _, issue := range validationResult.Issues {
			logger.Error("Security issue found", map[string]interface{}{
				"type":        issue.Type,
				"severity":    issue.Severity,
				"description": issue.Description,
				"location":    issue.Location,
				"remediation": issue.Remediation,
			})
		}
		return fmt.Errorf("configuration has %d security issues", len(validationResult.Issues))
	}

	logger.Info("Configuration validation successful", map[string]interface{}{
		"status":         validationResult.OverallStatus,
		"warnings":       len(validationResult.Warnings),
		"recommendations": len(validationResult.Recommendations),
	})

	// Demonstrate secrets management
	if err := demonstrateSecretsManagement(securityManager, logger); err != nil {
		return fmt.Errorf("secrets management demo failed: %v", err)
	}

	// Perform vulnerability scan
	if err := performSecurityScan(securityManager, logger); err != nil {
		return fmt.Errorf("security scan failed: %v", err)
	}

	return nil
}

// demonstrateSecretsManagement shows how to use the secrets management system
func demonstrateSecretsManagement(securityManager *security.SecurityManager, logger security.Logger) error {
	secretsManager := securityManager.GetSecretsManager()
	if secretsManager == nil {
		return fmt.Errorf("secrets manager not available")
	}

	ctx := context.Background()

	// Store a secret
	secret := &security.Secret{
		Name:  "database_password",
		Type:  security.SecretTypePassword,
		Value: "super_secure_password_123!",
		Tags:  []string{"production", "database"},
		Metadata: map[string]string{
			"environment": "production",
			"service":     "backup-system",
		},
	}

	if err := secretsManager.StoreSecret(ctx, secret); err != nil {
		return fmt.Errorf("failed to store secret: %v", err)
	}

	logger.Info("Secret stored successfully", map[string]interface{}{
		"secret_id": secret.ID,
		"name":      secret.Name,
	})

	// Retrieve the secret
	retrievedSecret, err := secretsManager.GetSecret(ctx, secret.ID)
	if err != nil {
		return fmt.Errorf("failed to retrieve secret: %v", err)
	}

	logger.Info("Secret retrieved successfully", map[string]interface{}{
		"secret_id": retrievedSecret.ID,
		"name":      retrievedSecret.Name,
		"version":   retrievedSecret.Version,
	})

	// Scan for exposed secrets in a file
	scanResult, err := secretsManager.ScanFile("config/demo-config.yaml")
	if err != nil {
		logger.Warn("Secret scan failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		logger.Info("Secret scan completed", map[string]interface{}{
			"matches_found": len(scanResult.Matches),
			"file_path":     scanResult.FilePath,
		})

		for _, match := range scanResult.Matches {
			logger.Warn("Potential secret found", map[string]interface{}{
				"type":       match.Type,
				"line":       match.Line,
				"confidence": match.Confidence,
				"redacted":   match.Redacted,
			})
		}
	}

	return nil
}

// performSecurityScan demonstrates vulnerability scanning
func performSecurityScan(securityManager *security.SecurityManager, logger security.Logger) error {
	scanner := securityManager.GetVulnerabilityScanner()
	if scanner == nil {
		return fmt.Errorf("vulnerability scanner not available")
	}

	ctx := context.Background()

	logger.Info("Starting security scan", nil)

	// Perform comprehensive scan
	scanResult, err := securityManager.PerformSecurityScan(ctx)
	if err != nil {
		return fmt.Errorf("security scan failed: %v", err)
	}

	logger.Info("Security scan completed", map[string]interface{}{
		"scan_id":        scanResult.ScanID,
		"duration":       scanResult.Duration,
		"total_vulns":    scanResult.Summary.TotalVulns,
		"critical_count": scanResult.Summary.CriticalCount,
		"high_count":     scanResult.Summary.HighCount,
		"medium_count":   scanResult.Summary.MediumCount,
	})

	// Log vulnerabilities found
	for _, vuln := range scanResult.Vulnerabilities {
		logger.Warn("Vulnerability found", map[string]interface{}{
			"id":          vuln.ID,
			"type":        string(vuln.Type),
			"level":       string(vuln.Level),
			"title":       vuln.Title,
			"location":    vuln.Location.FilePath,
			"line":        vuln.Location.LineNumber,
			"confidence":  vuln.Confidence,
		})
	}

	return nil
}

// secureWebServerExample demonstrates secure web server with all security features
func secureWebServerExample(logger security.Logger) error {
	// Initialize security manager
	securityConfig := security.DefaultSecurityConfig()
	securityManager, err := security.NewSecurityManager(securityConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize security manager: %v", err)
	}

	// Create secure HTTP client using TLS manager
	tlsManager := securityManager.GetTLSManager()
	var httpClient *http.Client
	if tlsManager != nil {
		httpClient = tlsManager.GetHTTPClient()
	} else {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Create HTTP handlers with security integration
	http.HandleFunc("/webhook/backup-complete", func(w http.ResponseWriter, r *http.Request) {
		handleSecureWebhook(w, r, securityManager, logger)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handleHealthCheck(w, r, securityManager, logger)
	})

	http.HandleFunc("/security/status", func(w http.ResponseWriter, r *http.Request) {
		handleSecurityStatus(w, r, securityManager, logger)
	})

	logger.Info("Starting secure web server", map[string]interface{}{
		"address":     "0.0.0.0:8443",
		"tls_enabled": tlsManager != nil,
	})

	// Start HTTPS server with TLS configuration
	if tlsManager != nil {
		tlsConfig := tlsManager.GetTLSConfig()
		server := &http.Server{
			Addr:      ":8443",
			TLSConfig: tlsConfig,
		}
		return server.ListenAndServeTLS("", "")
	} else {
		// Fallback to HTTP (not recommended for production)
		return http.ListenAndServe(":8080", nil)
	}
}

// handleSecureWebhook demonstrates secure webhook processing
func handleSecureWebhook(w http.ResponseWriter, r *http.Request, securityManager *security.SecurityManager, logger security.Logger) {
	startTime := time.Now()

	// Create webhook request
	webhookRequest := &security.WebhookRequest{
		RequestID: fmt.Sprintf("req_%d", time.Now().Unix()),
		Method:    r.Method,
		Endpoint:  r.URL.Path,
		Headers:   make(map[string]string),
		SourceIP:  getClientIP(r),
		UserAgent: r.UserAgent(),
	}

	// Copy headers
	for key, values := range r.Header {
		if len(values) > 0 {
			webhookRequest.Headers[key] = values[0]
		}
	}

	// Read body
	if r.Body != nil {
		defer r.Body.Close()
		// In production, limit body size
		// webhookRequest.Body, _ = io.ReadAll(io.LimitReader(r.Body, 1024*1024)) // 1MB limit
	}

	// Process request securely
	response, err := securityManager.SecureWebhookRequest(r.Context(), webhookRequest)
	if err != nil {
		logger.Error("Secure webhook processing failed", map[string]interface{}{
			"error":      err.Error(),
			"request_id": webhookRequest.RequestID,
			"source_ip":  webhookRequest.SourceIP,
			"duration":   time.Since(startTime),
		})

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Send successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	responseData := map[string]interface{}{
		"status":     response.Status,
		"message":    "Webhook processed successfully",
		"request_id": response.RequestID,
		"timestamp":  response.Timestamp,
	}

	json.NewEncoder(w).Encode(responseData)

	logger.Info("Webhook processed successfully", map[string]interface{}{
		"request_id": webhookRequest.RequestID,
		"source_ip":  webhookRequest.SourceIP,
		"duration":   time.Since(startTime),
	})
}

// handleHealthCheck provides security-aware health check
func handleHealthCheck(w http.ResponseWriter, r *http.Request, securityManager *security.SecurityManager, logger security.Logger) {
	status := securityManager.GetSecurityStatus()

	w.Header().Set("Content-Type", "application/json")
	
	if status.OverallStatus == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	healthResponse := map[string]interface{}{
		"status":    status.OverallStatus,
		"timestamp": time.Now(),
		"components": map[string]interface{}{
			"secrets_management":      status.SecretsManagement.Status,
			"authentication":          status.Authentication.Status,
			"tls":                     status.TLS.Status,
			"vulnerability_scanning":  status.VulnerabilityScanning.Status,
			"audit_logging":           status.AuditLogging.Status,
		},
		"security_score": calculateSecurityScore(status),
	}

	json.NewEncoder(w).Encode(healthResponse)
}

// handleSecurityStatus provides detailed security status
func handleSecurityStatus(w http.ResponseWriter, r *http.Request, securityManager *security.SecurityManager, logger security.Logger) {
	// This endpoint should be protected - demonstrate authentication
	authManager := securityManager.GetAuthManager()
	if authManager != nil {
		// Extract session or API key from request
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID != "" {
			session, err := authManager.ValidateSession(sessionID)
			if err != nil {
				http.Error(w, "Invalid session", http.StatusUnauthorized)
				return
			}

			// Check authorization
			if err := authManager.Authorize(session, security.ResourceSystem, security.PermissionRead); err != nil {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}
		} else {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
	}

	status := securityManager.GetSecurityStatus()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// Helper functions

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.Split(xff, ",")[0]
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

func calculateSecurityScore(status *security.SecurityStatus) float64 {
	score := 0.0
	components := 0

	if status.SecretsManagement.Enabled && status.SecretsManagement.Health == "good" {
		score += 20
	}
	components++

	if status.Authentication.Enabled && status.Authentication.Health == "good" {
		score += 25
	}
	components++

	if status.TLS.Enabled && status.TLS.Health == "good" {
		score += 20
	}
	components++

	if status.VulnerabilityScanning.Enabled && status.VulnerabilityScanning.Health == "good" {
		score += 20
	}
	components++

	if status.AuditLogging.Enabled && status.AuditLogging.Health == "good" {
		score += 15
	}
	components++

	return score
}

// ConsoleLogger implements the Logger interface for demonstration
type ConsoleLogger struct{}

func (l *ConsoleLogger) Info(message string, fields map[string]interface{}) {
	log.Printf("[INFO] %s %v", message, fields)
}

func (l *ConsoleLogger) Error(message string, fields map[string]interface{}) {
	log.Printf("[ERROR] %s %v", message, fields)
}

func (l *ConsoleLogger) Warn(message string, fields map[string]interface{}) {
	log.Printf("[WARN] %s %v", message, fields)
}

func (l *ConsoleLogger) Debug(message string, fields map[string]interface{}) {
	log.Printf("[DEBUG] %s %v", message, fields)
}