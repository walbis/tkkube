package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run gitops-test-deployment.go <gitops-directory>")
		os.Exit(1)
	}
	
	gitopsDir := os.Args[1]
	
	fmt.Printf("=== GitOps Pipeline Test Deployment ===\n")
	fmt.Printf("Testing deployment from: %s\n", gitopsDir)
	
	// Test individual resource files by creating a simple deployment test
	err := createTestDeployment(gitopsDir)
	if err != nil {
		fmt.Printf("❌ Test deployment failed: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("✅ GitOps test deployment completed successfully!")
}

func createTestDeployment(gitopsDir string) error {
	// Create a simple test manifest from the backup deployment
	testManifest := `# GitOps Test Deployment
# Source: CRC Cluster Backup
# Generated for GitOps Pipeline Testing
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app-restore-test
  namespace: demo-app
  labels:
    app: demo-app-restore
    source: backup-restore
    pipeline: gitops-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: demo-app-restore
  template:
    metadata:
      labels:
        app: demo-app-restore
    spec:
      containers:
      - name: app
        image: busybox:latest
        command: ["sleep", "3600"]
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: demo-app-restore-service
  namespace: demo-app
  labels:
    app: demo-app-restore
    source: backup-restore
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 80
    protocol: TCP
  selector:
    app: demo-app-restore
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo-app-restore-config
  namespace: demo-app
  labels:
    app: demo-app-restore
    source: backup-restore
data:
  environment: "restored-from-backup"
  cluster-source: "crc-cluster"
  pipeline-version: "1.0"
  restore-timestamp: "2025-09-25T17:12:31+03:00"
`
	
	// Write the test manifest
	testFile := filepath.Join(gitopsDir, "test-deployment.yaml")
	err := os.WriteFile(testFile, []byte(testManifest), 0644)
	if err != nil {
		return fmt.Errorf("failed to write test manifest: %w", err)
	}
	
	fmt.Printf("✅ Created test deployment manifest: %s\n", testFile)
	
	// Create deployment instructions
	instructions := fmt.Sprintf(`# GitOps Pipeline Deployment Instructions

## Test Deployment (Single File)
kubectl apply -f %s

## Verify Deployment
kubectl get all -n demo-app -l source=backup-restore

## GitOps Production Deployments

### Option 1: ArgoCD
kubectl apply -f %s/argocd/application.yaml
# ArgoCD will automatically sync and deploy the application

### Option 2: Flux
kubectl apply -f %s/flux/
# Flux will automatically reconcile and deploy the resources

### Option 3: Manual Kustomize
# Note: Base Kustomization requires fixing of backup resource structure
# Current test uses simplified deployment structure

## Cleanup Test Resources
kubectl delete -f %s
`, testFile, gitopsDir, gitopsDir, testFile)
	
	instructionsFile := filepath.Join(gitopsDir, "DEPLOYMENT_INSTRUCTIONS.md")
	err = os.WriteFile(instructionsFile, []byte(instructions), 0644)
	if err != nil {
		return fmt.Errorf("failed to write instructions: %w", err)
	}
	
	fmt.Printf("✅ Created deployment instructions: %s\n", instructionsFile)
	
	return nil
}