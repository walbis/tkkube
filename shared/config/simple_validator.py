"""
Simple configuration validation without external dependencies.
This provides basic validation for the shared configuration system.
"""

import re
from typing import Dict, Any, List, Optional, Union
from urllib.parse import urlparse


class ValidationError:
    """Represents a single validation error or warning."""
    
    def __init__(self, field: str, value: Any, message: str, level: str = "error"):
        self.field = field
        self.value = value
        self.message = message
        self.level = level  # "error" or "warning"


class ValidationResult:
    """Container for validation results including errors and warnings."""
    
    def __init__(self):
        self.errors: List[ValidationError] = []
        self.warnings: List[ValidationError] = []
        self.valid: bool = True
    
    def add_error(self, field: str, value: Any, message: str):
        """Add a validation error."""
        self.errors.append(ValidationError(field, value, message, "error"))
        self.valid = False
    
    def add_warning(self, field: str, value: Any, message: str):
        """Add a validation warning."""
        self.warnings.append(ValidationError(field, value, message, "warning"))
    
    def format_result(self) -> str:
        """Format the validation result for display."""
        output = []
        
        if self.valid:
            output.append("✅ Configuration is valid")
            if self.warnings:
                output.append(f"\n⚠️  {len(self.warnings)} warning(s):")
                for warning in self.warnings:
                    output.append(f"  - {warning.field}: {warning.message}")
        else:
            output.append(f"❌ Configuration validation failed with {len(self.errors)} error(s):\n")
            for error in self.errors:
                output.append(f"  ❌ {error.field}: {error.message}")
                if error.value is not None and error.value != "":
                    output.append(f"     Current value: {error.value}")
            
            if self.warnings:
                output.append(f"\n⚠️  {len(self.warnings)} warning(s):")
                for warning in self.warnings:
                    output.append(f"  - {warning.field}: {warning.message}")
        
        return "\n".join(output)


class SimpleConfigValidator:
    """Simple configuration validator without external dependencies."""
    
    def __init__(self, config_dict: Dict[str, Any]):
        self.config_dict = config_dict
        self.result = ValidationResult()
    
    def validate(self) -> ValidationResult:
        """Perform configuration validation."""
        self._validate_storage()
        self._validate_cluster()
        self._validate_backup()
        self._validate_gitops()
        self._validate_pipeline()
        self._validate_cross_field_rules()
        
        return self.result
    
    def _validate_storage(self):
        """Validate storage configuration."""
        storage = self.config_dict.get('storage', {})
        
        # Required fields
        required_fields = ['type', 'endpoint', 'access_key', 'secret_key', 'bucket']
        for field in required_fields:
            if not storage.get(field):
                self.result.add_error(f"storage.{field}", storage.get(field), f"Storage {field} is required")
        
        # Validate storage type
        storage_type = storage.get('type', '')
        if storage_type and storage_type not in ['minio', 's3']:
            self.result.add_error("storage.type", storage_type, "Storage type must be 'minio' or 's3'")
        
        # Validate endpoint format
        endpoint = storage.get('endpoint', '')
        if endpoint and not self._is_valid_endpoint(endpoint):
            self.result.add_error("storage.endpoint", endpoint, "Invalid endpoint format")
        
        # Validate bucket name
        bucket = storage.get('bucket', '')
        if bucket and not self._is_valid_bucket_name(bucket):
            self.result.add_error("storage.bucket", bucket, "Invalid bucket name format")
        
        # S3 specific validation
        if storage_type == 's3' and not storage.get('region'):
            self.result.add_error("storage.region", storage.get('region'), "Region is required for S3 storage")
    
    def _validate_cluster(self):
        """Validate cluster configuration."""
        cluster = self.config_dict.get('cluster', {})
        
        # Required cluster name
        if not cluster.get('name'):
            self.result.add_error("cluster.name", cluster.get('name'), "Cluster name is required")
        
        # Validate cluster type
        cluster_type = cluster.get('type', 'kubernetes')
        if cluster_type not in ['kubernetes', 'openshift']:
            self.result.add_error("cluster.type", cluster_type, "Cluster type must be 'kubernetes' or 'openshift'")
        
        # Validate domain format
        domain = cluster.get('domain', '')
        if domain and not self._is_valid_domain(domain):
            self.result.add_error("cluster.domain", domain, "Invalid domain format")
    
    def _validate_backup(self):
        """Validate backup configuration."""
        backup = self.config_dict.get('backup', {})
        
        # Validate filtering mode
        filtering = backup.get('filtering', {})
        mode = filtering.get('mode', 'whitelist')
        if mode not in ['whitelist', 'blacklist', 'hybrid']:
            self.result.add_error("backup.filtering.mode", mode, "Invalid filtering mode")
        
        # Validate behavior settings
        behavior = backup.get('behavior', {})
        batch_size = behavior.get('batch_size', 50)
        if batch_size <= 0 or batch_size > 1000:
            self.result.add_error("backup.behavior.batch_size", batch_size, "Batch size must be between 1 and 1000")
        elif batch_size > 500:
            self.result.add_warning("backup.behavior.batch_size", batch_size, "Large batch size may cause performance issues")
        
        # Validate cleanup settings
        cleanup = backup.get('cleanup', {})
        retention_days = cleanup.get('retention_days', 7)
        if retention_days < 0:
            self.result.add_error("backup.cleanup.retention_days", retention_days, "Retention days cannot be negative")
        elif retention_days == 0:
            self.result.add_warning("backup.cleanup.retention_days", retention_days, "Zero retention days means immediate deletion")
    
    def _validate_gitops(self):
        """Validate GitOps configuration."""
        gitops = self.config_dict.get('gitops', {})
        repository = gitops.get('repository', {})
        
        # Required repository URL
        url = repository.get('url', '')
        if not url:
            self.result.add_error("gitops.repository.url", url, "Git repository URL is required")
        elif not self._is_valid_git_url(url):
            self.result.add_error("gitops.repository.url", url, "Invalid Git repository URL format")
        
        # Validate branch name
        branch = repository.get('branch', 'main')
        if not self._is_valid_branch_name(branch):
            self.result.add_error("gitops.repository.branch", branch, "Invalid Git branch name")
        
        # Validate authentication
        auth = repository.get('auth', {})
        auth_method = auth.get('method', 'ssh')
        if auth_method not in ['ssh', 'pat', 'basic', 'none']:
            self.result.add_error("gitops.repository.auth.method", auth_method, "Invalid authentication method")
        
        # Method-specific validation
        if auth_method == 'ssh':
            ssh_config = auth.get('ssh', {})
            if not ssh_config.get('private_key_path'):
                self.result.add_error("gitops.repository.auth.ssh.private_key_path", "", "SSH private key path is required")
        elif auth_method == 'pat':
            pat_config = auth.get('pat', {})
            if not pat_config.get('token'):
                self.result.add_error("gitops.repository.auth.pat.token", "", "PAT token is required")
        elif auth_method == 'basic':
            basic_config = auth.get('basic', {})
            if not basic_config.get('username'):
                self.result.add_error("gitops.repository.auth.basic.username", "", "Username is required for basic auth")
            if not basic_config.get('password'):
                self.result.add_error("gitops.repository.auth.basic.password", "", "Password is required for basic auth")
    
    def _validate_pipeline(self):
        """Validate pipeline configuration."""
        pipeline = self.config_dict.get('pipeline', {})
        
        # Validate mode
        mode = pipeline.get('mode', 'sequential')
        if mode not in ['sequential', 'parallel', 'manual']:
            self.result.add_error("pipeline.mode", mode, "Invalid pipeline mode")
        
        # Validate automation settings
        automation = pipeline.get('automation', {})
        if automation.get('enabled', True):
            max_wait = automation.get('max_wait_time', 300)
            if max_wait <= 0:
                self.result.add_error("pipeline.automation.max_wait_time", max_wait, "Max wait time must be positive")
            
            trigger_methods = automation.get('trigger_methods', [])
            valid_methods = ['file', 'process', 'webhook', 'script']
            for method in trigger_methods:
                if method not in valid_methods:
                    self.result.add_error("pipeline.automation.trigger_methods", method, f"Invalid trigger method: {method}")
    
    def _validate_cross_field_rules(self):
        """Validate cross-field dependencies."""
        # Rule: ArgoCD enabled requires Git repository
        gitops = self.config_dict.get('gitops', {})
        structure = gitops.get('structure', {})
        argocd = structure.get('argocd', {})
        
        if argocd.get('enabled', True) and not gitops.get('repository', {}).get('url'):
            self.result.add_error("gitops.repository.url", "", "Git repository URL is required when ArgoCD is enabled")
        
        # Rule: S3 auto-create bucket warning
        storage = self.config_dict.get('storage', {})
        if storage.get('auto_create_bucket') and storage.get('type') == 's3':
            self.result.add_warning("storage.auto_create_bucket", True, "Ensure AWS credentials have bucket creation permissions")
        
        # Rule: Pipeline automation requires trigger methods
        pipeline = self.config_dict.get('pipeline', {})
        automation = pipeline.get('automation', {})
        if automation.get('enabled') and not automation.get('trigger_methods'):
            self.result.add_error("pipeline.automation.trigger_methods", [], "At least one trigger method required when automation is enabled")
    
    # Helper validation methods
    
    def _is_valid_endpoint(self, endpoint: str) -> bool:
        """Validate endpoint format."""
        if endpoint.startswith(('http://', 'https://')):
            try:
                parsed = urlparse(endpoint)
                return parsed.hostname is not None
            except:
                return False
        else:
            # host:port format
            return bool(re.match(r'^[a-zA-Z0-9\-\.]+:\d+$', endpoint))
    
    def _is_valid_bucket_name(self, bucket: str) -> bool:
        """Validate S3/MinIO bucket naming rules."""
        if len(bucket) < 3 or len(bucket) > 63:
            return False
        return bool(re.match(r'^[a-z0-9][a-z0-9\-]*[a-z0-9]$', bucket))
    
    def _is_valid_domain(self, domain: str) -> bool:
        """Validate domain format."""
        pattern = r'^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$'
        return bool(re.match(pattern, domain))
    
    def _is_valid_git_url(self, url: str) -> bool:
        """Validate Git repository URL."""
        git_patterns = [
            r'^git@',  # SSH format
            r'^ssh://',  # SSH URL format
            r'^https?://.*\.git$',  # HTTPS with .git
            r'^https?://(github\.com|gitlab\.com|bitbucket\.org)',  # Popular hosts
        ]
        return any(re.match(pattern, url) for pattern in git_patterns)
    
    def _is_valid_branch_name(self, branch: str) -> bool:
        """Validate Git branch name."""
        if not branch or branch.startswith('/') or branch.endswith('/'):
            return False
        return bool(re.match(r'^[a-zA-Z0-9/_\-\.]+$', branch))


def validate_config(config_dict: Dict[str, Any]) -> ValidationResult:
    """
    Main validation entry point.
    
    Args:
        config_dict: Configuration dictionary to validate
    
    Returns:
        ValidationResult with errors and warnings
    """
    validator = SimpleConfigValidator(config_dict)
    return validator.validate()


def validate_config_file(config_path: str) -> ValidationResult:
    """
    Validate a configuration file.
    
    Args:
        config_path: Path to YAML configuration file
    
    Returns:
        ValidationResult with errors and warnings
    """
    import yaml
    from pathlib import Path
    
    config_file = Path(config_path)
    if not config_file.exists():
        result = ValidationResult()
        result.add_error("file", str(config_path), "Configuration file does not exist")
        return result
    
    try:
        with open(config_file, 'r') as f:
            config_dict = yaml.safe_load(f)
    except Exception as e:
        result = ValidationResult()
        result.add_error("file", str(config_path), f"Failed to parse YAML: {e}")
        return result
    
    return validate_config(config_dict or {})


# Test the validation
if __name__ == "__main__":
    # Test valid configuration
    valid_config = {
        'storage': {
            'type': 'minio',
            'endpoint': 'localhost:9000',
            'access_key': 'minioadmin',
            'secret_key': 'minioadmin',
            'bucket': 'test-bucket'
        },
        'cluster': {
            'name': 'test-cluster',
            'type': 'kubernetes'
        },
        'gitops': {
            'repository': {
                'url': 'git@github.com:user/repo.git',
                'branch': 'main',
                'auth': {
                    'method': 'ssh',
                    'ssh': {
                        'private_key_path': '/home/user/.ssh/id_rsa'
                    }
                }
            }
        }
    }
    
    print("Testing valid configuration:")
    result = validate_config(valid_config)
    print(result.format_result())
    print()
    
    # Test invalid configuration
    invalid_config = {
        'storage': {
            'type': 'invalid',
            'endpoint': '',
            'bucket': 'INVALID_BUCKET'
        },
        'gitops': {
            'repository': {
                'url': 'invalid-url',
                'auth': {
                    'method': 'ssh',
                    'ssh': {}
                }
            }
        }
    }
    
    print("Testing invalid configuration:")
    result2 = validate_config(invalid_config)
    print(result2.format_result())