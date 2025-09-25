package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"encoding/json"
	"gopkg.in/yaml.v3"
)

type GitOpsConfig struct {
	BackupDirectory  string            `yaml:"backup_directory"`
	GitOpsDirectory  string            `yaml:"gitops_directory"`
	ClusterName      string            `yaml:"cluster_name"`
	Namespace        string            `yaml:"namespace"`
	AppName          string            `yaml:"app_name"`
	Labels           map[string]string `yaml:"labels"`
	Timestamp        string            `yaml:"timestamp"`
}

type KustomizationFile struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   map[string]string `yaml:"metadata"`
	Resources  []string          `yaml:"resources"`
	Images     []ImageConfig     `yaml:"images,omitempty"`
	ConfigMapGenerator []ConfigMapGenerator `yaml:"configMapGenerator,omitempty"`
}

type ImageConfig struct {
	Name    string `yaml:"name"`
	NewTag  string `yaml:"newTag"`
}

type ConfigMapGenerator struct {
	Name  string   `yaml:"name"`
	Files []string `yaml:"files"`
}

type ArgoApplication struct {
	APIVersion string                  `yaml:"apiVersion"`
	Kind       string                  `yaml:"kind"`
	Metadata   ApplicationMetadata     `yaml:"metadata"`
	Spec       ApplicationSpec         `yaml:"spec"`
}

type ApplicationMetadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
}

type ApplicationSpec struct {
	Project     string                  `yaml:"project"`
	Source      ApplicationSource       `yaml:"source"`
	Destination ApplicationDestination  `yaml:"destination"`
	SyncPolicy  ApplicationSyncPolicy   `yaml:"syncPolicy"`
}

type ApplicationSource struct {
	RepoURL        string `yaml:"repoURL"`
	Path           string `yaml:"path"`
	TargetRevision string `yaml:"targetRevision"`
}

type ApplicationDestination struct {
	Server    string `yaml:"server"`
	Namespace string `yaml:"namespace"`
}

type ApplicationSyncPolicy struct {
	Automated ApplicationAutomated `yaml:"automated"`
	SyncOptions []string           `yaml:"syncOptions"`
}

type ApplicationAutomated struct {
	Prune    bool `yaml:"prune"`
	SelfHeal bool `yaml:"selfHeal"`
}

func main() {
	fmt.Println("=== Backup-to-GitOps Pipeline Orchestrator ===")
	
	// Find the existing backup directory
	backupDir := findLatestBackupDirectory()
	if backupDir == "" {
		fmt.Println("❌ No backup directory found")
		os.Exit(1)
	}
	
	fmt.Printf("📁 Using backup directory: %s\n", backupDir)
	
	config := GitOpsConfig{
		BackupDirectory: backupDir,
		GitOpsDirectory: fmt.Sprintf("gitops-%s", strings.Replace(backupDir, "backup_", "", 1)),
		ClusterName:     "crc-cluster", 
		Namespace:       "demo-app",
		AppName:         "demo-app-restore",
		Labels: map[string]string{
			"app.kubernetes.io/name":       "demo-app",
			"app.kubernetes.io/component":  "backup-restore",
			"app.kubernetes.io/managed-by": "gitops-pipeline",
			"backup.source.cluster":        "crc-cluster",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	
	err := executeGitOpsPipeline(config)
	if err != nil {
		fmt.Printf("❌ GitOps pipeline failed: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("✅ Backup-to-GitOps pipeline completed successfully!")
}

func executeGitOpsPipeline(config GitOpsConfig) error {
	fmt.Println("🚀 Executing Backup-to-GitOps Pipeline...")
	
	// Create GitOps directory structure
	err := createGitOpsStructure(config)
	if err != nil {
		return fmt.Errorf("failed to create GitOps structure: %w", err)
	}
	
	// Generate Kustomization files
	err = generateKustomizationFiles(config)
	if err != nil {
		return fmt.Errorf("failed to generate Kustomization files: %w", err)
	}
	
	// Copy and organize backup files
	err = organizeBackupFiles(config)
	if err != nil {
		return fmt.Errorf("failed to organize backup files: %w", err)
	}
	
	// Generate ArgoCD Application manifest
	err = generateArgoApplication(config)
	if err != nil {
		return fmt.Errorf("failed to generate ArgoCD application: %w", err)
	}
	
	// Generate Flux HelmRelease (alternative)
	err = generateFluxManifests(config)
	if err != nil {
		return fmt.Errorf("failed to generate Flux manifests: %w", err)
	}
	
	// Create pipeline summary
	err = createPipelineSummary(config)
	if err != nil {
		return fmt.Errorf("failed to create pipeline summary: %w", err)
	}
	
	return nil
}

func createGitOpsStructure(config GitOpsConfig) error {
	fmt.Println("📂 Creating GitOps directory structure...")
	
	directories := []string{
		config.GitOpsDirectory,
		filepath.Join(config.GitOpsDirectory, "base"),
		filepath.Join(config.GitOpsDirectory, "overlays"),
		filepath.Join(config.GitOpsDirectory, "overlays", "development"),
		filepath.Join(config.GitOpsDirectory, "overlays", "staging"),
		filepath.Join(config.GitOpsDirectory, "overlays", "production"),
		filepath.Join(config.GitOpsDirectory, "argocd"),
		filepath.Join(config.GitOpsDirectory, "flux"),
		filepath.Join(config.GitOpsDirectory, "backup-source"),
	}
	
	for _, dir := range directories {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		fmt.Printf("  ✅ Created: %s\n", dir)
	}
	
	return nil
}

func generateKustomizationFiles(config GitOpsConfig) error {
	fmt.Println("📝 Generating Kustomization files...")
	
	// Base Kustomization
	baseKustomization := KustomizationFile{
		APIVersion: "kustomize.config.k8s.io/v1beta1",
		Kind:       "Kustomization",
		Metadata: map[string]string{
			"name":      fmt.Sprintf("%s-base", config.AppName),
			"namespace": config.Namespace,
		},
		Resources: []string{
			"../backup-source/deployments.yaml",
			"../backup-source/services.yaml",
			"../backup-source/configmaps.yaml",
		},
		Images: []ImageConfig{
			{
				Name:   "busybox",
				NewTag: "latest",
			},
		},
	}
	
	err := writeYAMLFile(filepath.Join(config.GitOpsDirectory, "base", "kustomization.yaml"), baseKustomization)
	if err != nil {
		return fmt.Errorf("failed to write base kustomization: %w", err)
	}
	
	// Overlay Kustomizations for each environment
	environments := []string{"development", "staging", "production"}
	replicas := map[string]int{"development": 1, "staging": 2, "production": 3}
	
	for _, env := range environments {
		overlayKustomization := KustomizationFile{
			APIVersion: "kustomize.config.k8s.io/v1beta1",
			Kind:       "Kustomization",
			Metadata: map[string]string{
				"name":      fmt.Sprintf("%s-%s", config.AppName, env),
				"namespace": config.Namespace,
			},
			Resources: []string{
				"../../base",
			},
		}
		
		// Create patches for environment-specific configurations
		overlayDir := filepath.Join(config.GitOpsDirectory, "overlays", env)
		err := writeYAMLFile(filepath.Join(overlayDir, "kustomization.yaml"), overlayKustomization)
		if err != nil {
			return fmt.Errorf("failed to write %s overlay kustomization: %w", env, err)
		}
		
		// Create replica patch
		replicaPatch := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
spec:
  replicas: %d
---
# Environment: %s
# Restored from backup: %s
`, replicas[env], env, config.BackupDirectory)
		
		err = os.WriteFile(filepath.Join(overlayDir, "replica-patch.yaml"), []byte(replicaPatch), 0644)
		if err != nil {
			return fmt.Errorf("failed to write replica patch for %s: %w", env, err)
		}
		
		fmt.Printf("  ✅ Generated %s overlay\n", env)
	}
	
	fmt.Println("  ✅ All Kustomization files generated")
	return nil
}

func organizeBackupFiles(config GitOpsConfig) error {
	fmt.Println("📋 Organizing backup files for GitOps...")
	
	backupSourceDir := filepath.Join(config.GitOpsDirectory, "backup-source")
	
	// List files in backup directory
	files, err := os.ReadDir(config.BackupDirectory)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}
	
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		
		srcFile := filepath.Join(config.BackupDirectory, file.Name())
		dstFile := filepath.Join(backupSourceDir, file.Name())
		
		// Copy file
		data, err := os.ReadFile(srcFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", srcFile, err)
		}
		
		// Add GitOps metadata comment header
		gitopsHeader := fmt.Sprintf(`# GitOps Managed Resource
# Source: %s
# Cluster: %s
# Namespace: %s
# Pipeline: backup-to-gitops
# Generated: %s
---
`, config.BackupDirectory, config.ClusterName, config.Namespace, config.Timestamp)
		
		gitopsData := gitopsHeader + string(data)
		
		err = os.WriteFile(dstFile, []byte(gitopsData), 0644)
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", dstFile, err)
		}
		
		fmt.Printf("  ✅ Organized: %s → %s\n", file.Name(), "backup-source/"+file.Name())
	}
	
	return nil
}

func generateArgoApplication(config GitOpsConfig) error {
	fmt.Println("🔄 Generating ArgoCD Application manifest...")
	
	app := ArgoApplication{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "Application",
		Metadata: ApplicationMetadata{
			Name:      config.AppName,
			Namespace: "argocd",
			Labels:    config.Labels,
		},
		Spec: ApplicationSpec{
			Project: "default",
			Source: ApplicationSource{
				RepoURL:        "https://github.com/your-org/gitops-repo.git",
				Path:           fmt.Sprintf("%s/overlays/development", config.GitOpsDirectory),
				TargetRevision: "HEAD",
			},
			Destination: ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: config.Namespace,
			},
			SyncPolicy: ApplicationSyncPolicy{
				Automated: ApplicationAutomated{
					Prune:    true,
					SelfHeal: true,
				},
				SyncOptions: []string{
					"CreateNamespace=true",
					"PruneLast=true",
					"ApplyOutOfSyncOnly=true",
				},
			},
		},
	}
	
	err := writeYAMLFile(filepath.Join(config.GitOpsDirectory, "argocd", "application.yaml"), app)
	if err != nil {
		return fmt.Errorf("failed to write ArgoCD application: %w", err)
	}
	
	fmt.Println("  ✅ ArgoCD Application manifest generated")
	return nil
}

func generateFluxManifests(config GitOpsConfig) error {
	fmt.Println("🌊 Generating Flux manifests...")
	
	// Flux GitRepository
	gitRepo := map[string]interface{}{
		"apiVersion": "source.toolkit.fluxcd.io/v1beta2",
		"kind":       "GitRepository",
		"metadata": map[string]interface{}{
			"name":      fmt.Sprintf("%s-source", config.AppName),
			"namespace": "flux-system",
			"labels":    config.Labels,
		},
		"spec": map[string]interface{}{
			"interval": "1m",
			"ref": map[string]string{
				"branch": "main",
			},
			"url": "https://github.com/your-org/gitops-repo.git",
		},
	}
	
	// Flux Kustomization
	kustomization := map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1beta2",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      config.AppName,
			"namespace": "flux-system",
			"labels":    config.Labels,
		},
		"spec": map[string]interface{}{
			"interval": "5m",
			"sourceRef": map[string]string{
				"kind": "GitRepository",
				"name": fmt.Sprintf("%s-source", config.AppName),
			},
			"path":  fmt.Sprintf("./%s/overlays/development", config.GitOpsDirectory),
			"prune": true,
			"targetNamespace": config.Namespace,
		},
	}
	
	err := writeYAMLFile(filepath.Join(config.GitOpsDirectory, "flux", "gitrepository.yaml"), gitRepo)
	if err != nil {
		return fmt.Errorf("failed to write Flux GitRepository: %w", err)
	}
	
	err = writeYAMLFile(filepath.Join(config.GitOpsDirectory, "flux", "kustomization.yaml"), kustomization)
	if err != nil {
		return fmt.Errorf("failed to write Flux Kustomization: %w", err)
	}
	
	fmt.Println("  ✅ Flux manifests generated")
	return nil
}

func createPipelineSummary(config GitOpsConfig) error {
	fmt.Println("📊 Creating pipeline execution summary...")
	
	summary := map[string]interface{}{
		"pipeline_execution": map[string]interface{}{
			"timestamp":         config.Timestamp,
			"status":           "SUCCESS",
			"backup_source":    config.BackupDirectory,
			"gitops_output":    config.GitOpsDirectory,
			"cluster":          config.ClusterName,
			"namespace":        config.Namespace,
			"application_name": config.AppName,
		},
		"artifacts_generated": map[string]interface{}{
			"base_kustomization":      "base/kustomization.yaml",
			"environment_overlays":    []string{"development", "staging", "production"},
			"argocd_application":     "argocd/application.yaml",
			"flux_manifests":         []string{"flux/gitrepository.yaml", "flux/kustomization.yaml"},
			"backup_sources":         []string{"deployments.yaml", "services.yaml", "configmaps.yaml"},
		},
		"deployment_instructions": map[string]interface{}{
			"argocd": map[string]string{
				"step1": "kubectl apply -f argocd/application.yaml",
				"step2": "ArgoCD will automatically sync the application",
			},
			"flux": map[string]interface{}{
				"step1": "kubectl apply -f flux/",
				"step2": "Flux will automatically reconcile the resources",
			},
			"manual": map[string]string{
				"step1": "kubectl apply -k overlays/development",
				"step2": "Verify deployment: kubectl get all -n demo-app",
			},
		},
	}
	
	summaryData, err := yaml.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}
	
	err = os.WriteFile(filepath.Join(config.GitOpsDirectory, "pipeline-summary.yaml"), summaryData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write pipeline summary: %w", err)
	}
	
	// Also create JSON version
	jsonData, _ := json.MarshalIndent(summary, "", "  ")
	err = os.WriteFile(filepath.Join(config.GitOpsDirectory, "pipeline-summary.json"), jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write JSON summary: %w", err)
	}
	
	fmt.Println("  ✅ Pipeline summary created")
	return nil
}

func findLatestBackupDirectory() string {
	files, err := os.ReadDir(".")
	if err != nil {
		return ""
	}
	
	for _, file := range files {
		if file.IsDir() && strings.HasPrefix(file.Name(), "backup_demo-app_") {
			return file.Name()
		}
	}
	
	return ""
}

func writeYAMLFile(filename string, data interface{}) error {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, yamlData, 0644)
}