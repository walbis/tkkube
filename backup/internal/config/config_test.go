package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		validate    func(t *testing.T, config *Config)
	}{
		{
			name: "valid_configuration",
			envVars: map[string]string{
				"MINIO_ENDPOINT":    "localhost:9000",
				"MINIO_ACCESS_KEY":  "testkey",
				"MINIO_SECRET_KEY":  "testsecret",
				"MINIO_BUCKET":      "test-bucket",
				"MINIO_USE_SSL":     "false",
				"BATCH_SIZE":        "100",
				"RETRY_ATTEMPTS":    "5",
				"RETRY_DELAY":       "10s",
				"RETENTION_DAYS":    "14",
			},
			expectError: false,
			validate: func(t *testing.T, config *Config) {
				assert.Equal(t, "localhost:9000", config.MinIOEndpoint)
				assert.Equal(t, "testkey", config.MinIOAccessKey)
				assert.Equal(t, "testsecret", config.MinIOSecretKey)
				assert.Equal(t, "test-bucket", config.MinIOBucket)
				assert.False(t, config.MinIOUseSSL)
				assert.Equal(t, 100, config.BatchSize)
				assert.Equal(t, 5, config.RetryAttempts)
				assert.Equal(t, 10*time.Second, config.RetryDelay)
				assert.Equal(t, 14, config.RetentionDays)
			},
		},
		{
			name: "missing_required_fields",
			envVars: map[string]string{
				"MINIO_ENDPOINT": "localhost:9000",
				// Missing access key and secret key
			},
			expectError: true,
		},
		{
			name: "invalid_batch_size",
			envVars: map[string]string{
				"MINIO_ENDPOINT":   "localhost:9000",
				"MINIO_ACCESS_KEY": "testkey",
				"MINIO_SECRET_KEY": "testsecret",
				"BATCH_SIZE":       "1500", // Too large
			},
			expectError: true,
		},
		{
			name: "invalid_retry_attempts",
			envVars: map[string]string{
				"MINIO_ENDPOINT":   "localhost:9000",
				"MINIO_ACCESS_KEY": "testkey",
				"MINIO_SECRET_KEY": "testsecret",
				"RETRY_ATTEMPTS":   "15", // Too large
			},
			expectError: true,
		},
		{
			name: "default_values",
			envVars: map[string]string{
				"MINIO_ENDPOINT":   "localhost:9000",
				"MINIO_ACCESS_KEY": "testkey",
				"MINIO_SECRET_KEY": "testsecret",
			},
			expectError: false,
			validate: func(t *testing.T, config *Config) {
				assert.Equal(t, "cluster-backups", config.MinIOBucket)
				assert.True(t, config.MinIOUseSSL)
				assert.Equal(t, 50, config.BatchSize)
				assert.Equal(t, 3, config.RetryAttempts)
				assert.Equal(t, 5*time.Second, config.RetryDelay)
				assert.Equal(t, 7, config.RetentionDays)
				assert.True(t, config.EnableCleanup)
				assert.False(t, config.CleanupOnStartup)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearEnv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Load configuration
			config, err := LoadConfig()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				if tt.validate != nil {
					tt.validate(t, config)
				}
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid_config",
			config: &Config{
				MinIOEndpoint:   "localhost:9000",
				MinIOAccessKey:  "testkey",
				MinIOSecretKey:  "testsecret",
				BatchSize:       50,
				RetryAttempts:   3,
				RetentionDays:   7,
			},
			wantErr: false,
		},
		{
			name: "missing_endpoint",
			config: &Config{
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				BatchSize:      50,
				RetryAttempts:  3,
				RetentionDays:  7,
			},
			wantErr: true,
			errMsg:  "MINIO_ENDPOINT is required",
		},
		{
			name: "missing_access_key",
			config: &Config{
				MinIOEndpoint:  "localhost:9000",
				MinIOSecretKey: "testsecret",
				BatchSize:      50,
				RetryAttempts:  3,
				RetentionDays:  7,
			},
			wantErr: true,
			errMsg:  "MINIO_ACCESS_KEY is required",
		},
		{
			name: "invalid_batch_size_zero",
			config: &Config{
				MinIOEndpoint:  "localhost:9000",
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				BatchSize:      0,
				RetryAttempts:  3,
				RetentionDays:  7,
			},
			wantErr: true,
			errMsg:  "batch size must be between 1 and 1000",
		},
		{
			name: "invalid_batch_size_too_large",
			config: &Config{
				MinIOEndpoint:  "localhost:9000",
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				BatchSize:      1500,
				RetryAttempts:  3,
				RetentionDays:  7,
			},
			wantErr: true,
			errMsg:  "batch size must be between 1 and 1000",
		},
		{
			name: "invalid_retry_attempts",
			config: &Config{
				MinIOEndpoint:  "localhost:9000",
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				BatchSize:      50,
				RetryAttempts:  15,
				RetentionDays:  7,
			},
			wantErr: true,
			errMsg:  "retry attempts must be between 0 and 10",
		},
		{
			name: "invalid_retention_days",
			config: &Config{
				MinIOEndpoint:  "localhost:9000",
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				BatchSize:      50,
				RetryAttempts:  3,
				RetentionDays:  400,
			},
			wantErr: true,
			errMsg:  "retention days must be between 1 and 365",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadBackupConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		validate func(t *testing.T, config *BackupConfig)
	}{
		{
			name: "default_configuration",
			envVars: map[string]string{},
			validate: func(t *testing.T, config *BackupConfig) {
				assert.Equal(t, "whitelist", config.FilteringMode)
				assert.Equal(t, "auto-detect", config.OpenShiftMode)
				assert.True(t, config.ValidateYAML)
				assert.True(t, config.SkipInvalidResources)
				assert.Equal(t, 7, config.RetentionDays)
			},
		},
		{
			name: "custom_configuration",
			envVars: map[string]string{
				"INCLUDE_RESOURCES":    "deployments,services,configmaps",
				"EXCLUDE_RESOURCES":    "events,nodes",
				"INCLUDE_NAMESPACES":   "default,kube-system",
				"LABEL_SELECTOR":       "app=test",
				"ANNOTATION_SELECTOR":  "backup=enabled",
				"MAX_RESOURCE_SIZE":    "20Mi",
				"RETENTION_DAYS":       "30",
				"OPENSHIFT_MODE":       "enabled",
			},
			validate: func(t *testing.T, config *BackupConfig) {
				assert.Equal(t, []string{"deployments", "services", "configmaps"}, config.IncludeResources)
				assert.Equal(t, []string{"events", "nodes"}, config.ExcludeResources)
				assert.Equal(t, []string{"default", "kube-system"}, config.IncludeNamespaces)
				assert.Equal(t, "app=test", config.LabelSelector)
				assert.Equal(t, "backup=enabled", config.AnnotationSelector)
				assert.Equal(t, "20Mi", config.MaxResourceSize)
				assert.Equal(t, 30, config.RetentionDays)
				assert.Equal(t, "enabled", config.OpenShiftMode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearEnv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Load backup configuration
			config, err := LoadBackupConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty_string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single_item",
			input:    "item1",
			expected: []string{"item1"},
		},
		{
			name:     "multiple_items",
			input:    "item1,item2,item3",
			expected: []string{"item1", "item2", "item3"},
		},
		{
			name:     "items_with_spaces",
			input:    "item1, item2 , item3",
			expected: []string{"item1", "item2", "item3"},
		},
		{
			name:     "empty_items_filtered",
			input:    "item1,,item2,",
			expected: []string{"item1", "item2"},
		},
		{
			name:     "only_spaces_and_commas",
			input:    " , , ",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommaSeparated(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSecretValue(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "env_value_exists",
			key:          "TEST_KEY",
			defaultValue: "default",
			envValue:     "env_value",
			expected:     "env_value",
		},
		{
			name:         "env_value_missing",
			key:          "MISSING_KEY",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Unsetenv(tt.key)

			// Set environment variable if provided
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			result := getSecretValue(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)

			// Cleanup
			os.Unsetenv(tt.key)
		})
	}
}

// clearEnv clears all environment variables that might affect tests
func clearEnv() {
	envVars := []string{
		"CLUSTER_DOMAIN", "CLUSTER_NAME", "MINIO_ENDPOINT", "MINIO_ACCESS_KEY",
		"MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_SSL", "BATCH_SIZE",
		"RETRY_ATTEMPTS", "RETRY_DELAY", "ENABLE_CLEANUP", "RETENTION_DAYS",
		"CLEANUP_ON_STARTUP", "AUTO_CREATE_BUCKET", "INCLUDE_RESOURCES",
		"EXCLUDE_RESOURCES", "INCLUDE_NAMESPACES", "EXCLUDE_NAMESPACES",
		"LABEL_SELECTOR", "ANNOTATION_SELECTOR", "MAX_RESOURCE_SIZE",
		"FOLLOW_OWNER_REFERENCES", "INCLUDE_MANAGED_FIELDS", "INCLUDE_STATUS",
		"OPENSHIFT_MODE", "INCLUDE_OPENSHIFT_RESOURCES", "VALIDATE_YAML",
		"SKIP_INVALID_RESOURCES",
	}

	for _, env := range envVars {
		os.Unsetenv(env)
	}
}

// Benchmark tests for performance validation
func BenchmarkLoadConfig(b *testing.B) {
	// Setup environment
	envVars := map[string]string{
		"MINIO_ENDPOINT":   "localhost:9000",
		"MINIO_ACCESS_KEY": "testkey",
		"MINIO_SECRET_KEY": "testsecret",
		"MINIO_BUCKET":     "test-bucket",
	}

	for key, value := range envVars {
		os.Setenv(key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadConfig()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseCommaSeparated(b *testing.B) {
	input := "item1,item2,item3,item4,item5,item6,item7,item8,item9,item10"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseCommaSeparated(input)
	}
}