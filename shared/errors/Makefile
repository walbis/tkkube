# Makefile for shared-errors module
# Provides build automation for Go and Python error handling system

.PHONY: test test-go test-python coverage coverage-go lint lint-go lint-python format clean all help

# Default target
all: lint test coverage

# Help target
help:
	@echo "Available targets:"
	@echo "  test         - Run all tests (Go and Python)"
	@echo "  test-go      - Run Go tests"
	@echo "  test-python  - Run Python tests"
	@echo "  coverage     - Generate test coverage reports"
	@echo "  coverage-go  - Generate Go test coverage"
	@echo "  lint         - Run all linters"
	@echo "  lint-go      - Run Go linters"
	@echo "  lint-python  - Run Python linters"
	@echo "  format       - Format all source code"
	@echo "  clean        - Clean build artifacts"
	@echo "  all          - Run lint, test, and coverage (default)"

# Test targets
test: test-go test-python

test-go:
	@echo "Running Go tests..."
	go test -v ./...

test-python:
	@echo "Running Python tests..."
	python3 -m unittest test_errors.py -v

# Coverage targets
coverage: coverage-go
	@echo "Coverage reports generated"

coverage-go:
	@echo "Generating Go test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Go coverage report: coverage.html"

# Lint targets
lint: lint-go lint-python

lint-go:
	@echo "Running Go linters..."
	go fmt ./...
	go vet ./...
	@echo "Go linting complete"

lint-python:
	@echo "Running Python linters..."
	@command -v python3 >/dev/null 2>&1 || { echo "Python3 not found"; exit 1; }
	python3 -m py_compile errors.py test_errors.py
	@echo "Python linting complete"

# Format target
format:
	@echo "Formatting source code..."
	go fmt ./...
	@echo "Formatting complete"

# Clean target
clean:
	@echo "Cleaning build artifacts..."
	rm -f coverage.out coverage.html
	find . -name "*.pyc" -delete
	find . -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "Clean complete"

# Quality targets
quality-check: lint test coverage
	@echo "Quality check complete - all tests passed and linting clean"

# CI/CD friendly targets
ci-test: test
	@echo "CI tests completed"

ci-coverage: coverage-go
	@echo "CI coverage analysis completed"

# Development targets
dev-setup:
	@echo "Setting up development environment..."
	go mod download
	@echo "Development setup complete"

# Benchmark target (if needed for performance testing)
bench:
	@echo "Running Go benchmarks..."
	go test -bench=. -benchmem ./...