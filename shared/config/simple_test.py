#!/usr/bin/env python3
"""
Simple test to validate Python configuration loader functionality.
"""

import os
import tempfile
import yaml
from loader import ConfigLoader, SharedConfig


def test_basic_loading():
    """Test basic configuration loading."""
    print("Testing basic configuration loading...")
    
    # Create a test config
    config_data = {
        'storage': {
            'endpoint': 'localhost:9000',
            'access_key': 'testkey',
            'secret_key': 'testsecret',
            'bucket': 'test-bucket'
        },
        'cluster': {
            'name': 'test-cluster'
        }
    }
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        yaml.dump(config_data, f)
        config_path = f.name
    
    try:
        loader = ConfigLoader([config_path])
        config = loader.load()
        
        assert config.storage.endpoint == 'localhost:9000'
        assert config.storage.access_key == 'testkey'
        assert config.cluster.name == 'test-cluster'
        
        print("✓ Basic loading test passed")
        
    finally:
        os.unlink(config_path)


def test_validation_errors():
    """Test validation error handling."""
    print("Testing validation errors...")
    
    # Test missing endpoint
    config_data = {
        'storage': {
            'access_key': 'testkey',
            'secret_key': 'testsecret',
            'bucket': 'test-bucket'
        }
    }
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        yaml.dump(config_data, f)
        config_path = f.name
    
    try:
        loader = ConfigLoader([config_path])
        try:
            config = loader.load()
            print("✗ Expected validation error but none was raised")
            return False
        except ValueError as e:
            if "Storage endpoint is required" in str(e):
                print("✓ Validation error test passed")
                return True
            else:
                print(f"✗ Unexpected error: {e}")
                return False
        
    finally:
        os.unlink(config_path)


def test_environment_overrides():
    """Test environment variable overrides."""
    print("Testing environment overrides...")
    
    # Set environment variable
    os.environ['MINIO_ENDPOINT'] = 'override.example.com:9000'
    
    try:
        config_data = {
            'storage': {
                'endpoint': 'localhost:9000',
                'access_key': 'testkey',
                'secret_key': 'testsecret',
                'bucket': 'test-bucket'
            }
        }
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(config_data, f)
            config_path = f.name
        
        try:
            loader = ConfigLoader([config_path])
            config = loader.load()
            
            if config.storage.endpoint == 'override.example.com:9000':
                print("✓ Environment override test passed")
                return True
            else:
                print(f"✗ Environment override failed: got {config.storage.endpoint}")
                return False
            
        finally:
            os.unlink(config_path)
            
    finally:
        os.environ.pop('MINIO_ENDPOINT', None)


def main():
    """Run all tests."""
    print("Running Python configuration loader tests...\n")
    
    tests_passed = 0
    total_tests = 3
    
    try:
        test_basic_loading()
        tests_passed += 1
    except Exception as e:
        print(f"✗ Basic loading test failed: {e}")
    
    try:
        if test_validation_errors():
            tests_passed += 1
    except Exception as e:
        print(f"✗ Validation test failed: {e}")
    
    try:
        if test_environment_overrides():
            tests_passed += 1
    except Exception as e:
        print(f"✗ Environment override test failed: {e}")
    
    print(f"\nTest Results: {tests_passed}/{total_tests} tests passed")
    
    if tests_passed == total_tests:
        print("All Python tests passed! ✓")
        return True
    else:
        print("Some Python tests failed! ✗")
        return False


if __name__ == '__main__':
    success = main()
    exit(0 if success else 1)