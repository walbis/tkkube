package sharedconfig

import (
	"os"
	"strconv"
	"time"
)

// TimeoutConfig defines configurable timeout and retry settings
type TimeoutConfig struct {
	// HTTP Server timeouts
	HTTPReadTimeout       time.Duration `yaml:"http_read_timeout"`
	HTTPWriteTimeout      time.Duration `yaml:"http_write_timeout"`
	HTTPIdleTimeout       time.Duration `yaml:"http_idle_timeout"`
	HTTPShutdownTimeout   time.Duration `yaml:"http_shutdown_timeout"`

	// Restore operation timeouts
	RestoreOperationTimeout time.Duration `yaml:"restore_operation_timeout"`
	RestoreValidationTimeout time.Duration `yaml:"restore_validation_timeout"`
	RestoreResourceTimeout   time.Duration `yaml:"restore_resource_timeout"`

	// Component health check intervals
	HealthCheckInterval     time.Duration `yaml:"health_check_interval"`
	MonitoringInterval      time.Duration `yaml:"monitoring_interval"`
	MetricsCollectionInterval time.Duration `yaml:"metrics_collection_interval"`

	// Event handling timeouts
	EventHandlerTimeout     time.Duration `yaml:"event_handler_timeout"`
	EventBusTimeout         time.Duration `yaml:"event_bus_timeout"`

	// Backup client timeouts
	BackupClientTimeout     time.Duration `yaml:"backup_client_timeout"`
	BackupPollingInterval   time.Duration `yaml:"backup_polling_interval"`

	// GitOps operation timeouts
	GitOpsCloneTimeout      time.Duration `yaml:"gitops_clone_timeout"`
	GitOpsSyncTimeout       time.Duration `yaml:"gitops_sync_timeout"`
	GitOpsCommitTimeout     time.Duration `yaml:"gitops_commit_timeout"`

	// Security operation timeouts
	SecurityValidationTimeout time.Duration `yaml:"security_validation_timeout"`
	PermissionCheckTimeout    time.Duration `yaml:"permission_check_timeout"`
}


// DefaultTimeoutConfig returns default timeout configuration
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		// HTTP Server defaults
		HTTPReadTimeout:       30 * time.Second,
		HTTPWriteTimeout:      30 * time.Second,
		HTTPIdleTimeout:       60 * time.Second,
		HTTPShutdownTimeout:   30 * time.Second,

		// Restore operation defaults
		RestoreOperationTimeout:  30 * time.Minute,
		RestoreValidationTimeout: 5 * time.Minute,
		RestoreResourceTimeout:   2 * time.Minute,

		// Component health check defaults
		HealthCheckInterval:       30 * time.Second,
		MonitoringInterval:        15 * time.Second,
		MetricsCollectionInterval: 60 * time.Second,

		// Event handling defaults
		EventHandlerTimeout: 30 * time.Second,
		EventBusTimeout:     10 * time.Second,

		// Backup client defaults
		BackupClientTimeout:   30 * time.Second,
		BackupPollingInterval: 5 * time.Second,

		// GitOps operation defaults
		GitOpsCloneTimeout:  10 * time.Minute,
		GitOpsSyncTimeout:   15 * time.Minute,
		GitOpsCommitTimeout: 2 * time.Minute,

		// Security operation defaults
		SecurityValidationTimeout: 30 * time.Second,
		PermissionCheckTimeout:    10 * time.Second,
	}
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		// General retry defaults (legacy fields)
		MaxAttempts:     3,
		InitialDelay:    1 * time.Second,
		MaxDelay:        30 * time.Second,
		Multiplier:      2.0,

		// New general retry defaults
		MaxRetries:      3,
		BaseRetryDelay:  1 * time.Second,
		MaxRetryDelay:   30 * time.Second,
		RetryMultiplier: 2.0,

		// Specific operation retry defaults
		RestoreMaxRetries:    5,
		RestoreRetryDelay:    10 * time.Second,
		ValidationMaxRetries: 3,
		ValidationRetryDelay: 5 * time.Second,
		GitOpsMaxRetries:     3,
		GitOpsRetryDelay:     15 * time.Second,
		SecurityMaxRetries:   2,
		SecurityRetryDelay:   3 * time.Second,

		// Circuit breaker defaults
		CircuitBreakerThreshold:    5,
		CircuitBreakerTimeout:      60 * time.Second,
		CircuitBreakerRecoveryTime: 300 * time.Second,
	}
}

// LoadTimeoutConfigFromEnv loads timeout configuration from environment variables
func LoadTimeoutConfigFromEnv() TimeoutConfig {
	config := DefaultTimeoutConfig()

	// HTTP Server timeouts
	if val := getEnvDuration("HTTP_READ_TIMEOUT"); val > 0 {
		config.HTTPReadTimeout = val
	}
	if val := getEnvDuration("HTTP_WRITE_TIMEOUT"); val > 0 {
		config.HTTPWriteTimeout = val
	}
	if val := getEnvDuration("HTTP_IDLE_TIMEOUT"); val > 0 {
		config.HTTPIdleTimeout = val
	}
	if val := getEnvDuration("HTTP_SHUTDOWN_TIMEOUT"); val > 0 {
		config.HTTPShutdownTimeout = val
	}

	// Restore operation timeouts
	if val := getEnvDuration("RESTORE_OPERATION_TIMEOUT"); val > 0 {
		config.RestoreOperationTimeout = val
	}
	if val := getEnvDuration("RESTORE_VALIDATION_TIMEOUT"); val > 0 {
		config.RestoreValidationTimeout = val
	}
	if val := getEnvDuration("RESTORE_RESOURCE_TIMEOUT"); val > 0 {
		config.RestoreResourceTimeout = val
	}

	// Component health check intervals
	if val := getEnvDuration("HEALTH_CHECK_INTERVAL"); val > 0 {
		config.HealthCheckInterval = val
	}
	if val := getEnvDuration("MONITORING_INTERVAL"); val > 0 {
		config.MonitoringInterval = val
	}
	if val := getEnvDuration("METRICS_COLLECTION_INTERVAL"); val > 0 {
		config.MetricsCollectionInterval = val
	}

	// Event handling timeouts
	if val := getEnvDuration("EVENT_HANDLER_TIMEOUT"); val > 0 {
		config.EventHandlerTimeout = val
	}
	if val := getEnvDuration("EVENT_BUS_TIMEOUT"); val > 0 {
		config.EventBusTimeout = val
	}

	// Backup client timeouts
	if val := getEnvDuration("BACKUP_CLIENT_TIMEOUT"); val > 0 {
		config.BackupClientTimeout = val
	}
	if val := getEnvDuration("BACKUP_POLLING_INTERVAL"); val > 0 {
		config.BackupPollingInterval = val
	}

	// GitOps operation timeouts
	if val := getEnvDuration("GITOPS_CLONE_TIMEOUT"); val > 0 {
		config.GitOpsCloneTimeout = val
	}
	if val := getEnvDuration("GITOPS_SYNC_TIMEOUT"); val > 0 {
		config.GitOpsSyncTimeout = val
	}
	if val := getEnvDuration("GITOPS_COMMIT_TIMEOUT"); val > 0 {
		config.GitOpsCommitTimeout = val
	}

	// Security operation timeouts
	if val := getEnvDuration("SECURITY_VALIDATION_TIMEOUT"); val > 0 {
		config.SecurityValidationTimeout = val
	}
	if val := getEnvDuration("PERMISSION_CHECK_TIMEOUT"); val > 0 {
		config.PermissionCheckTimeout = val
	}

	return config
}

// LoadRetryConfigFromEnv loads retry configuration from environment variables
func LoadRetryConfigFromEnv() RetryConfig {
	config := DefaultRetryConfig()

	// General retry settings (legacy compatibility)
	if val := getEnvInt("MAX_ATTEMPTS"); val > 0 {
		config.MaxAttempts = val
	}
	if val := getEnvDuration("INITIAL_DELAY"); val > 0 {
		config.InitialDelay = val
	}
	if val := getEnvDuration("MAX_DELAY"); val > 0 {
		config.MaxDelay = val
	}
	if val := getEnvFloat("MULTIPLIER"); val > 0 {
		config.Multiplier = val
	}

	// New general retry settings
	if val := getEnvInt("MAX_RETRIES"); val > 0 {
		config.MaxRetries = val
		config.MaxAttempts = val // sync legacy field
	}
	if val := getEnvDuration("BASE_RETRY_DELAY"); val > 0 {
		config.BaseRetryDelay = val
		config.InitialDelay = val // sync legacy field
	}
	if val := getEnvDuration("MAX_RETRY_DELAY"); val > 0 {
		config.MaxRetryDelay = val
		config.MaxDelay = val // sync legacy field
	}
	if val := getEnvFloat("RETRY_MULTIPLIER"); val > 0 {
		config.RetryMultiplier = val
		config.Multiplier = val // sync legacy field
	}

	// Specific operation retries
	if val := getEnvInt("RESTORE_MAX_RETRIES"); val > 0 {
		config.RestoreMaxRetries = val
	}
	if val := getEnvDuration("RESTORE_RETRY_DELAY"); val > 0 {
		config.RestoreRetryDelay = val
	}
	if val := getEnvInt("VALIDATION_MAX_RETRIES"); val > 0 {
		config.ValidationMaxRetries = val
	}
	if val := getEnvDuration("VALIDATION_RETRY_DELAY"); val > 0 {
		config.ValidationRetryDelay = val
	}
	if val := getEnvInt("GITOPS_MAX_RETRIES"); val > 0 {
		config.GitOpsMaxRetries = val
	}
	if val := getEnvDuration("GITOPS_RETRY_DELAY"); val > 0 {
		config.GitOpsRetryDelay = val
	}
	if val := getEnvInt("SECURITY_MAX_RETRIES"); val > 0 {
		config.SecurityMaxRetries = val
	}
	if val := getEnvDuration("SECURITY_RETRY_DELAY"); val > 0 {
		config.SecurityRetryDelay = val
	}

	// Circuit breaker settings
	if val := getEnvInt("CIRCUIT_BREAKER_THRESHOLD"); val > 0 {
		config.CircuitBreakerThreshold = val
	}
	if val := getEnvDuration("CIRCUIT_BREAKER_TIMEOUT"); val > 0 {
		config.CircuitBreakerTimeout = val
	}
	if val := getEnvDuration("CIRCUIT_BREAKER_RECOVERY_TIME"); val > 0 {
		config.CircuitBreakerRecoveryTime = val
	}

	return config
}

// Helper functions for environment variable parsing

func getEnvDuration(key string) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return 0
	}
	
	duration, err := time.ParseDuration(val)
	if err != nil {
		return 0
	}
	
	return duration
}

func getEnvInt(key string) int {
	val := os.Getenv(key)
	if val == "" {
		return 0
	}
	
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	
	return intVal
}

func getEnvFloat(key string) float64 {
	val := os.Getenv(key)
	if val == "" {
		return 0
	}
	
	floatVal, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	
	return floatVal
}

// GetTimeoutFromEnv is a convenience function to get a single timeout value
func GetTimeoutFromEnv(key string, defaultValue time.Duration) time.Duration {
	if val := getEnvDuration(key); val > 0 {
		return val
	}
	return defaultValue
}

// GetRetryCountFromEnv is a convenience function to get retry count
func GetRetryCountFromEnv(key string, defaultValue int) int {
	if val := getEnvInt(key); val > 0 {
		return val
	}
	return defaultValue
}