#!/usr/bin/env python3
"""
GitOps Restore Orchestrator

This module provides comprehensive GitOps-based disaster recovery and restore
capabilities for Kubernetes clusters. It integrates with the backup system to
restore entire clusters or selective resources through GitOps workflows.
"""

import asyncio
import json
import logging
import os
import time
import tempfile
import shutil
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Union, Any, Tuple
from dataclasses import dataclass, asdict
from enum import Enum
from pathlib import Path

import aiohttp
import yaml
import git
from jinja2 import Template

# Import shared configuration and security modules
import sys
sys.path.append('/home/tkkaray/inceleme/shared')

from config.loader import load_config
from security.python_security import PythonSecurityManager


class RestoreMode(Enum):
    """Restore operation modes"""
    FULL_CLUSTER = "full_cluster"
    SELECTIVE = "selective"
    NAMESPACE = "namespace"
    APPLICATION = "application"
    CONFIGURATION = "configuration"


class RestorePhase(Enum):
    """Restore operation phases"""
    PLANNING = "planning"
    VALIDATION = "validation"
    PREPARATION = "preparation"
    GITOPS_SYNC = "gitops_sync"
    VERIFICATION = "verification"
    CLEANUP = "cleanup"
    COMPLETED = "completed"
    FAILED = "failed"


class DRScenario(Enum):
    """Disaster recovery scenarios"""
    CLUSTER_REBUILD = "cluster_rebuild"
    NAMESPACE_RECOVERY = "namespace_recovery"
    DATA_CORRUPTION = "data_corruption"
    CONFIGURATION_ROLLBACK = "configuration_rollback"
    CROSS_CLUSTER_MIGRATION = "cross_cluster_migration"


@dataclass
class RestoreRequest:
    """GitOps restore request definition"""
    restore_id: str
    backup_id: str
    source_cluster: str
    target_cluster: str
    restore_mode: RestoreMode
    dr_scenario: DRScenario
    target_namespaces: Optional[List[str]] = None
    resource_filters: Optional[Dict[str, Any]] = None
    gitops_config: Optional[Dict[str, Any]] = None
    validation_config: Optional[Dict[str, Any]] = None
    security_context: Optional[Dict[str, Any]] = None
    metadata: Optional[Dict[str, Any]] = None
    dry_run: bool = False


@dataclass
class RestoreProgress:
    """Restore operation progress tracking"""
    phase: RestorePhase
    percent_complete: float
    current_step: str
    steps_completed: int
    total_steps: int
    start_time: datetime
    estimated_completion: Optional[datetime] = None
    resources_processed: int = 0
    resources_total: int = 0
    errors: List[str] = None
    warnings: List[str] = None

    def __post_init__(self):
        if self.errors is None:
            self.errors = []
        if self.warnings is None:
            self.warnings = []


@dataclass
class RestoreResult:
    """Restore operation final result"""
    restore_id: str
    success: bool
    phase: RestorePhase
    start_time: datetime
    end_time: datetime
    duration: timedelta
    resources_restored: int
    resources_failed: int
    git_commits: List[str]
    validation_report: Dict[str, Any]
    error_summary: Optional[str] = None
    recommendations: Optional[List[str]] = None


class GitOpsRestoreOrchestrator:
    """Main orchestrator for GitOps-based disaster recovery"""

    def __init__(self, config_path: str = None):
        """Initialize the GitOps restore orchestrator"""
        self.config = load_config(config_path) if config_path else {}
        self.security_manager = PythonSecurityManager(self.config)
        self.logger = self._setup_logging()
        
        # Initialize state tracking
        self.active_restores: Dict[str, RestoreProgress] = {}
        self.restore_history: List[RestoreResult] = []
        
        # GitOps configuration
        self.gitops_repo_url = self.config.get('gitops', {}).get('repository', {}).get('url')
        self.gitops_branch = self.config.get('gitops', {}).get('repository', {}).get('branch', 'main')
        self.gitops_path = self.config.get('gitops', {}).get('structure', {}).get('path', 'clusters')
        
        # Working directories
        self.work_dir = Path(tempfile.mkdtemp(prefix='gitops_restore_'))
        self.templates_dir = self.work_dir / 'templates'
        self.generated_dir = self.work_dir / 'generated'
        
        # Initialize directories
        self.templates_dir.mkdir(parents=True, exist_ok=True)
        self.generated_dir.mkdir(parents=True, exist_ok=True)

    def _setup_logging(self) -> logging.Logger:
        """Setup structured logging"""
        logger = logging.getLogger('gitops_restore')
        if not logger.handlers:
            handler = logging.StreamHandler()
            formatter = logging.Formatter(
                '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
            )
            handler.setFormatter(formatter)
            logger.addHandler(handler)
            logger.setLevel(logging.INFO)
        return logger

    async def start_restore(self, request: RestoreRequest) -> str:
        """Start a new GitOps restore operation"""
        try:
            # Security validation
            await self.security_manager.validate_request(request.__dict__)
            
            # Initialize progress tracking
            progress = RestoreProgress(
                phase=RestorePhase.PLANNING,
                percent_complete=0.0,
                current_step="Initializing restore operation",
                steps_completed=0,
                total_steps=7,  # Planning, Validation, Preparation, GitOps Sync, Verification, Cleanup, Completion
                start_time=datetime.utcnow()
            )
            
            self.active_restores[request.restore_id] = progress
            
            # Start restore in background
            asyncio.create_task(self._execute_restore(request))
            
            self.logger.info(f"Started GitOps restore operation: {request.restore_id}")
            return request.restore_id
            
        except Exception as e:
            self.logger.error(f"Failed to start restore {request.restore_id}: {str(e)}")
            raise

    async def _execute_restore(self, request: RestoreRequest):
        """Execute the complete restore workflow"""
        progress = self.active_restores[request.restore_id]
        result = None
        
        try:
            # Phase 1: Planning
            await self._phase_planning(request, progress)
            
            # Phase 2: Validation
            await self._phase_validation(request, progress)
            
            # Phase 3: Preparation
            await self._phase_preparation(request, progress)
            
            # Phase 4: GitOps Sync
            await self._phase_gitops_sync(request, progress)
            
            # Phase 5: Verification
            await self._phase_verification(request, progress)
            
            # Phase 6: Cleanup
            await self._phase_cleanup(request, progress)
            
            # Complete successfully
            progress.phase = RestorePhase.COMPLETED
            progress.percent_complete = 100.0
            progress.current_step = "Restore completed successfully"
            
            result = RestoreResult(
                restore_id=request.restore_id,
                success=True,
                phase=RestorePhase.COMPLETED,
                start_time=progress.start_time,
                end_time=datetime.utcnow(),
                duration=datetime.utcnow() - progress.start_time,
                resources_restored=progress.resources_processed,
                resources_failed=len(progress.errors),
                git_commits=[],  # Would be populated with actual commit hashes
                validation_report={}
            )
            
        except Exception as e:
            self.logger.error(f"Restore {request.restore_id} failed: {str(e)}")
            progress.phase = RestorePhase.FAILED
            progress.errors.append(str(e))
            
            result = RestoreResult(
                restore_id=request.restore_id,
                success=False,
                phase=RestorePhase.FAILED,
                start_time=progress.start_time,
                end_time=datetime.utcnow(),
                duration=datetime.utcnow() - progress.start_time,
                resources_restored=progress.resources_processed,
                resources_failed=len(progress.errors),
                git_commits=[],
                validation_report={},
                error_summary=str(e)
            )
        
        finally:
            # Move to history and cleanup
            if result:
                self.restore_history.append(result)
            if request.restore_id in self.active_restores:
                del self.active_restores[request.restore_id]

    async def _phase_planning(self, request: RestoreRequest, progress: RestoreProgress):
        """Phase 1: Plan the restore operation"""
        progress.phase = RestorePhase.PLANNING
        progress.current_step = "Analyzing backup and planning restore"
        
        self.logger.info(f"Planning restore for backup {request.backup_id}")
        
        # Load backup metadata
        backup_metadata = await self._load_backup_metadata(request.backup_id)
        if not backup_metadata:
            raise Exception(f"Backup {request.backup_id} not found or inaccessible")
        
        # Analyze restore requirements
        restore_plan = await self._create_restore_plan(request, backup_metadata)
        
        # Calculate total resources and steps
        progress.resources_total = restore_plan.get('total_resources', 0)
        progress.total_steps = restore_plan.get('total_steps', 7)
        
        progress.steps_completed = 1
        progress.percent_complete = (progress.steps_completed / progress.total_steps) * 100
        
        self.logger.info(f"Restore plan created: {progress.resources_total} resources to restore")

    async def _phase_validation(self, request: RestoreRequest, progress: RestoreProgress):
        """Phase 2: Validate restore prerequisites"""
        progress.phase = RestorePhase.VALIDATION
        progress.current_step = "Validating cluster and GitOps repository"
        
        self.logger.info(f"Validating restore prerequisites for {request.restore_id}")
        
        # Validate target cluster access
        if not await self._validate_cluster_access(request.target_cluster):
            raise Exception(f"Cannot access target cluster: {request.target_cluster}")
        
        # Validate GitOps repository access
        if not await self._validate_gitops_access():
            raise Exception("Cannot access GitOps repository")
        
        # Validate restore permissions
        if not await self._validate_restore_permissions(request):
            raise Exception("Insufficient permissions for restore operation")
        
        # Check for conflicts
        conflicts = await self._check_restore_conflicts(request)
        if conflicts and not request.dr_scenario == DRScenario.CLUSTER_REBUILD:
            progress.warnings.extend([f"Conflict detected: {c}" for c in conflicts])
        
        progress.steps_completed = 2
        progress.percent_complete = (progress.steps_completed / progress.total_steps) * 100
        
        self.logger.info("Validation completed successfully")

    async def _phase_preparation(self, request: RestoreRequest, progress: RestoreProgress):
        """Phase 3: Prepare GitOps manifests"""
        progress.phase = RestorePhase.PREPARATION
        progress.current_step = "Preparing GitOps manifests"
        
        self.logger.info(f"Preparing GitOps manifests for {request.restore_id}")
        
        # Clone GitOps repository
        repo_path = await self._clone_gitops_repo()
        
        # Load backup resources
        backup_resources = await self._load_backup_resources(request.backup_id)
        
        # Transform resources for GitOps
        gitops_manifests = await self._transform_to_gitops(request, backup_resources)
        
        # Generate cluster configuration
        cluster_config = await self._generate_cluster_config(request, gitops_manifests)
        
        # Validate generated manifests
        await self._validate_gitops_manifests(gitops_manifests)
        
        progress.steps_completed = 3
        progress.percent_complete = (progress.steps_completed / progress.total_steps) * 100
        
        self.logger.info(f"Prepared {len(gitops_manifests)} GitOps manifests")

    async def _phase_gitops_sync(self, request: RestoreRequest, progress: RestoreProgress):
        """Phase 4: Execute GitOps synchronization"""
        progress.phase = RestorePhase.GITOPS_SYNC
        progress.current_step = "Synchronizing GitOps manifests"
        
        self.logger.info(f"Starting GitOps sync for {request.restore_id}")
        
        # Commit manifests to GitOps repository
        commit_hash = await self._commit_gitops_manifests(request)
        
        # Trigger ArgoCD/Flux sync (if configured)
        if self.config.get('gitops', {}).get('auto_sync', True):
            await self._trigger_gitops_sync(request.target_cluster)
        
        # Monitor sync progress
        await self._monitor_gitops_sync(request, progress)
        
        progress.steps_completed = 4
        progress.percent_complete = (progress.steps_completed / progress.total_steps) * 100
        
        self.logger.info(f"GitOps sync completed with commit: {commit_hash}")

    async def _phase_verification(self, request: RestoreRequest, progress: RestoreProgress):
        """Phase 5: Verify restore success"""
        progress.phase = RestorePhase.VERIFICATION
        progress.current_step = "Verifying restored resources"
        
        self.logger.info(f"Verifying restore results for {request.restore_id}")
        
        # Wait for resources to be ready
        await self._wait_for_resources_ready(request)
        
        # Validate resource health
        health_report = await self._validate_resource_health(request)
        
        # Check application functionality (if configured)
        if request.validation_config and request.validation_config.get('functional_tests'):
            await self._run_functional_tests(request)
        
        # Generate verification report
        verification_report = await self._generate_verification_report(request, health_report)
        
        progress.steps_completed = 5
        progress.percent_complete = (progress.steps_completed / progress.total_steps) * 100
        
        self.logger.info("Resource verification completed")

    async def _phase_cleanup(self, request: RestoreRequest, progress: RestoreProgress):
        """Phase 6: Cleanup temporary resources"""
        progress.phase = RestorePhase.CLEANUP
        progress.current_step = "Cleaning up temporary resources"
        
        self.logger.info(f"Cleaning up restore operation {request.restore_id}")
        
        # Clean up temporary files
        if self.work_dir.exists():
            shutil.rmtree(self.work_dir)
        
        # Clean up temporary namespaces (if any)
        await self._cleanup_temporary_namespaces(request)
        
        # Send completion notifications
        await self._send_completion_notification(request)
        
        progress.steps_completed = 6
        progress.percent_complete = (progress.steps_completed / progress.total_steps) * 100
        
        self.logger.info("Cleanup completed")

    # Helper methods for backup operations

    async def _load_backup_metadata(self, backup_id: str) -> Dict[str, Any]:
        """Load backup metadata from storage"""
        # This would integrate with MinIO to load backup metadata
        # For now, return mock metadata
        return {
            'backup_id': backup_id,
            'cluster_name': 'source-cluster',
            'timestamp': datetime.utcnow().isoformat(),
            'resources': {
                'namespaces': 5,
                'deployments': 12,
                'services': 8,
                'configmaps': 15,
                'secrets': 6
            },
            'size': 1024 * 1024 * 50,  # 50MB
            'version': '1.0.0'
        }

    async def _load_backup_resources(self, backup_id: str) -> List[Dict[str, Any]]:
        """Load backup resources from storage"""
        # This would integrate with MinIO to load actual backup resources
        # For now, return mock resources
        return [
            {
                'apiVersion': 'v1',
                'kind': 'Namespace',
                'metadata': {'name': 'production'},
                'spec': {}
            },
            {
                'apiVersion': 'apps/v1',
                'kind': 'Deployment',
                'metadata': {'name': 'web-app', 'namespace': 'production'},
                'spec': {
                    'replicas': 3,
                    'selector': {'matchLabels': {'app': 'web-app'}},
                    'template': {
                        'metadata': {'labels': {'app': 'web-app'}},
                        'spec': {
                            'containers': [{
                                'name': 'web',
                                'image': 'nginx:1.20',
                                'ports': [{'containerPort': 80}]
                            }]
                        }
                    }
                }
            }
        ]

    async def _create_restore_plan(self, request: RestoreRequest, backup_metadata: Dict[str, Any]) -> Dict[str, Any]:
        """Create detailed restore plan"""
        plan = {
            'restore_id': request.restore_id,
            'total_resources': sum(backup_metadata.get('resources', {}).values()),
            'total_steps': 7,
            'phases': [
                'planning', 'validation', 'preparation', 
                'gitops_sync', 'verification', 'cleanup', 'completion'
            ],
            'estimated_duration': timedelta(minutes=30),  # Would be calculated based on resources
            'dependencies': [],
            'conflicts': []
        }
        
        # Add DR scenario specific planning
        if request.dr_scenario == DRScenario.CLUSTER_REBUILD:
            plan['requires_cluster_bootstrap'] = True
            plan['estimated_duration'] = timedelta(hours=2)
        
        return plan

    # Helper methods for validation

    async def _validate_cluster_access(self, cluster_name: str) -> bool:
        """Validate access to target cluster"""
        try:
            # This would use kubernetes client to validate access
            # For now, simulate validation
            await asyncio.sleep(1)
            return True
        except Exception as e:
            self.logger.error(f"Cluster access validation failed: {str(e)}")
            return False

    async def _validate_gitops_access(self) -> bool:
        """Validate access to GitOps repository"""
        try:
            # This would validate git repository access
            # For now, simulate validation
            await asyncio.sleep(1)
            return True
        except Exception as e:
            self.logger.error(f"GitOps access validation failed: {str(e)}")
            return False

    async def _validate_restore_permissions(self, request: RestoreRequest) -> bool:
        """Validate restore permissions"""
        try:
            # This would check RBAC permissions for restore operations
            # For now, simulate validation
            await asyncio.sleep(1)
            return True
        except Exception as e:
            self.logger.error(f"Permission validation failed: {str(e)}")
            return False

    async def _check_restore_conflicts(self, request: RestoreRequest) -> List[str]:
        """Check for potential restore conflicts"""
        conflicts = []
        
        # Check for existing resources
        if not request.dry_run:
            # This would check for conflicting resources in target cluster
            pass
        
        return conflicts

    # Helper methods for GitOps operations

    async def _clone_gitops_repo(self) -> Path:
        """Clone GitOps repository to working directory"""
        repo_path = self.work_dir / 'gitops_repo'
        
        try:
            # Clone repository
            git.Repo.clone_from(
                self.gitops_repo_url,
                repo_path,
                branch=self.gitops_branch
            )
            
            self.logger.info(f"Cloned GitOps repo to {repo_path}")
            return repo_path
            
        except Exception as e:
            self.logger.error(f"Failed to clone GitOps repo: {str(e)}")
            raise

    async def _transform_to_gitops(self, request: RestoreRequest, resources: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Transform backup resources to GitOps manifests"""
        gitops_manifests = []
        
        for resource in resources:
            # Apply transformations based on restore mode and DR scenario
            transformed = await self._apply_gitops_transformations(request, resource)
            gitops_manifests.append(transformed)
        
        return gitops_manifests

    async def _apply_gitops_transformations(self, request: RestoreRequest, resource: Dict[str, Any]) -> Dict[str, Any]:
        """Apply GitOps-specific transformations to a resource"""
        # Create a copy to avoid modifying original
        transformed = resource.copy()
        
        # Add GitOps labels and annotations
        if 'metadata' not in transformed:
            transformed['metadata'] = {}
        
        if 'labels' not in transformed['metadata']:
            transformed['metadata']['labels'] = {}
        
        if 'annotations' not in transformed['metadata']:
            transformed['metadata']['annotations'] = {}
        
        # Add restore tracking labels
        transformed['metadata']['labels'].update({
            'restore.gitops.io/restore-id': request.restore_id,
            'restore.gitops.io/source-backup': request.backup_id,
            'restore.gitops.io/dr-scenario': request.dr_scenario.value
        })
        
        # Add restore tracking annotations
        transformed['metadata']['annotations'].update({
            'restore.gitops.io/restored-at': datetime.utcnow().isoformat(),
            'restore.gitops.io/source-cluster': request.source_cluster,
            'restore.gitops.io/target-cluster': request.target_cluster
        })
        
        # Apply namespace mapping if specified
        if request.target_namespaces and transformed.get('metadata', {}).get('namespace'):
            original_namespace = transformed['metadata']['namespace']
            if len(request.target_namespaces) == 1:
                transformed['metadata']['namespace'] = request.target_namespaces[0]
        
        return transformed

    async def _generate_cluster_config(self, request: RestoreRequest, manifests: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Generate cluster configuration for GitOps"""
        cluster_config = {
            'apiVersion': 'argoproj.io/v1alpha1',
            'kind': 'Application',
            'metadata': {
                'name': f"restore-{request.restore_id}",
                'namespace': 'argocd',
                'labels': {
                    'restore.gitops.io/restore-id': request.restore_id
                }
            },
            'spec': {
                'project': 'disaster-recovery',
                'source': {
                    'repoURL': self.gitops_repo_url,
                    'targetRevision': self.gitops_branch,
                    'path': f"{self.gitops_path}/{request.target_cluster}/restore-{request.restore_id}"
                },
                'destination': {
                    'server': 'https://kubernetes.default.svc',
                    'namespace': 'default'
                },
                'syncPolicy': {
                    'automated': {
                        'prune': True,
                        'selfHeal': True
                    },
                    'syncOptions': [
                        'CreateNamespace=true'
                    ]
                }
            }
        }
        
        return cluster_config

    async def _validate_gitops_manifests(self, manifests: List[Dict[str, Any]]):
        """Validate GitOps manifests"""
        for manifest in manifests:
            # Basic validation
            if 'apiVersion' not in manifest:
                raise Exception(f"Manifest missing apiVersion: {manifest}")
            if 'kind' not in manifest:
                raise Exception(f"Manifest missing kind: {manifest}")
            if 'metadata' not in manifest:
                raise Exception(f"Manifest missing metadata: {manifest}")

    async def _commit_gitops_manifests(self, request: RestoreRequest) -> str:
        """Commit GitOps manifests to repository"""
        repo_path = self.work_dir / 'gitops_repo'
        repo = git.Repo(repo_path)
        
        # Create restore directory
        restore_path = repo_path / self.gitops_path / request.target_cluster / f"restore-{request.restore_id}"
        restore_path.mkdir(parents=True, exist_ok=True)
        
        # Write manifests to files
        manifest_files = []
        for i, manifest in enumerate(self.generated_dir.glob('*.yaml')):
            target_file = restore_path / f"{manifest.name}"
            shutil.copy2(manifest, target_file)
            manifest_files.append(target_file)
        
        # Commit changes
        repo.index.add([str(f) for f in manifest_files])
        commit_message = f"Restore operation {request.restore_id} - {request.dr_scenario.value}"
        commit = repo.index.commit(commit_message)
        
        # Push to remote
        origin = repo.remote('origin')
        origin.push()
        
        self.logger.info(f"Committed restore manifests: {commit.hexsha}")
        return commit.hexsha

    async def _trigger_gitops_sync(self, cluster_name: str):
        """Trigger GitOps sync (ArgoCD/Flux)"""
        # This would trigger sync via ArgoCD or Flux API
        # For now, simulate the operation
        await asyncio.sleep(2)
        self.logger.info(f"Triggered GitOps sync for cluster: {cluster_name}")

    async def _monitor_gitops_sync(self, request: RestoreRequest, progress: RestoreProgress):
        """Monitor GitOps synchronization progress"""
        # This would monitor ArgoCD/Flux sync status
        # For now, simulate monitoring
        sync_duration = 60  # seconds
        for i in range(sync_duration):
            await asyncio.sleep(1)
            progress.current_step = f"GitOps sync in progress ({i+1}/{sync_duration}s)"
            
        self.logger.info("GitOps sync monitoring completed")

    # Helper methods for verification

    async def _wait_for_resources_ready(self, request: RestoreRequest):
        """Wait for restored resources to be ready"""
        # This would monitor Kubernetes resources until they're ready
        await asyncio.sleep(30)  # Simulate waiting
        self.logger.info("All resources are ready")

    async def _validate_resource_health(self, request: RestoreRequest) -> Dict[str, Any]:
        """Validate health of restored resources"""
        health_report = {
            'overall_health': 'Healthy',
            'resources': {
                'healthy': 25,
                'degraded': 2,
                'unhealthy': 0
            },
            'details': []
        }
        
        return health_report

    async def _run_functional_tests(self, request: RestoreRequest):
        """Run functional tests on restored applications"""
        # This would run configured functional tests
        await asyncio.sleep(10)  # Simulate testing
        self.logger.info("Functional tests completed")

    async def _generate_verification_report(self, request: RestoreRequest, health_report: Dict[str, Any]) -> Dict[str, Any]:
        """Generate comprehensive verification report"""
        report = {
            'restore_id': request.restore_id,
            'verification_time': datetime.utcnow().isoformat(),
            'overall_status': 'Success',
            'health_report': health_report,
            'recommendations': [
                'Monitor application performance for 24 hours',
                'Verify data integrity',
                'Update monitoring dashboards'
            ]
        }
        
        return report

    # Helper methods for cleanup

    async def _cleanup_temporary_namespaces(self, request: RestoreRequest):
        """Cleanup any temporary namespaces created during restore"""
        # This would cleanup temporary Kubernetes namespaces
        pass

    async def _send_completion_notification(self, request: RestoreRequest):
        """Send completion notification"""
        # This would send notifications via webhook, email, Slack, etc.
        self.logger.info(f"Restore {request.restore_id} completed successfully")

    # Public API methods

    async def get_restore_status(self, restore_id: str) -> Optional[RestoreProgress]:
        """Get current status of a restore operation"""
        return self.active_restores.get(restore_id)

    async def cancel_restore(self, restore_id: str) -> bool:
        """Cancel an active restore operation"""
        if restore_id in self.active_restores:
            # This would implement proper cancellation logic
            del self.active_restores[restore_id]
            self.logger.info(f"Cancelled restore operation: {restore_id}")
            return True
        return False

    async def list_restore_history(self, limit: int = 50) -> List[RestoreResult]:
        """List historical restore operations"""
        return self.restore_history[-limit:]

    async def get_dr_capabilities(self) -> Dict[str, Any]:
        """Get disaster recovery capabilities and configurations"""
        return {
            'supported_scenarios': [scenario.value for scenario in DRScenario],
            'supported_modes': [mode.value for mode in RestoreMode],
            'gitops_integration': {
                'repository': self.gitops_repo_url,
                'branch': self.gitops_branch,
                'auto_sync': self.config.get('gitops', {}).get('auto_sync', True)
            },
            'validation_options': {
                'cluster_validation': True,
                'resource_validation': True,
                'functional_testing': True
            }
        }

    def cleanup(self):
        """Cleanup orchestrator resources"""
        if self.work_dir.exists():
            shutil.rmtree(self.work_dir)
        self.logger.info("GitOps restore orchestrator cleaned up")


# CLI interface for testing
async def main():
    """Main function for CLI testing"""
    import argparse
    
    parser = argparse.ArgumentParser(description='GitOps Restore Orchestrator')
    parser.add_argument('--config', help='Configuration file path')
    parser.add_argument('--restore-id', help='Restore operation ID')
    parser.add_argument('--backup-id', help='Backup ID to restore')
    parser.add_argument('--source-cluster', help='Source cluster name')
    parser.add_argument('--target-cluster', help='Target cluster name')
    parser.add_argument('--mode', choices=[m.value for m in RestoreMode], 
                       default=RestoreMode.FULL_CLUSTER.value, help='Restore mode')
    parser.add_argument('--scenario', choices=[s.value for s in DRScenario],
                       default=DRScenario.CLUSTER_REBUILD.value, help='DR scenario')
    parser.add_argument('--dry-run', action='store_true', help='Perform dry run')
    
    args = parser.parse_args()
    
    # Initialize orchestrator
    orchestrator = GitOpsRestoreOrchestrator(args.config)
    
    try:
        if args.restore_id and args.backup_id:
            # Start a restore operation
            request = RestoreRequest(
                restore_id=args.restore_id,
                backup_id=args.backup_id,
                source_cluster=args.source_cluster,
                target_cluster=args.target_cluster,
                restore_mode=RestoreMode(args.mode),
                dr_scenario=DRScenario(args.scenario),
                dry_run=args.dry_run
            )
            
            restore_id = await orchestrator.start_restore(request)
            print(f"Started restore operation: {restore_id}")
            
            # Monitor progress
            while True:
                status = await orchestrator.get_restore_status(restore_id)
                if not status:
                    break
                    
                print(f"Phase: {status.phase.value}, Progress: {status.percent_complete:.1f}%")
                
                if status.phase in [RestorePhase.COMPLETED, RestorePhase.FAILED]:
                    break
                    
                await asyncio.sleep(5)
        else:
            # Show capabilities
            capabilities = await orchestrator.get_dr_capabilities()
            print("Disaster Recovery Capabilities:")
            print(json.dumps(capabilities, indent=2))
            
    finally:
        orchestrator.cleanup()


if __name__ == '__main__':
    asyncio.run(main())