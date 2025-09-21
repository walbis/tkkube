package triggers

import (
	"context"
	"fmt"
	"math"
	"time"

	sharedconfig "shared-config/config"
)

// RetryConfig defines retry behavior configuration
type RetryConfig struct {
	MaxRetries      int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffFactor   float64
	JitterEnabled   bool
	RetryableErrors []string
}

// RetryHandler provides robust retry mechanisms for trigger operations
type RetryHandler struct {
	config *sharedconfig.SharedConfig
	logger Logger
}

// NewRetryHandler creates a new retry handler
func NewRetryHandler(config *sharedconfig.SharedConfig, logger Logger) *RetryHandler {
	return &RetryHandler{
		config: config,
		logger: logger,
	}
}

// RetryOperation executes an operation with configurable retry logic
func (rh *RetryHandler) RetryOperation(ctx context.Context, operation func() error, retryConfig RetryConfig) error {
	var lastErr error
	
	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay
			delay := rh.calculateBackoffDelay(attempt, retryConfig)
			
			rh.logger.Info("retrying_operation", map[string]interface{}{
				"attempt":       attempt,
				"max_retries":   retryConfig.MaxRetries,
				"delay_seconds": delay.Seconds(),
				"last_error":    lastErr.Error(),
			})
			
			// Wait with context cancellation support
			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled: %v", ctx.Err())
			case <-time.After(delay):
				// Continue with retry
			}
		}
		
		// Execute the operation
		err := operation()
		if err == nil {
			if attempt > 0 {
				rh.logger.Info("operation_succeeded_after_retry", map[string]interface{}{
					"attempts": attempt + 1,
				})
			}
			return nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !rh.isRetryableError(err, retryConfig.RetryableErrors) {
			rh.logger.Error("non_retryable_error", map[string]interface{}{
				"attempt": attempt,
				"error":   err.Error(),
			})
			return fmt.Errorf("non-retryable error: %v", err)
		}
		
		// Log retry attempt
		rh.logger.Error("operation_failed_will_retry", map[string]interface{}{
			"attempt":     attempt,
			"max_retries": retryConfig.MaxRetries,
			"error":       err.Error(),
		})
	}
	
	rh.logger.Error("operation_failed_after_all_retries", map[string]interface{}{
		"max_retries": retryConfig.MaxRetries,
		"final_error": lastErr.Error(),
	})
	
	return fmt.Errorf("operation failed after %d retries: %v", retryConfig.MaxRetries, lastErr)
}

// RetryGitOpsTrigger retries GitOps trigger operations with appropriate backoff
func (rh *RetryHandler) RetryGitOpsTrigger(ctx context.Context, trigger *AutoTrigger, event *BackupCompletionEvent) (*TriggerResult, error) {
	retryConfig := RetryConfig{
		MaxRetries:    rh.config.Pipeline.ErrorHandling.MaxRetries,
		InitialDelay:  rh.config.Pipeline.ErrorHandling.RetryDelay,
		MaxDelay:      5 * time.Minute,
		BackoffFactor: 2.0,
		JitterEnabled: true,
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"temporary failure",
			"network",
			"webhook",
			"process failed",
		},
	}
	
	var result *TriggerResult
	
	err := rh.RetryOperation(ctx, func() error {
		var triggerErr error
		result, triggerErr = trigger.TriggerGitOpsGeneration(ctx, event)
		
		// Consider the operation failed if result is not successful
		if triggerErr != nil {
			return triggerErr
		}
		
		if result != nil && !result.Success {
			return fmt.Errorf("trigger operation unsuccessful: %s", result.Error)
		}
		
		return nil
	}, retryConfig)
	
	return result, err
}

// calculateBackoffDelay calculates the delay for exponential backoff with jitter
func (rh *RetryHandler) calculateBackoffDelay(attempt int, config RetryConfig) time.Duration {
	// Calculate exponential backoff: initialDelay * (backoffFactor ^ (attempt - 1))
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt-1))
	
	// Apply maximum delay limit
	if time.Duration(delay) > config.MaxDelay {
		delay = float64(config.MaxDelay)
	}
	
	// Add jitter to prevent thundering herd
	if config.JitterEnabled {
		// Add up to 25% random jitter
		nanoTime := float64(time.Now().UnixNano() % 1000)
		jitter := delay * 0.25 * (2*nanoTime/1000.0 - 1) / 1000.0
		delay += jitter
		
		// Ensure delay is not negative
		if delay < 0 {
			delay = float64(config.InitialDelay)
		}
	}
	
	return time.Duration(delay)
}

// isRetryableError checks if an error should trigger a retry
func (rh *RetryHandler) isRetryableError(err error, retryableErrors []string) bool {
	if err == nil {
		return false
	}
	
	errorMsg := err.Error()
	
	// Check against configured retryable error patterns
	for _, pattern := range retryableErrors {
		if containsIgnoreCase(errorMsg, pattern) {
			return true
		}
	}
	
	// Default retryable patterns
	defaultRetryablePatterns := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"timeout",
		"temporary failure",
		"network unreachable",
		"no route to host",
		"service unavailable",
		"gateway timeout",
		"too many requests",
		"rate limit",
	}
	
	for _, pattern := range defaultRetryablePatterns {
		if containsIgnoreCase(errorMsg, pattern) {
			return true
		}
	}
	
	return false
}

// CircuitBreaker provides circuit breaker pattern for trigger operations
type CircuitBreaker struct {
	config           *sharedconfig.SharedConfig
	logger           Logger
	failureCount     int
	lastFailureTime  time.Time
	state            CircuitBreakerState
	failureThreshold int
	resetTimeout     time.Duration
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *sharedconfig.SharedConfig, logger Logger) *CircuitBreaker {
	return &CircuitBreaker{
		config:           config,
		logger:           logger,
		failureThreshold: 5, // Open circuit after 5 failures
		resetTimeout:     60 * time.Second, // Try to reset after 1 minute
		state:            CircuitBreakerClosed,
	}
}

// Execute executes an operation through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func() error) error {
	// Check circuit breaker state
	if cb.state == CircuitBreakerOpen {
		// Check if enough time has passed to try again
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.logger.Info("circuit_breaker_half_open", map[string]interface{}{
				"failure_count": cb.failureCount,
				"reset_timeout": cb.resetTimeout.Seconds(),
			})
			cb.state = CircuitBreakerHalfOpen
		} else {
			return fmt.Errorf("circuit breaker is open, operation blocked")
		}
	}
	
	// Execute the operation
	err := operation()
	
	if err != nil {
		// Record failure
		cb.failureCount++
		cb.lastFailureTime = time.Now()
		
		cb.logger.Error("circuit_breaker_operation_failed", map[string]interface{}{
			"failure_count": cb.failureCount,
			"threshold":     cb.failureThreshold,
			"error":         err.Error(),
		})
		
		// Open circuit if threshold exceeded
		if cb.failureCount >= cb.failureThreshold {
			cb.state = CircuitBreakerOpen
			cb.logger.Error("circuit_breaker_opened", map[string]interface{}{
				"failure_count": cb.failureCount,
				"threshold":     cb.failureThreshold,
			})
		} else if cb.state == CircuitBreakerHalfOpen {
			// Reset to open on failure in half-open state
			cb.state = CircuitBreakerOpen
		}
		
		return err
	}
	
	// Operation succeeded
	if cb.state == CircuitBreakerHalfOpen || cb.failureCount > 0 {
		cb.logger.Info("circuit_breaker_reset", map[string]interface{}{
			"previous_failure_count": cb.failureCount,
		})
		cb.failureCount = 0
		cb.state = CircuitBreakerClosed
	}
	
	return nil
}

// ResilientTrigger combines retry logic and circuit breaker for maximum reliability
type ResilientTrigger struct {
	trigger        *AutoTrigger
	retryHandler   *RetryHandler
	circuitBreaker *CircuitBreaker
	config         *sharedconfig.SharedConfig
	logger         Logger
}

// NewResilientTrigger creates a new resilient trigger with retry and circuit breaker
func NewResilientTrigger(config *sharedconfig.SharedConfig, logger Logger) *ResilientTrigger {
	trigger := NewAutoTrigger(config, logger)
	retryHandler := NewRetryHandler(config, logger)
	circuitBreaker := NewCircuitBreaker(config, logger)
	
	return &ResilientTrigger{
		trigger:        trigger,
		retryHandler:   retryHandler,
		circuitBreaker: circuitBreaker,
		config:         config,
		logger:         logger,
	}
}

// TriggerWithResilience triggers GitOps generation with full resilience features
func (rt *ResilientTrigger) TriggerWithResilience(ctx context.Context, event *BackupCompletionEvent) (*TriggerResult, error) {
	rt.logger.Info("resilient_trigger_start", map[string]interface{}{
		"backup_id": event.BackupID,
		"cluster":   event.ClusterName,
	})
	
	var result *TriggerResult
	
	// Execute through circuit breaker and retry handler
	err := rt.circuitBreaker.Execute(ctx, func() error {
		var triggerErr error
		result, triggerErr = rt.retryHandler.RetryGitOpsTrigger(ctx, rt.trigger, event)
		return triggerErr
	})
	
	if err != nil {
		rt.logger.Error("resilient_trigger_failed", map[string]interface{}{
			"backup_id": event.BackupID,
			"error":     err.Error(),
		})
		
		// Return a failed result if we don't have one
		if result == nil {
			result = &TriggerResult{
				Success:   false,
				Timestamp: time.Now(),
				Method:    "resilient_trigger",
				Error:     err.Error(),
			}
		}
	} else {
		rt.logger.Info("resilient_trigger_success", map[string]interface{}{
			"backup_id": event.BackupID,
			"method":    result.Method,
			"duration":  result.Duration,
		})
	}
	
	return result, err
}

// Helper function for case-insensitive string contains
func containsIgnoreCase(str, substr string) bool {
	return len(str) >= len(substr) && 
		(str == substr || 
		 (len(str) > len(substr) && 
		  (str[:len(substr)] == substr || 
		   str[len(str)-len(substr):] == substr ||
		   findSubstring(str, substr))))
}

// findSubstring performs a simple substring search
func findSubstring(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}