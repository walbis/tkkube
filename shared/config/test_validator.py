"""
Tests for enhanced schema validation.
"""

import pytest
import yaml
from pathlib import Path
from datetime import timedelta

from .validator import (
    ValidationError,
    ValidationResult,
    ValidatedStorageConfig,
    ValidatedBackupConfig,
    ValidatedGitOpsConfig,
    ValidatedPipelineConfig,
    ConfigValidator,
    validate_config,
    validate_config_file
)


class TestValidationError:
    """Tests for ValidationError class."""
    
    def test_validation_error_creation(self):
        """Test ValidationError creation."""
        error = ValidationError("field", "value", "message", "error")
        assert error.field == "field"
        assert error.value == "value"
        assert error.message == "message"
        assert error.level == "error"
    
    def test_validation_error_repr(self):
        """Test ValidationError string representation."""
        error = ValidationError("test_field", "test_value", "test message", "error")
        repr_str = repr(error)
        assert "test_field" in repr_str
        assert "test message" in repr_str


class TestValidationResult:
    """Tests for ValidationResult class."""
    
    def test_validation_result_creation(self):
        """Test ValidationResult creation."""
        result = ValidationResult()
        assert result.errors == []
        assert result.warnings == []
        assert result.valid is True
    
    def test_add_error(self):
        """Test adding errors."""
        result = ValidationResult()
        result.add_error("field", "value", "error message")
        
        assert len(result.errors) == 1
        assert result.errors[0].field == "field"
        assert result.errors[0].level == "error"
        assert result.valid is False
    
    def test_add_warning(self):
        """Test adding warnings."""
        result = ValidationResult()
        result.add_warning("field", "value", "warning message")
        
        assert len(result.warnings) == 1
        assert result.warnings[0].field == "field"
        assert result.warnings[0].level == "warning"
        assert result.valid is True  # warnings don't affect validity
    
    def test_format_result_valid(self):
        """Test formatting valid result."""
        result = ValidationResult()
        result.add_warning("test.field", "value", "test warning")
        
        formatted = result.format_result()
        assert "✅ Configuration is valid" in formatted
        assert "⚠️" in formatted
        assert "test warning" in formatted
    
    def test_format_result_invalid(self):
        """Test formatting invalid result."""
        result = ValidationResult()
        result.add_error("test.field", "value", "test error")
        result.add_warning("test.field2", "value2", "test warning")
        
        formatted = result.format_result()
        assert "❌ Configuration validation failed" in formatted
        assert "test error" in formatted
        assert "test warning" in formatted


class TestValidatedStorageConfig:
    """Tests for ValidatedStorageConfig."""
    
    def test_valid_minio_config(self):
        """Test valid MinIO configuration."""
        config_data = {
            "type": "minio",
            "endpoint": "localhost:9000",
            "access_key": "minioadmin",
            "secret_key": "minioadmin",
            "bucket": "test-bucket",
            "use_ssl": False,
            "region": "us-east-1"
        }
        
        config = ValidatedStorageConfig(**config_data)
        assert config.type == "minio"
        assert config.endpoint == "localhost:9000"
    
    def test_valid_s3_config(self):
        """Test valid S3 configuration."""
        config_data = {
            "type": "s3",
            "endpoint": "https://s3.amazonaws.com",
            "access_key": "AKIAIOSFODNN7EXAMPLE",
            "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
            "bucket": "my-bucket",
            "use_ssl": True,
            "region": "us-west-2"
        }
        
        config = ValidatedStorageConfig(**config_data)
        assert config.type == "s3"
        assert config.region == "us-west-2"
    
    def test_invalid_storage_type(self):
        """Test invalid storage type."""
        config_data = {
            "type": "invalid",
            "endpoint": "localhost:9000",
            "access_key": "minioadmin",
            "secret_key": "minioadmin",
            "bucket": "test-bucket"
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedStorageConfig(**config_data)
    
    def test_invalid_endpoint(self):
        """Test invalid endpoint."""
        config_data = {
            "type": "minio",
            "endpoint": "invalid-endpoint",
            "access_key": "minioadmin",
            "secret_key": "minioadmin",
            "bucket": "test-bucket"
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedStorageConfig(**config_data)
    
    def test_invalid_bucket_name(self):
        """Test invalid bucket name."""
        config_data = {
            "type": "minio",
            "endpoint": "localhost:9000",
            "access_key": "minioadmin",
            "secret_key": "minioadmin",
            "bucket": "INVALID_BUCKET_NAME"  # uppercase not allowed
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedStorageConfig(**config_data)
    
    def test_s3_without_region(self):
        """Test S3 configuration without region."""
        config_data = {
            "type": "s3",
            "endpoint": "https://s3.amazonaws.com",
            "access_key": "AKIAIOSFODNN7EXAMPLE",
            "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
            "bucket": "my-bucket",
            "region": ""
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedStorageConfig(**config_data)


class TestValidatedGitOpsConfig:
    """Tests for ValidatedGitOpsConfig."""
    
    def test_valid_ssh_config(self):
        """Test valid SSH Git configuration."""
        config_data = {
            "repository": {
                "url": "git@github.com:user/repo.git",
                "branch": "main",
                "auth": {
                    "method": "ssh",
                    "ssh": {
                        "private_key_path": "/home/user/.ssh/id_rsa"
                    }
                }
            }
        }
        
        config = ValidatedGitOpsConfig(**config_data)
        assert config.repository.url == "git@github.com:user/repo.git"
        assert config.repository.auth.method == "ssh"
    
    def test_valid_pat_config(self):
        """Test valid PAT configuration."""
        config_data = {
            "repository": {
                "url": "https://github.com/user/repo.git",
                "branch": "develop",
                "auth": {
                    "method": "pat",
                    "pat": {
                        "token": "ghp_1234567890abcdef",
                        "username": "user"
                    }
                }
            }
        }
        
        config = ValidatedGitOpsConfig(**config_data)
        assert config.repository.auth.method == "pat"
        assert config.repository.auth.pat.token == "ghp_1234567890abcdef"
    
    def test_invalid_git_url(self):
        """Test invalid Git URL."""
        config_data = {
            "repository": {
                "url": "invalid-url",
                "branch": "main"
            }
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedGitOpsConfig(**config_data)
    
    def test_ssh_without_key(self):
        """Test SSH auth without private key."""
        config_data = {
            "repository": {
                "url": "git@github.com:user/repo.git",
                "branch": "main",
                "auth": {
                    "method": "ssh",
                    "ssh": {}
                }
            }
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedGitOpsConfig(**config_data)
    
    def test_pat_without_token(self):
        """Test PAT auth without token."""
        config_data = {
            "repository": {
                "url": "https://github.com/user/repo.git",
                "branch": "main",
                "auth": {
                    "method": "pat",
                    "pat": {}
                }
            }
        }
        
        with pytest.raises(Exception):  # Pydantic validation error
            ValidatedGitOpsConfig(**config_data)


class TestConfigValidator:
    """Tests for ConfigValidator class."""
    
    def test_valid_configuration(self):
        """Test validation of valid configuration."""
        config_dict = {
            "schema_version": "1.0.0",
            "storage": {
                "type": "minio",
                "endpoint": "localhost:9000",
                "access_key": "minioadmin",
                "secret_key": "minioadmin",
                "bucket": "test-bucket",
                "use_ssl": False,
                "region": "us-east-1"
            },
            "backup": {
                "behavior": {
                    "batch_size": 50,
                    "validate_yaml": True
                },
                "cleanup": {
                    "enabled": True,
                    "retention_days": 7
                }
            },
            "gitops": {
                "repository": {
                    "url": "git@github.com:user/repo.git",
                    "branch": "main",
                    "auth": {
                        "method": "ssh",
                        "ssh": {
                            "private_key_path": "/home/user/.ssh/id_rsa"
                        }
                    }
                }
            },
            "pipeline": {
                "mode": "sequential",
                "automation": {
                    "enabled": True,
                    "trigger_methods": ["file", "process"]
                }
            }
        }
        
        validator = ConfigValidator(config_dict)
        result = validator.validate()
        
        assert result.valid is True
        assert len(result.errors) == 0
    
    def test_invalid_configuration(self):
        """Test validation of invalid configuration."""
        config_dict = {
            "storage": {
                "type": "invalid-type",
                "endpoint": "",  # required field empty
                "bucket": "INVALID_BUCKET"  # invalid bucket name
            },
            "gitops": {
                "repository": {
                    "url": "invalid-url",
                    "auth": {
                        "method": "ssh",
                        "ssh": {}  # missing private key
                    }
                }
            }
        }
        
        validator = ConfigValidator(config_dict)
        result = validator.validate()
        
        assert result.valid is False
        assert len(result.errors) > 0
    
    def test_cross_field_validation(self):
        """Test cross-field validation rules."""
        config_dict = {
            "storage": {
                "type": "s3",
                "auto_create_bucket": True,
                "endpoint": "https://s3.amazonaws.com",
                "access_key": "test",
                "secret_key": "test",
                "bucket": "test-bucket",
                "region": "us-west-2"
            },
            "gitops": {
                "structure": {
                    "argocd": {
                        "enabled": True
                    }
                }
                # Missing repository URL when ArgoCD is enabled
            }
        }
        
        validator = ConfigValidator(config_dict)
        result = validator.validate()
        
        # Should have at least one error (missing Git URL) and one warning (S3 permissions)
        assert len(result.errors) >= 1
        assert len(result.warnings) >= 1


class TestValidateConfigFunction:
    """Tests for the main validate_config function."""
    
    def test_validate_config_valid(self):
        """Test validate_config with valid configuration."""
        config_dict = {
            "storage": {
                "type": "minio",
                "endpoint": "localhost:9000",
                "access_key": "minioadmin",
                "secret_key": "minioadmin",
                "bucket": "test-bucket"
            },
            "gitops": {
                "repository": {
                    "url": "git@github.com:user/repo.git",
                    "branch": "main",
                    "auth": {
                        "method": "ssh",
                        "ssh": {
                            "private_key_path": "/home/user/.ssh/id_rsa"
                        }
                    }
                }
            }
        }
        
        result = validate_config(config_dict)
        assert result.valid is True
    
    def test_validate_config_invalid(self):
        """Test validate_config with invalid configuration."""
        config_dict = {
            "storage": {
                "type": "invalid",
                "endpoint": "",
                "access_key": "",
                "secret_key": "",
                "bucket": ""
            }
        }
        
        result = validate_config(config_dict)
        assert result.valid is False
        assert len(result.errors) > 0


class TestValidateConfigFile:
    """Tests for validate_config_file function."""
    
    def test_validate_config_file_not_exists(self):
        """Test validation of non-existent file."""
        result = validate_config_file("/non/existent/file.yaml")
        assert result.valid is False
        assert len(result.errors) == 1
        assert "does not exist" in result.errors[0].message
    
    def test_validate_config_file_invalid_yaml(self, tmp_path):
        """Test validation of invalid YAML file."""
        config_file = tmp_path / "invalid.yaml"
        config_file.write_text("invalid: yaml: content: [")
        
        result = validate_config_file(str(config_file))
        assert result.valid is False
        assert "Failed to parse YAML" in result.errors[0].message
    
    def test_validate_config_file_valid(self, tmp_path):
        """Test validation of valid YAML file."""
        config_data = {
            "storage": {
                "type": "minio",
                "endpoint": "localhost:9000",
                "access_key": "minioadmin",
                "secret_key": "minioadmin",
                "bucket": "test-bucket"
            },
            "gitops": {
                "repository": {
                    "url": "git@github.com:user/repo.git",
                    "branch": "main",
                    "auth": {
                        "method": "ssh",
                        "ssh": {
                            "private_key_path": "/home/user/.ssh/id_rsa"
                        }
                    }
                }
            }
        }
        
        config_file = tmp_path / "valid.yaml"
        config_file.write_text(yaml.dump(config_data))
        
        result = validate_config_file(str(config_file))
        assert result.valid is True


class TestPerformanceValidation:
    """Tests for performance-related validation."""
    
    def test_large_batch_size_warning(self):
        """Test warning for large batch sizes."""
        config_dict = {
            "backup": {
                "behavior": {
                    "batch_size": 800  # Large batch size
                }
            }
        }
        
        validator = ConfigValidator(config_dict)
        result = validator.validate()
        
        # Should have a warning about large batch size
        warning_messages = [w.message for w in result.warnings]
        assert any("performance issues" in msg for msg in warning_messages)
    
    def test_zero_retention_warning(self):
        """Test warning for zero retention days."""
        config_dict = {
            "backup": {
                "cleanup": {
                    "enabled": True,
                    "retention_days": 0
                }
            }
        }
        
        validator = ConfigValidator(config_dict)
        result = validator.validate()
        
        # Should have a warning about immediate deletion
        warning_messages = [w.message for w in result.warnings]
        assert any("immediate deletion" in msg for msg in warning_messages)


class TestEdgeCases:
    """Tests for edge cases and error conditions."""
    
    def test_empty_config(self):
        """Test validation of empty configuration."""
        config_dict = {}
        
        result = validate_config(config_dict)
        # Empty config should be invalid due to missing required fields
        assert result.valid is False
    
    def test_partial_config(self):
        """Test validation of partial configuration."""
        config_dict = {
            "storage": {
                "type": "minio",
                "endpoint": "localhost:9000"
                # Missing required fields
            }
        }
        
        result = validate_config(config_dict)
        assert result.valid is False
        assert len(result.errors) > 0
    
    def test_nested_validation_errors(self):
        """Test that nested validation errors are properly reported."""
        config_dict = {
            "storage": {
                "connection": {
                    "timeout": -1,  # Invalid negative timeout
                    "max_retries": -5  # Invalid negative retries
                }
            }
        }
        
        result = validate_config(config_dict)
        assert result.valid is False
        
        # Check that field paths are properly formatted
        error_fields = [e.field for e in result.errors]
        assert any("storage" in field for field in error_fields)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])