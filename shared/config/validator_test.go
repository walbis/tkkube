package sharedconfig

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidator_ValidateSchemaVersion(t *testing.T) {
	tests := []struct {
		name          string
		schemaVersion string
		expectError   bool
	}{
		{"Valid semantic version", "1.0.0", false},
		{"Valid with v prefix", "v1.2.3", false},
		{"Valid with pre-release", "1.0.0-alpha.1", false},
		{"Valid with build metadata", "1.0.0+build.1", false},
		{"Empty version", "", true},
		{"Invalid format", "1.0", true},
		{"Invalid format", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &SharedConfig{SchemaVersion: tt.schemaVersion}
			validator := NewConfigValidator(config)
			validator.validateSchemaVersion()

			hasError := len(validator.result.Errors) > 0
			if hasError != tt.expectError {
				t.Errorf("Expected error: %v, got error: %v", tt.expectError, hasError)
			}
		})
	}
}

func TestConfigValidator_ValidateStorage(t *testing.T) {
	tests := []struct {
		name        string
		storage     StorageConfig
		expectError bool
		errorCount  int
	}{
		{
			name: "Valid MinIO configuration",
			storage: StorageConfig{
				Type:      "minio",
				Endpoint:  "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Bucket:    "test-bucket",
				UseSSL:    false,
				Region:    "us-east-1",
				Connection: ConnectionConfig{
					Timeout:    30,
					MaxRetries: 3,
					RetryDelay: 5 * time.Second,
				},
			},
			expectError: false,
			errorCount:  0,
		},
		{
			name: "Valid S3 configuration",
			storage: StorageConfig{
				Type:      "s3",
				Endpoint:  "https://s3.amazonaws.com",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				Bucket:    "my-bucket",
				UseSSL:    true,
				Region:    "us-west-2",
				Connection: ConnectionConfig{
					Timeout:    30,
					MaxRetries: 3,
					RetryDelay: 5 * time.Second,
				},
			},
			expectError: false,
			errorCount:  0,
		},
		{
			name: "Missing required fields",
			storage: StorageConfig{
				Type: "minio",
			},
			expectError: true,
			errorCount:  5, // endpoint, access_key, secret_key, bucket, connection issues
		},
		{
			name: "Invalid storage type",
			storage: StorageConfig{
				Type:      "invalid",
				Endpoint:  "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Bucket:    "test-bucket",
			},
			expectError: true,
			errorCount:  2, // type + region for S3
		},
		{
			name: "Invalid bucket name",
			storage: StorageConfig{
				Type:      "minio",
				Endpoint:  "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				Bucket:    "INVALID_BUCKET_NAME",
			},
			expectError: true,
			errorCount:  2, // bucket + region
		},
		{
			name: "S3 without region",
			storage: StorageConfig{
				Type:      "s3",
				Endpoint:  "https://s3.amazonaws.com",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				Bucket:    "my-bucket",
				Region:    "",
			},
			expectError: true,
			errorCount:  2, // region + endpoint format
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &SharedConfig{Storage: tt.storage}
			validator := NewConfigValidator(config)
			validator.validateStorage()

			hasError := len(validator.result.Errors) > 0
			if hasError != tt.expectError {
				t.Errorf("Expected error: %v, got error: %v", tt.expectError, hasError)
			}

			if len(validator.result.Errors) != tt.errorCount {
				t.Errorf("Expected %d errors, got %d", tt.errorCount, len(validator.result.Errors))
			}
		})
	}
}

func TestConfigValidator_ValidateCluster(t *testing.T) {
	tests := []struct {
		name        string
		cluster     ClusterConfig
		expectError bool
	}{
		{
			name: "Valid Kubernetes cluster",
			cluster: ClusterConfig{
				Name:   "test-cluster",
				Domain: "cluster.local",
				Type:   "kubernetes",
			},
			expectError: false,
		},
		{
			name: "Valid OpenShift cluster",
			cluster: ClusterConfig{
				Name:   "openshift-cluster",
				Domain: "cluster.local",
				Type:   "openshift",
				OpenShift: OpenShiftConfig{
					Mode:             "enabled",
					IncludeResources: true,
				},
			},
			expectError: false,
		},
		{
			name: "Missing cluster name",
			cluster: ClusterConfig{
				Domain: "cluster.local",
				Type:   "kubernetes",
			},
			expectError: true,
		},
		{
			name: "Invalid cluster type",
			cluster: ClusterConfig{
				Name:   "test-cluster",
				Domain: "cluster.local",
				Type:   "invalid",
			},
			expectError: true,
		},
		{
			name: "Invalid domain",
			cluster: ClusterConfig{
				Name:   "test-cluster",
				Domain: "invalid..domain",
				Type:   "kubernetes",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &SharedConfig{Cluster: tt.cluster}
			validator := NewConfigValidator(config)
			validator.validateCluster()

			hasError := len(validator.result.Errors) > 0
			if hasError != tt.expectError {
				t.Errorf("Expected error: %v, got error: %v", tt.expectError, hasError)
			}
		})
	}
}

func TestConfigValidator_ValidateGitOps(t *testing.T) {
	tests := []struct {
		name        string
		gitops      GitOpsConfig
		expectError bool
	}{
		{
			name: "Valid SSH configuration",
			gitops: GitOpsConfig{
				Repository: RepositoryConfig{
					URL:    "git@github.com:user/repo.git",
					Branch: "main",
					Auth: AuthConfig{
						Method: "ssh",
						SSH: SSHAuthConfig{
							PrivateKeyPath: "/home/user/.ssh/id_rsa",
						},
					},
				},
				Structure: StructureConfig{
					ArgoCD: ArgoCDConfig{
						Enabled:   true,
						Namespace: "argocd",
						Project:   "default",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid HTTPS configuration",
			gitops: GitOpsConfig{
				Repository: RepositoryConfig{
					URL:    "https://github.com/user/repo.git",
					Branch: "develop",
					Auth: AuthConfig{
						Method: "pat",
						PAT: PATAuthConfig{
							Token:    "ghp_1234567890abcdef",
							Username: "user",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Missing repository URL",
			gitops: GitOpsConfig{
				Repository: RepositoryConfig{
					Branch: "main",
				},
			},
			expectError: true,
		},
		{
			name: "Invalid Git URL",
			gitops: GitOpsConfig{
				Repository: RepositoryConfig{
					URL:    "invalid-url",
					Branch: "main",
				},
			},
			expectError: true,
		},
		{
			name: "SSH auth without private key",
			gitops: GitOpsConfig{
				Repository: RepositoryConfig{
					URL:    "git@github.com:user/repo.git",
					Branch: "main",
					Auth: AuthConfig{
						Method: "ssh",
						SSH:    SSHAuthConfig{},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &SharedConfig{GitOps: tt.gitops}
			validator := NewConfigValidator(config)
			validator.validateGitOps()

			hasError := len(validator.result.Errors) > 0
			if hasError != tt.expectError {
				t.Errorf("Expected error: %v, got error: %v", tt.expectError, hasError)
			}
		})
	}
}

func TestConfigValidator_CrossFieldValidation(t *testing.T) {
	tests := []struct {
		name           string
		config         *SharedConfig
		expectWarnings int
		expectErrors   int
	}{
		{
			name: "ArgoCD enabled without Git repository",
			config: &SharedConfig{
				GitOps: GitOpsConfig{
					Structure: StructureConfig{
						ArgoCD: ArgoCDConfig{
							Enabled: true,
						},
					},
				},
			},
			expectErrors: 1,
		},
		{
			name: "Auto-create bucket with S3",
			config: &SharedConfig{
				Storage: StorageConfig{
					Type:             "s3",
					AutoCreateBucket: true,
				},
			},
			expectWarnings: 1,
		},
		{
			name: "Strict validation with high concurrency",
			config: &SharedConfig{
				Security: SecurityConfig{
					Validation: ValidationConfig{
						StrictMode: true,
					},
				},
				Performance: PerformanceConfig{
					Limits: LimitsConfig{
						MaxConcurrentOperations: 100,
					},
				},
			},
			expectWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewConfigValidator(tt.config)
			validator.validateCrossFieldRules()

			if len(validator.result.Errors) != tt.expectErrors {
				t.Errorf("Expected %d errors, got %d", tt.expectErrors, len(validator.result.Errors))
			}

			if len(validator.result.Warnings) != tt.expectWarnings {
				t.Errorf("Expected %d warnings, got %d", tt.expectWarnings, len(validator.result.Warnings))
			}
		})
	}
}

func TestValidateConfig_CompleteConfiguration(t *testing.T) {
	config := &SharedConfig{
		SchemaVersion: "1.0.0",
		Description:   "Test configuration",
		Storage: StorageConfig{
			Type:      "minio",
			Endpoint:  "localhost:9000",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Bucket:    "test-bucket",
			UseSSL:    false,
			Region:    "us-east-1",
			Connection: ConnectionConfig{
				Timeout:    30,
				MaxRetries: 3,
				RetryDelay: 5 * time.Second,
			},
		},
		Cluster: ClusterConfig{
			Name:   "test-cluster",
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
				URL:    "git@github.com:user/repo.git",
				Branch: "main",
				Auth: AuthConfig{
					Method: "ssh",
					SSH: SSHAuthConfig{
						PrivateKeyPath: "/home/user/.ssh/id_rsa",
					},
				},
			},
			Structure: StructureConfig{
				ArgoCD: ArgoCDConfig{
					Enabled:   true,
					Namespace: "argocd",
					Project:   "default",
				},
			},
		},
		Pipeline: PipelineConfig{
			Mode: "sequential",
			Automation: AutomationConfig{
				Enabled:                 true,
				TriggerOnBackupComplete: true,
				MaxWaitTime:             300,
				TriggerMethods:          []string{"file", "process", "webhook"},
			},
			Notifications: NotificationsConfig{
				Webhook: WebhookConfig{
					URL: "https://hooks.slack.com/test", // Required for notifications
				},
			},
		},
		Security: SecurityConfig{
			Secrets: SecretsConfig{
				Provider: "env",
			},
			Validation: ValidationConfig{
				StrictMode: true,
			},
		},
		Performance: PerformanceConfig{
			Limits: LimitsConfig{
				MaxConcurrentOperations: 10,
			},
		},
	}

	result, err := ValidateConfig(config)
	if err != nil {
		t.Fatalf("Expected valid configuration, got error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected configuration to be valid, got: %s", FormatValidationResult(result))
	}
}

func TestValidateConfig_InvalidConfiguration(t *testing.T) {
	config := &SharedConfig{
		// Missing required fields
		Storage: StorageConfig{
			Type: "invalid-type",
		},
		Cluster: ClusterConfig{
			Type: "invalid-type",
		},
	}

	result, err := ValidateConfig(config)
	if err == nil {
		t.Fatal("Expected validation to fail")
	}

	if result.Valid {
		t.Error("Expected configuration to be invalid")
	}

	if len(result.Errors) == 0 {
		t.Error("Expected validation errors")
	}
}

func TestFormatValidationResult(t *testing.T) {
	result := &ValidationResult{
		Valid: false,
		Errors: []ValidationError{
			{Field: "storage.endpoint", Value: "", Message: "Storage endpoint is required", Level: "error"},
			{Field: "cluster.name", Value: "", Message: "Cluster name is required", Level: "error"},
		},
		Warnings: []ValidationError{
			{Field: "backup.behavior.batch_size", Value: 500, Message: "Large batch size may cause performance issues", Level: "warning"},
		},
	}

	formatted := FormatValidationResult(result)

	// Check that the formatted output contains expected elements
	if !strings.Contains(formatted, "❌ Configuration validation failed") {
		t.Error("Expected error header in formatted output")
	}

	if !strings.Contains(formatted, "storage.endpoint") {
		t.Error("Expected error field in formatted output")
	}

	if !strings.Contains(formatted, "⚠️") {
		t.Error("Expected warning indicator in formatted output")
	}
}

func TestValidationHelperFunctions(t *testing.T) {
	tests := []struct {
		name     string
		function func(string) bool
		input    string
		expected bool
	}{
		{"Valid semver", isValidSemver, "1.0.0", true},
		{"Invalid semver", isValidSemver, "1.0", false},
		{"Valid endpoint", isValidEndpoint, "localhost:9000", true},
		{"Invalid endpoint", isValidEndpoint, "invalid", false},
		{"Valid bucket name", isValidBucketName, "my-bucket", true},
		{"Invalid bucket name", isValidBucketName, "My-Bucket", false},
		{"Valid DNS name", isValidDNSName, "test-cluster", true},
		{"Invalid DNS name", isValidDNSName, "Test_Cluster", false},
		{"Valid domain", isValidDomain, "cluster.local", true},
		{"Invalid domain", isValidDomain, "cluster..local", false},
		{"Valid Git URL", isValidGitURL, "git@github.com:user/repo.git", true},
		{"Invalid Git URL", isValidGitURL, "invalid-url", false},
		{"Valid size", isValidSize, "10Mi", true},
		{"Invalid size", isValidSize, "10X", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.function(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v for input %s, got %v", tt.expected, tt.input, result)
			}
		})
	}
}