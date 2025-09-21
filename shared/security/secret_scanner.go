package security

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SecretMatch represents a detected secret in content
type SecretMatch struct {
	Type        string `json:"type"`
	Pattern     string `json:"pattern"`
	Match       string `json:"match"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Context     string `json:"context"`
	Severity    string `json:"severity"`
	Confidence  int    `json:"confidence"`
	Redacted    string `json:"redacted"`
}

// SecretScanResult represents the result of a secret scan
type SecretScanResult struct {
	FilePath    string         `json:"file_path,omitempty"`
	Matches     []SecretMatch  `json:"matches"`
	ScanTime    time.Time      `json:"scan_time"`
	Duration    time.Duration  `json:"duration"`
	LinesScanned int           `json:"lines_scanned"`
	Summary     ScanSummary    `json:"summary"`
}

// ScanSummary provides summary statistics for a scan
type ScanSummary struct {
	TotalMatches int `json:"total_matches"`
	HighSeverity int `json:"high_severity"`
	MediumSeverity int `json:"medium_severity"`
	LowSeverity  int `json:"low_severity"`
	UniqueTypes  int `json:"unique_types"`
}

// SecretPattern represents a pattern for detecting secrets
type SecretPattern struct {
	Name        string  `json:"name"`
	Pattern     string  `json:"pattern"`
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Description string  `json:"description"`
	Confidence  int     `json:"confidence"`
	Enabled     bool    `json:"enabled"`
}

// SecretScanner scans content for exposed secrets
type SecretScanner struct {
	config   ScanningConfig
	patterns []SecretPattern
	compiled []*regexp.Regexp
	logger   Logger
}

// NewSecretScanner creates a new secret scanner
func NewSecretScanner(config ScanningConfig, logger Logger) *SecretScanner {
	scanner := &SecretScanner{
		config: config,
		logger: logger,
	}

	// Load and compile patterns
	scanner.loadPatterns()
	scanner.compilePatterns()

	return scanner
}

// ScanContent scans text content for secrets
func (ss *SecretScanner) ScanContent(content string) (*SecretScanResult, error) {
	startTime := time.Now()
	
	result := &SecretScanResult{
		ScanTime: startTime,
		Matches:  []SecretMatch{},
		Summary:  ScanSummary{},
	}

	lines := strings.Split(content, "\n")
	result.LinesScanned = len(lines)

	// Scan each line with each pattern
	for lineNum, line := range lines {
		for i, pattern := range ss.patterns {
			if !pattern.Enabled {
				continue
			}

			// Check exclusions
			if ss.isExcluded(line) {
				continue
			}

			// Apply pattern
			matches := ss.compiled[i].FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) > 1 {
					secretMatch := SecretMatch{
						Type:       pattern.Type,
						Pattern:    pattern.Name,
						Match:      match[1], // First capture group
						Line:       lineNum + 1,
						Column:     strings.Index(line, match[1]) + 1,
						Context:    ss.getContext(lines, lineNum, 2),
						Severity:   pattern.Severity,
						Confidence: pattern.Confidence,
						Redacted:   ss.redactSecret(match[1]),
					}

					result.Matches = append(result.Matches, secretMatch)
				}
			}
		}
	}

	// Calculate summary
	result.Summary = ss.calculateSummary(result.Matches)
	result.Duration = time.Since(startTime)

	// Log scan results
	ss.logger.Info("secret scan completed", map[string]interface{}{
		"lines_scanned":  result.LinesScanned,
		"matches_found":  result.Summary.TotalMatches,
		"high_severity":  result.Summary.HighSeverity,
		"scan_duration":  result.Duration,
	})

	return result, nil
}

// loadPatterns loads secret detection patterns
func (ss *SecretScanner) loadPatterns() {
	ss.patterns = []SecretPattern{
		{
			Name:        "AWS Access Key ID",
			Pattern:     `(?i)aws[_-]?access[_-]?key[_-]?id["']?\s*[:=]\s*["']?([A-Z0-9]{20})`,
			Type:        "aws_access_key",
			Severity:    "HIGH",
			Description: "AWS Access Key ID detected",
			Confidence:  95,
			Enabled:     true,
		},
		{
			Name:        "AWS Secret Access Key",
			Pattern:     `(?i)aws[_-]?secret[_-]?access[_-]?key["']?\s*[:=]\s*["']?([A-Za-z0-9/+=]{40})`,
			Type:        "aws_secret_key",
			Severity:    "HIGH",
			Description: "AWS Secret Access Key detected",
			Confidence:  95,
			Enabled:     true,
		},
		{
			Name:        "GitHub Token",
			Pattern:     `(?i)github[_-]?token["']?\s*[:=]\s*["']?([a-zA-Z0-9]{40})`,
			Type:        "github_token",
			Severity:    "HIGH",
			Description: "GitHub Personal Access Token detected",
			Confidence:  90,
			Enabled:     true,
		},
		{
			Name:        "Google API Key",
			Pattern:     `(?i)google[_-]?api[_-]?key["']?\s*[:=]\s*["']?([A-Za-z0-9-_]{39})`,
			Type:        "google_api_key",
			Severity:    "HIGH",
			Description: "Google API Key detected",
			Confidence:  85,
			Enabled:     true,
		},
		{
			Name:        "Generic Secret",
			Pattern:     `(?i)secret["']?\s*[:=]\s*["']?([A-Za-z0-9-_!@#$%^&*]{16,})`,
			Type:        "generic_secret",
			Severity:    "MEDIUM",
			Description: "Generic secret pattern detected",
			Confidence:  70,
			Enabled:     true,
		},
		{
			Name:        "Generic Token",
			Pattern:     `(?i)token["']?\s*[:=]\s*["']?([A-Za-z0-9-_]{32,})`,
			Type:        "generic_token",
			Severity:    "MEDIUM",
			Description: "Generic token pattern detected",
			Confidence:  75,
			Enabled:     true,
		},
		{
			Name:        "Password",
			Pattern:     `(?i)password["']?\s*[:=]\s*["']?([A-Za-z0-9-_!@#$%^&*]{8,})`,
			Type:        "password",
			Severity:    "MEDIUM",
			Description: "Password detected",
			Confidence:  60,
			Enabled:     true,
		},
		{
			Name:        "Database Connection String",
			Pattern:     `(?i)(postgres|mysql|mongodb)://[a-zA-Z0-9_-]+:[a-zA-Z0-9_-]+@[a-zA-Z0-9.-]+`,
			Type:        "database_connection",
			Severity:    "HIGH",
			Description: "Database connection string with credentials",
			Confidence:  90,
			Enabled:     true,
		},
		{
			Name:        "Private Key",
			Pattern:     `-----BEGIN[A-Z ]+PRIVATE KEY-----`,
			Type:        "private_key",
			Severity:    "HIGH",
			Description: "Private key detected",
			Confidence:  100,
			Enabled:     true,
		},
		{
			Name:        "JWT Token",
			Pattern:     `eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`,
			Type:        "jwt_token",
			Severity:    "MEDIUM",
			Description: "JWT token detected",
			Confidence:  85,
			Enabled:     true,
		},
		{
			Name:        "MinIO/S3 Access Key",
			Pattern:     `(?i)(?:minio|s3)[_-]?access[_-]?key["']?\s*[:=]\s*["']?([A-Za-z0-9]{20,})`,
			Type:        "minio_access_key",
			Severity:    "HIGH",
			Description: "MinIO/S3 Access Key detected",
			Confidence:  95,
			Enabled:     true,
		},
		{
			Name:        "MinIO/S3 Secret Key",
			Pattern:     `(?i)(?:minio|s3)[_-]?secret[_-]?key["']?\s*[:=]\s*["']?([A-Za-z0-9/+=]{40,})`,
			Type:        "minio_secret_key",
			Severity:    "HIGH",
			Description: "MinIO/S3 Secret Key detected",
			Confidence:  95,
			Enabled:     true,
		},
		{
			Name:        "Webhook URL with Token",
			Pattern:     `https?://[^/]+/webhook/[^?\s]*\?[^&]*token=([A-Za-z0-9-_]{16,})`,
			Type:        "webhook_token",
			Severity:    "MEDIUM",
			Description: "Webhook URL with authentication token",
			Confidence:  80,
			Enabled:     true,
		},
		{
			Name:        "SSH Private Key",
			Pattern:     `-----BEGIN OPENSSH PRIVATE KEY-----`,
			Type:        "ssh_private_key",
			Severity:    "HIGH",
			Description: "SSH private key detected",
			Confidence:  100,
			Enabled:     true,
		},
		{
			Name:        "Base64 Encoded Secret",
			Pattern:     `(?i)(?:secret|token|key|password)["']?\s*[:=]\s*["']?([A-Za-z0-9+/]{32,}={0,2})`,
			Type:        "base64_secret",
			Severity:    "LOW",
			Description: "Base64 encoded secret pattern",
			Confidence:  50,
			Enabled:     true,
		},
	}

	// Add custom patterns from configuration
	for _, customPattern := range ss.config.Patterns {
		pattern := SecretPattern{
			Name:        "Custom Pattern",
			Pattern:     customPattern,
			Type:        "custom",
			Severity:    "MEDIUM",
			Description: "Custom secret pattern",
			Confidence:  70,
			Enabled:     true,
		}
		ss.patterns = append(ss.patterns, pattern)
	}
}

// compilePatterns compiles all patterns into regular expressions
func (ss *SecretScanner) compilePatterns() {
	ss.compiled = make([]*regexp.Regexp, len(ss.patterns))
	
	for i, pattern := range ss.patterns {
		if pattern.Enabled {
			compiled, err := regexp.Compile(pattern.Pattern)
			if err != nil {
				ss.logger.Error("failed to compile pattern", map[string]interface{}{
					"pattern": pattern.Name,
					"error":   err.Error(),
				})
				continue
			}
			ss.compiled[i] = compiled
		}
	}
}

// isExcluded checks if a line should be excluded from scanning
func (ss *SecretScanner) isExcluded(line string) bool {
	// Check if line contains exclusion patterns
	for _, exclusion := range ss.config.Exclusions {
		if strings.Contains(strings.ToLower(line), strings.ToLower(exclusion)) {
			return true
		}
	}

	// Skip comments and documentation
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") {
		return true
	}

	// Skip example/test patterns
	if strings.Contains(strings.ToLower(line), "example") ||
		strings.Contains(strings.ToLower(line), "test") ||
		strings.Contains(strings.ToLower(line), "demo") ||
		strings.Contains(strings.ToLower(line), "placeholder") {
		return true
	}

	return false
}

// getContext extracts context lines around a match
func (ss *SecretScanner) getContext(lines []string, lineNum, contextSize int) string {
	start := lineNum - contextSize
	if start < 0 {
		start = 0
	}
	
	end := lineNum + contextSize + 1
	if end > len(lines) {
		end = len(lines)
	}

	contextLines := lines[start:end]
	return strings.Join(contextLines, "\n")
}

// redactSecret redacts a secret value for safe logging
func (ss *SecretScanner) redactSecret(secret string) string {
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	
	// Show first 4 and last 4 characters
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}

// calculateSummary calculates summary statistics for scan results
func (ss *SecretScanner) calculateSummary(matches []SecretMatch) ScanSummary {
	summary := ScanSummary{}
	typeMap := make(map[string]bool)

	for _, match := range matches {
		summary.TotalMatches++
		
		switch strings.ToUpper(match.Severity) {
		case "HIGH":
			summary.HighSeverity++
		case "MEDIUM":
			summary.MediumSeverity++
		case "LOW":
			summary.LowSeverity++
		}

		typeMap[match.Type] = true
	}

	summary.UniqueTypes = len(typeMap)
	return summary
}

// GetPatterns returns all loaded patterns
func (ss *SecretScanner) GetPatterns() []SecretPattern {
	return ss.patterns
}

// EnablePattern enables a specific pattern
func (ss *SecretScanner) EnablePattern(name string) error {
	for i, pattern := range ss.patterns {
		if pattern.Name == name {
			ss.patterns[i].Enabled = true
			ss.compilePatterns() // Recompile
			return nil
		}
	}
	return fmt.Errorf("pattern not found: %s", name)
}

// DisablePattern disables a specific pattern
func (ss *SecretScanner) DisablePattern(name string) error {
	for i, pattern := range ss.patterns {
		if pattern.Name == name {
			ss.patterns[i].Enabled = false
			ss.compilePatterns() // Recompile
			return nil
		}
	}
	return fmt.Errorf("pattern not found: %s", name)
}

// AddCustomPattern adds a custom pattern
func (ss *SecretScanner) AddCustomPattern(pattern SecretPattern) error {
	// Validate pattern
	if _, err := regexp.Compile(pattern.Pattern); err != nil {
		return fmt.Errorf("invalid pattern: %v", err)
	}

	ss.patterns = append(ss.patterns, pattern)
	ss.compilePatterns()
	
	ss.logger.Info("custom pattern added", map[string]interface{}{
		"name": pattern.Name,
		"type": pattern.Type,
	})

	return nil
}