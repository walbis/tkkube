package triggers

import (
	"context"
	"fmt"
	"time"

	sharedconfig "shared-config/config"
)

// BackupTriggerIntegration provides integration between backup operations and auto-triggering
type BackupTriggerIntegration struct {
	autoTrigger *AutoTrigger
	config      *sharedconfig.SharedConfig
	logger      Logger
}

// NewBackupTriggerIntegration creates a new backup trigger integration
func NewBackupTriggerIntegration(config *sharedconfig.SharedConfig, logger Logger) *BackupTriggerIntegration {
	return &BackupTriggerIntegration{
		autoTrigger: NewAutoTrigger(config, logger),
		config:      config,
		logger:      logger,
	}
}

// OnBackupComplete handles backup completion events and triggers GitOps generation if configured
func (bti *BackupTriggerIntegration) OnBackupComplete(ctx context.Context, result *BackupResult) error {
	// Check if auto-triggering is enabled
	if !bti.config.Pipeline.Automation.Enabled || !bti.config.Pipeline.Automation.TriggerOnBackupComplete {
		bti.logger.Debug("auto_trigger_skipped", map[string]interface{}{
			"automation_enabled":        bti.config.Pipeline.Automation.Enabled,
			"trigger_on_backup_complete": bti.config.Pipeline.Automation.TriggerOnBackupComplete,
		})
		return nil
	}

	// Create backup completion event
	event := &BackupCompletionEvent{
		BackupID:        generateBackupID(bti.config.Cluster.Name, result.StartTime),
		ClusterName:     bti.config.Cluster.Name,
		Timestamp:       result.EndTime,
		Duration:        result.Duration,
		NamespacesCount: result.NamespacesBackedUp,
		ResourcesCount:  result.ResourcesBackedUp,
		Success:         len(result.Errors) == 0,
		MinIOBucket:     bti.config.Storage.Bucket,
		BackupLocation:  fmt.Sprintf("%s/%s", bti.config.Storage.Bucket, bti.config.Cluster.Name),
		Metadata: map[string]string{
			"backup_tool_version": "1.0.0",
			"cluster_domain":      bti.config.Cluster.Domain,
			"storage_endpoint":    bti.config.Storage.Endpoint,
		},
	}

	// Convert errors to string slice
	if len(result.Errors) > 0 {
		event.Errors = make([]string, len(result.Errors))
		for i, err := range result.Errors {
			event.Errors[i] = err.Error()
		}
	}

	bti.logger.Info("backup_complete_triggering_gitops", map[string]interface{}{
		"backup_id":        event.BackupID,
		"cluster":          event.ClusterName,
		"resources_backed_up": event.ResourcesCount,
		"namespaces_backed_up": event.NamespacesCount,
		"success":          event.Success,
		"duration_seconds": event.Duration.Seconds(),
	})

	// Trigger GitOps generation
	triggerResult, err := bti.autoTrigger.TriggerGitOpsGeneration(ctx, event)
	if err != nil {
		bti.logger.Error("gitops_trigger_failed", map[string]interface{}{
			"backup_id": event.BackupID,
			"error":     err.Error(),
			"method":    triggerResult.Method,
		})

		// Check if we should continue on error
		if bti.config.Pipeline.ErrorHandling.ContinueOnError {
			bti.logger.Info("continuing_despite_trigger_failure", map[string]interface{}{
				"backup_id": event.BackupID,
			})
			return nil
		}

		return fmt.Errorf("failed to trigger GitOps generation: %v", err)
	}

	bti.logger.Info("gitops_trigger_successful", map[string]interface{}{
		"backup_id":       event.BackupID,
		"trigger_method":  triggerResult.Method,
		"trigger_duration": triggerResult.Duration.Seconds(),
		"gitops_output":   triggerResult.Output,
	})

	return nil
}

// BackupResult represents the result of a backup operation (matches backup tool structure)
type BackupResult struct {
	NamespacesBackedUp int
	ResourcesBackedUp  int
	Errors             []error
	Duration           time.Duration
	StartTime          time.Time
	EndTime            time.Time
}

// generateBackupID creates a unique backup identifier
func generateBackupID(clusterName string, startTime time.Time) string {
	return fmt.Sprintf("%s-%d", clusterName, startTime.Unix())
}

// BackupTriggerWrapper wraps an existing backup function to add auto-triggering
func (bti *BackupTriggerIntegration) BackupTriggerWrapper(ctx context.Context, backupFunc func(ctx context.Context) (*BackupResult, error)) (*BackupResult, error) {
	// Execute the original backup function
	result, err := backupFunc(ctx)
	if err != nil {
		bti.logger.Error("backup_failed", map[string]interface{}{
			"error": err.Error(),
		})
		return result, err
	}

	// If backup was successful, trigger GitOps generation
	if triggerErr := bti.OnBackupComplete(ctx, result); triggerErr != nil {
		bti.logger.Error("backup_trigger_failed", map[string]interface{}{
			"backup_error":  err,
			"trigger_error": triggerErr.Error(),
		})

		// Return trigger error only if continue-on-error is disabled
		if !bti.config.Pipeline.ErrorHandling.ContinueOnError {
			return result, triggerErr
		}
	}

	return result, err
}

// WaitForBackupCompletion waits for backup completion signals
func (bti *BackupTriggerIntegration) WaitForBackupCompletion(ctx context.Context, timeout time.Duration) (*BackupCompletionEvent, error) {
	if !bti.config.Pipeline.Automation.WaitForBackup {
		return nil, fmt.Errorf("wait for backup is disabled")
	}

	bti.logger.Info("waiting_for_backup_completion", map[string]interface{}{
		"timeout_seconds": timeout.Seconds(),
	})

	// Create a context with timeout
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// For now, we'll check for signal files in the trigger directory
	// This could be extended to support other mechanisms like message queues
	triggerDir := "/tmp/backup-gitops-triggers"
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("timeout waiting for backup completion")
		case <-ticker.C:
			// Check for signal files
			if event, found := bti.checkForSignalFiles(triggerDir); found {
				bti.logger.Info("backup_completion_detected", map[string]interface{}{
					"backup_id": event.BackupID,
					"cluster":   event.ClusterName,
				})
				return event, nil
			}
		}
	}
}

// checkForSignalFiles checks for backup completion signal files
func (bti *BackupTriggerIntegration) checkForSignalFiles(triggerDir string) (*BackupCompletionEvent, bool) {
	// This is a simplified implementation
	// In a production system, you might want to use inotify or similar mechanisms
	// for more efficient file system monitoring
	
	// For now, return false as this would require file system scanning
	// In practice, this would be implemented with proper file watching
	return nil, false
}

// MonitorBackupSignals continuously monitors for backup completion signals
func (bti *BackupTriggerIntegration) MonitorBackupSignals(ctx context.Context) error {
	bti.logger.Info("starting_backup_signal_monitor", map[string]interface{}{
		"cluster": bti.config.Cluster.Name,
	})

	for {
		select {
		case <-ctx.Done():
			bti.logger.Info("backup_signal_monitor_stopping", map[string]interface{}{})
			return ctx.Err()
		default:
			// Wait for backup completion with configured timeout
			timeout := time.Duration(bti.config.Pipeline.Automation.MaxWaitTime) * time.Second
			event, err := bti.WaitForBackupCompletion(ctx, timeout)
			if err != nil {
				bti.logger.Error("backup_wait_failed", map[string]interface{}{
					"error": err.Error(),
				})
				time.Sleep(30 * time.Second) // Wait before retrying
				continue
			}

			// Trigger GitOps generation
			_, triggerErr := bti.autoTrigger.TriggerGitOpsGeneration(ctx, event)
			if triggerErr != nil {
				bti.logger.Error("gitops_trigger_from_monitor_failed", map[string]interface{}{
					"backup_id": event.BackupID,
					"error":     triggerErr.Error(),
				})
			}
		}
	}
}