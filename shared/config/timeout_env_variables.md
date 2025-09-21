# Configurable Timeout and Retry Environment Variables

This document lists all environment variables that can be used to configure timeout and retry behavior throughout the system.

## HTTP Server Timeouts

- `HTTP_READ_TIMEOUT` - HTTP server read timeout (default: 30s)
- `HTTP_WRITE_TIMEOUT` - HTTP server write timeout (default: 30s)  
- `HTTP_IDLE_TIMEOUT` - HTTP server idle timeout (default: 60s)
- `HTTP_SHUTDOWN_TIMEOUT` - HTTP server graceful shutdown timeout (default: 30s)

## Restore Operation Timeouts

- `RESTORE_OPERATION_TIMEOUT` - Overall restore operation timeout (default: 30m)
- `RESTORE_VALIDATION_TIMEOUT` - Pre-restore validation timeout (default: 5m)
- `RESTORE_RESOURCE_TIMEOUT` - Individual resource restore timeout (default: 2m)

## Component Health Check Intervals

- `HEALTH_CHECK_INTERVAL` - Component health check interval (default: 30s)
- `MONITORING_INTERVAL` - General monitoring interval (default: 15s)
- `METRICS_COLLECTION_INTERVAL` - Metrics collection interval (default: 60s)

## Event Handling Timeouts

- `EVENT_HANDLER_TIMEOUT` - Event handler execution timeout (default: 30s)
- `EVENT_BUS_TIMEOUT` - Event bus operation timeout (default: 10s)

## Backup Client Timeouts

- `BACKUP_CLIENT_TIMEOUT` - HTTP client timeout for backup operations (default: 30s)
- `BACKUP_POLLING_INTERVAL` - Backup status polling interval (default: 5s)

## GitOps Operation Timeouts

- `GITOPS_CLONE_TIMEOUT` - Git clone operation timeout (default: 10m)
- `GITOPS_SYNC_TIMEOUT` - GitOps sync operation timeout (default: 15m)
- `GITOPS_COMMIT_TIMEOUT` - Git commit operation timeout (default: 2m)

## Security Operation Timeouts

- `SECURITY_VALIDATION_TIMEOUT` - Security validation timeout (default: 30s)
- `PERMISSION_CHECK_TIMEOUT` - Permission check timeout (default: 10s)

## General Retry Settings

- `MAX_RETRIES` - Maximum retry attempts (default: 3)
- `BASE_RETRY_DELAY` - Base delay between retries (default: 1s)
- `MAX_RETRY_DELAY` - Maximum delay between retries (default: 30s)
- `RETRY_MULTIPLIER` - Exponential backoff multiplier (default: 2.0)

## Operation-Specific Retries

- `RESTORE_MAX_RETRIES` - Maximum retries for restore operations (default: 5)
- `RESTORE_RETRY_DELAY` - Delay between restore retries (default: 10s)
- `VALIDATION_MAX_RETRIES` - Maximum retries for validation (default: 3)
- `VALIDATION_RETRY_DELAY` - Delay between validation retries (default: 5s)
- `GITOPS_MAX_RETRIES` - Maximum retries for GitOps operations (default: 3)
- `GITOPS_RETRY_DELAY` - Delay between GitOps retries (default: 15s)
- `SECURITY_MAX_RETRIES` - Maximum retries for security checks (default: 2)
- `SECURITY_RETRY_DELAY` - Delay between security check retries (default: 3s)

## Circuit Breaker Settings

- `CIRCUIT_BREAKER_THRESHOLD` - Failure threshold to trigger circuit breaker (default: 5)
- `CIRCUIT_BREAKER_TIMEOUT` - Circuit breaker timeout (default: 60s)
- `CIRCUIT_BREAKER_RECOVERY_TIME` - Time before attempting recovery (default: 300s)

## Legacy Compatibility Fields

These fields are maintained for backward compatibility:

- `MAX_ATTEMPTS` - Maps to `MAX_RETRIES`
- `INITIAL_DELAY` - Maps to `BASE_RETRY_DELAY`
- `MAX_DELAY` - Maps to `MAX_RETRY_DELAY`
- `MULTIPLIER` - Maps to `RETRY_MULTIPLIER`

## Examples

```bash
# Set HTTP server timeouts to 45 seconds
export HTTP_READ_TIMEOUT=45s
export HTTP_WRITE_TIMEOUT=45s

# Set backup polling to check every 10 seconds
export BACKUP_POLLING_INTERVAL=10s

# Set restore operations to retry up to 10 times with 15 second delays
export RESTORE_MAX_RETRIES=10
export RESTORE_RETRY_DELAY=15s

# Set circuit breaker to trip after 3 failures
export CIRCUIT_BREAKER_THRESHOLD=3
```

## Duration Format

All duration values accept Go duration format strings:
- `ns` (nanoseconds)
- `us` or `µs` (microseconds)  
- `ms` (milliseconds)
- `s` (seconds)
- `m` (minutes)
- `h` (hours)

Examples: `30s`, `5m`, `2h30m`, `1500ms`