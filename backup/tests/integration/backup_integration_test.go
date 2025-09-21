package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"cluster-backup/internal/backup"
	"cluster-backup/internal/config"
	"cluster-backup/internal/logging"
	"cluster-backup/internal/metrics"
	"cluster-backup/tests/mocks"
)

// TestBackupIntegration tests backup operations with a real MinIO instance
func TestBackupIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start MinIO container
	minioContainer, endpoint, err := startMinIOContainer(t)
	if err != nil {
		t.Fatalf("Failed to start MinIO container: %v", err)
	}
	defer func() {
		if err := minioContainer.Terminate(context.Background()); err != nil {
			t.Logf("Failed to terminate MinIO container: %v", err)
		}
	}()

	// Create MinIO client
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	// Test configuration
	cfg := &config.Config{
		ClusterName:      "integration-test-cluster",
		MinIOEndpoint:    endpoint,
		MinIOAccessKey:   "minioadmin",
		MinIOSecretKey:   "minioadmin123",
		MinIOBucket:      "integration-test-bucket",
		MinIOUseSSL:      false,
		AutoCreateBucket: true,
		BatchSize:        10,
		RetryAttempts:    3,
		RetryDelay:       1 * time.Second,
	}

	backupCfg := &config.BackupConfig{
		FilteringMode:     "whitelist",
		IncludeNamespaces: []string{"default", "test-namespace"},
		IncludeResources:  []string{"pods", "services", "deployments", "configmaps"},
		ExcludeResources:  []string{"secrets", "events"},
	}

	// Create mock Kubernetes clients
	mockClients := mocks.NewMockKubernetesClients()
	logger := logging.NewStructuredLogger("integration-test", cfg.ClusterName)
	backupMetrics := metrics.NewBackupMetrics()

	// Create backup instance
	clusterBackup := backup.NewClusterBackup(
		cfg,
		backupCfg,
		mockClients.KubeClient,
		mockClients.DynamicClient,
		mockClients.DiscoveryClient,
		minioClient,
		logger,
		backupMetrics,
		context.Background(),
	)

	// Execute backup
	result, err := clusterBackup.ExecuteBackup()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify backup results
	assert.Equal(t, 2, result.NamespacesBackedUp) // default and test-namespace
	assert.Greater(t, result.ResourcesBackedUp, 0)
	assert.True(t, result.Duration > 0)
	assert.Empty(t, result.Errors)

	// Verify bucket was created
	exists, err := minioClient.BucketExists(context.Background(), cfg.MinIOBucket)
	require.NoError(t, err)
	assert.True(t, exists)

	// List objects in bucket to verify backup artifacts
	objectCh := minioClient.ListObjects(context.Background(), cfg.MinIOBucket, minio.ListObjectsOptions{
		Recursive: true,
	})

	objectCount := 0
	for object := range objectCh {
		require.NoError(t, object.Err)
		objectCount++
		t.Logf("Found backup object: %s", object.Key)
		
		// Verify object naming follows expected pattern
		// Expected: clusterbackup/{cluster-name}/{namespace}/{resource-type}/{resource-name}.yaml
		assert.Contains(t, object.Key, fmt.Sprintf("clusterbackup/%s/", cfg.ClusterName))
	}

	// Should have some backup objects
	assert.Greater(t, objectCount, 0, "Expected backup objects to be created")
}

// TestBackupWithMinIOFailure tests backup behavior when MinIO is unavailable
func TestBackupWithMinIOFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		ClusterName:      "failure-test-cluster",
		MinIOEndpoint:    "localhost:19999", // Non-existent endpoint
		MinIOAccessKey:   "testkey",
		MinIOSecretKey:   "testsecret",
		MinIOBucket:      "test-bucket",
		MinIOUseSSL:      false,
		AutoCreateBucket: false,
	}

	backupCfg := &config.BackupConfig{
		FilteringMode:     "whitelist",
		IncludeNamespaces: []string{"default"},
		IncludeResources:  []string{"pods"},
	}

	// Create MinIO client that will fail
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	require.NoError(t, err)

	mockClients := mocks.NewMockKubernetesClients()
	logger := logging.NewStructuredLogger("failure-test", cfg.ClusterName)
	backupMetrics := metrics.NewBackupMetrics()

	clusterBackup := backup.NewClusterBackup(
		cfg,
		backupCfg,
		mockClients.KubeClient,
		mockClients.DynamicClient,
		mockClients.DiscoveryClient,
		minioClient,
		logger,
		backupMetrics,
		context.Background(),
	)

	// Execute backup - should fail due to MinIO connectivity
	result, err := clusterBackup.ExecuteBackup()
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "MinIO connectivity test failed")
}

// startMinIOContainer starts a MinIO container for testing
func startMinIOContainer(t *testing.T) (testcontainers.Container, string, error) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin123",
		},
		Cmd:        []string{"server", "/data"},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	// Get the mapped port
	mappedPort, err := container.MappedPort(ctx, "9000")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", err
	}

	// Get the host IP
	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", err
	}

	endpoint := fmt.Sprintf("%s:%s", host, mappedPort.Port())
	
	// Wait a bit more for MinIO to be fully ready
	time.Sleep(2 * time.Second)

	return container, endpoint, nil
}

// TestConfigurationValidation tests configuration validation in integration context
func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "valid_configuration",
			config: &config.Config{
				ClusterName:    "test-cluster",
				MinIOEndpoint:  "localhost:9000",
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				MinIOBucket:    "test-bucket",
				BatchSize:      50,
				RetryAttempts:  3,
				RetentionDays:  7,
			},
			expectError: false,
		},
		{
			name: "invalid_batch_size",
			config: &config.Config{
				ClusterName:    "test-cluster",
				MinIOEndpoint:  "localhost:9000",
				MinIOAccessKey: "testkey",
				MinIOSecretKey: "testsecret",
				MinIOBucket:    "test-bucket",
				BatchSize:      0, // Invalid
				RetryAttempts:  3,
				RetentionDays:  7,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEnvironmentVariableIntegration tests configuration loading from environment
func TestEnvironmentVariableIntegration(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"CLUSTER_NAME", "MINIO_ENDPOINT", "MINIO_ACCESS_KEY",
		"MINIO_SECRET_KEY", "MINIO_BUCKET", "BATCH_SIZE",
	}
	
	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
	}
	
	// Clean up environment after test
	defer func() {
		for envVar, value := range originalEnv {
			if value == "" {
				os.Unsetenv(envVar)
			} else {
				os.Setenv(envVar, value)
			}
		}
	}()

	// Set test environment variables
	testEnv := map[string]string{
		"CLUSTER_NAME":     "env-test-cluster",
		"MINIO_ENDPOINT":   "env-minio:9000",
		"MINIO_ACCESS_KEY": "env-access-key",
		"MINIO_SECRET_KEY": "env-secret-key",
		"MINIO_BUCKET":     "env-test-bucket",
		"BATCH_SIZE":       "100",
	}

	for key, value := range testEnv {
		os.Setenv(key, value)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err)

	// Verify configuration values match environment variables
	assert.Equal(t, "env-test-cluster", cfg.ClusterName)
	assert.Equal(t, "env-minio:9000", cfg.MinIOEndpoint)
	assert.Equal(t, "env-access-key", cfg.MinIOAccessKey)
	assert.Equal(t, "env-secret-key", cfg.MinIOSecretKey)
	assert.Equal(t, "env-test-bucket", cfg.MinIOBucket)
	assert.Equal(t, 100, cfg.BatchSize)
}

// BenchmarkIntegrationBackup benchmarks the backup process
func BenchmarkIntegrationBackup(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping integration benchmark in short mode")
	}

	// Setup (this would be expensive, so normally you'd do this once)
	cfg := &config.Config{
		ClusterName:      "benchmark-cluster",
		MinIOEndpoint:    "localhost:9000", // Assumes MinIO is running
		MinIOAccessKey:   "minioadmin",
		MinIOSecretKey:   "minioadmin123",
		MinIOBucket:      "benchmark-bucket",
		MinIOUseSSL:      false,
		AutoCreateBucket: true,
	}

	backupCfg := &config.BackupConfig{
		FilteringMode:     "whitelist",
		IncludeNamespaces: []string{"default"},
		IncludeResources:  []string{"pods", "services"},
	}

	// Create MinIO client
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		b.Fatalf("Failed to create MinIO client: %v", err)
	}

	mockClients := mocks.NewMockKubernetesClients()
	logger := logging.NewStructuredLogger("benchmark", cfg.ClusterName)
	backupMetrics := metrics.NewBackupMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clusterBackup := backup.NewClusterBackup(
			cfg,
			backupCfg,
			mockClients.KubeClient,
			mockClients.DynamicClient,
			mockClients.DiscoveryClient,
			minioClient,
			logger,
			backupMetrics,
			context.Background(),
		)

		_, err := clusterBackup.ExecuteBackup()
		if err != nil {
			b.Fatalf("Backup failed: %v", err)
		}
	}
}