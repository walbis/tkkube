package triggers

import (
	"context"
	"fmt"
	"time"

	"shared-config/monitoring"
	sharedconfig "shared-config/config"
)

// MonitoredAutoTrigger wraps AutoTrigger with monitoring capabilities
type MonitoredAutoTrigger struct {
	trigger          *AutoTrigger
	resilientTrigger *ResilientTrigger
	metricsCollector monitoring.MetricsCollector
	logger           monitoring.Logger
	eventPublisher   monitoring.EventPublisher
}

// NewMonitoredAutoTrigger creates a monitored auto trigger
func NewMonitoredAutoTrigger(config *sharedconfig.SharedConfig, logger monitoring.Logger) *MonitoredAutoTrigger {
	// Create the base trigger with the monitoring logger
	trigger := NewAutoTrigger(config, &LoggerAdapter{logger: logger})
	
	// Create resilient trigger for enhanced reliability
	resilientTrigger := NewResilientTrigger(config, &LoggerAdapter{logger: logger})
	
	monitoringConfig := &monitoring.MonitoringConfig{
		MetricsEnabled: true,
		EventsEnabled:  true,
	}
	
	return &MonitoredAutoTrigger{
		trigger:          trigger,
		resilientTrigger: resilientTrigger,
		metricsCollector: monitoring.NewMetricsCollector(monitoringConfig),
		logger:           logger.WithFields(map[string]interface{}{"component": "auto_trigger"}),
		eventPublisher:   monitoring.NewEventPublisher(monitoringConfig, logger),
	}
}

// LoggerAdapter adapts the monitoring.Logger to the triggers.Logger interface
type LoggerAdapter struct {
	logger monitoring.Logger
}

func (la *LoggerAdapter) Info(message string, fields map[string]interface{}) {
	la.logger.Info(message, fields)
}

func (la *LoggerAdapter) Error(message string, fields map[string]interface{}) {
	la.logger.Error(message, fields)
}

func (la *LoggerAdapter) Debug(message string, fields map[string]interface{}) {
	la.logger.Debug(message, fields)
}

// Implement MonitoredComponent interface

func (mat *MonitoredAutoTrigger) GetComponentName() string {
	return "auto_trigger"
}

func (mat *MonitoredAutoTrigger) GetComponentVersion() string {
	return "1.0.0"
}

func (mat *MonitoredAutoTrigger) GetMetrics() map[string]interface{} {
	metrics := mat.metricsCollector.GetMetrics()
	
	result := make(map[string]interface{})
	
	// Aggregate trigger metrics
	var totalTriggers, successfulTriggers, failedTriggers float64
	var totalDuration time.Duration
	var retryAttempts float64
	
	methodMetrics := make(map[string]map[string]float64)
	
	for _, metric := range metrics {
		switch metric.Name {
		case monitoring.TriggerExecutionDuration:
			if duration, ok := metric.Metadata["unit"].(string); ok && duration == "milliseconds" {
				totalDuration += time.Duration(metric.Value) * time.Millisecond
			}
		case monitoring.TriggerSuccessTotal:
			successfulTriggers += metric.Value
			if method, ok := metric.Labels["method"]; ok {
				if methodMetrics[method] == nil {
					methodMetrics[method] = make(map[string]float64)
				}
				methodMetrics[method]["success"] += metric.Value
			}
		case monitoring.TriggerFailuresTotal:
			failedTriggers += metric.Value
			if method, ok := metric.Labels["method"]; ok {
				if methodMetrics[method] == nil {
					methodMetrics[method] = make(map[string]float64)
				}
				methodMetrics[method]["failures"] += metric.Value
			}
		case monitoring.TriggerRetryTotal:
			retryAttempts += metric.Value
		}
	}
	
	totalTriggers = successfulTriggers + failedTriggers
	
	result["total_triggers"] = totalTriggers
	result["successful_triggers"] = successfulTriggers
	result["failed_triggers"] = failedTriggers
	result["retry_attempts"] = retryAttempts
	result["method_metrics"] = methodMetrics
	
	if totalTriggers > 0 {
		result["success_rate"] = successfulTriggers / totalTriggers
		result["avg_duration_ms"] = totalDuration.Milliseconds() / int64(totalTriggers)
	}
	
	return result
}

func (mat *MonitoredAutoTrigger) ResetMetrics() {
	mat.metricsCollector.ResetMetrics()
}

func (mat *MonitoredAutoTrigger) HealthCheck(ctx context.Context) monitoring.HealthStatus {
	// Trigger system health is based on recent success rate and response times
	metrics := mat.GetMetrics()
	
	var status monitoring.HealthStatusType
	var message string
	
	totalTriggers := metrics["total_triggers"].(float64)
	
	if totalTriggers == 0 {
		status = monitoring.HealthStatusHealthy
		message = "Auto trigger system ready, no triggers executed yet"
	} else {
		successRate := metrics["success_rate"].(float64)
		retryAttempts := metrics["retry_attempts"].(float64)
		
		if successRate < 0.5 { // Less than 50% success rate
			status = monitoring.HealthStatusUnhealthy
			message = fmt.Sprintf("Low trigger success rate: %.1f%%", successRate*100)
		} else if successRate < 0.8 || retryAttempts/totalTriggers > 2 { // Less than 80% success or high retry rate
			status = monitoring.HealthStatusDegraded
			message = fmt.Sprintf("Degraded trigger performance: %.1f%% success, %.1f avg retries", 
				successRate*100, retryAttempts/totalTriggers)
		} else {
			status = monitoring.HealthStatusHealthy
			message = fmt.Sprintf("Trigger system performing well: %.1f%% success rate", successRate*100)
		}
	}
	
	return monitoring.HealthStatus{
		Status:    status,
		Message:   message,
		LastCheck: time.Now(),
		Metrics: map[string]interface{}{
			"total_triggers":  totalTriggers,
			"success_rate":    metrics["success_rate"],
			"retry_attempts":  metrics["retry_attempts"],
		},
	}
}

func (mat *MonitoredAutoTrigger) GetDependencies() []string {
	return []string{"gitops_binary", "pipeline_scripts", "webhook_endpoints", "filesystem"}
}

func (mat *MonitoredAutoTrigger) OnStart(ctx context.Context) error {
	mat.logger.Info("auto_trigger_starting", nil)
	
	event := monitoring.CreateSystemStartEvent(mat.GetComponentName())
	mat.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

func (mat *MonitoredAutoTrigger) OnStop(ctx context.Context) error {
	mat.logger.Info("auto_trigger_stopping", nil)
	
	event := monitoring.CreateSystemStopEvent(mat.GetComponentName())
	mat.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

// Monitored trigger methods

func (mat *MonitoredAutoTrigger) TriggerGitOpsGeneration(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	start := time.Now()
	triggerID := fmt.Sprintf("trigger_%d", time.Now().UnixNano())
	
	mat.logger.Info("trigger_gitops_start", map[string]interface{}{
		"trigger_id": triggerID,
		"backup_id":  event.BackupID,
		"cluster":    event.ClusterName,
	})
	
	// Record trigger attempt
	labels := map[string]string{
		"cluster": event.ClusterName,
	}
	mat.metricsCollector.IncCounter("trigger_attempts_total", labels, 1)
	
	// Publish trigger start event
	startEvent := monitoring.SystemEvent{
		ID:        fmt.Sprintf("evt_%s_start", triggerID),
		Timestamp: time.Now(),
		Type:      "trigger_gitops_start",
		Component: mat.GetComponentName(),
		Action:    "trigger_gitops",
		Status:    "started",
		Metadata: map[string]interface{}{
			"trigger_id": triggerID,
			"backup_id":  event.BackupID,
			"cluster":    event.ClusterName,
		},
	}
	mat.eventPublisher.PublishSystemEvent(startEvent)
	
	// Publish business event for backup completion triggering GitOps
	businessEvent := monitoring.BusinessEvent{
		ID:          fmt.Sprintf("biz_%s", triggerID),
		Timestamp:   time.Now(),
		Type:        "gitops_trigger_initiated",
		BusinessID:  event.BackupID,
		Description: "GitOps generation triggered by backup completion",
		Impact:      "positive",
		Tags: map[string]string{
			"cluster":    event.ClusterName,
			"trigger_id": triggerID,
		},
		Metadata: map[string]interface{}{
			"backup_duration_seconds": event.Duration.Seconds(),
			"resources_count":         event.ResourcesCount,
			"namespaces_count":        event.NamespacesCount,
		},
	}
	mat.eventPublisher.PublishBusinessEvent(businessEvent)
	
	// Use resilient trigger for enhanced reliability
	result, err := mat.resilientTrigger.TriggerWithResilience(ctx, event)
	
	duration := time.Since(start)
	
	// Record metrics based on results
	if err != nil || (result != nil && !result.Success) {
		mat.logger.Error("trigger_gitops_failed", map[string]interface{}{
			"trigger_id":  triggerID,
			"backup_id":   event.BackupID,
			"duration_ms": duration.Milliseconds(),
			"error":       getErrorString(err, result),
			"method":      getMethodString(result),
		})
		
		labels["status"] = "failed"
		if result != nil {
			labels["method"] = string(result.Method)
		}
		mat.metricsCollector.IncCounter(monitoring.TriggerFailuresTotal, labels, 1)
		mat.metricsCollector.RecordDuration(monitoring.TriggerExecutionDuration, labels, duration)
		
		// Publish error event
		errorEvent := monitoring.CreateErrorEvent(mat.GetComponentName(), "trigger_gitops", 
			getErrorFromResult(err, result), "error")
		errorEvent.Context["trigger_id"] = triggerID
		errorEvent.Context["backup_id"] = event.BackupID
		errorEvent.Context["duration_ms"] = duration.Milliseconds()
		if result != nil {
			errorEvent.Context["method"] = string(result.Method)
			errorEvent.Context["output"] = result.Output
		}
		mat.eventPublisher.PublishErrorEvent(errorEvent)
		
	} else {
		mat.logger.Info("trigger_gitops_success", map[string]interface{}{
			"trigger_id":  triggerID,
			"backup_id":   event.BackupID,
			"duration_ms": duration.Milliseconds(),
			"method":      string(result.Method),
			"output":      result.Output,
		})
		
		labels["status"] = "success"
		labels["method"] = string(result.Method)
		mat.metricsCollector.IncCounter(monitoring.TriggerSuccessTotal, labels, 1)
		mat.metricsCollector.RecordDuration(monitoring.TriggerExecutionDuration, labels, duration)
		
		// Publish business event for successful GitOps generation
		successEvent := monitoring.BusinessEvent{
			ID:          fmt.Sprintf("biz_%s_success", triggerID),
			Timestamp:   time.Now(),
			Type:        "gitops_generation_completed",
			BusinessID:  event.BackupID,
			Description: "GitOps manifests generated successfully from backup",
			Impact:      "positive",
			Tags: map[string]string{
				"cluster":    event.ClusterName,
				"method":     string(result.Method),
				"trigger_id": triggerID,
			},
			Metrics: map[string]float64{
				"duration_seconds": duration.Seconds(),
			},
		}
		mat.eventPublisher.PublishBusinessEvent(successEvent)
	}
	
	// Publish completion event
	completeEvent := monitoring.SystemEvent{
		ID:        fmt.Sprintf("evt_%s_complete", triggerID),
		Timestamp: time.Now(),
		Type:      "trigger_gitops_complete",
		Component: mat.GetComponentName(),
		Action:    "trigger_gitops",
		Status:    getEventStatus(err, result),
		Duration:  duration,
		Metadata: map[string]interface{}{
			"trigger_id": triggerID,
			"backup_id":  event.BackupID,
			"success":    err == nil && result != nil && result.Success,
		},
	}
	if result != nil {
		completeEvent.Metadata["method"] = string(result.Method)
	}
	mat.eventPublisher.PublishSystemEvent(completeEvent)
	
	return result, err
}

// Method-specific monitoring hooks

func (mat *MonitoredAutoTrigger) TriggerViaFileWithMonitoring(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	return mat.triggerWithMethodMonitoring(ctx, event, TriggerTypeFile, func() (*TriggerResult, error) {
		return mat.trigger.triggerViaFile(ctx, event)
	})
}

func (mat *MonitoredAutoTrigger) TriggerViaWebhookWithMonitoring(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	return mat.triggerWithMethodMonitoring(ctx, event, TriggerTypeWebhook, func() (*TriggerResult, error) {
		return mat.trigger.triggerViaWebhook(ctx, event)
	})
}

func (mat *MonitoredAutoTrigger) TriggerViaProcessWithMonitoring(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	return mat.triggerWithMethodMonitoring(ctx, event, TriggerTypeProcess, func() (*TriggerResult, error) {
		return mat.trigger.triggerViaProcess(ctx, event)
	})
}

func (mat *MonitoredAutoTrigger) TriggerViaScriptWithMonitoring(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	return mat.triggerWithMethodMonitoring(ctx, event, TriggerTypeScript, func() (*TriggerResult, error) {
		return mat.trigger.triggerViaScript(ctx, event)
	})
}

// triggerWithMethodMonitoring wraps individual trigger methods with monitoring
func (mat *MonitoredAutoTrigger) triggerWithMethodMonitoring(ctx context.Context, event *BackupCompletionEvent, 
	method TriggerType, triggerFunc func() (*TriggerResult, error)) (*TriggerResult, error) {
	
	start := time.Now()
	methodID := fmt.Sprintf("method_%s_%d", method, time.Now().UnixNano())
	
	mat.logger.Debug("trigger_method_start", map[string]interface{}{
		"method_id":  methodID,
		"method":     string(method),
		"backup_id":  event.BackupID,
	})
	
	// Record method attempt
	labels := map[string]string{
		"method":  string(method),
		"cluster": event.ClusterName,
	}
	mat.metricsCollector.IncCounter("trigger_method_attempts_total", labels, 1)
	
	// Publish method start event
	startEvent := monitoring.SystemEvent{
		ID:        fmt.Sprintf("evt_%s_start", methodID),
		Timestamp: time.Now(),
		Type:      "trigger_method_start",
		Component: mat.GetComponentName(),
		Action:    fmt.Sprintf("trigger_via_%s", method),
		Status:    "started",
		Metadata: map[string]interface{}{
			"method_id": methodID,
			"method":    string(method),
			"backup_id": event.BackupID,
		},
	}
	mat.eventPublisher.PublishSystemEvent(startEvent)
	
	// Execute the trigger method
	result, err := triggerFunc()
	
	duration := time.Since(start)
	
	// Record method-specific metrics
	if err != nil || (result != nil && !result.Success) {
		mat.logger.Error("trigger_method_failed", map[string]interface{}{
			"method_id":   methodID,
			"method":      string(method),
			"backup_id":   event.BackupID,
			"duration_ms": duration.Milliseconds(),
			"error":       getErrorString(err, result),
		})
		
		labels["status"] = "failed"
		mat.metricsCollector.IncCounter("trigger_method_failures_total", labels, 1)
		
		// Publish method error event
		errorEvent := monitoring.CreateErrorEvent(mat.GetComponentName(), 
			fmt.Sprintf("trigger_via_%s", method), getErrorFromResult(err, result), "warning")
		errorEvent.Context["method_id"] = methodID
		errorEvent.Context["method"] = string(method)
		errorEvent.Context["backup_id"] = event.BackupID
		mat.eventPublisher.PublishErrorEvent(errorEvent)
		
	} else {
		mat.logger.Debug("trigger_method_success", map[string]interface{}{
			"method_id":   methodID,
			"method":      string(method),
			"backup_id":   event.BackupID,
			"duration_ms": duration.Milliseconds(),
		})
		
		labels["status"] = "success"
		mat.metricsCollector.IncCounter("trigger_method_success_total", labels, 1)
	}
	
	mat.metricsCollector.RecordDuration("trigger_method_duration_seconds", labels, duration)
	
	// Publish method completion event
	completeEvent := monitoring.SystemEvent{
		ID:        fmt.Sprintf("evt_%s_complete", methodID),
		Timestamp: time.Now(),
		Type:      "trigger_method_complete",
		Component: mat.GetComponentName(),
		Action:    fmt.Sprintf("trigger_via_%s", method),
		Status:    getEventStatus(err, result),
		Duration:  duration,
		Metadata: map[string]interface{}{
			"method_id": methodID,
			"method":    string(method),
			"backup_id": event.BackupID,
			"success":   err == nil && result != nil && result.Success,
		},
	}
	mat.eventPublisher.PublishSystemEvent(completeEvent)
	
	return result, err
}

// Additional monitoring methods

func (mat *MonitoredAutoTrigger) GetMetricsCollector() monitoring.MetricsCollector {
	return mat.metricsCollector
}

func (mat *MonitoredAutoTrigger) GetEventPublisher() monitoring.EventPublisher {
	return mat.eventPublisher
}

func (mat *MonitoredAutoTrigger) GetLogger() monitoring.Logger {
	return mat.logger
}

// Helper functions

func getErrorString(err error, result *TriggerResult) string {
	if err != nil {
		return err.Error()
	}
	if result != nil && result.Error != "" {
		return result.Error
	}
	return "unknown error"
}

func getMethodString(result *TriggerResult) string {
	if result != nil {
		return string(result.Method)
	}
	return "unknown"
}

func getErrorFromResult(err error, result *TriggerResult) error {
	if err != nil {
		return err
	}
	if result != nil && result.Error != "" {
		return fmt.Errorf(result.Error)
	}
	return fmt.Errorf("trigger failed")
}

func getEventStatus(err error, result *TriggerResult) string {
	if err != nil {
		return "failed"
	}
	if result != nil && result.Success {
		return "success"
	}
	return "failed"
}