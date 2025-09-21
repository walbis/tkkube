# Enhanced Schema Validation for Shared Configuration System

This document describes the comprehensive validation system implemented for the shared configuration used by both the backup and GitOps tools.

## Overview

The enhanced validation system provides:

- **Comprehensive validation rules** for all configuration sections
- **Cross-field validation** to ensure configuration consistency
- **Clear error reporting** with specific field paths and helpful messages
- **Warning system** for potential issues that don't prevent operation
- **Multi-language support** with both Go and Python implementations

## Features

### ✅ **Validation Coverage**

- **Storage Configuration**: Endpoint format, credentials, bucket names, S3/MinIO-specific rules
- **Cluster Configuration**: Cluster names, types, domain validation
- **Backup Configuration**: Filtering modes, batch sizes, retention policies
- **GitOps Configuration**: Repository URLs, authentication methods, branch names
- **Pipeline Configuration**: Automation settings, trigger methods
- **Security Configuration**: Secret providers, validation settings
- **Performance Configuration**: Resource limits, optimization settings
- **Cross-field Rules**: ArgoCD dependencies, S3 permissions, automation requirements

### 🔧 **Validation Types**

1. **Required Field Validation**: Ensures essential fields are present
2. **Format Validation**: Validates URLs, domains, bucket names, etc.
3. **Range Validation**: Checks numeric ranges and limits
4. **Enum Validation**: Validates against allowed values
5. **Cross-field Validation**: Ensures configuration consistency
6. **Warning Generation**: Identifies potential issues

## Usage

### Go Implementation

```go
package main

import (
    "fmt"
    sharedconfig "shared-config/config"
)

func main() {
    // Load configuration with validation
    loader := sharedconfig.NewConfigLoader()
    config, err := loader.Load()
    if err != nil {
        fmt.Printf("Configuration error: %v\n", err)
        return
    }
    
    // Manual validation
    result, err := sharedconfig.ValidateConfig(config)
    if err != nil {
        fmt.Printf("Validation failed: %v\n", err)
        return
    }
    
    if !result.Valid {
        fmt.Printf("Configuration issues:\n%s\n", sharedconfig.FormatValidationResult(result))
        return
    }
    
    fmt.Println("✅ Configuration is valid!")
}
```

### Python Implementation

```python
from config.loader import ConfigLoader
from config.simple_validator import validate_config

# Load and validate configuration
loader = ConfigLoader()
try:
    config = loader.load()  # Automatically validates
    print("✅ Configuration loaded successfully")
except ValueError as e:
    print(f"❌ Configuration error: {e}")

# Manual validation
config_dict = {
    "storage": {
        "type": "minio",
        "endpoint": "localhost:9000",
        "access_key": "minioadmin",
        "secret_key": "minioadmin",
        "bucket": "test-bucket"
    }
}

result = validate_config(config_dict)
if result.valid:
    print("✅ Configuration is valid")
else:
    print(f"❌ Validation failed:\n{result.format_result()}")
```

## Validation Rules

### Storage Configuration

| Field | Rule | Example Error |
|-------|------|---------------|
| `type` | Must be 'minio' or 's3' | "Storage type must be 'minio' or 's3'" |
| `endpoint` | Valid host:port or URL | "Invalid endpoint format" |
| `bucket` | Valid S3/MinIO bucket name | "Invalid bucket name (3-63 chars, lowercase)" |
| `region` | Required for S3 | "Region is required for S3 storage" |
| `access_key` | Non-empty | "Storage access key is required" |
| `secret_key` | Non-empty | "Storage secret key is required" |

### GitOps Configuration

| Field | Rule | Example Error |
|-------|------|---------------|
| `repository.url` | Valid Git URL | "Invalid Git repository URL format" |
| `repository.branch` | Valid branch name | "Invalid Git branch name" |
| `auth.method` | ssh, pat, basic, none | "Invalid authentication method" |
| `auth.ssh.private_key_path` | Required for SSH | "SSH private key path is required" |
| `auth.pat.token` | Required for PAT | "PAT token is required" |

### Pipeline Configuration

| Field | Rule | Example Error |
|-------|------|---------------|
| `mode` | sequential, parallel, manual | "Invalid pipeline mode" |
| `automation.trigger_methods` | Valid methods array | "Invalid trigger method" |
| `automation.max_wait_time` | Positive integer | "Max wait time must be positive" |

### Cross-field Rules

| Rule | Description | Example |
|------|-------------|---------|
| ArgoCD Repository | ArgoCD enabled requires Git URL | "Git repository URL required when ArgoCD enabled" |
| S3 Auto-create | Warning for S3 bucket creation | "Ensure AWS credentials have bucket permissions" |
| Automation Methods | Enabled automation needs methods | "At least one trigger method required" |

## Error Examples

### ❌ **Validation Errors** (Block execution)

```
❌ Configuration validation failed with 3 error(s):

  ❌ storage.endpoint: Storage endpoint is required
  ❌ storage.bucket: Invalid bucket name (must be 3-63 characters, lowercase)
     Current value: INVALID_BUCKET_NAME
  ❌ gitops.repository.url: Invalid Git repository URL format
     Current value: invalid-url
```

### ⚠️ **Validation Warnings** (Allow execution)

```
✅ Configuration is valid

⚠️  2 warning(s):
  - backup.behavior.batch_size: Large batch size may cause performance issues
  - backup.cleanup.retention_days: Zero retention days means immediate deletion
```

## Configuration Examples

### ✅ **Valid Configuration**

```yaml
# Complete valid configuration
schema_version: "1.0.0"
storage:
  type: "minio"
  endpoint: "localhost:9000"
  access_key: "minioadmin"
  secret_key: "minioadmin"
  bucket: "cluster-backups"
  use_ssl: false

cluster:
  name: "production-cluster"
  domain: "cluster.local"
  type: "kubernetes"

backup:
  behavior:
    batch_size: 50
    validate_yaml: true
  cleanup:
    enabled: true
    retention_days: 7

gitops:
  repository:
    url: "git@github.com:company/cluster-config.git"
    branch: "main"
    auth:
      method: "ssh"
      ssh:
        private_key_path: "/home/user/.ssh/id_rsa"

pipeline:
  mode: "sequential"
  automation:
    enabled: true
    trigger_methods: ["file", "process"]
```

### ❌ **Invalid Configuration**

```yaml
# Configuration with errors
storage:
  type: "invalid-storage"  # ❌ Invalid type
  endpoint: ""             # ❌ Required field empty
  bucket: "INVALID_BUCKET" # ❌ Invalid bucket name

gitops:
  repository:
    url: "not-a-git-url"   # ❌ Invalid URL format
    auth:
      method: "ssh"
      ssh: {}              # ❌ Missing private key path

pipeline:
  automation:
    enabled: true
    trigger_methods: []    # ❌ Empty when automation enabled
```

## Integration with Configuration Loaders

The validation system is automatically integrated into both Go and Python configuration loaders:

### Go Integration

```go
// In loader.go
func (cl *ConfigLoader) validate(config *SharedConfig) error {
    validationResult, err := ValidateConfig(config)
    if err != nil {
        return err
    }
    
    // Print warnings
    if len(validationResult.Warnings) > 0 {
        fmt.Printf("Configuration loaded with warnings:\n%s\n", 
                   FormatValidationResult(validationResult))
    }
    
    if !validationResult.Valid {
        return fmt.Errorf("configuration validation failed:\n%s", 
                         FormatValidationResult(validationResult))
    }
    
    return nil
}
```

### Python Integration

```python
# In loader.py
def _validate(self, config: SharedConfig) -> None:
    from .simple_validator import validate_config
    
    config_dict = self.to_dict(config)
    validation_result = validate_config(config_dict)
    
    if not validation_result.valid:
        raise ValueError(f"Configuration validation failed:\n{validation_result.format_result()}")
    
    if validation_result.warnings:
        print(f"Configuration loaded with warnings:\n{validation_result.format_result()}")
```

## Testing

### Go Tests

```bash
# Run validation tests
go test ./config -v -run TestConfigValidator

# Run all tests
go test ./config -v
```

### Python Tests

```bash
# Test the simple validator
python3 config/simple_validator.py

# Test with specific configuration
python3 -c "
from config.simple_validator import validate_config
result = validate_config({'storage': {'type': 'minio'}})
print(result.format_result())
"
```

## Performance Considerations

- **Fast Validation**: Basic validation completes in <1ms for typical configurations
- **Memory Efficient**: Minimal memory overhead during validation
- **Error Batching**: Collects all errors in a single pass
- **Lazy Loading**: Validation only runs when configuration is loaded

## Customization

### Adding New Validation Rules

#### Go Implementation

```go
// Add to validator.go
func (cv *ConfigValidator) validateNewSection() {
    // Add validation logic
    if condition {
        cv.addError("section.field", value, "Error message")
    }
}

// Add to Validate() method
func (cv *ConfigValidator) Validate() *ValidationResult {
    // ... existing validations
    cv.validateNewSection()
    // ...
}
```

#### Python Implementation

```python
# Add to simple_validator.py
def _validate_new_section(self):
    """Validate new configuration section."""
    section = self.config_dict.get('new_section', {})
    
    if not section.get('required_field'):
        self.result.add_error("new_section.required_field", "", "Required field missing")

# Add to validate() method
def validate(self) -> ValidationResult:
    # ... existing validations
    self._validate_new_section()
    # ...
```

## Troubleshooting

### Common Issues

1. **"Module not found" errors**: 
   - Use `simple_validator.py` instead of `validator.py` if Pydantic is not available
   - Install dependencies: `pip install pydantic` (for advanced features)

2. **Path errors in validation**:
   - Check that file paths in configuration are absolute
   - Verify SSH key paths exist and are readable

3. **Network validation failures**:
   - Ensure endpoints are reachable
   - Check firewall settings for storage endpoints

### Debug Mode

Enable debug output for detailed validation information:

```bash
# Go
export LOG_LEVEL=debug

# Python
import logging
logging.basicConfig(level=logging.DEBUG)
```

## Future Enhancements

- [ ] **JSON Schema Support**: Generate JSON schemas for external validation
- [ ] **Custom Validators**: Plugin system for custom validation rules
- [ ] **Performance Metrics**: Validation timing and statistics
- [ ] **Configuration Templates**: Pre-validated configuration templates
- [ ] **Real-time Validation**: Watch file changes and re-validate automatically