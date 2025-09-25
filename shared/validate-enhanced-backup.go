package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

type ValidationResult struct {
	File          string
	Valid         bool
	HasAPIVersion bool
	HasKind       bool
	HasMetadata   bool
	ResourceCount int
	Issues        []string
}

type ValidationSummary struct {
	TotalFiles      int
	ValidFiles      int
	InvalidFiles    int
	TotalResources  int
	SchemaComplete  int
	Issues          []string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run validate-enhanced-backup.go <backup-directory>")
		os.Exit(1)
	}

	backupDir := os.Args[1]
	
	fmt.Printf("=== Enhanced Backup Validation ===\n")
	fmt.Printf("Validating backup directory: %s\n\n", backupDir)

	results, err := validateBackupDirectory(backupDir)
	if err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
		os.Exit(1)
	}

	summary := generateSummary(results)
	printResults(results, summary)

	if summary.InvalidFiles > 0 {
		os.Exit(1)
	}

	fmt.Println("✅ Enhanced backup validation passed!")
}

func validateBackupDirectory(backupDir string) ([]ValidationResult, error) {
	var results []ValidationResult

	// Find all YAML files in the backup directory
	files, err := filepath.Glob(filepath.Join(backupDir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to list YAML files: %w", err)
	}

	for _, file := range files {
		// Skip summary files
		if strings.Contains(filepath.Base(file), "summary") {
			continue
		}

		result := validateYAMLFile(file)
		results = append(results, result)
	}

	return results, nil
}

func validateYAMLFile(filename string) ValidationResult {
	result := ValidationResult{
		File:   filepath.Base(filename),
		Issues: []string{},
	}

	// Read file content
	content, err := os.ReadFile(filename)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Failed to read file: %v", err))
		return result
	}

	// Skip comments and split by document separator
	documents := strings.Split(string(content), "---")
	var yamlDocs []string
	
	for _, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc != "" && !strings.HasPrefix(doc, "#") {
			yamlDocs = append(yamlDocs, doc)
		}
	}

	if len(yamlDocs) == 0 {
		result.Issues = append(result.Issues, "No valid YAML documents found")
		return result
	}

	// Parse as resource list
	var resources []map[string]interface{}
	for _, doc := range yamlDocs {
		var resource interface{}
		err := yaml.Unmarshal([]byte(doc), &resource)
		if err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("Invalid YAML syntax: %v", err))
			continue
		}

		// Check if it's a list or single resource
		switch v := resource.(type) {
		case []interface{}:
			for _, item := range v {
				if resourceMap, ok := item.(map[string]interface{}); ok {
					resources = append(resources, resourceMap)
				}
			}
		case map[string]interface{}:
			resources = append(resources, v)
		}
	}

	result.ResourceCount = len(resources)

	// Validate each resource
	schemaCompleteCount := 0
	for i, resource := range resources {
		issues := validateResource(resource, i)
		result.Issues = append(result.Issues, issues...)
		
		// Check for required fields
		hasAPIVersion := resource["apiVersion"] != nil
		hasKind := resource["kind"] != nil
		hasMetadata := resource["metadata"] != nil

		if hasAPIVersion && hasKind && hasMetadata {
			schemaCompleteCount++
		}

		if !hasAPIVersion {
			result.Issues = append(result.Issues, fmt.Sprintf("Resource %d missing apiVersion", i))
		}
		if !hasKind {
			result.Issues = append(result.Issues, fmt.Sprintf("Resource %d missing kind", i))
		}
		if !hasMetadata {
			result.Issues = append(result.Issues, fmt.Sprintf("Resource %d missing metadata", i))
		}
	}

	// Overall validation
	result.Valid = len(result.Issues) == 0
	result.HasAPIVersion = schemaCompleteCount == len(resources)
	result.HasKind = schemaCompleteCount == len(resources)
	result.HasMetadata = schemaCompleteCount == len(resources)

	return result
}

func validateResource(resource map[string]interface{}, index int) []string {
	var issues []string

	// Validate apiVersion
	if apiVersion, ok := resource["apiVersion"]; ok {
		if apiVersionStr, ok := apiVersion.(string); ok {
			if !isValidAPIVersion(apiVersionStr) {
				issues = append(issues, fmt.Sprintf("Resource %d has invalid apiVersion: %s", index, apiVersionStr))
			}
		} else {
			issues = append(issues, fmt.Sprintf("Resource %d apiVersion is not a string", index))
		}
	}

	// Validate kind
	if kind, ok := resource["kind"]; ok {
		if kindStr, ok := kind.(string); ok {
			if !isValidKind(kindStr) {
				issues = append(issues, fmt.Sprintf("Resource %d has invalid kind: %s", index, kindStr))
			}
		} else {
			issues = append(issues, fmt.Sprintf("Resource %d kind is not a string", index))
		}
	}

	// Validate metadata
	if metadata, ok := resource["metadata"]; ok {
		if metadataMap, ok := metadata.(map[string]interface{}); ok {
			if name, hasName := metadataMap["name"]; !hasName || name == "" {
				issues = append(issues, fmt.Sprintf("Resource %d metadata missing name field", index))
			}
			if namespace, hasNamespace := metadataMap["namespace"]; hasNamespace {
				if namespaceStr, ok := namespace.(string); !ok || namespaceStr == "" {
					issues = append(issues, fmt.Sprintf("Resource %d metadata has invalid namespace", index))
				}
			}
		} else {
			issues = append(issues, fmt.Sprintf("Resource %d metadata is not a map", index))
		}
	}

	// Check for problematic runtime fields that should be removed
	problematicFields := []string{"resourceVersion", "uid", "selfLink", "generation"}
	for _, field := range problematicFields {
		if _, exists := resource[field]; exists {
			issues = append(issues, fmt.Sprintf("Resource %d contains runtime field '%s' that should be removed", index, field))
		}
	}

	return issues
}

func isValidAPIVersion(apiVersion string) bool {
	validAPIVersions := map[string]bool{
		"v1":                          true,
		"apps/v1":                     true,
		"networking.k8s.io/v1":        true,
		"rbac.authorization.k8s.io/v1": true,
		"batch/v1":                    true,
		"batch/v1beta1":               true,
		"extensions/v1beta1":          true,
	}
	return validAPIVersions[apiVersion]
}

func isValidKind(kind string) bool {
	validKinds := map[string]bool{
		"Deployment":             true,
		"Service":                true,
		"ConfigMap":              true,
		"Secret":                 true,
		"PersistentVolume":       true,
		"PersistentVolumeClaim":  true,
		"Namespace":              true,
		"Ingress":                true,
		"ServiceAccount":         true,
		"Role":                   true,
		"RoleBinding":            true,
		"ClusterRole":            true,
		"ClusterRoleBinding":     true,
		"Job":                    true,
		"CronJob":                true,
	}
	return validKinds[kind]
}

func generateSummary(results []ValidationResult) ValidationSummary {
	summary := ValidationSummary{}
	
	for _, result := range results {
		summary.TotalFiles++
		summary.TotalResources += result.ResourceCount
		
		if result.Valid {
			summary.ValidFiles++
		} else {
			summary.InvalidFiles++
		}

		if result.HasAPIVersion && result.HasKind && result.HasMetadata {
			summary.SchemaComplete += result.ResourceCount
		}

		summary.Issues = append(summary.Issues, result.Issues...)
	}

	return summary
}

func printResults(results []ValidationResult, summary ValidationSummary) {
	fmt.Println("📋 Validation Results:")
	fmt.Println("=====================")

	for _, result := range results {
		status := "✅"
		if !result.Valid {
			status = "❌"
		}

		fmt.Printf("%s %s (%d resources)\n", status, result.File, result.ResourceCount)
		
		if len(result.Issues) > 0 {
			for _, issue := range result.Issues {
				fmt.Printf("   ⚠️  %s\n", issue)
			}
		}
	}

	fmt.Println("\n📊 Summary:")
	fmt.Println("===========")
	fmt.Printf("Total Files: %d\n", summary.TotalFiles)
	fmt.Printf("Valid Files: %d\n", summary.ValidFiles)
	fmt.Printf("Invalid Files: %d\n", summary.InvalidFiles)
	fmt.Printf("Total Resources: %d\n", summary.TotalResources)
	fmt.Printf("Schema Complete Resources: %d\n", summary.SchemaComplete)
	
	if summary.TotalResources > 0 {
		completeness := float64(summary.SchemaComplete) / float64(summary.TotalResources) * 100
		fmt.Printf("Schema Completeness: %.1f%%\n", completeness)
	}

	if len(summary.Issues) > 0 {
		fmt.Println("\n⚠️  Issues Found:")
		for _, issue := range summary.Issues {
			fmt.Printf("   - %s\n", issue)
		}
	}
}