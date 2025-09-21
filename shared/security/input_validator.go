package security

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// ValidationLevel defines the strictness of validation
type ValidationLevel string

const (
	ValidationLevelPermissive ValidationLevel = "permissive"
	ValidationLevelStandard   ValidationLevel = "standard"
	ValidationLevelStrict     ValidationLevel = "strict"
	ValidationLevelParanoid   ValidationLevel = "paranoid"
)

// InputType defines the type of input being validated
type InputType string

const (
	InputTypeString     InputType = "string"
	InputTypeEmail      InputType = "email"
	InputTypeURL        InputType = "url"
	InputTypeFilePath   InputType = "filepath"
	InputTypeIPAddress  InputType = "ip"
	InputTypePort       InputType = "port"
	InputTypeJSON       InputType = "json"
	InputTypeXML        InputType = "xml"
	InputTypeSQL        InputType = "sql"
	InputTypeCommand    InputType = "command"
	InputTypeRegex      InputType = "regex"
	InputTypeDNSName    InputType = "dns"
	InputTypeToken      InputType = "token"
	InputTypeBase64     InputType = "base64"
)

// ValidationError represents an input validation error
type ValidationError struct {
	Field     string `json:"field"`
	Value     string `json:"value"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
	Code      string `json:"code"`
	Sanitized string `json:"sanitized,omitempty"`
}

// ValidationConfig configures input validation behavior
type ValidationConfig struct {
	Level              ValidationLevel `yaml:"level"`
	MaxStringLength    int            `yaml:"max_string_length"`
	MaxArrayLength     int            `yaml:"max_array_length"`
	AllowedCharacters  string         `yaml:"allowed_characters"`
	BlockedPatterns    []string       `yaml:"blocked_patterns"`
	RequiredPatterns   []string       `yaml:"required_patterns"`
	SanitizeHTML       bool           `yaml:"sanitize_html"`
	SanitizeSQL        bool           `yaml:"sanitize_sql"`
	SanitizeXSS        bool           `yaml:"sanitize_xss"`
	AllowPathTraversal bool           `yaml:"allow_path_traversal"`
	AllowNullBytes     bool           `yaml:"allow_null_bytes"`
	LogViolations      bool           `yaml:"log_violations"`
}

// InputValidator provides secure input validation and sanitization
type InputValidator struct {
	config  ValidationConfig
	logger  Logger
	auditor *AuditLogger
}

// NewInputValidator creates a new input validator
func NewInputValidator(config ValidationConfig, logger Logger, auditor *AuditLogger) *InputValidator {
	return &InputValidator{
		config:  config,
		logger:  logger,
		auditor: auditor,
	}
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		Level:              ValidationLevelStrict,
		MaxStringLength:    10000,
		MaxArrayLength:     1000,
		AllowedCharacters:  "", // Empty means allow all printable
		BlockedPatterns:    DefaultBlockedPatterns(),
		RequiredPatterns:   []string{},
		SanitizeHTML:       true,
		SanitizeSQL:        true,
		SanitizeXSS:        true,
		AllowPathTraversal: false,
		AllowNullBytes:     false,
		LogViolations:      true,
	}
}

// ValidateInput validates an input value based on its type
func (iv *InputValidator) ValidateInput(field string, value interface{}, inputType InputType) (*ValidationError, error) {
	// Convert value to string for processing
	strValue, ok := value.(string)
	if !ok {
		strValue = fmt.Sprintf("%v", value)
	}

	// Basic security checks
	if err := iv.performSecurityChecks(field, strValue); err != nil {
		return err, nil
	}

	// Type-specific validation
	switch inputType {
	case InputTypeString:
		return iv.validateString(field, strValue)
	case InputTypeEmail:
		return iv.validateEmail(field, strValue)
	case InputTypeURL:
		return iv.validateURL(field, strValue)
	case InputTypeFilePath:
		return iv.validateFilePath(field, strValue)
	case InputTypeIPAddress:
		return iv.validateIPAddress(field, strValue)
	case InputTypePort:
		return iv.validatePort(field, strValue)
	case InputTypeJSON:
		return iv.validateJSON(field, strValue)
	case InputTypeXML:
		return iv.validateXML(field, strValue)
	case InputTypeSQL:
		return iv.validateSQL(field, strValue)
	case InputTypeCommand:
		return iv.validateCommand(field, strValue)
	case InputTypeRegex:
		return iv.validateRegex(field, strValue)
	case InputTypeDNSName:
		return iv.validateDNSName(field, strValue)
	case InputTypeToken:
		return iv.validateToken(field, strValue)
	case InputTypeBase64:
		return iv.validateBase64(field, strValue)
	default:
		return iv.validateString(field, strValue)
	}
}

// SanitizeInput sanitizes input by removing/escaping dangerous content
func (iv *InputValidator) SanitizeInput(value string, inputType InputType) string {
	sanitized := value

	// Remove null bytes
	if !iv.config.AllowNullBytes {
		sanitized = strings.ReplaceAll(sanitized, "\x00", "")
	}

	// Type-specific sanitization
	switch inputType {
	case InputTypeString:
		sanitized = iv.sanitizeString(sanitized)
	case InputTypeURL:
		sanitized = iv.sanitizeURL(sanitized)
	case InputTypeFilePath:
		sanitized = iv.sanitizeFilePath(sanitized)
	case InputTypeSQL:
		if iv.config.SanitizeSQL {
			sanitized = iv.sanitizeSQL(sanitized)
		}
	case InputTypeJSON:
		sanitized = iv.sanitizeJSON(sanitized)
	case InputTypeXML:
		sanitized = iv.sanitizeXML(sanitized)
	}

	// HTML sanitization
	if iv.config.SanitizeHTML {
		sanitized = iv.sanitizeHTML(sanitized)
	}

	// XSS sanitization
	if iv.config.SanitizeXSS {
		sanitized = iv.sanitizeXSS(sanitized)
	}

	return sanitized
}

// performSecurityChecks performs basic security validation
func (iv *InputValidator) performSecurityChecks(field, value string) *ValidationError {
	// Check length limits
	if len(value) > iv.config.MaxStringLength {
		return &ValidationError{
			Field:    field,
			Value:    truncateString(value, 100),
			Type:     "length_violation",
			Message:  fmt.Sprintf("Input exceeds maximum length of %d characters", iv.config.MaxStringLength),
			Severity: "medium",
			Code:     "INPUT_TOO_LONG",
		}
	}

	// Check for null bytes
	if !iv.config.AllowNullBytes && strings.Contains(value, "\x00") {
		iv.logViolation(field, value, "null_byte_detected")
		return &ValidationError{
			Field:     field,
			Value:     truncateString(value, 100),
			Type:      "null_byte_violation",
			Message:   "Null bytes not allowed in input",
			Severity:  "high",
			Code:      "NULL_BYTE_DETECTED",
			Sanitized: strings.ReplaceAll(value, "\x00", ""),
		}
	}

	// Check for blocked patterns
	for _, pattern := range iv.config.BlockedPatterns {
		if matched, _ := regexp.MatchString("(?i)"+pattern, value); matched {
			iv.logViolation(field, value, "blocked_pattern_detected")
			return &ValidationError{
				Field:    field,
				Value:    truncateString(value, 100),
				Type:     "blocked_pattern",
				Message:  fmt.Sprintf("Input contains blocked pattern: %s", pattern),
				Severity: "high",
				Code:     "BLOCKED_PATTERN",
			}
		}
	}

	// Check for path traversal
	if !iv.config.AllowPathTraversal && iv.containsPathTraversal(value) {
		iv.logViolation(field, value, "path_traversal_detected")
		return &ValidationError{
			Field:    field,
			Value:    truncateString(value, 100),
			Type:     "path_traversal",
			Message:  "Path traversal sequences detected",
			Severity: "high",
			Code:     "PATH_TRAVERSAL",
		}
	}

	// Check for control characters
	if iv.containsControlCharacters(value) {
		iv.logViolation(field, value, "control_characters_detected")
		return &ValidationError{
			Field:    field,
			Value:    truncateString(value, 100),
			Type:     "control_characters",
			Message:  "Control characters detected in input",
			Severity: "medium",
			Code:     "CONTROL_CHARS",
		}
	}

	return nil
}

// validateString validates a generic string input
func (iv *InputValidator) validateString(field, value string) (*ValidationError, error) {
	// Check allowed characters if configured
	if iv.config.AllowedCharacters != "" {
		allowedChars := iv.config.AllowedCharacters
		for _, char := range value {
			if !strings.ContainsRune(allowedChars, char) {
				return &ValidationError{
					Field:    field,
					Value:    truncateString(value, 100),
					Type:     "invalid_character",
					Message:  fmt.Sprintf("Character '%c' not allowed", char),
					Severity: "medium",
					Code:     "INVALID_CHAR",
				}, nil
			}
		}
	}

	// Check for required patterns
	for _, pattern := range iv.config.RequiredPatterns {
		if matched, _ := regexp.MatchString(pattern, value); !matched {
			return &ValidationError{
				Field:    field,
				Value:    truncateString(value, 100),
				Type:     "required_pattern_missing",
				Message:  fmt.Sprintf("Required pattern not found: %s", pattern),
				Severity: "medium",
				Code:     "PATTERN_REQUIRED",
			}, nil
		}
	}

	return nil, nil
}

// validateEmail validates an email address
func (iv *InputValidator) validateEmail(field, value string) (*ValidationError, error) {
	emailRegex := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	if matched, _ := regexp.MatchString(emailRegex, value); !matched {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_email",
			Message:  "Invalid email format",
			Severity: "medium",
			Code:     "INVALID_EMAIL",
		}, nil
	}

	// Additional email security checks
	if strings.Contains(value, "..") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_email",
			Message:  "Email contains suspicious patterns",
			Severity: "medium",
			Code:     "SUSPICIOUS_EMAIL",
		}, nil
	}

	return nil, nil
}

// validateURL validates a URL
func (iv *InputValidator) validateURL(field, value string) (*ValidationError, error) {
	parsedURL, err := url.Parse(value)
	if err != nil {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_url",
			Message:  "Invalid URL format",
			Severity: "medium",
			Code:     "INVALID_URL",
		}, nil
	}

	// Check for allowed schemes
	allowedSchemes := map[string]bool{
		"http":  true,
		"https": true,
		"ftp":   false, // Configurable
		"file":  false,
	}

	if !allowedSchemes[parsedURL.Scheme] {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_url_scheme",
			Message:  fmt.Sprintf("URL scheme '%s' not allowed", parsedURL.Scheme),
			Severity: "high",
			Code:     "INVALID_SCHEME",
		}, nil
	}

	// Check for suspicious patterns
	if strings.Contains(value, "javascript:") || strings.Contains(value, "data:") {
		iv.logViolation(field, value, "suspicious_url_scheme")
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "suspicious_url",
			Message:  "URL contains suspicious scheme",
			Severity: "high",
			Code:     "SUSPICIOUS_URL",
		}, nil
	}

	return nil, nil
}

// validateFilePath validates a file path
func (iv *InputValidator) validateFilePath(field, value string) (*ValidationError, error) {
	// Clean the path
	cleanPath := filepath.Clean(value)

	// Check for path traversal
	if !iv.config.AllowPathTraversal && iv.containsPathTraversal(value) {
		return &ValidationError{
			Field:     field,
			Value:     value,
			Type:      "path_traversal",
			Message:   "Path traversal detected",
			Severity:  "high",
			Code:      "PATH_TRAVERSAL",
			Sanitized: cleanPath,
		}, nil
	}

	// Check for absolute paths (may be restricted)
	if filepath.IsAbs(value) && iv.config.Level == ValidationLevelStrict {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "absolute_path",
			Message:  "Absolute paths not allowed",
			Severity: "medium",
			Code:     "ABSOLUTE_PATH",
		}, nil
	}

	// Check for suspicious file extensions
	suspiciousExts := []string{".exe", ".bat", ".cmd", ".scr", ".com", ".pif"}
	ext := strings.ToLower(filepath.Ext(value))
	for _, suspExt := range suspiciousExts {
		if ext == suspExt {
			iv.logViolation(field, value, "suspicious_file_extension")
			return &ValidationError{
				Field:    field,
				Value:    value,
				Type:     "suspicious_extension",
				Message:  fmt.Sprintf("Suspicious file extension: %s", ext),
				Severity: "high",
				Code:     "SUSPICIOUS_EXTENSION",
			}, nil
		}
	}

	return nil, nil
}

// validateIPAddress validates an IP address
func (iv *InputValidator) validateIPAddress(field, value string) (*ValidationError, error) {
	ip := net.ParseIP(value)
	if ip == nil {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_ip",
			Message:  "Invalid IP address format",
			Severity: "medium",
			Code:     "INVALID_IP",
		}, nil
	}

	// Check for private/internal IPs if strict validation
	if iv.config.Level == ValidationLevelStrict && (ip.IsLoopback() || ip.IsPrivate()) {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "private_ip",
			Message:  "Private IP addresses not allowed",
			Severity: "medium",
			Code:     "PRIVATE_IP",
		}, nil
	}

	return nil, nil
}

// validatePort validates a port number
func (iv *InputValidator) validatePort(field, value string) (*ValidationError, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_port",
			Message:  "Invalid port number",
			Severity: "medium",
			Code:     "INVALID_PORT",
		}, nil
	}

	if port < 1 || port > 65535 {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "port_out_of_range",
			Message:  "Port number out of valid range (1-65535)",
			Severity: "medium",
			Code:     "PORT_RANGE",
		}, nil
	}

	// Check for well-known ports if strict
	if iv.config.Level == ValidationLevelStrict && port < 1024 {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "privileged_port",
			Message:  "Privileged port numbers not allowed",
			Severity: "medium",
			Code:     "PRIVILEGED_PORT",
		}, nil
	}

	return nil, nil
}

// validateJSON validates JSON content
func (iv *InputValidator) validateJSON(field, value string) (*ValidationError, error) {
	// Check for JSON injection patterns
	jsonInjectionPatterns := []string{
		`\\"`,
		`\\u`,
		`</script>`,
		`<script>`,
		`javascript:`,
	}

	for _, pattern := range jsonInjectionPatterns {
		if strings.Contains(strings.ToLower(value), strings.ToLower(pattern)) {
			iv.logViolation(field, value, "json_injection_detected")
			return &ValidationError{
				Field:    field,
				Value:    truncateString(value, 100),
				Type:     "json_injection",
				Message:  "Potential JSON injection detected",
				Severity: "high",
				Code:     "JSON_INJECTION",
			}, nil
		}
	}

	return nil, nil
}

// validateXML validates XML content
func (iv *InputValidator) validateXML(field, value string) (*ValidationError, error) {
	// Check for XML injection patterns
	xmlInjectionPatterns := []string{
		`<!entity`,
		`<!doctype`,
		`<![cdata[`,
		`<?xml-stylesheet`,
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range xmlInjectionPatterns {
		if strings.Contains(lowerValue, pattern) {
			iv.logViolation(field, value, "xml_injection_detected")
			return &ValidationError{
				Field:    field,
				Value:    truncateString(value, 100),
				Type:     "xml_injection",
				Message:  "Potential XML injection detected",
				Severity: "high",
				Code:     "XML_INJECTION",
			}, nil
		}
	}

	return nil, nil
}

// validateSQL validates SQL content
func (iv *InputValidator) validateSQL(field, value string) (*ValidationError, error) {
	// Check for SQL injection patterns
	sqlInjectionPatterns := []string{
		`'.*or.*'.*'`,
		`'.*union.*select`,
		`'.*drop.*table`,
		`'.*insert.*into`,
		`'.*delete.*from`,
		`'.*update.*set`,
		`--`,
		`;.*drop`,
		`;.*delete`,
		`/\*.*\*/`,
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range sqlInjectionPatterns {
		if matched, _ := regexp.MatchString(pattern, lowerValue); matched {
			iv.logViolation(field, value, "sql_injection_detected")
			return &ValidationError{
				Field:    field,
				Value:    truncateString(value, 100),
				Type:     "sql_injection",
				Message:  "Potential SQL injection detected",
				Severity: "high",
				Code:     "SQL_INJECTION",
			}, nil
		}
	}

	return nil, nil
}

// validateCommand validates command input
func (iv *InputValidator) validateCommand(field, value string) (*ValidationError, error) {
	// Check for command injection patterns
	cmdInjectionPatterns := []string{
		`&&`,
		`||`,
		`;`,
		`|`,
		"`",
		`$\(`,
		`>\s*/`,
		`<\s*/`,
		`>>\s*/`,
	}

	for _, pattern := range cmdInjectionPatterns {
		if matched, _ := regexp.MatchString(pattern, value); matched {
			iv.logViolation(field, value, "command_injection_detected")
			return &ValidationError{
				Field:    field,
				Value:    truncateString(value, 100),
				Type:     "command_injection",
				Message:  "Potential command injection detected",
				Severity: "high",
				Code:     "COMMAND_INJECTION",
			}, nil
		}
	}

	return nil, nil
}

// validateRegex validates a regular expression
func (iv *InputValidator) validateRegex(field, value string) (*ValidationError, error) {
	_, err := regexp.Compile(value)
	if err != nil {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_regex",
			Message:  fmt.Sprintf("Invalid regular expression: %v", err),
			Severity: "medium",
			Code:     "INVALID_REGEX",
		}, nil
	}

	return nil, nil
}

// validateDNSName validates a DNS name
func (iv *InputValidator) validateDNSName(field, value string) (*ValidationError, error) {
	dnsRegex := `^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`
	if matched, _ := regexp.MatchString(dnsRegex, value); !matched {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "invalid_dns_name",
			Message:  "Invalid DNS name format",
			Severity: "medium",
			Code:     "INVALID_DNS",
		}, nil
	}

	// Check length
	if len(value) > 253 {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "dns_name_too_long",
			Message:  "DNS name exceeds maximum length",
			Severity: "medium",
			Code:     "DNS_TOO_LONG",
		}, nil
	}

	return nil, nil
}

// validateToken validates a token format
func (iv *InputValidator) validateToken(field, value string) (*ValidationError, error) {
	// Check for minimum length
	if len(value) < 16 {
		return &ValidationError{
			Field:    field,
			Value:    value,
			Type:     "token_too_short",
			Message:  "Token must be at least 16 characters",
			Severity: "medium",
			Code:     "TOKEN_SHORT",
		}, nil
	}

	// Check for valid token characters
	tokenRegex := `^[A-Za-z0-9\-_\.]+$`
	if matched, _ := regexp.MatchString(tokenRegex, value); !matched {
		return &ValidationError{
			Field:    field,
			Value:    truncateString(value, 20),
			Type:     "invalid_token_format",
			Message:  "Token contains invalid characters",
			Severity: "medium",
			Code:     "INVALID_TOKEN",
		}, nil
	}

	return nil, nil
}

// validateBase64 validates base64 encoded content
func (iv *InputValidator) validateBase64(field, value string) (*ValidationError, error) {
	base64Regex := `^[A-Za-z0-9+/]*={0,2}$`
	if matched, _ := regexp.MatchString(base64Regex, value); !matched {
		return &ValidationError{
			Field:    field,
			Value:    truncateString(value, 50),
			Type:     "invalid_base64",
			Message:  "Invalid Base64 format",
			Severity: "medium",
			Code:     "INVALID_BASE64",
		}, nil
	}

	return nil, nil
}

// Sanitization methods

func (iv *InputValidator) sanitizeString(value string) string {
	// Remove control characters except \n, \r, \t
	var result strings.Builder
	for _, r := range value {
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func (iv *InputValidator) sanitizeURL(value string) string {
	// Basic URL sanitization
	sanitized := strings.TrimSpace(value)
	// Remove dangerous schemes
	if strings.HasPrefix(strings.ToLower(sanitized), "javascript:") ||
		strings.HasPrefix(strings.ToLower(sanitized), "data:") ||
		strings.HasPrefix(strings.ToLower(sanitized), "vbscript:") {
		return ""
	}
	return sanitized
}

func (iv *InputValidator) sanitizeFilePath(value string) string {
	// Clean the path and remove dangerous sequences
	cleaned := filepath.Clean(value)
	// Remove path traversal sequences
	cleaned = strings.ReplaceAll(cleaned, "../", "")
	cleaned = strings.ReplaceAll(cleaned, "..\\", "")
	return cleaned
}

func (iv *InputValidator) sanitizeSQL(value string) string {
	// Escape SQL special characters
	replacer := strings.NewReplacer(
		"'", "''",
		"--", "",
		";", "",
		"/*", "",
		"*/", "",
	)
	return replacer.Replace(value)
}

func (iv *InputValidator) sanitizeJSON(value string) string {
	// Escape JSON special characters
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return replacer.Replace(value)
}

func (iv *InputValidator) sanitizeXML(value string) string {
	// Escape XML special characters
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func (iv *InputValidator) sanitizeHTML(value string) string {
	// Basic HTML sanitization
	replacer := strings.NewReplacer(
		"<script", "&lt;script",
		"</script>", "&lt;/script&gt;",
		"<iframe", "&lt;iframe",
		"</iframe>", "&lt;/iframe&gt;",
		"<object", "&lt;object",
		"</object>", "&lt;/object&gt;",
		"<embed", "&lt;embed",
		"</embed>", "&lt;/embed&gt;",
		"javascript:", "",
		"vbscript:", "",
	)
	return replacer.Replace(value)
}

func (iv *InputValidator) sanitizeXSS(value string) string {
	// XSS sanitization
	xssPatterns := map[string]string{
		`<script.*?>.*?</script>`: "",
		`javascript:.*?["\s]`:     "",
		`on\w+\s*=.*?["\s]`:       "",
		`<iframe.*?>.*?</iframe>`: "",
		`<object.*?>.*?</object>`: "",
		`<embed.*?/>`:             "",
	}

	sanitized := value
	for pattern, replacement := range xssPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		sanitized = re.ReplaceAllString(sanitized, replacement)
	}

	return sanitized
}

// Helper methods

func (iv *InputValidator) containsPathTraversal(value string) bool {
	pathTraversalPatterns := []string{
		"../",
		"..\\",
		"%2e%2e%2f",
		"%2e%2e\\",
		"..%2f",
		"..%5c",
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range pathTraversalPatterns {
		if strings.Contains(lowerValue, pattern) {
			return true
		}
	}
	return false
}

func (iv *InputValidator) containsControlCharacters(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
	}
	return false
}

func (iv *InputValidator) logViolation(field, value, violationType string) {
	if iv.config.LogViolations && iv.auditor != nil {
		iv.auditor.LogSecurityViolation("", violationType, 
			fmt.Sprintf("Field: %s, Value: %s", field, truncateString(value, 100)), "medium")
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DefaultBlockedPatterns returns common blocked patterns
func DefaultBlockedPatterns() []string {
	return []string{
		// Script injection
		`<script.*?>`,
		`</script>`,
		`javascript:`,
		`vbscript:`,
		`onload\s*=`,
		`onerror\s*=`,
		`onclick\s*=`,
		
		// SQL injection
		`\bselect\b.*\bfrom\b`,
		`\bunion\b.*\bselect\b`,
		`\bdrop\b.*\btable\b`,
		`\binsert\b.*\binto\b`,
		`\bdelete\b.*\bfrom\b`,
		`\bupdate\b.*\bset\b`,
		
		// Command injection
		`\s*&&\s*`,
		`\s*\|\|\s*`,
		`\s*;\s*`,
		`\$\(.*\)`,
		"`.*`",
		
		// Path traversal
		`\.\.\/`,
		`\.\.\\`,
		`%2e%2e%2f`,
		
		// LDAP injection
		`\(\s*\|\s*\(`,
		`\)\s*\(\s*\|`,
		
		// XML/XXE
		`<!entity`,
		`<!doctype`,
		`<\?xml-stylesheet`,
		
		// NoSQL injection
		`\$where\s*:`,
		`\$ne\s*:`,
		`\$gt\s*:`,
	}
}