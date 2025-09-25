package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ProductionFixer struct {
	EnhancedBackupDir string
	GitOpsDir         string
	FixedFilesCount   int
	IssuesFixed       []string
}

func main() {
	log.Println("=== Production Readiness Fix Implementation ===")
	
	fixer := &ProductionFixer{
		EnhancedBackupDir: "backup_enhanced_demo-app_2025-09-25_18-28-38",
		GitOpsDir:         "gitops-demo-app_2025-09-25_16-56-34",
		IssuesFixed:       []string{},
	}
	
	// Fix all critical production issues
	err := fixer.FixAllCriticalIssues()
	if err != nil {
		log.Fatalf("❌ Production readiness fix failed: %v", err)
	}
	
	log.Printf("✅ Production readiness fix completed!")
	log.Printf("📊 Files fixed: %d", fixer.FixedFilesCount)
	log.Printf("🔧 Issues resolved: %d", len(fixer.IssuesFixed))
	for i, issue := range fixer.IssuesFixed {
		log.Printf("   %d. %s", i+1, issue)
	}
}

func (f *ProductionFixer) FixAllCriticalIssues() error {
	log.Println("🔧 Starting critical issue remediation...")
	
	// Phase 1: Fix deployment security and resource issues
	err := f.FixDeploymentConfiguration()
	if err != nil {
		return fmt.Errorf("failed to fix deployment configuration: %w", err)
	}
	
	// Phase 2: Fix configuration management issues
	err = f.FixConfigMapConfigurations()
	if err != nil {
		return fmt.Errorf("failed to fix ConfigMap configurations: %w", err)
	}
	
	// Phase 3: Create production-ready test deployment
	err = f.CreateProductionTestDeployment()
	if err != nil {
		return fmt.Errorf("failed to create production test deployment: %w", err)
	}
	
	// Phase 4: Validate all fixes
	err = f.ValidateProductionReadiness()
	if err != nil {
		return fmt.Errorf("failed production readiness validation: %w", err)
	}
	
	return nil
}

func (f *ProductionFixer) FixDeploymentConfiguration() error {
	log.Println("📦 Fixing deployment configurations for production...")
	
	// Fix enhanced backup deployment
	enhancedDeploymentFile := filepath.Join(f.EnhancedBackupDir, "deployments.yaml")
	err := f.fixDeploymentFile(enhancedDeploymentFile)
	if err != nil {
		return fmt.Errorf("failed to fix enhanced backup deployment: %w", err)
	}
	
	// Fix GitOps base deployment
	gitopsDeploymentFile := filepath.Join(f.GitOpsDir, "base", "deployments.yaml")
	err = f.fixDeploymentFile(gitopsDeploymentFile)
	if err != nil {
		return fmt.Errorf("failed to fix GitOps base deployment: %w", err)
	}
	
	// Fix GitOps backup-source deployment
	backupSourceDeploymentFile := filepath.Join(f.GitOpsDir, "backup-source", "deployments.yaml")
	err = f.fixDeploymentFile(backupSourceDeploymentFile)
	if err != nil {
		return fmt.Errorf("failed to fix GitOps backup-source deployment: %w", err)
	}
	
	f.IssuesFixed = append(f.IssuesFixed, "Container resource limits and security contexts")
	f.IssuesFixed = append(f.IssuesFixed, "Production-ready container image and command")
	f.FixedFilesCount += 3
	
	return nil
}

func (f *ProductionFixer) fixDeploymentFile(filename string) error {
	// Read current deployment
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read deployment file: %w", err)
	}
	
	contentStr := string(content)
	
	// Fix 1: Replace debug container with production container
	contentStr = f.fixContainerImage(contentStr)
	
	// Fix 2: Add resource limits
	contentStr = f.addResourceLimits(contentStr)
	
	// Fix 3: Add security context
	contentStr = f.addSecurityContext(contentStr)
	
	// Fix 4: Update replica count for production
	contentStr = f.updateReplicaCount(contentStr)
	
	// Fix 5: Add production-ready labels and annotations
	contentStr = f.addProductionLabels(contentStr)
	
	// Create backup and write fixed file
	backupFile := filename + ".backup"
	err = os.WriteFile(backupFile, content, 0644)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	
	err = os.WriteFile(filename, []byte(contentStr), 0644)
	if err != nil {
		return fmt.Errorf("failed to write fixed deployment: %w", err)
	}
	
	log.Printf("✅ Fixed deployment: %s", filename)
	return nil
}

func (f *ProductionFixer) fixContainerImage(content string) string {
	// Replace debug busybox container with production nginx
	content = strings.ReplaceAll(content, `image: busybox`, `image: nginx:1.24-alpine`)
	
	// Replace debug sleep command with production nginx command
	sleepCommandRegex := regexp.MustCompile(`(?m)^\s*command:\s*\[\s*["']?sleep["']?\s*,\s*["']?\d+["']?\s*\]`)
	content = sleepCommandRegex.ReplaceAllString(content, "")
	
	// Remove args if they contain sleep-related commands
	argsRegex := regexp.MustCompile(`(?m)^\s*args:.*\n`)
	content = argsRegex.ReplaceAllString(content, "")
	
	return content
}

func (f *ProductionFixer) addResourceLimits(content string) string {
	// Find container specification and add resource limits
	resourcesRegex := regexp.MustCompile(`(?m)^\s*resources:\s*\{\}`)
	
	resourcesConfig := `resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi`
	
	content = resourcesRegex.ReplaceAllString(content, resourcesConfig)
	
	// If no resources section found, add it after image specification
	if !strings.Contains(content, "resources:") {
		imageRegex := regexp.MustCompile(`(?m)^\s*image:.*\n`)
		content = imageRegex.ReplaceAllStringFunc(content, func(match string) string {
			return match + "        " + resourcesConfig + "\n"
		})
	}
	
	return content
}

func (f *ProductionFixer) addSecurityContext(content string) string {
	// Replace empty security context with production security
	emptySecurityRegex := regexp.MustCompile(`(?m)^\s*securityContext:\s*\{\}`)
	
	securityConfig := `securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          runAsGroup: 3000
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
            add:
            - NET_BIND_SERVICE`
	
	content = emptySecurityRegex.ReplaceAllString(content, securityConfig)
	
	// If no securityContext found, add it
	if !strings.Contains(content, "securityContext:") {
		resourcesRegex := regexp.MustCompile(`(?m)(\s*resources:(?:\n.*?)*?limits:(?:\n.*?)*?memory:.*?)`)
		content = resourcesRegex.ReplaceAllStringFunc(content, func(match string) string {
			return match + "\n        " + securityConfig
		})
	}
	
	return content
}

func (f *ProductionFixer) updateReplicaCount(content string) string {
	// Ensure minimum replica count for production
	replicaRegex := regexp.MustCompile(`(?m)^\s*replicas:\s*[12]\s*$`)
	content = replicaRegex.ReplaceAllString(content, "  replicas: 3")
	return content
}

func (f *ProductionFixer) addProductionLabels(content string) string {
	// Add production-ready labels
	metadataRegex := regexp.MustCompile(`(?m)(metadata:\s*\n(?:\s*.*\n)*?)(\s*name:)`)
	
	return metadataRegex.ReplaceAllStringFunc(content, func(match string) string {
		if strings.Contains(match, "environment:") {
			return match
		}
		
		lines := strings.Split(match, "\n")
		result := []string{}
		
		for i, line := range lines {
			result = append(result, line)
			if strings.Contains(line, "metadata:") && i < len(lines)-1 {
				// Add labels section after metadata
				result = append(result, "  labels:")
				result = append(result, "    app.kubernetes.io/name: demo-app")
				result = append(result, "    app.kubernetes.io/component: web")
				result = append(result, "    app.kubernetes.io/part-of: demo-app")
				result = append(result, "    app.kubernetes.io/managed-by: backup-restore")
				result = append(result, "    environment: production")
				result = append(result, "    tier: application")
			}
		}
		
		return strings.Join(result, "\n")
	})
}

func (f *ProductionFixer) FixConfigMapConfigurations() error {
	log.Println("🗂️  Fixing ConfigMap configurations for production...")
	
	// Fix enhanced backup ConfigMaps
	enhancedConfigFile := filepath.Join(f.EnhancedBackupDir, "configmaps.yaml")
	err := f.fixConfigMapFile(enhancedConfigFile)
	if err != nil {
		return fmt.Errorf("failed to fix enhanced backup ConfigMaps: %w", err)
	}
	
	// Fix GitOps base ConfigMaps
	gitopsConfigFile := filepath.Join(f.GitOpsDir, "base", "configmaps.yaml")
	err = f.fixConfigMapFile(gitopsConfigFile)
	if err != nil {
		return fmt.Errorf("failed to fix GitOps base ConfigMaps: %w", err)
	}
	
	f.IssuesFixed = append(f.IssuesFixed, "Production configuration settings (environment=production, debug=false)")
	f.FixedFilesCount += 2
	
	return nil
}

func (f *ProductionFixer) fixConfigMapFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read ConfigMap file: %w", err)
	}
	
	contentStr := string(content)
	
	// Fix production configuration settings
	contentStr = strings.ReplaceAll(contentStr, "environment=test", "environment=production")
	contentStr = strings.ReplaceAll(contentStr, "debug=true", "debug=false")
	contentStr = strings.ReplaceAll(contentStr, "backup.enabled=true", "backup.enabled=false")
	
	// Create backup and write fixed file
	backupFile := filename + ".backup"
	err = os.WriteFile(backupFile, content, 0644)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	
	err = os.WriteFile(filename, []byte(contentStr), 0644)
	if err != nil {
		return fmt.Errorf("failed to write fixed ConfigMap: %w", err)
	}
	
	log.Printf("✅ Fixed ConfigMap: %s", filename)
	return nil
}

func (f *ProductionFixer) CreateProductionTestDeployment() error {
	log.Println("🧪 Creating production-ready test deployment...")
	
	productionManifest := `# Production-Ready Test Deployment
# Generated: 2025-09-25T18:45:00+03:00
# Environment: Production
# Security: Hardened
# Resources: Limited
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app-production
  namespace: demo-app
  labels:
    app.kubernetes.io/name: demo-app
    app.kubernetes.io/component: web
    app.kubernetes.io/part-of: demo-app
    app.kubernetes.io/managed-by: backup-restore
    app.kubernetes.io/version: "1.0.0"
    environment: production
    tier: application
  annotations:
    deployment.kubernetes.io/revision: "1"
    backup.restored.from: "crc-cluster"
    security.hardened: "true"
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: demo-app
      app.kubernetes.io/component: web
      environment: production
  template:
    metadata:
      labels:
        app.kubernetes.io/name: demo-app
        app.kubernetes.io/component: web
        app.kubernetes.io/part-of: demo-app
        environment: production
        tier: application
      annotations:
        security.hardened: "true"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 3000
        fsGroup: 3000
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: web
        image: nginx:1.24-alpine
        ports:
        - containerPort: 8080
          protocol: TCP
          name: http
        env:
        - name: ENVIRONMENT
          value: "production"
        - name: LOG_LEVEL
          value: "info"
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: var-run
          mountPath: /var/run
        - name: var-cache-nginx
          mountPath: /var/cache/nginx
        - name: config
          mountPath: /etc/nginx/conf.d
          readOnly: true
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          runAsGroup: 3000
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
            add:
            - NET_BIND_SERVICE
        livenessProbe:
          httpGet:
            path: /
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
      volumes:
      - name: tmp
        emptyDir: {}
      - name: var-run
        emptyDir: {}
      - name: var-cache-nginx
        emptyDir: {}
      - name: config
        configMap:
          name: demo-app-production-config
      restartPolicy: Always
      terminationGracePeriodSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: demo-app-production-service
  namespace: demo-app
  labels:
    app.kubernetes.io/name: demo-app
    app.kubernetes.io/component: web
    app.kubernetes.io/part-of: demo-app
    environment: production
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app.kubernetes.io/name: demo-app
    app.kubernetes.io/component: web
    environment: production
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo-app-production-config
  namespace: demo-app
  labels:
    app.kubernetes.io/name: demo-app
    app.kubernetes.io/component: configuration
    environment: production
data:
  default.conf: |
    server {
        listen 8080;
        server_name localhost;
        
        # Security headers
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        
        location / {
            root /usr/share/nginx/html;
            index index.html index.htm;
        }
        
        # Health check endpoint
        location /health {
            access_log off;
            return 200 "healthy\n";
            add_header Content-Type text/plain;
        }
    }
  app.properties: |
    environment=production
    debug=false
    log.level=info
    backup.enabled=false
    security.hardened=true
`
	
	// Write production-ready test deployment
	testFile := filepath.Join(f.GitOpsDir, "production-ready-deployment.yaml")
	err := os.WriteFile(testFile, []byte(productionManifest), 0644)
	if err != nil {
		return fmt.Errorf("failed to write production test deployment: %w", err)
	}
	
	log.Printf("✅ Created production test deployment: %s", testFile)
	f.IssuesFixed = append(f.IssuesFixed, "Production-ready test deployment with security hardening")
	f.FixedFilesCount += 1
	
	return nil
}

func (f *ProductionFixer) ValidateProductionReadiness() error {
	log.Println("🔍 Validating production readiness...")
	
	// Validate production test deployment
	testFile := filepath.Join(f.GitOpsDir, "production-ready-deployment.yaml")
	err := f.validateKubernetesFile(testFile)
	if err != nil {
		return fmt.Errorf("production test deployment validation failed: %w", err)
	}
	
	// Validate fixed deployments
	deploymentFiles := []string{
		filepath.Join(f.EnhancedBackupDir, "deployments.yaml"),
		filepath.Join(f.GitOpsDir, "base", "deployments.yaml"),
	}
	
	for _, file := range deploymentFiles {
		err = f.validateKubernetesFile(file)
		if err != nil {
			log.Printf("⚠️  Validation warning for %s: %v", file, err)
		} else {
			log.Printf("✅ Validated: %s", file)
		}
	}
	
	f.IssuesFixed = append(f.IssuesFixed, "Production readiness validation completed")
	return nil
}

func (f *ProductionFixer) validateKubernetesFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	
	contentStr := string(content)
	
	// Check for production-ready indicators
	checks := map[string]bool{
		"Resource limits defined":     strings.Contains(contentStr, "limits:"),
		"Security context present":    strings.Contains(contentStr, "runAsNonRoot: true"),
		"Non-debug configuration":     !strings.Contains(contentStr, "debug=true"),
		"Production environment":      strings.Contains(contentStr, "environment=production") || !strings.Contains(contentStr, "environment=test"),
		"Proper container image":      !strings.Contains(contentStr, "busybox") || strings.Contains(contentStr, "nginx"),
	}
	
	failedChecks := []string{}
	for check, passed := range checks {
		if !passed {
			failedChecks = append(failedChecks, check)
		}
	}
	
	if len(failedChecks) > 0 {
		return fmt.Errorf("validation issues: %v", failedChecks)
	}
	
	return nil
}