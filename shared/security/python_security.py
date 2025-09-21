#!/usr/bin/env python3
"""
Python Security Integration Module

Provides security features for Python components in the backup-to-GitOps system,
including authentication, input validation, TLS configuration, and audit logging.
"""

import asyncio
import hashlib
import hmac
import json
import logging
import ssl
import time
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any, Callable, Union
from urllib.parse import urlparse
import re

import aiohttp
import cryptography
from cryptography.fernet import Fernet
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa, padding
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC

logger = logging.getLogger(__name__)


@dataclass
class SecurityConfig:
    """Security configuration for Python components"""
    enabled: bool = True
    strict_mode: bool = True
    api_key: Optional[str] = None
    secret_key: Optional[str] = None
    tls_enabled: bool = True
    tls_verify: bool = True
    tls_cert_path: Optional[str] = None
    tls_key_path: Optional[str] = None
    tls_ca_path: Optional[str] = None
    rate_limit_requests: int = 100
    rate_limit_window: int = 60
    audit_enabled: bool = True
    audit_log_path: str = "/var/log/backup-gitops/python-security.log"
    max_request_size: int = 1024 * 1024  # 1MB
    request_timeout: int = 30
    session_timeout: int = 1800  # 30 minutes


@dataclass
class AuthContext:
    """Authentication context for requests"""
    authenticated: bool = False
    client_id: str = ""
    auth_method: str = ""
    permissions: List[str] = None
    session_id: str = ""
    expires_at: Optional[datetime] = None
    metadata: Dict[str, Any] = None

    def __post_init__(self):
        if self.permissions is None:
            self.permissions = []
        if self.metadata is None:
            self.metadata = {}


@dataclass
class SecurityEvent:
    """Security event for audit logging"""
    event_type: str
    timestamp: datetime
    client_id: str
    source_ip: str
    endpoint: str
    method: str
    status: str
    message: str
    metadata: Dict[str, Any] = None

    def __post_init__(self):
        if self.metadata is None:
            self.metadata = {}


class SecurityError(Exception):
    """Base security exception"""
    def __init__(self, message: str, error_type: str = "security_error", status_code: int = 403):
        super().__init__(message)
        self.message = message
        self.error_type = error_type
        self.status_code = status_code


class AuthenticationError(SecurityError):
    """Authentication failure exception"""
    def __init__(self, message: str = "Authentication failed"):
        super().__init__(message, "authentication_error", 401)


class AuthorizationError(SecurityError):
    """Authorization failure exception"""
    def __init__(self, message: str = "Insufficient permissions"):
        super().__init__(message, "authorization_error", 403)


class ValidationError(SecurityError):
    """Input validation exception"""
    def __init__(self, message: str = "Validation failed"):
        super().__init__(message, "validation_error", 400)


class RateLimitError(SecurityError):
    """Rate limit exceeded exception"""
    def __init__(self, message: str = "Rate limit exceeded"):
        super().__init__(message, "rate_limit_error", 429)


class PythonSecurityManager:
    """Main security manager for Python components"""
    
    def __init__(self, config: SecurityConfig):
        self.config = config
        self.rate_limiter = RateLimiter(
            max_requests=config.rate_limit_requests,
            window_seconds=config.rate_limit_window
        )
        self.audit_logger = AuditLogger(config.audit_log_path) if config.audit_enabled else None
        self.input_validator = InputValidator()
        self.crypto_manager = CryptoManager(config.secret_key) if config.secret_key else None
        self.tls_context = self._create_tls_context() if config.tls_enabled else None
        
        logger.info("Python security manager initialized", extra={
            "strict_mode": config.strict_mode,
            "tls_enabled": config.tls_enabled,
            "audit_enabled": config.audit_enabled
        })
    
    def _create_tls_context(self) -> ssl.SSLContext:
        """Create SSL context for secure connections"""
        context = ssl.create_default_context()
        
        if self.config.tls_verify:
            context.check_hostname = True
            context.verify_mode = ssl.CERT_REQUIRED
        else:
            context.check_hostname = False
            context.verify_mode = ssl.CERT_NONE
            logger.warning("TLS verification disabled - not recommended for production")
        
        # Load custom certificates if provided
        if self.config.tls_cert_path and self.config.tls_key_path:
            context.load_cert_chain(self.config.tls_cert_path, self.config.tls_key_path)
        
        if self.config.tls_ca_path:
            context.load_verify_locations(self.config.tls_ca_path)
        
        # Set minimum TLS version
        context.minimum_version = ssl.TLSVersion.TLSv1_2
        
        return context
    
    async def authenticate_request(self, headers: Dict[str, str], source_ip: str = "") -> AuthContext:
        """Authenticate incoming request"""
        auth_context = AuthContext()
        
        try:
            # Check for API key authentication
            api_key = headers.get("X-API-Key") or headers.get("x-api-key")
            if api_key:
                if self.config.api_key and api_key == self.config.api_key:
                    auth_context.authenticated = True
                    auth_context.client_id = f"api-key-{api_key[:8]}"
                    auth_context.auth_method = "api_key"
                    auth_context.permissions = ["webhook_access", "status_read"]
                    auth_context.expires_at = datetime.utcnow() + timedelta(seconds=self.config.session_timeout)
                else:
                    raise AuthenticationError("Invalid API key")
            
            # Check for Bearer token authentication
            elif "Authorization" in headers:
                auth_header = headers["Authorization"]
                if auth_header.startswith("Bearer "):
                    token = auth_header[7:]  # Remove "Bearer " prefix
                    auth_context = await self._validate_bearer_token(token)
                elif auth_header.startswith("Basic "):
                    credentials = auth_header[6:]  # Remove "Basic " prefix
                    auth_context = await self._validate_basic_auth(credentials)
                else:
                    raise AuthenticationError("Unsupported authentication method")
            
            else:
                if self.config.strict_mode:
                    raise AuthenticationError("Authentication required")
                else:
                    # Allow unauthenticated access with limited permissions
                    auth_context.authenticated = False
                    auth_context.client_id = f"anonymous-{source_ip}"
                    auth_context.auth_method = "none"
                    auth_context.permissions = ["health_check"]
            
            # Log successful authentication
            if self.audit_logger:
                await self.audit_logger.log_event(SecurityEvent(
                    event_type="authentication",
                    timestamp=datetime.utcnow(),
                    client_id=auth_context.client_id,
                    source_ip=source_ip,
                    endpoint="auth",
                    method="AUTH",
                    status="success",
                    message=f"Authentication successful using {auth_context.auth_method}"
                ))
            
            return auth_context
            
        except SecurityError as e:
            # Log failed authentication
            if self.audit_logger:
                await self.audit_logger.log_event(SecurityEvent(
                    event_type="authentication_failed",
                    timestamp=datetime.utcnow(),
                    client_id="unknown",
                    source_ip=source_ip,
                    endpoint="auth",
                    method="AUTH",
                    status="failed",
                    message=str(e)
                ))
            raise
    
    async def _validate_bearer_token(self, token: str) -> AuthContext:
        """Validate Bearer token"""
        # In production, this would validate against a token store or JWT
        # For now, implement basic token validation
        if len(token) < 32:
            raise AuthenticationError("Invalid token format")
        
        auth_context = AuthContext(
            authenticated=True,
            client_id=f"token-{token[:8]}",
            auth_method="bearer_token",
            permissions=["webhook_access", "status_read"],
            session_id=token,
            expires_at=datetime.utcnow() + timedelta(seconds=self.config.session_timeout)
        )
        
        return auth_context
    
    async def _validate_basic_auth(self, credentials: str) -> AuthContext:
        """Validate Basic authentication"""
        try:
            import base64
            decoded = base64.b64decode(credentials).decode('utf-8')
            username, password = decoded.split(':', 1)
            
            # In production, validate against user store
            # For now, implement basic validation
            if username and password and len(password) >= 8:
                auth_context = AuthContext(
                    authenticated=True,
                    client_id=f"user-{username}",
                    auth_method="basic_auth",
                    permissions=["webhook_access", "status_read"],
                    expires_at=datetime.utcnow() + timedelta(seconds=self.config.session_timeout)
                )
                return auth_context
            else:
                raise AuthenticationError("Invalid credentials")
                
        except Exception:
            raise AuthenticationError("Invalid Basic auth format")
    
    def authorize_request(self, auth_context: AuthContext, required_permission: str) -> bool:
        """Check if authenticated user has required permission"""
        if not auth_context.authenticated and self.config.strict_mode:
            raise AuthorizationError("Authentication required")
        
        if required_permission in auth_context.permissions or "admin" in auth_context.permissions:
            return True
        
        raise AuthorizationError(f"Permission '{required_permission}' required")
    
    async def validate_request(self, method: str, endpoint: str, headers: Dict[str, str], 
                             body: Optional[bytes] = None, source_ip: str = "") -> Dict[str, Any]:
        """Validate incoming request"""
        # Rate limiting
        if not self.rate_limiter.allow(source_ip):
            raise RateLimitError("Rate limit exceeded")
        
        # Size validation
        if body and len(body) > self.config.max_request_size:
            raise ValidationError("Request body too large")
        
        # Method validation
        allowed_methods = ["GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS"]
        if method not in allowed_methods:
            raise ValidationError("Invalid HTTP method")
        
        # Content type validation for POST/PUT
        if method in ["POST", "PUT"] and body:
            content_type = headers.get("Content-Type", "")
            if not content_type.startswith("application/json"):
                raise ValidationError("Invalid content type")
        
        # Parse and validate JSON body
        parsed_body = None
        if body and len(body) > 0:
            try:
                parsed_body = json.loads(body.decode('utf-8'))
                # Additional JSON validation
                await self.input_validator.validate_json(parsed_body)
            except json.JSONDecodeError:
                raise ValidationError("Invalid JSON format")
            except Exception as e:
                raise ValidationError(f"JSON validation failed: {str(e)}")
        
        return {
            "method": method,
            "endpoint": endpoint,
            "headers": headers,
            "body": parsed_body,
            "source_ip": source_ip,
            "validated_at": datetime.utcnow()
        }
    
    async def create_secure_http_session(self) -> aiohttp.ClientSession:
        """Create HTTP session with security configuration"""
        timeout = aiohttp.ClientTimeout(total=self.config.request_timeout)
        
        connector = aiohttp.TCPConnector(
            ssl=self.tls_context,
            limit=100,
            limit_per_host=30
        )
        
        headers = {
            "User-Agent": "GitOps-Security-Client/1.0",
            "Accept": "application/json",
            "Content-Type": "application/json"
        }
        
        if self.config.api_key:
            headers["X-API-Key"] = self.config.api_key
        
        session = aiohttp.ClientSession(
            connector=connector,
            timeout=timeout,
            headers=headers
        )
        
        return session


class RateLimiter:
    """Token bucket rate limiter"""
    
    def __init__(self, max_requests: int, window_seconds: int):
        self.max_requests = max_requests
        self.window_seconds = window_seconds
        self.clients = {}
        self.cleanup_interval = 300  # 5 minutes
        self.last_cleanup = time.time()
    
    def allow(self, client_id: str) -> bool:
        """Check if request is allowed for client"""
        now = time.time()
        
        # Cleanup old entries
        if now - self.last_cleanup > self.cleanup_interval:
            self._cleanup(now)
        
        # Get or create client record
        if client_id not in self.clients:
            self.clients[client_id] = {
                "requests": [],
                "last_request": now
            }
        
        client = self.clients[client_id]
        
        # Remove requests outside the window
        window_start = now - self.window_seconds
        client["requests"] = [req_time for req_time in client["requests"] if req_time > window_start]
        
        # Check if limit exceeded
        if len(client["requests"]) >= self.max_requests:
            return False
        
        # Add current request
        client["requests"].append(now)
        client["last_request"] = now
        
        return True
    
    def _cleanup(self, now: float):
        """Remove old client records"""
        cutoff = now - (self.window_seconds * 2)
        
        to_remove = []
        for client_id, client in self.clients.items():
            if client["last_request"] < cutoff:
                to_remove.append(client_id)
        
        for client_id in to_remove:
            del self.clients[client_id]
        
        self.last_cleanup = now


class InputValidator:
    """Input validation and sanitization"""
    
    def __init__(self):
        # Common attack patterns
        self.sql_injection_patterns = [
            r"(?i)(union\s+select|drop\s+table|insert\s+into|delete\s+from)",
            r"(?i)(exec\s*\(|script\s*:|javascript\s*:)",
            r"['\"];?\s*(drop|alter|create|delete|insert|update)\s+",
        ]
        
        self.xss_patterns = [
            r"<script[^>]*>.*?</script>",
            r"(?i)on\w+\s*=",
            r"(?i)javascript\s*:",
            r"(?i)vbscript\s*:",
        ]
        
        self.path_traversal_patterns = [
            r"\.\./",
            r"\.\.\\",
            r"%2e%2e%2f",
            r"%2e%2e%5c",
        ]
    
    async def validate_json(self, data: Any) -> None:
        """Validate JSON data for security issues"""
        if isinstance(data, dict):
            for key, value in data.items():
                await self._validate_string(str(key))
                if isinstance(value, str):
                    await self._validate_string(value)
                elif isinstance(value, (dict, list)):
                    await self.validate_json(value)
        
        elif isinstance(data, list):
            for item in data:
                if isinstance(item, str):
                    await self._validate_string(item)
                elif isinstance(item, (dict, list)):
                    await self.validate_json(item)
        
        elif isinstance(data, str):
            await self._validate_string(data)
    
    async def _validate_string(self, value: str) -> None:
        """Validate string for malicious patterns"""
        # Check for SQL injection
        for pattern in self.sql_injection_patterns:
            if re.search(pattern, value):
                raise ValidationError("Potential SQL injection detected")
        
        # Check for XSS
        for pattern in self.xss_patterns:
            if re.search(pattern, value):
                raise ValidationError("Potential XSS attack detected")
        
        # Check for path traversal
        for pattern in self.path_traversal_patterns:
            if re.search(pattern, value):
                raise ValidationError("Potential path traversal detected")
        
        # Check for excessively long strings
        if len(value) > 10000:
            raise ValidationError("String too long")


class AuditLogger:
    """Security audit logging"""
    
    def __init__(self, log_path: str):
        self.log_path = log_path
        self.logger = logging.getLogger("security_audit")
        self.logger.setLevel(logging.INFO)
        
        # Create file handler
        handler = logging.FileHandler(log_path)
        formatter = logging.Formatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
        )
        handler.setFormatter(formatter)
        self.logger.addHandler(handler)
    
    async def log_event(self, event: SecurityEvent):
        """Log security event"""
        event_data = asdict(event)
        event_data["timestamp"] = event.timestamp.isoformat()
        
        self.logger.info(json.dumps(event_data))


class CryptoManager:
    """Cryptographic operations"""
    
    def __init__(self, secret_key: str):
        self.secret_key = secret_key.encode() if isinstance(secret_key, str) else secret_key
        self.fernet = Fernet(Fernet.generate_key())  # In production, derive from secret_key
    
    def encrypt_data(self, data: str) -> str:
        """Encrypt sensitive data"""
        return self.fernet.encrypt(data.encode()).decode()
    
    def decrypt_data(self, encrypted_data: str) -> str:
        """Decrypt sensitive data"""
        return self.fernet.decrypt(encrypted_data.encode()).decode()
    
    def sign_request(self, method: str, url: str, body: str = "") -> str:
        """Sign request for integrity verification"""
        message = f"{method}|{url}|{body}"
        signature = hmac.new(
            self.secret_key,
            message.encode(),
            hashlib.sha256
        ).hexdigest()
        return signature
    
    def verify_signature(self, method: str, url: str, body: str, signature: str) -> bool:
        """Verify request signature"""
        expected_signature = self.sign_request(method, url, body)
        return hmac.compare_digest(signature, expected_signature)


# Security middleware for web frameworks

class SecurityMiddleware:
    """ASGI/WSGI security middleware"""
    
    def __init__(self, security_manager: PythonSecurityManager):
        self.security_manager = security_manager
    
    async def __call__(self, scope, receive, send):
        """ASGI middleware implementation"""
        if scope["type"] != "http":
            return await self.app(scope, receive, send)
        
        # Extract request information
        method = scope["method"]
        path = scope["path"]
        headers = dict(scope["headers"])
        client_ip = scope.get("client", ["unknown", None])[0]
        
        # Convert headers to strings
        str_headers = {}
        for key, value in headers.items():
            if isinstance(key, bytes):
                key = key.decode()
            if isinstance(value, bytes):
                value = value.decode()
            str_headers[key] = value
        
        try:
            # Authenticate request
            auth_context = await self.security_manager.authenticate_request(str_headers, client_ip)
            
            # Read request body
            body = b""
            message = await receive()
            while message.get("more_body", False):
                body += message.get("body", b"")
                message = await receive()
            body += message.get("body", b"")
            
            # Validate request
            validated_request = await self.security_manager.validate_request(
                method, path, str_headers, body, client_ip
            )
            
            # Add security context to scope
            scope["auth_context"] = auth_context
            scope["validated_request"] = validated_request
            
            # Continue with the application
            await self.app(scope, receive, send)
            
        except SecurityError as e:
            # Send security error response
            response = {
                "error": e.message,
                "error_type": e.error_type,
                "timestamp": datetime.utcnow().isoformat()
            }
            
            await send({
                "type": "http.response.start",
                "status": e.status_code,
                "headers": [
                    [b"content-type", b"application/json"],
                    [b"x-content-type-options", b"nosniff"],
                    [b"x-frame-options", b"DENY"],
                ]
            })
            
            await send({
                "type": "http.response.body",
                "body": json.dumps(response).encode()
            })


# Utility functions

def create_security_config_from_dict(config_dict: Dict[str, Any]) -> SecurityConfig:
    """Create SecurityConfig from dictionary"""
    security_data = config_dict.get("security", {})
    
    return SecurityConfig(
        enabled=security_data.get("enabled", True),
        strict_mode=security_data.get("strict_mode", True),
        api_key=security_data.get("api_key"),
        secret_key=security_data.get("secret_key"),
        tls_enabled=security_data.get("tls", {}).get("enabled", True),
        tls_verify=security_data.get("tls", {}).get("verify", True),
        tls_cert_path=security_data.get("tls", {}).get("cert_path"),
        tls_key_path=security_data.get("tls", {}).get("key_path"),
        tls_ca_path=security_data.get("tls", {}).get("ca_path"),
        rate_limit_requests=security_data.get("rate_limit", {}).get("requests", 100),
        rate_limit_window=security_data.get("rate_limit", {}).get("window", 60),
        audit_enabled=security_data.get("audit", {}).get("enabled", True),
        audit_log_path=security_data.get("audit", {}).get("log_path", "/var/log/backup-gitops/python-security.log"),
        max_request_size=security_data.get("limits", {}).get("max_request_size", 1024 * 1024),
        request_timeout=security_data.get("limits", {}).get("request_timeout", 30),
        session_timeout=security_data.get("session", {}).get("timeout", 1800)
    )


async def secure_http_request(session: aiohttp.ClientSession, method: str, url: str, 
                            security_manager: PythonSecurityManager,
                            **kwargs) -> aiohttp.ClientResponse:
    """Make secure HTTP request with signature"""
    if security_manager.crypto_manager:
        # Add request signature
        body = kwargs.get("json", "")
        if body:
            body = json.dumps(body)
        
        signature = security_manager.crypto_manager.sign_request(method, url, body)
        
        if "headers" not in kwargs:
            kwargs["headers"] = {}
        kwargs["headers"]["X-Request-Signature"] = signature
    
    return await session.request(method, url, **kwargs)


# Example usage and integration helpers

class SecureGitOpsClient:
    """Example of secure GitOps client using the security framework"""
    
    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.security_config = create_security_config_from_dict(config)
        self.security_manager = PythonSecurityManager(self.security_config)
        self.session = None
    
    async def __aenter__(self):
        self.session = await self.security_manager.create_secure_http_session()
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self.session:
            await self.session.close()
    
    async def send_secure_webhook(self, url: str, data: Dict[str, Any]) -> Dict[str, Any]:
        """Send authenticated webhook request"""
        try:
            async with secure_http_request(
                self.session, "POST", url, self.security_manager, json=data
            ) as response:
                if response.status == 200:
                    return await response.json()
                else:
                    raise SecurityError(f"Webhook failed: {response.status}")
        
        except Exception as e:
            logger.error(f"Secure webhook failed: {e}")
            raise


if __name__ == "__main__":
    # Example usage
    async def main():
        config = {
            "security": {
                "enabled": True,
                "strict_mode": True,
                "api_key": "test-api-key-123",
                "tls": {"enabled": True, "verify": True},
                "rate_limit": {"requests": 100, "window": 60},
                "audit": {"enabled": True}
            }
        }
        
        security_manager = PythonSecurityManager(create_security_config_from_dict(config))
        
        # Test authentication
        headers = {"X-API-Key": "test-api-key-123"}
        auth_context = await security_manager.authenticate_request(headers, "127.0.0.1")
        print(f"Authentication successful: {auth_context.authenticated}")
        
        # Test validation
        test_body = json.dumps({"test": "data"}).encode()
        validated = await security_manager.validate_request(
            "POST", "/test", headers, test_body, "127.0.0.1"
        )
        print(f"Validation successful: {validated['validated_at']}")
    
    asyncio.run(main())