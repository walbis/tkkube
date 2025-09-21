package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sharedconfig "shared-config/config"
	httplib "shared-config/http"
)

// OptimizedAutoTrigger is an enhanced version of AutoTrigger with optimized HTTP client
type OptimizedAutoTrigger struct {
	config     *sharedconfig.SharedConfig
	logger     Logger
	httpManager *httplib.HTTPClientManager
}

// NewOptimizedAutoTrigger creates a new optimized auto-trigger instance
func NewOptimizedAutoTrigger(config *sharedconfig.SharedConfig, logger Logger) *OptimizedAutoTrigger {
	httpManager := httplib.NewHTTPClientManager(config)
	
	return &OptimizedAutoTrigger{
		config:     config,
		logger:     logger,
		httpManager: httpManager,
	}
}

// TriggerGitOpsGeneration triggers GitOps generation with optimized HTTP client
func (oat *OptimizedAutoTrigger) TriggerGitOpsGeneration(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()
	
	oat.logger.Info("optimized_auto_trigger_start", map[string]interface{}{
		"backup_id":    event.BackupID,
		"cluster":      event.ClusterName,
		"method":       oat.config.Pipeline.Automation.TriggerOnBackupComplete,
		"enabled":      oat.config.Pipeline.Automation.Enabled,
	})

	if !oat.config.Pipeline.Automation.Enabled {
		oat.logger.Info("optimized_auto_trigger_disabled", map[string]interface{}{
			"backup_id": event.BackupID,
		})
		return &TriggerResult{
			Success:   true,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    "disabled",
			Output:    "Auto-trigger is disabled",
		}, nil
	}

	// Get HTTP client metrics before operation
	preMetrics := oat.httpManager.GetMetrics()
	
	var result *TriggerResult
	var err error

	// Try different trigger methods in order of preference
	triggerMethods := oat.getTriggerMethods()

	for _, method := range triggerMethods {
		oat.logger.Debug("trying_optimized_trigger_method", map[string]interface{}{
			"method":    string(method),
			"backup_id": event.BackupID,
		})

		switch method {
		case TriggerTypeFile:
			result, err = oat.triggerViaFile(ctx, event)
		case TriggerTypeWebhook:
			result, err = oat.triggerViaOptimizedWebhook(ctx, event)
		case TriggerTypeProcess:
			result, err = oat.triggerViaProcess(ctx, event)
		case TriggerTypeScript:
			result, err = oat.triggerViaScript(ctx, event)
		default:
			continue
		}

		if err == nil && result.Success {
			// Log HTTP client metrics after successful operation
			postMetrics := oat.httpManager.GetMetrics()
			oat.logHTTPMetrics(preMetrics, postMetrics)
			
			oat.logger.Info("optimized_auto_trigger_success", map[string]interface{}{
				"method":       string(method),
				"backup_id":    event.BackupID,
				"duration":     result.Duration,
				"gitops_output": result.Output,
			})
			return result, nil
		}

		oat.logger.Error("optimized_auto_trigger_method_failed", map[string]interface{}{
			"method":    string(method),
			"backup_id": event.BackupID,
			"error":     err,
		})
	}

	// If all methods failed, return the last error
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    triggerMethods[len(triggerMethods)-1],
			Error:     err.Error(),
		}, err
	}

	return result, nil
}

// triggerViaOptimizedWebhook sends webhook using optimized HTTP client
func (oat *OptimizedAutoTrigger) triggerViaOptimizedWebhook(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	startTime := time.Now()

	webhookURL := oat.config.Pipeline.Notifications.Webhook.URL
	if webhookURL == "" {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     "Webhook URL not configured",
		}, fmt.Errorf("webhook URL not configured")
	}

	// Prepare webhook payload
	payload := map[string]interface{}{
		"event_type": "backup_complete",
		"timestamp":  event.Timestamp,
		"backup":     event,
		"trigger":    "auto_gitops_optimized",
	}

	// Get optimized webhook client
	webhookClient := oat.httpManager.GetWebhookClient()

	// Send webhook request using optimized client
	resp, err := webhookClient.PostJSON(ctx, webhookURL, payload)
	if err != nil {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     fmt.Sprintf("Optimized webhook request failed: %v", err),
		}, err
	}
	defer resp.Body.Close()

	// Read response body with size limits (handled by optimized client)
	responseBody := make([]byte, 4096) // Read up to 4KB of response
	n, _ := resp.Body.Read(responseBody)
	responseData := string(responseBody[:n])

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &TriggerResult{
			Success:   false,
			Timestamp: startTime,
			Duration:  time.Since(startTime),
			Method:    TriggerTypeWebhook,
			Error:     fmt.Sprintf("Optimized webhook returned status %d: %s", resp.StatusCode, responseData),
			Metadata: map[string]string{
				"webhook_url":     webhookURL,
				"response_status": fmt.Sprintf("%d", resp.StatusCode),
				"http_metrics":    oat.getHTTPMetricsString(),
			},
		}, fmt.Errorf("webhook failed with status %d", resp.StatusCode)
	}

	return &TriggerResult{
		Success:   true,
		Timestamp: startTime,
		Duration:  time.Since(startTime),
		Method:    TriggerTypeWebhook,
		Output:    fmt.Sprintf("Optimized webhook sent successfully (status %d): %s", resp.StatusCode, responseData),
		Metadata: map[string]string{
			"webhook_url":     webhookURL,
			"response_status": fmt.Sprintf("%d", resp.StatusCode),
			"http_metrics":    oat.getHTTPMetricsString(),
		},
	}, nil
}

// triggerViaFile reuses the original file-based triggering
func (oat *OptimizedAutoTrigger) triggerViaFile(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	// Delegate to original implementation
	originalTrigger := &AutoTrigger{
		config: oat.config,
		logger: oat.logger,
	}
	return originalTrigger.triggerViaFile(ctx, event)
}

// triggerViaProcess reuses the original process-based triggering
func (oat *OptimizedAutoTrigger) triggerViaProcess(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	// Delegate to original implementation
	originalTrigger := &AutoTrigger{
		config: oat.config,
		logger: oat.logger,
	}
	return originalTrigger.triggerViaProcess(ctx, event)
}

// triggerViaScript reuses the original script-based triggering
func (oat *OptimizedAutoTrigger) triggerViaScript(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	// Delegate to original implementation
	originalTrigger := &AutoTrigger{
		config: oat.config,
		logger: oat.logger,
	}
	return originalTrigger.triggerViaScript(ctx, event)
}

// getTriggerMethods returns the ordered list of trigger methods to try
func (oat *OptimizedAutoTrigger) getTriggerMethods() []TriggerType {
	methods := []TriggerType{}

	// Add methods based on configuration availability
	if oat.config.Pipeline.Notifications.Webhook.URL != "" {
		methods = append(methods, TriggerTypeWebhook)
	}

	// Always add process and file methods as fallbacks
	methods = append(methods, TriggerTypeProcess, TriggerTypeScript, TriggerTypeFile)

	return methods
}

// GetHTTPMetrics returns current HTTP client metrics
func (oat *OptimizedAutoTrigger) GetHTTPMetrics() httplib.HTTPPoolMetrics {
	return oat.httpManager.GetMetrics()
}

// GetConnectionStats returns detailed connection statistics
func (oat *OptimizedAutoTrigger) GetConnectionStats() map[string]interface{} {
	return oat.httpManager.GetConnectionStats()
}

// PerformHealthCheck checks the health of all HTTP clients
func (oat *OptimizedAutoTrigger) PerformHealthCheck(ctx context.Context) map[string]error {
	return oat.httpManager.HealthCheck(ctx)
}

// ResetHTTPMetrics resets all HTTP client metrics
func (oat *OptimizedAutoTrigger) ResetHTTPMetrics() {
	oat.httpManager.ResetMetrics()
}

// Close cleanly shuts down the optimized trigger and HTTP clients
func (oat *OptimizedAutoTrigger) Close() error {
	return oat.httpManager.Close()
}

// Helper methods

func (oat *OptimizedAutoTrigger) getHTTPMetricsString() string {
	stats := oat.httpManager.GetConnectionStats()
	metrics, _ := json.Marshal(map[string]interface{}{
		"success_rate":    stats["success_rate"],
		"avg_response_ms": stats["avg_response_time"],
		"retry_rate":      stats["retry_rate"],
		"circuit_breaker_rate": stats["circuit_breaker_rate"],
	})
	return string(metrics)
}

func (oat *OptimizedAutoTrigger) logHTTPMetrics(preMetrics, postMetrics httplib.HTTPPoolMetrics) {
	deltaRequests := postMetrics.AggregatedMetrics.TotalRequests - preMetrics.AggregatedMetrics.TotalRequests
	deltaSuccess := postMetrics.AggregatedMetrics.SuccessfulReqs - preMetrics.AggregatedMetrics.SuccessfulReqs
	deltaRetries := postMetrics.AggregatedMetrics.RetryAttempts - preMetrics.AggregatedMetrics.RetryAttempts
	
	if deltaRequests > 0 {
		oat.logger.Info("http_client_performance", map[string]interface{}{
			"requests_made":    deltaRequests,
			"successful_reqs":  deltaSuccess,
			"retry_attempts":   deltaRetries,
			"avg_response_ms":  postMetrics.AggregatedMetrics.AvgResponseTime.Milliseconds(),
			"total_clients":    postMetrics.TotalClients,
		})
	}
}

// OptimizedResilientTrigger combines the optimized HTTP client with retry/circuit breaker patterns
type OptimizedResilientTrigger struct {
	optimizedTrigger *OptimizedAutoTrigger
	retryHandler     *RetryHandler
	circuitBreaker   *CircuitBreaker
	config           *sharedconfig.SharedConfig
	logger           Logger
}

// NewOptimizedResilientTrigger creates a new optimized resilient trigger
func NewOptimizedResilientTrigger(config *sharedconfig.SharedConfig, logger Logger) *OptimizedResilientTrigger {
	optimizedTrigger := NewOptimizedAutoTrigger(config, logger)
	retryHandler := NewRetryHandler(config, logger)
	circuitBreaker := NewCircuitBreaker(config, logger)

	return &OptimizedResilientTrigger{
		optimizedTrigger: optimizedTrigger,
		retryHandler:     retryHandler,
		circuitBreaker:   circuitBreaker,
		config:           config,
		logger:           logger,
	}
}

// TriggerWithResilience triggers GitOps generation with full resilience and HTTP optimization
func (ort *OptimizedResilientTrigger) TriggerWithResilience(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	ort.logger.Info("optimized_resilient_trigger_start", map[string]interface{}{
		"backup_id": event.BackupID,
		"cluster":   event.ClusterName,
	})

	var result *TriggerResult

	// Execute through circuit breaker and retry handler with optimized HTTP client
	err := ort.circuitBreaker.Execute(ctx, func() error {
		var triggerErr error
		result, triggerErr = ort.optimizedTrigger.TriggerGitOpsGeneration(ctx, event)
		
		// Consider the operation failed if result is not successful
		if triggerErr != nil {
			return triggerErr
		}
		
		if result != nil && !result.Success {
			return fmt.Errorf("optimized trigger operation unsuccessful: %s", result.Error)
		}
		
		return nil
	})

	if err != nil {
		ort.logger.Error("optimized_resilient_trigger_failed", map[string]interface{}{
			"backup_id": event.BackupID,
			"error":     err.Error(),
		})

		// Return a failed result if we don't have one
		if result == nil {
			result = &TriggerResult{
				Success:   false,
				Timestamp: time.Now(),
				Method:    "optimized_resilient_trigger",
				Error:     err.Error(),
			}
		}
	} else {
		// Log HTTP performance metrics on success
		httpStats := ort.optimizedTrigger.GetConnectionStats()
		ort.logger.Info("optimized_resilient_trigger_success", map[string]interface{}{
			"backup_id":       event.BackupID,
			"method":          result.Method,
			"duration":        result.Duration,
			"http_success_rate": httpStats["success_rate"],
			"avg_response_ms": httpStats["avg_response_time"],
		})
	}

	return result, err
}

// GetHTTPMetrics returns comprehensive HTTP client metrics
func (ort *OptimizedResilientTrigger) GetHTTPMetrics() httplib.HTTPPoolMetrics {
	return ort.optimizedTrigger.GetHTTPMetrics()
}

// GetConnectionStats returns detailed connection statistics
func (ort *OptimizedResilientTrigger) GetConnectionStats() map[string]interface{} {
	return ort.optimizedTrigger.GetConnectionStats()
}

// PerformHealthCheck checks the health of all components
func (ort *OptimizedResilientTrigger) PerformHealthCheck(ctx context.Context) map[string]error {
	return ort.optimizedTrigger.PerformHealthCheck(ctx)
}

// Close cleanly shuts down all components
func (ort *OptimizedResilientTrigger) Close() error {
	return ort.optimizedTrigger.Close()
}