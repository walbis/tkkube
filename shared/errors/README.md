# Shared Error Handling System

A standardized error handling library providing consistent error patterns across Go and Python components for the Kubernetes backup and restore system.

## Features

- **Consistent Error Classification**: 20+ standardized error codes with automatic severity mapping
- **Rich Error Context**: Structured error information with component, operation, and custom context
- **Retryability Detection**: Automatic marking of retryable vs non-retryable errors
- **Multi-language Support**: Identical error patterns in both Go and Python
- **Production Ready**: Comprehensive logging, monitoring integration, and user-friendly messages
- **High Test Coverage**: 93% Go coverage with comprehensive Python test suite

## Quick Start

### Go Usage

```go
import "shared-errors"

// Create a backup operation error
err := errors.NewBackupError("upload", "failed to upload resource", cause)

// Create a validation error
err := errors.NewValidationError("config", "timeout", "timeout must be positive")

// Handle errors with context
handler := errors.NewErrorHandler("backup_service", logger)
stdErr := handler.Handle(err, "backup_operation")
```

### Python Usage

```python
from errors import new_backup_error, new_validation_error, ErrorHandler

# Create a backup operation error
err = new_backup_error("upload", "failed to upload resource", cause)

# Create a validation error  
err = new_validation_error("config", "timeout", "timeout must be positive")

# Handle errors with context
handler = ErrorHandler("backup_service", logger)
std_err = handler.handle(err, "backup_operation")
```

## Error Codes

The system provides standardized error codes organized by category:

- **Infrastructure**: `INFRASTRUCTURE`, `NETWORK_TIMEOUT`, `KUBERNETES_API`, `STORAGE_TIMEOUT`
- **Configuration**: `CONFIGURATION`, `VALIDATION`, `PERMISSION`, `AUTHENTICATION`
- **Operations**: `BACKUP_OPERATION`, `RESTORE_OPERATION`, `CIRCUIT_BREAKER`, `RETRY_EXHAUSTED`
- **Data**: `DATA_CORRUPTION`, `DATA_FORMAT`, `DATA_VALIDATION`
- **System**: `SYSTEM_OVERLOAD`, `SYSTEM_TIMEOUT`, `UNKNOWN`

## Severity Levels

Errors are automatically assigned severity levels:

- **CRITICAL**: Data corruption, system overload
- **HIGH**: Infrastructure failures, operation failures
- **MEDIUM**: Network timeouts, retry exhaustion
- **LOW**: Validation errors, format issues

## Development

### Prerequisites

- Go 1.24.0 or later
- Python 3.8 or later

### Building and Testing

```bash
# Run all tests
make test

# Run Go tests only
make test-go

# Run Python tests only
make test-python

# Generate coverage report
make coverage

# Run linters
make lint

# Format code
make format

# Clean build artifacts
make clean
```

### Project Structure

```
shared/errors/
├── errors.go              # Go error system
├── errors_test.go         # Go tests
├── handlers.go            # Go error handlers
├── handlers_test.go       # Go handler tests
├── errors.py              # Python error system
├── test_errors.py         # Python tests
├── go.mod                 # Go module definition
├── Makefile               # Build automation
├── README.md              # This file
├── ERROR_HANDLING_GUIDE.md # Comprehensive usage guide
└── .gitignore             # Git ignore patterns
```

## Documentation

- [Error Handling Guide](ERROR_HANDLING_GUIDE.md) - Comprehensive implementation guide with examples
- [Code Quality Analysis](CODE_QUALITY_ANALYSIS_REPORT.md) - Quality assessment and metrics

## Quality Metrics

- **Test Coverage**: 93% Go coverage, comprehensive Python test suite
- **Code Quality**: A- grade (87/100) with clean static analysis
- **Documentation**: Complete with migration examples and best practices
- **Architecture**: Consistent cross-language patterns with structured context

## Contributing

1. Follow existing code patterns and conventions
2. Add tests for new functionality
3. Run `make test` and `make lint` before committing
4. Update documentation for new features

## License

This error handling system is part of the Kubernetes backup and restore project.