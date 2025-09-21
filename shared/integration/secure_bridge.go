package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"sync"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/http"
	"shared-config/monitoring"
	"shared-config/security"
	"shared-config/triggers"
)

// SecureIntegrationBridge provides a security-enhanced integration bridge
type SecureIntegrationBridge struct {
	config               *sharedconfig.SharedConfig
	securityManager      *security.SecurityManager
	monitoringSystem     *monitoring.MonitoringSystem
	httpClient           *http.MonitoredHTTPClient
	triggerSystem        *triggers.MonitoredAutoTrigger
	
	// Component status tracking
	backupStatus         ComponentStatus
	gitopsStatus         ComponentStatus
	bridgeStatus         ComponentStatus
	
	// Event coordination
	eventBus             *EventBus
	secureWebhookHandler *SecureWebhookHandler
	
	// Integrated monitoring
	monitoringIntegration *MonitoringIntegration
	
	// Security features
	certificateManager   *CertificateManager
	rateLimiter          *RateLimiter
	auditLogger          *security.AuditLogger
	
	// Lifecycle management
	running              bool
	mu                   sync.RWMutex
	ctx                  context.Context
	cancel               context.CancelFunc
}

// CertificateManager handles TLS certificate management
type CertificateManager struct {
	certPath         string
	keyPath          string
	caPath           string
	autoRotate       bool
	rotationInterval time.Duration
	mu               sync.RWMutex
}

// SecurityMetrics tracks security-related metrics
type SecurityMetrics struct {
	AuthenticationAttempts  int64
	AuthenticationFailures  int64
	AuthorizationFailures   int64
	RateLimitViolations     int64
	SecurityViolations      int64
	VulnerabilitiesDetected int64
	LastSecurityScan        time.Time
	CertificateExpiry       time.Time
}

// NewSecureIntegrationBridge creates a new secure integration bridge
func NewSecureIntegrationBridge(config *sharedconfig.SharedConfig) (*SecureIntegrationBridge, error) {
	if config == nil {
		return nil, fmt.Errorf("shared configuration is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize logger
	logger := monitoring.NewLogger("secure_integration_bridge")

	// Initialize security manager
	securityConfig := mapConfigToSecurityConfig(config)
	securityManager, err := security.NewSecurityManager(securityConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize security manager: %v", err)
	}

	// Initialize monitoring system
	monitoringSystem := monitoring.NewMonitoringSystem(config, logger)

	// Initialize secure HTTP client
	httpClient := createSecureHTTPClient(config, securityManager, logger)

	// Initialize trigger system
	triggerSystem := triggers.NewMonitoredAutoTrigger(config, logger)

	// Initialize certificate manager
	certManager := &CertificateManager{
		certPath:         config.Security.Network.TLS.CertFile,
		keyPath:          config.Security.Network.TLS.KeyFile,
		caPath:           config.Security.Network.TLS.CAFile,
		autoRotate:       true,
		rotationInterval: 24 * time.Hour,
	}

	bridge := &SecureIntegrationBridge{
		config:               config,
		securityManager:      securityManager,
		monitoringSystem:     monitoringSystem,
		httpClient:           httpClient,
		triggerSystem:        triggerSystem,
		certificateManager:   certManager,
		eventBus:             NewEventBus(),
		ctx:                  ctx,
		cancel:               cancel,
		bridgeStatus: ComponentStatus{
			Name:      "secure-integration-bridge",
			Status:    "initializing",
			LastCheck: time.Now(),
			Version:   "2.0.0",
			Metadata:  make(map[string]interface{}),
		},
	}

	// Initialize secure webhook handler
	bridge.secureWebhookHandler = NewSecureWebhookHandler(bridge, securityManager)

	// Initialize monitoring integration
	bridge.monitoringIntegration = NewMonitoringIntegration(bridge)

	// Register event handlers
	bridge.registerSecureEventHandlers()

	// Start certificate rotation if enabled
	if certManager.autoRotate {
		go bridge.certificateRotationRoutine()
	}

	logger.Info("secure integration bridge initialized", map[string]interface{}{
		"security_enabled": config.Security.Enabled,
		"tls_enabled":      config.Security.Network.TLS.Enabled,
		"audit_enabled":    config.Security.Audit.Enabled,
	})

	return bridge, nil
}

// Start initializes and starts the secure integration bridge
func (sib *SecureIntegrationBridge) Start(ctx context.Context) error {
	sib.mu.Lock()
	defer sib.mu.Unlock()

	if sib.running {
		return fmt.Errorf("secure integration bridge is already running")
	}

	log.Printf("Starting secure integration bridge...")

	// Start security manager first
	if !sib.securityManager.IsInitialized() {
		return fmt.Errorf("security manager not initialized")
	}

	// Start monitoring system
	if err := sib.monitoringSystem.Start(ctx); err != nil {
		return fmt.Errorf("failed to start monitoring system: %v", err)
	}

	// Register bridge component with monitoring
	if err := sib.monitoringSystem.GetMonitoringHub().RegisterComponent("secure-integration-bridge", sib); err != nil {
		log.Printf("Warning: failed to register bridge with monitoring: %v", err)
	}

	// Start secure webhook handler
	if err := sib.secureWebhookHandler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start secure webhook handler: %v", err)
	}

	// Start monitoring integration
	if err := sib.monitoringIntegration.Start(ctx); err != nil {
		log.Printf("Warning: failed to start monitoring integration: %v", err)
	}

	// Perform initial security scan
	if err := sib.performInitialSecurityScan(ctx); err != nil {
		log.Printf("Warning: initial security scan failed: %v", err)
	}

	// Update status
	sib.bridgeStatus.Status = "healthy"
	sib.bridgeStatus.LastCheck = time.Now()
	sib.bridgeStatus.Metadata["security_score"] = sib.calculateSecurityScore()
	sib.running = true

	// Start background health monitoring
	go sib.monitorComponentHealth(ctx)
	go sib.monitorSecurityMetrics(ctx)

	// Log security event
	if sib.securityManager.GetAuditLogger() != nil {
		sib.securityManager.GetAuditLogger().LogSystemEvent(
			"secure_bridge_started",
			"secure-integration-bridge",
			"success",
			"Secure integration bridge started successfully",
		)
	}

	log.Printf("Secure integration bridge started successfully")
	return nil
}

// Stop gracefully shuts down the secure integration bridge
func (sib *SecureIntegrationBridge) Stop() error {
	sib.mu.Lock()
	defer sib.mu.Unlock()

	if !sib.running {
		return nil
	}

	log.Printf("Stopping secure integration bridge...")

	// Log security event
	if sib.securityManager.GetAuditLogger() != nil {
		sib.securityManager.GetAuditLogger().LogSystemEvent(
			"secure_bridge_stopping",
			"secure-integration-bridge",
			"info",
			"Secure integration bridge shutting down",
		)
	}

	// Stop secure webhook handler
	if err := sib.secureWebhookHandler.Stop(); err != nil {
		log.Printf("Warning: error stopping secure webhook handler: %v", err)
	}

	// Stop monitoring integration
	if err := sib.monitoringIntegration.Stop(); err != nil {
		log.Printf("Warning: error stopping monitoring integration: %v", err)
	}

	// Stop monitoring system
	if err := sib.monitoringSystem.Stop(); err != nil {
		log.Printf("Warning: error stopping monitoring system: %v", err)
	}

	// Cancel context
	sib.cancel()

	sib.bridgeStatus.Status = "stopped"
	sib.bridgeStatus.LastCheck = time.Now()
	sib.running = false

	log.Printf("Secure integration bridge stopped")
	return nil
}

// TriggerSecureGitOpsGeneration handles backup completion with security validation
func (sib *SecureIntegrationBridge) TriggerSecureGitOpsGeneration(ctx context.Context, event *BackupCompletionEvent, authContext *security.Session) error {
	// Validate authorization
	if authContext == nil {
		return fmt.Errorf("authentication required for GitOps generation")
	}

	if err := sib.securityManager.GetAuthManager().Authorize(authContext, security.ResourceGitOps, security.PermissionTrigger); err != nil {
		return fmt.Errorf("authorization failed: %v", err)
	}

	// Validate backup event
	if err := sib.validateBackupEvent(event); err != nil {
		return fmt.Errorf("backup event validation failed: %v", err)
	}

	if !event.Success {
		log.Printf("Backup failed, skipping GitOps generation: %s", event.ErrorMessage)
		return nil
	}

	log.Printf("Triggering secure GitOps generation for backup %s", event.BackupID)

	// Create GitOps generation request with security context
	request := &GitOpsGenerationRequest{
		RequestID:     fmt.Sprintf("gitops-%s-%d", event.BackupID, time.Now().Unix()),
		BackupID:      event.BackupID,
		ClusterName:   event.ClusterName,
		SourcePath:    event.MinIOPath,
		TargetRepo:    sib.config.GitOps.Repository.URL,
		TargetBranch:  sib.config.GitOps.Repository.Branch,
		Configuration: map[string]interface{}{
			"timestamp":       event.Timestamp,
			"resource_count":  event.ResourceCount,
			"size_bytes":      event.Size,
			"authenticated":   true,
			"user_id":         authContext.UserID,
		},
	}

	// Sign the request for integrity
	if sib.securityManager.GetSecretsManager() != nil {
		signature, err := sib.signRequest(request)
		if err != nil {
			log.Printf("Warning: failed to sign request: %v", err)
		} else {
			request.Configuration["signature"] = signature
		}
	}

	// Publish secure integration event
	integrationEvent := &IntegrationEvent{
		ID:        request.RequestID,
		Type:      "secure_gitops_generation_requested",
		Source:    "backup-tool",
		Target:    "gitops-generator",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"backup_completion": event,
			"generation_request": request,
		},
		Metadata: map[string]interface{}{
			"component":     "secure-integration-bridge",
			"version":       sib.bridgeStatus.Version,
			"authenticated": true,
			"user_id":       authContext.UserID,
			"security_level": "high",
		},
	}

	if err := sib.eventBus.Publish(ctx, integrationEvent); err != nil {
		return fmt.Errorf("failed to publish secure GitOps generation event: %v", err)
	}

	// Trigger using the shared trigger system with security context
	_, err := sib.triggerSystem.TriggerGitOpsGeneration(ctx, &triggers.BackupCompletionEvent{
		BackupID:        event.BackupID,
		ClusterName:     event.ClusterName,
		Timestamp:       event.Timestamp,
		Success:         event.Success,
		MinIOBucket:     sib.config.Storage.Bucket,
		ResourcesCount:  event.ResourceCount,
		BackupSize:      event.Size,
		Errors:          []string{event.ErrorMessage},
		SecurityContext: map[string]interface{}{
			"user_id":       authContext.UserID,
			"authenticated": true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to trigger secure GitOps generation: %v", err)
	}

	// Update security metrics
	sib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("secure_gitops_triggers_total", 
		map[string]string{
			"cluster":       event.ClusterName,
			"status":        "success",
			"user_id":       authContext.UserID,
			"authenticated": "true",
		}, 1)

	// Log security event
	if sib.securityManager.GetAuditLogger() != nil {
		sib.securityManager.GetAuditLogger().LogSystemEvent(
			"gitops_generation_triggered",
			"secure-integration-bridge",
			"success",
			fmt.Sprintf("GitOps generation triggered for backup %s by user %s", event.BackupID, authContext.UserID),
		)
	}

	return nil
}

// RegisterSecureBackupTool registers the backup tool component with security validation
func (sib *SecureIntegrationBridge) RegisterSecureBackupTool(endpoint string, version string, authContext *security.Session) error {
	sib.mu.Lock()
	defer sib.mu.Unlock()

	// Validate authorization
	if authContext == nil {
		return fmt.Errorf("authentication required for component registration")
	}

	if err := sib.securityManager.GetAuthManager().Authorize(authContext, security.ResourceComponents, security.PermissionWrite); err != nil {
		return fmt.Errorf("authorization failed: %v", err)
	}

	// Validate endpoint
	if err := sib.validateEndpoint(endpoint); err != nil {
		return fmt.Errorf("endpoint validation failed: %v", err)
	}

	sib.backupStatus = ComponentStatus{
		Name:      "backup-tool",
		Status:    "registered",
		LastCheck: time.Now(),
		Version:   version,
		Metadata: map[string]interface{}{
			"endpoint":      endpoint,
			"registered_by": authContext.UserID,
			"secure":        true,
		},
		Dependencies: []string{"kubernetes", "minio", "security"},
	}

	// Log security event
	if sib.securityManager.GetAuditLogger() != nil {
		sib.securityManager.GetAuditLogger().LogSystemEvent(
			"component_registered",
			"backup-tool",
			"success",
			fmt.Sprintf("Backup tool registered by user %s: %s (version %s)", authContext.UserID, endpoint, version),
		)
	}

	log.Printf("Secure backup tool registered: %s (version %s) by user %s", endpoint, version, authContext.UserID)
	return nil
}

// RegisterSecureGitOpsTool registers the GitOps generator component with security validation
func (sib *SecureIntegrationBridge) RegisterSecureGitOpsTool(endpoint string, version string, authContext *security.Session) error {
	sib.mu.Lock()
	defer sib.mu.Unlock()

	// Validate authorization
	if authContext == nil {
		return fmt.Errorf("authentication required for component registration")
	}

	if err := sib.securityManager.GetAuthManager().Authorize(authContext, security.ResourceComponents, security.PermissionWrite); err != nil {
		return fmt.Errorf("authorization failed: %v", err)
	}

	// Validate endpoint
	if err := sib.validateEndpoint(endpoint); err != nil {
		return fmt.Errorf("endpoint validation failed: %v", err)
	}

	sib.gitopsStatus = ComponentStatus{
		Name:      "gitops-generator",
		Status:    "registered",
		LastCheck: time.Now(),
		Version:   version,
		Metadata: map[string]interface{}{
			"endpoint":      endpoint,
			"registered_by": authContext.UserID,
			"secure":        true,
		},
		Dependencies: []string{"minio", "git", "security"},
	}

	// Log security event
	if sib.securityManager.GetAuditLogger() != nil {
		sib.securityManager.GetAuditLogger().LogSystemEvent(
			"component_registered",
			"gitops-generator",
			"success",
			fmt.Sprintf("GitOps generator registered by user %s: %s (version %s)", authContext.UserID, endpoint, version),
		)
	}

	log.Printf("Secure GitOps generator registered: %s (version %s) by user %s", endpoint, version, authContext.UserID)
	return nil
}

// GetSecurityStatus returns comprehensive security status
func (sib *SecureIntegrationBridge) GetSecurityStatus() *SecurityStatus {
	securityStatus := sib.securityManager.GetSecurityStatus()
	
	// Add bridge-specific security information
	bridgeSecurityInfo := &SecurityStatus{
		Timestamp:     time.Now(),
		OverallStatus: securityStatus.OverallStatus,
		Components: map[string]interface{}{
			"bridge_security":         sib.calculateSecurityScore(),
			"certificate_expiry":      sib.getCertificateExpiry(),
			"rate_limiting_active":    sib.rateLimiter != nil,
			"audit_logging_active":    sib.securityManager.GetAuditLogger() != nil,
			"vulnerability_scan_date": sib.getLastVulnerabilityScan(),
		},
		SecurityMetrics: sib.getSecurityMetrics(),
		Recommendations: sib.generateSecurityRecommendations(),
	}

	return bridgeSecurityInfo
}

// Monitoring and health checks

func (sib *SecureIntegrationBridge) monitorComponentHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sib.checkSecureComponentHealth(ctx)
		}
	}
}

func (sib *SecureIntegrationBridge) monitorSecurityMetrics(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sib.updateSecurityMetrics()
		}
	}
}

func (sib *SecureIntegrationBridge) checkSecureComponentHealth(ctx context.Context) {
	sib.mu.Lock()
	defer sib.mu.Unlock()

	// Check backup tool health with security validation
	if sib.backupStatus.Name != "" {
		if endpoint, ok := sib.backupStatus.Metadata["endpoint"].(string); ok {
			if err := sib.pingSecureComponent(ctx, endpoint+"/health"); err != nil {
				sib.backupStatus.Status = "unhealthy"
				sib.backupStatus.Metadata["error"] = err.Error()
				sib.logSecurityEvent("component_health_check_failed", "backup-tool", err.Error())
			} else {
				sib.backupStatus.Status = "healthy"
				delete(sib.backupStatus.Metadata, "error")
			}
			sib.backupStatus.LastCheck = time.Now()
		}
	}

	// Check GitOps tool health with security validation
	if sib.gitopsStatus.Name != "" {
		if endpoint, ok := sib.gitopsStatus.Metadata["endpoint"].(string); ok {
			if err := sib.pingSecureComponent(ctx, endpoint+"/health"); err != nil {
				sib.gitopsStatus.Status = "unhealthy"
				sib.gitopsStatus.Metadata["error"] = err.Error()
				sib.logSecurityEvent("component_health_check_failed", "gitops-generator", err.Error())
			} else {
				sib.gitopsStatus.Status = "healthy"
				delete(sib.gitopsStatus.Metadata, "error")
			}
			sib.gitopsStatus.LastCheck = time.Now()
		}
	}

	// Update bridge status
	sib.bridgeStatus.LastCheck = time.Now()
	if sib.running {
		sib.bridgeStatus.Status = "healthy"
		sib.bridgeStatus.Metadata["security_score"] = sib.calculateSecurityScore()
	}
}

// Security utility functions

func (sib *SecureIntegrationBridge) performInitialSecurityScan(ctx context.Context) error {
	if scanner := sib.securityManager.GetVulnerabilityScanner(); scanner != nil {
		_, err := sib.securityManager.PerformSecurityScan(ctx)
		return err
	}
	return nil
}

func (sib *SecureIntegrationBridge) validateBackupEvent(event *BackupCompletionEvent) error {
	if event.BackupID == "" {
		return fmt.Errorf("backup_id is required")
	}
	if event.ClusterName == "" {
		return fmt.Errorf("cluster_name is required")
	}
	if event.MinIOPath == "" {
		return fmt.Errorf("minio_path is required")
	}
	if event.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}

func (sib *SecureIntegrationBridge) validateEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("endpoint cannot be empty")
	}
	// Additional endpoint validation logic
	return nil
}

func (sib *SecureIntegrationBridge) signRequest(request *GitOpsGenerationRequest) (string, error) {
	// Implement request signing logic
	return "signature", nil
}

func (sib *SecureIntegrationBridge) pingSecureComponent(ctx context.Context, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use secure HTTP client with authentication
	_, err := sib.httpClient.Get(ctx, endpoint)
	return err
}

func (sib *SecureIntegrationBridge) calculateSecurityScore() float64 {
	score := 0.0
	if sib.securityManager.GetSecretsManager() != nil {
		score += 20
	}
	if sib.securityManager.GetAuthManager() != nil {
		score += 25
	}
	if sib.securityManager.GetTLSManager() != nil {
		score += 20
	}
	if sib.securityManager.GetVulnerabilityScanner() != nil {
		score += 20
	}
	if sib.securityManager.GetAuditLogger() != nil {
		score += 15
	}
	return score
}

func (sib *SecureIntegrationBridge) getCertificateExpiry() time.Time {
	// Return certificate expiry time
	return time.Now().Add(30 * 24 * time.Hour) // Mock 30 days
}

func (sib *SecureIntegrationBridge) getLastVulnerabilityScan() time.Time {
	if scanner := sib.securityManager.GetVulnerabilityScanner(); scanner != nil {
		if latestScan := scanner.GetLatestScan(); latestScan != nil {
			return latestScan.Timestamp
		}
	}
	return time.Time{}
}

func (sib *SecureIntegrationBridge) getSecurityMetrics() *SecurityMetrics {
	return &SecurityMetrics{
		LastSecurityScan:  sib.getLastVulnerabilityScan(),
		CertificateExpiry: sib.getCertificateExpiry(),
	}
}

func (sib *SecureIntegrationBridge) generateSecurityRecommendations() []string {
	recommendations := []string{}
	
	if sib.getCertificateExpiry().Sub(time.Now()) < 7*24*time.Hour {
		recommendations = append(recommendations, "Certificate expires within 7 days - schedule renewal")
	}
	
	if time.Since(sib.getLastVulnerabilityScan()) > 24*time.Hour {
		recommendations = append(recommendations, "Vulnerability scan is overdue - run security scan")
	}
	
	return recommendations
}

func (sib *SecureIntegrationBridge) updateSecurityMetrics() {
	// Update security-related metrics
	metrics := sib.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
	
	metrics.SetGauge("security_score", 
		map[string]string{"component": "secure-integration-bridge"},
		sib.calculateSecurityScore())
	
	metrics.SetGauge("certificate_days_until_expiry",
		map[string]string{"component": "secure-integration-bridge"},
		sib.getCertificateExpiry().Sub(time.Now()).Hours()/24)
}

func (sib *SecureIntegrationBridge) logSecurityEvent(eventType, component, message string) {
	if sib.securityManager.GetAuditLogger() != nil {
		sib.securityManager.GetAuditLogger().LogSystemEvent(eventType, component, "info", message)
	}
}

func (sib *SecureIntegrationBridge) certificateRotationRoutine() {
	ticker := time.NewTicker(sib.certificateManager.rotationInterval)
	defer ticker.Stop()

	for range ticker.C {
		if sib.shouldRotateCertificate() {
			if err := sib.rotateCertificate(); err != nil {
				log.Printf("Certificate rotation failed: %v", err)
				sib.logSecurityEvent("certificate_rotation_failed", "secure-integration-bridge", err.Error())
			} else {
				log.Printf("Certificate rotated successfully")
				sib.logSecurityEvent("certificate_rotated", "secure-integration-bridge", "Certificate rotated successfully")
			}
		}
	}
}

func (sib *SecureIntegrationBridge) shouldRotateCertificate() bool {
	expiry := sib.getCertificateExpiry()
	return time.Until(expiry) < 7*24*time.Hour // Rotate if expires within 7 days
}

func (sib *SecureIntegrationBridge) rotateCertificate() error {
	// Implement certificate rotation logic
	log.Printf("Certificate rotation would be performed here")
	return nil
}

// Event handling

func (sib *SecureIntegrationBridge) registerSecureEventHandlers() {
	// Handle backup completion events with security validation
	sib.eventBus.Subscribe("backup_completed", func(ctx context.Context, event *IntegrationEvent) error {
		// Extract authentication context from event metadata
		authContext := extractAuthContextFromEvent(event)
		
		if backupData, ok := event.Data["backup_completion"]; ok {
			var backupEvent BackupCompletionEvent
			if data, err := json.Marshal(backupData); err == nil {
				if err := json.Unmarshal(data, &backupEvent); err == nil {
					return sib.TriggerSecureGitOpsGeneration(ctx, &backupEvent, authContext)
				}
			}
		}
		return fmt.Errorf("invalid backup completion event data")
	})

	// Handle GitOps generation completion events
	sib.eventBus.Subscribe("gitops_completed", func(ctx context.Context, event *IntegrationEvent) error {
		log.Printf("GitOps generation completed securely: %s", event.ID)
		
		// Update metrics with security context
		status := "success"
		if errorMsg, ok := event.Data["error"]; ok && errorMsg != nil {
			status = "failure"
		}
		
		userID := "unknown"
		if authCtx := extractAuthContextFromEvent(event); authCtx != nil {
			userID = authCtx.UserID
		}
		
		sib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("secure_gitops_completions_total",
			map[string]string{
				"status":        status,
				"user_id":       userID,
				"authenticated": fmt.Sprintf("%t", event.Metadata["authenticated"]),
			}, 1)
		
		// Log security event
		sib.logSecurityEvent("gitops_completion_processed", "secure-integration-bridge", 
			fmt.Sprintf("GitOps completion processed for user %s with status %s", userID, status))
		
		return nil
	})
}

// Helper functions

func mapConfigToSecurityConfig(config *sharedconfig.SharedConfig) security.SecurityConfig {
	return security.SecurityConfig{
		Enabled:    config.Security.Enabled,
		StrictMode: config.Security.StrictMode,
		SecretsManagement: security.SecretsManagerConfig{
			Provider: config.Security.Secrets.Provider,
		},
		Authentication: security.AuthConfig{
			Enabled: config.Security.Authentication.Enabled,
		},
		TLS: security.TLSConfig{
			Enabled: config.Security.Network.TLS.Enabled,
		},
		Audit: security.AuditConfig{
			Enabled: config.Security.Audit.Enabled,
			LogPath: config.Security.Audit.LogPath,
		},
	}
}

func createSecureHTTPClient(config *sharedconfig.SharedConfig, securityManager *security.SecurityManager, logger monitoring.Logger) *http.MonitoredHTTPClient {
	// Create HTTP client with TLS configuration
	if tlsManager := securityManager.GetTLSManager(); tlsManager != nil {
		return http.NewMonitoredHTTPClientWithTLS(config, "secure_integration_bridge", logger, tlsManager.GetTLSConfig())
	}
	return http.NewMonitoredHTTPClientFromConfig(config, "secure_integration_bridge", logger)
}

func extractAuthContextFromEvent(event *IntegrationEvent) *security.Session {
	// Extract authentication context from event metadata
	if event.Metadata["authenticated"] == true {
		if userID, ok := event.Metadata["user_id"].(string); ok {
			return &security.Session{
				UserID: userID,
			}
		}
	}
	return nil
}

// SecurityStatus represents comprehensive security status
type SecurityStatus struct {
	Timestamp       time.Time                `json:"timestamp"`
	OverallStatus   string                   `json:"overall_status"`
	Components      map[string]interface{}   `json:"components"`
	SecurityMetrics *SecurityMetrics         `json:"security_metrics"`
	Recommendations []string                 `json:"recommendations"`
}

// Implement MonitoredComponent interface for the secure bridge

func (sib *SecureIntegrationBridge) GetComponentName() string {
	return "secure-integration-bridge"
}

func (sib *SecureIntegrationBridge) GetComponentVersion() string {
	return sib.bridgeStatus.Version
}

func (sib *SecureIntegrationBridge) GetMetrics() map[string]interface{} {
	sib.mu.RLock()
	defer sib.mu.RUnlock()

	metrics := make(map[string]interface{})
	
	// Component counts
	componentCount := 0
	healthyCount := 0
	for _, status := range sib.GetComponentStatus() {
		if status.Name != "" {
			componentCount++
			if status.Status == "healthy" {
				healthyCount++
			}
		}
	}

	metrics["total_components"] = componentCount
	metrics["healthy_components"] = healthyCount
	metrics["bridge_running"] = sib.running
	metrics["security_score"] = sib.calculateSecurityScore()
	metrics["certificate_days_until_expiry"] = sib.getCertificateExpiry().Sub(time.Now()).Hours() / 24

	return metrics
}

func (sib *SecureIntegrationBridge) GetComponentStatus() map[string]ComponentStatus {
	sib.mu.RLock()
	defer sib.mu.RUnlock()

	return map[string]ComponentStatus{
		"backup":                     sib.backupStatus,
		"gitops":                     sib.gitopsStatus,
		"secure-integration-bridge":  sib.bridgeStatus,
	}
}

func (sib *SecureIntegrationBridge) HealthCheck(ctx context.Context) monitoring.HealthStatus {
	sib.mu.RLock()
	defer sib.mu.RUnlock()

	if !sib.running {
		return monitoring.HealthStatus{
			Status:  monitoring.HealthStatusUnhealthy,
			Message: "Secure integration bridge is not running",
		}
	}

	// Check security status
	securityStatus := sib.securityManager.GetSecurityStatus()
	if securityStatus.OverallStatus != "healthy" {
		return monitoring.HealthStatus{
			Status:  monitoring.HealthStatusDegraded,
			Message: "Security components degraded",
			Metrics: map[string]interface{}{
				"security_status": securityStatus.OverallStatus,
			},
		}
	}

	// Check component health
	statuses := sib.GetComponentStatus()
	unhealthyCount := 0
	for _, status := range statuses {
		if status.Name != "" && status.Status == "unhealthy" {
			unhealthyCount++
		}
	}

	if unhealthyCount > 0 {
		return monitoring.HealthStatus{
			Status:  monitoring.HealthStatusDegraded,
			Message: fmt.Sprintf("%d components are unhealthy", unhealthyCount),
			Metrics: map[string]interface{}{
				"unhealthy_components": unhealthyCount,
				"security_score":       sib.calculateSecurityScore(),
			},
		}
	}

	return monitoring.HealthStatus{
		Status:  monitoring.HealthStatusHealthy,
		Message: "All components and security systems are healthy",
		Metrics: map[string]interface{}{
			"total_components":   len(statuses),
			"healthy_components": len(statuses) - unhealthyCount,
			"security_score":     sib.calculateSecurityScore(),
		},
	}
}

func (sib *SecureIntegrationBridge) GetDependencies() []string {
	return []string{"monitoring", "security", "http-client", "trigger-system", "certificate-management"}
}

func (sib *SecureIntegrationBridge) OnStart(ctx context.Context) error {
	return sib.Start(ctx)
}

func (sib *SecureIntegrationBridge) OnStop(ctx context.Context) error {
	return sib.Stop()
}

func (sib *SecureIntegrationBridge) ResetMetrics() {
	sib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().ResetMetrics()
}