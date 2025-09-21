package errors

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// MockLogger for testing
type MockLogger struct {
	ErrorCalls    []LogCall
	ErrorErrCalls []LogErrCall
	WarnCalls     []LogCall
	InfoCalls     []LogCall
}

type LogCall struct {
	Operation string
	Message   string
	Data      map[string]interface{}
}

type LogErrCall struct {
	Operation string
	Message   string
	Data      map[string]interface{}
	Err       error
}

func (ml *MockLogger) Error(operation, message string, data map[string]interface{}) {
	ml.ErrorCalls = append(ml.ErrorCalls, LogCall{operation, message, data})
}

func (ml *MockLogger) ErrorWithErr(operation, message string, data map[string]interface{}, err error) {
	ml.ErrorErrCalls = append(ml.ErrorErrCalls, LogErrCall{operation, message, data, err})
}

func (ml *MockLogger) Warn(operation, message string, data map[string]interface{}) {
	ml.WarnCalls = append(ml.WarnCalls, LogCall{operation, message, data})
}

func (ml *MockLogger) Info(operation, message string, data map[string]interface{}) {
	ml.InfoCalls = append(ml.InfoCalls, LogCall{operation, message, data})
}

func TestErrorHandler(t *testing.T) {
	logger := &MockLogger{}
	handler := NewErrorHandler("test_component", logger)

	t.Run("Handle_StandardError", func(t *testing.T) {
		originalErr := NewBackupError("backup_op", "backup failed", nil)
		handledErr := handler.Handle(originalErr, "test_operation")

		if handledErr != originalErr {
			t.Errorf("expected same error instance")
		}

		// Should log as high severity error
		if len(logger.ErrorErrCalls) != 1 {
			t.Errorf("expected 1 error log call, got %d", len(logger.ErrorErrCalls))
		}
	})

	t.Run("Handle_RegularError", func(t *testing.T) {
		logger = &MockLogger{} // Reset
		handler = NewErrorHandler("test_component", logger)

		regularErr := errors.New("regular error")
		handledErr := handler.Handle(regularErr, "test_operation")

		if handledErr.Code != ErrCodeUnknown {
			t.Errorf("expected unknown error code")
		}
		if handledErr.Component != "test_component" {
			t.Errorf("expected test_component")
		}
	})

	t.Run("HandleWithContext", func(t *testing.T) {
		logger = &MockLogger{} // Reset
		handler = NewErrorHandler("test_component", logger)

		regularErr := errors.New("test error")
		ctx := Context{"key1": "value1", "key2": 42}

		handledErr := handler.HandleWithContext(regularErr, "test_op", ctx)

		if handledErr.Context["key1"] != "value1" {
			t.Errorf("expected context to be added")
		}
		if handledErr.Context["key2"] != 42 {
			t.Errorf("expected context to be added")
		}
	})
}

func TestErrorRecovery(t *testing.T) {
	logger := &MockLogger{}
	handler := NewErrorHandler("test_component", logger)
	recovery := NewErrorRecovery(handler)

	t.Run("SafeExecute_Success", func(t *testing.T) {
		err := recovery.SafeExecute("test_op", func() error {
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("SafeExecute_Error", func(t *testing.T) {
		testErr := errors.New("test error")
		err := recovery.SafeExecute("test_op", func() error {
			return testErr
		})

		if err != testErr {
			t.Errorf("expected original error, got %v", err)
		}
	})

	t.Run("SafeExecute_Panic", func(t *testing.T) {
		logger = &MockLogger{} // Reset
		handler = NewErrorHandler("test_component", logger)
		recovery = NewErrorRecovery(handler)

		err := recovery.SafeExecute("test_op", func() error {
			panic("test panic")
		})

		// Should return error (panic is recovered and returned as error)
		if err == nil {
			t.Errorf("expected error after panic recovery")
		}

		// Should be a StandardError with panic details
		stdErr, ok := err.(*StandardError)
		if !ok {
			t.Errorf("expected StandardError from panic recovery")
		}
		if stdErr.Code != ErrCodeUnknown {
			t.Errorf("expected unknown error code for panic")
		}

		// Should have logged the panic (ErrCodeUnknown maps to SeverityMedium, so should be in WarnCalls)
		if len(logger.WarnCalls) == 0 && len(logger.ErrorErrCalls) == 0 {
			t.Errorf("expected panic to be logged, got %d warn calls and %d error calls", len(logger.WarnCalls), len(logger.ErrorErrCalls))
		}
	})

	t.Run("WithTimeout_Success", func(t *testing.T) {
		ctx := context.Background()
		err := recovery.WithTimeout(ctx, "test_op", 100*time.Millisecond, func() error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("WithTimeout_Timeout", func(t *testing.T) {
		logger = &MockLogger{} // Reset
		handler = NewErrorHandler("test_component", logger)
		recovery = NewErrorRecovery(handler)

		ctx := context.Background()
		err := recovery.WithTimeout(ctx, "test_op", 10*time.Millisecond, func() error {
			time.Sleep(50 * time.Millisecond)
			return nil
		})

		if err == nil {
			t.Errorf("expected timeout error")
		}

		stdErr, ok := err.(*StandardError)
		if !ok {
			t.Errorf("expected StandardError")
		}
		if stdErr.Code != ErrCodeSystemTimeout {
			t.Errorf("expected timeout error code")
		}
	})
}

func TestErrorReporter(t *testing.T) {
	logger := &MockLogger{}
	handler := NewErrorHandler("test_component", logger)
	reporter := NewErrorReporter(handler)

	t.Run("Report_and_Metrics", func(t *testing.T) {
		err1 := NewBackupError("op1", "backup failed", nil)
		err2 := NewBackupError("op2", "another backup failed", nil)
		err3 := NewValidationError("config", "field", "validation failed")

		reporter.Report(err1, "test_op1")
		reporter.Report(err2, "test_op2")
		reporter.Report(err3, "test_op3")

		metrics := reporter.GetMetrics()

		if metrics[ErrCodeBackupOperation] != 2 {
			t.Errorf("expected 2 backup operation errors, got %d", metrics[ErrCodeBackupOperation])
		}
		if metrics[ErrCodeValidation] != 1 {
			t.Errorf("expected 1 validation error, got %d", metrics[ErrCodeValidation])
		}
	})

	t.Run("ResetMetrics", func(t *testing.T) {
		reporter.ResetMetrics()
		metrics := reporter.GetMetrics()

		if len(metrics) != 0 {
			t.Errorf("expected empty metrics after reset, got %v", metrics)
		}
	})
}

func TestValidationHelper(t *testing.T) {
	validator := NewValidationHelper("test_component")

	t.Run("Required_Valid", func(t *testing.T) {
		err := validator.Required("username", "john_doe")
		if err != nil {
			t.Errorf("expected no error for valid required field")
		}
	})

	t.Run("Required_Invalid", func(t *testing.T) {
		err := validator.Required("username", "")
		if err == nil {
			t.Errorf("expected error for empty required field")
		}
		if err.Code != ErrCodeValidation {
			t.Errorf("expected validation error code")
		}
	})

	t.Run("Range_Valid", func(t *testing.T) {
		err := validator.Range("timeout", 30, 10, 60)
		if err != nil {
			t.Errorf("expected no error for valid range")
		}
	})

	t.Run("Range_Invalid_Low", func(t *testing.T) {
		err := validator.Range("timeout", 5, 10, 60)
		if err == nil {
			t.Errorf("expected error for value below range")
		}
	})

	t.Run("Range_Invalid_High", func(t *testing.T) {
		err := validator.Range("timeout", 70, 10, 60)
		if err == nil {
			t.Errorf("expected error for value above range")
		}
	})

	t.Run("ValidateConfig", func(t *testing.T) {
		// Test with various config types to ensure the function works
		testCases := []struct {
			name   string
			config interface{}
		}{
			{
				name:   "nil_config",
				config: nil,
			},
			{
				name:   "string_config",
				config: "test_config",
			},
			{
				name:   "map_config",
				config: map[string]interface{}{"key": "value"},
			},
			{
				name:   "struct_config",
				config: struct{ Field string }{Field: "value"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				multiErr := validator.ValidateConfig(tc.config)

				// ValidateConfig should return a MultiError
				if multiErr == nil {
					t.Errorf("expected MultiError, got nil")
					return
				}

				// Verify component and operation are set
				if multiErr.Component != "test_component" {
					t.Errorf("expected component 'test_component', got %s", multiErr.Component)
				}
				if multiErr.Operation != "config_validation" {
					t.Errorf("expected operation 'config_validation', got %s", multiErr.Operation)
				}

				// Since this is a placeholder implementation, it should return empty errors
				if multiErr.HasErrors() {
					t.Errorf("expected no errors for placeholder implementation, got %d errors", len(multiErr.Errors))
				}

				// Test ToError() returns nil for empty MultiError
				if multiErr.ToError() != nil {
					t.Errorf("expected ToError() to return nil for empty MultiError")
				}

				// Test Error() method
				errorStr := multiErr.Error()
				if errorStr != "no errors" {
					t.Errorf("expected 'no errors' message, got '%s'", errorStr)
				}
			})
		}
	})
}

func TestErrorFormatter(t *testing.T) {
	formatter := NewErrorFormatter()

	t.Run("ToJSON_StandardError", func(t *testing.T) {
		err := NewBackupError("backup_op", "backup failed", errors.New("underlying"))
		jsonData, jsonErr := formatter.ToJSON(err)

		if jsonErr != nil {
			t.Errorf("expected no error converting to JSON, got %v", jsonErr)
		}
		if len(jsonData) == 0 {
			t.Errorf("expected JSON data")
		}
	})

	t.Run("ToJSON_RegularError", func(t *testing.T) {
		err := errors.New("regular error")
		jsonData, jsonErr := formatter.ToJSON(err)

		if jsonErr != nil {
			t.Errorf("expected no error converting to JSON, got %v", jsonErr)
		}
		if len(jsonData) == 0 {
			t.Errorf("expected JSON data")
		}
	})

	t.Run("ToUserFriendly_WithUserMessage", func(t *testing.T) {
		err := NewBackupError("backup_op", "backup failed", nil).
			WithUserMessage("Backup process encountered an issue. Please try again.")

		msg := formatter.ToUserFriendly(err)
		expected := "Backup process encountered an issue. Please try again."

		if msg != expected {
			t.Errorf("expected user message, got '%s'", msg)
		}
	})

	t.Run("ToUserFriendly_ByCode", func(t *testing.T) {
		err := New(ErrCodeNetworkTimeout, "network", "connect", "connection timed out")
		msg := formatter.ToUserFriendly(err)
		expected := "Network connection timed out. Please try again."

		if msg != expected {
			t.Errorf("expected timeout message, got '%s'", msg)
		}
	})

	t.Run("ToUserFriendly_RegularError", func(t *testing.T) {
		err := errors.New("regular error")
		msg := formatter.ToUserFriendly(err)
		expected := "An error occurred. Please try again."

		if msg != expected {
			t.Errorf("expected generic message, got '%s'", msg)
		}
	})
}

// Benchmark tests
func BenchmarkErrorHandler_Handle(b *testing.B) {
	logger := &MockLogger{}
	handler := NewErrorHandler("test", logger)
	err := errors.New("test error")

	for i := 0; i < b.N; i++ {
		handler.Handle(err, "test_op")
	}
}

// TestUncoveredHandlerFunctions tests functions that had 0% coverage
func TestHandleAndReturn(t *testing.T) {
	logger := &MockLogger{}
	handler := NewErrorHandler("test_component", logger)

	t.Run("HandleAndReturn_StandardError", func(t *testing.T) {
		originalErr := NewBackupError("backup_op", "backup failed", nil)
		returnedErr := handler.HandleAndReturn(originalErr, "test_operation")

		// Should return the same error (StandardError)
		if returnedErr != originalErr {
			t.Errorf("expected same error instance to be returned")
		}

		// Should have logged the error
		if len(logger.ErrorErrCalls) != 1 {
			t.Errorf("expected 1 error log call, got %d", len(logger.ErrorErrCalls))
		}
	})

	t.Run("HandleAndReturn_RegularError", func(t *testing.T) {
		logger = &MockLogger{} // Reset
		handler = NewErrorHandler("test_component", logger)

		regularErr := errors.New("regular error")
		returnedErr := handler.HandleAndReturn(regularErr, "test_operation")

		// Should return a StandardError
		stdErr, ok := returnedErr.(*StandardError)
		if !ok {
			t.Errorf("expected StandardError to be returned")
		}

		if stdErr.Code != ErrCodeUnknown {
			t.Errorf("expected unknown error code")
		}
		if stdErr.Component != "test_component" {
			t.Errorf("expected test_component")
		}
	})

	t.Run("HandleAndReturn_NilError", func(t *testing.T) {
		logger = &MockLogger{} // Reset
		handler = NewErrorHandler("test_component", logger)

		returnedErr := handler.HandleAndReturn(nil, "test_operation")

		// Should return nil
		if returnedErr != nil {
			t.Errorf("expected nil to be returned for nil error, got %v (type: %T)", returnedErr, returnedErr)
		}

		// Should not have logged anything
		if len(logger.ErrorErrCalls) != 0 || len(logger.WarnCalls) != 0 || len(logger.InfoCalls) != 0 {
			t.Errorf("expected no logging for nil error")
		}
	})
}

// TestDefaultLogger tests the DefaultLogger implementation
func TestDefaultLogger(t *testing.T) {
	logger := &DefaultLogger{}

	// These tests mainly ensure the methods don't panic
	// Since DefaultLogger uses the standard log package, we can't easily capture output

	t.Run("Error", func(t *testing.T) {
		// Should not panic
		logger.Error("test_op", "test message", map[string]interface{}{"key": "value"})
	})

	t.Run("ErrorWithErr", func(t *testing.T) {
		// Should not panic
		logger.ErrorWithErr("test_op", "test message", map[string]interface{}{"key": "value"}, errors.New("cause"))
	})

	t.Run("Warn", func(t *testing.T) {
		// Should not panic
		logger.Warn("test_op", "test message", map[string]interface{}{"key": "value"})
	})

	t.Run("Info", func(t *testing.T) {
		// Should not panic
		logger.Info("test_op", "test message", map[string]interface{}{"key": "value"})
	})
}

// TestMultiErrorZeroCoverage tests the MultiError.Error method with empty errors
func TestMultiErrorEdgeCases(t *testing.T) {
	t.Run("Error_EmptyErrors", func(t *testing.T) {
		multiErr := NewMultiError("test_component", "test_operation")
		errorStr := multiErr.Error()
		expected := "no errors"
		if errorStr != expected {
			t.Errorf("expected '%s', got '%s'", expected, errorStr)
		}
	})

	t.Run("Error_SingleError", func(t *testing.T) {
		multiErr := NewMultiError("test_component", "test_operation")
		singleErr := NewValidationError("config", "field", "invalid value")
		multiErr.Add(singleErr)

		errorStr := multiErr.Error()
		expected := singleErr.Error()
		if errorStr != expected {
			t.Errorf("expected '%s', got '%s'", expected, errorStr)
		}
	})

	t.Run("Error_MultipleErrors", func(t *testing.T) {
		multiErr := NewMultiError("test_component", "test_operation")
		err1 := NewValidationError("config", "field1", "invalid value1")
		err2 := NewValidationError("config", "field2", "invalid value2")
		multiErr.Add(err1)
		multiErr.Add(err2)

		errorStr := multiErr.Error()
		expected := fmt.Sprintf("multiple errors (2): %s", err1.Error())
		if errorStr != expected {
			t.Errorf("expected '%s', got '%s'", expected, errorStr)
		}
	})
}

func BenchmarkErrorReporter_Report(b *testing.B) {
	logger := &MockLogger{}
	handler := NewErrorHandler("test", logger)
	reporter := NewErrorReporter(handler)
	err := NewBackupError("test", "test error", nil)

	for i := 0; i < b.N; i++ {
		reporter.Report(err, "test_op")
	}
}
