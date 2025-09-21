package security

import (
	"context"
	"fmt"
	"sync"
	"time"

	sharedconfig "shared-config/config"
)

// SecurityManager coordinates all security components
type SecurityManager struct {
	config             SecurityConfig
	logger             Logger
	secretsManager     *SecretsManager
	inputValidator     *InputValidator
	authManager        *AuthManager
	tlsManager         *TLSManager
	vulnerabilityScanner *VulnerabilityScanner
	auditLogger        *AuditLogger
	secretScanner      *SecretScanner
	initialized        bool
	mu                 sync.RWMutex
}

// SecurityConfig aggregates all security configurations
type SecurityConfig struct {
	Enabled                bool                    `yaml:"enabled"`
	StrictMode             bool                    `yaml:"strict_mode"`
	SecretsManagement      SecretsManagerConfig    `yaml:"secrets_management"`
	InputValidation        ValidationConfig        `yaml:"input_validation"`
	Authentication         AuthConfig              `yaml:"authentication"`
	TLS                    TLSConfig               `yaml:"tls"`
	VulnerabilityScanning  ScanConfig              `yaml:"vulnerability_scanning"`
	Audit                  AuditConfig             `yaml:"audit"`
	ComplianceFrameworks   []string                `yaml:"compliance_frameworks"`
	SecurityPolicies       SecurityPoliciesConfig  `yaml:"security_policies"`
	IncidentResponse       IncidentResponseConfig  `yaml:"incident_response"`
}

// SecurityPoliciesConfig defines security policies
type SecurityPoliciesConfig struct {
	MaxFailedAttempts     int           `yaml:"max_failed_attempts"`
	LockoutDuration       time.Duration `yaml:"lockout_duration"`
	PasswordRotationDays  int           `yaml:"password_rotation_days"`
	SessionTimeoutMinutes int           `yaml:"session_timeout_minutes"`
	RequireMFA            bool          `yaml:"require_mfa"`
	AllowedIPRanges       []string      `yaml:"allowed_ip_ranges"`
	BlockedIPRanges       []string      `yaml:"blocked_ip_ranges"`
	DataRetentionDays     int           `yaml:"data_retention_days"`
}

// IncidentResponseConfig configures incident response
type IncidentResponseConfig struct {
	Enabled           bool          `yaml:"enabled"`
	AlertThresholds   AlertConfig   `yaml:"alert_thresholds"`
	NotificationURL   string        `yaml:"notification_url"`
	EscalationRules   []EscalationRule `yaml:"escalation_rules"`
	AutoResponse      bool          `yaml:"auto_response"`
	QuarantineMode    bool          `yaml:"quarantine_mode"`
}

// AlertConfig defines alerting thresholds
type AlertConfig struct {
	FailedAuthPerMinute    int `yaml:"failed_auth_per_minute"`
	VulnerabilitiesFound   int `yaml:"vulnerabilities_found"`
	SuspiciousPatterns     int `yaml:"suspicious_patterns"`
	UnauthorizedAccess     int `yaml:"unauthorized_access"`
}

// EscalationRule defines incident escalation
type EscalationRule struct {
	Severity      string        `yaml:"severity"`
	TimeLimit     time.Duration `yaml:"time_limit"`
	NotifyEmail   string        `yaml:"notify_email"`
	AutoEscalate  bool          `yaml:"auto_escalate"`
}

// SecurityStatus represents the overall security status
type SecurityStatus struct {
	Timestamp              time.Time              `json:"timestamp"`
	OverallStatus          string                 `json:"overall_status"`
	SecretsManagement      ComponentStatus        `json:"secrets_management"`
	Authentication         ComponentStatus        `json:"authentication"`
	TLS                    ComponentStatus        `json:"tls"`
	VulnerabilityScanning  ComponentStatus        `json:"vulnerability_scanning"`
	AuditLogging           ComponentStatus        `json:"audit_logging"`
	ComplianceStatus       map[string]string      `json:"compliance_status"`
	RecentIncidents        []SecurityIncident     `json:"recent_incidents"`
	Recommendations        []string               `json:"recommendations"`
}

// ComponentStatus represents the status of a security component
type ComponentStatus struct {
	Enabled       bool      `json:"enabled"`
	Status        string    `json:"status"`
	LastCheck     time.Time `json:"last_check"`
	Issues        []string  `json:"issues"`
	Health        string    `json:"health"`
}

// SecurityIncident represents a security incident
type SecurityIncident struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Severity    string            `json:"severity"`
	Description string            `json:"description"`
	Timestamp   time.Time         `json:"timestamp"`
	Status      string            `json:"status"`
	Actions     []string          `json:"actions"`
	Metadata    map[string]string `json:"metadata"`
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config SecurityConfig, logger Logger) (*SecurityManager, error) {
	sm := &SecurityManager{
		config: config,
		logger: logger,
	}

	if err := sm.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize security manager: %v", err)
	}

	return sm, nil
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Enabled:               true,
		StrictMode:            true,
		SecretsManagement:     *DefaultSecretsManagerConfig(),
		InputValidation:       DefaultValidationConfig(),
		Authentication:        DefaultAuthConfig(),
		TLS:                   DefaultTLSConfig(),
		VulnerabilityScanning: DefaultScanConfig(),
		Audit:                 AuditConfig{
			Enabled:    true,
			LogPath:    "/var/log/backup-gitops/security-audit.log",
			LogLevel:   "info",
			MaxSize:    100,
			MaxBackups: 10,
			MaxAge:     30,
		},
		ComplianceFrameworks: []string{"SOC2", "ISO27001", "NIST"},
		SecurityPolicies: SecurityPoliciesConfig{
			MaxFailedAttempts:     5,
			LockoutDuration:       15 * time.Minute,
			PasswordRotationDays:  90,
			SessionTimeoutMinutes: 30,
			RequireMFA:            false,
			DataRetentionDays:     365,
		},
		IncidentResponse: IncidentResponseConfig{
			Enabled: true,
			AlertThresholds: AlertConfig{
				FailedAuthPerMinute:  10,
				VulnerabilitiesFound: 5,
				SuspiciousPatterns:   3,
				UnauthorizedAccess:   1,
			},
			AutoResponse:   true,
			QuarantineMode: false,
		},
	}
}

// initialize sets up all security components
func (sm *SecurityManager) initialize() error {
	var err error

	// Initialize audit logger first
	if sm.config.Audit.Enabled {
		sm.auditLogger = NewAuditLogger(sm.config.Audit, sm.logger)
	}

	// Initialize secrets manager
	if sm.config.SecretsManagement.Provider != "" {
		sm.secretsManager, err = NewSecretsManager(&sm.config.SecretsManagement, sm.logger)
		if err != nil {
			return fmt.Errorf("failed to initialize secrets manager: %v", err)
		}

		if sm.config.SecretsManagement.Scanning.Enabled {
			sm.secretScanner = NewSecretScanner(sm.config.SecretsManagement.Scanning, sm.logger)
		}
	}

	// Initialize input validator
	sm.inputValidator = NewInputValidator(sm.config.InputValidation, sm.logger, sm.auditLogger)

	// Initialize authentication manager
	if sm.config.Authentication.Enabled {
		sm.authManager = NewAuthManager(sm.config.Authentication, sm.logger, sm.auditLogger)
	}

	// Initialize TLS manager
	if sm.config.TLS.Enabled {
		sm.tlsManager, err = NewTLSManager(sm.config.TLS, sm.logger, sm.auditLogger)
		if err != nil {
			return fmt.Errorf("failed to initialize TLS manager: %v", err)
		}
	}

	// Initialize vulnerability scanner
	if sm.config.VulnerabilityScanning.Enabled {
		sm.vulnerabilityScanner = NewVulnerabilityScanner(sm.config.VulnerabilityScanning, sm.logger, sm.auditLogger)
	}

	sm.initialized = true

	// Log security manager initialization
	if sm.auditLogger != nil {
		sm.auditLogger.LogSystemEvent("security_manager_initialized", "security_manager", "success",
			"Security manager initialized with all components")
	}

	sm.logger.Info("security manager initialized", map[string]interface{}{
		"strict_mode":            sm.config.StrictMode,
		"secrets_management":     sm.config.SecretsManagement.Provider != "",
		"authentication":         sm.config.Authentication.Enabled,
		"tls":                    sm.config.TLS.Enabled,
		"vulnerability_scanning": sm.config.VulnerabilityScanning.Enabled,
		"audit_logging":          sm.config.Audit.Enabled,
	})

	return nil
}

// ValidateSharedConfig validates a shared configuration for security issues
func (sm *SecurityManager) ValidateSharedConfig(config *sharedconfig.SharedConfig) (*SecurityValidationResult, error) {
	result := &SecurityValidationResult{
		Timestamp:     time.Now(),
		OverallStatus: "pass",
		Issues:        []SecurityIssue{},
		Warnings:      []SecurityWarning{},
		Recommendations: []string{},
	}

	// Validate secrets in configuration
	if sm.secretScanner != nil {
		configContent, err := configToString(config)
		if err == nil {
			scanResult, err := sm.secretScanner.ScanContent(configContent)
			if err == nil && len(scanResult.Matches) > 0 {
				for _, match := range scanResult.Matches {
					issue := SecurityIssue{
						Type:        "exposed_secret",
						Severity:    "high",
						Description: fmt.Sprintf("Potential %s found in configuration", match.Type),
						Location:    fmt.Sprintf("Configuration field (line %d)", match.Line),
						Remediation: "Remove hardcoded secrets and use secure secret management",
					}
					result.Issues = append(result.Issues, issue)
					result.OverallStatus = "fail"
				}
			}
		}
	}

	// Validate authentication configuration
	if config.Security.Secrets.Provider == "env" {
		warning := SecurityWarning{
			Type:        "weak_secret_management",
			Description: "Using environment variables for secret management",
			Impact:      "Secrets may be exposed in process lists or logs",
			Recommendation: "Consider using a dedicated secret management system",
		}
		result.Warnings = append(result.Warnings, warning)
	}

	// Validate TLS configuration
	if !config.Security.Network.VerifySSL {
		issue := SecurityIssue{
			Type:        "insecure_tls",
			Severity:    "high",
			Description: "TLS verification is disabled",
			Location:    "security.network.verify_ssl",
			Remediation: "Enable TLS verification for secure communications",
		}
		result.Issues = append(result.Issues, issue)
		result.OverallStatus = "fail"
	}

	// Validate webhook authentication
	if config.Pipeline.Automation.WebhookTrigger.Enabled && 
		!config.Pipeline.Automation.WebhookTrigger.Authentication.Enabled {
		issue := SecurityIssue{
			Type:        "unauthenticated_webhook",
			Severity:    "medium",
			Description: "Webhook endpoint is not authenticated",
			Location:    "pipeline.automation.webhook_trigger.authentication",
			Remediation: "Enable webhook authentication to prevent unauthorized access",
		}
		result.Issues = append(result.Issues, issue)
		if result.OverallStatus == "pass" {
			result.OverallStatus = "warning"
		}
	}

	// Validate backup credentials
	if config.Storage.AccessKey == "minioadmin" || config.Storage.SecretKey == "minioadmin123" {
		issue := SecurityIssue{
			Type:        "default_credentials",
			Severity:    "critical",
			Description: "Default credentials detected in storage configuration",
			Location:    "storage.access_key/secret_key",
			Remediation: "Change default credentials to strong, unique values",
		}
		result.Issues = append(result.Issues, issue)
		result.OverallStatus = "fail"
	}

	// Generate recommendations
	result.Recommendations = sm.generateSecurityRecommendations(config, result)

	return result, nil
}

// SecureWebhookRequest validates and processes webhook requests securely
func (sm *SecurityManager) SecureWebhookRequest(ctx context.Context, request *WebhookRequest) (*WebhookResponse, error) {
	// Validate input
	if sm.inputValidator != nil {
		if err := sm.validateWebhookInput(request); err != nil {
			sm.auditLogger.LogWebhookEvent(request.SourceIP, request.Endpoint, request.Method, "validation_failed", request.UserAgent)
			return nil, fmt.Errorf("input validation failed: %v", err)
		}
	}

	// Authenticate request
	if sm.authManager != nil && sm.config.Authentication.Enabled {
		session, err := sm.authenticateWebhookRequest(ctx, request)
		if err != nil {
			sm.auditLogger.LogWebhookEvent(request.SourceIP, request.Endpoint, request.Method, "auth_failed", request.UserAgent)
			return nil, fmt.Errorf("authentication failed: %v", err)
		}
		request.Session = session
	}

	// Authorize request
	if sm.authManager != nil && request.Session != nil {
		if err := sm.authManager.Authorize(request.Session, ResourceWebhooks, PermissionWebhookAccess); err != nil {
			sm.auditLogger.LogWebhookEvent(request.SourceIP, request.Endpoint, request.Method, "auth_denied", request.UserAgent)
			return nil, fmt.Errorf("authorization failed: %v", err)
		}
	}

	// Process request
	response := &WebhookResponse{
		Status:    "success",
		Timestamp: time.Now(),
		RequestID: request.RequestID,
	}

	sm.auditLogger.LogWebhookEvent(request.SourceIP, request.Endpoint, request.Method, "success", request.UserAgent)

	return response, nil
}

// GetSecurityStatus returns the current security status
func (sm *SecurityManager) GetSecurityStatus() *SecurityStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status := &SecurityStatus{
		Timestamp:        time.Now(),
		OverallStatus:    "healthy",
		ComplianceStatus: make(map[string]string),
		RecentIncidents:  []SecurityIncident{},
		Recommendations:  []string{},
	}

	// Check secrets management status
	status.SecretsManagement = ComponentStatus{
		Enabled:   sm.secretsManager != nil,
		Status:    "operational",
		LastCheck: time.Now(),
		Health:    "good",
	}

	// Check authentication status
	status.Authentication = ComponentStatus{
		Enabled:   sm.authManager != nil,
		Status:    "operational",
		LastCheck: time.Now(),
		Health:    "good",
	}

	// Check TLS status
	status.TLS = ComponentStatus{
		Enabled:   sm.tlsManager != nil,
		Status:    "operational",
		LastCheck: time.Now(),
		Health:    "good",
	}

	// Check vulnerability scanning status
	status.VulnerabilityScanning = ComponentStatus{
		Enabled:   sm.vulnerabilityScanner != nil,
		Status:    "operational",
		LastCheck: time.Now(),
		Health:    "good",
	}

	// Check recent vulnerabilities
	if sm.vulnerabilityScanner != nil {
		latestScan := sm.vulnerabilityScanner.GetLatestScan()
		if latestScan != nil && latestScan.Summary.CriticalCount > 0 {
			status.OverallStatus = "warning"
			status.Recommendations = append(status.Recommendations, 
				fmt.Sprintf("Address %d critical vulnerabilities found in latest scan", latestScan.Summary.CriticalCount))
		}
	}

	// Check audit logging status
	status.AuditLogging = ComponentStatus{
		Enabled:   sm.auditLogger != nil,
		Status:    "operational",
		LastCheck: time.Now(),
		Health:    "good",
	}

	// Check compliance status
	for _, framework := range sm.config.ComplianceFrameworks {
		status.ComplianceStatus[framework] = sm.assessComplianceFramework(framework)
	}

	return status
}

// PerformSecurityScan performs a comprehensive security scan
func (sm *SecurityManager) PerformSecurityScan(ctx context.Context) (*ScanResult, error) {
	if sm.vulnerabilityScanner == nil {
		return nil, fmt.Errorf("vulnerability scanner not initialized")
	}

	return sm.vulnerabilityScanner.PerformScan(ctx, sm.config.VulnerabilityScanning.Targets)
}

// Helper types for webhook processing

type WebhookRequest struct {
	RequestID string
	Method    string
	Endpoint  string
	Headers   map[string]string
	Body      []byte
	SourceIP  string
	UserAgent string
	Session   *Session
}

type WebhookResponse struct {
	Status    string
	Message   string
	Data      interface{}
	Timestamp time.Time
	RequestID string
}

type SecurityValidationResult struct {
	Timestamp       time.Time           `json:"timestamp"`
	OverallStatus   string              `json:"overall_status"`
	Issues          []SecurityIssue     `json:"issues"`
	Warnings        []SecurityWarning   `json:"warnings"`
	Recommendations []string            `json:"recommendations"`
}

type SecurityIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Remediation string `json:"remediation"`
}

type SecurityWarning struct {
	Type           string `json:"type"`
	Description    string `json:"description"`
	Impact         string `json:"impact"`
	Recommendation string `json:"recommendation"`
}

// Helper methods

func (sm *SecurityManager) validateWebhookInput(request *WebhookRequest) error {
	// Validate method
	if err, _ := sm.inputValidator.ValidateInput("method", request.Method, InputTypeString); err != nil {
		return err
	}

	// Validate endpoint
	if err, _ := sm.inputValidator.ValidateInput("endpoint", request.Endpoint, InputTypeURL); err != nil {
		return err
	}

	// Validate headers
	for key, value := range request.Headers {
		if err, _ := sm.inputValidator.ValidateInput("header_"+key, value, InputTypeString); err != nil {
			return fmt.Errorf("invalid header %s: %v", key, err)
		}
	}

	return nil
}

func (sm *SecurityManager) authenticateWebhookRequest(ctx context.Context, request *WebhookRequest) (*Session, error) {
	// Extract authentication credentials from request
	credentials := make(map[string]string)
	credentials["source_ip"] = request.SourceIP
	credentials["user_agent"] = request.UserAgent

	// Check for authentication header
	if authHeader, exists := request.Headers["Authorization"]; exists {
		if strings.HasPrefix(authHeader, "Bearer ") {
			credentials["token"] = strings.TrimPrefix(authHeader, "Bearer ")
			return sm.authManager.Authenticate(ctx, AuthMethodBearer, credentials)
		} else if strings.HasPrefix(authHeader, "Basic ") {
			// Handle basic auth
			credentials["basic"] = strings.TrimPrefix(authHeader, "Basic ")
			return sm.authManager.Authenticate(ctx, AuthMethodBasic, credentials)
		}
	}

	// Check for API key
	if apiKey, exists := request.Headers["X-API-Key"]; exists {
		credentials["api_key"] = apiKey
		return sm.authManager.Authenticate(ctx, AuthMethodAPIKey, credentials)
	}

	return nil, fmt.Errorf("no valid authentication credentials found")
}

func (sm *SecurityManager) generateSecurityRecommendations(config *sharedconfig.SharedConfig, result *SecurityValidationResult) []string {
	var recommendations []string

	// Generate recommendations based on issues found
	for _, issue := range result.Issues {
		switch issue.Type {
		case "exposed_secret":
			recommendations = append(recommendations, "Implement proper secrets management using HashiCorp Vault or AWS Secrets Manager")
		case "insecure_tls":
			recommendations = append(recommendations, "Enable TLS verification and use strong cipher suites")
		case "default_credentials":
			recommendations = append(recommendations, "Change all default credentials to strong, unique values")
		case "unauthenticated_webhook":
			recommendations = append(recommendations, "Implement webhook authentication using API keys or tokens")
		}
	}

	// General security recommendations
	if len(result.Issues) == 0 {
		recommendations = append(recommendations, "Consider implementing certificate pinning for enhanced security")
		recommendations = append(recommendations, "Enable regular vulnerability scanning and monitoring")
		recommendations = append(recommendations, "Implement comprehensive audit logging")
	}

	return recommendations
}

func (sm *SecurityManager) assessComplianceFramework(framework string) string {
	switch framework {
	case "SOC2":
		return sm.assessSOC2Compliance()
	case "ISO27001":
		return sm.assessISO27001Compliance()
	case "NIST":
		return sm.assessNISTCompliance()
	default:
		return "unknown"
	}
}

func (sm *SecurityManager) assessSOC2Compliance() string {
	// Assess SOC2 Type II compliance
	score := 0
	total := 5

	if sm.config.Authentication.Enabled {
		score++
	}
	if sm.config.TLS.Enabled {
		score++
	}
	if sm.config.Audit.Enabled {
		score++
	}
	if sm.config.VulnerabilityScanning.Enabled {
		score++
	}
	if sm.config.SecretsManagement.Provider != "env" {
		score++
	}

	percentage := float64(score) / float64(total) * 100
	if percentage >= 80 {
		return "compliant"
	} else if percentage >= 60 {
		return "partially_compliant"
	}
	return "non_compliant"
}

func (sm *SecurityManager) assessISO27001Compliance() string {
	// Simplified ISO27001 assessment
	return "partially_compliant"
}

func (sm *SecurityManager) assessNISTCompliance() string {
	// Simplified NIST Cybersecurity Framework assessment
	return "partially_compliant"
}

func configToString(config *sharedconfig.SharedConfig) (string, error) {
	// Convert configuration to string for scanning
	// This is a simplified implementation
	content := fmt.Sprintf("storage.access_key=%s\n", config.Storage.AccessKey)
	content += fmt.Sprintf("storage.secret_key=%s\n", config.Storage.SecretKey)
	content += fmt.Sprintf("gitops.repository.auth.ssh.passphrase=%s\n", config.GitOps.Repository.Auth.SSH.Passphrase)
	content += fmt.Sprintf("gitops.repository.auth.pat.token=%s\n", config.GitOps.Repository.Auth.PAT.Token)
	content += fmt.Sprintf("gitops.repository.auth.basic.password=%s\n", config.GitOps.Repository.Auth.Basic.Password)
	return content, nil
}

// GetSecretsManager returns the secrets manager instance
func (sm *SecurityManager) GetSecretsManager() *SecretsManager {
	return sm.secretsManager
}

// GetAuthManager returns the authentication manager instance
func (sm *SecurityManager) GetAuthManager() *AuthManager {
	return sm.authManager
}

// GetTLSManager returns the TLS manager instance
func (sm *SecurityManager) GetTLSManager() *TLSManager {
	return sm.tlsManager
}

// GetInputValidator returns the input validator instance
func (sm *SecurityManager) GetInputValidator() *InputValidator {
	return sm.inputValidator
}

// GetVulnerabilityScanner returns the vulnerability scanner instance
func (sm *SecurityManager) GetVulnerabilityScanner() *VulnerabilityScanner {
	return sm.vulnerabilityScanner
}

// IsInitialized returns whether the security manager is fully initialized
func (sm *SecurityManager) IsInitialized() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.initialized
}