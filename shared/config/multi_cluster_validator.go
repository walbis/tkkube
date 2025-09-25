package sharedconfig

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MultiClusterValidator provides validation for multi-cluster configurations
type MultiClusterValidator struct{}

// NewMultiClusterValidator creates a new multi-cluster validator
func NewMultiClusterValidator() *MultiClusterValidator {
	return &MultiClusterValidator{}
}

// MultiClusterValidationResult represents the result of multi-cluster configuration validation
type MultiClusterValidationResult struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ValidateMultiClusterConfig validates the complete multi-cluster configuration
func (v *MultiClusterValidator) ValidateMultiClusterConfig(config *MultiClusterConfig) *MultiClusterValidationResult {
	result := &MultiClusterValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Skip validation if multi-cluster is not enabled
	if !config.Enabled {
		result.Warnings = append(result.Warnings, "Multi-cluster support is disabled")
		return result
	}

	// Validate basic configuration
	v.validateBasicConfig(config, result)
	
	// Validate clusters
	v.validateClusters(config, result)
	
	// Validate coordination settings
	v.validateCoordination(config, result)
	
	// Validate scheduling settings
	v.validateScheduling(config, result)

	// Set overall validation result
	result.Valid = len(result.Errors) == 0

	return result
}

// validateBasicConfig validates basic multi-cluster settings
func (v *MultiClusterValidator) validateBasicConfig(config *MultiClusterConfig, result *MultiClusterValidationResult) {
	// Validate execution mode
	validModes := []string{"sequential", "parallel"}
	if !v.isValidChoice(config.Mode, validModes) {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("invalid execution mode '%s', must be one of: %v", config.Mode, validModes))
	}

	// Validate default cluster
	if config.DefaultCluster == "" {
		result.Errors = append(result.Errors, "default_cluster is required when multi-cluster is enabled")
	} else {
		// Check if default cluster exists in clusters list
		found := false
		for _, cluster := range config.Clusters {
			if cluster.Name == config.DefaultCluster {
				found = true
				break
			}
		}
		if !found {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("default_cluster '%s' not found in clusters list", config.DefaultCluster))
		}
	}

	// Validate cluster count
	if len(config.Clusters) == 0 {
		result.Errors = append(result.Errors, "at least one cluster must be configured when multi-cluster is enabled")
	} else if len(config.Clusters) == 1 {
		result.Warnings = append(result.Warnings, "only one cluster configured, consider using single-cluster mode")
	}
}

// validateClusters validates individual cluster configurations
func (v *MultiClusterValidator) validateClusters(config *MultiClusterConfig, result *MultiClusterValidationResult) {
	clusterNames := make(map[string]bool)
	endpoints := make(map[string]string)

	for i, cluster := range config.Clusters {
		clusterPrefix := fmt.Sprintf("clusters[%d]", i)

		// Validate cluster name
		if cluster.Name == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: cluster name is required", clusterPrefix))
			continue
		}

		// Check for duplicate cluster names
		if clusterNames[cluster.Name] {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("%s: duplicate cluster name '%s'", clusterPrefix, cluster.Name))
		}
		clusterNames[cluster.Name] = true

		// Validate cluster name format
		if !v.isValidClusterName(cluster.Name) {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("%s: invalid cluster name '%s', must contain only alphanumeric characters and hyphens", 
					clusterPrefix, cluster.Name))
		}

		// Validate endpoint
		if cluster.Endpoint == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: endpoint is required", clusterPrefix))
		} else {
			// Check for duplicate endpoints
			if existingCluster, exists := endpoints[cluster.Endpoint]; exists {
				result.Warnings = append(result.Warnings, 
					fmt.Sprintf("%s: endpoint '%s' is already used by cluster '%s'", 
						clusterPrefix, cluster.Endpoint, existingCluster))
			} else {
				endpoints[cluster.Endpoint] = cluster.Name
			}

			// Validate endpoint format
			if !v.isValidEndpoint(cluster.Endpoint) {
				result.Errors = append(result.Errors, 
					fmt.Sprintf("%s: invalid endpoint format '%s'", clusterPrefix, cluster.Endpoint))
			}
		}

		// Validate token
		if cluster.Token == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: token is required", clusterPrefix))
		} else {
			// Basic token format validation
			if len(cluster.Token) < 10 {
				result.Warnings = append(result.Warnings, 
					fmt.Sprintf("%s: token seems too short, ensure it's a valid Kubernetes token", clusterPrefix))
			}
		}

		// Validate authentication configuration
		v.validateAuthConfig(&cluster.Auth, &cluster.TLS, fmt.Sprintf("%s.auth", clusterPrefix), result)

		// Validate storage configuration
		v.validateStorageConfig(&cluster.Storage, fmt.Sprintf("%s.storage", clusterPrefix), result)
	}
}

// validateCoordination validates coordination settings
func (v *MultiClusterValidator) validateCoordination(config *MultiClusterConfig, result *MultiClusterValidationResult) {
	coord := config.Coordination

	// Validate timeout
	if coord.Timeout <= 0 {
		result.Errors = append(result.Errors, "coordination.timeout must be greater than 0")
	} else if coord.Timeout < 30 {
		result.Warnings = append(result.Warnings, "coordination.timeout is very low, consider increasing for stability")
	} else if coord.Timeout > 3600 {
		result.Warnings = append(result.Warnings, "coordination.timeout is very high, consider reducing for responsiveness")
	}

	// Validate retry attempts
	if coord.RetryAttempts < 0 {
		result.Errors = append(result.Errors, "coordination.retry_attempts must be non-negative")
	} else if coord.RetryAttempts > 10 {
		result.Warnings = append(result.Warnings, "coordination.retry_attempts is very high, may cause excessive delays")
	}

	// Validate failure threshold
	if coord.FailureThreshold <= 0 {
		result.Errors = append(result.Errors, "coordination.failure_threshold must be greater than 0")
	} else if coord.FailureThreshold > len(config.Clusters) {
		result.Warnings = append(result.Warnings, "coordination.failure_threshold is higher than cluster count")
	}

	// Validate health check interval
	if coord.HealthCheckInterval == "" {
		result.Errors = append(result.Errors, "coordination.health_check_interval is required")
	} else {
		if _, err := time.ParseDuration(coord.HealthCheckInterval); err != nil {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("coordination.health_check_interval has invalid duration format: %v", err))
		}
	}
}

// validateScheduling validates scheduling settings
func (v *MultiClusterValidator) validateScheduling(config *MultiClusterConfig, result *MultiClusterValidationResult) {
	sched := config.Scheduling

	// Validate strategy
	validStrategies := []string{"round_robin", "least_loaded", "priority"}
	if !v.isValidChoice(sched.Strategy, validStrategies) {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("scheduling.strategy '%s' is invalid, must be one of: %v", sched.Strategy, validStrategies))
	}

	// Validate max concurrent clusters
	if sched.MaxConcurrentClusters <= 0 {
		result.Errors = append(result.Errors, "scheduling.max_concurrent_clusters must be greater than 0")
	} else if sched.MaxConcurrentClusters > len(config.Clusters) {
		result.Warnings = append(result.Warnings, 
			"scheduling.max_concurrent_clusters is higher than total cluster count")
	}

	// Validate cluster priorities if strategy is priority-based
	if sched.Strategy == "priority" {
		v.validateClusterPriorities(config, result)
	}
}

// validateClusterPriorities validates cluster priority settings
func (v *MultiClusterValidator) validateClusterPriorities(config *MultiClusterConfig, result *MultiClusterValidationResult) {
	sched := config.Scheduling
	
	if len(sched.ClusterPriorities) == 0 && sched.Strategy == "priority" {
		result.Warnings = append(result.Warnings, 
			"no cluster priorities defined for priority-based scheduling")
		return
	}

	prioritizedClusters := make(map[string]bool)
	priorities := make(map[int][]string)

	for i, priority := range sched.ClusterPriorities {
		prefix := fmt.Sprintf("scheduling.cluster_priorities[%d]", i)

		// Validate cluster name
		if priority.Cluster == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: cluster name is required", prefix))
			continue
		}

		// Check if cluster exists
		clusterExists := false
		for _, cluster := range config.Clusters {
			if cluster.Name == priority.Cluster {
				clusterExists = true
				break
			}
		}
		if !clusterExists {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("%s: cluster '%s' not found in clusters list", prefix, priority.Cluster))
		}

		// Check for duplicate cluster priorities
		if prioritizedClusters[priority.Cluster] {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("%s: duplicate priority definition for cluster '%s'", prefix, priority.Cluster))
		}
		prioritizedClusters[priority.Cluster] = true

		// Validate priority value
		if priority.Priority <= 0 {
			result.Errors = append(result.Errors, 
				fmt.Sprintf("%s: priority must be greater than 0", prefix))
		}

		// Track priorities for duplicate checking
		priorities[priority.Priority] = append(priorities[priority.Priority], priority.Cluster)
	}

	// Check for clusters without priorities
	for _, cluster := range config.Clusters {
		if !prioritizedClusters[cluster.Name] {
			result.Warnings = append(result.Warnings, 
				fmt.Sprintf("cluster '%s' has no priority defined, will use default", cluster.Name))
		}
	}

	// Warn about duplicate priorities
	for priority, clusters := range priorities {
		if len(clusters) > 1 {
			result.Warnings = append(result.Warnings, 
				fmt.Sprintf("multiple clusters have priority %d: %v", priority, clusters))
		}
	}
}

// validateStorageConfig validates storage configuration for a cluster
func (v *MultiClusterValidator) validateStorageConfig(storage *StorageConfig, prefix string, result *MultiClusterValidationResult) {
	// Validate storage type
	validTypes := []string{"minio", "s3"}
	if !v.isValidChoice(storage.Type, validTypes) {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s.type '%s' is invalid, must be one of: %v", prefix, storage.Type, validTypes))
	}

	// Validate endpoint
	if storage.Endpoint == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.endpoint is required", prefix))
	}

	// Validate credentials
	if storage.AccessKey == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.access_key is required", prefix))
	}
	if storage.SecretKey == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.secret_key is required", prefix))
	}

	// Validate bucket name
	if storage.Bucket == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.bucket is required", prefix))
	} else if !v.isValidBucketName(storage.Bucket) {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s.bucket '%s' has invalid format", prefix, storage.Bucket))
	}

	// Validate region
	if storage.Region == "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s.region is not specified, using default", prefix))
	}
}

// Helper validation functions

// isValidChoice checks if value is in the list of valid choices
func (v *MultiClusterValidator) isValidChoice(value string, validChoices []string) bool {
	for _, choice := range validChoices {
		if value == choice {
			return true
		}
	}
	return false
}

// isValidClusterName validates cluster name format
func (v *MultiClusterValidator) isValidClusterName(name string) bool {
	// Allow alphanumeric characters and hyphens, must start and end with alphanumeric
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?$`, name)
	return matched && len(name) <= 63 // Kubernetes name length limit
}

// isValidEndpoint validates endpoint URL format
func (v *MultiClusterValidator) isValidEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}

	// Must be HTTPS for security
	if u.Scheme != "https" {
		return false
	}

	// Must have host
	if u.Host == "" {
		return false
	}

	return true
}

// isValidBucketName validates S3/MinIO bucket name format
func (v *MultiClusterValidator) isValidBucketName(name string) bool {
	// Basic S3 bucket name validation
	if len(name) < 3 || len(name) > 63 {
		return false
	}

	// Must start and end with lowercase letter or number
	matched, _ := regexp.MatchString(`^[a-z0-9].*[a-z0-9]$`, name)
	if !matched {
		return false
	}

	// Cannot contain uppercase letters, underscores, or consecutive periods
	if strings.ContainsAny(name, "A-Z_") || strings.Contains(name, "..") {
		return false
	}

	return true
}

// ValidateForProduction performs additional validation for production environments
func (v *MultiClusterValidator) ValidateForProduction(config *MultiClusterConfig) *ValidationResult {
	multiResult := v.ValidateMultiClusterConfig(config)

	if !config.Enabled {
		return v.convertToValidationResult(multiResult)
	}

	// Additional production-specific validations
	
	// Ensure HTTPS endpoints
	for i, cluster := range config.Clusters {
		if !strings.HasPrefix(cluster.Endpoint, "https://") {
			multiResult.Errors = append(multiResult.Errors, 
				fmt.Sprintf("clusters[%d]: production deployments must use HTTPS endpoints", i))
		}

		// Check for secure storage
		if cluster.Storage.Type == "minio" && !cluster.Storage.UseSSL {
			multiResult.Warnings = append(multiResult.Warnings, 
				fmt.Sprintf("clusters[%d]: consider enabling SSL for MinIO in production", i))
		}
	}

	// Ensure reasonable coordination settings for production
	if config.Coordination.Timeout < 60 {
		multiResult.Warnings = append(multiResult.Warnings, 
			"coordination.timeout is low for production, consider increasing for stability")
	}

	if config.Coordination.RetryAttempts < 2 {
		multiResult.Warnings = append(multiResult.Warnings, 
			"coordination.retry_attempts is low for production, consider increasing for resilience")
	}

	// Check for production-appropriate cluster count
	if len(config.Clusters) < 2 {
		multiResult.Warnings = append(multiResult.Warnings, 
			"production deployments typically require multiple clusters for high availability")
	}

	return v.convertToValidationResult(multiResult)
}

// validateAuthConfig validates authentication and TLS configuration
func (v *MultiClusterValidator) validateAuthConfig(auth *ClusterAuthConfig, tls *ClusterTLSConfig, prefix string, result *MultiClusterValidationResult) {
	// Create authentication manager for validation
	authManager := NewClusterAuthManager()
	
	// Validate authentication method
	validMethods := []string{"token", "service_account", "oidc", "exec"}
	if auth.Method != "" && !v.isValidChoice(auth.Method, validMethods) {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s.method '%s' is invalid, must be one of: %v", prefix, auth.Method, validMethods))
	}

	// Validate specific authentication configurations
	switch auth.Method {
	case "token", "":
		v.validateTokenAuthConfig(&auth.Token, fmt.Sprintf("%s.token", prefix), result)
	case "service_account":
		v.validateServiceAccountConfig(&auth.ServiceAccount, fmt.Sprintf("%s.service_account", prefix), result)
	case "oidc":
		v.validateOIDCConfig(&auth.OIDC, fmt.Sprintf("%s.oidc", prefix), result)
	case "exec":
		v.validateExecConfig(&auth.Exec, fmt.Sprintf("%s.exec", prefix), result)
	}

	// Validate TLS configuration
	v.validateTLSConfig(tls, fmt.Sprintf("%s.tls", prefix), result)
	
	// Use auth manager for comprehensive validation
	dummyCluster := MultiClusterClusterConfig{
		Name: "validation-dummy",
		Auth: *auth,
		TLS:  *tls,
	}
	
	if err := authManager.ValidateAuthentication(&dummyCluster); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", prefix, err.Error()))
	}
	
	if err := authManager.ValidateTLSConfig(tls, "validation-dummy"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.tls: %s", prefix, err.Error()))
	}
}

// validateTokenAuthConfig validates token authentication configuration
func (v *MultiClusterValidator) validateTokenAuthConfig(tokenConfig *TokenAuthConfig, prefix string, result *MultiClusterValidationResult) {
	if tokenConfig.Value == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.value is required", prefix))
		return
	}
	
	if len(tokenConfig.Value) < 10 {
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("%s.value seems too short, ensure it's a valid token", prefix))
	}
	
	validTypes := []string{"bearer", "service_account"}
	if tokenConfig.Type != "" && !v.isValidChoice(tokenConfig.Type, validTypes) {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s.type '%s' is invalid, must be one of: %v", prefix, tokenConfig.Type, validTypes))
	}
	
	if tokenConfig.RefreshThreshold < 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.refresh_threshold must be non-negative", prefix))
	}
}

// validateServiceAccountConfig validates service account authentication configuration
func (v *MultiClusterValidator) validateServiceAccountConfig(saConfig *ServiceAccountConfig, prefix string, result *MultiClusterValidationResult) {
	if saConfig.TokenPath == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.token_path is required", prefix))
	}
	
	// Note: We don't check if files exist during validation as they might not be available
	// in the validation environment. The auth manager will handle runtime validation.
}

// validateOIDCConfig validates OIDC authentication configuration
func (v *MultiClusterValidator) validateOIDCConfig(oidcConfig *OIDCConfig, prefix string, result *MultiClusterValidationResult) {
	if oidcConfig.IssuerURL == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.issuer_url is required", prefix))
	}
	
	if oidcConfig.ClientID == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.client_id is required", prefix))
	}
	
	if oidcConfig.IDToken == "" && oidcConfig.RefreshToken == "" {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s: either id_token or refresh_token is required", prefix))
	}
}

// validateExecConfig validates exec authentication configuration
func (v *MultiClusterValidator) validateExecConfig(execConfig *ExecConfig, prefix string, result *MultiClusterValidationResult) {
	if execConfig.Command == "" {
		result.Errors = append(result.Errors, fmt.Sprintf("%s.command is required", prefix))
	}
}

// validateTLSConfig validates TLS configuration
func (v *MultiClusterValidator) validateTLSConfig(tlsConfig *ClusterTLSConfig, prefix string, result *MultiClusterValidationResult) {
	// If insecure is true, warn about security implications
	if tlsConfig.Insecure {
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("%s.insecure is true, this disables TLS verification and is not recommended for production", prefix))
		return
	}

	// Check for conflicting certificate configurations
	hasCAData := tlsConfig.CAData != ""
	hasCABundle := tlsConfig.CABundle != ""
	hasCertData := tlsConfig.CertData != "" && tlsConfig.KeyData != ""
	hasCertFiles := tlsConfig.CertFile != "" && tlsConfig.KeyFile != ""
	
	if hasCAData && hasCABundle {
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("%s: both ca_data and ca_bundle specified, ca_data will take precedence", prefix))
	}
	
	if hasCertData && hasCertFiles {
		result.Warnings = append(result.Warnings, 
			fmt.Sprintf("%s: both cert_data/key_data and cert_file/key_file specified, cert_data will take precedence", prefix))
	}
	
	// Check for incomplete client certificate configuration
	if (tlsConfig.CertData != "" || tlsConfig.CertFile != "") && 
	   (tlsConfig.KeyData == "" && tlsConfig.KeyFile == "") {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s: client certificate specified but no key provided", prefix))
	}
	
	if (tlsConfig.KeyData != "" || tlsConfig.KeyFile != "") && 
	   (tlsConfig.CertData == "" && tlsConfig.CertFile == "") {
		result.Errors = append(result.Errors, 
			fmt.Sprintf("%s: client key specified but no certificate provided", prefix))
	}
}

// convertToValidationResult converts MultiClusterValidationResult to ValidationResult
func (v *MultiClusterValidator) convertToValidationResult(multiResult *MultiClusterValidationResult) *ValidationResult {
	result := &ValidationResult{
		Valid: multiResult.Valid,
	}
	
	// Convert errors
	for _, errMsg := range multiResult.Errors {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "multi_cluster",
			Message: errMsg,
		})
	}
	
	// Convert warnings  
	for _, warnMsg := range multiResult.Warnings {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "multi_cluster",
			Message: warnMsg,
		})
	}
	
	return result
}