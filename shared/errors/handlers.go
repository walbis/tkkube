package errors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// ErrorHandler provides centralized error handling and logging
type ErrorHandler struct {
	component string
	logger    Logger
}

// Logger interface for error logging
type Logger interface {
	Error(operation, message string, data map[string]interface{})
	ErrorWithErr(operation, message string, data map[string]interface{}, err error)
	Warn(operation, message string, data map[string]interface{})
	Info(operation, message string, data map[string]interface{})
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(component string, logger Logger) *ErrorHandler {
	return &ErrorHandler{
		component: component,
		logger:    logger,
	}
}

// Handle processes and logs a standardized error
func (eh *ErrorHandler) Handle(err error, operation string) *StandardError {
	// Convert to StandardError if needed
	stdErr := eh.ToStandardError(err, operation)

	// Log the error based on severity
	eh.logError(stdErr)

	return stdErr
}

// ToStandardError converts any error to a StandardError
func (eh *ErrorHandler) ToStandardError(err error, operation string) *StandardError {
	if err == nil {
		return nil
	}

	// Already a StandardError
	if stdErr, ok := err.(*StandardError); ok {
		return stdErr
	}

	// Create new StandardError
	return Wrap(err, ErrCodeUnknown, eh.component, operation, "unexpected error")
}

// logError logs the error based on its severity
func (eh *ErrorHandler) logError(err *StandardError) {
	if err == nil {
		return
	}

	data := map[string]interface{}{
		"error_code": string(err.Code),
		"severity":   string(err.Severity),
		"retryable":  err.Retryable,
		"timestamp":  err.Timestamp.Format(time.RFC3339),
		"context":    err.Context,
	}

	switch err.Severity {
	case SeverityCritical, SeverityHigh:
		eh.logger.ErrorWithErr(err.Operation, err.Message, data, err.Cause)
	case SeverityMedium:
		eh.logger.Warn(err.Operation, err.Message, data)
	case SeverityLow:
		eh.logger.Info(err.Operation, err.Message, data)
	}
}

// HandleWithContext handles an error with additional context
func (eh *ErrorHandler) HandleWithContext(err error, operation string, ctx Context) *StandardError {
	stdErr := eh.Handle(err, operation)
	if stdErr != nil {
		for k, v := range ctx {
			stdErr.WithContext(k, v)
		}
	}
	return stdErr
}

// HandleAndReturn handles an error and returns it (for error returns)
func (eh *ErrorHandler) HandleAndReturn(err error, operation string) error {
	stdErr := eh.Handle(err, operation)
	if stdErr == nil {
		return nil
	}
	return stdErr
}

// ErrorRecovery provides error recovery patterns
type ErrorRecovery struct {
	handler *ErrorHandler
}

// NewErrorRecovery creates a new error recovery handler
func NewErrorRecovery(handler *ErrorHandler) *ErrorRecovery {
	return &ErrorRecovery{handler: handler}
}

// SafeExecute executes a function with panic recovery
func (er *ErrorRecovery) SafeExecute(operation string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			panicErr := New(ErrCodeUnknown, er.handler.component, operation, fmt.Sprintf("panic recovered: %v", r))
			panicErr.AddStackTrace()
			er.handler.Handle(panicErr, operation)
			err = panicErr // Return the panic as an error
		}
	}()

	return fn()
}

// WithTimeout executes a function with timeout handling
func (er *ErrorRecovery) WithTimeout(ctx context.Context, operation string, timeout time.Duration, fn func() error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- er.SafeExecute(operation, fn)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		timeoutErr := New(ErrCodeSystemTimeout, er.handler.component, operation,
			fmt.Sprintf("operation timed out after %v", timeout))
		return er.handler.Handle(timeoutErr, operation)
	}
}

// ErrorReporter provides error reporting and metrics
type ErrorReporter struct {
	handler *ErrorHandler
	metrics map[ErrorCode]int
}

// NewErrorReporter creates a new error reporter
func NewErrorReporter(handler *ErrorHandler) *ErrorReporter {
	return &ErrorReporter{
		handler: handler,
		metrics: make(map[ErrorCode]int),
	}
}

// Report reports an error and updates metrics
func (er *ErrorReporter) Report(err error, operation string) *StandardError {
	stdErr := er.handler.Handle(err, operation)
	if stdErr != nil {
		er.metrics[stdErr.Code]++
	}
	return stdErr
}

// GetMetrics returns error metrics
func (er *ErrorReporter) GetMetrics() map[ErrorCode]int {
	result := make(map[ErrorCode]int)
	for k, v := range er.metrics {
		result[k] = v
	}
	return result
}

// ResetMetrics resets error metrics
func (er *ErrorReporter) ResetMetrics() {
	er.metrics = make(map[ErrorCode]int)
}

// ValidationHelper provides validation error utilities
type ValidationHelper struct {
	component string
}

// NewValidationHelper creates a new validation helper
func NewValidationHelper(component string) *ValidationHelper {
	return &ValidationHelper{component: component}
}

// Required validates that a field is not empty
func (vh *ValidationHelper) Required(field, value string) *StandardError {
	if value == "" {
		return NewValidationError(vh.component, field, fmt.Sprintf("%s is required", field))
	}
	return nil
}

// Range validates that a value is within a range
func (vh *ValidationHelper) Range(field string, value, min, max int) *StandardError {
	if value < min || value > max {
		return NewValidationError(vh.component, field,
			fmt.Sprintf("%s must be between %d and %d, got %d", field, min, max, value))
	}
	return nil
}

// ValidateConfig validates a configuration object
func (vh *ValidationHelper) ValidateConfig(config interface{}) *MultiError {
	multiErr := NewMultiError(vh.component, "config_validation")

	// Use reflection or specific validation logic here
	// This is a placeholder for configuration validation

	return multiErr
}

// ErrorFormatter provides error formatting utilities
type ErrorFormatter struct{}

// NewErrorFormatter creates a new error formatter
func NewErrorFormatter() *ErrorFormatter {
	return &ErrorFormatter{}
}

// ToJSON converts an error to JSON format
func (ef *ErrorFormatter) ToJSON(err error) ([]byte, error) {
	if stdErr, ok := err.(*StandardError); ok {
		return json.MarshalIndent(stdErr, "", "  ")
	}

	// Create a simple error representation for non-StandardErrors
	simple := map[string]interface{}{
		"message": err.Error(),
		"type":    "unknown",
	}
	return json.MarshalIndent(simple, "", "  ")
}

// ToUserFriendly converts an error to a user-friendly message
func (ef *ErrorFormatter) ToUserFriendly(err error) string {
	if stdErr, ok := err.(*StandardError); ok && stdErr.UserMessage != "" {
		return stdErr.UserMessage
	}

	// Provide generic user-friendly messages based on error codes
	if stdErr, ok := err.(*StandardError); ok {
		switch stdErr.Code {
		case ErrCodeNetworkTimeout:
			return "Network connection timed out. Please try again."
		case ErrCodePermission:
			return "Permission denied. Please check your access rights."
		case ErrCodeValidation:
			return "Invalid input provided. Please check your data and try again."
		case ErrCodeBackupOperation:
			return "Backup operation failed. Please contact support if this persists."
		case ErrCodeStorageSpace:
			return "Insufficient storage space available."
		default:
			return "An unexpected error occurred. Please try again or contact support."
		}
	}

	return "An error occurred. Please try again."
}

// DefaultLogger provides a default logger implementation
type DefaultLogger struct{}

// Error implements Logger interface
func (dl *DefaultLogger) Error(operation, message string, data map[string]interface{}) {
	log.Printf("[ERROR] %s: %s - %v", operation, message, data)
}

// ErrorWithErr implements Logger interface
func (dl *DefaultLogger) ErrorWithErr(operation, message string, data map[string]interface{}, err error) {
	log.Printf("[ERROR] %s: %s - %v - Cause: %v", operation, message, data, err)
}

// Warn implements Logger interface
func (dl *DefaultLogger) Warn(operation, message string, data map[string]interface{}) {
	log.Printf("[WARN] %s: %s - %v", operation, message, data)
}

// Info implements Logger interface
func (dl *DefaultLogger) Info(operation, message string, data map[string]interface{}) {
	log.Printf("[INFO] %s: %s - %v", operation, message, data)
}
