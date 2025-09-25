package sharedconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// SharedConfig represents the complete unified configuration
type SharedConfig struct {
	SchemaVersion string                  `yaml:"schema_version"`
	Description   string                  `yaml:"description"`
	Storage       StorageConfig           `yaml:"storage"`
	Cluster       SingleClusterConfig     `yaml:"cluster"`
	MultiCluster  MultiClusterConfig      `yaml:"multi_cluster"`
	Backup        BackupConfig            `yaml:"backup"`
	GitOps        GitOpsConfig            `yaml:"gitops"`
	Pipeline      PipelineConfig          `yaml:"pipeline"`
	Observability ObservabilityConfig     `yaml:"observability"`
	Security      SecurityConfig          `yaml:"security"`
	Performance   PerformanceConfig       `yaml:"performance"`
	Features      FeaturesConfig          `yaml:"features"`
	Integration   IntegrationConfig       `yaml:"integration"`
	Timeouts      TimeoutConfig           `yaml:"timeouts"`
	Retries       RetryConfig             `yaml:"retries"`
}

// StorageConfig defines storage backend configuration
type StorageConfig struct {
	Type             string                  `yaml:"type"`
	Endpoint         string                  `yaml:"endpoint"`
	AccessKey        string                  `yaml:"access_key"`
	SecretKey        string                  `yaml:"secret_key"`
	Bucket           string                  `yaml:"bucket"`
	UseSSL           bool                    `yaml:"use_ssl"`
	Region           string                  `yaml:"region"`
	AutoCreateBucket bool                    `yaml:"auto_create_bucket"`
	FallbackBuckets  []string                `yaml:"fallback_buckets"`
	Connection       ConnectionConfig        `yaml:"connection"`
}

// ConnectionConfig defines connection parameters
type ConnectionConfig struct {
	Timeout    int           `yaml:"timeout"`
	MaxRetries int           `yaml:"max_retries"`
	RetryDelay time.Duration `yaml:"retry_delay"`
}

// ClusterConfig defines cluster-specific settings
type SingleClusterConfig struct {
	Name      string            `yaml:"name"`
	Domain    string            `yaml:"domain"`
	Type      string            `yaml:"type"`
	OpenShift OpenShiftConfig   `yaml:"openshift"`
}

// OpenShiftConfig defines OpenShift-specific settings
type OpenShiftConfig struct {
	Mode             string `yaml:"mode"`
	IncludeResources bool   `yaml:"include_resources"`
}

// BackupConfig defines backup behavior and filtering
type BackupConfig struct {
	Filtering FilteringConfig `yaml:"filtering"`
	Behavior  BehaviorConfig  `yaml:"behavior"`
	Cleanup   CleanupConfig   `yaml:"cleanup"`
}

// FilteringConfig defines resource and namespace filtering
type FilteringConfig struct {
	Mode               string              `yaml:"mode"`
	Resources          ResourceFilter      `yaml:"resources"`
	Namespaces         NamespaceFilter     `yaml:"namespaces"`
	LabelSelector      string              `yaml:"label_selector"`
	AnnotationSelector string              `yaml:"annotation_selector"`
}

// ResourceFilter defines resource inclusion/exclusion
type ResourceFilter struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// NamespaceFilter defines namespace inclusion/exclusion
type NamespaceFilter struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// BehaviorConfig defines backup behavior settings
type BehaviorConfig struct {
	BatchSize              int    `yaml:"batch_size"`
	ValidateYAML           bool   `yaml:"validate_yaml"`
	SkipInvalidResources   bool   `yaml:"skip_invalid_resources"`
	IncludeManagedFields   bool   `yaml:"include_managed_fields"`
	IncludeStatus          bool   `yaml:"include_status"`
	MaxResourceSize        string `yaml:"max_resource_size"`
	FollowOwnerReferences  bool   `yaml:"follow_owner_references"`
}

// CleanupConfig defines cleanup policy
type CleanupConfig struct {
	Enabled           bool `yaml:"enabled"`
	RetentionDays     int  `yaml:"retention_days"`
	CleanupOnStartup  bool `yaml:"cleanup_on_startup"`
}

// GitOpsConfig defines GitOps generation settings
type GitOpsConfig struct {
	Repository RepositoryConfig `yaml:"repository"`
	Structure  StructureConfig  `yaml:"structure"`
}

// RepositoryConfig defines Git repository settings
type RepositoryConfig struct {
	URL    string     `yaml:"url"`
	Branch string     `yaml:"branch"`
	Auth   AuthConfig `yaml:"auth"`
}

// AuthConfig defines authentication settings
type AuthConfig struct {
	Method string          `yaml:"method"`
	SSH    SSHAuthConfig   `yaml:"ssh"`
	PAT    PATAuthConfig   `yaml:"pat"`
	Basic  BasicAuthConfig `yaml:"basic"`
}

// SSHAuthConfig defines SSH authentication
type SSHAuthConfig struct {
	PrivateKeyPath string `yaml:"private_key_path"`
	Passphrase     string `yaml:"passphrase"`
}

// PATAuthConfig defines Personal Access Token authentication
type PATAuthConfig struct {
	Token    string `yaml:"token"`
	Username string `yaml:"username"`
}

// BasicAuthConfig defines basic authentication
type BasicAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// StructureConfig defines GitOps structure generation
type StructureConfig struct {
	BaseDir      string              `yaml:"base_dir"`
	Environments []EnvironmentConfig `yaml:"environments"`
	ArgoCD       ArgoCDConfig        `yaml:"argocd"`
	Kustomize    KustomizeConfig     `yaml:"kustomize"`
}

// EnvironmentConfig defines environment-specific settings
type EnvironmentConfig struct {
	Name       string `yaml:"name"`
	ClusterURL string `yaml:"cluster_url"`
	AutoSync   bool   `yaml:"auto_sync"`
	Replicas   int    `yaml:"replicas"`
}

// ArgoCDConfig defines ArgoCD settings
type ArgoCDConfig struct {
	Enabled    bool           `yaml:"enabled"`
	Namespace  string         `yaml:"namespace"`
	Project    string         `yaml:"project"`
	SyncPolicy SyncPolicyConf `yaml:"sync_policy"`
}

// SyncPolicyConfig defines ArgoCD sync policy
type SyncPolicyConf struct {
	Automated bool `yaml:"automated"`
	Prune     bool `yaml:"prune"`
	SelfHeal  bool `yaml:"self_heal"`
}

// KustomizeConfig defines Kustomize settings
type KustomizeConfig struct {
	Enabled        bool `yaml:"enabled"`
	StrategicMerge bool `yaml:"strategic_merge"`
}

// PipelineConfig defines pipeline integration settings
type PipelineConfig struct {
	Mode          string              `yaml:"mode"`
	Automation    AutomationConfig    `yaml:"automation"`
	Notifications NotificationsConfig `yaml:"notifications"`
	ErrorHandling ErrorHandlingConfig `yaml:"error_handling"`
}

// AutomationConfig defines automation settings
type AutomationConfig struct {
	Enabled                  bool                 `yaml:"enabled"`
	TriggerOnBackupComplete  bool                 `yaml:"trigger_on_backup_complete"`
	WaitForBackup            bool                 `yaml:"wait_for_backup"`
	MaxWaitTime              int                  `yaml:"max_wait_time"`
	TriggerMethods           []string             `yaml:"trigger_methods"`
	FileTrigger              FileTriggerConfig    `yaml:"file_trigger"`
	ProcessTrigger           ProcessTriggerConfig `yaml:"process_trigger"`
	WebhookTrigger           WebhookTriggerConfig `yaml:"webhook_trigger"`
}

// FileTriggerConfig defines file-based trigger settings
type FileTriggerConfig struct {
	Enabled                bool   `yaml:"enabled"`
	Directory              string `yaml:"directory"`
	CleanupAfterProcessing bool   `yaml:"cleanup_after_processing"`
}

// ProcessTriggerConfig defines process-based trigger settings
type ProcessTriggerConfig struct {
	Enabled           bool   `yaml:"enabled"`
	GitOpsBinaryPath  string `yaml:"gitops_binary_path"`
	AdditionalArgs    string `yaml:"additional_args"`
}

// WebhookTriggerConfig defines webhook-based trigger settings
type WebhookTriggerConfig struct {
	Enabled        bool                     `yaml:"enabled"`
	ServerHost     string                   `yaml:"server_host"`
	ServerPort     int                      `yaml:"server_port"`
	EndpointPath   string                   `yaml:"endpoint_path"`
	Authentication WebhookAuthConfig        `yaml:"authentication"`
}

// WebhookAuthConfig defines webhook authentication settings
type WebhookAuthConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Token      string `yaml:"token"`
	HeaderName string `yaml:"header_name"`
}

// NotificationsConfig defines notification settings
type NotificationsConfig struct {
	Enabled bool            `yaml:"enabled"`
	Webhook WebhookConfig   `yaml:"webhook"`
	Slack   SlackConfig     `yaml:"slack"`
}

// WebhookConfig defines webhook notifications
type WebhookConfig struct {
	URL       string `yaml:"url"`
	OnSuccess bool   `yaml:"on_success"`
	OnFailure bool   `yaml:"on_failure"`
}

// SlackConfig defines Slack notifications
type SlackConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	Channel    string `yaml:"channel"`
}

// ErrorHandlingConfig defines error handling behavior
type ErrorHandlingConfig struct {
	ContinueOnError bool          `yaml:"continue_on_error"`
	MaxRetries      int           `yaml:"max_retries"`
	RetryDelay      time.Duration `yaml:"retry_delay"`
}

// ObservabilityConfig defines observability settings
type ObservabilityConfig struct {
	Metrics MetricsConfig `yaml:"metrics"`
	Logging LoggingConfig `yaml:"logging"`
	Tracing TracingConfig `yaml:"tracing"`
}

// MetricsConfig defines metrics settings
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// LoggingConfig defines logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

// TracingConfig defines tracing settings
type TracingConfig struct {
	Enabled    bool    `yaml:"enabled"`
	Endpoint   string  `yaml:"endpoint"`
	SampleRate float64 `yaml:"sample_rate"`
}

// SecurityConfig defines security settings
type SecurityConfig struct {
	Secrets    SecretsConfig    `yaml:"secrets"`
	Network    NetworkConfig    `yaml:"network"`
	Validation ValidationConfig `yaml:"validation"`
}

// SecretsConfig defines secret management
type SecretsConfig struct {
	Provider      string             `yaml:"provider"`
	Vault         VaultConfig        `yaml:"vault"`
	AWSSecrets    AWSSecretsConfig   `yaml:"aws_secrets"`
	AzureKeyVault AzureKeyVaultConf  `yaml:"azure_keyvault"`
}

// VaultConfig defines HashiCorp Vault settings
type VaultConfig struct {
	Address string `yaml:"address"`
	Token   string `yaml:"token"`
	Path    string `yaml:"path"`
}

// AWSSecretsConfig defines AWS Secrets Manager settings
type AWSSecretsConfig struct {
	Region     string `yaml:"region"`
	SecretName string `yaml:"secret_name"`
}

// AzureKeyVaultConfig defines Azure Key Vault settings
type AzureKeyVaultConf struct {
	VaultName string `yaml:"vault_name"`
	TenantID  string `yaml:"tenant_id"`
}

// NetworkConfig defines network security settings
type NetworkConfig struct {
	VerifySSL  bool   `yaml:"verify_ssl"`
	CABundle   string `yaml:"ca_bundle"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
}

// ValidationConfig defines validation settings
type ValidationConfig struct {
	StrictMode      bool   `yaml:"strict_mode"`
	ScanForSecrets  bool   `yaml:"scan_for_secrets"`
	MaxFileSize     string `yaml:"max_file_size"`
}

// PerformanceConfig defines performance settings
type PerformanceConfig struct {
	Limits       LimitsConfig       `yaml:"limits"`
	Optimization OptimizationConfig `yaml:"optimization"`
}

// LimitsConfig defines resource limits
type LimitsConfig struct {
	MaxConcurrentOperations int    `yaml:"max_concurrent_operations"`
	MemoryLimit             string `yaml:"memory_limit"`
	CPULimit                string `yaml:"cpu_limit"`
}

// OptimizationConfig defines optimization settings
type OptimizationConfig struct {
	BatchProcessing bool `yaml:"batch_processing"`
	Compression     bool `yaml:"compression"`
	Caching         bool `yaml:"caching"`
	CacheTTL        int  `yaml:"cache_ttl"`
}

// MultiClusterConfig defines multi-cluster support configuration
type MultiClusterConfig struct {
	Enabled        bool                           `yaml:"enabled"`
	Mode           string                         `yaml:"mode"`
	DefaultCluster string                         `yaml:"default_cluster"`
	Clusters       []MultiClusterClusterConfig   `yaml:"clusters"`
	Coordination   CoordinationConfig            `yaml:"coordination"`
	Scheduling     SchedulingConfig              `yaml:"scheduling"`
}

// MultiClusterClusterConfig defines individual cluster configuration for multi-cluster
type MultiClusterClusterConfig struct {
	Name     string                    `yaml:"name"`
	Endpoint string                    `yaml:"endpoint"`
	Auth     ClusterAuthConfig         `yaml:"auth"`
	TLS      ClusterTLSConfig          `yaml:"tls"`
	Storage  StorageConfig             `yaml:"storage"`
	
	// Legacy support - deprecated in favor of Auth
	Token    string                    `yaml:"token,omitempty"`
}

// ClusterAuthConfig defines authentication configuration for a cluster
type ClusterAuthConfig struct {
	Method         string                 `yaml:"method"`           // token, service_account, oidc, exec
	Token          TokenAuthConfig        `yaml:"token"`
	ServiceAccount ServiceAccountConfig   `yaml:"service_account"`
	OIDC           OIDCConfig            `yaml:"oidc"`
	Exec           ExecConfig            `yaml:"exec"`
}

// TokenAuthConfig defines token-based authentication
type TokenAuthConfig struct {
	Value            string `yaml:"value"`
	Type             string `yaml:"type"`              // bearer, service_account
	RefreshThreshold int    `yaml:"refresh_threshold"` // seconds before expiry to refresh
}

// ServiceAccountConfig defines service account authentication
type ServiceAccountConfig struct {
	TokenPath  string `yaml:"token_path"`
	CACertPath string `yaml:"ca_cert_path"`
}

// OIDCConfig defines OIDC authentication
type OIDCConfig struct {
	IssuerURL    string `yaml:"issuer_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	IDToken      string `yaml:"id_token"`
	RefreshToken string `yaml:"refresh_token"`
}

// ExecConfig defines exec-based authentication (for dynamic token retrieval)
type ExecConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
}

// ClusterTLSConfig defines TLS configuration for cluster connections
type ClusterTLSConfig struct {
	Insecure   bool   `yaml:"insecure"`
	CABundle   string `yaml:"ca_bundle"`    // path to CA bundle file
	CAData     string `yaml:"ca_data"`      // base64 encoded CA certificate data
	CertFile   string `yaml:"cert_file"`    // path to client certificate file
	KeyFile    string `yaml:"key_file"`     // path to client key file
	CertData   string `yaml:"cert_data"`    // base64 encoded client certificate data
	KeyData    string `yaml:"key_data"`     // base64 encoded client key data
	ServerName string `yaml:"server_name"`  // server name for SNI
}

// CoordinationConfig defines multi-cluster coordination settings
type CoordinationConfig struct {
	Timeout             int    `yaml:"timeout"`
	RetryAttempts       int    `yaml:"retry_attempts"`
	FailureThreshold    int    `yaml:"failure_threshold"`
	HealthCheckInterval string `yaml:"health_check_interval"`
}

// SchedulingConfig defines multi-cluster scheduling settings
type SchedulingConfig struct {
	Strategy              string             `yaml:"strategy"`
	MaxConcurrentClusters int                `yaml:"max_concurrent_clusters"`
	ClusterPriorities     []ClusterPriority  `yaml:"cluster_priorities"`
}

// ClusterPriority defines cluster priority for scheduling
type ClusterPriority struct {
	Cluster  string `yaml:"cluster"`
	Priority int    `yaml:"priority"`
}

// FeaturesConfig defines feature flags
type FeaturesConfig struct {
	Experimental ExperimentalFeatures `yaml:"experimental"`
	Preview      PreviewFeatures      `yaml:"preview"`
}

// ExperimentalFeatures defines experimental features
type ExperimentalFeatures struct {
	MultiClusterSupport bool `yaml:"multi_cluster_support"`
	IncrementalBackup   bool `yaml:"incremental_backup"`
	DifferentialSync    bool `yaml:"differential_sync"`
}

// PreviewFeatures defines preview features
type PreviewFeatures struct {
	UIDashboard bool `yaml:"ui_dashboard"`
	RestAPI     bool `yaml:"rest_api"`
}

// IntegrationConfig defines integration bridge settings
type IntegrationConfig struct {
	Enabled       bool                    `yaml:"enabled"`
	WebhookPort   int                     `yaml:"webhook_port"`
	Bridge        BridgeConfig            `yaml:"bridge"`
	Communication CommunicationConfig     `yaml:"communication"`
	Triggers      TriggerIntegrationConfig `yaml:"triggers"`
}

// BridgeConfig defines integration bridge settings
type BridgeConfig struct {
	Enabled         bool          `yaml:"enabled"`
	HealthInterval  time.Duration `yaml:"health_interval"`
	EventBufferSize int           `yaml:"event_buffer_size"`
	MaxConcurrency  int           `yaml:"max_concurrency"`
}

// CommunicationConfig defines cross-component communication
type CommunicationConfig struct {
	Method        string                 `yaml:"method"` // webhook, event-bus, file
	Endpoints     EndpointsConfig        `yaml:"endpoints"`
	Authentication AuthenticationConfig  `yaml:"authentication"`
	Retry         RetryConfig            `yaml:"retry"`
}

// EndpointsConfig defines component endpoints
type EndpointsConfig struct {
	BackupTool     string `yaml:"backup_tool"`
	GitOpsGenerator string `yaml:"gitops_generator"`
	IntegrationBridge string `yaml:"integration_bridge"`
}

// AuthenticationConfig defines authentication for communication
type AuthenticationConfig struct {
	Enabled bool   `yaml:"enabled"`
	Method  string `yaml:"method"` // token, mutual-tls, none
	Token   string `yaml:"token"`
	TLS     TLSConfig `yaml:"tls"`
}

// TLSConfig defines TLS settings
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	// General retry settings
	MaxAttempts  int           `yaml:"max_attempts"`
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Multiplier   float64       `yaml:"multiplier"`

	// Legacy field mappings for backward compatibility
	MaxRetries      int           `yaml:"max_retries"`
	BaseRetryDelay  time.Duration `yaml:"base_retry_delay"`
	MaxRetryDelay   time.Duration `yaml:"max_retry_delay"`
	RetryMultiplier float64       `yaml:"retry_multiplier"`

	// Specific operation retries
	RestoreMaxRetries       int           `yaml:"restore_max_retries"`
	RestoreRetryDelay       time.Duration `yaml:"restore_retry_delay"`
	ValidationMaxRetries    int           `yaml:"validation_max_retries"`
	ValidationRetryDelay    time.Duration `yaml:"validation_retry_delay"`
	GitOpsMaxRetries        int           `yaml:"gitops_max_retries"`
	GitOpsRetryDelay        time.Duration `yaml:"gitops_retry_delay"`
	SecurityMaxRetries      int           `yaml:"security_max_retries"`
	SecurityRetryDelay      time.Duration `yaml:"security_retry_delay"`

	// Circuit breaker settings
	CircuitBreakerThreshold    int           `yaml:"circuit_breaker_threshold"`
	CircuitBreakerTimeout      time.Duration `yaml:"circuit_breaker_timeout"`
	CircuitBreakerRecoveryTime time.Duration `yaml:"circuit_breaker_recovery_time"`
}

// TriggerIntegrationConfig defines trigger integration settings
type TriggerIntegrationConfig struct {
	AutoTrigger    bool          `yaml:"auto_trigger"`
	DelayAfterBackup time.Duration `yaml:"delay_after_backup"`
	ParallelExecution bool         `yaml:"parallel_execution"`
	FallbackMethods []string      `yaml:"fallback_methods"`
}

// ConfigLoader handles loading and merging configurations
type ConfigLoader struct {
	configPaths    []string
	envPrefix      string
	skipValidation bool // For testing purposes
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(paths ...string) *ConfigLoader {
	return &ConfigLoader{
		configPaths:    paths,
		envPrefix:      "BACKUP_",
		skipValidation: false,
	}
}

// NewConfigLoaderForTesting creates a configuration loader that skips validation
func NewConfigLoaderForTesting(paths ...string) *ConfigLoader {
	return &ConfigLoader{
		configPaths:    paths,
		envPrefix:      "BACKUP_",
		skipValidation: true,
	}
}

// Load loads and merges configuration from multiple sources
func (cl *ConfigLoader) Load() (*SharedConfig, error) {
	config := &SharedConfig{
		// Set default values
		Storage: StorageConfig{
			Type:      "minio",
			UseSSL:    true,
			Region:    "us-east-1",
			Connection: ConnectionConfig{
				Timeout:    30,
				MaxRetries: 3,
				RetryDelay: 5 * time.Second,
			},
		},
		Cluster: SingleClusterConfig{
			Domain: "cluster.local",
			Type:   "kubernetes",
		},
		Backup: BackupConfig{
			Behavior: BehaviorConfig{
				BatchSize:    50,
				ValidateYAML: true,
			},
			Cleanup: CleanupConfig{
				Enabled:       true,
				RetentionDays: 7,
			},
		},
		GitOps: GitOpsConfig{
			Repository: RepositoryConfig{
				Branch: "main",
				Auth: AuthConfig{
					Method: "ssh",
				},
			},
		},
		Observability: ObservabilityConfig{
			Logging: LoggingConfig{
				Level:  "info",
				Format: "json",
			},
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    8080,
				Path:    "/metrics",
			},
		},
	}
	
	// Load from files in order
	for _, path := range cl.configPaths {
		if err := cl.loadFile(path, config); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to load config from %s: %v", path, err)
			}
		}
	}
	
	// Apply environment variable overrides
	cl.applyEnvironmentOverrides(config)
	
	// Load timeout and retry configurations from environment variables
	config.Timeouts = LoadTimeoutConfigFromEnv()
	config.Retries = LoadRetryConfigFromEnv()
	
	// Expand environment variables in string fields
	if err := cl.expandEnvironmentVariables(config); err != nil {
		return nil, fmt.Errorf("failed to expand environment variables: %v", err)
	}
	
	// Validate the final configuration (skip if testing)
	if !cl.skipValidation {
		if err := cl.validate(config); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %v", err)
		}
	}
	
	return config, nil
}

// loadFile loads configuration from a YAML file
func (cl *ConfigLoader) loadFile(path string, config *SharedConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	return yaml.Unmarshal(data, config)
}

// applyEnvironmentOverrides applies environment variable overrides
func (cl *ConfigLoader) applyEnvironmentOverrides(config *SharedConfig) {
	// Storage configuration
	if v := os.Getenv("MINIO_ENDPOINT"); v != "" {
		config.Storage.Endpoint = v
	}
	if v := os.Getenv("MINIO_ACCESS_KEY"); v != "" {
		config.Storage.AccessKey = v
	}
	if v := os.Getenv("MINIO_SECRET_KEY"); v != "" {
		config.Storage.SecretKey = v
	}
	if v := os.Getenv("MINIO_BUCKET"); v != "" {
		config.Storage.Bucket = v
	}
	if v := os.Getenv("MINIO_USE_SSL"); v != "" {
		config.Storage.UseSSL = v == "true"
	}
	
	// Cluster configuration
	if v := os.Getenv("CLUSTER_NAME"); v != "" {
		config.Cluster.Name = v
	}
	if v := os.Getenv("CLUSTER_DOMAIN"); v != "" {
		config.Cluster.Domain = v
	}
	
	// Git configuration
	if v := os.Getenv("GIT_REPOSITORY"); v != "" {
		config.GitOps.Repository.URL = v
	}
	if v := os.Getenv("GIT_BRANCH"); v != "" {
		config.GitOps.Repository.Branch = v
	}
	if v := os.Getenv("GIT_AUTH_METHOD"); v != "" {
		config.GitOps.Repository.Auth.Method = v
	}
	
	// Backup configuration
	if v := os.Getenv("BATCH_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			config.Backup.Behavior.BatchSize = size
		}
	}
	if v := os.Getenv("RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil {
			config.Backup.Cleanup.RetentionDays = days
		}
	}
	
	// Logging configuration
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Observability.Logging.Level = v
	}
}

// expandEnvironmentVariables expands ${VAR} references in string fields
func (cl *ConfigLoader) expandEnvironmentVariables(config *SharedConfig) error {
	// This would use reflection to walk through all string fields
	// and expand environment variable references
	// For brevity, showing key fields only
	
	config.Storage.Endpoint = os.ExpandEnv(config.Storage.Endpoint)
	config.Storage.AccessKey = os.ExpandEnv(config.Storage.AccessKey)
	config.Storage.SecretKey = os.ExpandEnv(config.Storage.SecretKey)
	config.Storage.Bucket = os.ExpandEnv(config.Storage.Bucket)
	
	config.Cluster.Name = os.ExpandEnv(config.Cluster.Name)
	config.Cluster.Domain = os.ExpandEnv(config.Cluster.Domain)
	
	config.GitOps.Repository.URL = os.ExpandEnv(config.GitOps.Repository.URL)
	
	// Expand multi-cluster configuration
	for i := range config.MultiCluster.Clusters {
		cluster := &config.MultiCluster.Clusters[i]
		
		// Basic cluster configuration
		cluster.Name = os.ExpandEnv(cluster.Name)
		cluster.Endpoint = os.ExpandEnv(cluster.Endpoint)
		cluster.Token = os.ExpandEnv(cluster.Token) // Legacy support
		
		// Authentication configuration
		cluster.Auth.Method = os.ExpandEnv(cluster.Auth.Method)
		cluster.Auth.Token.Value = os.ExpandEnv(cluster.Auth.Token.Value)
		cluster.Auth.Token.Type = os.ExpandEnv(cluster.Auth.Token.Type)
		cluster.Auth.ServiceAccount.TokenPath = os.ExpandEnv(cluster.Auth.ServiceAccount.TokenPath)
		cluster.Auth.ServiceAccount.CACertPath = os.ExpandEnv(cluster.Auth.ServiceAccount.CACertPath)
		cluster.Auth.OIDC.IssuerURL = os.ExpandEnv(cluster.Auth.OIDC.IssuerURL)
		cluster.Auth.OIDC.ClientID = os.ExpandEnv(cluster.Auth.OIDC.ClientID)
		cluster.Auth.OIDC.ClientSecret = os.ExpandEnv(cluster.Auth.OIDC.ClientSecret)
		cluster.Auth.OIDC.IDToken = os.ExpandEnv(cluster.Auth.OIDC.IDToken)
		cluster.Auth.OIDC.RefreshToken = os.ExpandEnv(cluster.Auth.OIDC.RefreshToken)
		cluster.Auth.Exec.Command = os.ExpandEnv(cluster.Auth.Exec.Command)
		for j := range cluster.Auth.Exec.Args {
			cluster.Auth.Exec.Args[j] = os.ExpandEnv(cluster.Auth.Exec.Args[j])
		}
		for j := range cluster.Auth.Exec.Env {
			cluster.Auth.Exec.Env[j] = os.ExpandEnv(cluster.Auth.Exec.Env[j])
		}
		
		// TLS configuration
		cluster.TLS.CABundle = os.ExpandEnv(cluster.TLS.CABundle)
		cluster.TLS.CAData = os.ExpandEnv(cluster.TLS.CAData)
		cluster.TLS.CertFile = os.ExpandEnv(cluster.TLS.CertFile)
		cluster.TLS.KeyFile = os.ExpandEnv(cluster.TLS.KeyFile)
		cluster.TLS.CertData = os.ExpandEnv(cluster.TLS.CertData)
		cluster.TLS.KeyData = os.ExpandEnv(cluster.TLS.KeyData)
		cluster.TLS.ServerName = os.ExpandEnv(cluster.TLS.ServerName)
		
		// Storage configuration
		cluster.Storage.Endpoint = os.ExpandEnv(cluster.Storage.Endpoint)
		cluster.Storage.AccessKey = os.ExpandEnv(cluster.Storage.AccessKey)
		cluster.Storage.SecretKey = os.ExpandEnv(cluster.Storage.SecretKey)
		cluster.Storage.Bucket = os.ExpandEnv(cluster.Storage.Bucket)
		cluster.Storage.Region = os.ExpandEnv(cluster.Storage.Region)
	}
	config.MultiCluster.DefaultCluster = os.ExpandEnv(config.MultiCluster.DefaultCluster)
	config.MultiCluster.Coordination.HealthCheckInterval = os.ExpandEnv(config.MultiCluster.Coordination.HealthCheckInterval)
	
	return nil
}

// validate validates the configuration using the enhanced validator
func (cl *ConfigLoader) validate(config *SharedConfig) error {
	validationResult, err := ValidateConfig(config)
	if err != nil {
		return err
	}
	
	// Print validation warnings if any
	if len(validationResult.Warnings) > 0 {
		fmt.Printf("Configuration loaded with warnings:\n%s\n", FormatValidationResult(validationResult))
	}
	
	if !validationResult.Valid {
		return fmt.Errorf("configuration validation failed:\n%s", FormatValidationResult(validationResult))
	}
	
	return nil
}

// GetBackupToolConfig converts shared config to backup tool specific config
func (sc *SharedConfig) GetBackupToolConfig() map[string]interface{} {
	return map[string]interface{}{
		"ClusterName":       sc.Cluster.Name,
		"ClusterDomain":     sc.Cluster.Domain,
		"MinIOEndpoint":     sc.Storage.Endpoint,
		"MinIOAccessKey":    sc.Storage.AccessKey,
		"MinIOSecretKey":    sc.Storage.SecretKey,
		"MinIOBucket":       sc.Storage.Bucket,
		"MinIOUseSSL":       sc.Storage.UseSSL,
		"BatchSize":         sc.Backup.Behavior.BatchSize,
		"RetryAttempts":     sc.Storage.Connection.MaxRetries,
		"RetentionDays":     sc.Backup.Cleanup.RetentionDays,
		"EnableCleanup":     sc.Backup.Cleanup.Enabled,
		"CleanupOnStartup":  sc.Backup.Cleanup.CleanupOnStartup,
		"AutoCreateBucket":  sc.Storage.AutoCreateBucket,
	}
}

// SaveToFile saves the configuration to a YAML file
func (sc *SharedConfig) SaveToFile(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	
	// Marshal to YAML
	data, err := yaml.Marshal(sc)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}
	
	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	
	return nil
}

// DefaultConfigPaths returns the default configuration file paths
func DefaultConfigPaths() []string {
	paths := []string{
		"./shared-config.yaml",
		"./config/shared-config.yaml",
		"/etc/backup-gitops/config.yaml",
	}
	
	// Add home directory config
	if home := os.Getenv("HOME"); home != "" {
		paths = append(paths, filepath.Join(home, ".backup-gitops", "config.yaml"))
	}
	
	return paths
}