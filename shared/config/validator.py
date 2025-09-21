"""
Enhanced schema validation for shared configuration system.

This module provides comprehensive validation with Pydantic v2,
including cross-field validation, custom validators, and detailed error reporting.
"""

import re
from typing import Dict, Any, List, Optional, Union, Set
from pathlib import Path
from urllib.parse import urlparse
from pydantic import BaseModel, Field, field_validator, model_validator, ValidationError as PydanticValidationError
from pydantic.config import ConfigDict
from datetime import timedelta


class ValidationError:
    """Represents a single validation error or warning."""
    
    def __init__(self, field: str, value: Any, message: str, level: str = "error"):
        self.field = field
        self.value = value
        self.message = message
        self.level = level  # "error" or "warning"
    
    def __repr__(self):
        return f"ValidationError(field={self.field}, message={self.message}, level={self.level})"


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


# Enhanced Pydantic models with validation

class ValidatedConnectionConfig(BaseModel):
    """Connection configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    timeout: int = Field(default=30, gt=0, description="Connection timeout in seconds")
    max_retries: int = Field(default=3, ge=0, description="Maximum number of retries")
    retry_delay: timedelta = Field(default=timedelta(seconds=5), description="Delay between retries")
    
    @field_validator('retry_delay')
    @classmethod
    def validate_retry_delay(cls, v):
        if v.total_seconds() < 0:
            raise ValueError("Retry delay cannot be negative")
        return v


class ValidatedStorageConfig(BaseModel):
    """Storage configuration with enhanced validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    type: str = Field(default="minio", pattern="^(minio|s3)$", description="Storage type")
    endpoint: str = Field(..., min_length=1, description="Storage endpoint")
    access_key: str = Field(..., min_length=1, description="Access key")
    secret_key: str = Field(..., min_length=1, description="Secret key")
    bucket: str = Field(default="cluster-backups", description="Bucket name")
    use_ssl: bool = Field(default=True, description="Use SSL/TLS")
    region: str = Field(default="us-east-1", description="AWS region")
    auto_create_bucket: bool = Field(default=False, description="Auto-create bucket")
    fallback_buckets: List[str] = Field(default_factory=list, description="Fallback buckets")
    connection: ValidatedConnectionConfig = Field(default_factory=ValidatedConnectionConfig)
    
    @field_validator('endpoint')
    @classmethod
    def validate_endpoint(cls, v):
        """Validate endpoint format."""
        if not v:
            raise ValueError("Endpoint is required")
        
        # Check if it's a URL
        if v.startswith(('http://', 'https://')):
            try:
                urlparse(v)
            except Exception:
                raise ValueError("Invalid endpoint URL format")
        else:
            # Check host:port format
            if not re.match(r'^[a-zA-Z0-9\-\.]+:\d+$', v):
                raise ValueError("Invalid endpoint format (expected host:port or URL)")
        return v
    
    @field_validator('bucket', 'fallback_buckets')
    @classmethod
    def validate_bucket_name(cls, v):
        """Validate S3/MinIO bucket naming rules."""
        def is_valid_bucket(name):
            if not name:
                return True  # Allow empty for optional fields
            if len(name) < 3 or len(name) > 63:
                return False
            if not re.match(r'^[a-z0-9][a-z0-9\-]*[a-z0-9]$', name):
                return False
            return True
        
        if isinstance(v, list):
            for bucket in v:
                if not is_valid_bucket(bucket):
                    raise ValueError(f"Invalid bucket name: {bucket}")
        else:
            if not is_valid_bucket(v):
                raise ValueError(f"Invalid bucket name: {v}")
        return v
    
    @model_validator(mode='after')
    def validate_s3_requirements(self):
        """Validate S3-specific requirements."""
        if self.type == 's3' and not self.region:
            raise ValueError("Region is required for S3 storage")
        return self


class ValidatedResourceFilter(BaseModel):
    """Resource filter with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    include: List[str] = Field(default_factory=list, description="Resources to include")
    exclude: List[str] = Field(default_factory=list, description="Resources to exclude")
    
    @field_validator('include', 'exclude')
    @classmethod
    def validate_resource_types(cls, v):
        """Validate Kubernetes resource types."""
        valid_resources = {
            'pods', 'services', 'deployments', 'statefulsets', 'daemonsets',
            'configmaps', 'secrets', 'persistentvolumeclaims', 'persistentvolumes',
            'ingresses', 'networkpolicies', 'serviceaccounts', 'roles', 'rolebindings',
            'clusterroles', 'clusterrolebindings', 'namespaces', 'nodes',
            'customresourcedefinitions', 'horizontalpodautoscalers', 'verticalpodautoscalers',
            'poddisruptionbudgets', 'priorityclasses', 'storageclasses', 'replicasets',
            'jobs', 'cronjobs', 'endpoints', 'events'
        }
        
        warnings = []
        for resource in v:
            if resource.lower() not in valid_resources:
                warnings.append(f"'{resource}' may not be a valid Kubernetes resource type")
        
        # Store warnings for later processing (handled by validator)
        if warnings:
            cls._warnings = warnings
        
        return v


class ValidatedNamespaceFilter(BaseModel):
    """Namespace filter with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    include: List[str] = Field(default_factory=list, description="Namespaces to include")
    exclude: List[str] = Field(default_factory=list, description="Namespaces to exclude")


class ValidatedFilteringConfig(BaseModel):
    """Filtering configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    mode: str = Field(default="whitelist", pattern="^(whitelist|blacklist|hybrid)$")
    resources: ValidatedResourceFilter = Field(default_factory=ValidatedResourceFilter)
    namespaces: ValidatedNamespaceFilter = Field(default_factory=ValidatedNamespaceFilter)
    label_selector: str = Field(default="", description="Kubernetes label selector")
    annotation_selector: str = Field(default="", description="Kubernetes annotation selector")


class ValidatedBehaviorConfig(BaseModel):
    """Backup behavior configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    batch_size: int = Field(default=50, gt=0, le=1000, description="Batch size for operations")
    validate_yaml: bool = Field(default=True, description="Validate YAML syntax")
    skip_invalid_resources: bool = Field(default=True, description="Skip invalid resources")
    include_managed_fields: bool = Field(default=False, description="Include managed fields")
    include_status: bool = Field(default=False, description="Include resource status")
    max_resource_size: str = Field(default="10Mi", description="Maximum resource size")
    follow_owner_references: bool = Field(default=False, description="Follow owner references")
    
    @field_validator('max_resource_size')
    @classmethod
    def validate_size_format(cls, v):
        """Validate Kubernetes size format."""
        if v and not re.match(r'^\d+(\.\d+)?([KMGT]i?)?$', v):
            raise ValueError("Invalid size format (expected format like '10Mi', '1Gi')")
        return v


class ValidatedCleanupConfig(BaseModel):
    """Cleanup configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    enabled: bool = Field(default=True, description="Enable cleanup")
    retention_days: int = Field(default=7, ge=0, description="Retention days")
    cleanup_on_startup: bool = Field(default=False, description="Cleanup on startup")


class ValidatedBackupConfig(BaseModel):
    """Backup configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    filtering: ValidatedFilteringConfig = Field(default_factory=ValidatedFilteringConfig)
    behavior: ValidatedBehaviorConfig = Field(default_factory=ValidatedBehaviorConfig)
    cleanup: ValidatedCleanupConfig = Field(default_factory=ValidatedCleanupConfig)


class ValidatedGitAuthSSH(BaseModel):
    """SSH authentication configuration."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    private_key_path: str = Field(default="", description="SSH private key path")
    passphrase: str = Field(default="", description="SSH key passphrase")


class ValidatedGitAuthPAT(BaseModel):
    """Personal Access Token authentication configuration."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    token: str = Field(default="", description="Personal access token")
    username: str = Field(default="", description="Username (optional for some platforms)")


class ValidatedGitAuthBasic(BaseModel):
    """Basic authentication configuration."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    username: str = Field(default="", description="Username")
    password: str = Field(default="", description="Password")


class ValidatedGitAuth(BaseModel):
    """Git authentication configuration."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    method: str = Field(default="ssh", pattern="^(ssh|pat|basic|none)$")
    ssh: ValidatedGitAuthSSH = Field(default_factory=ValidatedGitAuthSSH)
    pat: ValidatedGitAuthPAT = Field(default_factory=ValidatedGitAuthPAT)
    basic: ValidatedGitAuthBasic = Field(default_factory=ValidatedGitAuthBasic)
    
    @model_validator(mode='after')
    def validate_auth_requirements(self):
        """Validate authentication method requirements."""
        if self.method == "ssh" and not self.ssh.private_key_path:
            raise ValueError("SSH private key path is required for SSH authentication")
        elif self.method == "pat" and not self.pat.token:
            raise ValueError("PAT token is required for PAT authentication")
        elif self.method == "basic":
            if not self.basic.username:
                raise ValueError("Username is required for basic authentication")
            if not self.basic.password:
                raise ValueError("Password is required for basic authentication")
        return self


class ValidatedGitRepository(BaseModel):
    """Git repository configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    url: str = Field(..., min_length=1, description="Git repository URL")
    branch: str = Field(default="main", min_length=1, description="Git branch")
    auth: ValidatedGitAuth = Field(default_factory=ValidatedGitAuth)
    
    @field_validator('url')
    @classmethod
    def validate_git_url(cls, v):
        """Validate Git repository URL."""
        if not v:
            raise ValueError("Git repository URL is required")
        
        # Check for common Git URL patterns
        valid_patterns = [
            r'^git@',  # SSH format
            r'^ssh://',  # SSH URL format
            r'^https?://.*\.git$',  # HTTPS with .git extension
            r'^https?://(github\.com|gitlab\.com|bitbucket\.org)',  # Popular Git hosts
        ]
        
        if not any(re.match(pattern, v) for pattern in valid_patterns):
            raise ValueError("Invalid Git repository URL format")
        
        return v
    
    @field_validator('branch')
    @classmethod
    def validate_branch_name(cls, v):
        """Validate Git branch name."""
        if not re.match(r'^[a-zA-Z0-9/_\-\.]+$', v):
            raise ValueError("Invalid Git branch name")
        if v.startswith('/') or v.endswith('/'):
            raise ValueError("Branch name cannot start or end with '/'")
        return v


class ValidatedEnvironment(BaseModel):
    """Environment configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    name: str = Field(..., min_length=1, description="Environment name")
    cluster_url: str = Field(default="", description="Cluster URL")
    auto_sync: bool = Field(default=False, description="Enable auto-sync")
    replicas: int = Field(default=1, ge=0, description="Number of replicas")
    
    @field_validator('cluster_url')
    @classmethod
    def validate_cluster_url(cls, v):
        """Validate cluster URL."""
        if v and not re.match(r'^https?://', v):
            raise ValueError("Cluster URL must start with http:// or https://")
        return v


class ValidatedArgoCDConfig(BaseModel):
    """ArgoCD configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    enabled: bool = Field(default=True, description="Enable ArgoCD")
    namespace: str = Field(default="argocd", description="ArgoCD namespace")
    project: str = Field(default="default", description="ArgoCD project")
    sync_policy: Dict[str, bool] = Field(
        default_factory=lambda: {"automated": False, "prune": False, "self_heal": False}
    )


class ValidatedGitOpsStructure(BaseModel):
    """GitOps structure configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    base_dir: str = Field(default="namespaces", description="Base directory")
    environments: List[ValidatedEnvironment] = Field(default_factory=list)
    argocd: ValidatedArgoCDConfig = Field(default_factory=ValidatedArgoCDConfig)
    kustomize: Dict[str, bool] = Field(
        default_factory=lambda: {"enabled": True, "strategic_merge": True}
    )
    
    @model_validator(mode='after')
    def validate_argocd_requirements(self):
        """Validate ArgoCD requirements."""
        if self.argocd.enabled:
            if not self.argocd.namespace:
                raise ValueError("ArgoCD namespace is required when ArgoCD is enabled")
            if not self.argocd.project:
                raise ValueError("ArgoCD project is required when ArgoCD is enabled")
        return self


class ValidatedGitOpsConfig(BaseModel):
    """GitOps configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    repository: ValidatedGitRepository
    structure: ValidatedGitOpsStructure = Field(default_factory=ValidatedGitOpsStructure)


class ValidatedPipelineAutomation(BaseModel):
    """Pipeline automation configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    enabled: bool = Field(default=True)
    trigger_on_backup_complete: bool = Field(default=True)
    wait_for_backup: bool = Field(default=True)
    max_wait_time: int = Field(default=300, gt=0)
    trigger_methods: List[str] = Field(default_factory=lambda: ["file", "process", "webhook"])
    
    @field_validator('trigger_methods')
    @classmethod
    def validate_trigger_methods(cls, v):
        """Validate trigger methods."""
        valid_methods = {"file", "process", "webhook", "script"}
        for method in v:
            if method not in valid_methods:
                raise ValueError(f"Invalid trigger method: {method}")
        return v


class ValidatedPipelineConfig(BaseModel):
    """Pipeline configuration with validation."""
    model_config = ConfigDict(validate_default=True, extra='forbid')
    
    mode: str = Field(default="sequential", pattern="^(sequential|parallel|manual)$")
    automation: ValidatedPipelineAutomation = Field(default_factory=ValidatedPipelineAutomation)
    
    @model_validator(mode='after')
    def validate_automation_requirements(self):
        """Validate automation requirements."""
        if self.automation.enabled and not self.automation.trigger_methods:
            raise ValueError("At least one trigger method is required when automation is enabled")
        return self


class ConfigValidator:
    """Main configuration validator with cross-field validation."""
    
    def __init__(self, config_dict: Dict[str, Any]):
        self.config_dict = config_dict
        self.result = ValidationResult()
    
    def validate(self) -> ValidationResult:
        """Perform comprehensive configuration validation."""
        # Validate individual sections
        self._validate_storage()
        self._validate_backup()
        self._validate_gitops()
        self._validate_pipeline()
        
        # Cross-field validation
        self._validate_cross_field_rules()
        
        return self.result
    
    def _validate_storage(self):
        """Validate storage configuration."""
        try:
            storage_config = self.config_dict.get('storage', {})
            ValidatedStorageConfig(**storage_config)
        except PydanticValidationError as e:
            for error in e.errors():
                field = "storage." + ".".join(str(loc) for loc in error['loc'])
                self.result.add_error(field, error.get('input'), error['msg'])
    
    def _validate_backup(self):
        """Validate backup configuration."""
        try:
            backup_config = self.config_dict.get('backup', {})
            validated = ValidatedBackupConfig(**backup_config)
            
            # Check for warnings
            if validated.behavior.batch_size > 500:
                self.result.add_warning(
                    "backup.behavior.batch_size",
                    validated.behavior.batch_size,
                    "Large batch size may cause performance issues"
                )
            
            if validated.cleanup.enabled and validated.cleanup.retention_days == 0:
                self.result.add_warning(
                    "backup.cleanup.retention_days",
                    0,
                    "Zero retention days means immediate deletion"
                )
                
        except PydanticValidationError as e:
            for error in e.errors():
                field = "backup." + ".".join(str(loc) for loc in error['loc'])
                self.result.add_error(field, error.get('input'), error['msg'])
    
    def _validate_gitops(self):
        """Validate GitOps configuration."""
        try:
            gitops_config = self.config_dict.get('gitops', {})
            ValidatedGitOpsConfig(**gitops_config)
        except PydanticValidationError as e:
            for error in e.errors():
                field = "gitops." + ".".join(str(loc) for loc in error['loc'])
                self.result.add_error(field, error.get('input'), error['msg'])
    
    def _validate_pipeline(self):
        """Validate pipeline configuration."""
        try:
            pipeline_config = self.config_dict.get('pipeline', {})
            ValidatedPipelineConfig(**pipeline_config)
        except PydanticValidationError as e:
            for error in e.errors():
                field = "pipeline." + ".".join(str(loc) for loc in error['loc'])
                self.result.add_error(field, error.get('input'), error['msg'])
    
    def _validate_cross_field_rules(self):
        """Validate cross-field dependencies and rules."""
        # Rule: If ArgoCD is enabled, GitOps repository must be configured
        gitops = self.config_dict.get('gitops', {})
        if gitops.get('structure', {}).get('argocd', {}).get('enabled', True):
            if not gitops.get('repository', {}).get('url'):
                self.result.add_error(
                    "gitops.repository.url",
                    "",
                    "Git repository URL is required when ArgoCD is enabled"
                )
        
        # Rule: If auto-create bucket is enabled for S3, warn about permissions
        storage = self.config_dict.get('storage', {})
        if storage.get('auto_create_bucket') and storage.get('type') == 's3':
            self.result.add_warning(
                "storage.auto_create_bucket",
                True,
                "Ensure AWS credentials have bucket creation permissions"
            )
        
        # Rule: If strict validation is enabled with high concurrency, warn about performance
        security = self.config_dict.get('security', {})
        performance = self.config_dict.get('performance', {})
        if security.get('validation', {}).get('strict_mode', True):
            if performance.get('limits', {}).get('max_concurrent_operations', 10) > 50:
                self.result.add_warning(
                    "security.validation.strict_mode",
                    True,
                    "Strict validation with high concurrency may impact performance"
                )


def validate_config(config_dict: Dict[str, Any]) -> ValidationResult:
    """
    Main entry point for configuration validation.
    
    Args:
        config_dict: Dictionary containing the configuration to validate
    
    Returns:
        ValidationResult object containing errors and warnings
    """
    validator = ConfigValidator(config_dict)
    return validator.validate()


def validate_config_file(config_path: Union[str, Path]) -> ValidationResult:
    """
    Validate a configuration file.
    
    Args:
        config_path: Path to the configuration file
    
    Returns:
        ValidationResult object containing errors and warnings
    """
    import yaml
    
    config_path = Path(config_path)
    if not config_path.exists():
        result = ValidationResult()
        result.add_error("file", str(config_path), "Configuration file does not exist")
        return result
    
    try:
        with open(config_path, 'r') as f:
            config_dict = yaml.safe_load(f)
    except Exception as e:
        result = ValidationResult()
        result.add_error("file", str(config_path), f"Failed to parse YAML: {e}")
        return result
    
    return validate_config(config_dict)