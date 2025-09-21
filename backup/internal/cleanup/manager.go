package cleanup

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/minio/minio-go/v7"

	"cluster-backup/internal/config"
	"cluster-backup/internal/logging"
	"cluster-backup/internal/metrics"
)

// Manager handles cleanup operations for old backup files
type Manager struct {
	config      *config.Config
	minioClient *minio.Client
	logger      *logging.StructuredLogger
	metrics     *metrics.BackupMetrics
	ctx         context.Context
}

// CleanupResult represents the result of a cleanup operation
type CleanupResult struct {
	FilesDeleted  int
	FilesScanned  int
	SpaceFreed    int64
	Errors        []error
	Duration      time.Duration
	StartTime     time.Time
	EndTime       time.Time
}

// NewManager creates a new cleanup manager
func NewManager(
	config *config.Config,
	minioClient *minio.Client,
	logger *logging.StructuredLogger,
	metrics *metrics.BackupMetrics,
	ctx context.Context,
) *Manager {
	return &Manager{
		config:      config,
		minioClient: minioClient,
		logger:      logger,
		metrics:     metrics,
		ctx:         ctx,
	}
}

// PerformCleanup performs cleanup of old backup files based on retention policy
func (cm *Manager) PerformCleanup() (*CleanupResult, error) {
	startTime := time.Now()
	cm.logger.Info("cleanup_start", "Starting backup cleanup operation", map[string]interface{}{
		"retention_days": cm.config.RetentionDays,
		"bucket":         cm.config.MinIOBucket,
	})

	result := &CleanupResult{
		StartTime: startTime,
		Errors:    []error{},
	}

	// Calculate cutoff time for retention
	cutoffTime := time.Now().AddDate(0, 0, -cm.config.RetentionDays)
	cm.logger.Info("cleanup_cutoff", "Cleanup cutoff time calculated", map[string]interface{}{
		"cutoff_time":    cutoffTime.Format(time.RFC3339),
		"retention_days": cm.config.RetentionDays,
	})

	// List all objects in the backup bucket
	objectCh := cm.minioClient.ListObjects(cm.ctx, cm.config.MinIOBucket, minio.ListObjectsOptions{
		Recursive: true,
	})

	var objectsToDelete []string
	var totalSize int64

	for object := range objectCh {
		if object.Err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("error listing object: %v", object.Err))
			continue
		}

		result.FilesScanned++

		// Check if object is older than retention period
		if object.LastModified.Before(cutoffTime) {
			objectsToDelete = append(objectsToDelete, object.Key)
			totalSize += object.Size

			cm.logger.Debug("cleanup_candidate", "Found object candidate for deletion", map[string]interface{}{
				"object_key":      object.Key,
				"last_modified":   object.LastModified.Format(time.RFC3339),
				"size_bytes":      object.Size,
				"age_days":        int(time.Since(object.LastModified).Hours() / 24),
			})
		}
	}

	cm.logger.Info("cleanup_scan_complete", "Completed scanning objects for cleanup", map[string]interface{}{
		"files_scanned":        result.FilesScanned,
		"files_to_delete":      len(objectsToDelete),
		"estimated_space_mb":   totalSize / (1024 * 1024),
	})

	if len(objectsToDelete) == 0 {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		cm.logger.Info("cleanup_complete", "No files to cleanup", map[string]interface{}{
			"files_scanned": result.FilesScanned,
			"duration_ms":   result.Duration.Milliseconds(),
		})
		return result, nil
	}

	// Delete objects in batches for better performance
	deletedCount, failedDeletes := cm.batchDeleteObjects(objectsToDelete)
	result.FilesDeleted = deletedCount
	result.SpaceFreed = totalSize // This is an estimate
	
	// Add any delete errors to the result
	for _, deleteErr := range failedDeletes {
		result.Errors = append(result.Errors, fmt.Errorf("failed to delete object: %s", deleteErr))
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	cm.logger.Info("cleanup_complete", "Completed backup cleanup operation", map[string]interface{}{
		"files_scanned":   result.FilesScanned,
		"files_deleted":   result.FilesDeleted,
		"space_freed_mb":  result.SpaceFreed / (1024 * 1024),
		"error_count":     len(result.Errors),
		"duration_ms":     result.Duration.Milliseconds(),
	})

	return result, nil
}

// batchDeleteObjects deletes objects in batches for better performance
func (cm *Manager) batchDeleteObjects(objectKeys []string) (int, []string) {
	const batchSize = 1000
	deletedCount := 0
	var failedDeletes []string

	// Sort keys for predictable deletion order
	sort.Strings(objectKeys)

	for i := 0; i < len(objectKeys); i += batchSize {
		end := i + batchSize
		if end > len(objectKeys) {
			end = len(objectKeys)
		}

		batch := objectKeys[i:end]
		cm.logger.Debug("cleanup_batch", "Processing deletion batch", map[string]interface{}{
			"batch_start": i,
			"batch_end":   end,
			"batch_size":  len(batch),
		})

		// Create channel for batch deletion
		objectsCh := make(chan minio.ObjectInfo, len(batch))
		for _, key := range batch {
			objectsCh <- minio.ObjectInfo{Key: key}
		}
		close(objectsCh)

		// Perform batch deletion
		ctx, cancel := context.WithTimeout(cm.ctx, 5*time.Minute)
		errorCh := cm.minioClient.RemoveObjects(ctx, cm.config.MinIOBucket, objectsCh, minio.RemoveObjectsOptions{})

		// Process deletion results
		batchDeletedCount := 0
		for removeErr := range errorCh {
			if removeErr.Err != nil {
				failedDeletes = append(failedDeletes, removeErr.ObjectName)
				cm.logger.Warning("cleanup_delete_failed", "Failed to delete object", map[string]interface{}{
					"object_key": removeErr.ObjectName,
					"error":      removeErr.Err.Error(),
				})
			} else {
				batchDeletedCount++
			}
		}

		deletedCount += batchDeletedCount
		cancel()

		cm.logger.Debug("cleanup_batch_complete", "Completed deletion batch", map[string]interface{}{
			"batch_deleted": batchDeletedCount,
			"batch_failed":  len(batch) - batchDeletedCount,
			"total_deleted": deletedCount,
		})
	}

	return deletedCount, failedDeletes
}

// ShouldCleanupOnStartup determines if cleanup should be performed on startup
func (cm *Manager) ShouldCleanupOnStartup() bool {
	return cm.config.EnableCleanup && cm.config.CleanupOnStartup
}

// ShouldCleanupAfterBackup determines if cleanup should be performed after backup
func (cm *Manager) ShouldCleanupAfterBackup() bool {
	return cm.config.EnableCleanup && !cm.config.CleanupOnStartup
}

// GetRetentionInfo returns information about the current retention policy
func (cm *Manager) GetRetentionInfo() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        cm.config.EnableCleanup,
		"retention_days": cm.config.RetentionDays,
		"cleanup_timing": cm.getCleanupTiming(),
		"cutoff_time":    time.Now().AddDate(0, 0, -cm.config.RetentionDays).Format(time.RFC3339),
	}
}

// getCleanupTiming returns a string describing when cleanup runs
func (cm *Manager) getCleanupTiming() string {
	if !cm.config.EnableCleanup {
		return "disabled"
	}
	
	if cm.config.CleanupOnStartup {
		return "on_startup"
	}
	
	return "after_backup"
}

// EstimateCleanupImpact estimates how many files would be deleted without actually deleting them
func (cm *Manager) EstimateCleanupImpact() (*CleanupEstimate, error) {
	cutoffTime := time.Now().AddDate(0, 0, -cm.config.RetentionDays)
	
	objectCh := cm.minioClient.ListObjects(cm.ctx, cm.config.MinIOBucket, minio.ListObjectsOptions{
		Recursive: true,
	})

	estimate := &CleanupEstimate{
		CutoffTime: cutoffTime,
	}

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("error listing object for estimate: %v", object.Err)
		}

		estimate.TotalFiles++
		estimate.TotalSize += object.Size

		if object.LastModified.Before(cutoffTime) {
			estimate.FilesToDelete++
			estimate.SpaceToFree += object.Size
			
			// Track oldest file
			if estimate.OldestFile.IsZero() || object.LastModified.Before(estimate.OldestFile) {
				estimate.OldestFile = object.LastModified
			}
		} else {
			// Track newest file to keep
			if estimate.NewestFileToKeep.IsZero() || object.LastModified.After(estimate.NewestFileToKeep) {
				estimate.NewestFileToKeep = object.LastModified
			}
		}
	}

	return estimate, nil
}

// CleanupEstimate provides information about what would be cleaned up
type CleanupEstimate struct {
	TotalFiles         int
	FilesToDelete      int
	TotalSize          int64
	SpaceToFree        int64
	CutoffTime         time.Time
	OldestFile         time.Time
	NewestFileToKeep   time.Time
}

// GetSummary returns a human-readable summary of the cleanup estimate
func (ce *CleanupEstimate) GetSummary() map[string]interface{} {
	retentionDays := int(time.Since(ce.CutoffTime).Hours() / 24)
	
	summary := map[string]interface{}{
		"total_files":           ce.TotalFiles,
		"files_to_delete":       ce.FilesToDelete,
		"files_to_keep":         ce.TotalFiles - ce.FilesToDelete,
		"total_size_mb":         ce.TotalSize / (1024 * 1024),
		"space_to_free_mb":      ce.SpaceToFree / (1024 * 1024),
		"retention_days":        retentionDays,
		"cutoff_time":           ce.CutoffTime.Format(time.RFC3339),
	}
	
	if !ce.OldestFile.IsZero() {
		summary["oldest_file_age_days"] = int(time.Since(ce.OldestFile).Hours() / 24)
	}
	
	if !ce.NewestFileToKeep.IsZero() {
		summary["newest_file_to_keep_age_days"] = int(time.Since(ce.NewestFileToKeep).Hours() / 24)
	}
	
	return summary
}

// ValidateRetentionPolicy validates that the retention policy makes sense
func (cm *Manager) ValidateRetentionPolicy() error {
	if cm.config.RetentionDays <= 0 {
		return fmt.Errorf("retention days must be positive, got %d", cm.config.RetentionDays)
	}
	
	if cm.config.RetentionDays > 3650 { // 10 years
		return fmt.Errorf("retention days seems too high, got %d (max recommended: 3650)", cm.config.RetentionDays)
	}
	
	return nil
}