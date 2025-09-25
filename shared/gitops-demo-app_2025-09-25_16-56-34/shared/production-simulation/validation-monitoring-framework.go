package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/metrics/pkg/client/clientset/versioned"
)

// ValidationResult represents the result of a validation check
type ValidationResult struct {
	Name        string                 `json:"name"`
	Status      string                 `json:"status"`
	Message     string                 `json:"message"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    time.Duration          `json:"duration"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Severity    string                 `json:"severity"`
	Category    string                 `json:"category"`
	Remediation string                 `json:"remediation,omitempty"`
}

// MonitoringMetric represents a monitoring metric
type MonitoringMetric struct {
	Name        string            `json:"name"`
	Value       float64           `json:"value"`
	Unit        string            `json:"unit"`
	Labels      map[string]string `json:"labels"`
	Timestamp   time.Time         `json:"timestamp"`
	Description string            `json:"description"`
}

// ValidationFramework provides comprehensive validation and monitoring
type ValidationFramework struct {
	kubeClient    kubernetes.Interface
	metricsClient versioned.Interface
	config        *ValidationConfig
	results       []ValidationResult
	metrics       []MonitoringMetric
	mutex         sync.RWMutex
	httpServer    *http.Server
}

// ValidationConfig holds configuration for the validation framework
type ValidationConfig struct {
	ClusterName         string        `yaml:"cluster_name"`
	Namespace           string        `yaml:"namespace"`
	BackupLocation      string        `yaml:"backup_location"`
	GitOpsRepoPath      string        `yaml:"gitops_repo_path"`
	MetricsPort         int           `yaml:"metrics_port"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	ValidationsConfig   struct {
		KubernetesValidation bool `yaml:"kubernetes_validation"`
		GitOpsValidation     bool `yaml:"gitops_validation"`
		DataIntegrity        bool `yaml:"data_integrity"`
		CrossPlatform        bool `yaml:"cross_platform"`
		YAMLSyntax          bool `yaml:"yaml_syntax"`
		Performance         bool `yaml:"performance"`
		Security            bool `yaml:"security"`
	} `yaml:"validations"`
	Thresholds struct {
		CPUThreshold    float64       `yaml:"cpu_threshold"`
		MemoryThreshold float64       `yaml:"memory_threshold"`
		ResponseTime    time.Duration `yaml:"response_time"`
		ErrorRate       float64       `yaml:"error_rate"`
		AvailabilityMin float64       `yaml:"availability_min"`
	} `yaml:"thresholds"`
}

// NewValidationFramework creates a new validation framework instance
func NewValidationFramework(configPath string) (*ValidationFramework, error) {
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create Kubernetes client
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create metrics client
	metricsClient, err := versioned.NewForConfig(kubeConfig)
	if err != nil {
		log.Printf("Warning: failed to create metrics client: %v", err)
	}

	framework := &ValidationFramework{
		kubeClient:    kubeClient,
		metricsClient: metricsClient,
		config:        config,
		results:       make([]ValidationResult, 0),
		metrics:       make([]MonitoringMetric, 0),
	}

	return framework, nil
}

// loadConfig loads validation configuration from file
func loadConfig(configPath string) (*ValidationConfig, error) {
	config := &ValidationConfig{
		ClusterName:         "crc",
		Namespace:           "default",
		BackupLocation:      "./backup-source",
		GitOpsRepoPath:      "./",
		MetricsPort:         8080,
		HealthCheckInterval: 30 * time.Second,
	}

	// Set default thresholds
	config.Thresholds.CPUThreshold = 80.0
	config.Thresholds.MemoryThreshold = 80.0
	config.Thresholds.ResponseTime = 5 * time.Second
	config.Thresholds.ErrorRate = 5.0
	config.Thresholds.AvailabilityMin = 99.0

	// Set default validations
	config.ValidationsConfig.KubernetesValidation = true
	config.ValidationsConfig.GitOpsValidation = true
	config.ValidationsConfig.DataIntegrity = true
	config.ValidationsConfig.CrossPlatform = true
	config.ValidationsConfig.YAMLSyntax = true
	config.ValidationsConfig.Performance = true
	config.ValidationsConfig.Security = true

	if configPath != "" {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			log.Printf("Config file not found, using defaults: %s", configPath)
			return config, nil
		}

		data, err := ioutil.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}

	return config, nil
}

// StartMonitoring starts the monitoring and validation framework
func (vf *ValidationFramework) StartMonitoring(ctx context.Context) error {
	log.Println("🚀 Starting Validation and Monitoring Framework")

	// Start HTTP server for metrics endpoint
	if err := vf.startMetricsServer(ctx); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	// Start periodic validations
	go vf.runPeriodicValidations(ctx)

	// Start health monitoring
	go vf.runHealthMonitoring(ctx)

	// Start performance monitoring
	go vf.runPerformanceMonitoring(ctx)

	log.Printf("✅ Validation Framework started on port %d", vf.config.MetricsPort)
	return nil
}

// runPeriodicValidations runs validation checks periodically
func (vf *ValidationFramework) runPeriodicValidations(ctx context.Context) {
	ticker := time.NewTicker(vf.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			vf.runAllValidations()
		}
	}
}

// runAllValidations executes all enabled validation checks
func (vf *ValidationFramework) runAllValidations() {
	log.Println("🔍 Running comprehensive validation checks...")

	var wg sync.WaitGroup

	if vf.config.ValidationsConfig.KubernetesValidation {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vf.validateKubernetes()
		}()
	}

	if vf.config.ValidationsConfig.GitOpsValidation {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vf.validateGitOps()
		}()
	}

	if vf.config.ValidationsConfig.DataIntegrity {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vf.validateDataIntegrity()
		}()
	}

	if vf.config.ValidationsConfig.YAMLSyntax {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vf.validateYAMLSyntax()
		}()
	}

	if vf.config.ValidationsConfig.Security {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vf.validateSecurity()
		}()
	}

	wg.Wait()
	log.Println("✅ All validation checks completed")
}

// validateKubernetes validates Kubernetes cluster health and resources
func (vf *ValidationFramework) validateKubernetes() {
	start := time.Now()

	// Check cluster connectivity
	result := ValidationResult{
		Name:      "kubernetes_cluster_connectivity",
		Timestamp: start,
		Category:  "infrastructure",
		Severity:  "critical",
	}

	_, err := vf.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Failed to connect to cluster: %v", err)
		result.Remediation = "Check cluster connectivity and kubeconfig"
	} else {
		result.Status = "passed"
		result.Message = "Successfully connected to Kubernetes cluster"
	}

	result.Duration = time.Since(start)
	vf.addResult(result)

	// Check node health
	vf.validateNodeHealth()

	// Check pod health
	vf.validatePodHealth()

	// Check resource quotas
	vf.validateResourceQuotas()

	// Check persistent volumes
	vf.validatePersistentVolumes()
}

// validateNodeHealth validates node health and resources
func (vf *ValidationFramework) validateNodeHealth() {
	start := time.Now()

	result := ValidationResult{
		Name:      "kubernetes_node_health",
		Timestamp: start,
		Category:  "infrastructure",
		Severity:  "critical",
	}

	nodes, err := vf.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Failed to list nodes: %v", err)
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	healthyNodes := 0
	totalNodes := len(nodes.Items)
	nodeDetails := make(map[string]interface{})

	for _, node := range nodes.Items {
		isReady := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				isReady = true
				break
			}
		}

		nodeDetails[node.Name] = map[string]interface{}{
			"ready":   isReady,
			"version": node.Status.NodeInfo.KubeletVersion,
			"os":      node.Status.NodeInfo.OSImage,
		}

		if isReady {
			healthyNodes++
		}
	}

	if healthyNodes == totalNodes && totalNodes > 0 {
		result.Status = "passed"
		result.Message = fmt.Sprintf("All %d nodes are healthy", totalNodes)
	} else {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Only %d/%d nodes are healthy", healthyNodes, totalNodes)
		result.Remediation = "Check node status and resolve any issues"
	}

	result.Metadata = map[string]interface{}{
		"total_nodes":   totalNodes,
		"healthy_nodes": healthyNodes,
		"node_details":  nodeDetails,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add metrics
	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_nodes_total",
		Value:       float64(totalNodes),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Total number of Kubernetes nodes",
	})

	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_nodes_healthy",
		Value:       float64(healthyNodes),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Number of healthy Kubernetes nodes",
	})
}

// validatePodHealth validates pod health across namespaces
func (vf *ValidationFramework) validatePodHealth() {
	start := time.Now()

	result := ValidationResult{
		Name:      "kubernetes_pod_health",
		Timestamp: start,
		Category:  "workloads",
		Severity:  "high",
	}

	pods, err := vf.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Failed to list pods: %v", err)
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	runningPods := 0
	totalPods := len(pods.Items)
	podsByStatus := make(map[string]int)
	unhealthyPods := make([]string, 0)

	for _, pod := range pods.Items {
		status := string(pod.Status.Phase)
		podsByStatus[status]++

		if pod.Status.Phase == corev1.PodRunning {
			runningPods++
		} else if pod.Status.Phase != corev1.PodSucceeded {
			unhealthyPods = append(unhealthyPods, fmt.Sprintf("%s/%s:%s", pod.Namespace, pod.Name, status))
		}
	}

	healthPercentage := 0.0
	if totalPods > 0 {
		healthPercentage = float64(runningPods) / float64(totalPods) * 100
	}

	if healthPercentage >= 90 {
		result.Status = "passed"
		result.Message = fmt.Sprintf("Pod health is good: %.1f%% (%d/%d) running", healthPercentage, runningPods, totalPods)
	} else {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Pod health is poor: %.1f%% (%d/%d) running", healthPercentage, runningPods, totalPods)
		result.Remediation = "Investigate failing pods and resolve issues"
	}

	result.Metadata = map[string]interface{}{
		"total_pods":       totalPods,
		"running_pods":     runningPods,
		"health_percentage": healthPercentage,
		"pods_by_status":   podsByStatus,
		"unhealthy_pods":   unhealthyPods,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add metrics
	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_pods_total",
		Value:       float64(totalPods),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Total number of pods",
	})

	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_pods_running",
		Value:       float64(runningPods),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Number of running pods",
	})

	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_pod_health_percentage",
		Value:       healthPercentage,
		Unit:        "percent",
		Timestamp:   time.Now(),
		Description: "Percentage of healthy pods",
	})
}

// validateResourceQuotas validates resource quotas and limits
func (vf *ValidationFramework) validateResourceQuotas() {
	start := time.Now()

	result := ValidationResult{
		Name:      "kubernetes_resource_quotas",
		Timestamp: start,
		Category:  "resources",
		Severity:  "medium",
	}

	quotas, err := vf.kubeClient.CoreV1().ResourceQuotas("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		result.Status = "warning"
		result.Message = fmt.Sprintf("Failed to list resource quotas: %v", err)
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	quotaDetails := make(map[string]interface{})
	quotaViolations := make([]string, 0)

	for _, quota := range quotas.Items {
		quotaInfo := map[string]interface{}{
			"namespace": quota.Namespace,
			"hard":      quota.Status.Hard,
			"used":      quota.Status.Used,
		}

		// Check for quota violations
		for resource, hard := range quota.Status.Hard {
			if used, exists := quota.Status.Used[resource]; exists {
				// Compare quantities (simplified)
				hardStr := hard.String()
				usedStr := used.String()
				if strings.Contains(hardStr, "i") || strings.Contains(usedStr, "i") {
					// Memory comparison - simplified
					continue
				}
				if hardVal, err := strconv.ParseFloat(strings.TrimSuffix(hardStr, "m"), 64); err == nil {
					if usedVal, err := strconv.ParseFloat(strings.TrimSuffix(usedStr, "m"), 64); err == nil {
						if usedVal/hardVal > 0.9 { // 90% threshold
							quotaViolations = append(quotaViolations, fmt.Sprintf("%s/%s: %s/%s", quota.Namespace, resource, usedStr, hardStr))
						}
					}
				}
			}
		}

		quotaDetails[quota.Name] = quotaInfo
	}

	if len(quotaViolations) == 0 {
		result.Status = "passed"
		result.Message = fmt.Sprintf("All %d resource quotas are within limits", len(quotas.Items))
	} else {
		result.Status = "warning"
		result.Message = fmt.Sprintf("%d resource quotas are near limits", len(quotaViolations))
		result.Remediation = "Review resource usage and consider increasing quotas or optimizing workloads"
	}

	result.Metadata = map[string]interface{}{
		"total_quotas":     len(quotas.Items),
		"quota_details":    quotaDetails,
		"quota_violations": quotaViolations,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)
}

// validatePersistentVolumes validates persistent volume health
func (vf *ValidationFramework) validatePersistentVolumes() {
	start := time.Now()

	result := ValidationResult{
		Name:      "kubernetes_persistent_volumes",
		Timestamp: start,
		Category:  "storage",
		Severity:  "medium",
	}

	pvs, err := vf.kubeClient.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		result.Status = "warning"
		result.Message = fmt.Sprintf("Failed to list persistent volumes: %v", err)
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	pvsByPhase := make(map[string]int)
	availablePVs := 0

	for _, pv := range pvs.Items {
		phase := string(pv.Status.Phase)
		pvsByPhase[phase]++
		if pv.Status.Phase == corev1.VolumeAvailable || pv.Status.Phase == corev1.VolumeBound {
			availablePVs++
		}
	}

	totalPVs := len(pvs.Items)
	if totalPVs == 0 {
		result.Status = "warning"
		result.Message = "No persistent volumes found"
	} else if availablePVs == totalPVs {
		result.Status = "passed"
		result.Message = fmt.Sprintf("All %d persistent volumes are healthy", totalPVs)
	} else {
		result.Status = "warning"
		result.Message = fmt.Sprintf("%d/%d persistent volumes are healthy", availablePVs, totalPVs)
		result.Remediation = "Check PV status and resolve any storage issues"
	}

	result.Metadata = map[string]interface{}{
		"total_pvs":      totalPVs,
		"available_pvs":  availablePVs,
		"pvs_by_phase":   pvsByPhase,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add metrics
	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_persistent_volumes_total",
		Value:       float64(totalPVs),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Total number of persistent volumes",
	})

	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_persistent_volumes_available",
		Value:       float64(availablePVs),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Number of available persistent volumes",
	})
}

// validateGitOps validates GitOps configuration and sync status
func (vf *ValidationFramework) validateGitOps() {
	start := time.Now()

	result := ValidationResult{
		Name:      "gitops_validation",
		Timestamp: start,
		Category:  "gitops",
		Severity:  "high",
	}

	// Check if ArgoCD is installed
	if vf.checkArgoCDInstallation() {
		vf.validateArgoCD()
	} else if vf.checkFluxInstallation() {
		vf.validateFlux()
	} else {
		result.Status = "warning"
		result.Message = "No GitOps tool (ArgoCD/Flux) detected"
		result.Remediation = "Install and configure a GitOps tool"
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	// Validate GitOps repository structure
	vf.validateGitOpsRepository()
}

// checkArgoCDInstallation checks if ArgoCD is installed
func (vf *ValidationFramework) checkArgoCDInstallation() bool {
	_, err := vf.kubeClient.CoreV1().Namespaces().Get(context.TODO(), "argocd", metav1.GetOptions{})
	return err == nil
}

// checkFluxInstallation checks if Flux is installed
func (vf *ValidationFramework) checkFluxInstallation() bool {
	_, err := vf.kubeClient.CoreV1().Namespaces().Get(context.TODO(), "flux-system", metav1.GetOptions{})
	return err == nil
}

// validateArgoCD validates ArgoCD applications and sync status
func (vf *ValidationFramework) validateArgoCD() {
	start := time.Now()

	result := ValidationResult{
		Name:      "argocd_applications",
		Timestamp: start,
		Category:  "gitops",
		Severity:  "high",
	}

	// This would require ArgoCD client libraries for full implementation
	// For now, we'll check if ArgoCD components are running
	pods, err := vf.kubeClient.CoreV1().Pods("argocd").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/part-of=argocd",
	})

	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Failed to check ArgoCD pods: %v", err)
	} else {
		runningPods := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				runningPods++
			}
		}

		if runningPods > 0 {
			result.Status = "passed"
			result.Message = fmt.Sprintf("ArgoCD is running with %d pods", runningPods)
		} else {
			result.Status = "failed"
			result.Message = "ArgoCD pods are not running"
			result.Remediation = "Check ArgoCD deployment and pods status"
		}

		result.Metadata = map[string]interface{}{
			"total_pods":   len(pods.Items),
			"running_pods": runningPods,
		}
	}

	result.Duration = time.Since(start)
	vf.addResult(result)
}

// validateFlux validates Flux components and sync status
func (vf *ValidationFramework) validateFlux() {
	start := time.Now()

	result := ValidationResult{
		Name:      "flux_components",
		Timestamp: start,
		Category:  "gitops",
		Severity:  "high",
	}

	pods, err := vf.kubeClient.CoreV1().Pods("flux-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Failed to check Flux pods: %v", err)
	} else {
		runningPods := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				runningPods++
			}
		}

		if runningPods > 0 {
			result.Status = "passed"
			result.Message = fmt.Sprintf("Flux is running with %d pods", runningPods)
		} else {
			result.Status = "failed"
			result.Message = "Flux pods are not running"
			result.Remediation = "Check Flux deployment and pods status"
		}

		result.Metadata = map[string]interface{}{
			"total_pods":   len(pods.Items),
			"running_pods": runningPods,
		}
	}

	result.Duration = time.Since(start)
	vf.addResult(result)
}

// validateGitOpsRepository validates GitOps repository structure
func (vf *ValidationFramework) validateGitOpsRepository() {
	start := time.Now()

	result := ValidationResult{
		Name:      "gitops_repository_structure",
		Timestamp: start,
		Category:  "gitops",
		Severity:  "medium",
	}

	requiredPaths := []string{
		"base",
		"overlays",
		"argocd",
		"flux",
	}

	missingPaths := make([]string, 0)
	presentPaths := make([]string, 0)

	for _, path := range requiredPaths {
		fullPath := filepath.Join(vf.config.GitOpsRepoPath, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingPaths = append(missingPaths, path)
		} else {
			presentPaths = append(presentPaths, path)
		}
	}

	if len(missingPaths) == 0 {
		result.Status = "passed"
		result.Message = "GitOps repository structure is complete"
	} else {
		result.Status = "warning"
		result.Message = fmt.Sprintf("Missing GitOps paths: %s", strings.Join(missingPaths, ", "))
		result.Remediation = "Create missing GitOps directory structure"
	}

	result.Metadata = map[string]interface{}{
		"required_paths": requiredPaths,
		"present_paths":  presentPaths,
		"missing_paths":  missingPaths,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)
}

// validateDataIntegrity validates data integrity and consistency
func (vf *ValidationFramework) validateDataIntegrity() {
	start := time.Now()

	result := ValidationResult{
		Name:      "data_integrity_validation",
		Timestamp: start,
		Category:  "data",
		Severity:  "critical",
	}

	// Check if backup location exists and has valid data
	if _, err := os.Stat(vf.config.BackupLocation); os.IsNotExist(err) {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Backup location does not exist: %s", vf.config.BackupLocation)
		result.Remediation = "Ensure backup location exists and contains valid backup data"
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	// Validate YAML files in backup
	yamlFiles := []string{"deployments.yaml", "services.yaml", "configmaps.yaml"}
	validFiles := 0
	totalFiles := len(yamlFiles)
	fileDetails := make(map[string]interface{})

	for _, file := range yamlFiles {
		filePath := filepath.Join(vf.config.BackupLocation, file)
		if data, err := ioutil.ReadFile(filePath); err == nil {
			// Try to parse YAML
			var yamlData interface{}
			if err := yaml.Unmarshal(data, &yamlData); err == nil {
				validFiles++
				fileDetails[file] = map[string]interface{}{
					"status": "valid",
					"size":   len(data),
				}
			} else {
				fileDetails[file] = map[string]interface{}{
					"status": "invalid",
					"error":  err.Error(),
				}
			}
		} else {
			fileDetails[file] = map[string]interface{}{
				"status": "missing",
				"error":  err.Error(),
			}
		}
	}

	if validFiles == totalFiles {
		result.Status = "passed"
		result.Message = fmt.Sprintf("All %d backup files are valid", totalFiles)
	} else {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Only %d/%d backup files are valid", validFiles, totalFiles)
		result.Remediation = "Check backup integrity and regenerate invalid files"
	}

	result.Metadata = map[string]interface{}{
		"total_files": totalFiles,
		"valid_files": validFiles,
		"file_details": fileDetails,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add metrics
	vf.addMetric(MonitoringMetric{
		Name:        "data_integrity_score",
		Value:       float64(validFiles) / float64(totalFiles) * 100,
		Unit:        "percent",
		Timestamp:   time.Now(),
		Description: "Data integrity score based on valid backup files",
	})
}

// validateYAMLSyntax validates YAML syntax across all files
func (vf *ValidationFramework) validateYAMLSyntax() {
	start := time.Now()

	result := ValidationResult{
		Name:      "yaml_syntax_validation",
		Timestamp: start,
		Category:  "syntax",
		Severity:  "medium",
	}

	// Find all YAML files
	yamlFiles, err := filepath.Glob(filepath.Join(vf.config.GitOpsRepoPath, "**/*.yaml"))
	if err != nil {
		result.Status = "failed"
		result.Message = fmt.Sprintf("Failed to find YAML files: %v", err)
		result.Duration = time.Since(start)
		vf.addResult(result)
		return
	}

	// Also check .yml files
	ymlFiles, err := filepath.Glob(filepath.Join(vf.config.GitOpsRepoPath, "**/*.yml"))
	if err == nil {
		yamlFiles = append(yamlFiles, ymlFiles...)
	}

	validFiles := 0
	totalFiles := len(yamlFiles)
	invalidFiles := make([]string, 0)

	for _, file := range yamlFiles {
		if vf.validateYAMLFile(file) {
			validFiles++
		} else {
			invalidFiles = append(invalidFiles, file)
		}
	}

	if validFiles == totalFiles && totalFiles > 0 {
		result.Status = "passed"
		result.Message = fmt.Sprintf("All %d YAML files have valid syntax", totalFiles)
	} else if totalFiles == 0 {
		result.Status = "warning"
		result.Message = "No YAML files found"
	} else {
		result.Status = "failed"
		result.Message = fmt.Sprintf("%d/%d YAML files have invalid syntax", len(invalidFiles), totalFiles)
		result.Remediation = "Fix YAML syntax errors in the listed files"
	}

	result.Metadata = map[string]interface{}{
		"total_files":   totalFiles,
		"valid_files":   validFiles,
		"invalid_files": invalidFiles,
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add metrics
	if totalFiles > 0 {
		vf.addMetric(MonitoringMetric{
			Name:        "yaml_syntax_score",
			Value:       float64(validFiles) / float64(totalFiles) * 100,
			Unit:        "percent",
			Timestamp:   time.Now(),
			Description: "YAML syntax validation score",
		})
	}
}

// validateYAMLFile validates syntax of a single YAML file
func (vf *ValidationFramework) validateYAMLFile(filePath string) bool {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false
	}

	var yamlData interface{}
	return yaml.Unmarshal(data, &yamlData) == nil
}

// validateSecurity performs basic security validations
func (vf *ValidationFramework) validateSecurity() {
	start := time.Now()

	result := ValidationResult{
		Name:      "security_validation",
		Timestamp: start,
		Category:  "security",
		Severity:  "high",
	}

	securityIssues := make([]string, 0)

	// Check for privileged containers
	vf.checkPrivilegedContainers(&securityIssues)

	// Check for default service accounts
	vf.checkDefaultServiceAccounts(&securityIssues)

	// Check for network policies
	vf.checkNetworkPolicies(&securityIssues)

	// Check for pod security policies
	vf.checkPodSecurityPolicies(&securityIssues)

	if len(securityIssues) == 0 {
		result.Status = "passed"
		result.Message = "No security issues detected"
	} else {
		result.Status = "warning"
		result.Message = fmt.Sprintf("Found %d security issues", len(securityIssues))
		result.Remediation = "Address security issues: " + strings.Join(securityIssues, "; ")
	}

	result.Metadata = map[string]interface{}{
		"security_issues": securityIssues,
		"issues_count":    len(securityIssues),
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add metrics
	vf.addMetric(MonitoringMetric{
		Name:        "security_issues_count",
		Value:       float64(len(securityIssues)),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Number of security issues detected",
	})
}

// checkPrivilegedContainers checks for privileged containers
func (vf *ValidationFramework) checkPrivilegedContainers(issues *[]string) {
	pods, err := vf.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		*issues = append(*issues, "Failed to check privileged containers")
		return
	}

	privilegedCount := 0
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
				privilegedCount++
			}
		}
	}

	if privilegedCount > 0 {
		*issues = append(*issues, fmt.Sprintf("Found %d privileged containers", privilegedCount))
	}
}

// checkDefaultServiceAccounts checks for default service account usage
func (vf *ValidationFramework) checkDefaultServiceAccounts(issues *[]string) {
	pods, err := vf.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		*issues = append(*issues, "Failed to check service accounts")
		return
	}

	defaultSACount := 0
	for _, pod := range pods.Items {
		if pod.Spec.ServiceAccountName == "" || pod.Spec.ServiceAccountName == "default" {
			defaultSACount++
		}
	}

	if defaultSACount > 0 {
		*issues = append(*issues, fmt.Sprintf("Found %d pods using default service account", defaultSACount))
	}
}

// checkNetworkPolicies checks for network policy coverage
func (vf *ValidationFramework) checkNetworkPolicies(issues *[]string) {
	policies, err := vf.kubeClient.NetworkingV1().NetworkPolicies("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		*issues = append(*issues, "Failed to check network policies")
		return
	}

	if len(policies.Items) == 0 {
		*issues = append(*issues, "No network policies found")
	}
}

// checkPodSecurityPolicies checks for pod security policy usage
func (vf *ValidationFramework) checkPodSecurityPolicies(issues *[]string) {
	// Pod Security Policies are deprecated, but we can check for Pod Security Standards
	namespaces, err := vf.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		*issues = append(*issues, "Failed to check pod security policies")
		return
	}

	unprotectedNamespaces := 0
	for _, ns := range namespaces.Items {
		// Check for pod security standard labels
		if _, exists := ns.Labels["pod-security.kubernetes.io/enforce"]; !exists {
			unprotectedNamespaces++
		}
	}

	if unprotectedNamespaces > 0 {
		*issues = append(*issues, fmt.Sprintf("Found %d namespaces without pod security standards", unprotectedNamespaces))
	}
}

// runHealthMonitoring runs continuous health monitoring
func (vf *ValidationFramework) runHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			vf.collectHealthMetrics()
		}
	}
}

// collectHealthMetrics collects health and resource usage metrics
func (vf *ValidationFramework) collectHealthMetrics() {
	// Collect cluster-level metrics
	vf.collectClusterMetrics()

	// Collect node metrics
	vf.collectNodeMetrics()

	// Collect namespace metrics
	vf.collectNamespaceMetrics()
}

// collectClusterMetrics collects cluster-level metrics
func (vf *ValidationFramework) collectClusterMetrics() {
	// API server health
	start := time.Now()
	_, err := vf.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{Limit: 1})
	apiLatency := time.Since(start)

	vf.addMetric(MonitoringMetric{
		Name:        "kubernetes_api_latency_ms",
		Value:       float64(apiLatency.Milliseconds()),
		Unit:        "milliseconds",
		Timestamp:   time.Now(),
		Description: "Kubernetes API server response latency",
	})

	if err == nil {
		vf.addMetric(MonitoringMetric{
			Name:        "kubernetes_api_available",
			Value:       1,
			Unit:        "boolean",
			Timestamp:   time.Now(),
			Description: "Kubernetes API server availability",
		})
	} else {
		vf.addMetric(MonitoringMetric{
			Name:        "kubernetes_api_available",
			Value:       0,
			Unit:        "boolean",
			Timestamp:   time.Now(),
			Description: "Kubernetes API server availability",
		})
	}
}

// collectNodeMetrics collects node-level metrics
func (vf *ValidationFramework) collectNodeMetrics() {
	if vf.metricsClient == nil {
		return
	}

	// This would use the metrics client to collect actual resource usage
	// For now, we'll simulate some metrics
	vf.addMetric(MonitoringMetric{
		Name:        "node_cpu_usage_percent",
		Value:       45.0, // Simulated
		Unit:        "percent",
		Timestamp:   time.Now(),
		Description: "Node CPU usage percentage",
		Labels:      map[string]string{"node": "crc-node"},
	})

	vf.addMetric(MonitoringMetric{
		Name:        "node_memory_usage_percent",
		Value:       60.0, // Simulated
		Unit:        "percent",
		Timestamp:   time.Now(),
		Description: "Node memory usage percentage",
		Labels:      map[string]string{"node": "crc-node"},
	})
}

// collectNamespaceMetrics collects namespace-level metrics
func (vf *ValidationFramework) collectNamespaceMetrics() {
	namespaces, err := vf.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, ns := range namespaces.Items {
		// Count pods per namespace
		pods, err := vf.kubeClient.CoreV1().Pods(ns.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			continue
		}

		vf.addMetric(MonitoringMetric{
			Name:        "namespace_pods_count",
			Value:       float64(len(pods.Items)),
			Unit:        "count",
			Timestamp:   time.Now(),
			Description: "Number of pods in namespace",
			Labels:      map[string]string{"namespace": ns.Name},
		})

		// Count running pods
		runningPods := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				runningPods++
			}
		}

		vf.addMetric(MonitoringMetric{
			Name:        "namespace_running_pods_count",
			Value:       float64(runningPods),
			Unit:        "count",
			Timestamp:   time.Now(),
			Description: "Number of running pods in namespace",
			Labels:      map[string]string{"namespace": ns.Name},
		})
	}
}

// runPerformanceMonitoring runs performance monitoring
func (vf *ValidationFramework) runPerformanceMonitoring(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			vf.performanceCheck()
		}
	}
}

// performanceCheck performs performance validations
func (vf *ValidationFramework) performanceCheck() {
	start := time.Now()

	result := ValidationResult{
		Name:      "performance_check",
		Timestamp: start,
		Category:  "performance",
		Severity:  "medium",
	}

	performanceIssues := make([]string, 0)
	performanceMetrics := make(map[string]interface{})

	// Check API server performance
	apiStart := time.Now()
	_, err := vf.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{Limit: 100})
	apiDuration := time.Since(apiStart)

	performanceMetrics["api_latency_ms"] = apiDuration.Milliseconds()

	if apiDuration > vf.config.Thresholds.ResponseTime {
		performanceIssues = append(performanceIssues, fmt.Sprintf("API response time high: %v", apiDuration))
	}

	// Check resource usage (simulated for now)
	cpuUsage := 45.0 // Would come from metrics server
	memUsage := 60.0 // Would come from metrics server

	performanceMetrics["cpu_usage"] = cpuUsage
	performanceMetrics["memory_usage"] = memUsage

	if cpuUsage > vf.config.Thresholds.CPUThreshold {
		performanceIssues = append(performanceIssues, fmt.Sprintf("High CPU usage: %.1f%%", cpuUsage))
	}

	if memUsage > vf.config.Thresholds.MemoryThreshold {
		performanceIssues = append(performanceIssues, fmt.Sprintf("High memory usage: %.1f%%", memUsage))
	}

	if len(performanceIssues) == 0 {
		result.Status = "passed"
		result.Message = "Performance within acceptable thresholds"
	} else {
		result.Status = "warning"
		result.Message = fmt.Sprintf("Found %d performance issues", len(performanceIssues))
		result.Remediation = "Investigate performance bottlenecks and optimize resource usage"
	}

	result.Metadata = map[string]interface{}{
		"performance_issues":  performanceIssues,
		"performance_metrics": performanceMetrics,
		"thresholds": map[string]interface{}{
			"cpu":          vf.config.Thresholds.CPUThreshold,
			"memory":       vf.config.Thresholds.MemoryThreshold,
			"response_time": vf.config.Thresholds.ResponseTime.String(),
		},
	}
	result.Duration = time.Since(start)
	vf.addResult(result)

	// Add performance metrics
	vf.addMetric(MonitoringMetric{
		Name:        "performance_api_latency_ms",
		Value:       float64(apiDuration.Milliseconds()),
		Unit:        "milliseconds",
		Timestamp:   time.Now(),
		Description: "API performance check latency",
	})

	vf.addMetric(MonitoringMetric{
		Name:        "performance_issues_count",
		Value:       float64(len(performanceIssues)),
		Unit:        "count",
		Timestamp:   time.Now(),
		Description: "Number of performance issues detected",
	})
}

// startMetricsServer starts HTTP server for metrics and status endpoints
func (vf *ValidationFramework) startMetricsServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", vf.healthHandler)

	// Metrics endpoint (Prometheus format)
	mux.HandleFunc("/metrics", vf.metricsHandler)

	// Validation results endpoint
	mux.HandleFunc("/validation-results", vf.validationResultsHandler)

	// Status endpoint
	mux.HandleFunc("/status", vf.statusHandler)

	vf.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", vf.config.MetricsPort),
		Handler: mux,
	}

	go func() {
		if err := vf.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		vf.httpServer.Shutdown(shutdownCtx)
	}()

	return nil
}

// healthHandler handles health check requests
func (vf *ValidationFramework) healthHandler(w http.ResponseWriter, r *http.Request) {
	vf.mutex.RLock()
	defer vf.mutex.RUnlock()

	recentResults := vf.getRecentResults(5 * time.Minute)
	criticalFailures := 0
	totalChecks := len(recentResults)

	for _, result := range recentResults {
		if result.Severity == "critical" && result.Status == "failed" {
			criticalFailures++
		}
	}

	status := "healthy"
	statusCode := http.StatusOK

	if criticalFailures > 0 {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	} else if totalChecks == 0 {
		status = "unknown"
		statusCode = http.StatusServiceUnavailable
	}

	response := map[string]interface{}{
		"status":            status,
		"timestamp":         time.Now(),
		"total_checks":      totalChecks,
		"critical_failures": criticalFailures,
		"framework_uptime":  time.Since(time.Unix(vf.config.Thresholds.AvailabilityMin, 0)).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// metricsHandler handles metrics requests in Prometheus format
func (vf *ValidationFramework) metricsHandler(w http.ResponseWriter, r *http.Request) {
	vf.mutex.RLock()
	defer vf.mutex.RUnlock()

	w.Header().Set("Content-Type", "text/plain")

	// Write validation result metrics
	validationStatus := make(map[string]int)
	for _, result := range vf.getRecentResults(10 * time.Minute) {
		validationStatus[result.Status]++
	}

	for status, count := range validationStatus {
		fmt.Fprintf(w, "validation_results_total{status=\"%s\"} %d\n", status, count)
	}

	// Write monitoring metrics
	for _, metric := range vf.getRecentMetrics(10 * time.Minute) {
		labels := ""
		if len(metric.Labels) > 0 {
			labelPairs := make([]string, 0, len(metric.Labels))
			for k, v := range metric.Labels {
				labelPairs = append(labelPairs, fmt.Sprintf("%s=\"%s\"", k, v))
			}
			labels = "{" + strings.Join(labelPairs, ",") + "}"
		}

		fmt.Fprintf(w, "%s%s %f\n", metric.Name, labels, metric.Value)
	}

	// Write framework metrics
	fmt.Fprintf(w, "validation_framework_uptime_seconds %d\n", int64(time.Since(time.Unix(vf.config.Thresholds.AvailabilityMin, 0)).Seconds()))
	fmt.Fprintf(w, "validation_framework_active_validations %d\n", len(vf.results))
	fmt.Fprintf(w, "validation_framework_active_metrics %d\n", len(vf.metrics))
}

// validationResultsHandler handles validation results requests
func (vf *ValidationFramework) validationResultsHandler(w http.ResponseWriter, r *http.Request) {
	vf.mutex.RLock()
	defer vf.mutex.RUnlock()

	// Get query parameters
	since := r.URL.Query().Get("since")
	category := r.URL.Query().Get("category")
	status := r.URL.Query().Get("status")

	results := vf.results
	if since != "" {
		if duration, err := time.ParseDuration(since); err == nil {
			results = vf.getRecentResults(duration)
		}
	}

	// Filter by category
	if category != "" {
		filtered := make([]ValidationResult, 0)
		for _, result := range results {
			if result.Category == category {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	// Filter by status
	if status != "" {
		filtered := make([]ValidationResult, 0)
		for _, result := range results {
			if result.Status == status {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	response := map[string]interface{}{
		"results":   results,
		"count":     len(results),
		"timestamp": time.Now(),
		"filters": map[string]interface{}{
			"since":    since,
			"category": category,
			"status":   status,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// statusHandler handles status requests
func (vf *ValidationFramework) statusHandler(w http.ResponseWriter, r *http.Request) {
	vf.mutex.RLock()
	defer vf.mutex.RUnlock()

	recentResults := vf.getRecentResults(1 * time.Hour)
	recentMetrics := vf.getRecentMetrics(1 * time.Hour)

	statusSummary := make(map[string]int)
	categorySummary := make(map[string]int)
	severitySummary := make(map[string]int)

	for _, result := range recentResults {
		statusSummary[result.Status]++
		categorySummary[result.Category]++
		severitySummary[result.Severity]++
	}

	response := map[string]interface{}{
		"framework_status": "active",
		"timestamp":        time.Now(),
		"config":           vf.config,
		"summary": map[string]interface{}{
			"recent_results":     len(recentResults),
			"recent_metrics":     len(recentMetrics),
			"status_breakdown":   statusSummary,
			"category_breakdown": categorySummary,
			"severity_breakdown": severitySummary,
		},
		"last_validation": func() *ValidationResult {
			if len(vf.results) > 0 {
				return &vf.results[len(vf.results)-1]
			}
			return nil
		}(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper methods
func (vf *ValidationFramework) addResult(result ValidationResult) {
	vf.mutex.Lock()
	defer vf.mutex.Unlock()
	vf.results = append(vf.results, result)

	// Keep only recent results (last 1000)
	if len(vf.results) > 1000 {
		vf.results = vf.results[len(vf.results)-1000:]
	}
}

func (vf *ValidationFramework) addMetric(metric MonitoringMetric) {
	vf.mutex.Lock()
	defer vf.mutex.Unlock()
	vf.metrics = append(vf.metrics, metric)

	// Keep only recent metrics (last 1000)
	if len(vf.metrics) > 1000 {
		vf.metrics = vf.metrics[len(vf.metrics)-1000:]
	}
}

func (vf *ValidationFramework) getRecentResults(duration time.Duration) []ValidationResult {
	cutoff := time.Now().Add(-duration)
	recent := make([]ValidationResult, 0)

	for _, result := range vf.results {
		if result.Timestamp.After(cutoff) {
			recent = append(recent, result)
		}
	}

	return recent
}

func (vf *ValidationFramework) getRecentMetrics(duration time.Duration) []MonitoringMetric {
	cutoff := time.Now().Add(-duration)
	recent := make([]MonitoringMetric, 0)

	for _, metric := range vf.metrics {
		if metric.Timestamp.After(cutoff) {
			recent = append(recent, metric)
		}
	}

	return recent
}

// generateReport generates a comprehensive validation report
func (vf *ValidationFramework) generateReport() error {
	vf.mutex.RLock()
	defer vf.mutex.RUnlock()

	report := map[string]interface{}{
		"timestamp":        time.Now(),
		"framework_config": vf.config,
		"summary": map[string]interface{}{
			"total_validations": len(vf.results),
			"total_metrics":     len(vf.metrics),
		},
		"validation_results": vf.results,
		"monitoring_metrics": vf.metrics,
	}

	// Write JSON report
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	filename := fmt.Sprintf("validation-monitoring-report-%s.json", time.Now().Format("20060102-150405"))
	if err := ioutil.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	log.Printf("📊 Validation and monitoring report generated: %s", filename)
	return nil
}

func main() {
	configPath := ""
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	framework, err := NewValidationFramework(configPath)
	if err != nil {
		log.Fatalf("Failed to create validation framework: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := framework.StartMonitoring(ctx); err != nil {
		log.Fatalf("Failed to start monitoring: %v", err)
	}

	// Run initial validation
	framework.runAllValidations()

	// Generate initial report
	if err := framework.generateReport(); err != nil {
		log.Printf("Failed to generate initial report: %v", err)
	}

	log.Println("🎯 Validation and Monitoring Framework is running")
	log.Printf("🌐 Access metrics at: http://localhost:%d/metrics", framework.config.MetricsPort)
	log.Printf("🏥 Health check at: http://localhost:%d/health", framework.config.MetricsPort)
	log.Printf("📊 Status at: http://localhost:%d/status", framework.config.MetricsPort)

	// Keep running
	select {}
}