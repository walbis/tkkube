# Error Handling Best Practices Guide

This guide provides comprehensive documentation for the standardized error handling system implemented across the Kubernetes backup and restore system.

## Overview

The standardized error handling system provides:
- **Consistent error patterns** across Go and Python components
- **Rich error context** with structured information
- **Severity classification** for appropriate handling
- **Retryability detection** for resilience patterns
- **User-friendly messages** for external interfaces
- **Comprehensive logging** with structured data

## Architecture

### Error Codes
All errors are classified using standardized error codes:

```go
// Infrastructure errors
ErrCodeInfrastructure    = "INFRASTRUCTURE"
ErrCodeNetworkTimeout    = "NETWORK_TIMEOUT"
ErrCodeKubernetesAPI     = "KUBERNETES_API"

// Configuration errors  
ErrCodeConfiguration     = "CONFIGURATION"
ErrCodeValidation        = "VALIDATION"

// Operation errors
ErrCodeBackupOperation   = "BACKUP_OPERATION"
ErrCodeRestoreOperation  = "RESTORE_OPERATION"
ErrCodeCircuitBreaker    = "CIRCUIT_BREAKER"
```

### Severity Levels
Errors are automatically assigned severity levels:

- **CRITICAL**: Data corruption, system overload
- **HIGH**: Infrastructure failures, operation failures
- **MEDIUM**: Network timeouts, retry exhaustion
- **LOW**: Validation errors, format issues

### Retryability
Errors are automatically marked as retryable or non-retryable:

- **Retryable**: Network timeouts, API failures, storage timeouts
- **Non-retryable**: Validation errors, permission denied, data corruption

## Usage Examples

### Go Implementation

#### Basic Error Creation
```go
import "shared-config/errors"

// Create a backup operation error
err := errors.NewBackupError("upload", "failed to upload resource", cause)

// Create a validation error
err := errors.NewValidationError("config", "timeout", "timeout must be positive")

// Create a custom error with context
err := errors.New(errors.ErrCodeKubernetesAPI, "backup", "list_pods", "API call failed").
    WithContext("namespace", "default").
    WithContext("resource_type", "pods").
    WithUserMessage("Unable to access cluster resources. Please check permissions.")
```

#### Error Handling
```go
// Use error handler for consistent logging
handler := errors.NewErrorHandler("backup_service", logger)
stdErr := handler.Handle(err, "backup_operation")

// Check error properties
if errors.IsRetryable(err) {
    // Retry the operation
}

if errors.IsCode(err, errors.ErrCodeCircuitBreaker) {
    // Handle circuit breaker specifically
}

switch errors.GetSeverity(err) {
case errors.SeverityCritical:
    // Alert on-call team
case errors.SeverityHigh:
    // Log as error
}
```

#### Configuration Validation
```go
func (c *Config) Validate() error {
    validator := errors.NewValidationHelper("config")
    multiErr := errors.NewMultiError("config", "validation")
    
    // Required field validations
    if err := validator.Required("MINIO_ENDPOINT", c.MinIOEndpoint); err != nil {
        multiErr.Add(err)
    }
    
    // Range validations
    if err := validator.Range("batch_size", c.BatchSize, 1, 1000); err != nil {
        multiErr.Add(err)
    }
    
    return multiErr.ToError()
}
```

### Python Implementation

#### Basic Error Creation
```python
from shared.errors.errors import (
    new_backup_error, new_validation_error, StandardError, ErrorCode
)

# Create a backup operation error
err = new_backup_error("upload", "failed to upload resource", cause)

# Create a validation error  
err = new_validation_error("config", "timeout", "timeout must be positive")

# Create a custom error with context
err = StandardError(
    ErrorCode.KUBERNETES_API, 
    "backup", 
    "list_pods", 
    "API call failed"
).with_context("namespace", "default").with_user_message("Unable to access cluster resources.")
```

#### Error Handling
```python
from shared.errors.errors import ErrorHandler, is_retryable, is_code, get_severity

# Use error handler for consistent logging
handler = ErrorHandler("backup_service", logger)
std_err = handler.handle(err, "backup_operation")

# Check error properties
if is_retryable(err):
    # Retry the operation
    pass

if is_code(err, ErrorCode.CIRCUIT_BREAKER):
    # Handle circuit breaker specifically
    pass

severity = get_severity(err)
if severity == Severity.CRITICAL:
    # Alert on-call team
    pass
```

## Migration Guide

### Replacing fmt.Errorf
**Before:**
```go
return fmt.Errorf("failed to connect to MinIO: %v", err)
```

**After:**
```go
return errors.NewStorageError("connect", "failed to connect to MinIO", err)
```

### Replacing errors.New
**Before:**
```go
return errors.New("invalid configuration: timeout must be positive")
```

**After:**
```go
return errors.NewValidationError("config", "timeout", "timeout must be positive")
```

### Adding Context
**Before:**
```go
return fmt.Errorf("backup failed for namespace %s", namespace)
```

**After:**
```go
return errors.NewBackupError("namespace_backup", "backup failed").
    WithContext("namespace", namespace).
    WithContext("resource_count", resourceCount)
```

## Best Practices

### 1. Use Appropriate Error Codes
Choose the most specific error code that matches the error condition:

```go
// Good - specific error code
return errors.NewKubernetesError("list_pods", "API timeout", err)

// Avoid - generic error code
return errors.New(errors.ErrCodeUnknown, "k8s", "api", "error", err)
```

### 2. Add Meaningful Context
Include relevant context that helps with debugging:

```go
return errors.NewBackupError("resource_backup", "failed to backup resource").
    WithContext("namespace", namespace).
    WithContext("resource_type", resourceType).
    WithContext("resource_name", resourceName).
    WithContext("retry_count", retryCount)
```

### 3. Provide User-Friendly Messages
Add user-friendly messages for errors that may be shown to users:

```go
return errors.NewValidationError("config", "retention_days", 
    "retention days must be between 1 and 365").
    WithUserMessage("Please set a retention period between 1 and 365 days.")
```

### 4. Use Error Handlers for Consistent Logging
```go
handler := errors.NewErrorHandler("component_name", logger)
return handler.HandleAndReturn(err, "operation_name")
```

### 5. Leverage Error Recovery Patterns
```go
recovery := errors.NewErrorRecovery(handler)

// Safe execution with panic recovery
err := recovery.SafeExecute("risky_operation", func() error {
    return riskyOperation()
})

// Execution with timeout
err := recovery.WithTimeout(ctx, "long_operation", 30*time.Second, func() error {
    return longRunningOperation()
})
```

## Error Testing

### Go Testing
```go
func TestConfigValidation(t *testing.T) {
    config := &Config{BatchSize: 1500} // Invalid
    err := config.Validate()
    
    // Test error code
    assert.True(t, errors.IsCode(err, errors.ErrCodeValidation))
    
    // Test multi-error
    multiErr, ok := err.(*errors.MultiError)
    assert.True(t, ok)
    assert.True(t, multiErr.HasErrors())
}

func TestErrorRetryability(t *testing.T) {
    err := errors.NewKubernetesError("api_call", "timeout", nil)
    assert.True(t, errors.IsRetryable(err))
    
    err = errors.NewValidationError("config", "field", "invalid")
    assert.False(t, errors.IsRetryable(err))
}
```

### Python Testing
```python
def test_error_properties(self):
    err = new_kubernetes_error("api_call", "timeout")
    self.assertTrue(is_retryable(err))
    self.assertEqual(get_severity(err), Severity.HIGH)
    
def test_validation_helper(self):
    validator = ValidationHelper("config")
    err = validator.required("username", "")
    self.assertIsNotNone(err)
    self.assertEqual(err.code, ErrorCode.VALIDATION)
```

## Monitoring and Observability

### Error Metrics
The error system automatically tracks metrics:

- **Error counts by code**: Track frequency of different error types
- **Error severity distribution**: Monitor system health
- **Retryable vs non-retryable**: Measure resilience effectiveness

### Structured Logging
All errors are logged with structured data:

```json
{
  "level": "error",
  "operation": "backup_namespace",
  "message": "failed to backup resources",
  "error_code": "KUBERNETES_API",
  "severity": "HIGH",
  "retryable": true,
  "context": {
    "namespace": "default",
    "resource_count": 10
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Integration with Existing Systems

### Circuit Breakers
```go
// Circuit breaker errors are automatically handled
if err := circuitBreaker.Execute(operation); err != nil {
    if errors.IsCode(err, errors.ErrCodeCircuitBreaker) {
        // Circuit is open, handle gracefully
        return errors.NewBackupError("circuit_open", 
            "service temporarily unavailable", err).
            WithUserMessage("Service is temporarily unavailable. Please try again later.")
    }
}
```

### Retry Logic
```go
func retryableOperation() error {
    return retryExecutor.Execute(func() error {
        err := apiCall()
        if err != nil && !errors.IsRetryable(err) {
            // Don't retry non-retryable errors
            return retry.StopRetrying(err)
        }
        return err
    })
}
```

### Monitoring Integration
```go
// Error reporter for metrics
reporter := errors.NewErrorReporter(handler)

// This automatically updates metrics
stdErr := reporter.Report(err, "operation")

// Get error metrics
metrics := reporter.GetMetrics()
for code, count := range metrics {
    prometheusCounter.WithLabelValues(string(code)).Add(float64(count))
}
```

## Common Patterns

### 1. Validation Pattern
```go
func ValidateConfig(config *Config) error {
    validator := errors.NewValidationHelper("config")
    multiErr := errors.NewMultiError("config", "validation")
    
    if err := validator.Required("endpoint", config.Endpoint); err != nil {
        multiErr.Add(err)
    }
    if err := validator.Range("timeout", config.Timeout, 1, 300); err != nil {
        multiErr.Add(err)
    }
    
    return multiErr.ToError()
}
```

### 2. Service Layer Pattern
```go
type BackupService struct {
    errorHandler *errors.ErrorHandler
}

func (s *BackupService) BackupNamespace(namespace string) error {
    err := s.performBackup(namespace)
    if err != nil {
        return s.errorHandler.HandleAndReturn(err, "backup_namespace")
    }
    return nil
}
```

### 3. Recovery Pattern
```go
func (s *Service) SafeOperation() error {
    recovery := errors.NewErrorRecovery(s.errorHandler)
    
    return recovery.SafeExecute("safe_operation", func() error {
        // Operation that might panic
        return s.riskyOperation()
    })
}
```

This standardized error handling system ensures consistent, maintainable, and observable error handling across the entire backup and restore system.