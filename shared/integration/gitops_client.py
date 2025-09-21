#!/usr/bin/env python3
"""
GitOps Client for Integration Bridge

Provides Python interface for integrating with the GitOps generator component
and communicating with the integration bridge.
"""

import asyncio
import json
import logging
import time
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any, Union
from urllib.parse import urljoin

import aiohttp
import yaml

logger = logging.getLogger(__name__)


@dataclass
class GitOpsRequest:
    """GitOps generation request"""
    request_id: str
    backup_id: str
    cluster_name: str
    source_path: str
    target_repo: str
    target_branch: str = "main"
    configuration: Optional[Dict[str, Any]] = None
    timestamp: Optional[datetime] = None
    
    def __post_init__(self):
        if self.timestamp is None:
            self.timestamp = datetime.utcnow()
        if self.configuration is None:
            self.configuration = {}


@dataclass
class GitOpsResponse:
    """GitOps generation response"""
    request_id: str
    status: str
    message: str
    start_time: datetime
    estimated_time: Optional[timedelta] = None
    progress: Optional[Dict[str, Any]] = None
    metadata: Optional[Dict[str, Any]] = None


@dataclass
class GitOpsStatus:
    """GitOps generation status"""
    request_id: str
    status: str  # pending, running, completed, failed
    start_time: datetime
    end_time: Optional[datetime] = None
    duration: Optional[timedelta] = None
    files_generated: int = 0
    files_committed: int = 0
    git_commit_hash: Optional[str] = None
    error_message: Optional[str] = None
    progress: Optional[Dict[str, Any]] = None
    metadata: Optional[Dict[str, Any]] = None


@dataclass
class GitOpsProgress:
    """GitOps generation progress"""
    total_resources: int
    processed_resources: int
    percent_complete: float
    current_namespace: str
    current_resource: str
    files_generated: int
    estimated_files: int


class GitOpsClient:
    """Client for communicating with GitOps generator and integration bridge"""
    
    def __init__(self, config: Dict[str, Any], monitoring_client=None):
        self.config = config
        self.monitoring = monitoring_client
        
        # Extract configuration
        integration_config = config.get('integration', {})
        endpoints = integration_config.get('communication', {}).get('endpoints', {})
        
        self.gitops_url = endpoints.get('gitops_generator', 'http://localhost:8081')
        self.bridge_url = endpoints.get('integration_bridge', 'http://localhost:8080')
        
        # HTTP client configuration
        timeout = aiohttp.ClientTimeout(total=30)
        self.session = aiohttp.ClientSession(timeout=timeout)
        
        logger.info(f"GitOps client initialized - GitOps: {self.gitops_url}, Bridge: {self.bridge_url}")
    
    async def close(self):
        """Close the HTTP session"""
        if self.session:
            await self.session.close()
    
    async def __aenter__(self):
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()
    
    async def register_with_bridge(self, version: str = "2.1.0") -> bool:
        """Register this GitOps client with the integration bridge"""
        try:
            registration_data = {
                "endpoint": self.gitops_url,
                "version": version
            }
            
            async with self.session.post(
                urljoin(self.bridge_url, "/register/gitops"),
                json=registration_data
            ) as response:
                if response.status == 200:
                    logger.info("Successfully registered with integration bridge")
                    if self.monitoring:
                        self.monitoring.inc_counter("gitops_client_registrations", {"status": "success"})
                    return True
                else:
                    logger.error(f"Registration failed with status: {response.status}")
                    if self.monitoring:
                        self.monitoring.inc_counter("gitops_client_registrations", {"status": "failure"})
                    return False
                    
        except Exception as e:
            logger.error(f"Failed to register with bridge: {e}")
            if self.monitoring:
                self.monitoring.inc_counter("gitops_client_registrations", {"status": "error"})
            return False
    
    async def start_gitops_generation(self, request: GitOpsRequest) -> GitOpsResponse:
        """Start GitOps generation process"""
        try:
            start_time = time.time()
            
            async with self.session.post(
                urljoin(self.gitops_url, "/api/gitops/generate"),
                json=asdict(request)
            ) as response:
                duration = time.time() - start_time
                
                if self.monitoring:
                    self.monitoring.record_duration(
                        "gitops_client_request_duration",
                        {"operation": "start_generation"},
                        duration
                    )
                
                if response.status == 200:
                    data = await response.json()
                    gitops_response = GitOpsResponse(**data)
                    
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "start_generation", "status": "success"}
                        )
                    
                    return gitops_response
                else:
                    error_text = await response.text()
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "start_generation", "status": "failed"}
                        )
                    raise Exception(f"GitOps generation failed: {response.status} - {error_text}")
                    
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "gitops_client_requests",
                    {"operation": "start_generation", "status": "error"}
                )
            raise Exception(f"Failed to start GitOps generation: {e}")
    
    async def get_gitops_status(self, request_id: str) -> GitOpsStatus:
        """Get status of GitOps generation"""
        try:
            start_time = time.time()
            
            async with self.session.get(
                urljoin(self.gitops_url, f"/api/gitops/status/{request_id}")
            ) as response:
                duration = time.time() - start_time
                
                if self.monitoring:
                    self.monitoring.record_duration(
                        "gitops_client_request_duration",
                        {"operation": "get_status"},
                        duration
                    )
                
                if response.status == 200:
                    data = await response.json()
                    # Convert datetime strings back to datetime objects
                    if 'start_time' in data:
                        data['start_time'] = datetime.fromisoformat(data['start_time'].replace('Z', '+00:00'))
                    if 'end_time' in data and data['end_time']:
                        data['end_time'] = datetime.fromisoformat(data['end_time'].replace('Z', '+00:00'))
                    
                    status = GitOpsStatus(**data)
                    
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "get_status", "status": "success"}
                        )
                    
                    return status
                elif response.status == 404:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "get_status", "status": "not_found"}
                        )
                    raise Exception(f"GitOps request not found: {request_id}")
                else:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "get_status", "status": "failed"}
                        )
                    raise Exception(f"Failed to get status: {response.status}")
                    
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "gitops_client_requests",
                    {"operation": "get_status", "status": "error"}
                )
            raise Exception(f"Failed to get GitOps status: {e}")
    
    async def list_gitops_requests(self, limit: int = 50, offset: int = 0) -> List[GitOpsStatus]:
        """List GitOps generation requests"""
        try:
            start_time = time.time()
            
            params = {"limit": limit, "offset": offset}
            async with self.session.get(
                urljoin(self.gitops_url, "/api/gitops/list"),
                params=params
            ) as response:
                duration = time.time() - start_time
                
                if self.monitoring:
                    self.monitoring.record_duration(
                        "gitops_client_request_duration",
                        {"operation": "list_requests"},
                        duration
                    )
                
                if response.status == 200:
                    data = await response.json()
                    
                    # Convert data to GitOpsStatus objects
                    requests = []
                    for item in data:
                        if 'start_time' in item:
                            item['start_time'] = datetime.fromisoformat(item['start_time'].replace('Z', '+00:00'))
                        if 'end_time' in item and item['end_time']:
                            item['end_time'] = datetime.fromisoformat(item['end_time'].replace('Z', '+00:00'))
                        requests.append(GitOpsStatus(**item))
                    
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "list_requests", "status": "success"}
                        )
                    
                    return requests
                else:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "list_requests", "status": "failed"}
                        )
                    raise Exception(f"Failed to list requests: {response.status}")
                    
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "gitops_client_requests",
                    {"operation": "list_requests", "status": "error"}
                )
            raise Exception(f"Failed to list GitOps requests: {e}")
    
    async def wait_for_completion(self, request_id: str, timeout: int = 600) -> GitOpsStatus:
        """Wait for GitOps generation to complete"""
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            try:
                status = await self.get_gitops_status(request_id)
                
                if status.status == "completed":
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_completions",
                            {"status": "success"}
                        )
                    return status
                elif status.status == "failed":
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_completions",
                            {"status": "failure"}
                        )
                    raise Exception(f"GitOps generation failed: {status.error_message}")
                elif status.status in ["running", "pending"]:
                    # Continue waiting
                    await asyncio.sleep(5)
                    continue
                else:
                    raise Exception(f"Unknown GitOps status: {status.status}")
                    
            except Exception as e:
                logger.error(f"Error while waiting for completion: {e}")
                await asyncio.sleep(5)
        
        raise Exception(f"GitOps generation timed out after {timeout} seconds")
    
    async def cancel_gitops_generation(self, request_id: str) -> bool:
        """Cancel GitOps generation"""
        try:
            start_time = time.time()
            
            async with self.session.post(
                urljoin(self.gitops_url, f"/api/gitops/cancel/{request_id}")
            ) as response:
                duration = time.time() - start_time
                
                if self.monitoring:
                    self.monitoring.record_duration(
                        "gitops_client_request_duration",
                        {"operation": "cancel_generation"},
                        duration
                    )
                
                if response.status == 200:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "cancel_generation", "status": "success"}
                        )
                    return True
                else:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_requests",
                            {"operation": "cancel_generation", "status": "failed"}
                        )
                    return False
                    
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "gitops_client_requests",
                    {"operation": "cancel_generation", "status": "error"}
                )
            logger.error(f"Failed to cancel GitOps generation: {e}")
            return False
    
    async def notify_completion(self, status: GitOpsStatus) -> bool:
        """Notify integration bridge of GitOps completion"""
        try:
            webhook_request = {
                "id": f"gitops-completion-{status.request_id}",
                "type": "gitops_completed",
                "source": "gitops-generator",
                "timestamp": datetime.utcnow().isoformat(),
                "data": {
                    "request_id": status.request_id,
                    "status": status.status,
                    "files_generated": status.files_generated,
                    "files_committed": status.files_committed,
                    "git_commit_hash": status.git_commit_hash,
                    "error": status.error_message,
                    "duration_seconds": status.duration.total_seconds() if status.duration else None
                }
            }
            
            async with self.session.post(
                urljoin(self.bridge_url, "/webhooks/gitops/completed"),
                json=webhook_request
            ) as response:
                if response.status == 200:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_notifications",
                            {"type": "completion", "status": "success"}
                        )
                    return True
                else:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "gitops_client_notifications",
                            {"type": "completion", "status": "failed"}
                        )
                    logger.error(f"Completion notification failed: {response.status}")
                    return False
                    
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "gitops_client_notifications",
                    {"type": "completion", "status": "error"}
                )
            logger.error(f"Failed to notify completion: {e}")
            return False
    
    async def get_health_status(self) -> Dict[str, Any]:
        """Get health status of GitOps service"""
        try:
            async with self.session.get(
                urljoin(self.gitops_url, "/health")
            ) as response:
                if response.status == 200:
                    return await response.json()
                else:
                    raise Exception(f"Health check failed: {response.status}")
                    
        except Exception as e:
            raise Exception(f"Failed to get health status: {e}")
    
    def create_request_from_backup(self, backup_event: Dict[str, Any], config_override: Optional[Dict[str, Any]] = None) -> GitOpsRequest:
        """Create GitOps request from backup completion event"""
        backup_id = backup_event.get("backup_id")
        cluster_name = backup_event.get("cluster_name")
        minio_path = backup_event.get("minio_path")
        
        # Get GitOps configuration
        gitops_config = self.config.get("gitops", {})
        target_repo = gitops_config.get("repository", "")
        target_branch = gitops_config.get("branch", "main")
        
        # Apply any configuration overrides
        if config_override:
            target_repo = config_override.get("target_repo", target_repo)
            target_branch = config_override.get("target_branch", target_branch)
        
        # Create request
        request = GitOpsRequest(
            request_id=f"gitops-{backup_id}-{int(time.time())}",
            backup_id=backup_id,
            cluster_name=cluster_name,
            source_path=minio_path,
            target_repo=target_repo,
            target_branch=target_branch,
            configuration={
                "backup_timestamp": backup_event.get("timestamp"),
                "resource_count": backup_event.get("resource_count"),
                "size_bytes": backup_event.get("size"),
                "original_cluster": cluster_name
            }
        )
        
        return request


# Convenience functions for common operations

async def process_backup_completion(config: Dict[str, Any], backup_event: Dict[str, Any], monitoring_client=None) -> GitOpsStatus:
    """Process backup completion and generate GitOps artifacts"""
    async with GitOpsClient(config, monitoring_client) as client:
        # Create GitOps request from backup event
        request = client.create_request_from_backup(backup_event)
        
        # Start GitOps generation
        response = await client.start_gitops_generation(request)
        logger.info(f"Started GitOps generation: {response.request_id}")
        
        # Wait for completion
        status = await client.wait_for_completion(request.request_id)
        
        # Notify bridge of completion
        await client.notify_completion(status)
        
        return status


async def get_integration_status(config: Dict[str, Any]) -> Dict[str, Any]:
    """Get overall integration status"""
    async with GitOpsClient(config) as client:
        try:
            gitops_health = await client.get_health_status()
            
            # Try to get bridge status
            try:
                async with client.session.get(
                    urljoin(client.bridge_url, "/status")
                ) as response:
                    if response.status == 200:
                        bridge_status = await response.json()
                    else:
                        bridge_status = {"error": f"Bridge unreachable: {response.status}"}
            except Exception as e:
                bridge_status = {"error": f"Bridge unreachable: {e}"}
            
            return {
                "gitops": gitops_health,
                "bridge": bridge_status,
                "timestamp": datetime.utcnow().isoformat()
            }
            
        except Exception as e:
            return {
                "error": str(e),
                "timestamp": datetime.utcnow().isoformat()
            }


if __name__ == "__main__":
    # Example usage
    import sys
    
    async def main():
        # Load configuration
        config = {
            "integration": {
                "communication": {
                    "endpoints": {
                        "gitops_generator": "http://localhost:8081",
                        "integration_bridge": "http://localhost:8080"
                    }
                }
            },
            "gitops": {
                "repository": "https://github.com/org/gitops-repo",
                "branch": "main"
            }
        }
        
        # Test integration
        status = await get_integration_status(config)
        print(json.dumps(status, indent=2))
    
    if len(sys.argv) > 1 and sys.argv[1] == "test":
        asyncio.run(main())