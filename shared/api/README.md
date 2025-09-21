# Kubernetes Backup & Restore API Documentation

This directory contains the OpenAPI 3.0 specification for the Kubernetes Backup & Restore System REST API.

## 📄 Files

- **`openapi.yaml`** - Complete OpenAPI 3.0 specification with all endpoints, schemas, and examples
- **`README.md`** - This documentation file

## 🚀 Quick Start

### Viewing the API Documentation

You can view the interactive API documentation using several tools:

#### Swagger UI (Local)
```bash
# Using Docker
docker run -p 8081:8080 -e SWAGGER_JSON=/openapi.yaml -v $(pwd)/openapi.yaml:/openapi.yaml swaggerapi/swagger-ui

# Access at: http://localhost:8081
```

#### Swagger Editor (Online)
1. Go to [editor.swagger.io](https://editor.swagger.io/)
2. Copy and paste the contents of `openapi.yaml`
3. View the interactive documentation and generate client SDKs

#### Redoc (Alternative)
```bash
# Using npx
npx redoc-cli serve openapi.yaml

# Using Docker
docker run -p 8080:80 -v $(pwd)/openapi.yaml:/usr/share/nginx/html/openapi.yaml redocly/redoc
```

### Code Generation

Generate client SDKs in various languages:

```bash
# Install OpenAPI Generator
npm install @openapitools/openapi-generator-cli -g

# Generate Python client
openapi-generator-cli generate -i openapi.yaml -g python -o ./clients/python

# Generate Go client
openapi-generator-cli generate -i openapi.yaml -g go -o ./clients/go

# Generate JavaScript/TypeScript client
openapi-generator-cli generate -i openapi.yaml -g typescript-axios -o ./clients/typescript
```

## 🏗️ API Overview

### Base URL
- **Development**: `http://localhost:8080`
- **Production**: `https://api.backup-system.example.com`

### Authentication

The API supports multiple authentication methods:

1. **API Key** (Recommended for services)
   ```bash
   curl -H "X-API-Key: your-api-key" https://api.example.com/health
   ```

2. **Bearer Token** (JWT)
   ```bash
   curl -H "Authorization: Bearer your-jwt-token" https://api.example.com/health
   ```

3. **Basic Auth**
   ```bash
   curl -u username:password https://api.example.com/health
   ```

### Rate Limiting

API endpoints are rate-limited to prevent abuse:
- **Standard endpoints**: 100 requests per minute
- **Health endpoints**: 300 requests per minute
- **Resource-intensive operations**: 10 requests per minute

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
```

## 🔧 API Endpoints Overview

### Core Endpoint Categories

| Category | Base Path | Description |
|----------|-----------|-------------|
| **Health** | `/health`, `/status`, `/metrics` | System health and monitoring |
| **Restore** | `/api/v1/restore` | Backup restore operations |
| **Disaster Recovery** | `/api/v1/dr` | DR scenario management |
| **Backups** | `/api/v1/backups` | Backup information and validation |
| **Clusters** | `/api/v1/clusters` | Kubernetes cluster management |
| **Integration** | `/api/v1/integration` | Event bus and bridge management |
| **Webhooks** | `/webhook/*`, `/register/*` | Webhook processing and registration |

### Key Operations

#### Starting a Restore Operation
```bash
curl -X POST https://api.example.com/api/v1/restore \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "backup_id": "550e8400-e29b-41d4-a716-446655440000",
    "cluster_name": "production-cluster",
    "restore_mode": "complete",
    "validation_mode": "strict",
    "conflict_strategy": "merge"
  }'
```

#### Checking Restore Status
```bash
curl https://api.example.com/api/v1/restore/{restoreId} \
  -H "X-API-Key: your-api-key"
```

#### Listing Available Backups
```bash
curl "https://api.example.com/api/v1/backups?cluster_name=production-cluster&limit=10" \
  -H "X-API-Key: your-api-key"
```

#### Health Check
```bash
curl https://api.example.com/health \
  -H "X-API-Key: your-api-key"
```

## 📊 Response Format

All API responses follow a consistent format:

### Success Response
```json
{
  "success": true,
  "data": {
    // Response data here
  },
  "message": "Operation completed successfully",
  "timestamp": "2023-12-01T10:30:00Z",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Error Response
```json
{
  "success": false,
  "error": {
    "code": "validation_error",
    "message": "Request validation failed",
    "details": {
      "field": "backup_id",
      "issue": "Invalid UUID format"
    }
  },
  "timestamp": "2023-12-01T10:30:00Z",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

## 🔍 Key Data Models

### RestoreRequest
Main request object for starting restore operations:
```yaml
backup_id: string (UUID, required)
cluster_name: string (required)
restore_mode: enum [complete, selective, incremental, validation]
validation_mode: enum [strict, permissive, skip]
conflict_strategy: enum [skip, overwrite, merge, fail]
target_namespaces: array of strings (optional)
resource_types: array of strings (optional)
dry_run: boolean (default: false)
```

### RestoreOperation
Complete restore operation status and progress:
```yaml
request: RestoreRequest
status: enum [pending, validating, restoring, completed, failed, cancelled]
start_time: timestamp
progress: RestoreProgress
results: RestoreResults (when completed)
errors: array of RestoreError
```

### ValidationReport
Comprehensive validation results:
```yaml
validation_status: enum [passed, failed, warning]
total_checks: integer
passed_checks: integer
failed_checks: integer
checks: array of ValidationCheck
```

## 🎯 Common Use Cases

### 1. Simple Complete Restore
```bash
# Start a complete restore from a specific backup
curl -X POST https://api.example.com/api/v1/restore \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "backup_id": "backup-123",
    "cluster_name": "production",
    "restore_mode": "complete"
  }'
```

### 2. Selective Namespace Restore
```bash
# Restore only specific namespaces
curl -X POST https://api.example.com/api/v1/restore \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "backup_id": "backup-123",
    "cluster_name": "production",
    "restore_mode": "selective",
    "target_namespaces": ["app-namespace", "data-namespace"]
  }'
```

### 3. Dry Run Validation
```bash
# Validate restore without actually performing it
curl -X POST https://api.example.com/api/v1/restore \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "backup_id": "backup-123",
    "cluster_name": "production",
    "restore_mode": "validation",
    "dry_run": true
  }'
```

### 4. Disaster Recovery Scenario
```bash
# Execute a predefined DR scenario
curl -X POST https://api.example.com/api/v1/dr/execute \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "source_cluster": "primary",
    "target_cluster": "dr-site",
    "scenario_type": "cluster_rebuild",
    "automation_level": "assisted"
  }'
```

### 5. Monitor Restore Progress
```bash
# Get real-time restore progress
RESTORE_ID="550e8400-e29b-41d4-a716-446655440000"
while true; do
  curl -s https://api.example.com/api/v1/restore/$RESTORE_ID \
    -H "X-API-Key: your-api-key" | \
    jq '.data.progress.percent_complete'
  sleep 5
done
```

## ⚠️ Error Codes

Common error codes returned by the API:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `validation_error` | 400 | Request validation failed |
| `unauthorized` | 401 | Authentication required |
| `forbidden` | 403 | Insufficient permissions |
| `not_found` | 404 | Resource not found |
| `conflict` | 409 | Resource conflict |
| `rate_limit_exceeded` | 429 | Rate limit exceeded |
| `internal_error` | 500 | Internal server error |
| `service_unavailable` | 503 | Service temporarily unavailable |

## 📈 Performance Considerations

### Pagination
List endpoints support pagination:
```bash
# Get first page (50 items)
curl "https://api.example.com/api/v1/backups?limit=50&offset=0"

# Get next page
curl "https://api.example.com/api/v1/backups?limit=50&offset=50"
```

### Filtering
Most list endpoints support filtering:
```bash
# Filter backups by cluster
curl "https://api.example.com/api/v1/backups?cluster_name=production"

# Filter restore history by status
curl "https://api.example.com/api/v1/restore/history?status=completed"
```

### Long-Running Operations
For long-running operations (restores, DR scenarios):
1. Operations return immediately with `202 Accepted`
2. Use the returned operation ID to poll for status
3. Monitor progress via the progress object
4. Set up webhooks for completion notifications

## 🔒 Security Best Practices

1. **Use HTTPS**: Always use HTTPS in production
2. **Rotate API Keys**: Regularly rotate API keys and tokens
3. **Validate Input**: The API validates all input, but validate on client side too
4. **Rate Limiting**: Implement client-side rate limiting to avoid 429 errors
5. **Error Handling**: Don't expose API keys in error logs
6. **Webhooks**: Use webhook signatures to verify authenticity

## 🧪 Testing

### Using curl
```bash
# Test health endpoint
curl -v https://api.example.com/health

# Test with authentication
curl -H "X-API-Key: test-key" https://api.example.com/api/v1/clusters
```

### Using Postman
1. Import the OpenAPI spec into Postman
2. Set up environment variables for API key and base URL
3. Use the generated collection for testing

### Integration Tests
The API includes comprehensive integration tests. See the test files for examples of proper API usage.

## 📞 Support

For API support and questions:
- **Documentation**: This README and the OpenAPI spec
- **Integration Examples**: See `/shared/examples/` directory
- **Issues**: Report bugs and feature requests via GitHub issues
- **Email**: support@example.com

## 🔄 Versioning

The API uses semantic versioning:
- **Major version** (v1, v2): Breaking changes
- **Minor version**: New features, backward compatible
- **Patch version**: Bug fixes, backward compatible

Current version: **v1.0.0**

Version information is included in:
- OpenAPI spec version field
- Response headers: `X-API-Version: 1.0.0`
- Health endpoint response