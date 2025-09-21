#!/usr/bin/env python3
"""
Secure GitOps Client with Security Framework Integration

Enhanced GitOps client that integrates with the Python security framework,
providing authentication, encryption, input validation, and audit logging.
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

# Import the security framework
from security.python_security import (
    PythonSecurityManager, SecurityConfig, AuthContext, SecurityEvent,
    SecurityError, AuthenticationError, AuthorizationError, ValidationError,
    create_security_config_from_dict, secure_http_request
)

logger = logging.getLogger(__name__)


@dataclass
class SecureGitOpsRequest:
    """Secure GitOps generation request with authentication context"""
    request_id: str
    backup_id: str
    cluster_name: str
    source_path: str
    target_repo: str
    target_branch: str = "main"
    configuration: Optional[Dict[str, Any]] = None
    timestamp: Optional[datetime] = None
    auth_context: Optional[AuthContext] = None
    security_metadata: Optional[Dict[str, Any]] = None
    
    def __post_init__(self):
        if self.timestamp is None:
            self.timestamp = datetime.utcnow()
        if self.configuration is None:
            self.configuration = {}
        if self.security_metadata is None:
            self.security_metadata = {}


@dataclass
class SecureGitOpsResponse:
    """Secure GitOps generation response with security context"""
    request_id: str
    status: str
    message: str
    start_time: datetime
    estimated_time: Optional[timedelta] = None
    progress: Optional[Dict[str, Any]] = None
    metadata: Optional[Dict[str, Any]] = None
    security_context: Optional[Dict[str, Any]] = None


class SecureGitOpsClient:
    """Secure GitOps client with comprehensive security integration"""
    
    def __init__(self, config: Dict[str, Any], monitoring_client=None):
        self.config = config
        self.monitoring = monitoring_client
        
        # Initialize security manager
        self.security_config = create_security_config_from_dict(config)
        self.security_manager = PythonSecurityManager(self.security_config)
        
        # Extract configuration
        integration_config = config.get('integration', {})
        endpoints = integration_config.get('communication', {}).get('endpoints', {})
        
        self.gitops_url = endpoints.get('gitops_generator', 'https://localhost:8443')
        self.bridge_url = endpoints.get('integration_bridge', 'https://localhost:8443')
        
        # Session will be created with security configuration
        self.session = None
        self.auth_context = None
        
        logger.info(f"Secure GitOps client initialized - GitOps: {self.gitops_url}, Bridge: {self.bridge_url}")
    
    async def __aenter__(self):
        # Create secure HTTP session
        self.session = await self.security_manager.create_secure_http_session()
        
        # Authenticate the client
        await self._authenticate_client()
        
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self.session:
            await self.session.close()
    
    async def _authenticate_client(self):
        """Authenticate the client with the security framework"""
        try:
            headers = {}
            if self.security_config.api_key:
                headers["X-API-Key"] = self.security_config.api_key
            
            self.auth_context = await self.security_manager.authenticate_request(
                headers, 
                source_ip="127.0.0.1"  # Client-side authentication
            )
            
            logger.info(f"Client authenticated successfully: {self.auth_context.client_id}")
            
        except SecurityError as e:
            logger.error(f"Client authentication failed: {e}")
            raise
    
    async def register_with_bridge(self, version: str = "2.1.0") -> bool:
        """Register this secure GitOps client with the integration bridge"""
        try:
            # Validate authentication
            if not self.auth_context or not self.auth_context.authenticated:
                raise AuthenticationError("Client must be authenticated to register")
            
            registration_data = {
                "endpoint": self.gitops_url,
                "version": version,
                "security_enabled": True,
                "auth_method": self.auth_context.auth_method,
                "registration_time": datetime.utcnow().isoformat()
            }
            
            # Sign the request for integrity
            if self.security_manager.crypto_manager:
                signature = self.security_manager.crypto_manager.sign_request(
                    "POST", "/register/gitops", json.dumps(registration_data)
                )
                registration_data["signature"] = signature
            
            async with secure_http_request(
                self.session, "POST", 
                urljoin(self.bridge_url, "/register/gitops"),
                self.security_manager,
                json=registration_data
            ) as response:
                if response.status == 200:
                    logger.info("Successfully registered with integration bridge")
                    if self.monitoring:
                        self.monitoring.inc_counter("secure_gitops_client_registrations", {"status": "success"})
                    
                    # Log security event
                    await self._log_security_event(
                        "client_registration",
                        "success",
                        f"Client registered with bridge successfully"
                    )
                    
                    return True
                else:
                    error_text = await response.text()
                    logger.error(f"Registration failed with status: {response.status} - {error_text}")
                    if self.monitoring:
                        self.monitoring.inc_counter("secure_gitops_client_registrations", {"status": "failure"})
                    
                    await self._log_security_event(
                        "client_registration_failed",
                        "failed",
                        f"Registration failed: {response.status} - {error_text}"
                    )
                    
                    return False
                    
        except SecurityError as e:
            logger.error(f"Security error during registration: {e}")
            if self.monitoring:
                self.monitoring.inc_counter("secure_gitops_client_registrations", {"status": "security_error"})
            raise
        except Exception as e:
            logger.error(f"Failed to register with bridge: {e}")
            if self.monitoring:
                self.monitoring.inc_counter("secure_gitops_client_registrations", {"status": "error"})
            raise
    
    async def start_secure_gitops_generation(self, request: SecureGitOpsRequest) -> SecureGitOpsResponse:
        """Start secure GitOps generation process"""
        try:
            start_time = time.time()
            
            # Validate authentication and authorization
            if not self.auth_context or not self.auth_context.authenticated:
                raise AuthenticationError("Authentication required for GitOps generation")
            
            self.security_manager.authorize_request(self.auth_context, "gitops_generate")
            
            # Add security context to request
            request.auth_context = self.auth_context
            request.security_metadata = {
                "client_id": self.auth_context.client_id,
                "auth_method": self.auth_context.auth_method,
                "request_time": datetime.utcnow().isoformat(),
                "security_level": "high"
            }
            
            # Validate request data
            await self._validate_gitops_request(request)
            
            # Prepare secure request data
            request_data = asdict(request)
            request_data["timestamp"] = request.timestamp.isoformat() if request.timestamp else None
            
            # Sign the request
            if self.security_manager.crypto_manager:
                signature = self.security_manager.crypto_manager.sign_request(
                    "POST", "/api/gitops/generate", json.dumps(request_data)
                )
                request_data["signature"] = signature
            
            async with secure_http_request(
                self.session, "POST",
                urljoin(self.gitops_url, "/api/gitops/generate"),
                self.security_manager,
                json=request_data
            ) as response:
                duration = time.time() - start_time
                
                if self.monitoring:
                    self.monitoring.record_duration(
                        "secure_gitops_client_request_duration",
                        {"operation": "start_generation"},
                        duration
                    )
                
                if response.status == 200:
                    data = await response.json()
                    
                    # Add security context to response
                    data["security_context"] = {
                        "authenticated": True,
                        "client_id": self.auth_context.client_id,
                        "request_validated": True
                    }
                    
                    gitops_response = SecureGitOpsResponse(**data)
                    
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_requests",
                            {"operation": "start_generation", "status": "success"}
                        )
                    
                    # Log security event
                    await self._log_security_event(
                        "gitops_generation_started",
                        "success",
                        f"GitOps generation started for backup {request.backup_id}"
                    )
                    
                    return gitops_response
                else:
                    error_text = await response.text()
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_requests",
                            {"operation": "start_generation", "status": "failed"}
                        )
                    
                    await self._log_security_event(
                        "gitops_generation_failed",
                        "failed",
                        f"GitOps generation failed: {response.status} - {error_text}"
                    )
                    
                    raise SecurityError(f"GitOps generation failed: {response.status} - {error_text}")
                    
        except SecurityError as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "secure_gitops_client_requests",
                    {"operation": "start_generation", "status": "security_error"}
                )
            raise
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "secure_gitops_client_requests",
                    {"operation": "start_generation", "status": "error"}
                )
            raise SecurityError(f"Failed to start secure GitOps generation: {e}")
    
    async def get_secure_gitops_status(self, request_id: str) -> Dict[str, Any]:
        """Get status of GitOps generation with security validation"""
        try:
            # Validate authentication
            if not self.auth_context or not self.auth_context.authenticated:
                raise AuthenticationError("Authentication required for status access")
            
            self.security_manager.authorize_request(self.auth_context, "gitops_status")
            
            start_time = time.time()
            
            async with secure_http_request(
                self.session, "GET",
                urljoin(self.gitops_url, f"/api/gitops/status/{request_id}"),
                self.security_manager
            ) as response:
                duration = time.time() - start_time
                
                if self.monitoring:
                    self.monitoring.record_duration(
                        "secure_gitops_client_request_duration",
                        {"operation": "get_status"},
                        duration
                    )
                
                if response.status == 200:
                    data = await response.json()
                    
                    # Add security context
                    data["security_context"] = {
                        "authenticated": True,
                        "client_id": self.auth_context.client_id,
                        "access_time": datetime.utcnow().isoformat()
                    }
                    
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_requests",
                            {"operation": "get_status", "status": "success"}
                        )
                    
                    return data
                elif response.status == 404:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_requests",
                            {"operation": "get_status", "status": "not_found"}
                        )
                    raise SecurityError(f"GitOps request not found: {request_id}")
                else:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_requests",
                            {"operation": "get_status", "status": "failed"}
                        )
                    raise SecurityError(f"Failed to get status: {response.status}")
                    
        except SecurityError as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "secure_gitops_client_requests",
                    {"operation": "get_status", "status": "security_error"}
                )
            raise
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "secure_gitops_client_requests",
                    {"operation": "get_status", "status": "error"}
                )
            raise SecurityError(f"Failed to get secure GitOps status: {e}")
    
    async def notify_secure_completion(self, status_data: Dict[str, Any]) -> bool:
        """Notify integration bridge of GitOps completion with security"""
        try:
            # Validate authentication
            if not self.auth_context or not self.auth_context.authenticated:
                raise AuthenticationError("Authentication required for completion notification")
            
            webhook_request = {
                "id": f"secure-gitops-completion-{status_data.get('request_id')}",
                "type": "gitops_completed",
                "source": "secure-gitops-generator",
                "timestamp": datetime.utcnow().isoformat(),
                "data": status_data,
                "security_context": {
                    "authenticated": True,
                    "client_id": self.auth_context.client_id,
                    "auth_method": self.auth_context.auth_method,
                    "security_level": "high"
                }
            }
            
            # Sign the notification
            if self.security_manager.crypto_manager:
                signature = self.security_manager.crypto_manager.sign_request(
                    "POST", "/webhooks/gitops/completed", json.dumps(webhook_request)
                )
                webhook_request["signature"] = signature
            
            async with secure_http_request(
                self.session, "POST",
                urljoin(self.bridge_url, "/webhooks/gitops/completed"),
                self.security_manager,
                json=webhook_request
            ) as response:
                if response.status == 200:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_notifications",
                            {"type": "completion", "status": "success"}
                        )
                    
                    await self._log_security_event(
                        "completion_notification_sent",
                        "success",
                        f"Completion notification sent for request {status_data.get('request_id')}"
                    )
                    
                    return True
                else:
                    if self.monitoring:
                        self.monitoring.inc_counter(
                            "secure_gitops_client_notifications",
                            {"type": "completion", "status": "failed"}
                        )
                    
                    error_text = await response.text()
                    await self._log_security_event(
                        "completion_notification_failed",
                        "failed",
                        f"Completion notification failed: {response.status} - {error_text}"
                    )
                    
                    logger.error(f"Secure completion notification failed: {response.status}")
                    return False
                    
        except SecurityError as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "secure_gitops_client_notifications",
                    {"type": "completion", "status": "security_error"}
                )
            logger.error(f"Security error in completion notification: {e}")
            raise
        except Exception as e:
            if self.monitoring:
                self.monitoring.inc_counter(
                    "secure_gitops_client_notifications",
                    {"type": "completion", "status": "error"}
                )
            logger.error(f"Failed to send secure completion notification: {e}")
            return False
    
    async def get_secure_health_status(self) -> Dict[str, Any]:
        """Get health status with security context"""
        try:
            async with secure_http_request(
                self.session, "GET",
                urljoin(self.gitops_url, "/health"),
                self.security_manager
            ) as response:
                if response.status == 200:
                    health_data = await response.json()
                    
                    # Add security status
                    health_data["security"] = {
                        "authentication_enabled": True,
                        "tls_enabled": self.security_config.tls_enabled,
                        "audit_enabled": self.security_config.audit_enabled,
                        "client_authenticated": self.auth_context.authenticated if self.auth_context else False,
                        "security_score": await self._calculate_security_score()
                    }
                    
                    return health_data
                else:
                    raise SecurityError(f"Health check failed: {response.status}")
                    
        except Exception as e:
            raise SecurityError(f"Failed to get secure health status: {e}")
    
    async def _validate_gitops_request(self, request: SecureGitOpsRequest):
        """Validate GitOps request for security issues"""
        # Validate required fields
        if not request.request_id:
            raise ValidationError("request_id is required")
        if not request.backup_id:
            raise ValidationError("backup_id is required")
        if not request.cluster_name:
            raise ValidationError("cluster_name is required")
        if not request.source_path:
            raise ValidationError("source_path is required")
        
        # Validate data using security manager
        request_data = asdict(request)
        await self.security_manager.validate_request(
            "POST", "/api/gitops/generate", 
            {}, json.dumps(request_data).encode(), 
            self.auth_context.client_id if self.auth_context else "unknown"
        )
    
    async def _log_security_event(self, event_type: str, status: str, message: str):
        """Log security event"""
        if self.security_manager.audit_logger:
            event = SecurityEvent(
                event_type=event_type,
                timestamp=datetime.utcnow(),
                client_id=self.auth_context.client_id if self.auth_context else "unknown",
                source_ip="127.0.0.1",  # Client-side
                endpoint="gitops_client",
                method="CLIENT",
                status=status,
                message=message
            )
            await self.security_manager.audit_logger.log_event(event)
    
    async def _calculate_security_score(self) -> float:
        """Calculate security score for the client"""
        score = 0.0
        
        if self.auth_context and self.auth_context.authenticated:
            score += 30
        if self.security_config.tls_enabled:
            score += 25
        if self.security_config.audit_enabled:
            score += 20
        if self.security_manager.crypto_manager:
            score += 15
        if self.security_config.strict_mode:
            score += 10
        
        return score
    
    def create_secure_request_from_backup(self, backup_event: Dict[str, Any], 
                                        config_override: Optional[Dict[str, Any]] = None) -> SecureGitOpsRequest:
        """Create secure GitOps request from backup completion event"""
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
        
        # Create secure request
        request = SecureGitOpsRequest(
            request_id=f"secure-gitops-{backup_id}-{int(time.time())}",
            backup_id=backup_id,
            cluster_name=cluster_name,
            source_path=minio_path,
            target_repo=target_repo,
            target_branch=target_branch,
            configuration={
                "backup_timestamp": backup_event.get("timestamp"),
                "resource_count": backup_event.get("resource_count"),
                "size_bytes": backup_event.get("size"),
                "original_cluster": cluster_name,
                "security_enabled": True,
                "client_authenticated": self.auth_context.authenticated if self.auth_context else False
            },
            auth_context=self.auth_context
        )
        
        return request


# Secure webhook server for receiving backup completion events

class SecureWebhookServer:
    """Secure webhook server with Python security integration"""
    
    def __init__(self, config: Dict[str, Any], host: str = "0.0.0.0", port: int = 8443):
        self.config = config
        self.host = host
        self.port = port
        
        # Initialize security manager
        self.security_config = create_security_config_from_dict(config)
        self.security_manager = PythonSecurityManager(self.security_config)
        
        self.app = None
        self.runner = None
        
        logger.info(f"Secure webhook server initialized on {host}:{port}")
    
    async def start(self):
        """Start the secure webhook server"""
        from aiohttp import web, web_runner
        
        app = web.Application(middlewares=[self._security_middleware])
        
        # Add routes
        app.router.add_get('/health', self._handle_health)
        app.router.add_post('/webhook/backup-complete', self._handle_backup_complete)
        app.router.add_get('/security/status', self._handle_security_status)
        
        # Configure SSL context
        ssl_context = None
        if self.security_manager.tls_context:
            ssl_context = self.security_manager.tls_context
        
        runner = web_runner.AppRunner(app)
        await runner.setup()
        
        site = web_runner.TCPSite(
            runner, 
            self.host, 
            self.port,
            ssl_context=ssl_context
        )
        await site.start()
        
        self.app = app
        self.runner = runner
        
        logger.info(f"Secure webhook server started on {'https' if ssl_context else 'http'}://{self.host}:{self.port}")
    
    async def stop(self):
        """Stop the secure webhook server"""
        if self.runner:
            await self.runner.cleanup()
        logger.info("Secure webhook server stopped")
    
    async def _security_middleware(self, request, handler):
        """Security middleware for request validation"""
        try:
            # Extract request information
            headers = dict(request.headers)
            client_ip = request.remote
            method = request.method
            path = request.path
            
            # Read request body
            body = None
            if request.can_read_body:
                body = await request.read()
            
            # Authenticate request
            auth_context = await self.security_manager.authenticate_request(headers, client_ip)
            
            # Validate request
            validated_request = await self.security_manager.validate_request(
                method, path, headers, body, client_ip
            )
            
            # Add security context to request
            request["auth_context"] = auth_context
            request["validated_request"] = validated_request
            
            # Process request
            response = await handler(request)
            
            return response
            
        except SecurityError as e:
            return web.json_response(
                {
                    "error": e.message,
                    "error_type": e.error_type,
                    "timestamp": datetime.utcnow().isoformat()
                },
                status=e.status_code
            )
    
    async def _handle_health(self, request):
        """Handle health check requests"""
        health_status = {
            "status": "healthy",
            "timestamp": datetime.utcnow().isoformat(),
            "security": {
                "authentication_enabled": True,
                "tls_enabled": self.security_config.tls_enabled,
                "audit_enabled": self.security_config.audit_enabled
            }
        }
        
        return web.json_response(health_status)
    
    async def _handle_backup_complete(self, request):
        """Handle secure backup completion webhooks"""
        auth_context = request.get("auth_context")
        validated_request = request.get("validated_request")
        
        if not auth_context or not auth_context.authenticated:
            raise AuthenticationError("Authentication required")
        
        # Authorize the request
        self.security_manager.authorize_request(auth_context, "webhook_backup")
        
        # Process backup completion
        backup_data = validated_request["body"]
        
        # Log security event
        if self.security_manager.audit_logger:
            await self.security_manager.audit_logger.log_event(SecurityEvent(
                event_type="backup_webhook_received",
                timestamp=datetime.utcnow(),
                client_id=auth_context.client_id,
                source_ip=validated_request["source_ip"],
                endpoint=validated_request["endpoint"],
                method=validated_request["method"],
                status="success",
                message=f"Backup completion webhook processed for {backup_data.get('backup_id')}"
            ))
        
        response_data = {
            "success": True,
            "message": "Backup completion processed securely",
            "backup_id": backup_data.get("backup_id"),
            "timestamp": datetime.utcnow().isoformat(),
            "security_context": {
                "authenticated": True,
                "client_id": auth_context.client_id
            }
        }
        
        return web.json_response(response_data)
    
    async def _handle_security_status(self, request):
        """Handle security status requests"""
        auth_context = request.get("auth_context")
        
        if not auth_context or not auth_context.authenticated:
            raise AuthenticationError("Authentication required")
        
        # Authorize the request
        self.security_manager.authorize_request(auth_context, "security_status")
        
        status = {
            "security_enabled": True,
            "authentication_active": True,
            "tls_enabled": self.security_config.tls_enabled,
            "audit_enabled": self.security_config.audit_enabled,
            "rate_limiting_active": True,
            "timestamp": datetime.utcnow().isoformat(),
            "client_context": {
                "client_id": auth_context.client_id,
                "auth_method": auth_context.auth_method,
                "permissions": auth_context.permissions
            }
        }
        
        return web.json_response(status)


# Convenience functions for secure operations

async def process_secure_backup_completion(config: Dict[str, Any], backup_event: Dict[str, Any], 
                                         monitoring_client=None) -> Dict[str, Any]:
    """Process backup completion securely and generate GitOps artifacts"""
    async with SecureGitOpsClient(config, monitoring_client) as client:
        # Create secure GitOps request from backup event
        request = client.create_secure_request_from_backup(backup_event)
        
        # Start secure GitOps generation
        response = await client.start_secure_gitops_generation(request)
        logger.info(f"Started secure GitOps generation: {response.request_id}")
        
        # Get final status (would implement polling in production)
        status = await client.get_secure_gitops_status(request.request_id)
        
        # Notify bridge of completion
        await client.notify_secure_completion(status)
        
        return status


async def get_secure_integration_status(config: Dict[str, Any]) -> Dict[str, Any]:
    """Get overall secure integration status"""
    async with SecureGitOpsClient(config) as client:
        try:
            gitops_health = await client.get_secure_health_status()
            
            # Try to get bridge status
            try:
                async with secure_http_request(
                    client.session, "GET",
                    urljoin(client.bridge_url, "/status"),
                    client.security_manager
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
                "security": {
                    "authentication_enabled": True,
                    "client_authenticated": client.auth_context.authenticated if client.auth_context else False,
                    "security_score": await client._calculate_security_score()
                },
                "timestamp": datetime.utcnow().isoformat()
            }
            
        except Exception as e:
            return {
                "error": str(e),
                "timestamp": datetime.utcnow().isoformat()
            }


if __name__ == "__main__":
    # Example usage
    async def main():
        # Load secure configuration
        config = {
            "security": {
                "enabled": True,
                "strict_mode": True,
                "api_key": "secure-api-key-123",
                "tls": {"enabled": True, "verify": True},
                "rate_limit": {"requests": 100, "window": 60},
                "audit": {"enabled": True}
            },
            "integration": {
                "communication": {
                    "endpoints": {
                        "gitops_generator": "https://localhost:8443",
                        "integration_bridge": "https://localhost:8443"
                    }
                }
            },
            "gitops": {
                "repository": "https://github.com/org/gitops-repo",
                "branch": "main"
            }
        }
        
        # Test secure integration
        status = await get_secure_integration_status(config)
        print(json.dumps(status, indent=2))
        
        # Test secure backup processing
        backup_event = {
            "backup_id": "test-backup-123",
            "cluster_name": "test-cluster",
            "minio_path": "test-cluster/2024/01/15/test-backup-123",
            "timestamp": datetime.utcnow().isoformat(),
            "resource_count": 10,
            "size": 1024000
        }
        
        result = await process_secure_backup_completion(config, backup_event)
        print(f"Secure processing result: {result}")
    
    asyncio.run(main())