package sharedconfig

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
	Level   string // "error", "warning"
}

// ValidationResult holds all validation errors and warnings
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
	Valid    bool
}

// ConfigValidator handles configuration validation
type ConfigValidator struct {
	config *SharedConfig
	result *ValidationResult
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator(config *SharedConfig) *ConfigValidator {
	return &ConfigValidator{
		config: config,
		result: &ValidationResult{
			Errors:   []ValidationError{},
			Warnings: []ValidationError{},
			Valid:    true,
		},
	}
}

// Validate performs comprehensive configuration validation
func (cv *ConfigValidator) Validate() *ValidationResult {
	// Schema version validation
	cv.validateSchemaVersion()
	
	// Storage validation
	cv.validateStorage()
	
	// Cluster validation
	cv.validateCluster()
	
	// Backup validation
	cv.validateBackup()
	
	// GitOps validation
	cv.validateGitOps()
	
	// Pipeline validation
	cv.validatePipeline()
	
	// Security validation
	cv.validateSecurity()
	
	// Performance validation
	cv.validatePerformance()
	
	// Cross-field validation
	cv.validateCrossFieldRules()
	
	// Set overall validity
	cv.result.Valid = len(cv.result.Errors) == 0
	
	return cv.result
}

// validateSchemaVersion validates the schema version
func (cv *ConfigValidator) validateSchemaVersion() {
	if cv.config.SchemaVersion == "" {
		cv.addError("schema_version", "", "Schema version is required")
	} else if !isValidSemver(cv.config.SchemaVersion) {
		cv.addError("schema_version", cv.config.SchemaVersion, "Invalid semantic version format")
	}
}

// validateStorage validates storage configuration
func (cv *ConfigValidator) validateStorage() {
	s := &cv.config.Storage
	
	// Validate storage type
	if s.Type != "minio" && s.Type != "s3" {
		cv.addError("storage.type", s.Type, "Storage type must be 'minio' or 's3'")
	}
	
	// Validate endpoint
	if s.Endpoint == "" {
		cv.addError("storage.endpoint", "", "Storage endpoint is required")
	} else if !isValidEndpoint(s.Endpoint) {
		cv.addError("storage.endpoint", s.Endpoint, "Invalid endpoint format (expected host:port or URL)")
	}
	
	// Validate credentials
	if s.AccessKey == "" {
		cv.addError("storage.access_key", "", "Access key is required")
	}
	if s.SecretKey == "" {
		cv.addError("storage.secret_key", "", "Secret key is required")
	}
	
	// Validate bucket name
	if s.Bucket == "" {
		cv.addError("storage.bucket", "", "Bucket name is required")
	} else if !isValidBucketName(s.Bucket) {
		cv.addError("storage.bucket", s.Bucket, "Invalid bucket name (must be 3-63 characters, lowercase letters, numbers, and hyphens)")
	}
	
	// Validate region
	if s.Type == "s3" && s.Region == "" {
		cv.addError("storage.region", "", "Region is required for S3 storage")
	}
	
	// Validate connection settings
	if s.Connection.Timeout <= 0 {
		cv.addError("storage.connection.timeout", s.Connection.Timeout, "Timeout must be positive")
	}
	if s.Connection.MaxRetries < 0 {
		cv.addError("storage.connection.max_retries", s.Connection.MaxRetries, "Max retries cannot be negative")
	}
	if s.Connection.RetryDelay < 0 {
		cv.addError("storage.connection.retry_delay", s.Connection.RetryDelay, "Retry delay cannot be negative")
	}
	
	// Validate fallback buckets
	for i, bucket := range s.FallbackBuckets {
		if !isValidBucketName(bucket) {
			cv.addError(fmt.Sprintf("storage.fallback_buckets[%d]", i), bucket, "Invalid fallback bucket name")
		}
	}
}

// validateCluster validates cluster configuration
func (cv *ConfigValidator) validateCluster() {
	c := &cv.config.Cluster
	
	// Validate cluster name
	if c.Name == "" {
		cv.addError("cluster.name", "", "Cluster name is required")
	} else if !isValidDNSName(c.Name) {
		cv.addError("cluster.name", c.Name, "Invalid cluster name (must be valid DNS label)")
	}
	
	// Validate domain
	if c.Domain != "" && !isValidDomain(c.Domain) {
		cv.addError("cluster.domain", c.Domain, "Invalid domain format")
	}
	
	// Validate cluster type
	validTypes := []string{"kubernetes", "openshift"}
	if !contains(validTypes, c.Type) {
		cv.addError("cluster.type", c.Type, "Cluster type must be 'kubernetes' or 'openshift'")
	}
	
	// Validate OpenShift settings if applicable
	if c.Type == "openshift" {
		validModes := []string{"auto-detect", "enabled", "disabled"}
		if !contains(validModes, c.OpenShift.Mode) {
			cv.addError("cluster.openshift.mode", c.OpenShift.Mode, "OpenShift mode must be 'auto-detect', 'enabled', or 'disabled'")
		}
	}
}

// validateBackup validates backup configuration
func (cv *ConfigValidator) validateBackup() {
	b := &cv.config.Backup
	
	// Validate filtering mode
	validModes := []string{"whitelist", "blacklist", "hybrid"}
	if !contains(validModes, b.Filtering.Mode) {
		cv.addError("backup.filtering.mode", b.Filtering.Mode, "Filtering mode must be 'whitelist', 'blacklist', or 'hybrid'")
	}
	
	// Validate resource types
	for i, resource := range b.Filtering.Resources.Include {
		if !isValidResourceType(resource) {
			cv.addWarning(fmt.Sprintf("backup.filtering.resources.include[%d]", i), resource, "May not be a valid Kubernetes resource type")
		}
	}
	for i, resource := range b.Filtering.Resources.Exclude {
		if !isValidResourceType(resource) {
			cv.addWarning(fmt.Sprintf("backup.filtering.resources.exclude[%d]", i), resource, "May not be a valid Kubernetes resource type")
		}
	}
	
	// Validate batch size
	if b.Behavior.BatchSize <= 0 {
		cv.addError("backup.behavior.batch_size", b.Behavior.BatchSize, "Batch size must be positive")
	} else if b.Behavior.BatchSize > 1000 {
		cv.addWarning("backup.behavior.batch_size", b.Behavior.BatchSize, "Large batch size may cause performance issues")
	}
	
	// Validate max resource size
	if b.Behavior.MaxResourceSize != "" && !isValidSize(b.Behavior.MaxResourceSize) {
		cv.addError("backup.behavior.max_resource_size", b.Behavior.MaxResourceSize, "Invalid size format (expected format like '10Mi', '1Gi')")
	}
	
	// Validate cleanup settings
	if b.Cleanup.RetentionDays < 0 {
		cv.addError("backup.cleanup.retention_days", b.Cleanup.RetentionDays, "Retention days cannot be negative")
	} else if b.Cleanup.RetentionDays == 0 {
		cv.addWarning("backup.cleanup.retention_days", b.Cleanup.RetentionDays, "Zero retention days means immediate deletion")
	}
}

// validateGitOps validates GitOps configuration
func (cv *ConfigValidator) validateGitOps() {
	g := &cv.config.GitOps
	
	// Validate repository URL
	if g.Repository.URL == "" {
		cv.addError("gitops.repository.url", "", "Git repository URL is required")
	} else if !isValidGitURL(g.Repository.URL) {
		cv.addError("gitops.repository.url", g.Repository.URL, "Invalid Git repository URL")
	}
	
	// Validate branch
	if g.Repository.Branch == "" {
		cv.addError("gitops.repository.branch", "", "Git branch is required")
	} else if !isValidBranchName(g.Repository.Branch) {
		cv.addError("gitops.repository.branch", g.Repository.Branch, "Invalid Git branch name")
	}
	
	// Validate authentication
	validAuthMethods := []string{"ssh", "pat", "basic", "none"}
	if !contains(validAuthMethods, g.Repository.Auth.Method) {
		cv.addError("gitops.repository.auth.method", g.Repository.Auth.Method, "Auth method must be 'ssh', 'pat', 'basic', or 'none'")
	}
	
	// Validate auth-specific settings
	switch g.Repository.Auth.Method {
	case "ssh":
		if g.Repository.Auth.SSH.PrivateKeyPath == "" {
			cv.addError("gitops.repository.auth.ssh.private_key_path", "", "SSH private key path is required for SSH authentication")
		}
	case "pat":
		if g.Repository.Auth.PAT.Token == "" {
			cv.addError("gitops.repository.auth.pat.token", "", "PAT token is required for PAT authentication")
		}
	case "basic":
		if g.Repository.Auth.Basic.Username == "" {
			cv.addError("gitops.repository.auth.basic.username", "", "Username is required for basic authentication")
		}
		if g.Repository.Auth.Basic.Password == "" {
			cv.addError("gitops.repository.auth.basic.password", "", "Password is required for basic authentication")
		}
	}
	
	// Validate environments
	for i, env := range g.Structure.Environments {
		if env.Name == "" {
			cv.addError(fmt.Sprintf("gitops.structure.environments[%d].name", i), "", "Environment name is required")
		}
		if env.ClusterURL != "" && !isValidURL(env.ClusterURL) {
			cv.addError(fmt.Sprintf("gitops.structure.environments[%d].cluster_url", i), env.ClusterURL, "Invalid cluster URL")
		}
		if env.Replicas < 0 {
			cv.addError(fmt.Sprintf("gitops.structure.environments[%d].replicas", i), env.Replicas, "Replicas cannot be negative")
		}
	}
	
	// Validate ArgoCD settings
	if g.Structure.ArgoCD.Enabled {
		if g.Structure.ArgoCD.Namespace == "" {
			cv.addError("gitops.structure.argocd.namespace", "", "ArgoCD namespace is required when ArgoCD is enabled")
		}
		if g.Structure.ArgoCD.Project == "" {
			cv.addError("gitops.structure.argocd.project", "", "ArgoCD project is required when ArgoCD is enabled")
		}
	}
}

// validatePipeline validates pipeline configuration
func (cv *ConfigValidator) validatePipeline() {
	p := &cv.config.Pipeline
	
	// Validate mode
	validModes := []string{"sequential", "parallel", "manual"}
	if !contains(validModes, p.Mode) {
		cv.addError("pipeline.mode", p.Mode, "Pipeline mode must be 'sequential', 'parallel', or 'manual'")
	}
	
	// Validate automation settings
	if p.Automation.Enabled {
		if p.Automation.MaxWaitTime <= 0 {
			cv.addError("pipeline.automation.max_wait_time", p.Automation.MaxWaitTime, "Max wait time must be positive when automation is enabled")
		}
		
		// Validate trigger methods
		validTriggerMethods := []string{"file", "process", "webhook", "script"}
		for i, method := range p.Automation.TriggerMethods {
			if !contains(validTriggerMethods, method) {
				cv.addError(fmt.Sprintf("pipeline.automation.trigger_methods[%d]", i), method, "Invalid trigger method")
			}
		}
		
		// Validate webhook settings if webhook trigger is enabled
		if p.Automation.WebhookTrigger.Enabled {
			if p.Automation.WebhookTrigger.ServerPort <= 0 || p.Automation.WebhookTrigger.ServerPort > 65535 {
				cv.addError("pipeline.automation.webhook_trigger.server_port", p.Automation.WebhookTrigger.ServerPort, "Invalid webhook server port")
			}
		}
	}
	
	// Validate error handling
	if p.ErrorHandling.MaxRetries < 0 {
		cv.addError("pipeline.error_handling.max_retries", p.ErrorHandling.MaxRetries, "Max retries cannot be negative")
	}
	if p.ErrorHandling.RetryDelay < 0 {
		cv.addError("pipeline.error_handling.retry_delay", p.ErrorHandling.RetryDelay, "Retry delay cannot be negative")
	}
}

// validateSecurity validates security configuration
func (cv *ConfigValidator) validateSecurity() {
	s := &cv.config.Security
	
	// Validate secret provider
	validProviders := []string{"env", "vault", "aws-secrets", "azure-keyvault"}
	if !contains(validProviders, s.Secrets.Provider) {
		cv.addError("security.secrets.provider", s.Secrets.Provider, "Invalid secret provider")
	}
	
	// Validate provider-specific settings
	switch s.Secrets.Provider {
	case "vault":
		if s.Secrets.Vault.Address == "" {
			cv.addError("security.secrets.vault.address", "", "Vault address is required for Vault provider")
		}
		if s.Secrets.Vault.Token == "" {
			cv.addError("security.secrets.vault.token", "", "Vault token is required for Vault provider")
		}
	case "aws-secrets":
		if s.Secrets.AWSSecrets.Region == "" {
			cv.addError("security.secrets.aws_secrets.region", "", "AWS region is required for AWS Secrets provider")
		}
		if s.Secrets.AWSSecrets.SecretName == "" {
			cv.addError("security.secrets.aws_secrets.secret_name", "", "Secret name is required for AWS Secrets provider")
		}
	case "azure-keyvault":
		if s.Secrets.AzureKeyVault.VaultName == "" {
			cv.addError("security.secrets.azure_keyvault.vault_name", "", "Vault name is required for Azure KeyVault provider")
		}
		if s.Secrets.AzureKeyVault.TenantID == "" {
			cv.addError("security.secrets.azure_keyvault.tenant_id", "", "Tenant ID is required for Azure KeyVault provider")
		}
	}
	
	// Validate max file size
	if s.Validation.MaxFileSize != "" && !isValidSize(s.Validation.MaxFileSize) {
		cv.addError("security.validation.max_file_size", s.Validation.MaxFileSize, "Invalid size format")
	}
}

// validatePerformance validates performance configuration
func (cv *ConfigValidator) validatePerformance() {
	p := &cv.config.Performance
	
	// Validate resource limits
	if p.Limits.MaxConcurrentOperations <= 0 {
		cv.addError("performance.limits.max_concurrent_operations", p.Limits.MaxConcurrentOperations, "Max concurrent operations must be positive")
	} else if p.Limits.MaxConcurrentOperations > 100 {
		cv.addWarning("performance.limits.max_concurrent_operations", p.Limits.MaxConcurrentOperations, "Very high concurrency may cause resource exhaustion")
	}
	
	if p.Limits.MemoryLimit != "" && !isValidSize(p.Limits.MemoryLimit) {
		cv.addError("performance.limits.memory_limit", p.Limits.MemoryLimit, "Invalid memory limit format")
	}
	
	if p.Limits.CPULimit != "" && !isValidCPULimit(p.Limits.CPULimit) {
		cv.addError("performance.limits.cpu_limit", p.Limits.CPULimit, "Invalid CPU limit format")
	}
	
	// Validate cache TTL
	if p.Optimization.Caching && p.Optimization.CacheTTL <= 0 {
		cv.addError("performance.optimization.cache_ttl", p.Optimization.CacheTTL, "Cache TTL must be positive when caching is enabled")
	}
}

// validateCrossFieldRules validates cross-field dependencies and rules
func (cv *ConfigValidator) validateCrossFieldRules() {
	// Rule: If OpenShift cluster type, OpenShift settings should be configured
	if cv.config.Cluster.Type == "openshift" && cv.config.Cluster.OpenShift.Mode == "disabled" {
		cv.addWarning("cluster.openshift.mode", cv.config.Cluster.OpenShift.Mode, "OpenShift mode is disabled for OpenShift cluster type")
	}
	
	// Rule: If auto-create bucket is enabled, verify credentials have sufficient permissions
	if cv.config.Storage.AutoCreateBucket && cv.config.Storage.Type == "s3" {
		cv.addWarning("storage.auto_create_bucket", true, "Ensure AWS credentials have bucket creation permissions")
	}
	
	// Rule: If ArgoCD is enabled, GitOps repository must be configured
	if cv.config.GitOps.Structure.ArgoCD.Enabled && cv.config.GitOps.Repository.URL == "" {
		cv.addError("gitops.repository.url", "", "Git repository URL is required when ArgoCD is enabled")
	}
	
	// Rule: If webhook notifications are enabled, URL must be provided
	if cv.config.Pipeline.Notifications.Enabled && cv.config.Pipeline.Notifications.Webhook.URL == "" {
		cv.addError("pipeline.notifications.webhook.url", "", "Webhook URL is required when notifications are enabled")
	}
	
	// Rule: If pipeline automation is enabled, at least one trigger method should be configured
	if cv.config.Pipeline.Automation.Enabled && len(cv.config.Pipeline.Automation.TriggerMethods) == 0 {
		cv.addError("pipeline.automation.trigger_methods", nil, "At least one trigger method is required when automation is enabled")
	}
	
	// Rule: If strict validation is enabled, warn about performance impact
	if cv.config.Security.Validation.StrictMode && cv.config.Performance.Limits.MaxConcurrentOperations > 50 {
		cv.addWarning("security.validation.strict_mode", true, "Strict validation with high concurrency may impact performance")
	}
	
	// Rule: If retention is enabled but days is 0, warn about immediate deletion
	if cv.config.Backup.Cleanup.Enabled && cv.config.Backup.Cleanup.RetentionDays == 0 {
		cv.addWarning("backup.cleanup.retention_days", 0, "Backups will be deleted immediately with 0 retention days")
	}
}

// Helper methods

func (cv *ConfigValidator) addError(field string, value interface{}, message string) {
	cv.result.Errors = append(cv.result.Errors, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
		Level:   "error",
	})
}

func (cv *ConfigValidator) addWarning(field string, value interface{}, message string) {
	cv.result.Warnings = append(cv.result.Warnings, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
		Level:   "warning",
	})
}

// Validation helper functions

func isValidSemver(version string) bool {
	// Simplified semantic version check
	pattern := `^v?\d+\.\d+\.\d+(-[a-zA-Z0-9\-\.]+)?(\+[a-zA-Z0-9\-\.]+)?$`
	match, _ := regexp.MatchString(pattern, version)
	return match
}

func isValidEndpoint(endpoint string) bool {
	// Check if it's a valid host:port or URL
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		_, err := url.Parse(endpoint)
		return err == nil
	}
	// Check host:port format
	pattern := `^[a-zA-Z0-9\-\.]+:\d+$`
	match, _ := regexp.MatchString(pattern, endpoint)
	return match
}

func isValidBucketName(bucket string) bool {
	// S3/MinIO bucket naming rules
	if len(bucket) < 3 || len(bucket) > 63 {
		return false
	}
	pattern := `^[a-z0-9][a-z0-9\-]*[a-z0-9]$`
	match, _ := regexp.MatchString(pattern, bucket)
	return match
}

func isValidDNSName(name string) bool {
	// DNS label validation
	pattern := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	match, _ := regexp.MatchString(pattern, name)
	return match && len(name) <= 63
}

func isValidDomain(domain string) bool {
	// Basic domain validation
	pattern := `^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`
	match, _ := regexp.MatchString(pattern, domain)
	return match
}

func isValidResourceType(resource string) bool {
	// Common Kubernetes resource types (not exhaustive)
	validTypes := []string{
		"pods", "services", "deployments", "statefulsets", "daemonsets",
		"configmaps", "secrets", "persistentvolumeclaims", "persistentvolumes",
		"ingresses", "networkpolicies", "serviceaccounts", "roles", "rolebindings",
		"clusterroles", "clusterrolebindings", "namespaces", "nodes",
		"customresourcedefinitions", "horizontalpodautoscalers", "verticalpodautoscalers",
		"poddisruptionbudgets", "priorityclasses", "storageclasses",
	}
	return contains(validTypes, strings.ToLower(resource))
}

func isValidSize(size string) bool {
	// Kubernetes size format validation
	pattern := `^\d+(\.\d+)?([KMGT]i?)?$`
	match, _ := regexp.MatchString(pattern, size)
	return match
}

func isValidGitURL(url string) bool {
	// Git URL validation (SSH or HTTPS)
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://") {
		return true
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return strings.HasSuffix(url, ".git") || strings.Contains(url, "github.com") || strings.Contains(url, "gitlab.com") || strings.Contains(url, "bitbucket.org")
	}
	return false
}

func isValidBranchName(branch string) bool {
	// Git branch name validation
	pattern := `^[a-zA-Z0-9/_\-\.]+$`
	match, _ := regexp.MatchString(pattern, branch)
	return match && !strings.HasPrefix(branch, "/") && !strings.HasSuffix(branch, "/")
}

func isValidURL(urlStr string) bool {
	_, err := url.Parse(urlStr)
	return err == nil && (strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://"))
}

func isValidCPULimit(cpu string) bool {
	// CPU limit format validation (e.g., "2", "500m", "0.5")
	pattern := `^\d+(\.\d+)?m?$`
	match, _ := regexp.MatchString(pattern, cpu)
	return match
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ValidateConfig is the main validation entry point
func ValidateConfig(config *SharedConfig) (*ValidationResult, error) {
	validator := NewConfigValidator(config)
	result := validator.Validate()
	
	if !result.Valid {
		return result, fmt.Errorf("configuration validation failed with %d errors", len(result.Errors))
	}
	
	return result, nil
}

// FormatValidationResult formats the validation result for display
func FormatValidationResult(result *ValidationResult) string {
	var output strings.Builder
	
	if result.Valid {
		output.WriteString("✅ Configuration is valid\n")
		if len(result.Warnings) > 0 {
			output.WriteString(fmt.Sprintf("\n⚠️  %d warning(s):\n", len(result.Warnings)))
			for _, warning := range result.Warnings {
				output.WriteString(fmt.Sprintf("  - %s: %s\n", warning.Field, warning.Message))
			}
		}
	} else {
		output.WriteString(fmt.Sprintf("❌ Configuration validation failed with %d error(s):\n\n", len(result.Errors)))
		for _, err := range result.Errors {
			output.WriteString(fmt.Sprintf("  ❌ %s: %s\n", err.Field, err.Message))
			if err.Value != nil && err.Value != "" {
				output.WriteString(fmt.Sprintf("     Current value: %v\n", err.Value))
			}
		}
		
		if len(result.Warnings) > 0 {
			output.WriteString(fmt.Sprintf("\n⚠️  %d warning(s):\n", len(result.Warnings)))
			for _, warning := range result.Warnings {
				output.WriteString(fmt.Sprintf("  - %s: %s\n", warning.Field, warning.Message))
			}
		}
	}
	
	return output.String()
}