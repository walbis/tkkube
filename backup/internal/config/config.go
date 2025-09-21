package config

import (
	"os"
	"strconv"
	"strings"
	"time"
	
	"shared-config/errors"
)

// Config holds the main backup configuration
type Config struct {
	ClusterDomain     string
	ClusterName       string
	MinIOEndpoint     string
	MinIOAccessKey    string
	MinIOSecretKey    string
	MinIOBucket       string
	MinIOUseSSL       bool
	BatchSize         int
	RetryAttempts     int
	RetryDelay        time.Duration
	// Cleanup configuration
	EnableCleanup     bool
	RetentionDays     int
	CleanupOnStartup  bool
	// Advanced bucket management
	AutoCreateBucket  bool
	FallbackBuckets   []string
	BucketRetryAttempts int
	BucketRetryDelay    time.Duration
}

// BackupConfig holds the backup-specific configuration
type BackupConfig struct {
	FilteringMode           string
	IncludeResources        []string
	ExcludeResources        []string
	IncludeNamespaces       []string
	ExcludeNamespaces       []string
	IncludeCRDs             []string
	LabelSelector           string
	AnnotationSelector      string
	MaxResourceSize         string
	FollowOwnerReferences   bool
	IncludeManagedFields    bool
	IncludeStatus           bool
	ValidateYAML            bool
	SkipInvalidResources    bool
	OpenShiftMode           string
	IncludeOpenShiftRes     bool
	EnableCleanup           bool
	CleanupOnStartup        bool
	RetentionDays           int
}

// LoadConfig loads the main configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		ClusterDomain:     getConfigValue("CLUSTER_DOMAIN"),
		ClusterName:       getConfigValue("CLUSTER_NAME"),
		MinIOEndpoint:     getConfigValueWithWarning("MINIO_ENDPOINT", "", "MinIO connection"),
		MinIOAccessKey:    getConfigValueWithWarning("MINIO_ACCESS_KEY", "", "MinIO authentication"),
		MinIOSecretKey:    getConfigValueWithWarning("MINIO_SECRET_KEY", "", "MinIO authentication"),
		MinIOBucket:       getConfigValueWithWarning("MINIO_BUCKET", "cluster-backups", "MinIO storage"),
		MinIOUseSSL:       getConfigValueWithWarning("MINIO_USE_SSL", "true", "MinIO security") == "true",
		BatchSize:         50,
		RetryAttempts:     3,
		RetryDelay:        5 * time.Second,
		EnableCleanup:     getConfigValueWithWarning("ENABLE_CLEANUP", "true", "cleanup policy") == "true",
		RetentionDays:     7,
		CleanupOnStartup:  getConfigValueWithWarning("CLEANUP_ON_STARTUP", "false", "cleanup timing") == "true",
		AutoCreateBucket:  getConfigValueWithWarning("AUTO_CREATE_BUCKET", "false", "bucket management") == "true",
		BucketRetryAttempts: 3,
		BucketRetryDelay:    2 * time.Second,
	}

	// Parse fallback buckets
	if fallbackStr := getConfigValueWithWarning("FALLBACK_BUCKETS", "", "bucket fallback"); fallbackStr != "" {
		config.FallbackBuckets = parseCommaSeparated(fallbackStr)
	}

	// Parse batch size with validation
	if batchStr := getConfigValueWithWarning("BATCH_SIZE", "50", "performance tuning"); batchStr != "" {
		if batch, err := strconv.Atoi(batchStr); err == nil {
			if batch > 0 && batch <= 1000 {
				config.BatchSize = batch
			}
		}
	}

	// Parse retry attempts with validation
	if retryStr := getConfigValueWithWarning("RETRY_ATTEMPTS", "3", "retry policy"); retryStr != "" {
		if retry, err := strconv.Atoi(retryStr); err == nil {
			if retry >= 0 && retry <= 10 {
				config.RetryAttempts = retry
			}
		}
	}

	// Parse retry delay with validation
	if delayStr := getConfigValueWithWarning("RETRY_DELAY", "5s", "retry timing"); delayStr != "" {
		if delay, err := time.ParseDuration(delayStr); err == nil {
			if delay >= time.Second && delay <= 5*time.Minute {
				config.RetryDelay = delay
			}
		}
	}

	// Parse retention days
	if retentionStr := getConfigValueWithWarning("RETENTION_DAYS", "7", "cleanup retention"); retentionStr != "" {
		if retention, err := strconv.Atoi(retentionStr); err == nil {
			if retention > 0 && retention <= 365 {
				config.RetentionDays = retention
			}
		}
	}

	// Validate required fields
	if err := config.Validate(); err != nil {
		return nil, errors.NewConfigurationError("config", "load", "configuration validation failed", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	validator := errors.NewValidationHelper("config")
	multiErr := errors.NewMultiError("config", "validation")
	
	// Required field validations
	if err := validator.Required("MINIO_ENDPOINT", c.MinIOEndpoint); err != nil {
		multiErr.Add(err)
	}
	if err := validator.Required("MINIO_ACCESS_KEY", c.MinIOAccessKey); err != nil {
		multiErr.Add(err)
	}
	if err := validator.Required("MINIO_SECRET_KEY", c.MinIOSecretKey); err != nil {
		multiErr.Add(err)
	}
	
	// Range validations
	if err := validator.Range("batch_size", c.BatchSize, 1, 1000); err != nil {
		multiErr.Add(err)
	}
	if err := validator.Range("retry_attempts", c.RetryAttempts, 0, 10); err != nil {
		multiErr.Add(err)
	}
	if err := validator.Range("retention_days", c.RetentionDays, 1, 365); err != nil {
		multiErr.Add(err)
	}
	
	return multiErr.ToError()
}

// LoadBackupConfig loads backup-specific configuration
func LoadBackupConfig() (*BackupConfig, error) {
	config := &BackupConfig{
		FilteringMode:           "whitelist",
		IncludeResources:        parseCommaSeparated(getConfigValueWithWarning("INCLUDE_RESOURCES", "", "resource inclusion")),
		ExcludeResources:        parseCommaSeparated(getConfigValueWithWarning("EXCLUDE_RESOURCES", "", "resource exclusion")),
		IncludeNamespaces:       parseCommaSeparated(getConfigValueWithWarning("INCLUDE_NAMESPACES", "", "namespace inclusion")),
		ExcludeNamespaces:       parseCommaSeparated(getConfigValueWithWarning("EXCLUDE_NAMESPACES", "", "namespace exclusion")),
		LabelSelector:           getConfigValueWithWarning("LABEL_SELECTOR", "", "label filtering"),
		AnnotationSelector:      getConfigValueWithWarning("ANNOTATION_SELECTOR", "", "annotation filtering"),
		MaxResourceSize:         getConfigValueWithWarning("MAX_RESOURCE_SIZE", "10Mi", "resource size limit"),
		FollowOwnerReferences:   getConfigValueWithWarning("FOLLOW_OWNER_REFERENCES", "false", "owner reference tracking") == "true",
		IncludeManagedFields:    getConfigValueWithWarning("INCLUDE_MANAGED_FIELDS", "false", "managed fields") == "true",
		IncludeStatus:           getConfigValueWithWarning("INCLUDE_STATUS", "false", "resource status") == "true",
		OpenShiftMode:           getConfigValueWithWarning("OPENSHIFT_MODE", "auto-detect", "OpenShift detection"),
		IncludeOpenShiftRes:     getConfigValueWithWarning("INCLUDE_OPENSHIFT_RESOURCES", "true", "OpenShift resources") == "true",
		ValidateYAML:            getConfigValueWithWarning("VALIDATE_YAML", "true", "YAML validation") == "true",
		SkipInvalidResources:    getConfigValueWithWarning("SKIP_INVALID_RESOURCES", "true", "invalid resource handling") == "true",
		EnableCleanup:           getConfigValueWithWarning("ENABLE_CLEANUP", "true", "cleanup policy") == "true",
		CleanupOnStartup:        getConfigValueWithWarning("CLEANUP_ON_STARTUP", "false", "startup cleanup") == "true",
		RetentionDays:           7,
	}

	// Parse retention days
	if retentionStr := getConfigValueWithWarning("RETENTION_DAYS", "7", "cleanup retention"); retentionStr != "" {
		if retention, err := strconv.Atoi(retentionStr); err == nil && retention > 0 && retention <= 365 {
			config.RetentionDays = retention
		}
	}

	return config, nil
}

// GetSecretValue retrieves a value from environment variables with fallback
func getSecretValue(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetConfigValue retrieves a configuration value from environment
func getConfigValue(key string) string {
	return os.Getenv(key)
}

// GetConfigValueWithWarning retrieves config value with warning for missing values
func getConfigValueWithWarning(key, defaultValue, configType string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ParseCommaSeparated parses comma-separated string into slice
func parseCommaSeparated(input string) []string {
	if input == "" {
		return []string{}
	}
	
	parts := strings.Split(input, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}