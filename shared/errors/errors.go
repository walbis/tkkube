// Package errors provides standardized error handling patterns for the backup system
package errors

import (
	"fmt"
	"runtime"
	"time"
)

// ErrorCode represents error classification codes
type ErrorCode string

const (
	// Infrastructure errors
	ErrCodeInfrastructure    ErrorCode = "INFRASTRUCTURE"
	ErrCodeNetworkTimeout    ErrorCode = "NETWORK_TIMEOUT"
	ErrCodeNetworkConnection ErrorCode = "NETWORK_CONNECTION"
	ErrCodeStorageTimeout    ErrorCode = "STORAGE_TIMEOUT"
	ErrCodeStorageSpace      ErrorCode = "STORAGE_SPACE"
	ErrCodeKubernetesAPI     ErrorCode = "KUBERNETES_API"

	// Configuration errors
	ErrCodeConfiguration  ErrorCode = "CONFIGURATION"
	ErrCodeValidation     ErrorCode = "VALIDATION"
	ErrCodePermission     ErrorCode = "PERMISSION"
	ErrCodeAuthentication ErrorCode = "AUTHENTICATION"

	// Operation errors
	ErrCodeBackupOperation  ErrorCode = "BACKUP_OPERATION"
	ErrCodeRestoreOperation ErrorCode = "RESTORE_OPERATION"
	ErrCodeRetryExhausted   ErrorCode = "RETRY_EXHAUSTED"
	ErrCodeCircuitBreaker   ErrorCode = "CIRCUIT_BREAKER"
	ErrCodeResourceLimit    ErrorCode = "RESOURCE_LIMIT"

	// Data errors
	ErrCodeDataCorruption ErrorCode = "DATA_CORRUPTION"
	ErrCodeDataFormat     ErrorCode = "DATA_FORMAT"
	ErrCodeDataValidation ErrorCode = "DATA_VALIDATION"

	// System errors
	ErrCodeSystemOverload ErrorCode = "SYSTEM_OVERLOAD"
	ErrCodeSystemTimeout  ErrorCode = "SYSTEM_TIMEOUT"
	ErrCodeUnknown        ErrorCode = "UNKNOWN"
)

// Severity represents error severity levels
type Severity string

const (
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

// Context holds structured error context
type Context map[string]interface{}

// StandardError represents a standardized error with rich context
type StandardError struct {
	Code        ErrorCode `json:"code"`
	Message     string    `json:"message"`
	Severity    Severity  `json:"severity"`
	Context     Context   `json:"context,omitempty"`
	Cause       error     `json:"-"` // Original error
	Timestamp   time.Time `json:"timestamp"`
	StackTrace  string    `json:"stack_trace,omitempty"`
	Component   string    `json:"component"`
	Operation   string    `json:"operation"`
	Retryable   bool      `json:"retryable"`
	UserMessage string    `json:"user_message,omitempty"`
}

// Error implements the error interface
func (e *StandardError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for error unwrapping
func (e *StandardError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches the target error
func (e *StandardError) Is(target error) bool {
	if se, ok := target.(*StandardError); ok {
		return e.Code == se.Code
	}
	return false
}

// WithContext adds context to the error
func (e *StandardError) WithContext(key string, value interface{}) *StandardError {
	if e.Context == nil {
		e.Context = make(Context)
	}
	e.Context[key] = value
	return e
}

// WithCause sets the underlying cause
func (e *StandardError) WithCause(cause error) *StandardError {
	e.Cause = cause
	return e
}

// WithUserMessage sets a user-friendly message
func (e *StandardError) WithUserMessage(message string) *StandardError {
	e.UserMessage = message
	return e
}

// New creates a new standardized error
func New(code ErrorCode, component, operation, message string) *StandardError {
	return &StandardError{
		Code:      code,
		Message:   message,
		Severity:  getSeverityForCode(code),
		Timestamp: time.Now(),
		Component: component,
		Operation: operation,
		Retryable: isRetryableCode(code),
		Context:   make(Context),
	}
}

// NewWithCause creates a new standardized error wrapping an existing error
func NewWithCause(code ErrorCode, component, operation, message string, cause error) *StandardError {
	return New(code, component, operation, message).WithCause(cause)
}

// Wrap wraps an existing error with standardized context
func Wrap(err error, code ErrorCode, component, operation, message string) *StandardError {
	return NewWithCause(code, component, operation, message, err)
}

// NewInfrastructureError creates infrastructure-related errors
func NewInfrastructureError(component, operation, message string, cause error) *StandardError {
	return NewWithCause(ErrCodeInfrastructure, component, operation, message, cause)
}

// NewConfigurationError creates configuration-related errors
func NewConfigurationError(component, operation, message string, cause error) *StandardError {
	return NewWithCause(ErrCodeConfiguration, component, operation, message, cause)
}

// NewBackupError creates backup operation errors
func NewBackupError(operation, message string, cause error) *StandardError {
	return NewWithCause(ErrCodeBackupOperation, "backup", operation, message, cause)
}

// NewRestoreError creates restore operation errors
func NewRestoreError(operation, message string, cause error) *StandardError {
	return NewWithCause(ErrCodeRestoreOperation, "restore", operation, message, cause)
}

// NewKubernetesError creates Kubernetes API errors
func NewKubernetesError(operation, message string, cause error) *StandardError {
	return NewWithCause(ErrCodeKubernetesAPI, "kubernetes", operation, message, cause)
}

// NewStorageError creates storage-related errors
func NewStorageError(operation, message string, cause error) *StandardError {
	return NewWithCause(ErrCodeStorageTimeout, "storage", operation, message, cause)
}

// NewValidationError creates validation errors
func NewValidationError(component, field, message string) *StandardError {
	return New(ErrCodeValidation, component, "validation", message).
		WithContext("field", field)
}

// IsCode checks if an error has a specific error code
func IsCode(err error, code ErrorCode) bool {
	if se, ok := err.(*StandardError); ok {
		return se.Code == code
	}
	return false
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	if se, ok := err.(*StandardError); ok {
		return se.Retryable
	}
	return false
}

// GetCode extracts the error code from an error
func GetCode(err error) ErrorCode {
	if se, ok := err.(*StandardError); ok {
		return se.Code
	}
	return ErrCodeUnknown
}

// GetSeverity extracts the severity from an error
func GetSeverity(err error) Severity {
	if se, ok := err.(*StandardError); ok {
		return se.Severity
	}
	return SeverityMedium
}

// AddStackTrace adds stack trace information to the error
func (e *StandardError) AddStackTrace() *StandardError {
	buf := make([]byte, 1024)
	runtime.Stack(buf, false)
	e.StackTrace = string(buf)
	return e
}

// getSeverityForCode maps error codes to default severity levels
func getSeverityForCode(code ErrorCode) Severity {
	switch code {
	case ErrCodeDataCorruption, ErrCodeSystemOverload:
		return SeverityCritical
	case ErrCodeInfrastructure, ErrCodeKubernetesAPI, ErrCodeBackupOperation, ErrCodeRestoreOperation:
		return SeverityHigh
	case ErrCodeNetworkTimeout, ErrCodeStorageTimeout, ErrCodeRetryExhausted:
		return SeverityMedium
	case ErrCodeValidation, ErrCodeDataFormat:
		return SeverityLow
	default:
		return SeverityMedium
	}
}

// isRetryableCode determines if an error code represents a retryable operation
func isRetryableCode(code ErrorCode) bool {
	switch code {
	case ErrCodeNetworkTimeout, ErrCodeNetworkConnection, ErrCodeStorageTimeout,
		ErrCodeKubernetesAPI, ErrCodeSystemTimeout:
		return true
	case ErrCodeValidation, ErrCodeDataCorruption, ErrCodePermission,
		ErrCodeAuthentication, ErrCodeDataFormat:
		return false
	default:
		return false
	}
}

// MultiError holds multiple errors with context
type MultiError struct {
	Errors    []*StandardError `json:"errors"`
	Context   Context          `json:"context,omitempty"`
	Component string           `json:"component"`
	Operation string           `json:"operation"`
}

// Error implements the error interface
func (me *MultiError) Error() string {
	if len(me.Errors) == 0 {
		return "no errors"
	}
	if len(me.Errors) == 1 {
		return me.Errors[0].Error()
	}
	return fmt.Sprintf("multiple errors (%d): %s", len(me.Errors), me.Errors[0].Error())
}

// Add adds an error to the MultiError
func (me *MultiError) Add(err *StandardError) {
	me.Errors = append(me.Errors, err)
}

// HasErrors returns true if there are any errors
func (me *MultiError) HasErrors() bool {
	return len(me.Errors) > 0
}

// ToError returns the MultiError as an error if there are errors, nil otherwise
func (me *MultiError) ToError() error {
	if !me.HasErrors() {
		return nil
	}
	return me
}

// NewMultiError creates a new MultiError
func NewMultiError(component, operation string) *MultiError {
	return &MultiError{
		Errors:    make([]*StandardError, 0),
		Context:   make(Context),
		Component: component,
		Operation: operation,
	}
}
