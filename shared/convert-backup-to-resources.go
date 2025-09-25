package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run convert-backup-to-resources.go <gitops-directory>")
		os.Exit(1)
	}
	
	gitopsDir := os.Args[1]
	baseDir := filepath.Join(gitopsDir, "base")
	
	fmt.Printf("Converting backup files in %s to individual Kubernetes resources...\n", baseDir)
	
	// Process each backup file
	files := []string{"deployments.yaml", "services.yaml", "configmaps.yaml"}
	
	for _, filename := range files {
		filePath := filepath.Join(baseDir, filename)
		err := convertFile(filePath)
		if err != nil {
			fmt.Printf("❌ Failed to convert %s: %v\n", filename, err)
		} else {
			fmt.Printf("✅ Converted %s\n", filename)
		}
	}
	
	fmt.Println("✅ All backup files converted to Kubernetes resources")
}

func convertFile(filePath string) error {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	
	content := string(data)
	
	// Split on the GitOps header
	parts := strings.Split(content, "---\n")
	if len(parts) < 2 {
		return fmt.Errorf("invalid file format")
	}
	
	header := parts[0] + "---"
	yamlContent := parts[1]
	
	// Parse the YAML list
	var resources []interface{}
	err = yaml.Unmarshal([]byte(yamlContent), &resources)
	if err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	
	// Convert to individual resources
	var newContent strings.Builder
	newContent.WriteString(header + "\n")
	
	for i, resource := range resources {
		if i > 0 {
			newContent.WriteString("---\n")
		}
		
		// Add apiVersion and kind if missing
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			continue
		}
		
		// Ensure apiVersion and kind are present
		if _, exists := resourceMap["apiVersion"]; !exists {
			// Try to infer from the file name
			if strings.Contains(filePath, "deployments") {
				resourceMap["apiVersion"] = "apps/v1"
				resourceMap["kind"] = "Deployment"
			} else if strings.Contains(filePath, "services") {
				resourceMap["apiVersion"] = "v1"
				resourceMap["kind"] = "Service"
			} else if strings.Contains(filePath, "configmaps") {
				resourceMap["apiVersion"] = "v1"
				resourceMap["kind"] = "ConfigMap"
			}
		}
		
		resourceYAML, err := yaml.Marshal(resource)
		if err != nil {
			return fmt.Errorf("failed to marshal resource %d: %w", i, err)
		}
		
		newContent.Write(resourceYAML)
	}
	
	// Write the converted file
	err = os.WriteFile(filePath, []byte(newContent.String()), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}