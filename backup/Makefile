# Makefile for Kubernetes Backup System
# Enhanced with comprehensive coverage analysis and quality gates

# Build variables
BINARY_NAME=backup
GO_VERSION=1.21
BUILD_DIR=build
COVERAGE_DIR=coverage
REPORTS_DIR=$(COVERAGE_DIR)/reports
TRENDS_DIR=$(COVERAGE_DIR)/trends

# Docker variables
DOCKER_IMAGE=backup
DOCKER_TAG=latest

# Test variables
TEST_TIMEOUT=10m
INTEGRATION_TEST_TAG=integration

# Coverage settings
COVERAGE_THRESHOLD=80
CRITICAL_THRESHOLD=90
COVERAGE_PROFILE=$(COVERAGE_DIR)/coverage.out
COVERAGE_HTML=$(REPORTS_DIR)/coverage.html
COVERAGE_JSON=$(REPORTS_DIR)/coverage.json

# Quality gate settings
QUALITY_GATES_BINARY=scripts/quality-gates
CONFIG_FILE=coverage-config.yml

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: clean
clean: ## Clean build artifacts and test results
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -f coverage.out coverage.html
	go clean -testcache
	go clean -cache

.PHONY: deps
deps: ## Download and verify dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod verify
	go mod tidy

.PHONY: build
build: deps ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags '-w -s' -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/backup

.PHONY: build-dev
build-dev: deps ## Build the binary with debug info
	@echo "Building $(BINARY_NAME) with debug info..."
	mkdir -p $(BUILD_DIR)
	go build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME)-dev ./cmd/backup

.PHONY: test-unit
test-unit: ## Run unit tests
	@echo "Running unit tests..."
	go test -v -race -timeout $(TEST_TIMEOUT) \
		./internal/config \
		./internal/logging \
		./internal/backup \
		./internal/metrics

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker)
	@echo "Running integration tests..."
	@echo "Starting MinIO container..."
	docker run -d --name test-minio \
		-p 9000:9000 \
		-e MINIO_ROOT_USER=minioadmin \
		-e MINIO_ROOT_PASSWORD=minioadmin123 \
		minio/minio:latest server /data || true
	@echo "Waiting for MinIO to be ready..."
	@timeout 60 bash -c 'until curl -f http://localhost:9000/minio/health/live; do sleep 2; done' || (echo "MinIO failed to start" && exit 1)
	@echo "Running integration tests..."
	go test -v -timeout $(TEST_TIMEOUT) -tags=$(INTEGRATION_TEST_TAG) ./tests/integration/... || \
		(docker stop test-minio && docker rm test-minio && exit 1)
	@echo "Stopping MinIO container..."
	docker stop test-minio && docker rm test-minio

.PHONY: test
test: test-unit test-integration ## Run all tests

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	mkdir -p $(COVERAGE_DIR)
	go test -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic \
		./internal/config \
		./internal/logging \
		./internal/backup \
		./internal/metrics
	go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	go tool cover -func=$(COVERAGE_DIR)/coverage.out

.PHONY: coverage-analysis
coverage-analysis: install-coverage-tools ## Run comprehensive coverage analysis
	@echo "Running comprehensive coverage analysis..."
	@./scripts/coverage-analysis.sh
	@echo "Coverage analysis completed. Reports available in $(REPORTS_DIR)/"

.PHONY: coverage-analysis-ci
coverage-analysis-ci: install-coverage-tools ## Run coverage analysis in CI mode
	@echo "Running coverage analysis in CI mode..."
	@./scripts/coverage-analysis.sh --ci-mode

.PHONY: coverage-dashboard
coverage-dashboard: ## Generate interactive coverage dashboard
	@echo "Generating coverage dashboard..."
	@python3 scripts/coverage-dashboard.py
	@echo "Dashboard generated: $(COVERAGE_DIR)/dashboard.html"

.PHONY: quality-gates
quality-gates: build-quality-gates ## Validate coverage quality gates
	@echo "Validating quality gates..."
	@if [ -f $(REPORTS_DIR)/latest.json ]; then \
		./$(QUALITY_GATES_BINARY) $(REPORTS_DIR)/latest.json $(CONFIG_FILE); \
	else \
		echo "Error: No coverage report found. Run 'make coverage-analysis' first."; \
		exit 1; \
	fi

.PHONY: build-quality-gates
build-quality-gates: ## Build quality gates validator
	@echo "Building quality gates validator..."
	@./scripts/build-quality-gates.sh

.PHONY: install-coverage-tools
install-coverage-tools: ## Install coverage analysis dependencies
	@echo "Installing coverage tools..."
	@mkdir -p $(COVERAGE_DIR) $(REPORTS_DIR) $(TRENDS_DIR)
	@chmod +x scripts/coverage-analysis.sh
	@chmod +x scripts/coverage-dashboard.py
	@if ! command -v python3 >/dev/null 2>&1; then \
		echo "Warning: Python3 not found. Dashboard generation will not work."; \
	fi

.PHONY: test-short
test-short: ## Run short tests only (no integration tests)
	@echo "Running short tests..."
	go test -short -v -race ./...

.PHONY: benchmark
benchmark: ## Run benchmark tests
	@echo "Running benchmark tests..."
	go test -bench=. -benchmem -run=^$$ \
		./internal/config \
		./internal/logging \
		./internal/backup

.PHONY: lint
lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it from https://golangci-lint.run/" && exit 1)
	golangci-lint run --timeout=5m

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

.PHONY: security
security: ## Run security scanner
	@echo "Running security scanner..."
	@which gosec > /dev/null || (echo "gosec not found. Install it with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest" && exit 1)
	gosec -fmt json -out security-report.json -stdout ./...

.PHONY: quality-check
quality-check: test lint security coverage-analysis quality-gates ## Run complete quality pipeline
	@echo "✅ All quality checks completed successfully"

.PHONY: check
check: fmt vet lint security test-short ## Run all checks (format, vet, lint, security, short tests)

.PHONY: ci-test
ci-test: install-coverage-tools test-unit test-integration ## Run all tests for CI
	@echo "CI tests completed"

.PHONY: ci-coverage
ci-coverage: install-coverage-tools coverage-analysis-ci quality-gates ## Run coverage analysis for CI
	@echo "CI coverage analysis completed"

.PHONY: ci-full
ci-full: install-tools ci-test lint security ci-coverage build ## Run full CI pipeline
	@echo "✅ Full CI pipeline completed successfully"

.PHONY: validate-thresholds
validate-thresholds: coverage-analysis ## Validate coverage meets thresholds
	@echo "Validating coverage thresholds..."
	@if [ -f $(REPORTS_DIR)/latest.json ]; then \
		OVERALL=$$(cat $(REPORTS_DIR)/latest.json | grep -o '"overall_coverage":[0-9.]*' | cut -d':' -f2); \
		if [ $$(echo "$$OVERALL >= $(COVERAGE_THRESHOLD)" | bc -l) -eq 1 ]; then \
			echo "✅ Overall coverage $$OVERALL% meets threshold $(COVERAGE_THRESHOLD)%"; \
		else \
			echo "❌ Overall coverage $$OVERALL% below threshold $(COVERAGE_THRESHOLD)%"; \
			exit 1; \
		fi; \
	else \
		echo "Error: Coverage report not found"; \
		exit 1; \
	fi

.PHONY: coverage-trends
coverage-trends: ## Analyze coverage trends
	@echo "Analyzing coverage trends..."
	@if [ -f $(TRENDS_DIR)/coverage_trends.csv ]; then \
		echo "Recent coverage trends:"; \
		tail -n 10 $(TRENDS_DIR)/coverage_trends.csv | column -t -s','; \
	else \
		echo "No trend data available. Run coverage analysis to generate trends."; \
	fi

.PHONY: reports
reports: coverage-analysis coverage-dashboard ## Generate all coverage reports
	@echo "All coverage reports generated:"
	@echo "  HTML Report:  $(REPORTS_DIR)/latest.html"
	@echo "  JSON Report:  $(REPORTS_DIR)/latest.json"
	@echo "  Dashboard:    $(COVERAGE_DIR)/dashboard.html"
	@echo "  Quality Gates: $(REPORTS_DIR)/latest_quality_gates.md"

.PHONY: coverage-summary
coverage-summary: ## Show coverage summary from latest report
	@if [ -f $(REPORTS_DIR)/latest.json ]; then \
		echo "Coverage Summary:"; \
		echo "================"; \
		cat $(REPORTS_DIR)/latest.json | python3 -c "import sys,json; data=json.load(sys.stdin); print(f'Overall: {data[\"overall_coverage\"]:.1f}%'); [print(f'{m[\"name\"]}: {m[\"coverage\"]:.1f}% ({\"✅\" if m[\"meets_threshold\"] else \"❌\"})') for m in data[\"modules\"]]"; \
	else \
		echo "No coverage report found. Run 'make coverage-analysis' first."; \
	fi

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: docker-test
docker-test: docker-build ## Test Docker image
	@echo "Testing Docker image..."
	docker run --rm $(DOCKER_IMAGE):$(DOCKER_TAG) --help

.PHONY: install-tools
install-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest

.PHONY: dev-setup
dev-setup: install-tools install-coverage-tools deps ## Set up development environment
	@echo "Development environment setup complete!"
	@echo "Available commands:"
	@echo "  make test                - Run tests"
	@echo "  make coverage-analysis   - Generate coverage reports"
	@echo "  make quality-check       - Run full quality pipeline"
	@echo "  make coverage-dashboard  - Generate interactive dashboard"

.PHONY: run
run: build ## Build and run the backup tool
	@echo "Running backup tool..."
	$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: run-dev
run-dev: ## Run backup tool in development mode (with live reload)
	@echo "Running backup tool in development mode..."
	go run ./cmd/backup

.PHONY: generate-mocks
generate-mocks: ## Generate mock files
	@echo "Generating mocks..."
	@which mockgen > /dev/null || (echo "mockgen not found. Install it with: go install github.com/golang/mock/mockgen@latest" && exit 1)
	go generate ./...

.PHONY: validate-config
validate-config: ## Validate configuration files
	@echo "Validating configuration files..."
	@for file in *.yaml *.yml; do \
		if [ -f "$$file" ]; then \
			echo "Validating $$file..."; \
			python -c "import yaml; yaml.safe_load(open('$$file'))" || exit 1; \
		fi \
	done

.PHONY: release-build
release-build: clean ## Build release binaries for multiple platforms
	@echo "Building release binaries..."
	mkdir -p $(BUILD_DIR)/releases
	
	# Linux AMD64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo \
		-ldflags '-w -s -X main.version=$(shell git describe --tags --always)' \
		-o $(BUILD_DIR)/releases/$(BINARY_NAME)-linux-amd64 ./cmd/backup
	
	# Linux ARM64
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -a -installsuffix cgo \
		-ldflags '-w -s -X main.version=$(shell git describe --tags --always)' \
		-o $(BUILD_DIR)/releases/$(BINARY_NAME)-linux-arm64 ./cmd/backup
	
	# Darwin AMD64
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo \
		-ldflags '-w -s -X main.version=$(shell git describe --tags --always)' \
		-o $(BUILD_DIR)/releases/$(BINARY_NAME)-darwin-amd64 ./cmd/backup
	
	# Darwin ARM64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -a -installsuffix cgo \
		-ldflags '-w -s -X main.version=$(shell git describe --tags --always)' \
		-o $(BUILD_DIR)/releases/$(BINARY_NAME)-darwin-arm64 ./cmd/backup
	
	@echo "Release binaries built in $(BUILD_DIR)/releases/"

.PHONY: docs
docs: ## Generate documentation
	@echo "Generating documentation..."
	@which godoc > /dev/null || (echo "godoc not found. Install it with: go install golang.org/x/tools/cmd/godoc@latest" && exit 1)
	mkdir -p docs
	godoc -html -goroot . > docs/api.html

# Default target
.DEFAULT_GOAL := help