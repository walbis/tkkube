"""
Tests for the Python error handling system.
"""

import unittest
from datetime import datetime
from errors import (
    StandardError, MultiError, ErrorCode, Severity,
    new_backup_error, new_validation_error, new_kubernetes_error,
    is_code, is_retryable, get_code, get_severity, wrap_error,
    ErrorHandler, ValidationHelper, ErrorFormatter
)


class TestStandardError(unittest.TestCase):
    """Test StandardError functionality."""
    
    def test_basic_error(self):
        """Test basic error creation."""
        err = StandardError(
            ErrorCode.BACKUP_OPERATION, 
            "backup", 
            "test_operation", 
            "test message"
        )
        
        self.assertEqual(err.code, ErrorCode.BACKUP_OPERATION)
        self.assertEqual(err.component, "backup")
        self.assertEqual(err.operation, "test_operation")
        self.assertEqual(err.message, "test message")
        self.assertEqual(err.severity, Severity.HIGH)
        self.assertFalse(err.retryable)
        self.assertIsInstance(err.timestamp, datetime)
    
    def test_error_with_cause(self):
        """Test error with underlying cause."""
        cause = ValueError("underlying error")
        err = StandardError(
            ErrorCode.NETWORK_TIMEOUT,
            "network",
            "connect",
            "connection failed",
            cause=cause
        )
        
        self.assertEqual(err.code, ErrorCode.NETWORK_TIMEOUT)
        self.assertEqual(err.cause, cause)
        self.assertTrue(err.retryable)
        self.assertEqual(err.severity, Severity.MEDIUM)
        self.assertIn("underlying error", str(err))
    
    def test_error_with_context(self):
        """Test error with context."""
        err = StandardError(
            ErrorCode.VALIDATION,
            "config",
            "validate",
            "invalid value"
        ).with_context("field", "timeout").with_context("value", 30)
        
        self.assertEqual(err.context["field"], "timeout")
        self.assertEqual(err.context["value"], 30)
    
    def test_error_serialization(self):
        """Test error serialization."""
        err = StandardError(
            ErrorCode.BACKUP_OPERATION,
            "backup",
            "test",
            "test error"
        ).with_context("resource_count", 5)
        
        # Test to_dict
        data = err.to_dict()
        self.assertEqual(data["code"], "BACKUP_OPERATION")
        self.assertEqual(data["component"], "backup")
        self.assertEqual(data["context"]["resource_count"], 5)
        
        # Test to_json
        json_str = err.to_json()
        self.assertIsInstance(json_str, str)
        self.assertIn("BACKUP_OPERATION", json_str)


class TestErrorHelpers(unittest.TestCase):
    """Test error helper functions."""
    
    def test_is_code(self):
        """Test is_code function."""
        err = new_backup_error("test", "backup failed")
        self.assertTrue(is_code(err, ErrorCode.BACKUP_OPERATION))
        self.assertFalse(is_code(err, ErrorCode.VALIDATION))
        
        # Test with regular exception
        regular_err = ValueError("regular error")
        self.assertFalse(is_code(regular_err, ErrorCode.BACKUP_OPERATION))
    
    def test_is_retryable(self):
        """Test is_retryable function."""
        retryable_err = new_kubernetes_error("api_call", "API timeout")
        non_retryable_err = new_validation_error("config", "timeout", "invalid timeout")
        
        self.assertTrue(is_retryable(retryable_err))
        self.assertFalse(is_retryable(non_retryable_err))
        
        # Test with regular exception
        regular_err = ValueError("regular error")
        self.assertFalse(is_retryable(regular_err))
    
    def test_get_code(self):
        """Test get_code function."""
        err = new_backup_error("test", "backup failed")
        self.assertEqual(get_code(err), ErrorCode.BACKUP_OPERATION)
        
        regular_err = ValueError("regular error")
        self.assertEqual(get_code(regular_err), ErrorCode.UNKNOWN)
    
    def test_get_severity(self):
        """Test get_severity function."""
        critical_err = StandardError(ErrorCode.DATA_CORRUPTION, "storage", "read", "data corrupted")
        self.assertEqual(get_severity(critical_err), Severity.CRITICAL)
        
        validation_err = new_validation_error("config", "field", "invalid value")
        self.assertEqual(get_severity(validation_err), Severity.LOW)


class TestMultiError(unittest.TestCase):
    """Test MultiError functionality."""
    
    def test_empty_multi_error(self):
        """Test empty multi error."""
        multi_err = MultiError("backup", "multi_operation")
        
        self.assertFalse(multi_err.has_errors())
        self.assertIsNone(multi_err.to_error())
    
    def test_multi_error_with_errors(self):
        """Test multi error with actual errors."""
        multi_err = MultiError("backup", "multi_operation")
        
        err1 = new_validation_error("config", "field1", "invalid field1")
        err2 = new_validation_error("config", "field2", "invalid field2")
        
        multi_err.add(err1)
        multi_err.add(err2)
        
        self.assertTrue(multi_err.has_errors())
        self.assertIsNotNone(multi_err.to_error())
        self.assertEqual(len(multi_err.errors), 2)


class TestConvenienceFunctions(unittest.TestCase):
    """Test convenience functions for creating errors."""
    
    def test_new_backup_error(self):
        """Test new_backup_error function."""
        err = new_backup_error("backup_op", "backup failed")
        self.assertEqual(err.code, ErrorCode.BACKUP_OPERATION)
        self.assertEqual(err.component, "backup")
        self.assertEqual(err.operation, "backup_op")
    
    def test_new_validation_error(self):
        """Test new_validation_error function."""
        err = new_validation_error("config", "username", "username is required")
        self.assertEqual(err.code, ErrorCode.VALIDATION)
        self.assertEqual(err.component, "config")
        self.assertEqual(err.context["field"], "username")
    
    def test_wrap_error(self):
        """Test wrap_error function."""
        original_err = ValueError("original error")
        wrapped_err = wrap_error(
            original_err, 
            ErrorCode.INFRASTRUCTURE, 
            "component", 
            "operation", 
            "wrapped message"
        )
        
        self.assertEqual(wrapped_err.code, ErrorCode.INFRASTRUCTURE)
        self.assertEqual(wrapped_err.cause, original_err)


class TestErrorHandler(unittest.TestCase):
    """Test ErrorHandler functionality."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.handler = ErrorHandler("test_component")
    
    def test_handle_standard_error(self):
        """Test handling StandardError."""
        original_err = new_backup_error("backup_op", "backup failed")
        handled_err = self.handler.handle(original_err, "test_operation")
        
        self.assertEqual(handled_err, original_err)
    
    def test_handle_regular_error(self):
        """Test handling regular exception."""
        regular_err = ValueError("regular error")
        handled_err = self.handler.handle(regular_err, "test_operation")
        
        self.assertEqual(handled_err.code, ErrorCode.UNKNOWN)
        self.assertEqual(handled_err.component, "test_component")
        self.assertEqual(handled_err.cause, regular_err)


class TestValidationHelper(unittest.TestCase):
    """Test ValidationHelper functionality."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.validator = ValidationHelper("test_component")
    
    def test_required_valid(self):
        """Test required validation with valid value."""
        err = self.validator.required("username", "john_doe")
        self.assertIsNone(err)
    
    def test_required_invalid(self):
        """Test required validation with invalid value."""
        err = self.validator.required("username", "")
        self.assertIsNotNone(err)
        self.assertEqual(err.code, ErrorCode.VALIDATION)
    
    def test_range_check_valid(self):
        """Test range check with valid value."""
        err = self.validator.range_check("timeout", 30, 10, 60)
        self.assertIsNone(err)
    
    def test_range_check_invalid_low(self):
        """Test range check with value below range."""
        err = self.validator.range_check("timeout", 5, 10, 60)
        self.assertIsNotNone(err)
        self.assertEqual(err.code, ErrorCode.VALIDATION)
    
    def test_range_check_invalid_high(self):
        """Test range check with value above range."""
        err = self.validator.range_check("timeout", 70, 10, 60)
        self.assertIsNotNone(err)
        self.assertEqual(err.code, ErrorCode.VALIDATION)


class TestErrorFormatter(unittest.TestCase):
    """Test ErrorFormatter functionality."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.formatter = ErrorFormatter()
    
    def test_to_user_friendly_with_user_message(self):
        """Test user-friendly formatting with custom message."""
        err = new_backup_error("backup_op", "backup failed").with_user_message(
            "Backup process encountered an issue. Please try again."
        )
        
        msg = self.formatter.to_user_friendly(err)
        expected = "Backup process encountered an issue. Please try again."
        self.assertEqual(msg, expected)
    
    def test_to_user_friendly_by_code(self):
        """Test user-friendly formatting by error code."""
        err = StandardError(ErrorCode.NETWORK_TIMEOUT, "network", "connect", "connection timed out")
        msg = self.formatter.to_user_friendly(err)
        expected = "Network connection timed out. Please try again."
        self.assertEqual(msg, expected)
    
    def test_to_user_friendly_regular_error(self):
        """Test user-friendly formatting for regular exception."""
        err = ValueError("regular error")
        msg = self.formatter.to_user_friendly(err)
        expected = "An error occurred. Please try again."
        self.assertEqual(msg, expected)


class TestSeverityMapping(unittest.TestCase):
    """Test severity mapping for error codes."""
    
    def test_severity_mapping(self):
        """Test severity mapping for various error codes."""
        test_cases = [
            (ErrorCode.DATA_CORRUPTION, Severity.CRITICAL),
            (ErrorCode.SYSTEM_OVERLOAD, Severity.CRITICAL),
            (ErrorCode.INFRASTRUCTURE, Severity.HIGH),
            (ErrorCode.BACKUP_OPERATION, Severity.HIGH),
            (ErrorCode.NETWORK_TIMEOUT, Severity.MEDIUM),
            (ErrorCode.VALIDATION, Severity.LOW),
            (ErrorCode.UNKNOWN, Severity.MEDIUM),
        ]
        
        for code, expected_severity in test_cases:
            with self.subTest(code=code):
                err = StandardError(code, "test", "test", "test")
                self.assertEqual(err.severity, expected_severity)


class TestRetryableMapping(unittest.TestCase):
    """Test retryable mapping for error codes."""
    
    def test_retryable_mapping(self):
        """Test retryable mapping for various error codes."""
        test_cases = [
            (ErrorCode.NETWORK_TIMEOUT, True),
            (ErrorCode.KUBERNETES_API, True),
            (ErrorCode.STORAGE_TIMEOUT, True),
            (ErrorCode.VALIDATION, False),
            (ErrorCode.DATA_CORRUPTION, False),
            (ErrorCode.PERMISSION, False),
        ]
        
        for code, expected_retryable in test_cases:
            with self.subTest(code=code):
                err = StandardError(code, "test", "test", "test")
                self.assertEqual(err.retryable, expected_retryable)


if __name__ == '__main__':
    unittest.main()