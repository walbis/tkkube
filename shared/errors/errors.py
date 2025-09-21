"""
Standardized error handling for Python components.
Provides consistent error patterns matching the Go error system.
"""

import json
import traceback
from datetime import datetime, timezone
from enum import Enum
from typing import Dict, Any, Optional, List


class ErrorCode(Enum):
    """Error classification codes matching Go implementation."""
    
    # Infrastructure errors
    INFRASTRUCTURE = "INFRASTRUCTURE"
    NETWORK_TIMEOUT = "NETWORK_TIMEOUT"
    NETWORK_CONNECTION = "NETWORK_CONNECTION"
    STORAGE_TIMEOUT = "STORAGE_TIMEOUT"
    STORAGE_SPACE = "STORAGE_SPACE"
    KUBERNETES_API = "KUBERNETES_API"
    
    # Configuration errors
    CONFIGURATION = "CONFIGURATION"
    VALIDATION = "VALIDATION"
    PERMISSION = "PERMISSION"
    AUTHENTICATION = "AUTHENTICATION"
    
    # Operation errors
    BACKUP_OPERATION = "BACKUP_OPERATION"
    RESTORE_OPERATION = "RESTORE_OPERATION"
    RETRY_EXHAUSTED = "RETRY_EXHAUSTED"
    CIRCUIT_BREAKER = "CIRCUIT_BREAKER"
    RESOURCE_LIMIT = "RESOURCE_LIMIT"
    
    # Data errors
    DATA_CORRUPTION = "DATA_CORRUPTION"
    DATA_FORMAT = "DATA_FORMAT"
    DATA_VALIDATION = "DATA_VALIDATION"
    
    # System errors
    SYSTEM_OVERLOAD = "SYSTEM_OVERLOAD"
    SYSTEM_TIMEOUT = "SYSTEM_TIMEOUT"
    UNKNOWN = "UNKNOWN"


class Severity(Enum):
    """Error severity levels."""
    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"
    CRITICAL = "CRITICAL"


class StandardError(Exception):
    """Standardized error with rich context."""
    
    def __init__(
        self,
        code: ErrorCode,
        component: str,
        operation: str,
        message: str,
        cause: Optional[Exception] = None,
        context: Optional[Dict[str, Any]] = None,
        user_message: Optional[str] = None
    ):
        self.code = code
        self.component = component
        self.operation = operation
        self.message = message
        self.cause = cause
        self.context = context or {}
        self.timestamp = datetime.now(timezone.utc)
        self.stack_trace = traceback.format_exc()
        self.user_message = user_message
        
        # Auto-assign severity and retryable based on code
        self.severity = self._get_severity_for_code(code)
        self.retryable = self._is_retryable_code(code)
        
        super().__init__(self._format_message())
    
    def _format_message(self) -> str:
        """Format the error message."""
        if self.cause:
            return f"[{self.code.value}] {self.message}: {str(self.cause)}"
        return f"[{self.code.value}] {self.message}"
    
    def _get_severity_for_code(self, code: ErrorCode) -> Severity:
        """Map error codes to default severity levels."""
        critical_codes = {ErrorCode.DATA_CORRUPTION, ErrorCode.SYSTEM_OVERLOAD}
        high_codes = {
            ErrorCode.INFRASTRUCTURE, ErrorCode.KUBERNETES_API,
            ErrorCode.BACKUP_OPERATION, ErrorCode.RESTORE_OPERATION
        }
        medium_codes = {
            ErrorCode.NETWORK_TIMEOUT, ErrorCode.STORAGE_TIMEOUT,
            ErrorCode.RETRY_EXHAUSTED
        }
        low_codes = {ErrorCode.VALIDATION, ErrorCode.DATA_FORMAT}
        
        if code in critical_codes:
            return Severity.CRITICAL
        elif code in high_codes:
            return Severity.HIGH
        elif code in medium_codes:
            return Severity.MEDIUM
        elif code in low_codes:
            return Severity.LOW
        else:
            return Severity.MEDIUM
    
    def _is_retryable_code(self, code: ErrorCode) -> bool:
        """Determine if an error code represents a retryable operation."""
        retryable_codes = {
            ErrorCode.NETWORK_TIMEOUT, ErrorCode.NETWORK_CONNECTION,
            ErrorCode.STORAGE_TIMEOUT, ErrorCode.KUBERNETES_API,
            ErrorCode.SYSTEM_TIMEOUT
        }
        non_retryable_codes = {
            ErrorCode.VALIDATION, ErrorCode.DATA_CORRUPTION,
            ErrorCode.PERMISSION, ErrorCode.AUTHENTICATION,
            ErrorCode.DATA_FORMAT
        }
        
        if code in retryable_codes:
            return True
        elif code in non_retryable_codes:
            return False
        else:
            return False
    
    def with_context(self, key: str, value: Any) -> 'StandardError':
        """Add context to the error."""
        self.context[key] = value
        return self
    
    def with_user_message(self, message: str) -> 'StandardError':
        """Set a user-friendly message."""
        self.user_message = message
        return self
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert error to dictionary representation."""
        return {
            'code': self.code.value,
            'message': self.message,
            'severity': self.severity.value,
            'context': self.context,
            'timestamp': self.timestamp.isoformat(),
            'component': self.component,
            'operation': self.operation,
            'retryable': self.retryable,
            'user_message': self.user_message,
            'cause': str(self.cause) if self.cause else None
        }
    
    def to_json(self) -> str:
        """Convert error to JSON representation."""
        return json.dumps(self.to_dict(), indent=2)


class MultiError(Exception):
    """Holds multiple errors with context."""
    
    def __init__(self, component: str, operation: str):
        self.errors: List[StandardError] = []
        self.component = component
        self.operation = operation
        self.context: Dict[str, Any] = {}
        super().__init__()
    
    def add(self, error: StandardError) -> None:
        """Add an error to the MultiError."""
        self.errors.append(error)
    
    def has_errors(self) -> bool:
        """Return True if there are any errors."""
        return len(self.errors) > 0
    
    def to_error(self) -> Optional['MultiError']:
        """Return the MultiError as an exception if there are errors."""
        if not self.has_errors():
            return None
        return self
    
    def __str__(self) -> str:
        if len(self.errors) == 0:
            return "no errors"
        if len(self.errors) == 1:
            return str(self.errors[0])
        return f"multiple errors ({len(self.errors)}): {str(self.errors[0])}"


# Convenience functions for common error types
def new_infrastructure_error(component: str, operation: str, message: str, cause: Exception = None) -> StandardError:
    """Create infrastructure-related errors."""
    return StandardError(ErrorCode.INFRASTRUCTURE, component, operation, message, cause)


def new_configuration_error(component: str, operation: str, message: str, cause: Exception = None) -> StandardError:
    """Create configuration-related errors."""
    return StandardError(ErrorCode.CONFIGURATION, component, operation, message, cause)


def new_backup_error(operation: str, message: str, cause: Exception = None) -> StandardError:
    """Create backup operation errors."""
    return StandardError(ErrorCode.BACKUP_OPERATION, "backup", operation, message, cause)


def new_restore_error(operation: str, message: str, cause: Exception = None) -> StandardError:
    """Create restore operation errors."""
    return StandardError(ErrorCode.RESTORE_OPERATION, "restore", operation, message, cause)


def new_kubernetes_error(operation: str, message: str, cause: Exception = None) -> StandardError:
    """Create Kubernetes API errors."""
    return StandardError(ErrorCode.KUBERNETES_API, "kubernetes", operation, message, cause)


def new_storage_error(operation: str, message: str, cause: Exception = None) -> StandardError:
    """Create storage-related errors."""
    return StandardError(ErrorCode.STORAGE_TIMEOUT, "storage", operation, message, cause)


def new_validation_error(component: str, field: str, message: str) -> StandardError:
    """Create validation errors."""
    return StandardError(ErrorCode.VALIDATION, component, "validation", message).with_context("field", field)


# Utility functions
def is_code(error: Exception, code: ErrorCode) -> bool:
    """Check if an error has a specific error code."""
    if isinstance(error, StandardError):
        return error.code == code
    return False


def is_retryable(error: Exception) -> bool:
    """Check if an error is retryable."""
    if isinstance(error, StandardError):
        return error.retryable
    return False


def get_code(error: Exception) -> ErrorCode:
    """Extract the error code from an error."""
    if isinstance(error, StandardError):
        return error.code
    return ErrorCode.UNKNOWN


def get_severity(error: Exception) -> Severity:
    """Extract the severity from an error."""
    if isinstance(error, StandardError):
        return error.severity
    return Severity.MEDIUM


def wrap_error(error: Exception, code: ErrorCode, component: str, operation: str, message: str) -> StandardError:
    """Wrap an existing error with standardized context."""
    return StandardError(code, component, operation, message, error)


class ErrorHandler:
    """Centralized error handling and logging."""
    
    def __init__(self, component: str, logger=None):
        self.component = component
        self.logger = logger
    
    def handle(self, error: Exception, operation: str) -> StandardError:
        """Process and log a standardized error."""
        std_err = self.to_standard_error(error, operation)
        self._log_error(std_err)
        return std_err
    
    def to_standard_error(self, error: Exception, operation: str) -> StandardError:
        """Convert any error to a StandardError."""
        if isinstance(error, StandardError):
            return error
        
        # Create new StandardError
        return wrap_error(error, ErrorCode.UNKNOWN, self.component, operation, "unexpected error")
    
    def _log_error(self, error: StandardError) -> None:
        """Log the error based on its severity."""
        if not self.logger:
            return
        
        log_data = {
            'error_code': error.code.value,
            'severity': error.severity.value,
            'retryable': error.retryable,
            'timestamp': error.timestamp.isoformat(),
            'context': error.context
        }
        
        if error.severity in [Severity.CRITICAL, Severity.HIGH]:
            self.logger.error(f"{error.operation}: {error.message}", extra=log_data, exc_info=error.cause)
        elif error.severity == Severity.MEDIUM:
            self.logger.warning(f"{error.operation}: {error.message}", extra=log_data)
        else:
            self.logger.info(f"{error.operation}: {error.message}", extra=log_data)


class ValidationHelper:
    """Validation error utilities."""
    
    def __init__(self, component: str):
        self.component = component
    
    def required(self, field: str, value: Any) -> Optional[StandardError]:
        """Validate that a field is not empty."""
        if value is None or value == "":
            return new_validation_error(self.component, field, f"{field} is required")
        return None
    
    def range_check(self, field: str, value: int, min_val: int, max_val: int) -> Optional[StandardError]:
        """Validate that a value is within a range."""
        if value < min_val or value > max_val:
            return new_validation_error(
                self.component, 
                field, 
                f"{field} must be between {min_val} and {max_val}, got {value}"
            )
        return None


class ErrorFormatter:
    """Error formatting utilities."""
    
    def to_user_friendly(self, error: Exception) -> str:
        """Convert an error to a user-friendly message."""
        if isinstance(error, StandardError) and error.user_message:
            return error.user_message
        
        if isinstance(error, StandardError):
            code_messages = {
                ErrorCode.NETWORK_TIMEOUT: "Network connection timed out. Please try again.",
                ErrorCode.PERMISSION: "Permission denied. Please check your access rights.",
                ErrorCode.VALIDATION: "Invalid input provided. Please check your data and try again.",
                ErrorCode.BACKUP_OPERATION: "Backup operation failed. Please contact support if this persists.",
                ErrorCode.STORAGE_SPACE: "Insufficient storage space available.",
            }
            return code_messages.get(error.code, "An unexpected error occurred. Please try again or contact support.")
        
        return "An error occurred. Please try again."


# Default formatter instance
error_formatter = ErrorFormatter()