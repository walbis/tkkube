# Testing Guide for Backup Tool

This document provides comprehensive information about the testing infrastructure for the Kubernetes backup tool.

## Overview

The backup tool includes a comprehensive test suite with:
- **Unit Tests**: Test individual components in isolation
- **Integration Tests**: Test with real MinIO instances using testcontainers
- **Benchmark Tests**: Performance testing for critical functions
- **Security Scanning**: Static analysis for security vulnerabilities
- **Linting**: Code quality and style checks

## Test Structure

```
backup/
├── internal/               # Main application code
│   ├── config/            # Configuration management
│   │   ├── config.go
│   │   └── config_test.go
│   ├── logging/           # Structured logging
│   │   ├── logger.go
│   │   └── logger_test.go
│   ├── backup/            # Core backup logic
│   │   ├── backup.go
│   │   └── backup_test.go
│   └── metrics/           # Prometheus metrics
│       └── metrics.go
├── tests/                 # Test infrastructure
│   ├── mocks/            # Mock implementations
│   │   ├── kubernetes_mock.go
│   │   └── minio_mock.go
│   └── integration/      # Integration tests
│       └── backup_integration_test.go
├── scripts/
│   └── run-tests.sh     # Test runner script
└── Makefile             # Build and test targets
```

## Running Tests

### Quick Start

```bash
# Run all tests
make test

# Run only unit tests
make test-unit

# Run only integration tests
make test-integration

# Run tests with coverage
make test-coverage

# Run short tests (no integration)
make test-short
```

### Using Test Script

The test script provides more control and better output:

```bash
# Run all tests
./scripts/run-tests.sh

# Run only unit tests
./scripts/run-tests.sh --unit-only

# Run only integration tests  
./scripts/run-tests.sh --integration-only

# Fast mode (unit tests, linting, build)
./scripts/run-tests.sh --fast

# Include benchmark tests
./scripts/run-tests.sh --benchmarks
```

### Manual Test Execution

```bash
# Unit tests for specific packages
go test -v ./internal/config
go test -v ./internal/logging
go test -v ./internal/backup

# Integration tests (requires Docker)
go test -v ./tests/integration/...

# All tests with race detection
go test -race ./...

# Benchmark tests
go test -bench=. -benchmem ./internal/...
```

## Test Categories

### Unit Tests

Unit tests verify individual components in isolation using mocks:

**Configuration Tests** (`internal/config/config_test.go`):
- Environment variable parsing
- Configuration validation
- Default value handling
- Error conditions

**Logging Tests** (`internal/logging/logger_test.go`):
- Structured log formatting
- Log level handling
- JSON marshaling
- Context management

**Backup Tests** (`internal/backup/backup_test.go`):
- Backup orchestration logic
- Namespace filtering
- Resource filtering
- Error handling

### Integration Tests

Integration tests use real external services (MinIO) via testcontainers:

**Backup Integration** (`tests/integration/backup_integration_test.go`):
- End-to-end backup workflow
- MinIO connectivity
- Bucket operations
- Error scenarios

### Mock Objects

Mock implementations for testing:

**Kubernetes Mock** (`tests/mocks/kubernetes_mock.go`):
- Fake Kubernetes clients
- Mock namespaces and resources
- Configurable error conditions

**MinIO Mock** (`tests/mocks/minio_mock.go`):
- In-memory object storage
- Bucket operations
- Object operations
- Call logging for verification

## Coverage Requirements

- **Target Coverage**: 80%
- **Current Coverage**: Tracked in CI
- **Coverage Report**: Generated as `coverage.html`

View coverage:
```bash
make test-coverage
open coverage.html
```

## Continuous Integration

GitHub Actions workflow (`.github/workflows/test.yml`) runs:

1. **Unit Tests**: Fast isolated tests
2. **Integration Tests**: With real MinIO service
3. **Linting**: Code quality checks
4. **Security Scanning**: Vulnerability detection
5. **Build Verification**: Binary compilation
6. **Docker Build**: Container image creation

## Test Configuration

### Environment Variables

Integration tests use these environment variables:

```bash
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin123
```

### Test Flags

Go test flags commonly used:

```bash
# Skip integration tests
go test -short ./...

# Verbose output
go test -v ./...

# Race condition detection
go test -race ./...

# Coverage measurement
go test -coverprofile=coverage.out ./...

# Timeout for long tests
go test -timeout 10m ./...
```

## Writing Tests

### Unit Test Example

```go
func TestConfigValidation(t *testing.T) {
    tests := []struct {
        name        string
        config      *Config
        expectError bool
    }{
        {
            name: "valid_config",
            config: &Config{
                MinIOEndpoint: "localhost:9000",
                MinIOAccessKey: "testkey",
                MinIOSecretKey: "testsecret",
            },
            expectError: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Integration Test Example

```go
func TestBackupIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Start MinIO container
    container, endpoint, err := startMinIOContainer(t)
    require.NoError(t, err)
    defer container.Terminate(context.Background())

    // Create real MinIO client
    minioClient, err := minio.New(endpoint, &minio.Options{
        Creds: credentials.NewStaticV4("minioadmin", "minioadmin123", ""),
        Secure: false,
    })
    require.NoError(t, err)

    // Run backup with real MinIO
    // ... test implementation
}
```

### Mock Usage Example

```go
func TestBackupWithMinio(t *testing.T) {
    mockMinio := mocks.NewMockMinioClient()
    mockMinio.AddTestBucket("test-bucket")
    
    backup := &ClusterBackup{
        minioClient: mockMinio,
        // ... other fields
    }

    err := backup.testMinIOConnectivity()
    assert.NoError(t, err)
    
    // Verify mock was called correctly
    callLog := mockMinio.GetCallLog()
    assert.Contains(t, callLog, "BucketExists(test-bucket)")
}
```

## Debugging Tests

### Verbose Output

```bash
go test -v ./internal/backup
```

### Test Specific Functions

```bash
go test -v -run TestBackupNamespace ./internal/backup
```

### Debug Integration Tests

```bash
# Keep MinIO container running for inspection
docker run -d --name debug-minio \
  -p 9000:9000 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin123 \
  minio/minio:latest server /data

# Run tests against persistent container
MINIO_ENDPOINT=localhost:9000 go test -v ./tests/integration/...
```

## Performance Testing

### Benchmark Tests

```bash
# Run all benchmarks
go test -bench=. ./...

# Benchmark specific functions
go test -bench=BenchmarkConfigLoad ./internal/config

# Memory allocation profiling
go test -bench=. -benchmem ./...
```

### Example Benchmark

```go
func BenchmarkConfigLoad(b *testing.B) {
    // Setup
    os.Setenv("MINIO_ENDPOINT", "localhost:9000")
    os.Setenv("MINIO_ACCESS_KEY", "testkey")
    // ... more setup

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := LoadConfig()
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## Troubleshooting

### Common Issues

**Integration tests fail with MinIO connection error**:
- Ensure Docker is running
- Check port conflicts (port 9000)
- Verify MinIO container health

**Tests are slow**:
- Use `-short` flag to skip integration tests
- Run specific test packages instead of all tests

**Coverage is low**:
- Add tests for untested functions
- Remove unused code
- Focus on critical paths

**Flaky tests**:
- Add proper timeouts
- Use deterministic test data
- Avoid race conditions

### Debugging Commands

```bash
# Check running containers
docker ps

# View MinIO container logs
docker logs test-minio

# Check test coverage
go tool cover -func=coverage.out

# Profile test execution
go test -cpuprofile=cpu.prof ./...
```

## Best Practices

1. **Test Naming**: Use descriptive test names that explain the scenario
2. **Test Organization**: Group related tests using subtests
3. **Mock Usage**: Use mocks for external dependencies
4. **Error Testing**: Always test error conditions
5. **Cleanup**: Ensure tests clean up resources
6. **Determinism**: Make tests deterministic and repeatable
7. **Performance**: Keep unit tests fast, use integration tests sparingly
8. **Documentation**: Document complex test scenarios

## Development Workflow

1. **Write Tests First**: Use TDD approach where appropriate
2. **Run Tests Frequently**: Use `make test-short` during development
3. **Check Coverage**: Ensure new code is tested
4. **Integration Testing**: Test with real services before committing
5. **CI Verification**: Ensure all tests pass in CI

## Tools and Dependencies

**Testing Libraries**:
- `github.com/stretchr/testify` - Assertions and test utilities
- `github.com/testcontainers/testcontainers-go` - Integration test containers
- `github.com/golang/mock` - Mock generation

**Quality Tools**:
- `golangci-lint` - Comprehensive linter
- `gosec` - Security scanner
- `go tool cover` - Coverage analysis