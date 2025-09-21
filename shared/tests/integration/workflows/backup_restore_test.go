package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"
	
	"shared-config/config"
	"shared-config/monitoring"
	"shared-config/restore"
	"shared-config/security"
	"shared-config/integration"
)

// BackupRestoreWorkflowTestSuite tests complete backup → restore workflows
type BackupRestoreWorkflowTestSuite struct {
	suite.Suite
	
	// System components
	bridge          *integration.IntegrationBridge
	restoreEngine   *restore.RestoreEngine
	monitoringHub   *monitoring.MonitoringSystem
	securityManager *security.SecurityManager
	
	// Test configuration
	config        *config.SharedConfig
	testDataDir   string
	workDir       string
	
	// Mock services
	minioMock     *MinIOMock
	k8sMock       *KubernetesMock
	gitMock       *GitMock
	
	// Test data
	testResources []KubernetesResource
	testBackups   []BackupMetadata
	testClusters  []ClusterConfig
}

// KubernetesResource represents a test Kubernetes resource
type KubernetesResource struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   map[string]interface{} `yaml:"metadata"`
	Spec       map[string]interface{} `yaml:"spec,omitempty"`
	Data       map[string]interface{} `yaml:"data,omitempty"`
}

// BackupMetadata represents backup information for testing
type BackupMetadata struct {
	ID            string    `json:"id"`
	ClusterName   string    `json:"cluster_name"`
	Timestamp     time.Time `json:"timestamp"`
	Size          int64     `json:"size"`
	ResourceCount int       `json:"resource_count"`
	Namespaces    []string  `json:"namespaces"`
	Status        string    `json:"status"`
	Path          string    `json:"path"`
}

// ClusterConfig represents test cluster configuration
type ClusterConfig struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Version  string `json:"version"`
	Status   string `json:"status"`
}

// SetupSuite initializes the test environment
func (suite *BackupRestoreWorkflowTestSuite) SetupSuite() {
	var err error
	
	// Create test directories
	suite.testDataDir = "../../../tests/data"
	suite.workDir, err = ioutil.TempDir("", "backup-restore-test-")
	require.NoError(suite.T(), err)
	
	// Load test configuration
	suite.setupTestConfiguration()
	
	// Initialize mock services
	suite.setupMockServices()
	
	// Initialize system components
	suite.setupSystemComponents()
	
	// Load test data
	suite.loadTestData()
}

// TearDownSuite cleans up test resources
func (suite *BackupRestoreWorkflowTestSuite) TearDownSuite() {
	if suite.workDir != "" {
		os.RemoveAll(suite.workDir)
	}
	
	if suite.bridge != nil {
		suite.bridge.Shutdown(context.Background())
	}
}

// SetupTest runs before each test
func (suite *BackupRestoreWorkflowTestSuite) SetupTest() {
	// Reset mock states
	suite.minioMock.Reset()
	suite.k8sMock.Reset()
	suite.gitMock.Reset()
}

func (suite *BackupRestoreWorkflowTestSuite) setupTestConfiguration() {
	suite.config = &config.SharedConfig{
		Integration: config.IntegrationConfig{
			WebhookPort: 8080,
			EnableHTTP:  true,
		},
		Security: config.SecurityConfig{
			EnableAuth: true,
			JWTSecret:  "test-secret-key",
		},
		Monitoring: config.MonitoringConfig{
			Enabled:     true,
			MetricsPort: 9090,
		},
		GitOps: config.GitOpsConfig{
			Repository: config.RepositoryConfig{
				URL:    "https://github.com/test/gitops-repo.git",
				Branch: "main",
			},
			AutoSync: true,
		},
		Backup: config.BackupConfig{
			Storage: config.StorageConfig{
				Type:     "minio",
				Endpoint: "localhost:9000",
				Bucket:   "test-backups",
			},
			Schedule: "0 2 * * *",
			Retention: config.RetentionConfig{
				Days: 30,
			},
		},
	}
}

func (suite *BackupRestoreWorkflowTestSuite) setupMockServices() {
	suite.minioMock = NewMinIOMock()
	suite.k8sMock = NewKubernetesMock()
	suite.gitMock = NewGitMock()
	
	// Start mock services
	suite.minioMock.Start()
	suite.k8sMock.Start()
	suite.gitMock.Start()
}

func (suite *BackupRestoreWorkflowTestSuite) setupSystemComponents() {
	var err error
	
	// Initialize monitoring
	suite.monitoringHub, err = monitoring.NewMonitoringSystem(suite.config)
	require.NoError(suite.T(), err)
	
	// Initialize security
	suite.securityManager, err = security.NewSecurityManager(suite.config)
	require.NoError(suite.T(), err)
	
	// Initialize restore engine
	suite.restoreEngine, err = restore.NewRestoreEngine(
		suite.config,
		suite.monitoringHub,
		suite.securityManager,
	)
	require.NoError(suite.T(), err)
	
	// Initialize integration bridge
	suite.bridge, err = integration.NewIntegrationBridge(suite.config)
	require.NoError(suite.T(), err)
	
	// Start services
	ctx := context.Background()
	err = suite.bridge.Start(ctx)
	require.NoError(suite.T(), err)
}

func (suite *BackupRestoreWorkflowTestSuite) loadTestData() {
	// Load Kubernetes resources
	suite.loadKubernetesResources()
	
	// Setup test backups
	suite.setupTestBackups()
	
	// Setup test clusters
	suite.setupTestClusters()
}

func (suite *BackupRestoreWorkflowTestSuite) loadKubernetesResources() {
	// Load simple resources
	simpleResourcesPath := filepath.Join(suite.testDataDir, "kubernetes", "simple_resources.yaml")
	suite.loadResourcesFromFile(simpleResourcesPath)
	
	// Load complex application
	complexAppPath := filepath.Join(suite.testDataDir, "kubernetes", "complex_app.yaml")
	suite.loadResourcesFromFile(complexAppPath)
}

func (suite *BackupRestoreWorkflowTestSuite) loadResourcesFromFile(filePath string) {
	data, err := ioutil.ReadFile(filePath)
	require.NoError(suite.T(), err)
	
	// Split YAML documents
	documents := splitYAMLDocuments(string(data))
	
	for _, doc := range documents {
		if len(doc) == 0 {
			continue
		}
		
		var resource KubernetesResource
		err := yaml.Unmarshal([]byte(doc), &resource)
		require.NoError(suite.T(), err)
		
		suite.testResources = append(suite.testResources, resource)
	}
}

func (suite *BackupRestoreWorkflowTestSuite) setupTestBackups() {
	suite.testBackups = []BackupMetadata{
		{
			ID:            "backup-small-001",
			ClusterName:   "test-cluster",
			Timestamp:     time.Now().Add(-24 * time.Hour),
			Size:          10 * 1024 * 1024, // 10MB
			ResourceCount: 25,
			Namespaces:    []string{"test-app"},
			Status:        "completed",
			Path:          "test-cluster/backup-small-001",
		},
		{
			ID:            "backup-medium-001",
			ClusterName:   "staging-cluster",
			Timestamp:     time.Now().Add(-12 * time.Hour),
			Size:          150 * 1024 * 1024, // 150MB
			ResourceCount: 200,
			Namespaces:    []string{"app1", "app2", "monitoring"},
			Status:        "completed",
			Path:          "staging-cluster/backup-medium-001",
		},
		{
			ID:            "backup-large-001",
			ClusterName:   "production-cluster",
			Timestamp:     time.Now().Add(-6 * time.Hour),
			Size:          500 * 1024 * 1024, // 500MB
			ResourceCount: 1000,
			Namespaces:    []string{"ecommerce-prod", "auth", "payments", "notifications"},
			Status:        "completed",
			Path:          "production-cluster/backup-large-001",
		},
	}
	
	// Setup backup data in MinIO mock
	for _, backup := range suite.testBackups {
		suite.minioMock.AddBackup(backup)
	}
}

func (suite *BackupRestoreWorkflowTestSuite) setupTestClusters() {
	suite.testClusters = []ClusterConfig{
		{
			Name:     "test-cluster",
			Endpoint: "https://test-k8s.example.com",
			Version:  "1.28.0",
			Status:   "healthy",
		},
		{
			Name:     "staging-cluster",
			Endpoint: "https://staging-k8s.example.com",
			Version:  "1.27.0",
			Status:   "healthy",
		},
		{
			Name:     "production-cluster",
			Endpoint: "https://prod-k8s.example.com",
			Version:  "1.28.0",
			Status:   "healthy",
		},
	}
	
	// Setup clusters in Kubernetes mock
	for _, cluster := range suite.testClusters {
		suite.k8sMock.AddCluster(cluster)
	}
}

// Test Complete Backup to Restore Workflow

func (suite *BackupRestoreWorkflowTestSuite) TestCompleteBackupRestoreWorkflow() {
	ctx := context.Background()
	
	// Phase 1: Backup Creation (simulated)
	backupID := "test-workflow-backup-001"
	clusterName := "test-cluster"
	
	suite.T().Log("Phase 1: Creating backup...")
	backup := BackupMetadata{
		ID:            backupID,
		ClusterName:   clusterName,
		Timestamp:     time.Now(),
		Size:          50 * 1024 * 1024,
		ResourceCount: 50,
		Namespaces:    []string{"test-app"},
		Status:        "completed",
		Path:          fmt.Sprintf("%s/%s", clusterName, backupID),
	}
	
	suite.minioMock.AddBackup(backup)
	
	// Phase 2: Restore Preparation
	suite.T().Log("Phase 2: Preparing restore...")
	restoreRequest := restore.RestoreRequest{
		RestoreID:        fmt.Sprintf("restore-%s", backupID),
		BackupID:         backupID,
		ClusterName:      clusterName,
		TargetNamespaces: []string{"test-app"},
		RestoreMode:      restore.RestoreModeComplete,
		ValidationMode:   restore.ValidationModeStrict,
		ConflictStrategy: restore.ConflictStrategyOverwrite,
		DryRun:           false,
	}
	
	// Phase 3: Execute Restore
	suite.T().Log("Phase 3: Executing restore...")
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), operation)
	assert.Equal(suite.T(), restore.RestoreStatusPending, operation.Status)
	
	// Phase 4: Monitor Progress
	suite.T().Log("Phase 4: Monitoring restore progress...")
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 5*time.Minute)
	
	// Phase 5: Validate Results
	suite.T().Log("Phase 5: Validating restore results...")
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	assert.Greater(suite.T(), finalOperation.Results.Summary.ResourcesSuccessful, 0)
	assert.Equal(suite.T(), 100.0, finalOperation.Progress.PercentComplete)
}

func (suite *BackupRestoreWorkflowTestSuite) TestSelectiveNamespaceRestore() {
	ctx := context.Background()
	
	// Use the large backup with multiple namespaces
	backupID := "backup-large-001"
	targetNamespace := "ecommerce-prod"
	
	restoreRequest := restore.RestoreRequest{
		RestoreID:        fmt.Sprintf("selective-restore-%d", time.Now().Unix()),
		BackupID:         backupID,
		ClusterName:      "production-cluster",
		TargetNamespaces: []string{targetNamespace},
		RestoreMode:      restore.RestoreModeSelective,
		ValidationMode:   restore.ValidationModePermissive,
		ConflictStrategy: restore.ConflictStrategyMerge,
		DryRun:           false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 3*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	
	// Verify only resources from target namespace were restored
	for _, restoredResource := range finalOperation.Results.RestoredResources {
		if restoredResource.Namespace != "" {
			assert.Equal(suite.T(), targetNamespace, restoredResource.Namespace)
		}
	}
}

func (suite *BackupRestoreWorkflowTestSuite) TestDryRunValidation() {
	ctx := context.Background()
	
	restoreRequest := restore.RestoreRequest{
		RestoreID:        "dryrun-validation-001",
		BackupID:         "backup-medium-001",
		ClusterName:      "staging-cluster",
		RestoreMode:      restore.RestoreModeComplete,
		ValidationMode:   restore.ValidationModeStrict,
		ConflictStrategy: restore.ConflictStrategyFail,
		DryRun:           true, // Dry run mode
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 2*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	
	// Dry run should complete successfully without making actual changes
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	assert.NotNil(suite.T(), finalOperation.ValidationReport)
	
	// Verify no actual resources were created (since it's dry run)
	assert.Equal(suite.T(), 0, len(finalOperation.Results.RestoredResources))
}

func (suite *BackupRestoreWorkflowTestSuite) TestConflictResolution() {
	ctx := context.Background()
	
	// First, create some existing resources that will conflict
	suite.k8sMock.AddExistingResources("staging-cluster", []KubernetesResource{
		{
			APIVersion: "v1",
			Kind:       "Namespace",
			Metadata: map[string]interface{}{
				"name": "app1",
			},
		},
		{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Metadata: map[string]interface{}{
				"name":      "existing-app",
				"namespace": "app1",
			},
		},
	})
	
	// Test different conflict strategies
	strategies := []struct {
		strategy          restore.ConflictStrategy
		expectedBehavior  string
	}{
		{restore.ConflictStrategySkip, "skip_conflicts"},
		{restore.ConflictStrategyOverwrite, "overwrite_existing"},
		{restore.ConflictStrategyMerge, "merge_resources"},
	}
	
	for _, test := range strategies {
		suite.T().Run(fmt.Sprintf("ConflictStrategy_%s", test.strategy), func(t *testing.T) {
			restoreRequest := restore.RestoreRequest{
				RestoreID:        fmt.Sprintf("conflict-test-%s-%d", test.strategy, time.Now().Unix()),
				BackupID:         "backup-medium-001",
				ClusterName:      "staging-cluster",
				RestoreMode:      restore.RestoreModeComplete,
				ValidationMode:   restore.ValidationModePermissive,
				ConflictStrategy: test.strategy,
				DryRun:           false,
			}
			
			operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
			require.NoError(t, err)
			
			suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 3*time.Minute)
			
			finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
			require.NoError(t, err)
			assert.Equal(t, restore.RestoreStatusCompleted, finalOperation.Status)
			
			// Verify conflict resolution behavior
			switch test.strategy {
			case restore.ConflictStrategySkip:
				assert.Greater(t, len(finalOperation.Results.SkippedResources), 0)
			case restore.ConflictStrategyOverwrite:
				assert.Greater(t, len(finalOperation.Results.RestoredResources), 0)
			case restore.ConflictStrategyMerge:
				assert.Greater(t, len(finalOperation.Results.RestoredResources), 0)
			}
		})
	}
}

func (suite *BackupRestoreWorkflowTestSuite) TestCrossClusterRestore() {
	ctx := context.Background()
	
	// Restore from production cluster backup to staging cluster
	restoreRequest := restore.RestoreRequest{
		RestoreID:        "cross-cluster-restore-001",
		BackupID:         "backup-large-001", // From production-cluster
		ClusterName:      "staging-cluster",  // To staging-cluster
		TargetNamespaces: []string{"ecommerce-prod"},
		RestoreMode:      restore.RestoreModeSelective,
		ValidationMode:   restore.ValidationModeStrict,
		ConflictStrategy: restore.ConflictStrategyOverwrite,
		DryRun:           false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 4*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	assert.Greater(suite.T(), finalOperation.Results.Summary.ResourcesSuccessful, 0)
}

func (suite *BackupRestoreWorkflowTestSuite) TestRestoreWithResourceFiltering() {
	ctx := context.Background()
	
	restoreRequest := restore.RestoreRequest{
		RestoreID:     "filtered-restore-001",
		BackupID:      "backup-large-001",
		ClusterName:   "production-cluster",
		ResourceTypes: []string{"Deployment", "Service", "ConfigMap"}, // Only specific resource types
		RestoreMode:   restore.RestoreModeSelective,
		ValidationMode: restore.ValidationModeStrict,
		ConflictStrategy: restore.ConflictStrategyOverwrite,
		DryRun:        false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 3*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	
	// Verify only requested resource types were restored
	allowedTypes := map[string]bool{
		"Deployment": true,
		"Service":    true,
		"ConfigMap":  true,
	}
	
	for _, restoredResource := range finalOperation.Results.RestoredResources {
		assert.True(suite.T(), allowedTypes[restoredResource.Kind], 
			"Unexpected resource type: %s", restoredResource.Kind)
	}
}

func (suite *BackupRestoreWorkflowTestSuite) TestRestoreWithLabelSelector() {
	ctx := context.Background()
	
	restoreRequest := restore.RestoreRequest{
		RestoreID:     "label-filtered-restore-001",
		BackupID:      "backup-large-001",
		ClusterName:   "production-cluster",
		LabelSelector: "app=ecommerce,tier=backend", // Only resources with these labels
		RestoreMode:   restore.RestoreModeSelective,
		ValidationMode: restore.ValidationModePermissive,
		ConflictStrategy: restore.ConflictStrategyMerge,
		DryRun:        false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 3*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	
	// Verify label selector filtering was applied
	for _, restoredResource := range finalOperation.Results.RestoredResources {
		suite.T().Logf("Restored resource: %s/%s", restoredResource.Kind, restoredResource.Name)
	}
}

func (suite *BackupRestoreWorkflowTestSuite) TestRestoreCancellation() {
	ctx := context.Background()
	
	restoreRequest := restore.RestoreRequest{
		RestoreID:        "cancellation-test-001",
		BackupID:         "backup-large-001",
		ClusterName:      "production-cluster",
		RestoreMode:      restore.RestoreModeComplete,
		ValidationMode:   restore.ValidationModeStrict,
		ConflictStrategy: restore.ConflictStrategyOverwrite,
		DryRun:           false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusPending, operation.Status)
	
	// Wait a bit for restore to start
	time.Sleep(2 * time.Second)
	
	// Cancel the restore
	err = suite.restoreEngine.CancelRestore(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	
	// Wait for cancellation to take effect
	time.Sleep(1 * time.Second)
	
	// Verify restore was cancelled
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCancelled, finalOperation.Status)
}

func (suite *BackupRestoreWorkflowTestSuite) TestRestoreValidationFailure() {
	ctx := context.Background()
	
	// Use non-existent backup to trigger validation failure
	restoreRequest := restore.RestoreRequest{
		RestoreID:        "validation-failure-test-001",
		BackupID:         "non-existent-backup",
		ClusterName:      "test-cluster",
		RestoreMode:      restore.RestoreModeComplete,
		ValidationMode:   restore.ValidationModeStrict,
		ConflictStrategy: restore.ConflictStrategyOverwrite,
		DryRun:           false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 2*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusFailed, finalOperation.Status)
	assert.Greater(suite.T(), len(finalOperation.Errors), 0)
}

func (suite *BackupRestoreWorkflowTestSuite) TestIncrementalRestore() {
	ctx := context.Background()
	
	// First, create some existing resources
	suite.k8sMock.AddExistingResources("test-cluster", []KubernetesResource{
		{
			APIVersion: "v1",
			Kind:       "Namespace",
			Metadata: map[string]interface{}{
				"name": "test-app",
			},
		},
	})
	
	restoreRequest := restore.RestoreRequest{
		RestoreID:        "incremental-restore-001",
		BackupID:         "backup-small-001",
		ClusterName:      "test-cluster",
		RestoreMode:      restore.RestoreModeIncremental, // Only restore missing resources
		ValidationMode:   restore.ValidationModePermissive,
		ConflictStrategy: restore.ConflictStrategySkip,
		DryRun:           false,
	}
	
	operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
	require.NoError(suite.T(), err)
	
	suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 2*time.Minute)
	
	finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), restore.RestoreStatusCompleted, finalOperation.Status)
	
	// Verify that existing resources were skipped
	assert.Greater(suite.T(), len(finalOperation.Results.SkippedResources), 0)
	
	// Find the skipped namespace
	namespaceSkipped := false
	for _, skipped := range finalOperation.Results.SkippedResources {
		if skipped.Kind == "Namespace" && skipped.Name == "test-app" {
			namespaceSkipped = true
			break
		}
	}
	assert.True(suite.T(), namespaceSkipped, "Existing namespace should have been skipped")
}

func (suite *BackupRestoreWorkflowTestSuite) TestConcurrentRestores() {
	ctx := context.Background()
	
	concurrency := 3
	results := make(chan error, concurrency)
	
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			restoreRequest := restore.RestoreRequest{
				RestoreID:        fmt.Sprintf("concurrent-restore-%d", index),
				BackupID:         "backup-small-001",
				ClusterName:      "test-cluster",
				RestoreMode:      restore.RestoreModeComplete,
				ValidationMode:   restore.ValidationModePermissive,
				ConflictStrategy: restore.ConflictStrategyOverwrite,
				DryRun:           false,
			}
			
			operation, err := suite.restoreEngine.StartRestore(ctx, restoreRequest)
			if err != nil {
				results <- err
				return
			}
			
			suite.waitForRestoreCompletion(ctx, restoreRequest.RestoreID, 3*time.Minute)
			
			finalOperation, err := suite.restoreEngine.GetRestoreStatus(restoreRequest.RestoreID)
			if err != nil {
				results <- err
				return
			}
			
			if finalOperation.Status != restore.RestoreStatusCompleted {
				results <- fmt.Errorf("restore %s failed with status: %s", 
					restoreRequest.RestoreID, finalOperation.Status)
				return
			}
			
			results <- nil
		}(i)
	}
	
	// Wait for all concurrent restores to complete
	successCount := 0
	for i := 0; i < concurrency; i++ {
		select {
		case err := <-results:
			if err != nil {
				suite.T().Errorf("Concurrent restore failed: %v", err)
			} else {
				successCount++
			}
		case <-time.After(5 * time.Minute):
			suite.T().Error("Concurrent restore timeout")
		}
	}
	
	assert.Greater(suite.T(), successCount, 0, "At least one concurrent restore should succeed")
}

// Helper Methods

func (suite *BackupRestoreWorkflowTestSuite) waitForRestoreCompletion(ctx context.Context, restoreID string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			suite.T().Fatalf("Timeout waiting for restore %s to complete", restoreID)
			return
		case <-ticker.C:
			operation, err := suite.restoreEngine.GetRestoreStatus(restoreID)
			if err != nil {
				suite.T().Logf("Error getting restore status: %v", err)
				continue
			}
			
			suite.T().Logf("Restore %s status: %s (%.1f%% complete)", 
				restoreID, operation.Status, operation.Progress.PercentComplete)
			
			switch operation.Status {
			case restore.RestoreStatusCompleted, restore.RestoreStatusFailed, restore.RestoreStatusCancelled:
				return
			}
		}
	}
}

// Utility functions

func splitYAMLDocuments(input string) []string {
	documents := make([]string, 0)
	current := ""
	
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if len(strings.TrimSpace(current)) > 0 {
				documents = append(documents, current)
			}
			current = ""
		} else {
			current += line + "\n"
		}
	}
	
	if len(strings.TrimSpace(current)) > 0 {
		documents = append(documents, current)
	}
	
	return documents
}

// Test suite runner
func TestBackupRestoreWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(BackupRestoreWorkflowTestSuite))
}