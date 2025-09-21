package backup

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cluster-backup/internal/config"
	"cluster-backup/internal/logging"
	"cluster-backup/internal/metrics"
	"cluster-backup/tests/mocks"
)

func TestNewClusterBackup(t *testing.T) {
	cfg := &config.Config{
		ClusterName:   "test-cluster",
		MinIOBucket:   "test-bucket",
		MinIOEndpoint: "localhost:9000",
	}
	
	backupCfg := &config.BackupConfig{
		FilteringMode: "whitelist",
	}
	
	mockClients := mocks.NewMockKubernetesClients()
	mockMinio := mocks.NewMockMinioClient()
	logger := logging.NewStructuredLogger("test", "test-cluster")
	backupMetrics := metrics.NewBackupMetrics()
	ctx := context.Background()

	backup := NewClusterBackup(
		cfg,
		backupCfg,
		mockClients.KubeClient,
		mockClients.DynamicClient,
		mockClients.DiscoveryClient,
		mockMinio,
		logger,
		backupMetrics,
		ctx,
	)

	assert.NotNil(t, backup)
	assert.Equal(t, cfg, backup.config)
	assert.Equal(t, backupCfg, backup.backupConfig)
	assert.Equal(t, logger, backup.logger)
	assert.Equal(t, backupMetrics, backup.metrics)
}

func TestClusterBackup_testMinIOConnectivity(t *testing.T) {
	tests := []struct {
		name             string
		bucketExists     bool
		autoCreateBucket bool
		minioError       bool
		expectError      bool
		expectBucketCall bool
		expectCreateCall bool
	}{
		{
			name:             "bucket_exists",
			bucketExists:     true,
			autoCreateBucket: false,
			minioError:       false,
			expectError:      false,
			expectBucketCall: true,
			expectCreateCall: false,
		},
		{
			name:             "bucket_missing_auto_create_enabled",
			bucketExists:     false,
			autoCreateBucket: true,
			minioError:       false,
			expectError:      false,
			expectBucketCall: true,
			expectCreateCall: true,
		},
		{
			name:             "bucket_missing_auto_create_disabled",
			bucketExists:     false,
			autoCreateBucket: false,
			minioError:       false,
			expectError:      true,
			expectBucketCall: true,
			expectCreateCall: false,
		},
		{
			name:             "minio_connectivity_error",
			bucketExists:     false,
			autoCreateBucket: false,
			minioError:       true,
			expectError:      true,
			expectBucketCall: true,
			expectCreateCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				ClusterName:      "test-cluster",
				MinIOBucket:      "test-bucket",
				AutoCreateBucket: tt.autoCreateBucket,
			}

			mockMinio := mocks.NewMockMinioClient()
			if tt.bucketExists {
				mockMinio.AddTestBucket("test-bucket")
			}
			if tt.minioError {
				mockMinio.SetError(true, "MinIO connection failed")
			}

			backup := &ClusterBackup{
				config:      cfg,
				minioClient: mockMinio,
				ctx:         context.Background(),
				logger:      logging.NewStructuredLogger("test", "test-cluster"),
			}

			err := backup.testMinIOConnectivity()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			callLog := mockMinio.GetCallLog()
			if tt.expectBucketCall {
				assert.Contains(t, callLog, "BucketExists(test-bucket)")
			}
			if tt.expectCreateCall {
				assert.Contains(t, callLog, "MakeBucket(test-bucket)")
			}
		})
	}
}

func TestClusterBackup_getNamespacesToBackup(t *testing.T) {
	mockClients := mocks.NewMockKubernetesClients()
	
	backup := &ClusterBackup{
		kubeClient: mockClients.KubeClient,
		ctx:        context.Background(),
		backupConfig: &config.BackupConfig{
			IncludeNamespaces: []string{},
			ExcludeNamespaces: []string{},
		},
	}

	namespaces, err := backup.getNamespacesToBackup()
	require.NoError(t, err)
	
	// Should get all namespaces from mock client
	expectedNamespaces := []string{"default", "kube-system", "test-namespace", "openshift-config"}
	assert.ElementsMatch(t, expectedNamespaces, namespaces)
}

func TestClusterBackup_filterNamespaces(t *testing.T) {
	tests := []struct {
		name              string
		allNamespaces     []string
		includeNamespaces []string
		excludeNamespaces []string
		expected          []string
	}{
		{
			name:              "include_list_specified",
			allNamespaces:     []string{"default", "kube-system", "test-ns", "app-ns"},
			includeNamespaces: []string{"test-ns", "app-ns"},
			excludeNamespaces: []string{"kube-system"},
			expected:          []string{"test-ns", "app-ns"},
		},
		{
			name:              "exclude_list_only",
			allNamespaces:     []string{"default", "kube-system", "test-ns", "app-ns"},
			includeNamespaces: []string{},
			excludeNamespaces: []string{"kube-system", "default"},
			expected:          []string{"test-ns", "app-ns"},
		},
		{
			name:              "no_filtering",
			allNamespaces:     []string{"default", "kube-system", "test-ns"},
			includeNamespaces: []string{},
			excludeNamespaces: []string{},
			expected:          []string{"default", "kube-system", "test-ns"},
		},
		{
			name:              "partial_matches_in_exclude",
			allNamespaces:     []string{"default", "kube-system", "kube-proxy", "test-ns"},
			includeNamespaces: []string{},
			excludeNamespaces: []string{"kube"},
			expected:          []string{"default", "test-ns"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backup := &ClusterBackup{
				backupConfig: &config.BackupConfig{
					IncludeNamespaces: tt.includeNamespaces,
					ExcludeNamespaces: tt.excludeNamespaces,
				},
			}

			result := backup.filterNamespaces(tt.allNamespaces)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestClusterBackup_shouldBackupResource(t *testing.T) {
	tests := []struct {
		name             string
		resourceName     string
		includeResources []string
		excludeResources []string
		expected         bool
	}{
		{
			name:             "include_list_contains_resource",
			resourceName:     "deployments",
			includeResources: []string{"deployments", "services"},
			excludeResources: []string{},
			expected:         true,
		},
		{
			name:             "include_list_missing_resource",
			resourceName:     "secrets",
			includeResources: []string{"deployments", "services"},
			excludeResources: []string{},
			expected:         false,
		},
		{
			name:             "exclude_list_contains_resource",
			resourceName:     "secrets",
			includeResources: []string{},
			excludeResources: []string{"secrets", "events"},
			expected:         false,
		},
		{
			name:             "exclude_list_missing_resource",
			resourceName:     "deployments",
			includeResources: []string{},
			excludeResources: []string{"secrets", "events"},
			expected:         true,
		},
		{
			name:             "no_filtering",
			resourceName:     "deployments",
			includeResources: []string{},
			excludeResources: []string{},
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backup := &ClusterBackup{
				backupConfig: &config.BackupConfig{
					IncludeResources: tt.includeResources,
					ExcludeResources: tt.excludeResources,
				},
			}

			result := backup.shouldBackupResource(tt.resourceName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClusterBackup_ExecuteBackup(t *testing.T) {
	tests := []struct {
		name               string
		bucketExists       bool
		autoCreateBucket   bool
		includeNamespaces  []string
		expectError        bool
		expectedNamespaces int
	}{
		{
			name:               "successful_backup_with_existing_bucket",
			bucketExists:       true,
			autoCreateBucket:   false,
			includeNamespaces:  []string{"default", "test-namespace"},
			expectError:        false,
			expectedNamespaces: 2,
		},
		{
			name:               "successful_backup_with_auto_create",
			bucketExists:       false,
			autoCreateBucket:   true,
			includeNamespaces:  []string{"default"},
			expectError:        false,
			expectedNamespaces: 1,
		},
		{
			name:               "bucket_missing_no_auto_create",
			bucketExists:       false,
			autoCreateBucket:   false,
			includeNamespaces:  []string{},
			expectError:        true,
			expectedNamespaces: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				ClusterName:      "test-cluster",
				MinIOBucket:      "test-bucket",
				AutoCreateBucket: tt.autoCreateBucket,
			}

			backupCfg := &config.BackupConfig{
				IncludeNamespaces: tt.includeNamespaces,
				ExcludeNamespaces: []string{},
				IncludeResources:  []string{"pods", "services"},
				ExcludeResources:  []string{},
			}

			mockClients := mocks.NewMockKubernetesClients()
			mockMinio := mocks.NewMockMinioClient()
			if tt.bucketExists {
				mockMinio.AddTestBucket("test-bucket")
			}

			backup := NewClusterBackup(
				cfg,
				backupCfg,
				mockClients.KubeClient,
				mockClients.DynamicClient,
				mockClients.DiscoveryClient,
				mockMinio,
				logging.NewStructuredLogger("test", "test-cluster"),
				metrics.NewBackupMetrics(),
				context.Background(),
			)

			result, err := backup.ExecuteBackup()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedNamespaces, result.NamespacesBackedUp)
				assert.True(t, result.Duration > 0)
				assert.False(t, result.StartTime.IsZero())
				assert.False(t, result.EndTime.IsZero())
			}
		})
	}
}

func TestClusterBackup_HelperFunctions(t *testing.T) {
	backup := &ClusterBackup{}

	t.Run("intersectStringSlices", func(t *testing.T) {
		slice1 := []string{"a", "b", "c", "d"}
		slice2 := []string{"b", "d", "e", "f"}
		expected := []string{"b", "d"}

		result := backup.intersectStringSlices(slice1, slice2)
		assert.ElementsMatch(t, expected, result)
	})

	t.Run("excludeStringSlices", func(t *testing.T) {
		slice1 := []string{"a", "b", "c", "d"}
		slice2 := []string{"b", "d"}
		expected := []string{"a", "c"}

		result := backup.excludeStringSlices(slice1, slice2)
		assert.ElementsMatch(t, expected, result)
	})

	t.Run("stringInSlice", func(t *testing.T) {
		slice := []string{"kube-system", "openshift", "default"}

		// Exact match
		assert.True(t, backup.stringInSlice("default", slice))
		
		// Partial match (contains)
		assert.True(t, backup.stringInSlice("kube-proxy", slice)) // Contains "kube"
		
		// No match
		assert.False(t, backup.stringInSlice("test-namespace", slice))
	})
}

// Benchmark tests
func BenchmarkClusterBackup_filterNamespaces(b *testing.B) {
	backup := &ClusterBackup{
		backupConfig: &config.BackupConfig{
			IncludeNamespaces: []string{},
			ExcludeNamespaces: []string{"kube-system", "kube-proxy", "openshift"},
		},
	}

	namespaces := []string{
		"default", "kube-system", "kube-proxy", "kube-public",
		"openshift-config", "openshift-monitoring", "test-ns1",
		"test-ns2", "app-namespace", "database-namespace",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backup.filterNamespaces(namespaces)
	}
}

func BenchmarkClusterBackup_shouldBackupResource(b *testing.B) {
	backup := &ClusterBackup{
		backupConfig: &config.BackupConfig{
			IncludeResources: []string{},
			ExcludeResources: []string{"events", "endpoints", "secrets"},
		},
	}

	resources := []string{
		"pods", "services", "deployments", "configmaps",
		"secrets", "events", "endpoints", "replicasets",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, resource := range resources {
			backup.shouldBackupResource(resource)
		}
	}
}