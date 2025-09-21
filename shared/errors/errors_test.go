package errors

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestStandardError(t *testing.T) {
	tests := []struct {
		name        string
		setupError  func() *StandardError
		expectCode  ErrorCode
		expectMsg   string
		expectRetry bool
	}{
		{
			name: "basic_error",
			setupError: func() *StandardError {
				return New(ErrCodeBackupOperation, "backup", "test_operation", "test message")
			},
			expectCode:  ErrCodeBackupOperation,
			expectMsg:   "[BACKUP_OPERATION] test message",
			expectRetry: false,
		},
		{
			name: "error_with_cause",
			setupError: func() *StandardError {
				cause := errors.New("underlying error")
				return NewWithCause(ErrCodeNetworkTimeout, "network", "connect", "connection failed", cause)
			},
			expectCode:  ErrCodeNetworkTimeout,
			expectMsg:   "[NETWORK_TIMEOUT] connection failed: underlying error",
			expectRetry: true,
		},
		{
			name: "error_with_context",
			setupError: func() *StandardError {
				return New(ErrCodeValidation, "config", "validate", "invalid value").
					WithContext("field", "timeout").
					WithContext("value", 30)
			},
			expectCode:  ErrCodeValidation,
			expectMsg:   "[VALIDATION] invalid value",
			expectRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setupError()

			if err.Code != tt.expectCode {
				t.Errorf("expected code %s, got %s", tt.expectCode, err.Code)
			}

			if err.Error() != tt.expectMsg {
				t.Errorf("expected message '%s', got '%s'", tt.expectMsg, err.Error())
			}

			if err.Retryable != tt.expectRetry {
				t.Errorf("expected retryable %v, got %v", tt.expectRetry, err.Retryable)
			}

			// Test timestamp is recent
			if time.Since(err.Timestamp) > time.Second {
				t.Errorf("timestamp should be recent")
			}
		})
	}
}

func TestErrorHelpers(t *testing.T) {
	t.Run("IsCode", func(t *testing.T) {
		err := NewBackupError("test", "backup failed", nil)
		if !IsCode(err, ErrCodeBackupOperation) {
			t.Errorf("expected IsCode to return true for backup error")
		}
		if IsCode(err, ErrCodeValidation) {
			t.Errorf("expected IsCode to return false for validation error")
		}
	})

	t.Run("IsRetryable", func(t *testing.T) {
		retryableErr := NewKubernetesError("api_call", "API timeout", nil)
		nonRetryableErr := NewValidationError("config", "timeout", "invalid timeout value")

		if !IsRetryable(retryableErr) {
			t.Errorf("expected kubernetes error to be retryable")
		}
		if IsRetryable(nonRetryableErr) {
			t.Errorf("expected validation error to not be retryable")
		}
	})

	t.Run("GetCode", func(t *testing.T) {
		err := NewConfigurationError("config", "load", "failed to load config", nil)
		if GetCode(err) != ErrCodeConfiguration {
			t.Errorf("expected configuration error code")
		}

		regularErr := errors.New("regular error")
		if GetCode(regularErr) != ErrCodeUnknown {
			t.Errorf("expected unknown error code for regular error")
		}
	})

	t.Run("GetSeverity", func(t *testing.T) {
		criticalErr := New(ErrCodeDataCorruption, "storage", "read", "data corrupted")
		if GetSeverity(criticalErr) != SeverityCritical {
			t.Errorf("expected critical severity for data corruption")
		}

		mediumErr := NewValidationError("config", "field", "invalid value")
		if GetSeverity(mediumErr) != SeverityLow {
			t.Errorf("expected low severity for validation error")
		}
	})
}

func TestMultiError(t *testing.T) {
	multiErr := NewMultiError("backup", "multi_operation")

	// Test empty multi error
	if multiErr.HasErrors() {
		t.Errorf("expected no errors initially")
	}
	if multiErr.ToError() != nil {
		t.Errorf("expected ToError to return nil for empty multi error")
	}

	// Add errors
	err1 := NewValidationError("config", "field1", "invalid field1")
	err2 := NewValidationError("config", "field2", "invalid field2")

	multiErr.Add(err1)
	multiErr.Add(err2)

	if !multiErr.HasErrors() {
		t.Errorf("expected errors after adding")
	}
	if multiErr.ToError() == nil {
		t.Errorf("expected ToError to return error when errors present")
	}

	if len(multiErr.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(multiErr.Errors))
	}
}

func TestErrorWrapping(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := Wrap(originalErr, ErrCodeInfrastructure, "component", "operation", "wrapped message")

	// Test unwrapping
	if wrappedErr.Unwrap() != originalErr {
		t.Errorf("expected unwrap to return original error")
	}

	// Test error chain
	if !errors.Is(wrappedErr, originalErr) {
		t.Errorf("expected errors.Is to find original error in chain")
	}
}

func TestSeverityMapping(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected Severity
	}{
		{ErrCodeDataCorruption, SeverityCritical},
		{ErrCodeSystemOverload, SeverityCritical},
		{ErrCodeInfrastructure, SeverityHigh},
		{ErrCodeBackupOperation, SeverityHigh},
		{ErrCodeNetworkTimeout, SeverityMedium},
		{ErrCodeValidation, SeverityLow},
		{ErrCodeUnknown, SeverityMedium},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			severity := getSeverityForCode(tt.code)
			if severity != tt.expected {
				t.Errorf("expected severity %s for code %s, got %s", tt.expected, tt.code, severity)
			}
		})
	}
}

func TestRetryableMapping(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected bool
	}{
		{ErrCodeNetworkTimeout, true},
		{ErrCodeKubernetesAPI, true},
		{ErrCodeStorageTimeout, true},
		{ErrCodeValidation, false},
		{ErrCodeDataCorruption, false},
		{ErrCodePermission, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			retryable := isRetryableCode(tt.code)
			if retryable != tt.expected {
				t.Errorf("expected retryable %v for code %s, got %v", tt.expected, tt.code, retryable)
			}
		})
	}
}

// Benchmark tests
func BenchmarkStandardErrorCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		New(ErrCodeBackupOperation, "backup", "test", "test message")
	}
}

func BenchmarkErrorWithCause(b *testing.B) {
	cause := errors.New("underlying error")
	for i := 0; i < b.N; i++ {
		NewWithCause(ErrCodeNetworkTimeout, "network", "connect", "connection failed", cause)
	}
}

// TestErrorCreationFunctions tests the specific error creation functions with 0% coverage
func TestNewInfrastructureError(t *testing.T) {
	tests := []struct {
		name      string
		component string
		operation string
		message   string
		cause     error
	}{
		{
			name:      "infrastructure_error_with_cause",
			component: "network",
			operation: "connect",
			message:   "failed to connect to database",
			cause:     errors.New("connection refused"),
		},
		{
			name:      "infrastructure_error_without_cause",
			component: "storage",
			operation: "mount",
			message:   "failed to mount volume",
			cause:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewInfrastructureError(tt.component, tt.operation, tt.message, tt.cause)

			// Verify error code
			if err.Code != ErrCodeInfrastructure {
				t.Errorf("expected code %s, got %s", ErrCodeInfrastructure, err.Code)
			}

			// Verify component and operation
			if err.Component != tt.component {
				t.Errorf("expected component %s, got %s", tt.component, err.Component)
			}
			if err.Operation != tt.operation {
				t.Errorf("expected operation %s, got %s", tt.operation, err.Operation)
			}

			// Verify message
			if err.Message != tt.message {
				t.Errorf("expected message %s, got %s", tt.message, err.Message)
			}

			// Verify cause
			if err.Cause != tt.cause {
				t.Errorf("expected cause %v, got %v", tt.cause, err.Cause)
			}

			// Verify severity (infrastructure errors should be high)
			if err.Severity != SeverityHigh {
				t.Errorf("expected severity %s, got %s", SeverityHigh, err.Severity)
			}

			// Verify retryable (infrastructure errors should not be retryable by default)
			if err.Retryable {
				t.Errorf("expected infrastructure error to not be retryable")
			}

			// Test error string format
			errorStr := err.Error()
			if tt.cause != nil {
				expected := fmt.Sprintf("[%s] %s: %v", ErrCodeInfrastructure, tt.message, tt.cause)
				if errorStr != expected {
					t.Errorf("expected error string '%s', got '%s'", expected, errorStr)
				}
			} else {
				expected := fmt.Sprintf("[%s] %s", ErrCodeInfrastructure, tt.message)
				if errorStr != expected {
					t.Errorf("expected error string '%s', got '%s'", expected, errorStr)
				}
			}
		})
	}
}

func TestNewRestoreError(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		message   string
		cause     error
	}{
		{
			name:      "restore_error_with_cause",
			operation: "extract_volume",
			message:   "failed to extract backup volume",
			cause:     errors.New("corrupted archive"),
		},
		{
			name:      "restore_error_without_cause",
			operation: "verify_backup",
			message:   "backup verification failed",
			cause:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRestoreError(tt.operation, tt.message, tt.cause)

			// Verify error code
			if err.Code != ErrCodeRestoreOperation {
				t.Errorf("expected code %s, got %s", ErrCodeRestoreOperation, err.Code)
			}

			// Verify component (should be "restore")
			if err.Component != "restore" {
				t.Errorf("expected component 'restore', got %s", err.Component)
			}

			// Verify operation
			if err.Operation != tt.operation {
				t.Errorf("expected operation %s, got %s", tt.operation, err.Operation)
			}

			// Verify message
			if err.Message != tt.message {
				t.Errorf("expected message %s, got %s", tt.message, err.Message)
			}

			// Verify cause
			if err.Cause != tt.cause {
				t.Errorf("expected cause %v, got %v", tt.cause, err.Cause)
			}

			// Verify severity (restore operations should be high)
			if err.Severity != SeverityHigh {
				t.Errorf("expected severity %s, got %s", SeverityHigh, err.Severity)
			}

			// Verify retryable (restore operations should not be retryable by default)
			if err.Retryable {
				t.Errorf("expected restore error to not be retryable")
			}

			// Test helper function works
			if !IsCode(err, ErrCodeRestoreOperation) {
				t.Errorf("IsCode should return true for restore operation error")
			}
		})
	}
}

func TestNewStorageError(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		message   string
		cause     error
	}{
		{
			name:      "storage_error_with_cause",
			operation: "write_data",
			message:   "failed to write backup data",
			cause:     errors.New("disk full"),
		},
		{
			name:      "storage_error_without_cause",
			operation: "read_metadata",
			message:   "metadata read timeout",
			cause:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewStorageError(tt.operation, tt.message, tt.cause)

			// Verify error code (note: NewStorageError uses ErrCodeStorageTimeout)
			if err.Code != ErrCodeStorageTimeout {
				t.Errorf("expected code %s, got %s", ErrCodeStorageTimeout, err.Code)
			}

			// Verify component (should be "storage")
			if err.Component != "storage" {
				t.Errorf("expected component 'storage', got %s", err.Component)
			}

			// Verify operation
			if err.Operation != tt.operation {
				t.Errorf("expected operation %s, got %s", tt.operation, err.Operation)
			}

			// Verify message
			if err.Message != tt.message {
				t.Errorf("expected message %s, got %s", tt.message, err.Message)
			}

			// Verify cause
			if err.Cause != tt.cause {
				t.Errorf("expected cause %v, got %v", tt.cause, err.Cause)
			}

			// Verify severity (storage timeout should be medium)
			if err.Severity != SeverityMedium {
				t.Errorf("expected severity %s, got %s", SeverityMedium, err.Severity)
			}

			// Verify retryable (storage timeout should be retryable)
			if !err.Retryable {
				t.Errorf("expected storage timeout error to be retryable")
			}

			// Test helper functions
			if !IsCode(err, ErrCodeStorageTimeout) {
				t.Errorf("IsCode should return true for storage timeout error")
			}
			if !IsRetryable(err) {
				t.Errorf("IsRetryable should return true for storage timeout error")
			}
		})
	}
}

func BenchmarkIsCode(b *testing.B) {
	err := NewBackupError("test", "backup failed", nil)
	for i := 0; i < b.N; i++ {
		IsCode(err, ErrCodeBackupOperation)
	}
}
