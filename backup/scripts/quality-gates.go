package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// CoverageReport represents the JSON coverage report structure
type CoverageReport struct {
	Timestamp          string    `json:"timestamp"`
	OverallCoverage    float64   `json:"overall_coverage"`
	GlobalThreshold    float64   `json:"global_threshold"`
	CriticalThreshold  float64   `json:"critical_threshold"`
	MeetsGlobalThreshold bool    `json:"meets_global_threshold"`
	Modules            []Module  `json:"modules"`
	Functions          []Function `json:"functions"`
}

// Module represents coverage data for a module
type Module struct {
	Name            string  `json:"name"`
	Coverage        float64 `json:"coverage"`
	Threshold       float64 `json:"threshold"`
	IsCritical      bool    `json:"is_critical"`
	MeetsThreshold  bool    `json:"meets_threshold"`
}

// Function represents coverage data for a function
type Function struct {
	File     string  `json:"file"`
	Function string  `json:"function"`
	Coverage float64 `json:"coverage"`
}

// Config represents the coverage configuration
type Config struct {
	Coverage struct {
		GlobalThreshold        float64 `yaml:"global_threshold"`
		CriticalPathThreshold  float64 `yaml:"critical_path_threshold"`
		QualityGates          struct {
			EnforceGlobal       bool `yaml:"enforce_global"`
			EnforceCritical     bool `yaml:"enforce_critical"`
			BlockOnViolation    bool `yaml:"block_on_violation"`
			BlockOnlyCritical   bool `yaml:"block_only_critical"`
		} `yaml:"quality_gates"`
	} `yaml:"coverage"`
	Modules map[string]struct {
		Threshold   float64 `yaml:"threshold"`
		IsCritical  bool    `yaml:"is_critical"`
		Description string  `yaml:"description"`
	} `yaml:"modules"`
}

// QualityGateResult represents the result of quality gate validation
type QualityGateResult struct {
	Passed             bool     `json:"passed"`
	GlobalGatePassed   bool     `json:"global_gate_passed"`
	CriticalGatePassed bool     `json:"critical_gate_passed"`
	Violations         []string `json:"violations"`
	Warnings           []string `json:"warnings"`
	Summary            string   `json:"summary"`
	ExitCode           int      `json:"exit_code"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <coverage-report.json> [config.yml]\n", os.Args[0])
		os.Exit(1)
	}

	reportPath := os.Args[1]
	configPath := "coverage-config.yml"
	if len(os.Args) > 2 {
		configPath = os.Args[2]
	}

	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config from %s: %v", configPath, err)
		// Use default configuration
		config = getDefaultConfig()
	}

	// Load coverage report
	report, err := loadCoverageReport(reportPath)
	if err != nil {
		log.Fatalf("Failed to load coverage report: %v", err)
	}

	// Validate quality gates
	result := validateQualityGates(report, config)

	// Output result as JSON
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal result: %v", err)
	}

	fmt.Println(string(resultJSON))

	// Print summary to stderr for visibility
	fmt.Fprintf(os.Stderr, "\n=== QUALITY GATES SUMMARY ===\n")
	fmt.Fprintf(os.Stderr, "%s\n", result.Summary)
	if len(result.Violations) > 0 {
		fmt.Fprintf(os.Stderr, "\nVIOLATIONS:\n")
		for _, violation := range result.Violations {
			fmt.Fprintf(os.Stderr, "  ❌ %s\n", violation)
		}
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "\nWARNINGS:\n")
		for _, warning := range result.Warnings {
			fmt.Fprintf(os.Stderr, "  ⚠️  %s\n", warning)
		}
	}
	fmt.Fprintf(os.Stderr, "===========================\n\n")

	os.Exit(result.ExitCode)
}

func loadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func getDefaultConfig() *Config {
	return &Config{
		Coverage: struct {
			GlobalThreshold        float64 `yaml:"global_threshold"`
			CriticalPathThreshold  float64 `yaml:"critical_path_threshold"`
			QualityGates          struct {
				EnforceGlobal       bool `yaml:"enforce_global"`
				EnforceCritical     bool `yaml:"enforce_critical"`
				BlockOnViolation    bool `yaml:"block_on_violation"`
				BlockOnlyCritical   bool `yaml:"block_only_critical"`
			} `yaml:"quality_gates"`
		}{
			GlobalThreshold:       80.0,
			CriticalPathThreshold: 90.0,
			QualityGates: struct {
				EnforceGlobal       bool `yaml:"enforce_global"`
				EnforceCritical     bool `yaml:"enforce_critical"`
				BlockOnViolation    bool `yaml:"block_on_violation"`
				BlockOnlyCritical   bool `yaml:"block_only_critical"`
			}{
				EnforceGlobal:     true,
				EnforceCritical:   true,
				BlockOnViolation:  true,
				BlockOnlyCritical: false,
			},
		},
	}
}

func loadCoverageReport(path string) (*CoverageReport, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var report CoverageReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}

	return &report, nil
}

func validateQualityGates(report *CoverageReport, config *Config) *QualityGateResult {
	result := &QualityGateResult{
		Passed:             true,
		GlobalGatePassed:   true,
		CriticalGatePassed: true,
		Violations:         []string{},
		Warnings:           []string{},
	}

	// Check global coverage threshold
	if config.Coverage.QualityGates.EnforceGlobal {
		if report.OverallCoverage < config.Coverage.GlobalThreshold {
			result.GlobalGatePassed = false
			result.Passed = false
			violation := fmt.Sprintf("Global coverage %.1f%% below threshold %.1f%%",
				report.OverallCoverage, config.Coverage.GlobalThreshold)
			result.Violations = append(result.Violations, violation)
		}
	}

	// Check critical path coverage
	criticalViolations := 0
	moduleViolations := 0
	
	for _, module := range report.Modules {
		if !module.MeetsThreshold {
			moduleViolations++
			violation := fmt.Sprintf("Module %s coverage %.1f%% below threshold %.1f%%",
				module.Name, module.Coverage, module.Threshold)
			
			if module.IsCritical {
				criticalViolations++
				result.CriticalGatePassed = false
				result.Violations = append(result.Violations, "CRITICAL: "+violation)
			} else {
				result.Warnings = append(result.Warnings, violation)
			}
		}
	}

	// Apply quality gate enforcement rules
	if config.Coverage.QualityGates.EnforceCritical && criticalViolations > 0 {
		result.Passed = false
	}

	if config.Coverage.QualityGates.BlockOnViolation && moduleViolations > 0 && !config.Coverage.QualityGates.BlockOnlyCritical {
		result.Passed = false
	}

	// Generate summary
	result.Summary = generateSummary(report, result, criticalViolations, moduleViolations)

	// Determine exit code
	if !result.Passed {
		if criticalViolations > 0 {
			result.ExitCode = 2 // Critical violations
		} else {
			result.ExitCode = 1 // Regular violations
		}
	} else {
		result.ExitCode = 0 // All gates passed
	}

	return result
}

func generateSummary(report *CoverageReport, result *QualityGateResult, criticalViolations, moduleViolations int) string {
	var summary strings.Builder

	// Overall status
	if result.Passed {
		summary.WriteString("✅ All quality gates PASSED")
	} else {
		if criticalViolations > 0 {
			summary.WriteString("🚨 CRITICAL quality gate violations detected")
		} else {
			summary.WriteString("❌ Quality gate violations detected")
		}
	}

	summary.WriteString(fmt.Sprintf("\n📊 Overall Coverage: %.1f%%", report.OverallCoverage))

	// Gate status
	summary.WriteString("\n\n🚪 Gate Status:")
	if result.GlobalGatePassed {
		summary.WriteString("\n  ✅ Global Coverage Gate: PASSED")
	} else {
		summary.WriteString("\n  ❌ Global Coverage Gate: FAILED")
	}

	if result.CriticalGatePassed {
		summary.WriteString("\n  ✅ Critical Path Gate: PASSED")
	} else {
		summary.WriteString("\n  ❌ Critical Path Gate: FAILED")
	}

	// Module statistics
	totalModules := len(report.Modules)
	passingModules := totalModules - moduleViolations
	criticalModules := 0
	criticalPassing := 0

	for _, module := range report.Modules {
		if module.IsCritical {
			criticalModules++
			if module.MeetsThreshold {
				criticalPassing++
			}
		}
	}

	summary.WriteString(fmt.Sprintf("\n\n📈 Module Statistics:"))
	summary.WriteString(fmt.Sprintf("\n  Total Modules: %d", totalModules))
	summary.WriteString(fmt.Sprintf("\n  Passing Modules: %d/%d (%.1f%%)", 
		passingModules, totalModules, float64(passingModules)/float64(totalModules)*100))
	summary.WriteString(fmt.Sprintf("\n  Critical Paths: %d/%d passing", criticalPassing, criticalModules))

	if len(result.Violations) > 0 {
		summary.WriteString(fmt.Sprintf("\n  Violations: %d", len(result.Violations)))
	}
	if len(result.Warnings) > 0 {
		summary.WriteString(fmt.Sprintf("\n  Warnings: %d", len(result.Warnings)))
	}

	return summary.String()
}

// Additional utility functions for integration

func init() {
	// Ensure we can handle environment variable overrides
	if threshold := os.Getenv("COVERAGE_GLOBAL_THRESHOLD"); threshold != "" {
		if val, err := strconv.ParseFloat(threshold, 64); err == nil {
			// This would be used to override config values
			_ = val
		}
	}
}

// GetCoverageRecommendations analyzes the coverage report and provides actionable recommendations
func GetCoverageRecommendations(report *CoverageReport, config *Config) []string {
	var recommendations []string

	// Find modules with the biggest gaps
	type moduleGap struct {
		name string
		gap  float64
		critical bool
	}

	var gaps []moduleGap
	for _, module := range report.Modules {
		if !module.MeetsThreshold {
			gap := module.Threshold - module.Coverage
			gaps = append(gaps, moduleGap{
				name:     module.Name,
				gap:      gap,
				critical: module.IsCritical,
			})
		}
	}

	// Sort by gap size (largest first)
	for i := 0; i < len(gaps)-1; i++ {
		for j := i + 1; j < len(gaps); j++ {
			if gaps[i].gap < gaps[j].gap || (gaps[i].gap == gaps[j].gap && !gaps[i].critical && gaps[j].critical) {
				gaps[i], gaps[j] = gaps[j], gaps[i]
			}
		}
	}

	// Generate recommendations
	if len(gaps) > 0 {
		recommendations = append(recommendations, "🎯 Priority Actions:")
		
		for i, gap := range gaps {
			if i >= 5 { // Limit to top 5 recommendations
				break
			}
			
			priority := "📋"
			if gap.critical {
				priority = "🚨"
			}
			
			recommendation := fmt.Sprintf("%s Focus on %s (need +%.1f%% coverage)", 
				priority, gap.name, gap.gap)
			recommendations = append(recommendations, recommendation)
		}
	}

	// General recommendations based on coverage level
	if report.OverallCoverage < 60 {
		recommendations = append(recommendations, 
			"📚 Consider implementing a testing strategy workshop",
			"🔧 Set up test generation tools and utilities",
			"📖 Review testing best practices documentation")
	} else if report.OverallCoverage < 80 {
		recommendations = append(recommendations,
			"🎯 Focus on edge cases and error paths",
			"🔄 Add integration tests for critical workflows",
			"🧪 Implement property-based testing for complex logic")
	} else {
		recommendations = append(recommendations,
			"🏆 Excellent coverage! Focus on test quality and maintenance",
			"🔍 Consider mutation testing to verify test effectiveness",
			"📊 Monitor coverage trends to prevent regression")
	}

	return recommendations
}

// ValidateConfigFile validates the coverage configuration file
func ValidateConfigFile(configPath string) error {
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate threshold ranges
	if config.Coverage.GlobalThreshold < 0 || config.Coverage.GlobalThreshold > 100 {
		return fmt.Errorf("global threshold must be between 0 and 100")
	}

	if config.Coverage.CriticalPathThreshold < 0 || config.Coverage.CriticalPathThreshold > 100 {
		return fmt.Errorf("critical path threshold must be between 0 and 100")
	}

	// Validate module thresholds
	for moduleName, moduleConfig := range config.Modules {
		if moduleConfig.Threshold < 0 || moduleConfig.Threshold > 100 {
			return fmt.Errorf("module %s threshold must be between 0 and 100", moduleName)
		}
	}

	return nil
}