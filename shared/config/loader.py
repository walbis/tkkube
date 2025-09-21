"""
Shared configuration loader for Python-based GitOps tool.

This module provides a unified configuration system that works with
the same YAML schema as the Go backup tool.
"""

import os
import yaml
import json
from pathlib import Path
from typing import Dict, Any, List, Optional, Union
from dataclasses import dataclass, field
from datetime import timedelta


@dataclass
class ConnectionConfig:
    """Connection configuration for storage backends."""
    timeout: int = 30
    max_retries: int = 3
    retry_delay: timedelta = timedelta(seconds=5)


@dataclass
class StorageConfig:
    """Storage backend configuration."""
    type: str = "minio"
    endpoint: str = ""
    access_key: str = ""
    secret_key: str = ""
    bucket: str = "cluster-backups"
    use_ssl: bool = True
    region: str = "us-east-1"
    auto_create_bucket: bool = False
    fallback_buckets: List[str] = field(default_factory=list)
    connection: ConnectionConfig = field(default_factory=ConnectionConfig)


@dataclass
class OpenShiftConfig:
    """OpenShift-specific configuration."""
    mode: str = "auto-detect"
    include_resources: bool = True


@dataclass
class ClusterConfig:
    """Cluster configuration."""
    name: str = ""
    domain: str = "cluster.local"
    type: str = "kubernetes"
    openshift: OpenShiftConfig = field(default_factory=OpenShiftConfig)


@dataclass
class ResourceFilter:
    """Resource filtering configuration."""
    include: List[str] = field(default_factory=list)
    exclude: List[str] = field(default_factory=list)


@dataclass
class NamespaceFilter:
    """Namespace filtering configuration."""
    include: List[str] = field(default_factory=list)
    exclude: List[str] = field(default_factory=list)


@dataclass
class FilteringConfig:
    """Backup filtering configuration."""
    mode: str = "whitelist"
    resources: ResourceFilter = field(default_factory=ResourceFilter)
    namespaces: NamespaceFilter = field(default_factory=NamespaceFilter)
    label_selector: str = ""
    annotation_selector: str = ""


@dataclass
class BehaviorConfig:
    """Backup behavior configuration."""
    batch_size: int = 50
    validate_yaml: bool = True
    skip_invalid_resources: bool = True
    include_managed_fields: bool = False
    include_status: bool = False
    max_resource_size: str = "10Mi"
    follow_owner_references: bool = False


@dataclass
class CleanupConfig:
    """Cleanup policy configuration."""
    enabled: bool = True
    retention_days: int = 7
    cleanup_on_startup: bool = False


@dataclass
class BackupConfig:
    """Backup configuration."""
    filtering: FilteringConfig = field(default_factory=FilteringConfig)
    behavior: BehaviorConfig = field(default_factory=BehaviorConfig)
    cleanup: CleanupConfig = field(default_factory=CleanupConfig)


@dataclass
class SSHAuthConfig:
    """SSH authentication configuration."""
    private_key_path: str = "~/.ssh/id_rsa"
    passphrase: str = ""


@dataclass
class PATAuthConfig:
    """Personal Access Token authentication configuration."""
    token: str = ""
    username: str = ""


@dataclass
class BasicAuthConfig:
    """Basic authentication configuration."""
    username: str = ""
    password: str = ""


@dataclass
class AuthConfig:
    """Git authentication configuration."""
    method: str = "ssh"
    ssh: SSHAuthConfig = field(default_factory=SSHAuthConfig)
    pat: PATAuthConfig = field(default_factory=PATAuthConfig)
    basic: BasicAuthConfig = field(default_factory=BasicAuthConfig)


@dataclass
class RepositoryConfig:
    """Git repository configuration."""
    url: str = ""
    branch: str = "main"
    auth: AuthConfig = field(default_factory=AuthConfig)


@dataclass
class EnvironmentConfig:
    """Environment-specific configuration."""
    name: str = ""
    cluster_url: str = ""
    auto_sync: bool = True
    replicas: int = 1


@dataclass
class SyncPolicyConfig:
    """ArgoCD sync policy configuration."""
    automated: bool = False
    prune: bool = False
    self_heal: bool = False


@dataclass
class ArgoCDConfig:
    """ArgoCD configuration."""
    enabled: bool = True
    namespace: str = "argocd"
    project: str = "default"
    sync_policy: SyncPolicyConfig = field(default_factory=SyncPolicyConfig)


@dataclass
class KustomizeConfig:
    """Kustomize configuration."""
    enabled: bool = True
    strategic_merge: bool = True


@dataclass
class StructureConfig:
    """GitOps structure configuration."""
    base_dir: str = "namespaces"
    environments: List[EnvironmentConfig] = field(default_factory=list)
    argocd: ArgoCDConfig = field(default_factory=ArgoCDConfig)
    kustomize: KustomizeConfig = field(default_factory=KustomizeConfig)


@dataclass
class GitOpsConfig:
    """GitOps configuration."""
    repository: RepositoryConfig = field(default_factory=RepositoryConfig)
    structure: StructureConfig = field(default_factory=StructureConfig)


@dataclass
class FileTriggerConfig:
    """File-based trigger configuration."""
    enabled: bool = True
    directory: str = "/tmp/backup-gitops-triggers"
    cleanup_after_processing: bool = True


@dataclass
class ProcessTriggerConfig:
    """Process-based trigger configuration."""
    enabled: bool = True
    gitops_binary_path: str = ""
    additional_args: str = ""


@dataclass
class WebhookAuthConfig:
    """Webhook authentication configuration."""
    enabled: bool = False
    token: str = ""
    header_name: str = "Authorization"


@dataclass
class WebhookTriggerConfig:
    """Webhook-based trigger configuration."""
    enabled: bool = False
    server_host: str = "0.0.0.0"
    server_port: int = 8080
    endpoint_path: str = "/webhook/backup-complete"
    authentication: WebhookAuthConfig = field(default_factory=WebhookAuthConfig)


@dataclass
class AutomationConfig:
    """Pipeline automation configuration."""
    enabled: bool = True
    trigger_on_backup_complete: bool = True
    wait_for_backup: bool = True
    max_wait_time: int = 300
    trigger_methods: List[str] = field(default_factory=lambda: ["file", "process", "webhook"])
    file_trigger: FileTriggerConfig = field(default_factory=FileTriggerConfig)
    process_trigger: ProcessTriggerConfig = field(default_factory=ProcessTriggerConfig)
    webhook_trigger: WebhookTriggerConfig = field(default_factory=WebhookTriggerConfig)


@dataclass
class WebhookConfig:
    """Webhook notification configuration."""
    url: str = ""
    on_success: bool = True
    on_failure: bool = True


@dataclass
class SlackConfig:
    """Slack notification configuration."""
    webhook_url: str = ""
    channel: str = "#backup-notifications"


@dataclass
class NotificationsConfig:
    """Notifications configuration."""
    enabled: bool = False
    webhook: WebhookConfig = field(default_factory=WebhookConfig)
    slack: SlackConfig = field(default_factory=SlackConfig)


@dataclass
class ErrorHandlingConfig:
    """Error handling configuration."""
    continue_on_error: bool = False
    max_retries: int = 3
    retry_delay: timedelta = timedelta(seconds=30)


@dataclass
class PipelineConfig:
    """Pipeline configuration."""
    mode: str = "sequential"
    automation: AutomationConfig = field(default_factory=AutomationConfig)
    notifications: NotificationsConfig = field(default_factory=NotificationsConfig)
    error_handling: ErrorHandlingConfig = field(default_factory=ErrorHandlingConfig)


@dataclass
class MetricsConfig:
    """Metrics configuration."""
    enabled: bool = True
    port: int = 8080
    path: str = "/metrics"


@dataclass
class LoggingConfig:
    """Logging configuration."""
    level: str = "info"
    format: str = "json"
    file: str = ""


@dataclass
class TracingConfig:
    """Tracing configuration."""
    enabled: bool = False
    endpoint: str = ""
    sample_rate: float = 0.1


@dataclass
class ObservabilityConfig:
    """Observability configuration."""
    metrics: MetricsConfig = field(default_factory=MetricsConfig)
    logging: LoggingConfig = field(default_factory=LoggingConfig)
    tracing: TracingConfig = field(default_factory=TracingConfig)


@dataclass
class SharedConfig:
    """Complete shared configuration."""
    schema_version: str = "1.0.0"
    description: str = "Unified configuration for Kubernetes backup and GitOps generation pipeline"
    storage: StorageConfig = field(default_factory=StorageConfig)
    cluster: ClusterConfig = field(default_factory=ClusterConfig)
    backup: BackupConfig = field(default_factory=BackupConfig)
    gitops: GitOpsConfig = field(default_factory=GitOpsConfig)
    pipeline: PipelineConfig = field(default_factory=PipelineConfig)
    observability: ObservabilityConfig = field(default_factory=ObservabilityConfig)


class ConfigLoader:
    """Configuration loader for shared configuration."""
    
    def __init__(self, config_paths: Optional[List[str]] = None):
        """Initialize configuration loader.
        
        Args:
            config_paths: List of configuration file paths to load
        """
        self.config_paths = config_paths or self._default_config_paths()
        self.env_prefix = "BACKUP_"
    
    @staticmethod
    def _default_config_paths() -> List[str]:
        """Get default configuration file paths."""
        paths = [
            "./shared-config.yaml",
            "./config/shared-config.yaml",
            "/etc/backup-gitops/config.yaml",
        ]
        
        # Add home directory config
        home = Path.home()
        paths.append(str(home / ".backup-gitops" / "config.yaml"))
        
        return paths
    
    def load(self) -> SharedConfig:
        """Load and merge configuration from multiple sources.
        
        Returns:
            SharedConfig: Loaded and validated configuration
        """
        config = SharedConfig()
        
        # Load from files in order
        for path in self.config_paths:
            config = self._load_file(path, config)
        
        # Apply environment variable overrides
        config = self._apply_environment_overrides(config)
        
        # Expand environment variables in string fields
        config = self._expand_environment_variables(config)
        
        # Validate the final configuration
        self._validate(config)
        
        return config
    
    def _load_file(self, path: str, config: SharedConfig) -> SharedConfig:
        """Load configuration from a YAML file.
        
        Args:
            path: Path to configuration file
            config: Existing configuration to merge with
            
        Returns:
            SharedConfig: Merged configuration
        """
        path_obj = Path(path)
        if not path_obj.exists():
            return config
        
        try:
            with open(path_obj, 'r') as f:
                data = yaml.safe_load(f)
            
            if data:
                # Merge configurations (simplified version)
                config = self._merge_configs(config, data)
        except Exception as e:
            print(f"Warning: Failed to load config from {path}: {e}")
        
        return config
    
    def _merge_configs(self, config: SharedConfig, data: Dict[str, Any]) -> SharedConfig:
        """Merge configuration data into existing config.
        
        Args:
            config: Existing configuration
            data: New configuration data to merge
            
        Returns:
            SharedConfig: Merged configuration
        """
        # Storage configuration
        if 'storage' in data:
            storage_data = data['storage']
            config.storage.type = storage_data.get('type', config.storage.type)
            config.storage.endpoint = storage_data.get('endpoint', config.storage.endpoint)
            config.storage.access_key = storage_data.get('access_key', config.storage.access_key)
            config.storage.secret_key = storage_data.get('secret_key', config.storage.secret_key)
            config.storage.bucket = storage_data.get('bucket', config.storage.bucket)
            config.storage.use_ssl = storage_data.get('use_ssl', config.storage.use_ssl)
            config.storage.region = storage_data.get('region', config.storage.region)
            config.storage.auto_create_bucket = storage_data.get('auto_create_bucket', config.storage.auto_create_bucket)
            
            if 'connection' in storage_data:
                conn = storage_data['connection']
                config.storage.connection.timeout = conn.get('timeout', config.storage.connection.timeout)
                config.storage.connection.max_retries = conn.get('max_retries', config.storage.connection.max_retries)
        
        # Cluster configuration
        if 'cluster' in data:
            cluster_data = data['cluster']
            config.cluster.name = cluster_data.get('name', config.cluster.name)
            config.cluster.domain = cluster_data.get('domain', config.cluster.domain)
            config.cluster.type = cluster_data.get('type', config.cluster.type)
        
        # GitOps configuration
        if 'gitops' in data:
            gitops_data = data['gitops']
            if 'repository' in gitops_data:
                repo = gitops_data['repository']
                config.gitops.repository.url = repo.get('url', config.gitops.repository.url)
                config.gitops.repository.branch = repo.get('branch', config.gitops.repository.branch)
                
                if 'auth' in repo:
                    auth = repo['auth']
                    config.gitops.repository.auth.method = auth.get('method', config.gitops.repository.auth.method)
        
        # Backup configuration
        if 'backup' in data:
            backup_data = data['backup']
            if 'behavior' in backup_data:
                behavior = backup_data['behavior']
                config.backup.behavior.batch_size = behavior.get('batch_size', config.backup.behavior.batch_size)
                config.backup.behavior.validate_yaml = behavior.get('validate_yaml', config.backup.behavior.validate_yaml)
            
            if 'cleanup' in backup_data:
                cleanup = backup_data['cleanup']
                config.backup.cleanup.enabled = cleanup.get('enabled', config.backup.cleanup.enabled)
                config.backup.cleanup.retention_days = cleanup.get('retention_days', config.backup.cleanup.retention_days)
        
        # Observability configuration
        if 'observability' in data:
            obs_data = data['observability']
            if 'logging' in obs_data:
                logging = obs_data['logging']
                config.observability.logging.level = logging.get('level', config.observability.logging.level)
                config.observability.logging.format = logging.get('format', config.observability.logging.format)
        
        return config
    
    def _apply_environment_overrides(self, config: SharedConfig) -> SharedConfig:
        """Apply environment variable overrides.
        
        Args:
            config: Configuration to override
            
        Returns:
            SharedConfig: Configuration with environment overrides applied
        """
        # Storage configuration
        config.storage.endpoint = os.getenv('MINIO_ENDPOINT', config.storage.endpoint)
        config.storage.access_key = os.getenv('MINIO_ACCESS_KEY', config.storage.access_key)
        config.storage.secret_key = os.getenv('MINIO_SECRET_KEY', config.storage.secret_key)
        config.storage.bucket = os.getenv('MINIO_BUCKET', config.storage.bucket)
        config.storage.use_ssl = os.getenv('MINIO_USE_SSL', str(config.storage.use_ssl)).lower() == 'true'
        
        # Cluster configuration
        config.cluster.name = os.getenv('CLUSTER_NAME', config.cluster.name)
        config.cluster.domain = os.getenv('CLUSTER_DOMAIN', config.cluster.domain)
        
        # Git configuration
        config.gitops.repository.url = os.getenv('GIT_REPOSITORY', config.gitops.repository.url)
        config.gitops.repository.branch = os.getenv('GIT_BRANCH', config.gitops.repository.branch)
        config.gitops.repository.auth.method = os.getenv('GIT_AUTH_METHOD', config.gitops.repository.auth.method)
        
        # Git authentication
        config.gitops.repository.auth.pat.token = os.getenv('GIT_PAT_TOKEN', config.gitops.repository.auth.pat.token)
        config.gitops.repository.auth.pat.username = os.getenv('GIT_PAT_USERNAME', config.gitops.repository.auth.pat.username)
        config.gitops.repository.auth.basic.username = os.getenv('GIT_USERNAME', config.gitops.repository.auth.basic.username)
        config.gitops.repository.auth.basic.password = os.getenv('GIT_PASSWORD', config.gitops.repository.auth.basic.password)
        config.gitops.repository.auth.ssh.private_key_path = os.getenv('GIT_SSH_KEY', config.gitops.repository.auth.ssh.private_key_path)
        
        # Backup configuration
        batch_size = os.getenv('BATCH_SIZE')
        if batch_size:
            try:
                config.backup.behavior.batch_size = int(batch_size)
            except ValueError:
                pass
        
        retention_days = os.getenv('RETENTION_DAYS')
        if retention_days:
            try:
                config.backup.cleanup.retention_days = int(retention_days)
            except ValueError:
                pass
        
        # Logging configuration
        config.observability.logging.level = os.getenv('LOG_LEVEL', config.observability.logging.level)
        config.observability.logging.format = os.getenv('LOG_FORMAT', config.observability.logging.format)
        
        return config
    
    def _expand_environment_variables(self, config: SharedConfig) -> SharedConfig:
        """Expand environment variables in string fields.
        
        Args:
            config: Configuration with potential environment variable references
            
        Returns:
            SharedConfig: Configuration with expanded environment variables
        """
        # Expand key string fields
        config.storage.endpoint = os.path.expandvars(config.storage.endpoint)
        config.storage.bucket = os.path.expandvars(config.storage.bucket)
        config.cluster.name = os.path.expandvars(config.cluster.name)
        config.cluster.domain = os.path.expandvars(config.cluster.domain)
        config.gitops.repository.url = os.path.expandvars(config.gitops.repository.url)
        
        # Expand home directory in paths
        config.gitops.repository.auth.ssh.private_key_path = os.path.expanduser(
            config.gitops.repository.auth.ssh.private_key_path
        )
        
        return config
    
    def _validate(self, config: SharedConfig) -> None:
        """Validate configuration using enhanced validator.
        
        Args:
            config: Configuration to validate
            
        Raises:
            ValueError: If configuration is invalid
        """
        from .validator import validate_config
        
        # Convert config to dict for validation
        config_dict = self.to_dict(config)
        
        # Perform validation
        validation_result = validate_config(config_dict)
        
        if not validation_result.valid:
            raise ValueError(f"Configuration validation failed:\n{validation_result.format_result()}")
        
        # Print warnings if any
        if validation_result.warnings:
            print(f"Configuration loaded with warnings:\n{validation_result.format_result()}")
    
    def load_without_validation(self) -> SharedConfig:
        """Load configuration without validation (for testing or special cases).
        
        Returns:
            SharedConfig: Loaded configuration without validation
        """
        config = SharedConfig()
        
        # Load from files in order
        for path in self.config_paths:
            config = self._load_file(path, config)
        
        # Apply environment variable overrides
        config = self._apply_environment_overrides(config)
        
        # Expand environment variables in string fields
        config = self._expand_environment_variables(config)
        
        return config
    
    def save_to_file(self, config: SharedConfig, path: str) -> None:
        """Save configuration to a YAML file.
        
        Args:
            config: Configuration to save
            path: Path to save configuration to
        """
        path_obj = Path(path)
        path_obj.parent.mkdir(parents=True, exist_ok=True)
        
        # Convert to dictionary for YAML serialization
        config_dict = self._config_to_dict(config)
        
        with open(path_obj, 'w') as f:
            yaml.dump(config_dict, f, default_flow_style=False)
    
    def _config_to_dict(self, config: SharedConfig) -> Dict[str, Any]:
        """Convert configuration to dictionary.
        
        Args:
            config: Configuration to convert
            
        Returns:
            Dict[str, Any]: Configuration as dictionary
        """
        return {
            'schema_version': config.schema_version,
            'description': config.description,
            'storage': {
                'type': config.storage.type,
                'endpoint': config.storage.endpoint,
                'access_key': config.storage.access_key,
                'secret_key': config.storage.secret_key,
                'bucket': config.storage.bucket,
                'use_ssl': config.storage.use_ssl,
                'region': config.storage.region,
                'auto_create_bucket': config.storage.auto_create_bucket,
                'fallback_buckets': config.storage.fallback_buckets,
                'connection': {
                    'timeout': config.storage.connection.timeout,
                    'max_retries': config.storage.connection.max_retries,
                    'retry_delay': int(config.storage.connection.retry_delay.total_seconds()),
                }
            },
            'cluster': {
                'name': config.cluster.name,
                'domain': config.cluster.domain,
                'type': config.cluster.type,
                'openshift': {
                    'mode': config.cluster.openshift.mode,
                    'include_resources': config.cluster.openshift.include_resources,
                }
            },
            'backup': {
                'filtering': {
                    'mode': config.backup.filtering.mode,
                    'resources': {
                        'include': config.backup.filtering.resources.include,
                        'exclude': config.backup.filtering.resources.exclude,
                    },
                    'namespaces': {
                        'include': config.backup.filtering.namespaces.include,
                        'exclude': config.backup.filtering.namespaces.exclude,
                    },
                    'label_selector': config.backup.filtering.label_selector,
                    'annotation_selector': config.backup.filtering.annotation_selector,
                },
                'behavior': {
                    'batch_size': config.backup.behavior.batch_size,
                    'validate_yaml': config.backup.behavior.validate_yaml,
                    'skip_invalid_resources': config.backup.behavior.skip_invalid_resources,
                    'include_managed_fields': config.backup.behavior.include_managed_fields,
                    'include_status': config.backup.behavior.include_status,
                    'max_resource_size': config.backup.behavior.max_resource_size,
                    'follow_owner_references': config.backup.behavior.follow_owner_references,
                },
                'cleanup': {
                    'enabled': config.backup.cleanup.enabled,
                    'retention_days': config.backup.cleanup.retention_days,
                    'cleanup_on_startup': config.backup.cleanup.cleanup_on_startup,
                }
            },
            'gitops': {
                'repository': {
                    'url': config.gitops.repository.url,
                    'branch': config.gitops.repository.branch,
                    'auth': {
                        'method': config.gitops.repository.auth.method,
                        'ssh': {
                            'private_key_path': config.gitops.repository.auth.ssh.private_key_path,
                            'passphrase': config.gitops.repository.auth.ssh.passphrase,
                        },
                        'pat': {
                            'token': config.gitops.repository.auth.pat.token,
                            'username': config.gitops.repository.auth.pat.username,
                        },
                        'basic': {
                            'username': config.gitops.repository.auth.basic.username,
                            'password': config.gitops.repository.auth.basic.password,
                        }
                    }
                },
                'structure': {
                    'base_dir': config.gitops.structure.base_dir,
                    'environments': [
                        {
                            'name': env.name,
                            'cluster_url': env.cluster_url,
                            'auto_sync': env.auto_sync,
                            'replicas': env.replicas,
                        }
                        for env in config.gitops.structure.environments
                    ],
                    'argocd': {
                        'enabled': config.gitops.structure.argocd.enabled,
                        'namespace': config.gitops.structure.argocd.namespace,
                        'project': config.gitops.structure.argocd.project,
                        'sync_policy': {
                            'automated': config.gitops.structure.argocd.sync_policy.automated,
                            'prune': config.gitops.structure.argocd.sync_policy.prune,
                            'self_heal': config.gitops.structure.argocd.sync_policy.self_heal,
                        }
                    },
                    'kustomize': {
                        'enabled': config.gitops.structure.kustomize.enabled,
                        'strategic_merge': config.gitops.structure.kustomize.strategic_merge,
                    }
                }
            },
            'pipeline': {
                'mode': config.pipeline.mode,
                'automation': {
                    'enabled': config.pipeline.automation.enabled,
                    'trigger_on_backup_complete': config.pipeline.automation.trigger_on_backup_complete,
                    'wait_for_backup': config.pipeline.automation.wait_for_backup,
                    'max_wait_time': config.pipeline.automation.max_wait_time,
                },
                'notifications': {
                    'enabled': config.pipeline.notifications.enabled,
                    'webhook': {
                        'url': config.pipeline.notifications.webhook.url,
                        'on_success': config.pipeline.notifications.webhook.on_success,
                        'on_failure': config.pipeline.notifications.webhook.on_failure,
                    },
                    'slack': {
                        'webhook_url': config.pipeline.notifications.slack.webhook_url,
                        'channel': config.pipeline.notifications.slack.channel,
                    }
                },
                'error_handling': {
                    'continue_on_error': config.pipeline.error_handling.continue_on_error,
                    'max_retries': config.pipeline.error_handling.max_retries,
                    'retry_delay': int(config.pipeline.error_handling.retry_delay.total_seconds()),
                }
            },
            'observability': {
                'metrics': {
                    'enabled': config.observability.metrics.enabled,
                    'port': config.observability.metrics.port,
                    'path': config.observability.metrics.path,
                },
                'logging': {
                    'level': config.observability.logging.level,
                    'format': config.observability.logging.format,
                    'file': config.observability.logging.file,
                },
                'tracing': {
                    'enabled': config.observability.tracing.enabled,
                    'endpoint': config.observability.tracing.endpoint,
                    'sample_rate': config.observability.tracing.sample_rate,
                }
            }
        }


def get_gitops_config_from_shared(config: SharedConfig) -> Dict[str, Any]:
    """Convert shared config to GitOps tool specific config.
    
    Args:
        config: Shared configuration
        
    Returns:
        Dict[str, Any]: GitOps tool configuration
    """
    return {
        'minio': {
            'endpoint': config.storage.endpoint,
            'access_key': config.storage.access_key,
            'secret_key': config.storage.secret_key,
            'bucket': config.storage.bucket,
            'secure': config.storage.use_ssl,
            'prefix': f"{config.cluster.name}/{config.cluster.domain}",
        },
        'git': {
            'repository': config.gitops.repository.url,
            'auth_method': config.gitops.repository.auth.method,
            'ssh': {
                'private_key_path': config.gitops.repository.auth.ssh.private_key_path,
                'passphrase': config.gitops.repository.auth.ssh.passphrase,
            },
            'pat': {
                'token': config.gitops.repository.auth.pat.token,
                'username': config.gitops.repository.auth.pat.username,
            },
            'basic': {
                'username': config.gitops.repository.auth.basic.username,
                'password': config.gitops.repository.auth.basic.password,
            }
        },
        'clusters': {
            'default': {
                env.name: env.cluster_url
                for env in config.gitops.structure.environments
            }
        },
        'environments': {
            env.name: {
                'sync_policy': 'automated' if env.auto_sync else 'manual',
                'replicas': env.replicas,
            }
            for env in config.gitops.structure.environments
        }
    }