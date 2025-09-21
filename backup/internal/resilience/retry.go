package resilience

import (
	"context"
	"fmt"
	"math"
	"time"
)

// RetryConfig defines retry behavior configuration
type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Multiplier   float64       `yaml:"multiplier"`
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation func() error

// RetryExecutor handles retry logic with exponential backoff
type RetryExecutor struct {
	config RetryConfig
}

// NewRetryExecutor creates a new retry executor with the given configuration
func NewRetryExecutor(config RetryConfig) *RetryExecutor {
	if config.Multiplier <= 1.0 {
		config.Multiplier = 2.0
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 1
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 1 * time.Second
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	
	return &RetryExecutor{
		config: config,
	}
}

// Execute runs the operation with retry logic and exponential backoff
func (r *RetryExecutor) Execute(operation RetryableOperation) error {
	return r.ExecuteWithContext(context.Background(), operation)
}

// ExecuteWithContext runs the operation with retry logic, respecting context cancellation
func (r *RetryExecutor) ExecuteWithContext(ctx context.Context, operation RetryableOperation) error {
	var lastErr error
	
	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		
		// Don't wait after the last attempt
		if attempt == r.config.MaxAttempts {
			break
		}
		
		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)
		
		// Wait with context cancellation support
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	// All attempts failed
	return &RetryExhaustedError{
		LastError:    lastErr,
		Attempts:     r.config.MaxAttempts,
		TotalElapsed: r.calculateTotalElapsed(),
	}
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (r *RetryExecutor) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: delay = initial_delay * (multiplier ^ (attempt - 1))
	multiplier := math.Pow(r.config.Multiplier, float64(attempt-1))
	delay := time.Duration(float64(r.config.InitialDelay) * multiplier)
	
	// Cap the delay at MaxDelay
	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}
	
	return delay
}

// calculateTotalElapsed calculates the total elapsed time for all retry attempts
func (r *RetryExecutor) calculateTotalElapsed() time.Duration {
	total := time.Duration(0)
	
	for attempt := 1; attempt < r.config.MaxAttempts; attempt++ {
		total += r.calculateDelay(attempt)
	}
	
	return total
}

// RetryExhaustedError is returned when all retry attempts are exhausted
type RetryExhaustedError struct {
	LastError    error
	Attempts     int
	TotalElapsed time.Duration
}

// Error implements the error interface
func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("retry exhausted after %d attempts (elapsed: %v): %v", 
		e.Attempts, e.TotalElapsed, e.LastError)
}

// Unwrap returns the underlying error for error unwrapping
func (e *RetryExhaustedError) Unwrap() error {
	return e.LastError
}

// IsRetryExhaustedError checks if an error is a retry exhausted error
func IsRetryExhaustedError(err error) bool {
	_, ok := err.(*RetryExhaustedError)
	return ok
}

// WithExponentialBackoff is a convenience function for simple retry with exponential backoff
func WithExponentialBackoff(operation RetryableOperation, maxAttempts int, initialDelay time.Duration) error {
	config := RetryConfig{
		MaxAttempts:  maxAttempts,
		InitialDelay: initialDelay,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
	
	executor := NewRetryExecutor(config)
	return executor.Execute(operation)
}

// WithContext is a convenience function for retry with context and exponential backoff
func WithContext(ctx context.Context, operation RetryableOperation, maxAttempts int, initialDelay time.Duration) error {
	config := RetryConfig{
		MaxAttempts:  maxAttempts,
		InitialDelay: initialDelay,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
	
	executor := NewRetryExecutor(config)
	return executor.ExecuteWithContext(ctx, operation)
}