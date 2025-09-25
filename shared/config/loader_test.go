package sharedconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoader_Load(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	configContent := `
schema_version: "1.0.0"
description: "Test configuration"

storage:
  type: "minio"
  endpoint: "localhost:9000"
  access_key: "testkey"
  secret_key: "testsecret"
  bucket: "test-bucket"
  use_ssl: false
  region: "us-east-1"
  auto_create_bucket: false
  connection:
    timeout: 30
    max_retries: 3
    retry_delay: 5s

cluster:
  name: "test-cluster"
  domain: "cluster.local"
  type: "kubernetes"

backup:
  behavior:
    batch_size: 25
    validate_yaml: true
  cleanup:
    enabled: true
    retention_days: 14

gitops:
  repository:
    url: "https://github.com/test/repo.git"
    branch: "main"
    auth:
      method: "ssh"

observability:
  logging:
    level: "debug"
    format: "json"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test loading configuration
	loader := NewConfigLoaderForTesting(configPath)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate loaded configuration
	if config.SchemaVersion != "1.0.0" {
		t.Errorf("Expected schema version '1.0.0', got '%s'", config.SchemaVersion)
	}

	if config.Storage.Endpoint != "localhost:9000" {
		t.Errorf("Expected endpoint 'localhost:9000', got '%s'", config.Storage.Endpoint)
	}

	if config.Storage.AccessKey != "testkey" {
		t.Errorf("Expected access key 'testkey', got '%s'", config.Storage.AccessKey)
	}

	if config.Cluster.Name != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got '%s'", config.Cluster.Name)
	}

	if config.Backup.Behavior.BatchSize != 25 {
		t.Errorf("Expected batch size 25, got %d", config.Backup.Behavior.BatchSize)
	}

	if config.Backup.Cleanup.RetentionDays != 14 {
		t.Errorf("Expected retention days 14, got %d", config.Backup.Cleanup.RetentionDays)
	}
}

func TestConfigLoader_EnvironmentOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("MINIO_ENDPOINT", "override.example.com:9000")
	os.Setenv("MINIO_ACCESS_KEY", "override-key")
	os.Setenv("CLUSTER_NAME", "override-cluster")
	os.Setenv("BATCH_SIZE", "100")
	defer func() {
		os.Unsetenv("MINIO_ENDPOINT")
		os.Unsetenv("MINIO_ACCESS_KEY")
		os.Unsetenv("CLUSTER_NAME")
		os.Unsetenv("BATCH_SIZE")
	}()

	// Create minimal config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	configContent := `
storage:
  endpoint: "localhost:9000"
  access_key: "testkey"
  secret_key: "testsecret"
  bucket: "test-bucket"

cluster:
  name: "test-cluster"

backup:
  behavior:
    batch_size: 25
  cleanup:
    retention_days: 7
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	loader := NewConfigLoaderForTesting(configPath)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment overrides
	if config.Storage.Endpoint != "override.example.com:9000" {
		t.Errorf("Expected overridden endpoint, got '%s'", config.Storage.Endpoint)
	}

	if config.Storage.AccessKey != "override-key" {
		t.Errorf("Expected overridden access key, got '%s'", config.Storage.AccessKey)
	}

	if config.Cluster.Name != "override-cluster" {
		t.Errorf("Expected overridden cluster name, got '%s'", config.Cluster.Name)
	}

	if config.Backup.Behavior.BatchSize != 100 {
		t.Errorf("Expected overridden batch size 100, got %d", config.Backup.Behavior.BatchSize)
	}
}

func TestConfigLoader_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		shouldError bool
		errorMsg    string
	}{
		{
			name: "missing endpoint",
			config: `
storage:
  access_key: "testkey"
  secret_key: "testsecret"
  bucket: "test-bucket"
`,
			shouldError: true,
			errorMsg:    "storage endpoint is required",
		},
		{
			name: "missing access key",
			config: `
storage:
  endpoint: "localhost:9000"
  secret_key: "testsecret"
  bucket: "test-bucket"
`,
			shouldError: true,
			errorMsg:    "storage access key is required",
		},
		{
			name: "invalid batch size",
			config: `
storage:
  endpoint: "localhost:9000"
  access_key: "testkey"
  secret_key: "testsecret"
  bucket: "test-bucket"
backup:
  behavior:
    batch_size: 2000
`,
			shouldError: true,
			errorMsg:    "batch size must be between 1 and 1000",
		},
		{
			name: "invalid retention days",
			config: `
storage:
  endpoint: "localhost:9000"
  access_key: "testkey"
  secret_key: "testsecret"
  bucket: "test-bucket"
backup:
  cleanup:
    retention_days: 500
`,
			shouldError: true,
			errorMsg:    "retention days must be between 1 and 365",
		},
		{
			name: "valid config",
			config: `
storage:
  endpoint: "localhost:9000"
  access_key: "testkey"
  secret_key: "testsecret"
  bucket: "test-bucket"
backup:
  behavior:
    batch_size: 50
  cleanup:
    retention_days: 7
`,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "test-config.yaml")

			err := os.WriteFile(configPath, []byte(tt.config), 0644)
			if err != nil {
				t.Fatalf("Failed to write test config: %v", err)
			}

			loader := NewConfigLoader(configPath)
			_, err = loader.Load()

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !containsError(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestSharedConfig_GetBackupToolConfig(t *testing.T) {
	config := &SharedConfig{
		Storage: StorageConfig{
			Endpoint:  "localhost:9000",
			AccessKey: "testkey",
			SecretKey: "testsecret",
			Bucket:    "test-bucket",
			UseSSL:    false,
		},
		Cluster: SingleClusterConfig{
			Name:   "test-cluster",
			Domain: "cluster.local",
		},
		Backup: BackupConfig{
			Behavior: BehaviorConfig{
				BatchSize: 25,
			},
			Cleanup: CleanupConfig{
				Enabled:       true,
				RetentionDays: 14,
			},
		},
	}

	backupConfig := config.GetBackupToolConfig()

	// Verify conversion
	if backupConfig["ClusterName"] != "test-cluster" {
		t.Errorf("Expected ClusterName 'test-cluster', got '%v'", backupConfig["ClusterName"])
	}

	if backupConfig["MinIOEndpoint"] != "localhost:9000" {
		t.Errorf("Expected MinIOEndpoint 'localhost:9000', got '%v'", backupConfig["MinIOEndpoint"])
	}

	if backupConfig["BatchSize"] != 25 {
		t.Errorf("Expected BatchSize 25, got %v", backupConfig["BatchSize"])
	}

	if backupConfig["RetentionDays"] != 14 {
		t.Errorf("Expected RetentionDays 14, got %v", backupConfig["RetentionDays"])
	}
}

func TestConfigLoader_SaveToFile(t *testing.T) {
	config := &SharedConfig{
		SchemaVersion: "1.0.0",
		Description:   "Test configuration for saving",
		Storage: StorageConfig{
			Type:      "minio",
			Endpoint:  "localhost:9000",
			AccessKey: "testkey",
			SecretKey: "testsecret",
			Bucket:    "test-bucket",
			UseSSL:    false,
		},
		Cluster: SingleClusterConfig{
			Name:   "test-cluster",
			Domain: "cluster.local",
		},
		Backup: BackupConfig{
			Behavior: BehaviorConfig{
				BatchSize: 50,
			},
			Cleanup: CleanupConfig{
				RetentionDays: 7,
			},
		},
	}

	tempDir := t.TempDir()
	savePath := filepath.Join(tempDir, "saved-config.yaml")

	err := config.SaveToFile(savePath)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify file was created and can be loaded back
	loader := NewConfigLoaderForTesting(savePath)
	loadedConfig, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loadedConfig.SchemaVersion != config.SchemaVersion {
		t.Errorf("Schema version mismatch after save/load")
	}

	if loadedConfig.Storage.Endpoint != config.Storage.Endpoint {
		t.Errorf("Storage endpoint mismatch after save/load")
	}
}

func TestDefaultConfigPaths(t *testing.T) {
	paths := DefaultConfigPaths()

	if len(paths) == 0 {
		t.Errorf("Expected at least one default config path")
	}

	// Verify expected paths are included
	expectedPaths := []string{
		"./shared-config.yaml",
		"./config/shared-config.yaml",
		"/etc/backup-gitops/config.yaml",
	}

	for _, expected := range expectedPaths {
		found := false
		for _, path := range paths {
			if path == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected path '%s' not found in default paths", expected)
		}
	}
}

func containsError(actual, expected string) bool {
	if len(expected) == 0 {
		return true
	}
	// Simple substring check
	for i := 0; i <= len(actual)-len(expected); i++ {
		if actual[i:i+len(expected)] == expected {
			return true
		}
	}
	return false
}