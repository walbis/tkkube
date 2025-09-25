package sharedconfig

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnhancedMultiClusterValidator provides comprehensive validation for multi-cluster configurations
// including token validation and cluster connectivity checks
type EnhancedMultiClusterValidator struct {
	baseValidator      *MultiClusterValidator
	authManager        *ClusterAuthManager
	
	// Configuration options
	enableConnectivityChecks bool
	enableTokenValidation   bool
	validationTimeout       time.Duration
	maxConcurrentChecks     int
	
	// State management
	mutex               sync.RWMutex
	lastValidationTime  time.Time
	cachedResults       map[string]*ClusterValidationResult
	cacheTimeout        time.Duration
}

// ClusterValidationResult represents detailed validation result for a single cluster
type ClusterValidationResult struct {
	ClusterName         string                    `json:"cluster_name"`
	Valid               bool                      `json:"valid"`
	Errors              []ValidationError         `json:"errors,omitempty"`
	Warnings            []ValidationError         `json:"warnings,omitempty"`
	ConnectivityStatus  *ConnectivityStatus       `json:"connectivity_status,omitempty"`
	TokenValidation     *TokenValidationResult    `json:"token_validation,omitempty"`
	PerformanceMetrics  *ValidationMetrics        `json:"performance_metrics,omitempty"`
	ValidatedAt         time.Time                 `json:"validated_at"`
}

// ConnectivityStatus represents cluster connectivity information
type ConnectivityStatus struct {
	Reachable           bool              `json:"reachable"`
	ResponseTime        time.Duration     `json:"response_time"`
	TLSValid            bool              `json:"tls_valid"`
	APIServerVersion    string            `json:"api_server_version,omitempty"`
	AuthenticationValid bool              `json:"authentication_valid"`
	ErrorDetails        string            `json:"error_details,omitempty"`
	LastChecked         time.Time         `json:"last_checked"`
}

// TokenValidationResult represents token validation information
type TokenValidationResult struct {
	Valid               bool              `json:"valid"`
	TokenType           string            `json:"token_type"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
	Permissions         []string          `json:"permissions,omitempty"`
	Subject             string            `json:"subject,omitempty"`
	Issuer              string            `json:"issuer,omitempty"`
	ErrorDetails        string            `json:"error_details,omitempty"`
	ValidationMethod    string            `json:"validation_method"`
}

// ValidationMetrics represents performance metrics for validation operations
type ValidationMetrics struct {
	ConfigurationValidationTime time.Duration `json:"configuration_validation_time"`
	ConnectivityCheckTime       time.Duration `json:"connectivity_check_time"`
	TokenValidationTime         time.Duration `json:"token_validation_time"`
	TotalValidationTime         time.Duration `json:"total_validation_time"`
}

// EnhancedValidationOptions configures validation behavior
type EnhancedValidationOptions struct {
	EnableConnectivityChecks bool
	EnableTokenValidation    bool
	EnableLiveValidation     bool
	ValidationTimeout        time.Duration
	MaxConcurrentChecks      int
	CacheTimeout             time.Duration
	SkipTLSVerification      bool  // For testing environments only
}

// NewEnhancedMultiClusterValidator creates a new enhanced validator
func NewEnhancedMultiClusterValidator(options *EnhancedValidationOptions) *EnhancedMultiClusterValidator {
	if options == nil {
		options = &EnhancedValidationOptions{
			EnableConnectivityChecks: true,
			EnableTokenValidation:    true,
			EnableLiveValidation:     false,
			ValidationTimeout:        30 * time.Second,
			MaxConcurrentChecks:      5,
			CacheTimeout:             5 * time.Minute,
		}
	}

	return &EnhancedMultiClusterValidator{
		baseValidator:           NewMultiClusterValidator(),
		authManager:            NewClusterAuthManager(),
		enableConnectivityChecks: options.EnableConnectivityChecks,
		enableTokenValidation:   options.EnableTokenValidation,
		validationTimeout:       options.ValidationTimeout,
		maxConcurrentChecks:     options.MaxConcurrentChecks,
		cachedResults:          make(map[string]*ClusterValidationResult),
		cacheTimeout:           options.CacheTimeout,
	}
}

// ValidateMultiClusterConfigurationWithLiveChecks performs comprehensive validation
// including configuration validation, token validation, and connectivity checks
func (ev *EnhancedMultiClusterValidator) ValidateMultiClusterConfigurationWithLiveChecks(config *MultiClusterConfig) *EnhancedValidationResult {
	log.Printf("Starting enhanced multi-cluster validation for %d clusters", len(config.Clusters))
	
	startTime := time.Now()
	
	result := &EnhancedValidationResult{
		OverallValid:      true,
		ValidationTime:    startTime,
		ClusterResults:    make(map[string]*ClusterValidationResult),
		GlobalErrors:      []ValidationError{},
		GlobalWarnings:    []ValidationError{},
	}

	// Step 1: Basic configuration validation
	configValidationStart := time.Now()
	baseResult := ev.baseValidator.ValidateMultiClusterConfig(config)
	configValidationTime := time.Since(configValidationStart)

	// Convert base validation results
	for _, errMsg := range baseResult.Errors {
		result.GlobalErrors = append(result.GlobalErrors, ValidationError{
			Field:   "configuration",
			Message: errMsg,
		})
	}
	
	for _, warnMsg := range baseResult.Warnings {
		result.GlobalWarnings = append(result.GlobalWarnings, ValidationError{
			Field:   "configuration", 
			Message: warnMsg,
		})
	}

	if !baseResult.Valid {
		result.OverallValid = false
		result.TotalValidationTime = time.Since(startTime)
		log.Printf("Configuration validation failed, skipping live checks")
		return result
	}

	// Step 2: Enhanced validation for each cluster
	if config.Enabled && len(config.Clusters) > 0 {
		clusterValidationStart := time.Now()
		ev.validateClustersEnhanced(config, result)
		clusterValidationTime := time.Since(clusterValidationStart)
		
		log.Printf("Cluster validation completed in %v", clusterValidationTime)
	}

	// Step 3: Cross-cluster validation
	ev.validateCrossClusterConfiguration(config, result)

	// Calculate total validation time and update metrics
	result.TotalValidationTime = time.Since(startTime)
	result.ConfigurationValidationTime = configValidationTime

	// Update overall validation status
	result.OverallValid = len(result.GlobalErrors) == 0
	
	// Count cluster-level failures
	for _, clusterResult := range result.ClusterResults {
		if !clusterResult.Valid {
			result.OverallValid = false
		}
	}

	log.Printf("Enhanced validation completed in %v, overall valid: %v", 
		result.TotalValidationTime, result.OverallValid)

	return result
}

// validateClustersEnhanced performs enhanced validation for all clusters
func (ev *EnhancedMultiClusterValidator) validateClustersEnhanced(config *MultiClusterConfig, result *EnhancedValidationResult) {
	// Use semaphore to limit concurrent validations
	semaphore := make(chan struct{}, ev.maxConcurrentChecks)
	var wg sync.WaitGroup
	resultMutex := sync.Mutex{}

	for i, cluster := range config.Clusters {
		wg.Add(1)
		
		go func(clusterIndex int, clusterConfig MultiClusterClusterConfig) {
			defer wg.Done()
			
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			log.Printf("Validating cluster: %s", clusterConfig.Name)
			
			clusterResult := ev.validateSingleClusterEnhanced(&clusterConfig, config)
			
			resultMutex.Lock()
			result.ClusterResults[clusterConfig.Name] = clusterResult
			resultMutex.Unlock()
			
		}(i, cluster)
	}

	wg.Wait()
}

// validateSingleClusterEnhanced performs comprehensive validation for a single cluster
func (ev *EnhancedMultiClusterValidator) validateSingleClusterEnhanced(cluster *MultiClusterClusterConfig, globalConfig *MultiClusterConfig) *ClusterValidationResult {
	startTime := time.Now()
	
	result := &ClusterValidationResult{
		ClusterName: cluster.Name,
		Valid:       true,
		Errors:      []ValidationError{},
		Warnings:    []ValidationError{},
		ValidatedAt: startTime,
		PerformanceMetrics: &ValidationMetrics{},
	}

	// Check cache first
	if cachedResult := ev.getCachedResult(cluster.Name); cachedResult != nil {
		log.Printf("Using cached validation result for cluster %s", cluster.Name)
		return cachedResult
	}

	// Step 1: Token validation
	if ev.enableTokenValidation {
		tokenValidationStart := time.Now()
		result.TokenValidation = ev.validateClusterToken(cluster)
		result.PerformanceMetrics.TokenValidationTime = time.Since(tokenValidationStart)
		
		if !result.TokenValidation.Valid {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "authentication.token",
				Message: fmt.Sprintf("Token validation failed: %s", result.TokenValidation.ErrorDetails),
			})
			result.Valid = false
		}
	}

	// Step 2: Connectivity validation
	if ev.enableConnectivityChecks {
		connectivityStart := time.Now()
		result.ConnectivityStatus = ev.validateClusterConnectivity(cluster)
		result.PerformanceMetrics.ConnectivityCheckTime = time.Since(connectivityStart)
		
		if !result.ConnectivityStatus.Reachable {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "connectivity",
				Message: fmt.Sprintf("Cluster unreachable: %s", result.ConnectivityStatus.ErrorDetails),
			})
			result.Valid = false
		} else if !result.ConnectivityStatus.AuthenticationValid {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "authentication",
				Message: "Authentication failed during connectivity test",
			})
			result.Valid = false
		}
		
		if !result.ConnectivityStatus.TLSValid {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   "tls",
				Message: "TLS validation failed or skipped",
			})
		}
	}

	// Step 3: Additional enhanced validations
	ev.validateClusterConfiguration(cluster, result)
	ev.validateStorageConnectivity(cluster, result)

	// Calculate total validation time
	result.PerformanceMetrics.TotalValidationTime = time.Since(startTime)
	result.PerformanceMetrics.ConfigurationValidationTime = 
		result.PerformanceMetrics.TotalValidationTime - 
		result.PerformanceMetrics.TokenValidationTime - 
		result.PerformanceMetrics.ConnectivityCheckTime

	// Cache the result
	ev.cacheResult(cluster.Name, result)

	log.Printf("Cluster %s validation completed: valid=%v, errors=%d, warnings=%d", 
		cluster.Name, result.Valid, len(result.Errors), len(result.Warnings))

	return result
}

// validateClusterToken performs comprehensive token validation
func (ev *EnhancedMultiClusterValidator) validateClusterToken(cluster *MultiClusterClusterConfig) *TokenValidationResult {
	result := &TokenValidationResult{
		Valid:            false,
		ValidationMethod: cluster.Auth.Method,
	}

	switch cluster.Auth.Method {
	case "token", "":
		return ev.validateBearerToken(&cluster.Auth.Token, cluster)
	case "service_account":
		return ev.validateServiceAccountToken(&cluster.Auth.ServiceAccount, cluster)
	case "oidc":
		return ev.validateOIDCToken(&cluster.Auth.OIDC, cluster)
	case "exec":
		return ev.validateExecToken(&cluster.Auth.Exec, cluster)
	default:
		result.ErrorDetails = fmt.Sprintf("Unknown authentication method: %s", cluster.Auth.Method)
		return result
	}
}

// validateBearerToken validates bearer token authentication
func (ev *EnhancedMultiClusterValidator) validateBearerToken(tokenConfig *TokenAuthConfig, cluster *MultiClusterClusterConfig) *TokenValidationResult {
	result := &TokenValidationResult{
		Valid:            false,
		TokenType:        "bearer",
		ValidationMethod: "token",
	}

	if tokenConfig.Value == "" {
		result.ErrorDetails = "Token value is empty"
		return result
	}

	// Basic format validation
	token := tokenConfig.Value
	if len(token) < 10 {
		result.ErrorDetails = "Token appears to be too short"
		return result
	}

	// Check for environment variable that hasn't been expanded
	if strings.Contains(token, "${") && strings.Contains(token, "}") {
		result.ErrorDetails = "Token appears to contain unexpanded environment variable"
		return result
	}

	// Try to extract token information (basic JWT parsing for service account tokens)
	if strings.Contains(token, ".") {
		// Looks like a JWT token
		parts := strings.Split(token, ".")
		if len(parts) == 3 {
			result.TokenType = "jwt"
			// Could add JWT parsing here to extract expiration, subject, etc.
		}
	}

	// If we can perform connectivity checks, test the token
	if ev.enableConnectivityChecks {
		if ev.testTokenAuthentication(cluster, token) {
			result.Valid = true
		} else {
			result.ErrorDetails = "Token authentication test failed"
		}
	} else {
		// Without connectivity checks, we can only do basic validation
		result.Valid = true
	}

	return result
}

// validateServiceAccountToken validates service account token authentication
func (ev *EnhancedMultiClusterValidator) validateServiceAccountToken(saConfig *ServiceAccountConfig, cluster *MultiClusterClusterConfig) *TokenValidationResult {
	result := &TokenValidationResult{
		Valid:            false,
		TokenType:        "service_account",
		ValidationMethod: "service_account",
	}

	// Validate token path
	if saConfig.TokenPath == "" {
		result.ErrorDetails = "Token path is required for service account authentication"
		return result
	}

	// Check if token file exists (if we're running in the right environment)
	if _, err := os.Stat(saConfig.TokenPath); os.IsNotExist(err) {
		// File doesn't exist - this might be expected in some validation environments
		result.ErrorDetails = fmt.Sprintf("Service account token file not found at %s", saConfig.TokenPath)
		// Don't mark as invalid - might be running outside the pod
		result.Valid = true
		return result
	}

	// Try to read the token
	tokenBytes, err := os.ReadFile(saConfig.TokenPath)
	if err != nil {
		result.ErrorDetails = fmt.Sprintf("Failed to read service account token: %v", err)
		return result
	}

	token := string(tokenBytes)
	if len(token) == 0 {
		result.ErrorDetails = "Service account token file is empty"
		return result
	}

	// Validate CA certificate if specified
	if saConfig.CACertPath != "" {
		if _, err := os.Stat(saConfig.CACertPath); os.IsNotExist(err) {
			result.ErrorDetails = fmt.Sprintf("CA certificate file not found at %s", saConfig.CACertPath)
			return result
		}
	}

	result.Valid = true
	return result
}

// validateOIDCToken validates OIDC token authentication
func (ev *EnhancedMultiClusterValidator) validateOIDCToken(oidcConfig *OIDCConfig, cluster *MultiClusterClusterConfig) *TokenValidationResult {
	result := &TokenValidationResult{
		Valid:            false,
		TokenType:        "oidc",
		ValidationMethod: "oidc",
		Issuer:           oidcConfig.IssuerURL,
	}

	// Basic OIDC configuration validation
	if oidcConfig.IssuerURL == "" {
		result.ErrorDetails = "OIDC issuer URL is required"
		return result
	}

	if oidcConfig.ClientID == "" {
		result.ErrorDetails = "OIDC client ID is required"
		return result
	}

	if oidcConfig.IDToken == "" && oidcConfig.RefreshToken == "" {
		result.ErrorDetails = "Either ID token or refresh token is required"
		return result
	}

	// Validate issuer URL format
	if _, err := url.Parse(oidcConfig.IssuerURL); err != nil {
		result.ErrorDetails = fmt.Sprintf("Invalid issuer URL format: %v", err)
		return result
	}

	// If we have an ID token, try to validate it
	if oidcConfig.IDToken != "" {
		// Basic JWT structure validation
		parts := strings.Split(oidcConfig.IDToken, ".")
		if len(parts) != 3 {
			result.ErrorDetails = "ID token does not appear to be a valid JWT"
			return result
		}
		// Could add JWT parsing and signature validation here
	}

	result.Valid = true
	return result
}

// validateExecToken validates exec authentication
func (ev *EnhancedMultiClusterValidator) validateExecToken(execConfig *ExecConfig, cluster *MultiClusterClusterConfig) *TokenValidationResult {
	result := &TokenValidationResult{
		Valid:            false,
		TokenType:        "exec",
		ValidationMethod: "exec",
	}

	// Validate command
	if execConfig.Command == "" {
		result.ErrorDetails = "Exec command is required"
		return result
	}

	// Check if command exists
	if _, err := os.Stat(execConfig.Command); os.IsNotExist(err) {
		// Try to find in PATH
		if !ev.commandExists(execConfig.Command) {
			result.ErrorDetails = fmt.Sprintf("Exec command not found: %s", execConfig.Command)
			return result
		}
	}

	// Could try executing the command to test token retrieval, but this might have side effects
	result.Valid = true
	return result
}

// validateClusterConnectivity performs connectivity validation
func (ev *EnhancedMultiClusterValidator) validateClusterConnectivity(cluster *MultiClusterClusterConfig) *ConnectivityStatus {
	status := &ConnectivityStatus{
		Reachable:           false,
		TLSValid:           false,
		AuthenticationValid: false,
		LastChecked:        time.Now(),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ev.validationTimeout)
	defer cancel()

	connectStart := time.Now()

	// Step 1: Basic network connectivity
	if !ev.testNetworkConnectivity(cluster.Endpoint) {
		status.ErrorDetails = "Network connectivity failed"
		status.ResponseTime = time.Since(connectStart)
		return status
	}
	
	status.Reachable = true
	status.ResponseTime = time.Since(connectStart)

	// Step 2: TLS validation
	if !cluster.TLS.Insecure {
		status.TLSValid = ev.testTLSConnectivity(cluster)
	} else {
		status.TLSValid = true // Considered "valid" if explicitly disabled
	}

	// Step 3: Kubernetes API connectivity and authentication
	if ev.testKubernetesAPIConnectivity(ctx, cluster) {
		status.AuthenticationValid = true
		
		// Try to get server version
		if version, err := ev.getServerVersion(ctx, cluster); err == nil {
			status.APIServerVersion = version
		}
	}

	return status
}

// testNetworkConnectivity tests basic network connectivity
func (ev *EnhancedMultiClusterValidator) testNetworkConnectivity(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}

	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// testTLSConnectivity tests TLS connectivity
func (ev *EnhancedMultiClusterValidator) testTLSConnectivity(cluster *MultiClusterClusterConfig) bool {
	u, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return false
	}

	config := &tls.Config{
		ServerName: u.Hostname(),
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		u.Host,
		config,
	)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// testTokenAuthentication tests if a token can authenticate
func (ev *EnhancedMultiClusterValidator) testTokenAuthentication(cluster *MultiClusterClusterConfig, token string) bool {
	// Create a simple HTTP request to the API server
	req, err := http.NewRequest("GET", cluster.Endpoint+"/api/v1", nil)
	if err != nil {
		return false
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cluster.TLS.Insecure,
			},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 200 or 401 both indicate the API server is responding
	// 403 might indicate insufficient permissions but valid auth
	return resp.StatusCode == 200 || resp.StatusCode == 403
}

// testKubernetesAPIConnectivity tests full Kubernetes API connectivity
func (ev *EnhancedMultiClusterValidator) testKubernetesAPIConnectivity(ctx context.Context, cluster *MultiClusterClusterConfig) bool {
	// Create REST config
	restConfig, err := ev.authManager.CreateRESTConfig(cluster)
	if err != nil {
		return false
	}

	// Set timeout
	restConfig.Timeout = 10 * time.Second

	// Create client
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return false
	}

	// Try to list namespaces with limit 1 as a connectivity test
	_, err = client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	return err == nil
}

// getServerVersion gets Kubernetes server version
func (ev *EnhancedMultiClusterValidator) getServerVersion(ctx context.Context, cluster *MultiClusterClusterConfig) (string, error) {
	restConfig, err := ev.authManager.CreateRESTConfig(cluster)
	if err != nil {
		return "", err
	}

	restConfig.Timeout = 5 * time.Second
	
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", err
	}

	version, err := client.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}

	return version.String(), nil
}

// validateClusterConfiguration performs additional cluster configuration validation
func (ev *EnhancedMultiClusterValidator) validateClusterConfiguration(cluster *MultiClusterClusterConfig, result *ClusterValidationResult) {
	// Validate cluster name format for Kubernetes compliance
	if !ev.isKubernetesCompliantName(cluster.Name) {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "name",
			Message: "Cluster name may not be Kubernetes compliant",
		})
	}

	// Validate endpoint security
	if !strings.HasPrefix(cluster.Endpoint, "https://") {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "endpoint",
			Message: "Cluster endpoint must use HTTPS for security",
		})
		result.Valid = false
	}

	// Check for default/demo tokens
	if ev.containsDemoToken(cluster) {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "authentication",
			Message: "Appears to contain demo or default token - not suitable for production",
		})
		result.Valid = false
	}
}

// validateStorageConnectivity validates storage backend connectivity
func (ev *EnhancedMultiClusterValidator) validateStorageConnectivity(cluster *MultiClusterClusterConfig, result *ClusterValidationResult) {
	if !ev.enableConnectivityChecks {
		return
	}

	storage := &cluster.Storage
	
	// Test storage endpoint connectivity
	if storage.Endpoint != "" {
		if !ev.testStorageConnectivity(storage) {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   "storage.connectivity",
				Message: "Storage endpoint connectivity test failed",
			})
		}
	}
}

// testStorageConnectivity tests storage endpoint connectivity
func (ev *EnhancedMultiClusterValidator) testStorageConnectivity(storage *StorageConfig) bool {
	// Parse endpoint
	endpoint := storage.Endpoint
	if !strings.HasPrefix(endpoint, "http") {
		if storage.UseSSL {
			endpoint = "https://" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}

	// Test basic connectivity
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// validateCrossClusterConfiguration validates configuration across clusters
func (ev *EnhancedMultiClusterValidator) validateCrossClusterConfiguration(config *MultiClusterConfig, result *EnhancedValidationResult) {
	if !config.Enabled || len(config.Clusters) < 2 {
		return
	}

	// Check for storage conflicts
	ev.validateStorageConfiguration(config, result)
	
	// Check for network conflicts
	ev.validateNetworkConfiguration(config, result)
	
	// Validate load distribution
	ev.validateLoadDistribution(config, result)
}

// validateStorageConfiguration validates storage setup across clusters
func (ev *EnhancedMultiClusterValidator) validateStorageConfiguration(config *MultiClusterConfig, result *EnhancedValidationResult) {
	buckets := make(map[string][]string)
	
	for _, cluster := range config.Clusters {
		key := fmt.Sprintf("%s/%s", cluster.Storage.Endpoint, cluster.Storage.Bucket)
		buckets[key] = append(buckets[key], cluster.Name)
	}
	
	for key, clusters := range buckets {
		if len(clusters) > 1 {
			result.GlobalWarnings = append(result.GlobalWarnings, ValidationError{
				Field:   "storage.buckets",
				Message: fmt.Sprintf("Multiple clusters using same bucket %s: %v", key, clusters),
			})
		}
	}
}

// validateNetworkConfiguration validates network setup
func (ev *EnhancedMultiClusterValidator) validateNetworkConfiguration(config *MultiClusterConfig, result *EnhancedValidationResult) {
	hosts := make(map[string][]string)
	
	for _, cluster := range config.Clusters {
		u, err := url.Parse(cluster.Endpoint)
		if err != nil {
			continue
		}
		hosts[u.Host] = append(hosts[u.Host], cluster.Name)
	}
	
	for host, clusters := range hosts {
		if len(clusters) > 1 {
			result.GlobalWarnings = append(result.GlobalWarnings, ValidationError{
				Field:   "network.endpoints",
				Message: fmt.Sprintf("Multiple clusters using same host %s: %v", host, clusters),
			})
		}
	}
}

// validateLoadDistribution validates load distribution configuration
func (ev *EnhancedMultiClusterValidator) validateLoadDistribution(config *MultiClusterConfig, result *EnhancedValidationResult) {
	if config.Scheduling.Strategy == "priority" && len(config.Scheduling.ClusterPriorities) > 0 {
		// Check for balanced priority distribution
		priorities := make(map[int]int)
		for _, priority := range config.Scheduling.ClusterPriorities {
			priorities[priority.Priority]++
		}
		
		// Warn if too many clusters have the same priority
		for priority, count := range priorities {
			if count > len(config.Clusters)/2 {
				result.GlobalWarnings = append(result.GlobalWarnings, ValidationError{
					Field:   "scheduling.priorities",
					Message: fmt.Sprintf("Too many clusters (%d) have priority %d", count, priority),
				})
			}
		}
	}
}

// Helper functions

// getCachedResult retrieves cached validation result
func (ev *EnhancedMultiClusterValidator) getCachedResult(clusterName string) *ClusterValidationResult {
	ev.mutex.RLock()
	defer ev.mutex.RUnlock()
	
	if result, exists := ev.cachedResults[clusterName]; exists {
		if time.Since(result.ValidatedAt) < ev.cacheTimeout {
			return result
		}
	}
	
	return nil
}

// cacheResult caches validation result
func (ev *EnhancedMultiClusterValidator) cacheResult(clusterName string, result *ClusterValidationResult) {
	ev.mutex.Lock()
	defer ev.mutex.Unlock()
	
	ev.cachedResults[clusterName] = result
}

// commandExists checks if a command exists in PATH
func (ev *EnhancedMultiClusterValidator) commandExists(command string) bool {
	if strings.Contains(command, "/") {
		// Absolute or relative path
		if _, err := os.Stat(command); err == nil {
			return true
		}
	}
	
	// Check in PATH
	_, err := exec.LookPath(command)
	return err == nil
}

// isKubernetesCompliantName checks if name follows Kubernetes naming conventions
func (ev *EnhancedMultiClusterValidator) isKubernetesCompliantName(name string) bool {
	// Kubernetes object names must:
	// - contain only lowercase alphanumeric characters or '-'
	// - start and end with an alphanumeric character
	// - be at most 253 characters long
	if len(name) > 253 {
		return false
	}
	
	matched, _ := regexp.MatchString(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, name)
	return matched
}

// containsDemoToken checks for demo or default tokens
func (ev *EnhancedMultiClusterValidator) containsDemoToken(cluster *MultiClusterClusterConfig) bool {
	demoPatterns := []string{
		"demo", "test", "example", "sample", "default", "admin", "password",
		"123456", "token123", "secret123",
	}
	
	var tokenValue string
	switch cluster.Auth.Method {
	case "token", "":
		tokenValue = strings.ToLower(cluster.Auth.Token.Value)
	case "oidc":
		tokenValue = strings.ToLower(cluster.Auth.OIDC.ClientSecret)
	}
	
	for _, pattern := range demoPatterns {
		if strings.Contains(tokenValue, pattern) {
			return true
		}
	}
	
	return false
}

// EnhancedValidationResult represents the complete validation result
type EnhancedValidationResult struct {
	OverallValid                  bool                                    `json:"overall_valid"`
	ValidationTime                time.Time                               `json:"validation_time"`
	TotalValidationTime           time.Duration                           `json:"total_validation_time"`
	ConfigurationValidationTime   time.Duration                           `json:"configuration_validation_time"`
	ClusterResults                map[string]*ClusterValidationResult    `json:"cluster_results"`
	GlobalErrors                  []ValidationError                       `json:"global_errors,omitempty"`
	GlobalWarnings                []ValidationError                       `json:"global_warnings,omitempty"`
	Summary                       *ValidationSummary                      `json:"summary"`
}

// ValidationSummary provides a summary of validation results
type ValidationSummary struct {
	TotalClusters        int `json:"total_clusters"`
	ValidClusters        int `json:"valid_clusters"`
	InvalidClusters      int `json:"invalid_clusters"`
	ClustersWithWarnings int `json:"clusters_with_warnings"`
	TotalErrors          int `json:"total_errors"`
	TotalWarnings        int `json:"total_warnings"`
}

// GenerateSummary generates a summary of validation results
func (result *EnhancedValidationResult) GenerateSummary() {
	summary := &ValidationSummary{
		TotalClusters: len(result.ClusterResults),
		TotalErrors:   len(result.GlobalErrors),
		TotalWarnings: len(result.GlobalWarnings),
	}
	
	for _, clusterResult := range result.ClusterResults {
		if clusterResult.Valid {
			summary.ValidClusters++
		} else {
			summary.InvalidClusters++
		}
		
		if len(clusterResult.Warnings) > 0 {
			summary.ClustersWithWarnings++
		}
		
		summary.TotalErrors += len(clusterResult.Errors)
		summary.TotalWarnings += len(clusterResult.Warnings)
	}
	
	result.Summary = summary
}