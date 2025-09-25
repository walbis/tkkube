package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	log.Println("=== GitOps ConfigMaps Metadata Fix ===")
	
	// Source: Enhanced backup with proper schema (working)
	sourceFile := "backup_enhanced_demo-app_2025-09-25_18-28-38/configmaps.yaml"
	
	// Target: GitOps artifacts that need fixing
	targetFiles := []string{
		"gitops-demo-app_2025-09-25_16-56-34/base/configmaps.yaml",
		"gitops-demo-app_2025-09-25_16-56-34/backup-source/configmaps.yaml",
	}
	
	// Check if source file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		log.Fatalf("❌ Source file not found: %s", sourceFile)
	}
	
	log.Printf("📂 Source file: %s", sourceFile)
	
	// Fix each target file
	for _, targetFile := range targetFiles {
		err := fixConfigMapFile(sourceFile, targetFile)
		if err != nil {
			log.Printf("❌ Failed to fix %s: %v", targetFile, err)
		} else {
			log.Printf("✅ Fixed ConfigMaps in %s", targetFile)
		}
	}
	
	// Validate the fixes
	log.Println("\n🔍 Validating fixes...")
	for _, targetFile := range targetFiles {
		err := validateConfigMapFile(targetFile)
		if err != nil {
			log.Printf("❌ Validation failed for %s: %v", targetFile, err)
		} else {
			log.Printf("✅ Validation passed for %s", targetFile)
		}
	}
	
	log.Println("\n✅ GitOps ConfigMaps metadata fix completed!")
	log.Println("🚀 Kustomize builds should now work properly")
}

func fixConfigMapFile(sourceFile, targetFile string) error {
	// Read source content (enhanced backup with proper schema)
	sourceContent, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	
	// Create target directory if it doesn't exist
	targetDir := filepath.Dir(targetFile)
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}
	
	// Create GitOps header based on target file type
	var header string
	if filepath.Base(targetFile) == "configmaps.yaml" && filepath.Dir(targetFile) == "gitops-demo-app_2025-09-25_16-56-34/base" {
		header = `# GitOps Base ConfigMaps - Schema Fixed
# Source: Enhanced backup with proper metadata
# Cluster: crc-cluster
# Namespace: demo-app
# Pipeline: backup-to-gitops (fixed)
# Schema Status: COMPLETE (metadata.name fields added)
# Generated: 2025-09-25T18:35:00+03:00
---
`
	} else {
		header = `# GitOps Managed Resource - Schema Fixed
# Source: backup_demo-app_2025-09-25_16-56-34 (enhanced)
# Cluster: crc-cluster
# Namespace: demo-app
# Pipeline: backup-to-gitops (fixed)
# Schema Status: COMPLETE (metadata.name fields added) 
# Generated: 2025-09-25T18:35:00+03:00
---
`
	}
	
	// Extract just the YAML content from source (skip the enhanced backup header)
	content := string(sourceContent)
	yamlStartIndex := findFirstYAMLDocument(content)
	if yamlStartIndex == -1 {
		return fmt.Errorf("no YAML content found in source file")
	}
	
	yamlContent := content[yamlStartIndex:]
	finalContent := header + yamlContent
	
	// Write to target file
	err = os.WriteFile(targetFile, []byte(finalContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write target file: %w", err)
	}
	
	return nil
}

func findFirstYAMLDocument(content string) int {
	// Find the first "apiVersion:" line which indicates start of YAML content
	lines := splitLines(content)
	for i, line := range lines {
		if len(line) > 0 && (line[0] != '#') && (line == "---" || startsWith(line, "apiVersion:")) {
			// Calculate byte position
			pos := 0
			for j := 0; j < i; j++ {
				pos += len(lines[j]) + 1 // +1 for newline
			}
			return pos
		}
	}
	return -1
}

func splitLines(content string) []string {
	var lines []string
	var currentLine string
	
	for _, char := range content {
		if char == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
		} else {
			currentLine += string(char)
		}
	}
	
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	return lines
}

func startsWith(str, prefix string) bool {
	return len(str) >= len(prefix) && str[:len(prefix)] == prefix
}

func validateConfigMapFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	
	contentStr := string(content)
	
	// Check for required elements
	requiredElements := []string{
		"apiVersion: v1",
		"kind: ConfigMap", 
		"metadata:",
		"name:", // This is the critical fix
		"data:",
	}
	
	for _, element := range requiredElements {
		if !contains(contentStr, element) {
			return fmt.Errorf("missing required element: %s", element)
		}
	}
	
	// Count ConfigMaps by counting "apiVersion: v1" + "kind: ConfigMap" pairs
	apiVersionCount := countOccurrences(contentStr, "apiVersion: v1")
	kindConfigMapCount := countOccurrences(contentStr, "kind: ConfigMap")
	
	if apiVersionCount != kindConfigMapCount {
		return fmt.Errorf("mismatch between apiVersion and kind counts: %d vs %d", apiVersionCount, kindConfigMapCount)
	}
	
	if apiVersionCount == 0 {
		return fmt.Errorf("no ConfigMap resources found")
	}
	
	log.Printf("📋 Found %d ConfigMap resources in %s", apiVersionCount, filename)
	return nil
}

func contains(str, substr string) bool {
	return len(str) >= len(substr) && findSubstring(str, substr) >= 0
}

func findSubstring(str, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(substr) > len(str) {
		return -1
	}
	
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func countOccurrences(str, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	
	count := 0
	start := 0
	
	for {
		pos := findSubstring(str[start:], substr)
		if pos == -1 {
			break
		}
		count++
		start += pos + len(substr)
	}
	
	return count
}