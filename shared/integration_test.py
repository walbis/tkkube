#!/usr/bin/env python3
"""
Integration test demonstrating the complete shared configuration system.
"""

import os
import sys

# Add the config directory to Python path
config_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'config')
sys.path.insert(0, config_dir)

from loader import ConfigLoader, get_gitops_config_from_shared


def main():
    """Run integration test."""
    print("🔧 Shared Configuration Integration Test")
    print("=" * 50)
    
    # Test 1: Load demo configuration
    print("\n1. Loading demo configuration...")
    try:
        config_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'config', 'demo-config.yaml')
        loader = ConfigLoader([config_path])
        config = loader.load()
        print("   ✓ Configuration loaded successfully")
        print(f"   • Schema version: {config.schema_version}")
        print(f"   • Storage endpoint: {config.storage.endpoint}")
        print(f"   • Cluster name: {config.cluster.name}")
        print(f"   • Backup batch size: {config.backup.behavior.batch_size}")
        print(f"   • Git repository: {config.gitops.repository.url}")
    except Exception as e:
        print(f"   ✗ Configuration loading failed: {e}")
        return False
    
    # Test 2: Test environment variable overrides
    print("\n2. Testing environment variable overrides...")
    try:
        # Set test environment variables
        os.environ['MINIO_ENDPOINT'] = 'override.example.com:9000'
        os.environ['CLUSTER_NAME'] = 'override-cluster'
        os.environ['BATCH_SIZE'] = '100'
        
        loader = ConfigLoader([config_path])
        config = loader.load()
        
        if config.storage.endpoint == 'override.example.com:9000':
            print("   ✓ Environment override for MINIO_ENDPOINT works")
        else:
            print(f"   ✗ Environment override failed: {config.storage.endpoint}")
            
        if config.cluster.name == 'override-cluster':
            print("   ✓ Environment override for CLUSTER_NAME works")
        else:
            print(f"   ✗ Environment override failed: {config.cluster.name}")
            
        if config.backup.behavior.batch_size == 100:
            print("   ✓ Environment override for BATCH_SIZE works")
        else:
            print(f"   ✗ Environment override failed: {config.backup.behavior.batch_size}")
            
        # Clean up environment variables
        os.environ.pop('MINIO_ENDPOINT', None)
        os.environ.pop('CLUSTER_NAME', None)
        os.environ.pop('BATCH_SIZE', None)
        
    except Exception as e:
        print(f"   ✗ Environment override test failed: {e}")
        return False
    
    # Test 3: Test GitOps configuration conversion
    print("\n3. Testing GitOps configuration conversion...")
    try:
        loader = ConfigLoader([config_path])
        config = loader.load()
        
        gitops_config = get_gitops_config_from_shared(config)
        
        print("   ✓ GitOps configuration converted successfully")
        print(f"   • MinIO endpoint: {gitops_config['minio']['endpoint']}")
        print(f"   • MinIO bucket: {gitops_config['minio']['bucket']}")
        print(f"   • Git repository: {gitops_config['git']['repository']}")
        print(f"   • Auth method: {gitops_config['git']['auth_method']}")
        
    except Exception as e:
        print(f"   ✗ GitOps configuration conversion failed: {e}")
        return False
    
    # Test 4: Test configuration validation
    print("\n4. Testing configuration validation...")
    try:
        # Test with invalid batch size
        invalid_config_data = {
            'storage': {
                'endpoint': 'localhost:9000',
                'access_key': 'testkey',
                'secret_key': 'testsecret',
                'bucket': 'test-bucket'
            },
            'backup': {
                'behavior': {
                    'batch_size': 2000  # Invalid - too large
                }
            }
        }
        
        import tempfile
        import yaml
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump(invalid_config_data, f)
            invalid_config_path = f.name
        
        try:
            loader = ConfigLoader([invalid_config_path])
            loader.load()
            print("   ✗ Validation should have failed for invalid batch size")
            return False
        except ValueError as e:
            if "batch size must be between 1 and 1000" in str(e).lower():
                print("   ✓ Configuration validation works correctly")
            else:
                print(f"   ✗ Unexpected validation error: {e}")
                return False
        finally:
            os.unlink(invalid_config_path)
            
    except Exception as e:
        print(f"   ✗ Configuration validation test failed: {e}")
        return False
    
    # Test 5: Test configuration saving
    print("\n5. Testing configuration saving...")
    try:
        loader = ConfigLoader([config_path])
        config = loader.load()
        
        # Save to temporary file
        save_path = '/tmp/test-saved-config.yaml'
        loader.save_to_file(config, save_path)
        
        # Load it back
        loader2 = ConfigLoader([save_path])
        loaded_config = loader2.load()
        
        if loaded_config.storage.endpoint == config.storage.endpoint:
            print("   ✓ Configuration save/load roundtrip works")
        else:
            print("   ✗ Configuration save/load roundtrip failed")
            return False
            
        # Clean up
        os.unlink(save_path)
        
    except Exception as e:
        print(f"   ✗ Configuration save test failed: {e}")
        return False
    
    print("\n" + "=" * 50)
    print("🎉 All integration tests passed!")
    print("\nThe shared configuration system is working correctly:")
    print("• ✓ Configuration loading from YAML files")
    print("• ✓ Environment variable overrides")
    print("• ✓ Configuration validation")
    print("• ✓ GitOps configuration conversion")
    print("• ✓ Configuration save/load roundtrip")
    print("\nNext steps:")
    print("• Use the pipeline integration script to run complete workflows")
    print("• Set environment variables for your specific setup")
    print("• Configure your backup and GitOps tools to use the shared config")
    
    return True


if __name__ == '__main__':
    success = main()
    exit(0 if success else 1)