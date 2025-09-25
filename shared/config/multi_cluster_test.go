package sharedconfig

import (
	"os"
	"testing"
)

func TestMultiClusterConfigLoading(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("MULTI_CLUSTER_ENABLED", "true")
	os.Setenv("MULTI_CLUSTER_MODE", "sequential")
	os.Setenv("DEFAULT_CLUSTER", "test-primary")
	os.Setenv("PRIMARY_CLUSTER_NAME", "test-primary")
	os.Setenv("PRIMARY_CLUSTER_ENDPOINT", "https://api.test-primary.k8s.local:6443")
	os.Setenv("PRIMARY_CLUSTER_TOKEN", "test-token-primary-123456789")
	os.Setenv("PRIMARY_CLUSTER_BUCKET", "test-primary-backups")
	os.Setenv("SECONDARY_CLUSTER_NAME", "test-secondary")
	os.Setenv("SECONDARY_CLUSTER_ENDPOINT", "https://api.test-secondary.k8s.local:6443")
	os.Setenv("SECONDARY_CLUSTER_TOKEN", "test-token-secondary-123456789")
	os.Setenv("SECONDARY_CLUSTER_BUCKET", "test-secondary-backups")
	defer cleanupTestEnv()

	loader := NewConfigLoaderForTesting("test-multi-cluster.yaml")
	sharedConfig, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load multi-cluster config: %v", err)
	}

	// Test multi-cluster configuration
	config := &sharedConfig.MultiCluster
	if !config.Enabled {
		t.Error("Multi-cluster should be enabled")
	}

	if config.Mode != "sequential" {
		t.Errorf("Expected mode 'sequential', got '%s'", config.Mode)
	}

	if config.DefaultCluster != "test-primary" {
		t.Errorf("Expected default cluster 'test-primary', got '%s'", config.DefaultCluster)
	}

	// Test cluster configurations
	if len(config.Clusters) < 2 {
		t.Fatalf("Expected at least 2 clusters, got %d", len(config.Clusters))
	}

	// Test primary cluster
	primaryCluster := findCluster(config.Clusters, "test-primary")
	if primaryCluster == nil {
		t.Fatal("Primary cluster not found")
	}

	if primaryCluster.Endpoint != "https://api.test-primary.k8s.local:6443" {
		t.Errorf("Primary cluster endpoint mismatch: %s", primaryCluster.Endpoint)
	}

	// Check for auth configuration (new) or legacy token
	if primaryCluster.Auth.Method == "token" && primaryCluster.Auth.Token.Value != "" {
		if primaryCluster.Auth.Token.Value != "test-token-primary-123456789" {
			t.Errorf("Primary cluster auth token mismatch: expected 'test-token-primary-123456789', got '%s'", primaryCluster.Auth.Token.Value)
		}
	} else if primaryCluster.Token != "test-token-primary-123456789" {
		t.Errorf("Primary cluster legacy token mismatch: expected 'test-token-primary-123456789', got '%s'", primaryCluster.Token)
	}

	if primaryCluster.Storage.Bucket != "test-primary-backups" {
		t.Errorf("Primary cluster bucket mismatch: %s", primaryCluster.Storage.Bucket)
	}

	// Test secondary cluster
	secondaryCluster := findCluster(config.Clusters, "test-secondary")
	if secondaryCluster == nil {
		t.Fatal("Secondary cluster not found")
	}

	if secondaryCluster.Endpoint != "https://api.test-secondary.k8s.local:6443" {
		t.Errorf("Secondary cluster endpoint mismatch: %s", secondaryCluster.Endpoint)
	}

	// Check for auth configuration (new) or legacy token
	if secondaryCluster.Auth.Method == "token" && secondaryCluster.Auth.Token.Value != "" {
		if secondaryCluster.Auth.Token.Value != "test-token-secondary-123456789" {
			t.Errorf("Secondary cluster auth token mismatch: expected 'test-token-secondary-123456789', got '%s'", secondaryCluster.Auth.Token.Value)
		}
	} else if secondaryCluster.Token != "test-token-secondary-123456789" {
		t.Errorf("Secondary cluster legacy token mismatch: expected 'test-token-secondary-123456789', got '%s'", secondaryCluster.Token)
	}

	// Test coordination settings
	coord := config.Coordination
	if coord.Timeout != 600 {
		t.Errorf("Expected coordination timeout 600, got %d", coord.Timeout)
	}

	if coord.RetryAttempts != 3 {
		t.Errorf("Expected retry attempts 3, got %d", coord.RetryAttempts)
	}

	if coord.FailureThreshold != 2 {
		t.Errorf("Expected failure threshold 2, got %d", coord.FailureThreshold)
	}

	if coord.HealthCheckInterval != "60s" {
		t.Errorf("Expected health check interval '60s', got '%s'", coord.HealthCheckInterval)
	}

	// Test scheduling settings
	sched := config.Scheduling
	if sched.Strategy != "round_robin" {
		t.Errorf("Expected scheduling strategy 'round_robin', got '%s'", sched.Strategy)
	}

	if sched.MaxConcurrentClusters != 2 {
		t.Errorf("Expected max concurrent clusters 2, got %d", sched.MaxConcurrentClusters)
	}
}

func TestMultiClusterManagerCreation(t *testing.T) {
	// Create test configuration
	config := &MultiClusterConfig{
		Enabled:        true,
		Mode:           "sequential",
		DefaultCluster: "test-cluster-1",
		Clusters: []MultiClusterClusterConfig{
			{
				Name:     "test-cluster-1",
				Endpoint: "https://api.test-cluster-1.k8s.local:6443",
				Token:    "test-token-1",
				Storage: StorageConfig{
					Type:      "minio",
					Endpoint:  "test-minio:9000",
					AccessKey: "testkey",
					SecretKey: "testsecret",
					Bucket:    "test-bucket-1",
					UseSSL:    false,
				},
			},
		},
		Coordination: CoordinationConfig{
			Timeout:             300,
			RetryAttempts:       3,
			FailureThreshold:    1,
			HealthCheckInterval: "30s",
		},
		Scheduling: SchedulingConfig{
			Strategy:              "round_robin",
			MaxConcurrentClusters: 1,
		},
	}

	// Test manager creation (will fail without actual Kubernetes API, but validates config)
	manager, err := NewMultiClusterManager(config)
	
	// Manager should be created successfully even without network connectivity
	// The health checks will handle connectivity issues
	if err != nil {
		t.Errorf("Expected manager creation to succeed, got error: %v", err)
	}

	if manager == nil {
		t.Error("Expected non-nil manager")
	}
	
	// Clean up the manager
	if manager != nil {
		manager.Close()
	}
}

func TestMultiClusterValidation(t *testing.T) {
	validator := NewMultiClusterValidator()

	tests := []struct {
		name        string
		config      *MultiClusterConfig
		expectValid bool
		expectError string
	}{
		{
			name: "valid_multi_cluster_config",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "cluster-1",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "cluster-1",
						Endpoint: "https://api.cluster-1.k8s.local:6443",
						Token:    "valid-token-123456789",
						Storage: StorageConfig{
							Type:      "s3",
							Endpoint:  "s3.amazonaws.com",
							AccessKey: "ACCESS123",
							SecretKey: "SECRET456",
							Bucket:    "validbucketname",
							UseSSL:    true,
							Region:    "us-east-1",
						},
					},
				},
				Coordination: CoordinationConfig{
					Timeout:             300,
					RetryAttempts:       3,
					FailureThreshold:    1,
					HealthCheckInterval: "30s",
				},
				Scheduling: SchedulingConfig{
					Strategy:              "round_robin",
					MaxConcurrentClusters: 1,
				},
			},
			expectValid: true,
		},
		{
			name: "invalid_mode",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "invalid-mode",
				DefaultCluster: "cluster-1",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "cluster-1",
						Endpoint: "https://api.cluster-1.k8s.local:6443",
						Token:    "valid-token",
						Storage:  StorageConfig{},
					},
				},
			},
			expectValid: false,
			expectError: "invalid execution mode",
		},
		{
			name: "missing_default_cluster",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "cluster-1",
						Endpoint: "https://api.cluster-1.k8s.local:6443",
						Token:    "valid-token",
						Storage:  StorageConfig{},
					},
				},
			},
			expectValid: false,
			expectError: "default_cluster is required",
		},
		{
			name: "no_clusters",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "cluster-1",
				Clusters:       []MultiClusterClusterConfig{},
			},
			expectValid: false,
			expectError: "at least one cluster must be configured",
		},
		{
			name: "invalid_endpoint",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "cluster-1",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "cluster-1",
						Endpoint: "http://insecure-endpoint:6443",
						Token:    "valid-token",
						Storage:  StorageConfig{},
					},
				},
			},
			expectValid: false,
			expectError: "invalid endpoint format",
		},
		{
			name: "missing_token",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "cluster-1",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "cluster-1",
						Endpoint: "https://api.cluster-1.k8s.local:6443",
						Token:    "",
						Storage:  StorageConfig{},
					},
				},
			},
			expectValid: false,
			expectError: "token is required",
		},
		{
			name: "invalid_timeout",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "cluster-1",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "cluster-1",
						Endpoint: "https://api.cluster-1.k8s.local:6443",
						Token:    "valid-token",
						Storage:  StorageConfig{},
					},
				},
				Coordination: CoordinationConfig{
					Timeout:             -10,
					RetryAttempts:       3,
					FailureThreshold:    1,
					HealthCheckInterval: "30s",
				},
			},
			expectValid: false,
			expectError: "coordination.timeout must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateMultiClusterConfig(tt.config)

			if result.Valid != tt.expectValid {
				t.Errorf("Expected valid=%v, got valid=%v. Errors: %v", tt.expectValid, result.Valid, result.Errors)
			}

			if tt.expectError != "" {
				found := false
				for _, err := range result.Errors {
					if containsString(err, tt.expectError) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing '%s', got errors: %v", tt.expectError, result.Errors)
				}
			}
		})
	}
}

func TestMultiClusterManagerClusterOperations(t *testing.T) {
	// Create a mock multi-cluster manager for testing operations
	config := &MultiClusterConfig{
		Enabled:        true,
		Mode:           "sequential",
		DefaultCluster: "cluster-1",
		Clusters: []MultiClusterClusterConfig{
			{
				Name:     "cluster-1",
				Endpoint: "https://api.cluster-1.k8s.local:6443",
				Token:    "token-1",
				Storage: StorageConfig{
					Bucket: "bucket-1",
				},
			},
			{
				Name:     "cluster-2",
				Endpoint: "https://api.cluster-2.k8s.local:6443",
				Token:    "token-2",
				Storage: StorageConfig{
					Bucket: "bucket-2",
				},
			},
		},
	}

	// Test configuration retrieval methods (these don't require actual Kubernetes connectivity)
	t.Run("get_cluster_names", func(t *testing.T) {
		// We'll create a minimal manager for testing without network calls
		manager := &MultiClusterManager{
			config: config,
		}

		names := manager.GetClusterNames()
		if len(names) != 2 {
			t.Errorf("Expected 2 cluster names, got %d", len(names))
		}

		expectedNames := []string{"cluster-1", "cluster-2"}
		for _, expected := range expectedNames {
			found := false
			for _, name := range names {
				if name == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected cluster name '%s' not found in %v", expected, names)
			}
		}
	})

	t.Run("get_cluster_config", func(t *testing.T) {
		manager := &MultiClusterManager{
			config: config,
		}

		clusterConfig, err := manager.GetClusterConfig("cluster-1")
		if err != nil {
			t.Errorf("Failed to get cluster config: %v", err)
		}

		if clusterConfig.Name != "cluster-1" {
			t.Errorf("Expected cluster name 'cluster-1', got '%s'", clusterConfig.Name)
		}

		if clusterConfig.Storage.Bucket != "bucket-1" {
			t.Errorf("Expected bucket 'bucket-1', got '%s'", clusterConfig.Storage.Bucket)
		}
	})

	t.Run("get_nonexistent_cluster_config", func(t *testing.T) {
		manager := &MultiClusterManager{
			config: config,
		}

		_, err := manager.GetClusterConfig("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent cluster")
		}
	})

	t.Run("is_multi_cluster_enabled", func(t *testing.T) {
		manager := &MultiClusterManager{
			config: config,
		}

		if !manager.IsMultiClusterEnabled() {
			t.Error("Expected multi-cluster to be enabled")
		}
	})

	t.Run("get_execution_mode", func(t *testing.T) {
		manager := &MultiClusterManager{
			config: config,
		}

		mode := manager.GetExecutionMode()
		if mode != "sequential" {
			t.Errorf("Expected execution mode 'sequential', got '%s'", mode)
		}
	})
}

// Helper functions

func findCluster(clusters []MultiClusterClusterConfig, name string) *MultiClusterClusterConfig {
	for i := range clusters {
		if clusters[i].Name == name {
			return &clusters[i]
		}
	}
	return nil
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   len(s) > len(substr) && s[len(s)-len(substr):] == substr ||
		   (len(s) > len(substr) && len(substr) > 0 && 
			func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())
}

func cleanupTestEnv() {
	envVars := []string{
		"MULTI_CLUSTER_ENABLED",
		"MULTI_CLUSTER_MODE", 
		"DEFAULT_CLUSTER",
		"PRIMARY_CLUSTER_NAME",
		"PRIMARY_CLUSTER_ENDPOINT",
		"PRIMARY_CLUSTER_TOKEN",
		"PRIMARY_CLUSTER_BUCKET",
		"SECONDARY_CLUSTER_NAME",
		"SECONDARY_CLUSTER_ENDPOINT",
		"SECONDARY_CLUSTER_TOKEN",
		"SECONDARY_CLUSTER_BUCKET",
	}

	for _, env := range envVars {
		os.Unsetenv(env)
	}
}