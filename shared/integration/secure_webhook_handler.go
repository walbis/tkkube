package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"shared-config/security"
)

// SecureWebhookHandler handles incoming webhooks with comprehensive security
type SecureWebhookHandler struct {
	bridge          *IntegrationBridge
	server          *http.Server
	mux             *http.ServeMux
	securityManager *security.SecurityManager
	rateLimiter     *RateLimiter
	running         bool
	mu              sync.RWMutex
}

// RateLimiter implements request rate limiting
type RateLimiter struct {
	requests    map[string]*RequestCount
	mu          sync.RWMutex
	maxRequests int
	window      time.Duration
	cleanup     time.Duration
}

// RequestCount tracks requests per client
type RequestCount struct {
	Count     int
	Window    time.Time
	LastSeen  time.Time
}

// SecureWebhookRequest extends WebhookRequest with security context
type SecureWebhookRequest struct {
	*WebhookRequest
	SecurityContext *SecurityContext
	ValidationResult *ValidationResult
}

// SecurityContext holds security-related information
type SecurityContext struct {
	ClientID        string
	AuthMethod      string
	Permissions     []string
	TrustedClient   bool
	RateLimitStatus *RateLimitStatus
}

// ValidationResult holds input validation results
type ValidationResult struct {
	Valid        bool
	Errors       []string
	Warnings     []string
	SanitizedData map[string]interface{}
}

// RateLimitStatus tracks rate limiting information
type RateLimitStatus struct {
	Allowed       bool
	RequestsCount int
	WindowStart   time.Time
	ResetTime     time.Time
}

// NewSecureWebhookHandler creates a new secure webhook handler
func NewSecureWebhookHandler(bridge *IntegrationBridge, securityManager *security.SecurityManager) *SecureWebhookHandler {
	mux := http.NewServeMux()
	
	// Initialize rate limiter (100 requests per minute)
	rateLimiter := &RateLimiter{
		requests:    make(map[string]*RequestCount),
		maxRequests: 100,
		window:      time.Minute,
		cleanup:     time.Minute * 5,
	}
	
	wh := &SecureWebhookHandler{
		bridge:          bridge,
		mux:             mux,
		securityManager: securityManager,
		rateLimiter:     rateLimiter,
	}

	// Register secure webhook endpoints
	wh.registerSecureRoutes()
	
	// Start cleanup routine for rate limiter
	go wh.rateLimiter.cleanupRoutine()

	return wh
}

// Start starts the secure webhook handler server
func (swh *SecureWebhookHandler) Start(ctx context.Context) error {
	swh.mu.Lock()
	defer swh.mu.Unlock()

	if swh.running {
		return fmt.Errorf("secure webhook handler is already running")
	}

	// Get port from configuration
	port := "8443" // Use HTTPS port by default
	if swh.bridge.config.Integration.Enabled && swh.bridge.config.Integration.WebhookPort != 0 {
		port = fmt.Sprintf("%d", swh.bridge.config.Integration.WebhookPort)
	}

	// Create server with security headers middleware
	swh.server = &http.Server{
		Addr:         ":" + port,
		Handler:      swh.securityMiddleware(swh.mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	swh.running = true

	// Start HTTPS server with TLS configuration
	go func() {
		log.Printf("Starting secure webhook handler on port %s", port)
		
		// Get TLS configuration from security manager
		if tlsManager := swh.securityManager.GetTLSManager(); tlsManager != nil {
			swh.server.TLSConfig = tlsManager.GetTLSConfig()
			if err := swh.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Printf("Secure webhook handler TLS error: %v", err)
			}
		} else {
			// Fallback to HTTP (not recommended for production)
			log.Printf("WARNING: TLS not available, falling back to HTTP")
			if err := swh.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Secure webhook handler HTTP error: %v", err)
			}
		}
	}()

	return nil
}

// Stop stops the secure webhook handler server
func (swh *SecureWebhookHandler) Stop() error {
	swh.mu.Lock()
	defer swh.mu.Unlock()

	if !swh.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := swh.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown secure webhook handler: %v", err)
	}

	swh.running = false
	log.Printf("Secure webhook handler stopped")
	return nil
}

// registerSecureRoutes sets up secure webhook endpoints
func (swh *SecureWebhookHandler) registerSecureRoutes() {
	// Public endpoints (with rate limiting)
	swh.mux.HandleFunc("/health", swh.handleSecureHealth)
	
	// Protected endpoints (with authentication)
	swh.mux.HandleFunc("/webhooks/backup/completed", swh.handleSecureBackupCompleted)
	swh.mux.HandleFunc("/webhooks/gitops/generate", swh.handleSecureGitOpsGenerate)
	swh.mux.HandleFunc("/webhooks/gitops/completed", swh.handleSecureGitOpsCompleted)
	swh.mux.HandleFunc("/register/backup", swh.handleSecureRegisterBackup)
	swh.mux.HandleFunc("/register/gitops", swh.handleSecureRegisterGitOps)
	swh.mux.HandleFunc("/status", swh.handleSecureStatus)
}

// securityMiddleware applies security measures to all requests
func (swh *SecureWebhookHandler) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		
		// Rate limiting
		clientIP := getClientIP(r)
		if !swh.rateLimiter.Allow(clientIP) {
			swh.sendSecurityError(w, r, http.StatusTooManyRequests, "Rate limit exceeded", "rate_limit_exceeded")
			return
		}
		
		// Request size limiting (1MB max)
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		
		// Log security event
		swh.logSecurityEvent(r, "request_received", "success", time.Since(startTime))
		
		next.ServeHTTP(w, r)
	})
}

// handleSecureHealth provides security-aware health check
func (swh *SecureWebhookHandler) handleSecureHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		swh.sendSecurityError(w, r, http.StatusMethodNotAllowed, "Method not allowed", "invalid_method")
		return
	}

	// Basic health check without sensitive information
	status := swh.bridge.HealthCheck(r.Context())
	
	response := map[string]interface{}{
		"status":    status.Status,
		"timestamp": time.Now(),
		"service":   "integration-bridge",
		"version":   swh.bridge.GetComponentVersion(),
	}
	
	// Don't expose detailed health information without authentication
	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	swh.sendJSONResponse(w, response)
}

// handleSecureBackupCompleted processes backup completion webhooks securely
func (swh *SecureWebhookHandler) handleSecureBackupCompleted(w http.ResponseWriter, r *http.Request) {
	secureRequest, err := swh.authenticateAndValidateRequest(r, "backup_webhook")
	if err != nil {
		swh.sendSecurityError(w, r, http.StatusUnauthorized, "Authentication failed", "auth_failed")
		return
	}

	// Parse backup completion data
	var backupEvent BackupCompletionEvent
	if err := swh.parseAndValidateJSON(secureRequest.Body, &backupEvent); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid backup data", "invalid_payload")
		return
	}

	// Additional validation for backup event
	if err := swh.validateBackupEvent(&backupEvent); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid backup event", "validation_failed")
		return
	}

	// Process backup completion
	if err := swh.bridge.TriggerGitOpsGeneration(r.Context(), &backupEvent); err != nil {
		swh.sendSecurityError(w, r, http.StatusInternalServerError, "Failed to trigger GitOps generation", "processing_failed")
		return
	}

	// Update security metrics
	swh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("secure_webhook_requests_total",
		map[string]string{"endpoint": "backup_completed", "status": "success", "client": secureRequest.SecurityContext.ClientID}, 1)

	// Log successful processing
	swh.logSecurityEvent(r, "backup_webhook_processed", "success", 0)

	response := map[string]interface{}{
		"success":   true,
		"message":   "Backup completion processed successfully",
		"backup_id": backupEvent.BackupID,
		"timestamp": time.Now(),
	}

	swh.sendJSONResponse(w, response)
}

// handleSecureGitOpsGenerate processes GitOps generation requests securely
func (swh *SecureWebhookHandler) handleSecureGitOpsGenerate(w http.ResponseWriter, r *http.Request) {
	secureRequest, err := swh.authenticateAndValidateRequest(r, "gitops_webhook")
	if err != nil {
		swh.sendSecurityError(w, r, http.StatusUnauthorized, "Authentication failed", "auth_failed")
		return
	}

	// Parse GitOps generation request
	var gitopsRequest GitOpsGenerationRequest
	if err := swh.parseAndValidateJSON(secureRequest.Body, &gitopsRequest); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid GitOps data", "invalid_payload")
		return
	}

	// Validate GitOps request
	if err := swh.validateGitOpsRequest(&gitopsRequest); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid GitOps request", "validation_failed")
		return
	}

	// Create integration event
	integrationEvent := &IntegrationEvent{
		ID:        gitopsRequest.RequestID,
		Type:      "gitops_generation_requested",
		Source:    secureRequest.SecurityContext.ClientID,
		Target:    "gitops-generator",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"request": gitopsRequest,
		},
		Metadata: map[string]interface{}{
			"authenticated": true,
			"client_id":     secureRequest.SecurityContext.ClientID,
		},
	}

	// Publish event
	if err := swh.bridge.eventBus.Publish(r.Context(), integrationEvent); err != nil {
		swh.sendSecurityError(w, r, http.StatusInternalServerError, "Failed to process GitOps request", "processing_failed")
		return
	}

	// Update security metrics
	swh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("secure_webhook_requests_total",
		map[string]string{"endpoint": "gitops_generate", "status": "success", "client": secureRequest.SecurityContext.ClientID}, 1)

	response := map[string]interface{}{
		"success":    true,
		"message":    "GitOps generation request processed",
		"request_id": gitopsRequest.RequestID,
		"timestamp":  time.Now(),
	}

	w.WriteHeader(http.StatusAccepted)
	swh.sendJSONResponse(w, response)
}

// handleSecureGitOpsCompleted processes GitOps completion notifications securely
func (swh *SecureWebhookHandler) handleSecureGitOpsCompleted(w http.ResponseWriter, r *http.Request) {
	secureRequest, err := swh.authenticateAndValidateRequest(r, "gitops_completion")
	if err != nil {
		swh.sendSecurityError(w, r, http.StatusUnauthorized, "Authentication failed", "auth_failed")
		return
	}

	// Parse completion data
	var completionData map[string]interface{}
	if err := swh.parseAndValidateJSON(secureRequest.Body, &completionData); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid completion data", "invalid_payload")
		return
	}

	// Create integration event
	integrationEvent := &IntegrationEvent{
		ID:        fmt.Sprintf("completion-%d", time.Now().Unix()),
		Type:      "gitops_completed",
		Source:    secureRequest.SecurityContext.ClientID,
		Target:    "integration-bridge",
		Timestamp: time.Now(),
		Data:      completionData,
		Metadata: map[string]interface{}{
			"authenticated": true,
			"client_id":     secureRequest.SecurityContext.ClientID,
		},
	}

	// Publish event
	if err := swh.bridge.eventBus.Publish(r.Context(), integrationEvent); err != nil {
		swh.sendSecurityError(w, r, http.StatusInternalServerError, "Failed to process completion notification", "processing_failed")
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"message":   "GitOps completion processed",
		"timestamp": time.Now(),
	}

	swh.sendJSONResponse(w, response)
}

// handleSecureRegisterBackup registers backup tool component securely
func (swh *SecureWebhookHandler) handleSecureRegisterBackup(w http.ResponseWriter, r *http.Request) {
	secureRequest, err := swh.authenticateAndValidateRequest(r, "component_registration")
	if err != nil {
		swh.sendSecurityError(w, r, http.StatusUnauthorized, "Authentication failed", "auth_failed")
		return
	}

	var request struct {
		Endpoint string `json:"endpoint"`
		Version  string `json:"version"`
	}

	if err := swh.parseAndValidateJSON(secureRequest.Body, &request); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid JSON payload", "invalid_payload")
		return
	}

	// Validate registration data
	if err := swh.validateRegistrationRequest(&request); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid registration data", "validation_failed")
		return
	}

	if err := swh.bridge.RegisterBackupTool(request.Endpoint, request.Version); err != nil {
		swh.sendSecurityError(w, r, http.StatusInternalServerError, "Failed to register backup tool", "registration_failed")
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"message":   "Backup tool registered successfully",
		"endpoint":  request.Endpoint,
		"version":   request.Version,
		"timestamp": time.Now(),
	}

	swh.sendJSONResponse(w, response)
}

// handleSecureRegisterGitOps registers GitOps generator component securely
func (swh *SecureWebhookHandler) handleSecureRegisterGitOps(w http.ResponseWriter, r *http.Request) {
	secureRequest, err := swh.authenticateAndValidateRequest(r, "component_registration")
	if err != nil {
		swh.sendSecurityError(w, r, http.StatusUnauthorized, "Authentication failed", "auth_failed")
		return
	}

	var request struct {
		Endpoint string `json:"endpoint"`
		Version  string `json:"version"`
	}

	if err := swh.parseAndValidateJSON(secureRequest.Body, &request); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid JSON payload", "invalid_payload")
		return
	}

	// Validate registration data
	if err := swh.validateRegistrationRequest(&request); err != nil {
		swh.sendSecurityError(w, r, http.StatusBadRequest, "Invalid registration data", "validation_failed")
		return
	}

	if err := swh.bridge.RegisterGitOpsTool(request.Endpoint, request.Version); err != nil {
		swh.sendSecurityError(w, r, http.StatusInternalServerError, "Failed to register GitOps tool", "registration_failed")
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"message":   "GitOps tool registered successfully",
		"endpoint":  request.Endpoint,
		"version":   request.Version,
		"timestamp": time.Now(),
	}

	swh.sendJSONResponse(w, response)
}

// handleSecureStatus returns component status information securely
func (swh *SecureWebhookHandler) handleSecureStatus(w http.ResponseWriter, r *http.Request) {
	secureRequest, err := swh.authenticateAndValidateRequest(r, "status_access")
	if err != nil {
		swh.sendSecurityError(w, r, http.StatusUnauthorized, "Authentication failed", "auth_failed")
		return
	}

	// Check if client has read permissions
	if !swh.hasPermission(secureRequest.SecurityContext, "status_read") {
		swh.sendSecurityError(w, r, http.StatusForbidden, "Insufficient permissions", "permission_denied")
		return
	}

	status := swh.bridge.GetComponentStatus()
	
	response := map[string]interface{}{
		"success":    true,
		"message":    "Component status retrieved",
		"timestamp":  time.Now(),
		"data":       status,
		"security": map[string]interface{}{
			"authenticated": true,
			"client_id":     secureRequest.SecurityContext.ClientID,
		},
	}

	swh.sendJSONResponse(w, response)
}

// authenticateAndValidateRequest performs authentication and validation
func (swh *SecureWebhookHandler) authenticateAndValidateRequest(r *http.Request, operation string) (*SecureWebhookRequest, error) {
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
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %v", err)
		}
		webhookRequest.Body = body
	}

	// Authenticate request using security manager
	response, err := swh.securityManager.SecureWebhookRequest(r.Context(), webhookRequest)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %v", err)
	}

	// Create security context
	securityContext := &SecurityContext{
		ClientID:      extractClientID(webhookRequest),
		AuthMethod:    extractAuthMethod(webhookRequest),
		Permissions:   extractPermissions(webhookRequest),
		TrustedClient: isTrustedClient(webhookRequest),
	}

	// Create validation result
	validationResult := &ValidationResult{
		Valid:         true,
		Errors:        []string{},
		Warnings:      []string{},
		SanitizedData: make(map[string]interface{}),
	}

	secureRequest := &SecureWebhookRequest{
		WebhookRequest:   webhookRequest,
		SecurityContext:  securityContext,
		ValidationResult: validationResult,
	}

	return secureRequest, nil
}

// Validation functions

func (swh *SecureWebhookHandler) validateBackupEvent(event *BackupCompletionEvent) error {
	if event.BackupID == "" {
		return fmt.Errorf("backup_id is required")
	}
	if event.ClusterName == "" {
		return fmt.Errorf("cluster_name is required")
	}
	if event.MinIOPath == "" {
		return fmt.Errorf("minio_path is required")
	}
	return nil
}

func (swh *SecureWebhookHandler) validateGitOpsRequest(request *GitOpsGenerationRequest) error {
	if request.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if request.BackupID == "" {
		return fmt.Errorf("backup_id is required")
	}
	if request.ClusterName == "" {
		return fmt.Errorf("cluster_name is required")
	}
	if request.SourcePath == "" {
		return fmt.Errorf("source_path is required")
	}
	return nil
}

func (swh *SecureWebhookHandler) validateRegistrationRequest(request *struct {
	Endpoint string `json:"endpoint"`
	Version  string `json:"version"`
}) error {
	if request.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if request.Version == "" {
		return fmt.Errorf("version is required")
	}
	// Validate endpoint format
	if !strings.HasPrefix(request.Endpoint, "http://") && !strings.HasPrefix(request.Endpoint, "https://") {
		return fmt.Errorf("endpoint must be a valid HTTP/HTTPS URL")
	}
	return nil
}

// Utility functions

func (swh *SecureWebhookHandler) parseAndValidateJSON(body []byte, target interface{}) error {
	if len(body) == 0 {
		return fmt.Errorf("empty request body")
	}
	
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}
	
	return nil
}

func (swh *SecureWebhookHandler) hasPermission(ctx *SecurityContext, permission string) bool {
	for _, p := range ctx.Permissions {
		if p == permission || p == "admin" {
			return true
		}
	}
	return false
}

func (swh *SecureWebhookHandler) sendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (swh *SecureWebhookHandler) sendSecurityError(w http.ResponseWriter, r *http.Request, statusCode int, message, errorType string) {
	// Log security event
	swh.logSecurityEvent(r, "security_error", errorType, 0)
	
	// Update error metrics
	swh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("secure_webhook_errors_total",
		map[string]string{"error_type": errorType, "status_code": fmt.Sprintf("%d", statusCode)}, 1)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := map[string]interface{}{
		"success":    false,
		"error":      message,
		"error_type": errorType,
		"timestamp":  time.Now(),
	}
	
	json.NewEncoder(w).Encode(response)
}

func (swh *SecureWebhookHandler) logSecurityEvent(r *http.Request, eventType, status string, duration time.Duration) {
	if swh.securityManager.GetAuditLogger() != nil {
		swh.securityManager.GetAuditLogger().LogWebhookEvent(
			getClientIP(r),
			r.URL.Path,
			r.Method,
			status,
			r.UserAgent(),
		)
	}
}

// Rate limiting implementation

func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	
	// Get or create request count for client
	count, exists := rl.requests[clientID]
	if !exists {
		rl.requests[clientID] = &RequestCount{
			Count:     1,
			Window:    now,
			LastSeen:  now,
		}
		return true
	}
	
	// Check if we're in a new window
	if now.Sub(count.Window) >= rl.window {
		count.Count = 1
		count.Window = now
		count.LastSeen = now
		return true
	}
	
	// Check if limit exceeded
	if count.Count >= rl.maxRequests {
		count.LastSeen = now
		return false
	}
	
	// Increment count
	count.Count++
	count.LastSeen = now
	return true
}

func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		
		for clientID, count := range rl.requests {
			if now.Sub(count.LastSeen) >= rl.cleanup {
				delete(rl.requests, clientID)
			}
		}
		
		rl.mu.Unlock()
	}
}

// Helper functions

func extractClientID(request *security.WebhookRequest) string {
	// Extract client ID from API key or certificate
	if apiKey := request.Headers["X-API-Key"]; apiKey != "" {
		// In production, this would map to actual client ID
		return fmt.Sprintf("api-key-%s", apiKey[:8])
	}
	return fmt.Sprintf("client-%s", request.SourceIP)
}

func extractAuthMethod(request *security.WebhookRequest) string {
	if request.Headers["X-API-Key"] != "" {
		return "api_key"
	}
	if strings.HasPrefix(request.Headers["Authorization"], "Bearer ") {
		return "bearer_token"
	}
	if strings.HasPrefix(request.Headers["Authorization"], "Basic ") {
		return "basic_auth"
	}
	return "none"
}

func extractPermissions(request *security.WebhookRequest) []string {
	// In production, this would be based on authenticated user/service
	// For now, return basic permissions
	return []string{"webhook_access", "status_read"}
}

func isTrustedClient(request *security.WebhookRequest) bool {
	// In production, this would check against a list of trusted clients
	return request.Headers["X-API-Key"] != ""
}

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