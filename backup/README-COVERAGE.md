# Coverage Analysis System

## Quick Start

### Setup
```bash
# Install coverage tools and dependencies
make dev-setup

# Run comprehensive coverage analysis
make coverage-analysis

# Generate interactive dashboard
make coverage-dashboard

# Validate quality gates
make quality-gates
```

### Key Commands
```bash
make coverage-analysis     # Full coverage analysis with reports
make coverage-dashboard    # Interactive HTML dashboard
make quality-gates         # Validate coverage thresholds
make quality-check         # Complete quality pipeline
make coverage-summary      # Quick coverage overview
make coverage-trends       # Show coverage trend analysis
```

## Coverage Standards

### Quality Gates
- **Global Coverage**: 80% minimum across all code
- **Critical Modules**: 90% minimum for mission-critical components
- **Module Thresholds**: 70-85% based on component importance

### Critical Modules
- `internal/backup` - Core backup functionality (90%)
- `internal/resilience` - Circuit breakers and retry logic (90%)
- `internal/orchestrator` - Backup orchestration (90%)

## Reports Generated

### Location: `coverage/reports/`
- **HTML Report** (`latest.html`) - Interactive coverage visualization
- **JSON Report** (`latest.json`) - Machine-readable data for CI/CD
- **Module Analysis** (`module_analysis_*.md`) - Detailed module breakdown
- **Critical Paths** (`critical_paths_*.md`) - Focus areas for improvement
- **Quality Gates** (`quality_gates_*.md`) - Threshold compliance status

### Dashboard: `coverage/dashboard.html`
Interactive web-based dashboard with:
- Coverage metrics and trends
- Module status visualization
- Quality gate compliance
- Historical trend analysis

## CI/CD Integration

### GitHub Actions
Automated workflow in `.github/workflows/coverage-check.yml`:
- Runs on every push and pull request
- Comments coverage results on PRs
- Sets commit status based on quality gates
- Archives coverage reports and trends

### Quality Gate Exit Codes
- `0`: All quality gates passed ✅
- `1`: Quality gate violations (warnings) ⚠️
- `2`: Critical path violations (blocking) 🚨
- `3`: Analysis failed ❌

## Configuration

### Main Config: `coverage-config.yml`
Customize:
- Coverage thresholds per module
- Quality gate enforcement rules
- Critical path definitions
- Reporting preferences
- CI/CD integration settings

### Environment Variables
```bash
COVERAGE_THRESHOLD=80        # Override global threshold
CRITICAL_THRESHOLD=90        # Override critical threshold
COVERAGE_CI_MODE=true        # Enable CI mode
```

## Usage Examples

### Development Workflow
```bash
# Start development
make dev-setup

# Write tests and code
# ...

# Check coverage
make coverage-analysis
make coverage-summary

# Review in browser
open coverage/dashboard.html

# Validate before commit
make quality-check
```

### CI/CD Pipeline
```bash
# Full CI pipeline
make ci-full

# Coverage-only check
make ci-coverage

# Validate thresholds
make validate-thresholds
```

### Coverage Improvement
```bash
# Identify improvement areas
make coverage-analysis
cat coverage/reports/critical_paths_*.md

# Focus on specific modules
# Add tests for uncovered functions

# Validate improvements
make quality-gates
```

## Troubleshooting

### Common Issues
1. **"No coverage report found"**
   - Run `make coverage-analysis` first
   
2. **"Python3 not found"**
   - Install Python 3.x for dashboard generation
   
3. **"Quality gates validator not found"**
   - Run `make build-quality-gates`

4. **Coverage analysis fails**
   - Check Go environment: `go version`
   - Verify dependencies: `go mod verify`

### Getting Help
```bash
make help                    # Show all available commands
./scripts/coverage-analysis.sh --help
```

## Advanced Features

### Trend Analysis
- Historical coverage tracking
- Regression detection
- Progress monitoring
- Commit-based correlation

### Quality Recommendations
- Automated improvement suggestions
- Priority-based recommendations
- Code-specific guidance
- Testing strategy advice

### Extensibility
- Configurable thresholds
- Custom quality gates
- Multiple output formats
- Integration with external tools

---

For detailed documentation, see [docs/COVERAGE_SYSTEM.md](docs/COVERAGE_SYSTEM.md)