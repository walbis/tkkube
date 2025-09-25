package sharedconfig

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnhancedMultiClusterValidator(t *testing.T) {
	// Create validator with test options
	options := &EnhancedValidationOptions{
		EnableConnectivityChecks: false, // Disable for unit tests
		EnableTokenValidation:    true,
		ValidationTimeout:        5 * time.Second,
		MaxConcurrentChecks:      2,
		CacheTimeout:             1 * time.Minute,
	}

	validator := NewEnhancedMultiClusterValidator(options)

	t.Run("validator_creation", func(t *testing.T) {
		if validator == nil {
			t.Fatal("Expected validator to be created")
		}

		if validator.baseValidator == nil {
			t.Error("Base validator not initialized")
		}

		if validator.authManager == nil {
			t.Error("Auth manager not initialized")
		}

		if validator.cachedResults == nil {
			t.Error("Cached results not initialized")
		}
	})

	t.Run("token_validation", func(t *testing.T) {
		testTokenValidation(t, validator)
	})

	t.Run("configuration_validation", func(t *testing.T) {
		testConfigurationValidation(t, validator)
	})

	t.Run("validation_caching", func(t *testing.T) {
		testValidationCaching(t, validator)
	})
}

func testTokenValidation(t *testing.T, validator *EnhancedMultiClusterValidator) {
	tests := []struct {
		name          string
		cluster       *MultiClusterClusterConfig
		expectValid   bool
		expectError   string
	}{
		{
			name: "valid_bearer_token",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "valid-token-12345678901234567890",
						Type:  "bearer",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "empty_token",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "",
						Type:  "bearer",
					},
				},
			},
			expectValid: false,
			expectError: "Token value is empty",
		},
		{
			name: "short_token",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "short",
						Type:  "bearer",
					},
				},
			},
			expectValid: false,
			expectError: "Token appears to be too short",
		},
		{
			name: "unexpanded_env_var_token",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "${CLUSTER_TOKEN}",
						Type:  "bearer",
					},
				},
			},
			expectValid: false,
			expectError: "Token appears to contain unexpanded environment variable",
		},
		{
			name: "valid_service_account",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "service_account",
					ServiceAccount: ServiceAccountConfig{
						TokenPath:  "/tmp/test-token",
						CACertPath: "/tmp/test-ca.crt",
					},
				},
			},
			expectValid: true, // Will be valid even if files don't exist in test environment
		},
		{
			name: "missing_service_account_token_path",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "service_account",
					ServiceAccount: ServiceAccountConfig{
						TokenPath: "",
					},
				},
			},
			expectValid: false,
			expectError: "Token path is required for service account authentication",
		},
		{
			name: "valid_oidc",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "oidc",
					OIDC: OIDCConfig{
						IssuerURL: "https://oidc.example.com",
						ClientID:  "test-client",
						IDToken:   "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImp0aSI6ImY2MTcyYWZmLTBjYmMtNGFmNC1hMGVkLTBkYmU4MDkzM2VkOCIsImlhdCI6MTYyNzg5MzQ5MSwiZXhwIjoxNjI3ODk3MDkxfQ.example-signature",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "oidc_missing_issuer",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "oidc",
					OIDC: OIDCConfig{
						ClientID: "test-client",
						IDToken:  "test-token",
					},
				},
			},
			expectValid: false,
			expectError: "OIDC issuer URL is required",
		},
		{
			name: "valid_exec",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "exec",
					Exec: ExecConfig{
						Command: "/bin/echo", // Should exist on most systems
						Args:    []string{"test-token"},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "exec_missing_command",
			cluster: &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "exec",
					Exec:   ExecConfig{},
				},
			},
			expectValid: false,
			expectError: "Exec command is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.validateClusterToken(tt.cluster)

			if result.Valid != tt.expectValid {
				t.Errorf("Expected valid=%v, got valid=%v", tt.expectValid, result.Valid)
			}

			if !tt.expectValid && tt.expectError != "" {
				if !strings.Contains(result.ErrorDetails, tt.expectError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectError, result.ErrorDetails)
				}
			}

			if result.ValidationMethod != tt.cluster.Auth.Method {
				t.Errorf("Expected validation method '%s', got '%s'", tt.cluster.Auth.Method, result.ValidationMethod)
			}
		})
	}
}

func testConfigurationValidation(t *testing.T, validator *EnhancedMultiClusterValidator) {
	tests := []struct {
		name        string
		config      *MultiClusterConfig
		expectValid bool
		expectError string
	}{
		{
			name: "valid_configuration",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "test-cluster",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "test-cluster",
						Endpoint: "https://api.test.k8s.local:6443",
						Auth: ClusterAuthConfig{
							Method: "token",
							Token: TokenAuthConfig{
								Value: "valid-token-123456789012345",
								Type:  "bearer",
							},
						},
						Storage: StorageConfig{
							Type:      "minio",
							Endpoint:  "minio.test.local:9000",
							AccessKey: "test-key",
							SecretKey: "test-secret",
							Bucket:    "test-bucket",
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
			name: "insecure_endpoint",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "test-cluster",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "test-cluster",
						Endpoint: "http://api.test.k8s.local:6443", // HTTP instead of HTTPS
						Auth: ClusterAuthConfig{
							Method: "token",
							Token: TokenAuthConfig{
								Value: "valid-token-123456789012345",
								Type:  "bearer",
							},
						},
						Storage: StorageConfig{
							Type:      "minio",
							Endpoint:  "minio.test.local:9000",
							AccessKey: "test-key",
							SecretKey: "test-secret",
							Bucket:    "test-bucket",
						},
					},
				},
			},
			expectValid: false,
			expectError: "Cluster endpoint must use HTTPS for security",
		},
		{
			name: "demo_token",
			config: &MultiClusterConfig{
				Enabled:        true,
				Mode:           "sequential",
				DefaultCluster: "test-cluster",
				Clusters: []MultiClusterClusterConfig{
					{
						Name:     "test-cluster",
						Endpoint: "https://api.test.k8s.local:6443",
						Auth: ClusterAuthConfig{
							Method: "token",
							Token: TokenAuthConfig{
								Value: "demo-token-123456789", // Contains "demo"
								Type:  "bearer",
							},
						},
						Storage: StorageConfig{
							Type:      "minio",
							Endpoint:  "minio.test.local:9000",
							AccessKey: "test-key",
							SecretKey: "test-secret",
							Bucket:    "test-bucket",
						},
					},
				},
			},
			expectValid: false,
			expectError: "Appears to contain demo or default token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateMultiClusterConfigurationWithLiveChecks(tt.config)

			if result.OverallValid != tt.expectValid {
				t.Errorf("Expected overall_valid=%v, got overall_valid=%v", tt.expectValid, result.OverallValid)
				
				// Print errors for debugging
				if len(result.GlobalErrors) > 0 {
					t.Logf("Global errors: %+v", result.GlobalErrors)
				}
				for clusterName, clusterResult := range result.ClusterResults {
					if len(clusterResult.Errors) > 0 {
						t.Logf("Cluster %s errors: %+v", clusterName, clusterResult.Errors)
					}
				}
			}

			if !tt.expectValid && tt.expectError != "" {
				found := false
				
				// Check global errors
				for _, err := range result.GlobalErrors {
					if strings.Contains(err.Message, tt.expectError) {
						found = true
						break
					}
				}
				
				// Check cluster errors if not found in global errors
				if !found {
					for _, clusterResult := range result.ClusterResults {
						for _, err := range clusterResult.Errors {
							if strings.Contains(err.Message, tt.expectError) {
								found = true
								break
							}
						}
						if found {
							break
						}
					}
				}
				
				if !found {
					t.Errorf("Expected error containing '%s' not found", tt.expectError)
				}
			}
		})
	}
}

func testValidationCaching(t *testing.T, validator *EnhancedMultiClusterValidator) {
	clusterName := "cache-test-cluster"
	
	// First validation should not be cached
	result1 := validator.getCachedResult(clusterName)
	if result1 != nil {
		t.Error("Expected no cached result initially")
	}
	
	// Create and cache a result
	testResult := &ClusterValidationResult{
		ClusterName: clusterName,
		Valid:       true,
		ValidatedAt: time.Now(),
	}
	
	validator.cacheResult(clusterName, testResult)
	
	// Should now return cached result
	result2 := validator.getCachedResult(clusterName)
	if result2 == nil {
		t.Error("Expected cached result to be returned")
		return
	}
	
	if result2.ClusterName != clusterName {
		t.Errorf("Expected cluster name '%s', got '%s'", clusterName, result2.ClusterName)
	}
}

func TestConnectivityChecks(t *testing.T) {
	// Create validator with connectivity checks enabled
	options := &EnhancedValidationOptions{
		EnableConnectivityChecks: true,
		EnableTokenValidation:    false, // Disable to focus on connectivity
		ValidationTimeout:        5 * time.Second,
	}

	validator := NewEnhancedMultiClusterValidator(options)

	t.Run("network_connectivity_test", func(t *testing.T) {
		// Test with a known reachable endpoint (example.com should be reachable)
		reachable := validator.testNetworkConnectivity("https://example.com:443")
		if !reachable {
			t.Log("Warning: example.com:443 not reachable - this might be expected in some test environments")
		}
		
		// Test with unreachable endpoint
		unreachable := validator.testNetworkConnectivity("https://invalid-domain-12345.example:6443")
		if unreachable {
			t.Error("Expected unreachable endpoint to fail connectivity test")
		}
	})

	t.Run("mock_server_connectivity", func(t *testing.T) {
		// Create a mock HTTP server for testing
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"kind":"APIVersions","versions":["v1"]}`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"kind":"Status","code":401}`))
			}
		}))
		defer server.Close()

		cluster := &MultiClusterClusterConfig{
			Name:     "mock-cluster",
			Endpoint: server.URL,
			Auth: ClusterAuthConfig{
				Method: "token",
				Token: TokenAuthConfig{
					Value: "test-token-123456789",
					Type:  "bearer",
				},
			},
			TLS: ClusterTLSConfig{
				Insecure: true, // Skip TLS verification for test server
			},
		}

		// Test connectivity with mock server
		status := validator.validateClusterConnectivity(cluster)
		
		if !status.Reachable {
			t.Errorf("Expected mock server to be reachable, got error: %s", status.ErrorDetails)
		}
		
		if status.ResponseTime <= 0 {
			t.Error("Expected response time to be greater than 0")
		}
		
		// TLS should be considered valid since we're allowing insecure
		if !status.TLSValid {
			t.Error("Expected TLS to be considered valid with insecure flag")
		}
	})
}

func TestLiveValidationService(t *testing.T) {
	config := createTestMultiClusterConfigForValidation()
	
	serviceConfig := &LiveValidationServiceConfig{
		Enabled:               true,
		ValidationInterval:    1 * time.Second, // Fast interval for testing
		HealthCheckInterval:   500 * time.Millisecond,
		HTTPServerPort:        8091, // Different port to avoid conflicts
		MaxHistoryEntries:     10,
		EnableEventHandlers:   true,
		ValidationOptions: &EnhancedValidationOptions{
			EnableConnectivityChecks: false, // Disable for unit tests
			EnableTokenValidation:    true,
		},
	}

	service := NewLiveValidationService(config, serviceConfig)

	t.Run("service_creation", func(t *testing.T) {
		if service == nil {
			t.Fatal("Expected service to be created")
		}

		if service.validator == nil {
			t.Error("Validator not initialized")
		}

		if service.config == nil {
			t.Error("Config not initialized")
		}
	})

	t.Run("event_handling", func(t *testing.T) {
		eventReceived := false
		
		service.RegisterEventHandler(EventValidationStarted, func(event *ValidationEvent) error {
			eventReceived = true
			if event.Type != EventValidationStarted {
				t.Errorf("Expected event type %s, got %s", EventValidationStarted, event.Type)
			}
			return nil
		})

		// Emit test event
		service.emitEvent(&ValidationEvent{
			Type:      EventValidationStarted,
			Timestamp: time.Now(),
			Severity:  SeverityInfo,
		})

		// Give some time for async event handling
		time.Sleep(100 * time.Millisecond)

		if !eventReceived {
			t.Error("Expected event to be received by handler")
		}
	})

	t.Run("validation_triggering", func(t *testing.T) {
		// Start service briefly to test validation
		err := service.Start()
		if err != nil {
			t.Fatalf("Failed to start service: %v", err)
		}
		defer service.Stop()

		// Wait for initial validation
		time.Sleep(2 * time.Second)

		// Check that validation was performed
		result := service.GetLastValidationResult()
		if result == nil {
			t.Error("Expected validation result to be available")
		}

		// Trigger manual validation
		service.TriggerValidation()
		time.Sleep(1 * time.Second)

		// Check that validation was updated
		newResult := service.GetLastValidationResult()
		if newResult == nil {
			t.Error("Expected updated validation result")
		}
	})
}

func TestEnhancedValidationHelperFunctions(t *testing.T) {
	validator := NewEnhancedMultiClusterValidator(nil)

	t.Run("kubernetes_compliant_names", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected bool
		}{
			{"valid_name", "test-cluster", true},
			{"valid_single_char", "a", true},
			{"valid_with_numbers", "cluster-123", true},
			{"invalid_uppercase", "Test-Cluster", false},
			{"invalid_underscore", "test_cluster", false},
			{"invalid_start_dash", "-test-cluster", false},
			{"invalid_end_dash", "test-cluster-", false},
			{"invalid_dots", "test.cluster", false},
			{"valid_long_name", strings.Repeat("a", 63), true},
			{"invalid_too_long", strings.Repeat("a", 64), false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := validator.isKubernetesCompliantName(tt.input)
				if result != tt.expected {
					t.Errorf("Expected %v for name '%s', got %v", tt.expected, tt.input, result)
				}
			})
		}
	})

	t.Run("demo_token_detection", func(t *testing.T) {
		tests := []struct {
			name     string
			cluster  *MultiClusterClusterConfig
			expected bool
		}{
			{
				name: "contains_demo",
				cluster: &MultiClusterClusterConfig{
					Auth: ClusterAuthConfig{
						Method: "token",
						Token:  TokenAuthConfig{Value: "demo-token-123"},
					},
				},
				expected: true,
			},
			{
				name: "contains_test",
				cluster: &MultiClusterClusterConfig{
					Auth: ClusterAuthConfig{
						Method: "token",
						Token:  TokenAuthConfig{Value: "test123456"},
					},
				},
				expected: true,
			},
			{
				name: "valid_production_token",
				cluster: &MultiClusterClusterConfig{
					Auth: ClusterAuthConfig{
						Method: "token",
						Token:  TokenAuthConfig{Value: "prod-secure-token-abcdef123456"},
					},
				},
				expected: false,
			},
			{
				name: "oidc_demo_secret",
				cluster: &MultiClusterClusterConfig{
					Auth: ClusterAuthConfig{
						Method: "oidc",
						OIDC:   OIDCConfig{ClientSecret: "demo-secret"},
					},
				},
				expected: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := validator.containsDemoToken(tt.cluster)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})

	t.Run("command_existence", func(t *testing.T) {
		// Test with a command that should exist
		if !validator.commandExists("ls") && !validator.commandExists("/bin/ls") {
			t.Log("Warning: Neither 'ls' nor '/bin/ls' found - this might be expected in some test environments")
		}

		// Test with a command that should not exist
		if validator.commandExists("definitely-does-not-exist-command-12345") {
			t.Error("Expected non-existent command to return false")
		}
	})
}

func TestServiceAccountTokenValidation(t *testing.T) {
	validator := NewEnhancedMultiClusterValidator(nil)

	// Create a temporary token file for testing
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	caPath := filepath.Join(tmpDir, "ca.crt")

	// Write test token
	err := os.WriteFile(tokenPath, []byte("test-service-account-token"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test token file: %v", err)
	}

	// Write test CA
	err = os.WriteFile(caPath, []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test CA file: %v", err)
	}

	tests := []struct {
		name        string
		config      *ServiceAccountConfig
		expectValid bool
		expectError string
	}{
		{
			name: "valid_service_account_files_exist",
			config: &ServiceAccountConfig{
				TokenPath:  tokenPath,
				CACertPath: caPath,
			},
			expectValid: true,
		},
		{
			name: "token_file_missing",
			config: &ServiceAccountConfig{
				TokenPath:  filepath.Join(tmpDir, "nonexistent-token"),
				CACertPath: caPath,
			},
			expectValid: true, // Still valid - might be running outside pod
		},
		{
			name: "no_token_path",
			config: &ServiceAccountConfig{
				TokenPath: "",
			},
			expectValid: false,
			expectError: "Token path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method:         "service_account",
					ServiceAccount: *tt.config,
				},
			}

			result := validator.validateServiceAccountToken(tt.config, cluster)

			if result.Valid != tt.expectValid {
				t.Errorf("Expected valid=%v, got valid=%v", tt.expectValid, result.Valid)
			}

			if !tt.expectValid && tt.expectError != "" {
				if !strings.Contains(result.ErrorDetails, tt.expectError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectError, result.ErrorDetails)
				}
			}
		})
	}
}

func TestValidationMetrics(t *testing.T) {
	config := createTestMultiClusterConfigForValidation()
	
	serviceConfig := &LiveValidationServiceConfig{
		ValidationOptions: &EnhancedValidationOptions{
			EnableConnectivityChecks: false,
			EnableTokenValidation:    true,
		},
	}

	service := NewLiveValidationService(config, serviceConfig)

	// Create a mock validation result
	result := &EnhancedValidationResult{
		OverallValid:    true,
		ValidationTime:  time.Now(),
		ClusterResults: map[string]*ClusterValidationResult{
			"cluster1": {
				ClusterName: "cluster1",
				Valid:       true,
				ValidatedAt: time.Now(),
				ConnectivityStatus: &ConnectivityStatus{
					ResponseTime: 100 * time.Millisecond,
				},
			},
			"cluster2": {
				ClusterName: "cluster2",
				Valid:       false,
				ValidatedAt: time.Now(),
				Errors: []ValidationError{
					{Field: "token", Message: "Invalid token"},
				},
			},
		},
	}

	// Update metrics
	service.updateValidationMetrics(result, 5*time.Second)

	// Check metrics
	service.metrics.mutex.RLock()
	defer service.metrics.mutex.RUnlock()

	if service.metrics.TotalValidations != 1 {
		t.Errorf("Expected 1 total validation, got %d", service.metrics.TotalValidations)
	}

	if service.metrics.SuccessfulValidations != 1 {
		t.Errorf("Expected 1 successful validation, got %d", service.metrics.SuccessfulValidations)
	}

	if service.metrics.AverageValidationTime != 5*time.Second {
		t.Errorf("Expected 5s average validation time, got %v", service.metrics.AverageValidationTime)
	}

	// Check cluster-specific metrics
	cluster1Metrics, exists := service.metrics.ClusterMetrics["cluster1"]
	if !exists {
		t.Error("Expected cluster1 metrics to exist")
	} else {
		if cluster1Metrics.SuccessfulValidations != 1 {
			t.Errorf("Expected 1 successful validation for cluster1, got %d", cluster1Metrics.SuccessfulValidations)
		}
	}

	cluster2Metrics, exists := service.metrics.ClusterMetrics["cluster2"]
	if !exists {
		t.Error("Expected cluster2 metrics to exist")
	} else {
		if cluster2Metrics.FailedValidations != 1 {
			t.Errorf("Expected 1 failed validation for cluster2, got %d", cluster2Metrics.FailedValidations)
		}
		if cluster2Metrics.ConsecutiveFailures != 1 {
			t.Errorf("Expected 1 consecutive failure for cluster2, got %d", cluster2Metrics.ConsecutiveFailures)
		}
	}
}

// Helper function for creating test configurations
func createTestMultiClusterConfigForValidation() *MultiClusterConfig {
	return &MultiClusterConfig{
		Enabled:        true,
		Mode:           "sequential",
		DefaultCluster: "test-cluster",
		Clusters: []MultiClusterClusterConfig{
			{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "test-token-123456789",
						Type:  "bearer",
					},
				},
				Storage: StorageConfig{
					Type:      "minio",
					Endpoint:  "minio.test.local:9000",
					AccessKey: "test-key",
					SecretKey: "test-secret",
					Bucket:    "test-bucket",
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
}

// Benchmark tests
func BenchmarkTokenValidation(b *testing.B) {
	validator := NewEnhancedMultiClusterValidator(nil)
	
	cluster := &MultiClusterClusterConfig{
		Name:     "benchmark-cluster",
		Endpoint: "https://api.test.k8s.local:6443",
		Auth: ClusterAuthConfig{
			Method: "token",
			Token: TokenAuthConfig{
				Value: "benchmark-token-12345678901234567890",
				Type:  "bearer",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.validateClusterToken(cluster)
	}
}

func BenchmarkFullValidation(b *testing.B) {
	validator := NewEnhancedMultiClusterValidator(&EnhancedValidationOptions{
		EnableConnectivityChecks: false, // Disable for benchmark
		EnableTokenValidation:    true,
	})
	
	config := createTestMultiClusterConfigForValidation()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateMultiClusterConfigurationWithLiveChecks(config)
	}
}