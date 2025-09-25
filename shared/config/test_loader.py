#!/usr/bin/env python3
"""
Tests for the shared configuration loader.
"""

import os
import tempfile
import yaml
from pathlib import Path
from loader import (
    ConfigLoader, SharedConfig, StorageConfig, SingleClusterConfig,
    BackupConfig, GitOpsConfig, get_gitops_config_from_shared
)

class TestRunner:
    """Simple test runner to replace pytest."""
    
    def __init__(self):
        self.tests_run = 0
        self.tests_passed = 0
        self.tests_failed = 0
    
    def assert_equal(self, actual, expected, message=""):
        if actual != expected:
            raise AssertionError(f"{message}: Expected {expected}, got {actual}")
    
    def assert_true(self, condition, message=""):
        if not condition:
            raise AssertionError(f"{message}: Expected True, got False")
    
    def assert_raises(self, exception_type, func, *args, **kwargs):
        try:
            func(*args, **kwargs)
            raise AssertionError(f"Expected {exception_type.__name__} to be raised")
        except exception_type:
            pass  # Expected exception
        except Exception as e:
            raise AssertionError(f"Expected {exception_type.__name__}, got {type(e).__name__}: {e}")
    
    def run_test(self, test_func):
        self.tests_run += 1
        try:
            test_func()
            self.tests_passed += 1
            print(f"✓ {test_func.__name__}")
        except Exception as e:
            self.tests_failed += 1
            print(f"✗ {test_func.__name__}: {e}")
    
    def report(self):
        print(f"\nTest Results: {self.tests_run} run, {self.tests_passed} passed, {self.tests_failed} failed")
        return self.tests_failed == 0

# Replace pytest.raises with a simple context manager
class raises:
    def __init__(self, exception_type):
        self.exception_type = exception_type
    
    def __enter__(self):
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        if exc_type is None:
            raise AssertionError(f"Expected {self.exception_type.__name__} to be raised")
        return isinstance(exc_val, self.exception_type)


class TestConfigLoader:
    """Test cases for ConfigLoader class."""
    
    def test_load_basic_config(self):
        """Test loading a basic configuration from file."""
        config_data = {
            'schema_version': '1.0.0',
            'description': 'Test configuration',
            'storage': {
                'type': 'minio',
                'endpoint': 'localhost:9000',
                'access_key': 'testkey',
                'secret_key': 'testsecret',
                'bucket': 'test-bucket',
                'use_ssl': False,
                'region': 'us-east-1'
            },
            'cluster': {
                'name': 'test-cluster',
                'domain': 'cluster.local',
                'type': 'kubernetes'
            },
            'backup': {
                'behavior': {
                    'batch_size': 25,
                    'validate_yaml': True
                },
                'cleanup': {
                    'enabled': True,
                    'retention_days': 14
                }
            },
            'gitops': {
                'repository': {
                    'url': 'https://github.com/test/repo.git',
                    'branch': 'main',
                    'auth': {
                        'method': 'ssh'
                    }
                }
            },
            'observability': {
                'logging': {
                    'level': 'debug',
                    'format': 'json'
                }
            }
        }
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(config_data, f)
            config_path = f.name
        
        try:
            loader = ConfigLoader([config_path])
            config = loader.load_without_validation()
            
            assert config.schema_version == '1.0.0'
            assert config.storage.endpoint == 'localhost:9000'
            assert config.storage.access_key == 'testkey'
            assert config.cluster.name == 'test-cluster'
            assert config.backup.behavior.batch_size == 25
            assert config.backup.cleanup.retention_days == 14
            assert config.observability.logging.level == 'debug'
            
        finally:
            os.unlink(config_path)
    
    def test_environment_overrides(self):
        """Test that environment variables override file values."""
        config_data = {
            'storage': {
                'endpoint': 'localhost:9000',
                'access_key': 'testkey',
                'secret_key': 'testsecret',
                'bucket': 'test-bucket'
            },
            'cluster': {
                'name': 'test-cluster'
            },
            'backup': {
                'behavior': {
                    'batch_size': 25
                },
                'cleanup': {
                    'retention_days': 7
                }
            }
        }
        
        # Set environment variables
        os.environ['MINIO_ENDPOINT'] = 'override.example.com:9000'
        os.environ['MINIO_ACCESS_KEY'] = 'override-key'
        os.environ['CLUSTER_NAME'] = 'override-cluster'
        os.environ['BATCH_SIZE'] = '100'
        
        try:
            with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
                yaml.dump(config_data, f)
                config_path = f.name
            
            try:
                loader = ConfigLoader([config_path])
                config = loader.load()
                
                assert config.storage.endpoint == 'override.example.com:9000'
                assert config.storage.access_key == 'override-key'
                assert config.cluster.name == 'override-cluster'
                assert config.backup.behavior.batch_size == 100
                
            finally:
                os.unlink(config_path)
                
        finally:
            # Clean up environment variables
            for var in ['MINIO_ENDPOINT', 'MINIO_ACCESS_KEY', 'CLUSTER_NAME', 'BATCH_SIZE']:
                os.environ.pop(var, None)
    
    def test_validation_errors(self):
        """Test configuration validation errors."""
        test_cases = [
            {
                'name': 'missing_endpoint',
                'config': {
                    'storage': {
                        'access_key': 'testkey',
                        'secret_key': 'testsecret',
                        'bucket': 'test-bucket'
                    }
                },
                'error_msg': 'Storage endpoint is required'
            },
            {
                'name': 'missing_access_key',
                'config': {
                    'storage': {
                        'endpoint': 'localhost:9000',
                        'secret_key': 'testsecret',
                        'bucket': 'test-bucket'
                    }
                },
                'error_msg': 'Storage access key is required'
            },
            {
                'name': 'invalid_batch_size',
                'config': {
                    'storage': {
                        'endpoint': 'localhost:9000',
                        'access_key': 'testkey',
                        'secret_key': 'testsecret',
                        'bucket': 'test-bucket'
                    },
                    'backup': {
                        'behavior': {
                            'batch_size': 2000
                        }
                    }
                },
                'error_msg': 'Batch size must be between 1 and 1000'
            },
            {
                'name': 'invalid_retention_days',
                'config': {
                    'storage': {
                        'endpoint': 'localhost:9000',
                        'access_key': 'testkey',
                        'secret_key': 'testsecret',
                        'bucket': 'test-bucket'
                    },
                    'backup': {
                        'cleanup': {
                            'retention_days': 500
                        }
                    }
                },
                'error_msg': 'Retention days must be between 1 and 365'
            }
        ]
        
        for test_case in test_cases:
            with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
                yaml.dump(test_case['config'], f)
                config_path = f.name
            
            try:
                loader = ConfigLoader([config_path])
                with raises(ValueError):
                    loader.load_without_validation()
                
            finally:
                os.unlink(config_path)
    
    def test_valid_configuration(self):
        """Test loading a valid configuration without errors."""
        config_data = {
            'storage': {
                'endpoint': 'localhost:9000',
                'access_key': 'testkey',
                'secret_key': 'testsecret',
                'bucket': 'test-bucket'
            },
            'backup': {
                'behavior': {
                    'batch_size': 50
                },
                'cleanup': {
                    'retention_days': 7
                }
            }
        }
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(config_data, f)
            config_path = f.name
        
        try:
            loader = ConfigLoader([config_path])
            config = loader.load_without_validation()  # Should not raise an exception
            
            assert config.storage.endpoint == 'localhost:9000'
            assert config.backup.behavior.batch_size == 50
            
        finally:
            os.unlink(config_path)
    
    def test_save_configuration(self):
        """Test saving configuration to file."""
        config = SharedConfig(
            schema_version='1.0.0',
            description='Test configuration for saving'
        )
        config.storage.endpoint = 'localhost:9000'
        config.storage.access_key = 'testkey'
        config.storage.secret_key = 'testsecret'
        config.storage.bucket = 'test-bucket'
        config.cluster.name = 'test-cluster'
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            save_path = f.name
        
        try:
            loader = ConfigLoader()
            loader.save_to_file(config, save_path)
            
            # Verify file was created and can be loaded back
            loader2 = ConfigLoader([save_path])
            loaded_config = loader2.load_without_validation()
            
            assert loaded_config.schema_version == config.schema_version
            assert loaded_config.storage.endpoint == config.storage.endpoint
            assert loaded_config.cluster.name == config.cluster.name
            
        finally:
            os.unlink(save_path)
    
    def test_default_config_paths(self):
        """Test default configuration paths."""
        loader = ConfigLoader()
        paths = loader.config_paths
        
        assert len(paths) > 0
        
        expected_paths = [
            './shared-config.yaml',
            './config/shared-config.yaml',
            '/etc/backup-gitops/config.yaml'
        ]
        
        for expected in expected_paths:
            assert expected in paths
    
    def test_environment_variable_expansion(self):
        """Test expansion of environment variables in configuration."""
        os.environ['TEST_ENDPOINT'] = 'expanded.example.com'
        os.environ['TEST_BUCKET'] = 'expanded-bucket'
        
        try:
            config_data = {
                'storage': {
                    'endpoint': '${TEST_ENDPOINT}:9000',
                    'access_key': 'testkey',
                    'secret_key': 'testsecret',
                    'bucket': '${TEST_BUCKET}'
                }
            }
            
            with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
                yaml.dump(config_data, f)
                config_path = f.name
            
            try:
                loader = ConfigLoader([config_path])
                config = loader.load()
                
                assert config.storage.endpoint == 'expanded.example.com:9000'
                assert config.storage.bucket == 'expanded-bucket'
                
            finally:
                os.unlink(config_path)
                
        finally:
            os.environ.pop('TEST_ENDPOINT', None)
            os.environ.pop('TEST_BUCKET', None)


class TestGitOpsConfigConversion:
    """Test cases for GitOps configuration conversion."""
    
    def test_get_gitops_config_from_shared(self):
        """Test conversion from shared config to GitOps specific config."""
        shared_config = SharedConfig()
        shared_config.storage.endpoint = 'localhost:9000'
        shared_config.storage.access_key = 'testkey'
        shared_config.storage.secret_key = 'testsecret'
        shared_config.storage.bucket = 'test-bucket'
        shared_config.storage.use_ssl = False
        shared_config.cluster.name = 'test-cluster'
        shared_config.cluster.domain = 'cluster.local'
        shared_config.gitops.repository.url = 'https://github.com/test/repo.git'
        shared_config.gitops.repository.auth.method = 'ssh'
        shared_config.gitops.repository.auth.ssh.private_key_path = '~/.ssh/id_rsa'
        
        gitops_config = get_gitops_config_from_shared(shared_config)
        
        # Verify MinIO configuration
        assert gitops_config['minio']['endpoint'] == 'localhost:9000'
        assert gitops_config['minio']['access_key'] == 'testkey'
        assert gitops_config['minio']['secret_key'] == 'testsecret'
        assert gitops_config['minio']['bucket'] == 'test-bucket'
        assert gitops_config['minio']['secure'] == False
        assert gitops_config['minio']['prefix'] == 'test-cluster/cluster.local'
        
        # Verify Git configuration
        assert gitops_config['git']['repository'] == 'https://github.com/test/repo.git'
        assert gitops_config['git']['auth_method'] == 'ssh'
        assert gitops_config['git']['ssh']['private_key_path'] == '~/.ssh/id_rsa'


if __name__ == '__main__':
    # Run tests using custom test runner
    runner = TestRunner()
    
    # Create test instance
    test_loader = TestConfigLoader()
    test_gitops = TestGitOpsConfigConversion()
    
    # Run all test methods
    runner.run_test(test_loader.test_load_basic_config)
    runner.run_test(test_loader.test_environment_overrides)
    runner.run_test(test_loader.test_validation_errors)
    runner.run_test(test_loader.test_valid_configuration)
    runner.run_test(test_loader.test_save_configuration)
    runner.run_test(test_loader.test_default_config_paths)
    runner.run_test(test_loader.test_environment_variable_expansion)
    runner.run_test(test_gitops.test_get_gitops_config_from_shared)
    
    # Print results
    success = runner.report()
    exit(0 if success else 1)