package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEvent represents a security audit event
type AuditEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"event_type"`
	UserID      string                 `json:"user_id,omitempty"`
	SourceIP    string                 `json:"source_ip,omitempty"`
	Action      string                 `json:"action"`
	Resource    string                 `json:"resource"`
	ResourceID  string                 `json:"resource_id,omitempty"`
	Result      string                 `json:"result"` // success, failure, denied
	Message     string                 `json:"message,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Severity    string                 `json:"severity"` // low, medium, high, critical
	Component   string                 `json:"component"`
	SessionID   string                 `json:"session_id,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Fingerprint string                 `json:"fingerprint,omitempty"`
}

// AuditLogger provides secure audit logging capabilities
type AuditLogger struct {
	config  AuditConfig
	logger  Logger
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config AuditConfig, logger Logger) *AuditLogger {
	auditor := &AuditLogger{
		config: config,
		logger: logger,
	}

	if err := auditor.initializeLogFile(); err != nil {
		logger.Error("failed to initialize audit log file", map[string]interface{}{
			"error": err.Error(),
		})
	}

	return auditor
}

// LogSecretAccess logs secret access events
func (al *AuditLogger) LogSecretAccess(action, secretID, secretName, result, message string) {
	event := AuditEvent{
		Timestamp:  time.Now(),
		EventType:  "secret_access",
		Action:     action,
		Resource:   "secret",
		ResourceID: secretID,
		Result:     result,
		Message:    message,
		Component:  "secrets_manager",
		Severity:   al.getSeverityForSecretAction(action, result),
		Details: map[string]interface{}{
			"secret_name": secretName,
		},
	}

	al.logEvent(event)
}

// LogAuthenticationEvent logs authentication events
func (al *AuditLogger) LogAuthenticationEvent(userID, sourceIP, method, result, message string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "authentication",
		UserID:    userID,
		SourceIP:  sourceIP,
		Action:    "authenticate",
		Resource:  "system",
		Result:    result,
		Message:   message,
		Component: "auth_manager",
		Severity:  al.getSeverityForAuthResult(result),
		Details: map[string]interface{}{
			"auth_method": method,
		},
	}

	al.logEvent(event)
}

// LogAuthorizationEvent logs authorization events
func (al *AuditLogger) LogAuthorizationEvent(userID, sourceIP, action, resource, result, message string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "authorization",
		UserID:    userID,
		SourceIP:  sourceIP,
		Action:    action,
		Resource:  resource,
		Result:    result,
		Message:   message,
		Component: "auth_manager",
		Severity:  al.getSeverityForAuthResult(result),
	}

	al.logEvent(event)
}

// LogConfigurationChange logs configuration changes
func (al *AuditLogger) LogConfigurationChange(userID, sourceIP, configPath, action, result, message string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "configuration_change",
		UserID:    userID,
		SourceIP:  sourceIP,
		Action:    action,
		Resource:  "configuration",
		ResourceID: configPath,
		Result:    result,
		Message:   message,
		Component: "config_manager",
		Severity:  "medium",
	}

	al.logEvent(event)
}

// LogSecurityViolation logs security violations
func (al *AuditLogger) LogSecurityViolation(sourceIP, violationType, details, severity string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "security_violation",
		SourceIP:  sourceIP,
		Action:    "violation_detected",
		Resource:  "system",
		Result:    "blocked",
		Message:   fmt.Sprintf("Security violation detected: %s", violationType),
		Component: "security_monitor",
		Severity:  severity,
		Details: map[string]interface{}{
			"violation_type": violationType,
			"violation_details": details,
		},
	}

	al.logEvent(event)
}

// LogWebhookEvent logs webhook-related events
func (al *AuditLogger) LogWebhookEvent(sourceIP, endpoint, method, result, userAgent string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "webhook_access",
		SourceIP:  sourceIP,
		Action:    method,
		Resource:  "webhook",
		ResourceID: endpoint,
		Result:    result,
		Message:   fmt.Sprintf("Webhook %s %s", method, endpoint),
		Component: "webhook_handler",
		UserAgent: userAgent,
		Severity:  al.getSeverityForWebhookResult(result),
	}

	al.logEvent(event)
}

// LogFileAccess logs file access events
func (al *AuditLogger) LogFileAccess(userID, filePath, action, result string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "file_access",
		UserID:    userID,
		Action:    action,
		Resource:  "file",
		ResourceID: filePath,
		Result:    result,
		Component: "file_manager",
		Severity:  "low",
		Details: map[string]interface{}{
			"file_path": filePath,
		},
	}

	al.logEvent(event)
}

// LogNetworkEvent logs network-related events
func (al *AuditLogger) LogNetworkEvent(sourceIP, destIP, port, protocol, action, result string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "network_access",
		SourceIP:  sourceIP,
		Action:    action,
		Resource:  "network",
		Result:    result,
		Component: "network_monitor",
		Severity:  al.getSeverityForNetworkResult(result),
		Details: map[string]interface{}{
			"destination_ip": destIP,
			"port":          port,
			"protocol":      protocol,
		},
	}

	al.logEvent(event)
}

// LogSystemEvent logs system-level events
func (al *AuditLogger) LogSystemEvent(action, component, result, message string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "system_event",
		Action:    action,
		Resource:  "system",
		Result:    result,
		Message:   message,
		Component: component,
		Severity:  "medium",
	}

	al.logEvent(event)
}

// LogSecretScan logs secret scanning events
func (al *AuditLogger) LogSecretScan(filePath string, secretsFound int, severity string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "secret_scan",
		Action:    "scan_file",
		Resource:  "file",
		ResourceID: filePath,
		Result:    "completed",
		Message:   fmt.Sprintf("Secret scan found %d potential secrets", secretsFound),
		Component: "secret_scanner",
		Severity:  severity,
		Details: map[string]interface{}{
			"secrets_found": secretsFound,
			"file_path":    filePath,
		},
	}

	al.logEvent(event)
}

// logEvent writes an audit event to the log
func (al *AuditLogger) logEvent(event AuditEvent) {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Add fingerprint for event correlation
	event.Fingerprint = al.generateFingerprint(event)

	// Write to audit log file
	if al.encoder != nil {
		if err := al.encoder.Encode(event); err != nil {
			al.logger.Error("failed to write audit event", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	// Also log to application logger based on severity
	switch event.Severity {
	case "critical", "high":
		al.logger.Error("audit_event", map[string]interface{}{
			"event_type": event.EventType,
			"action":     event.Action,
			"result":     event.Result,
			"message":    event.Message,
		})
	case "medium":
		al.logger.Warn("audit_event", map[string]interface{}{
			"event_type": event.EventType,
			"action":     event.Action,
			"result":     event.Result,
		})
	default:
		al.logger.Info("audit_event", map[string]interface{}{
			"event_type": event.EventType,
			"action":     event.Action,
			"result":     event.Result,
		})
	}
}

// initializeLogFile initializes the audit log file
func (al *AuditLogger) initializeLogFile() error {
	if al.config.LogPath == "" {
		return fmt.Errorf("audit log path not configured")
	}

	// Create log directory if it doesn't exist
	logDir := filepath.Dir(al.config.LogPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
	}

	// Open log file with append mode
	file, err := os.OpenFile(al.config.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %v", err)
	}

	al.file = file
	al.encoder = json.NewEncoder(file)

	// Log audit logger initialization
	initEvent := AuditEvent{
		Timestamp: time.Now(),
		EventType: "system_event",
		Action:    "audit_logger_initialized",
		Resource:  "system",
		Result:    "success",
		Component: "audit_logger",
		Severity:  "low",
		Message:   "Audit logger initialized successfully",
	}

	return al.encoder.Encode(initEvent)
}

// generateFingerprint generates a unique fingerprint for event correlation
func (al *AuditLogger) generateFingerprint(event AuditEvent) string {
	// Create a simple fingerprint based on event characteristics
	fingerprint := fmt.Sprintf("%s_%s_%s_%s_%d",
		event.EventType,
		event.Action,
		event.Resource,
		event.Result,
		event.Timestamp.Unix())
	
	// In production, use a proper hash function
	return fingerprint[:16]
}

// Severity assessment methods

func (al *AuditLogger) getSeverityForSecretAction(action, result string) string {
	if result == "failure" {
		return "high"
	}
	
	switch action {
	case "get", "decrypt":
		return "medium"
	case "store", "rotate", "delete":
		return "high"
	default:
		return "low"
	}
}

func (al *AuditLogger) getSeverityForAuthResult(result string) string {
	switch result {
	case "failure", "denied":
		return "high"
	case "success":
		return "medium"
	default:
		return "low"
	}
}

func (al *AuditLogger) getSeverityForWebhookResult(result string) string {
	switch result {
	case "unauthorized", "forbidden":
		return "high"
	case "error", "failure":
		return "medium"
	default:
		return "low"
	}
}

func (al *AuditLogger) getSeverityForNetworkResult(result string) string {
	switch result {
	case "blocked", "denied":
		return "medium"
	case "suspicious":
		return "high"
	default:
		return "low"
	}
}

// Close closes the audit logger
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.file != nil {
		// Log audit logger shutdown
		shutdownEvent := AuditEvent{
			Timestamp: time.Now(),
			EventType: "system_event",
			Action:    "audit_logger_shutdown",
			Resource:  "system",
			Result:    "success",
			Component: "audit_logger",
			Severity:  "low",
			Message:   "Audit logger shutdown",
		}
		
		al.encoder.Encode(shutdownEvent)
		return al.file.Close()
	}
	
	return nil
}

// GetAuditConfig returns the audit configuration
func (al *AuditLogger) GetAuditConfig() AuditConfig {
	return al.config
}