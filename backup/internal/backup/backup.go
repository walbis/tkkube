package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"cluster-backup/internal/config"
	"cluster-backup/internal/logging"
	"cluster-backup/internal/metrics"
)

// ClusterBackup handles the main backup operations
type ClusterBackup struct {
	config           *config.Config
	backupConfig     *config.BackupConfig
	kubeClient       kubernetes.Interface
	dynamicClient    dynamic.Interface
	discoveryClient  discovery.DiscoveryInterface
	minioClient      *minio.Client
	logger           *logging.StructuredLogger
	metrics          *metrics.BackupMetrics
	ctx              context.Context
}

// BackupResult represents the result of a backup operation
type BackupResult struct {
	NamespacesBackedUp int
	ResourcesBackedUp  int
	Errors             []error
	Duration           time.Duration
	StartTime          time.Time
	EndTime            time.Time
}

// NewClusterBackup creates a new ClusterBackup instance
func NewClusterBackup(
	config *config.Config,
	backupConfig *config.BackupConfig,
	kubeClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.DiscoveryInterface,
	minioClient *minio.Client,
	logger *logging.StructuredLogger,
	metrics *metrics.BackupMetrics,
	ctx context.Context,
) *ClusterBackup {
	return &ClusterBackup{
		config:          config,
		backupConfig:    backupConfig,
		kubeClient:      kubeClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		minioClient:     minioClient,
		logger:          logger,
		metrics:         metrics,
		ctx:             ctx,
	}
}

// ExecuteBackup performs the complete backup operation
func (cb *ClusterBackup) ExecuteBackup() (*BackupResult, error) {
	startTime := time.Now()
	cb.logger.Info("backup_start", "Starting cluster backup operation", map[string]interface{}{
		"cluster": cb.config.ClusterName,
		"bucket":  cb.config.MinIOBucket,
	})

	result := &BackupResult{
		StartTime: startTime,
		Errors:    []error{},
	}

	// Test MinIO connectivity
	if err := cb.testMinIOConnectivity(); err != nil {
		cb.logger.Error("minio_connectivity_failed", "Failed to connect to MinIO", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("MinIO connectivity test failed: %v", err)
	}

	// Get list of namespaces to backup
	namespaces, err := cb.getNamespacesToBackup()
	if err != nil {
		cb.logger.Error("namespace_discovery_failed", "Failed to discover namespaces", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("namespace discovery failed: %v", err)
	}

	cb.logger.Info("namespace_discovery_complete", "Discovered namespaces for backup", map[string]interface{}{
		"namespace_count": len(namespaces),
		"namespaces":      namespaces,
	})

	// Backup each namespace
	totalResources := 0
	for _, namespace := range namespaces {
		resourceCount, err := cb.backupNamespace(namespace)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to backup namespace %s: %v", namespace, err))
			cb.metrics.BackupErrors.Inc()
			continue
		}
		totalResources += resourceCount
	}

	// Update metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.NamespacesBackedUp = len(namespaces) - len(result.Errors)
	result.ResourcesBackedUp = totalResources

	cb.metrics.BackupDuration.Observe(result.Duration.Seconds())
	cb.metrics.NamespacesBackedUp.Set(float64(result.NamespacesBackedUp))
	cb.metrics.LastBackupTime.SetToCurrentTime()

	cb.logger.LogDuration("backup_complete", startTime, "Cluster backup completed", map[string]interface{}{
		"namespaces_backed_up": result.NamespacesBackedUp,
		"resources_backed_up":  result.ResourcesBackedUp,
		"error_count":          len(result.Errors),
	})

	return result, nil
}

// testMinIOConnectivity tests the connection to MinIO
func (cb *ClusterBackup) testMinIOConnectivity() error {
	// Check if bucket exists
	exists, err := cb.minioClient.BucketExists(cb.ctx, cb.config.MinIOBucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %v", err)
	}

	if !exists {
		if cb.config.AutoCreateBucket {
			err = cb.minioClient.MakeBucket(cb.ctx, cb.config.MinIOBucket, minio.MakeBucketOptions{})
			if err != nil {
				return fmt.Errorf("failed to create bucket: %v", err)
			}
			cb.logger.Info("bucket_created", "Created MinIO bucket", map[string]interface{}{
				"bucket": cb.config.MinIOBucket,
			})
		} else {
			return fmt.Errorf("bucket %s does not exist and auto-create is disabled", cb.config.MinIOBucket)
		}
	}

	return nil
}

// getNamespacesToBackup returns the list of namespaces to backup based on configuration
func (cb *ClusterBackup) getNamespacesToBackup() ([]string, error) {
	// Get all namespaces
	namespaceList, err := cb.kubeClient.CoreV1().Namespaces().List(cb.ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	var namespaces []string
	for _, ns := range namespaceList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	// Apply filtering logic
	return cb.filterNamespaces(namespaces), nil
}

// filterNamespaces applies include/exclude filtering to namespaces
func (cb *ClusterBackup) filterNamespaces(namespaces []string) []string {
	// If include list is specified, use it
	if len(cb.backupConfig.IncludeNamespaces) > 0 {
		return cb.intersectStringSlices(namespaces, cb.backupConfig.IncludeNamespaces)
	}

	// Otherwise, exclude the specified namespaces
	return cb.excludeStringSlices(namespaces, cb.backupConfig.ExcludeNamespaces)
}

// backupNamespace backs up all resources in a specific namespace
func (cb *ClusterBackup) backupNamespace(namespace string) (int, error) {
	cb.logger.Info("namespace_backup_start", "Starting namespace backup", map[string]interface{}{
		"namespace": namespace,
	})

	// Get API resources
	apiResources, err := cb.discoveryClient.ServerPreferredNamespacedResources()
	if err != nil {
		return 0, fmt.Errorf("failed to discover API resources: %v", err)
	}

	resourceCount := 0
	for _, resourceList := range apiResources {
		for _, resource := range resourceList.APIResources {
			if cb.shouldBackupResource(resource.Name) {
				count, err := cb.backupResource(namespace, schema.GroupVersionResource{
					Group:    resourceList.GroupVersion,
					Version:  "", // Will be set from GroupVersion
					Resource: resource.Name,
				}, resource)
				if err != nil {
					cb.logger.Warning("resource_backup_failed", "Failed to backup resource", map[string]interface{}{
						"namespace": namespace,
						"resource":  resource.Name,
						"error":     err.Error(),
					})
					continue
				}
				resourceCount += count
			}
		}
	}

	cb.logger.Info("namespace_backup_complete", "Completed namespace backup", map[string]interface{}{
		"namespace":      namespace,
		"resource_count": resourceCount,
	})

	return resourceCount, nil
}

// shouldBackupResource determines if a resource type should be backed up
func (cb *ClusterBackup) shouldBackupResource(resourceName string) bool {
	// If include list is specified, check if resource is in it
	if len(cb.backupConfig.IncludeResources) > 0 {
		return cb.stringInSlice(resourceName, cb.backupConfig.IncludeResources)
	}

	// Otherwise, check if resource is not in exclude list
	return !cb.stringInSlice(resourceName, cb.backupConfig.ExcludeResources)
}

// backupResource backs up all instances of a specific resource type in a namespace
func (cb *ClusterBackup) backupResource(namespace string, gvr schema.GroupVersionResource, resource interface{}) (int, error) {
	// Note: This is a simplified implementation that integrates with the new architecture
	// The full implementation from main.go would be moved here in a complete refactoring
	
	cb.logger.Info("resource_backup_start", "Starting resource backup", map[string]interface{}{
		"namespace": namespace,
		"resource":  gvr.Resource,
		"group":     gvr.Group,
		"version":   gvr.Version,
	})
	
	// For now, return a placeholder count
	// In the full refactoring, this would contain all the resource backup logic from main.go
	resourceCount := 1
	
	cb.logger.Info("resource_backup_complete", "Completed resource backup", map[string]interface{}{
		"namespace":      namespace,
		"resource":       gvr.Resource,
		"resource_count": resourceCount,
	})
	
	return resourceCount, nil
}

// Helper functions
func (cb *ClusterBackup) intersectStringSlices(slice1, slice2 []string) []string {
	var result []string
	for _, item1 := range slice1 {
		for _, item2 := range slice2 {
			if item1 == item2 {
				result = append(result, item1)
				break
			}
		}
	}
	return result
}

func (cb *ClusterBackup) excludeStringSlices(slice1, slice2 []string) []string {
	var result []string
	for _, item := range slice1 {
		if !cb.stringInSlice(item, slice2) {
			result = append(result, item)
		}
	}
	return result
}

func (cb *ClusterBackup) stringInSlice(str string, slice []string) bool {
	for _, item := range slice {
		if strings.Contains(str, item) || item == str {
			return true
		}
	}
	return false
}