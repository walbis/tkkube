package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	ClusterDomain     string
	ClusterName       string
	MinIOEndpoint     string
	MinIOAccessKey    string
	MinIOSecretKey    string
	MinIOBucket       string
	MinIOUseSSL       bool
	BatchSize         int
	RetryAttempts     int
	RetryDelay        time.Duration
	// Cleanup configuration
	EnableCleanup     bool
	RetentionDays     int
	CleanupOnStartup  bool
	// Advanced bucket management
	AutoCreateBucket  bool
	FallbackBuckets   []string
	BucketRetryAttempts int
	BucketRetryDelay    time.Duration
}

// Dynamic Priority System
type ResourcePriorityConfig struct {
	CoreResources         map[string]int            `yaml:"core_resources"`
	RBACResources         map[string]int            `yaml:"rbac_resources"`
	NetworkResources      map[string]int            `yaml:"network_resources"`
	WorkloadResources     map[string]int            `yaml:"workload_resources"`
	OpenShiftCore         map[string]int            `yaml:"openshift_core"`
	OpenShiftSecurity     map[string]int            `yaml:"openshift_security"`
	StorageResources      map[string]int            `yaml:"storage_resources"`
	CustomResources       map[string]int            `yaml:"custom_resources"`
	SpecialHandling       SpecialHandlingConfig     `yaml:"special_handling"`
	DynamicRules          DynamicRulesConfig        `yaml:"dynamic_rules"`
	BackupConfig          BackupBehaviorConfig      `yaml:"backup_config"`
}

type SpecialHandlingConfig struct {
	Events            EventsConfig              `yaml:"events"`
	Exclude           []string                  `yaml:"exclude"`
	NamespaceOverrides map[string]NamespaceOverride `yaml:"namespace_overrides"`
}

type EventsConfig struct {
	Priority        int `yaml:"priority"`
	RetentionHours  int `yaml:"retention_hours"`
}

type NamespaceOverride struct {
	PriorityBoost int `yaml:"priority_boost"`
}

type DynamicRulesConfig struct {
	LabelPriorities map[string]int `yaml:"label_priorities"`
	SizeRules       SizeRulesConfig `yaml:"size_rules"`
}

type SizeRulesConfig struct {
	LargeResources LargeResourceConfig `yaml:"large_resources"`
}

type LargeResourceConfig struct {
	SizeThreshold   string `yaml:"size_threshold"`
	PriorityPenalty int    `yaml:"priority_penalty"`
}

type BackupBehaviorConfig struct {
	MaxConcurrentPerType int                    `yaml:"max_concurrent_per_type"`
	RetryConfig          map[string]RetryConfig `yaml:"retry_config"`
}

type RetryConfig struct {
	MaxAttempts  int    `yaml:"max_attempts"`
	InitialDelay string `yaml:"initial_delay"`
	MaxDelay     string `yaml:"max_delay"`
}

// Priority Manager
type PriorityManager struct {
	config      *ResourcePriorityConfig
	lock        sync.RWMutex
	lastUpdate  time.Time
	configMap   string
	namespace   string
	clientset   kubernetes.Interface
}

type BackupConfig struct {
	// FilteringMode removed - hardcoded to "whitelist" mode
	IncludeResources        []string
	IncludeNamespaces       []string
	// IncludeCRDs removed - now fully dynamic
	LabelSelector           string
	AnnotationSelector      string
	MaxResourceSize         string
	FollowOwnerReferences   bool
	IncludeManagedFields    bool
	IncludeStatus           bool
	OpenShiftMode           string
	IncludeOpenShiftRes     bool
	ValidateYAML            bool
	SkipInvalidResources    bool
	// Cleanup configuration
	EnableCleanup           bool
	RetentionDays           int
	CleanupOnStartup        bool
}

type ClusterBackup struct {
	config       *Config
	backupConfig *BackupConfig
	minioClient  *minio.Client
	kubeClient   kubernetes.Interface
	dynamicClient dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	metrics      *BackupMetrics
	ctx          context.Context
	logger       *StructuredLogger
	priorityManager *PriorityManager
	// Circuit breakers for different operations
	minioCircuitBreaker *CircuitBreaker
	apiCircuitBreaker   *CircuitBreaker
	// Cache for API discovery results
	apiResourcesCache []*metav1.APIResourceList
	cacheTimestamp    time.Time
	// Cache for OpenShift detection result
	openShiftDetected *string
	openShiftCacheTime time.Time
}

type StructuredLogger struct {
	clusterName string
	logLevel    string
}

type LogEntry struct {
	Timestamp   string      `json:"timestamp"`
	Level       string      `json:"level"`
	Component   string      `json:"component"`
	Cluster     string      `json:"cluster"`
	Namespace   string      `json:"namespace,omitempty"`
	Resource    string      `json:"resource,omitempty"`
	Operation   string      `json:"operation"`
	Message     string      `json:"message"`
	Data        interface{} `json:"data,omitempty"`
	Error       string      `json:"error,omitempty"`
	Duration    float64     `json:"duration_ms,omitempty"`
}

type BackupMetrics struct {
	BackupDuration    prometheus.Histogram
	BackupErrors      prometheus.Counter
	ResourcesBackedUp prometheus.Counter
	LastBackupTime    prometheus.Gauge
	NamespacesBackedUp prometheus.Gauge
}

var (
	// Removed: defaultSystemNamespaces - using whitelist only
)

func main() {
	logger := NewStructuredLogger("backup", getSecretValue("CLUSTER_NAME", "default"))
	logger.Info("startup", "Starting Enhanced OpenShift Cluster Backup...", nil)

	// Check if it's a health check request
	if len(os.Args) > 1 && os.Args[1] == "--health-check" {
		fmt.Println("OK")
		os.Exit(0)
	}

	// Create main context with timeout (30 minutes max for backup operation)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	config, err := loadConfig(ctx)
	if err != nil {
		logger.Fatal("config_load", "Failed to load configuration", map[string]interface{}{"error": err.Error()})
	}

	backupConfig, err := loadBackupConfig()
	if err != nil {
		logger.Fatal("backup_config_load", "Failed to load backup configuration", map[string]interface{}{"error": err.Error()})
	}

	backup, err := NewClusterBackup(config, backupConfig, logger, ctx)
	if err != nil {
		logger.Fatal("backup_client_init", "Failed to create backup client", map[string]interface{}{"error": err.Error()})
	}

	logger.Info("config_loaded", "Configuration loaded successfully", map[string]interface{}{
		"cluster_name": config.ClusterName,
		"filtering_mode": "whitelist",
		"openshift_mode": backupConfig.OpenShiftMode,
		"minio_bucket": config.MinIOBucket,
	})

	// Start metrics server in a goroutine with error handling
	metricsErrChan := make(chan error, 1)
	go func() {
		if err := startMetricsServer(); err != nil {
			metricsErrChan <- err
		}
	}()
	
	// Check for metrics server startup errors (non-blocking)
	select {
	case err := <-metricsErrChan:
		logger.Error("metrics_server_failed", "Metrics server failed to start", map[string]interface{}{"error": err.Error()})
		// Continue execution - metrics failure shouldn't stop backup
	case <-time.After(2 * time.Second):
		// Metrics server started successfully or is starting
		logger.Info("metrics_server_started", "Metrics server started successfully", nil)
	}

	// Perform cleanup on startup if configured
	if backup.shouldCleanupOnStartup() {
		logger.Info("cleanup_startup", "Performing cleanup on startup", nil)
		if err := backup.performCleanup(); err != nil {
			logger.Error("cleanup_startup_failed", "Startup cleanup failed", map[string]interface{}{"error": err.Error()})
		}
	}

	if err := backup.Run(); err != nil {
		logger.Fatal("backup_run", "Backup failed", map[string]interface{}{"error": err.Error()})
	}

	// Perform cleanup after backup if configured
	if backup.backupConfig.EnableCleanup && !backup.backupConfig.CleanupOnStartup {
		logger.Info("cleanup_post_backup", "Performing cleanup after backup", nil)
		if err := backup.performCleanup(); err != nil {
			logger.Error("cleanup_post_backup_failed", "Post-backup cleanup failed", map[string]interface{}{"error": err.Error()})
		}
	}

	logger.Info("backup_complete", "Backup completed successfully", nil)
}

// detectClusterName attempts to dynamically detect the cluster name from Kubernetes API
func detectClusterName(ctx context.Context) string {
	log.Printf("=== CLUSTER NAME DETECTION START ===")
	
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get in-cluster config: %v", err)
		return "unknown-cluster"
	}
	log.Printf("✓ In-cluster config obtained successfully")
	
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset: %v", err)
		return "unknown-cluster"
	}
	log.Printf("✓ Kubernetes clientset created successfully")
	
	// Try to get cluster name from multiple sources
	// 1. Try OpenShift Infrastructure (if available)
	// 2. Try kube-system namespace labels
	// 3. Try nodes with cluster labels
	// 4. Fallback to hostname-based detection
	
	log.Printf("Step 1: Trying OpenShift Infrastructure detection...")
	if clusterName := detectOpenShiftClusterName(ctx, clientset); clusterName != "" {
		log.Printf("✓ SUCCESS: OpenShift Infrastructure detection returned: '%s'", clusterName)
		return clusterName
	}
	log.Printf("✗ OpenShift Infrastructure detection failed or empty")
	
	log.Printf("Step 2: Trying kube-system namespace labels...")
	if clusterName := detectFromNamespaceLabels(ctx, clientset); clusterName != "" {
		log.Printf("✓ SUCCESS: Namespace labels detection returned: '%s'", clusterName)
		return clusterName
	}
	log.Printf("✗ Namespace labels detection failed or empty")
	
	log.Printf("Step 3: Trying node labels...")
	if clusterName := detectFromNodeLabels(ctx, clientset); clusterName != "" {
		log.Printf("✓ SUCCESS: Node labels detection returned: '%s'", clusterName)
		return clusterName
	}
	log.Printf("✗ Node labels detection failed or empty")
	
	log.Printf("Step 4: Using hostname fallback...")
	fallbackName := detectFromHostname()
	log.Printf("✓ Hostname fallback returned: '%s'", fallbackName)
	log.Printf("=== CLUSTER NAME DETECTION END ===")
	
	return fallbackName
}

// detectOpenShiftClusterName tries to get cluster name from OpenShift Infrastructure
func detectOpenShiftClusterName(ctx context.Context, clientset *kubernetes.Clientset) string {
	log.Printf("  → OpenShift Infrastructure: Getting in-cluster config...")
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("  → OpenShift Infrastructure: Failed to get config: %v", err)
		return ""
	}
	log.Printf("  → OpenShift Infrastructure: Config obtained, creating dynamic client...")
	
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Printf("  → OpenShift Infrastructure: Failed to create dynamic client: %v", err)
		return ""
	}
	log.Printf("  → OpenShift Infrastructure: Dynamic client created successfully")
	
	// Try to get Infrastructure resource
	infrastructureGVR := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "infrastructures",
	}
	
	log.Printf("  → OpenShift Infrastructure: Querying resource config.openshift.io/v1/infrastructures/cluster...")
	infrastructure, err := dynamicClient.Resource(infrastructureGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		log.Printf("  → OpenShift Infrastructure: Failed to get Infrastructure resource: %v", err)
		return ""
	}
	log.Printf("  → OpenShift Infrastructure: Infrastructure resource obtained successfully")
	
	// Extract infrastructure name
	log.Printf("  → OpenShift Infrastructure: Extracting status.infrastructureName field...")
	if status, found, err := unstructured.NestedMap(infrastructure.Object, "status"); err == nil && found {
		log.Printf("  → OpenShift Infrastructure: Status field found, looking for infrastructureName...")
		if infrastructureName, found, err := unstructured.NestedString(status, "infrastructureName"); err == nil && found && infrastructureName != "" {
			log.Printf("  → OpenShift Infrastructure: Raw infrastructureName found: '%s'", infrastructureName)
			
			// Clean up the name to be more readable (remove random suffixes if needed)
			if len(infrastructureName) > 10 && strings.Contains(infrastructureName, "-") {
				parts := strings.Split(infrastructureName, "-")
				log.Printf("  → OpenShift Infrastructure: Parsing parts: %v", parts)
				if len(parts) >= 2 {
					// Keep first part and add "crc" if it looks like a CRC cluster
					if parts[0] == "crc" {
						log.Printf("  → OpenShift Infrastructure: Detected CRC cluster, returning 'crc-cluster'")
						return "crc-cluster"
					}
					cleanName := parts[0] + "-cluster"
					log.Printf("  → OpenShift Infrastructure: Cleaned name: '%s'", cleanName)
					return cleanName
				}
			}
			log.Printf("  → OpenShift Infrastructure: Using raw name: '%s'", infrastructureName)
			return infrastructureName
		} else {
			log.Printf("  → OpenShift Infrastructure: infrastructureName not found or empty (found=%v, err=%v)", found, err)
		}
	} else {
		log.Printf("  → OpenShift Infrastructure: Status field not found or error (found=%v, err=%v)", found, err)
	}
	
	log.Printf("  → OpenShift Infrastructure: No infrastructure name extracted")
	return ""
}

// detectFromNamespaceLabels tries to detect cluster name from kube-system namespace
func detectFromNamespaceLabels(ctx context.Context, clientset *kubernetes.Clientset) string {
	log.Printf("  → Namespace Labels: Getting kube-system namespace...")
	ns, err := clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		log.Printf("  → Namespace Labels: Failed to get kube-system namespace: %v", err)
		return ""
	}
	log.Printf("  → Namespace Labels: kube-system namespace obtained, checking labels...")
	
	// Check common cluster name labels
	labels := []string{"cluster.x-k8s.io/cluster-name", "cluster-name", "kubernetes.io/cluster-name"}
	for _, label := range labels {
		if name, exists := ns.Labels[label]; exists {
			log.Printf("  → Namespace Labels: Found label '%s' = '%s'", label, name)
			return name
		}
		log.Printf("  → Namespace Labels: Label '%s' not found", label)
	}
	
	log.Printf("  → Namespace Labels: No cluster name labels found")
	return ""
}

// detectFromNodeLabels tries to detect cluster name from node labels
func detectFromNodeLabels(ctx context.Context, clientset *kubernetes.Clientset) string {
	log.Printf("  → Node Labels: Getting first node...")
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil || len(nodes.Items) == 0 {
		log.Printf("  → Node Labels: Failed to get nodes or no nodes found: %v", err)
		return ""
	}
	
	node := nodes.Items[0]
	log.Printf("  → Node Labels: Got node '%s', checking labels...", node.Name)
	
	// Check common cluster name labels on nodes
	labels := []string{"cluster.x-k8s.io/cluster-name", "cluster-name", "kubernetes.io/cluster-name"}
	for _, label := range labels {
		if name, exists := node.Labels[label]; exists {
			log.Printf("  → Node Labels: Found label '%s' = '%s'", label, name)
			return name
		}
		log.Printf("  → Node Labels: Label '%s' not found", label)
	}
	
	log.Printf("  → Node Labels: No cluster name labels found")
	return ""
}

// detectFromHostname creates cluster name from hostname patterns
func detectFromHostname() string {
	log.Printf("  → Hostname: Getting hostname...")
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("  → Hostname: Failed to get hostname: %v", err)
		return "unknown-cluster"
	}
	log.Printf("  → Hostname: Got hostname: '%s'", hostname)
	
	// Extract meaningful part from hostname
	// Remove pod-specific suffixes and normalize
	parts := strings.Split(hostname, "-")
	log.Printf("  → Hostname: Hostname parts: %v", parts)
	if len(parts) > 2 {
		// Likely a pod name, try to extract meaningful cluster info
		log.Printf("  → Hostname: Multi-part hostname detected, using 'detected-cluster'")
		return "detected-cluster"
	}
	
	log.Printf("  → Hostname: Simple hostname, using 'local-cluster'")
	return "local-cluster"
}

// detectClusterDomain attempts to detect the cluster domain
func detectClusterDomain(ctx context.Context) string {
	log.Printf("=== CLUSTER DOMAIN DETECTION START ===")
	
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get in-cluster config for domain detection: %v", err)
		return "cluster.local"
	}
	log.Printf("✓ In-cluster config obtained for domain detection")
	
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset for domain detection: %v", err)
		return "cluster.local"
	}
	log.Printf("✓ Kubernetes clientset created for domain detection")
	
	// Try to get cluster domain from DNS service annotations first
	log.Printf("Step 1: Checking DNS service annotations...")
	services := []struct{ namespace, service string }{
		{"kube-system", "kube-dns"},
		{"kube-system", "coredns"},
		{"openshift-dns", "dns-default"},
	}
	
	for _, svcInfo := range services {
		log.Printf("  → Trying service %s/%s...", svcInfo.namespace, svcInfo.service)
		svc, err := clientset.CoreV1().Services(svcInfo.namespace).Get(ctx, svcInfo.service, metav1.GetOptions{})
		if err != nil {
			log.Printf("  → Service %s/%s not found: %v", svcInfo.namespace, svcInfo.service, err)
			continue
		}
		
		log.Printf("  → Found service %s/%s, checking annotations...", svcInfo.namespace, svcInfo.service)
		if domain, exists := svc.Annotations["cluster-domain"]; exists {
			log.Printf("  → ✓ Found cluster-domain annotation: %s", domain)
			return domain
		}
		log.Printf("  → No cluster-domain annotation found")
	}
	
	// Check DNS resource from config.openshift.io API for baseDomain
	log.Printf("Step 2: Checking OpenShift DNS resource for baseDomain...")
	if domain := detectDomainFromOpenShiftDNS(ctx); domain != "" {
		log.Printf("✓ SUCCESS: BaseDomain detected from OpenShift DNS: %s", domain)
		log.Printf("=== CLUSTER DOMAIN DETECTION END ===")
		return domain
	}
	
	// Check ConfigMap for DNS configuration as fallback
	log.Printf("Step 3: Checking DNS ConfigMaps as fallback...")
	if domain := detectDomainFromDNSConfig(ctx, clientset); domain != "" {
		log.Printf("✓ SUCCESS: Domain detected from DNS config: %s", domain)
		log.Printf("=== CLUSTER DOMAIN DETECTION END ===")
		return domain
	}
	
	log.Printf("✗ No domain found, using fallback: cluster.local")
	log.Printf("=== CLUSTER DOMAIN DETECTION END ===")
	return "cluster.local"
}

// detectDomainFromOpenShiftDNS tries to detect baseDomain from OpenShift DNS config.openshift.io resource
func detectDomainFromOpenShiftDNS(ctx context.Context) string {
	log.Printf("  → OpenShift DNS: Getting in-cluster config...")
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("  → OpenShift DNS: Failed to get config: %v", err)
		return ""
	}
	log.Printf("  → OpenShift DNS: Config obtained, creating dynamic client...")
	
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Printf("  → OpenShift DNS: Failed to create dynamic client: %v", err)
		return ""
	}
	log.Printf("  → OpenShift DNS: Dynamic client created successfully")
	
	// Query DNS resource from config.openshift.io
	dnsGVR := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "dnses",
	}
	
	log.Printf("  → OpenShift DNS: Querying config.openshift.io/v1/dnses/cluster...")
	dns, err := dynamicClient.Resource(dnsGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		log.Printf("  → OpenShift DNS: Failed to get DNS resource: %v", err)
		return ""
	}
	log.Printf("  → OpenShift DNS: DNS resource obtained successfully")
	
	// Extract baseDomain from spec
	log.Printf("  → OpenShift DNS: Extracting spec.baseDomain field...")
	if spec, found, err := unstructured.NestedMap(dns.Object, "spec"); err == nil && found {
		log.Printf("  → OpenShift DNS: Spec field found, looking for baseDomain...")
		if baseDomain, found, err := unstructured.NestedString(spec, "baseDomain"); err == nil && found && baseDomain != "" {
			log.Printf("  → OpenShift DNS: ✓ BaseDomain found: '%s'", baseDomain)
			return baseDomain
		} else {
			log.Printf("  → OpenShift DNS: baseDomain not found or empty (found=%v, err=%v)", found, err)
		}
	} else {
		log.Printf("  → OpenShift DNS: Spec field not found or error (found=%v, err=%v)", found, err)
	}
	
	log.Printf("  → OpenShift DNS: No baseDomain extracted")
	return ""
}

// detectDomainFromDNSConfig tries to detect domain from DNS ConfigMap
func detectDomainFromDNSConfig(ctx context.Context, clientset *kubernetes.Clientset) string {
	log.Printf("  → DNS ConfigMaps: Checking multiple sources...")
	
	// Try multiple namespaces and ConfigMap names
	configSources := []struct {
		namespace string
		configMap string
	}{
		{"kube-system", "coredns"},           // Standard Kubernetes
		{"openshift-dns", "dns-default"},    // OpenShift
		{"kube-system", "kube-dns"},         // Older Kubernetes
	}
	
	for _, source := range configSources {
		log.Printf("  → DNS ConfigMaps: Trying %s/%s...", source.namespace, source.configMap)
		cm, err := clientset.CoreV1().ConfigMaps(source.namespace).Get(ctx, source.configMap, metav1.GetOptions{})
		if err != nil {
			log.Printf("  → DNS ConfigMaps: %s/%s not found: %v", source.namespace, source.configMap, err)
			continue
		}
		
		log.Printf("  → DNS ConfigMaps: Found %s/%s, checking Corefile...", source.namespace, source.configMap)
		if corefile, exists := cm.Data["Corefile"]; exists {
			log.Printf("  → DNS ConfigMaps: Corefile found, parsing...")
			// Parse Corefile for kubernetes domain configuration
			lines := strings.Split(corefile, "\n")
			for _, line := range lines {
				// Look for kubernetes plugin line: "kubernetes cluster.local ..."
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "kubernetes ") {
					log.Printf("  → DNS ConfigMaps: Found kubernetes line: '%s'", line)
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						domain := parts[1]
						log.Printf("  → DNS ConfigMaps: Extracted domain: '%s'", domain)
						// Validate it looks like a domain
						if strings.Contains(domain, ".") {
							log.Printf("  → DNS ConfigMaps: ✓ Valid domain format: '%s'", domain)
							return domain
						}
						log.Printf("  → DNS ConfigMaps: Invalid domain format: '%s'", domain)
					}
				}
			}
		} else {
			log.Printf("  → DNS ConfigMaps: No Corefile found in %s/%s", source.namespace, source.configMap)
		}
	}
	
	log.Printf("  → DNS ConfigMaps: No domain found in any ConfigMap")
	return ""
}

func loadConfig(ctx context.Context) (*Config, error) {
	// Detect cluster name and domain dynamically, with ConfigMap override option
	detectedClusterName := detectClusterName(ctx)
	detectedClusterDomain := detectClusterDomain(ctx)
	
	// Allow ConfigMap override, but use detected values as defaults
	clusterName := getConfigValue("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = detectedClusterName
		log.Printf("Using detected cluster name: %s", clusterName)
	} else {
		log.Printf("Using ConfigMap cluster name: %s (detected: %s)", clusterName, detectedClusterName)
	}
	
	clusterDomain := getConfigValue("CLUSTER_DOMAIN") 
	if clusterDomain == "" {
		clusterDomain = detectedClusterDomain
		log.Printf("Using detected cluster domain: %s", clusterDomain)
	} else {
		log.Printf("Using ConfigMap cluster domain: %s (detected: %s)", clusterDomain, detectedClusterDomain)
	}
	
	config := &Config{
		ClusterDomain:     clusterDomain,
		ClusterName:       clusterName,
		MinIOEndpoint:     getConfigValueWithWarning("MINIO_ENDPOINT", "", "MinIO connection"),
		MinIOAccessKey:    getConfigValueWithWarning("MINIO_ACCESS_KEY", "", "MinIO authentication"),
		MinIOSecretKey:    getConfigValueWithWarning("MINIO_SECRET_KEY", "", "MinIO authentication"),
		MinIOBucket:       getConfigValueWithWarning("MINIO_BUCKET", "cluster-backups", "MinIO storage"),
		MinIOUseSSL:       getConfigValueWithWarning("MINIO_USE_SSL", "true", "MinIO security") == "true",
		BatchSize:         50,
		RetryAttempts:     3,
		RetryDelay:        5 * time.Second,
		// Cleanup configuration
		EnableCleanup:     getConfigValueWithWarning("ENABLE_CLEANUP", "true", "cleanup policy") == "true",
		RetentionDays:     7, // Default to 7 days
		CleanupOnStartup:  getConfigValueWithWarning("CLEANUP_ON_STARTUP", "false", "cleanup timing") == "true",
		// Advanced bucket management
		AutoCreateBucket:  getConfigValueWithWarning("AUTO_CREATE_BUCKET", "false", "bucket management") == "true",
		BucketRetryAttempts: 3,
		BucketRetryDelay:    2 * time.Second,
	}

	// Parse batch size from environment with validation
	if batchStr := getConfigValueWithWarning("BATCH_SIZE", "50", "performance tuning"); batchStr != "" {
		if batch, err := strconv.Atoi(batchStr); err == nil {
			if batch > 0 && batch <= 1000 {  // Reasonable bounds: 1-1000
				config.BatchSize = batch
			} else {
				log.Printf("Warning: Invalid BATCH_SIZE %d, using default 50 (valid range: 1-1000)", batch)
			}
		} else {
			log.Printf("Warning: Invalid BATCH_SIZE format '%s', using default 50", batchStr)
		}
	}

	// Parse retry attempts from environment with validation
	if retryStr := getConfigValueWithWarning("RETRY_ATTEMPTS", "3", "retry policy"); retryStr != "" {
		if retry, err := strconv.Atoi(retryStr); err == nil {
			if retry >= 0 && retry <= 10 {  // Reasonable bounds: 0-10
				config.RetryAttempts = retry
			} else {
				log.Printf("Warning: Invalid RETRY_ATTEMPTS %d, using default 3 (valid range: 0-10)", retry)
			}
		} else {
			log.Printf("Warning: Invalid RETRY_ATTEMPTS format '%s', using default 3", retryStr)
		}
	}

	// Parse retry delay from environment with validation
	if delayStr := getConfigValueWithWarning("RETRY_DELAY", "5s", "retry timing"); delayStr != "" {
		if delay, err := time.ParseDuration(delayStr); err == nil {
			if delay >= time.Second && delay <= 5*time.Minute {  // Bounds: 1s-5m
				config.RetryDelay = delay
			} else {
				log.Printf("Warning: Invalid RETRY_DELAY %v, using default 5s (valid range: 1s-5m)", delay)
			}
		} else {
			log.Printf("Warning: Invalid RETRY_DELAY format '%s', using default 5s", delayStr)
		}
	}

	// Parse fallback buckets from environment (comma-separated)
	if fallbackStr := getConfigValue("FALLBACK_BUCKETS"); fallbackStr != "" {
		fallbackBuckets := strings.Split(fallbackStr, ",")
		var validFallbacks []string
		for _, bucket := range fallbackBuckets {
			bucket = strings.TrimSpace(bucket)
			if bucket != "" && bucket != config.MinIOBucket {
				validFallbacks = append(validFallbacks, bucket)
			}
		}
		config.FallbackBuckets = validFallbacks
		log.Printf("Configured %d fallback buckets: %v", len(validFallbacks), validFallbacks)
	}
	
	// Parse bucket retry attempts from environment with validation
	if retryStr := getConfigValue("BUCKET_RETRY_ATTEMPTS"); retryStr != "" {
		if retries, err := strconv.Atoi(retryStr); err == nil {
			if retries >= 1 && retries <= 10 {
				config.BucketRetryAttempts = retries
			} else {
				log.Printf("Warning: Invalid BUCKET_RETRY_ATTEMPTS %d, using default 3 (valid range: 1-10)", retries)
			}
		} else {
			log.Printf("Warning: Invalid BUCKET_RETRY_ATTEMPTS format '%s', using default 3", retryStr)
		}
	}
	
	// Parse bucket retry delay from environment with validation
	if delayStr := getConfigValue("BUCKET_RETRY_DELAY"); delayStr != "" {
		if delay, err := time.ParseDuration(delayStr); err == nil {
			if delay >= time.Second && delay <= 30*time.Second {
				config.BucketRetryDelay = delay
			} else {
				log.Printf("Warning: Invalid BUCKET_RETRY_DELAY %v, using default 2s (valid range: 1s-30s)", delay)
			}
		} else {
			log.Printf("Warning: Invalid BUCKET_RETRY_DELAY format '%s', using default 2s", delayStr)
		}
	}

	// Parse retention days from environment with validation
	if retentionStr := getConfigValueWithWarning("RETENTION_DAYS", "7", "cleanup retention"); retentionStr != "" {
		if retention, err := strconv.Atoi(retentionStr); err == nil {
			if retention > 0 && retention <= 365 {  // Bounds: 1-365 days
				config.RetentionDays = retention
			} else {
				log.Printf("Warning: Invalid RETENTION_DAYS %d, using default 7 (valid range: 1-365)", retention)
			}
		} else {
			log.Printf("Warning: Invalid RETENTION_DAYS format '%s', using default 7", retentionStr)
		}
	}

	// Comprehensive configuration validation
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %v", err)
	}

	return config, nil
}

func loadBackupConfig() (*BackupConfig, error) {
	// Load all configuration from environment variables (set via envFrom ConfigMap)
	config := &BackupConfig{
		// FilteringMode removed - hardcoded to whitelist mode
		IncludeResources:        parseCommaSeparated(getConfigValueWithWarning("INCLUDE_RESOURCES", "", "resource inclusion")),
		IncludeNamespaces:       parseCommaSeparated(getConfigValueWithWarning("INCLUDE_NAMESPACES", "", "namespace inclusion")),
		// IncludeCRDs removed - now fully dynamic via API discovery
		LabelSelector:           getConfigValueWithWarning("LABEL_SELECTOR", "", "label filtering"),
		AnnotationSelector:      getConfigValueWithWarning("ANNOTATION_SELECTOR", "", "annotation filtering"),
		MaxResourceSize:         getConfigValueWithWarning("MAX_RESOURCE_SIZE", "10Mi", "resource size limit"),
		FollowOwnerReferences:   getConfigValueWithWarning("FOLLOW_OWNER_REFERENCES", "false", "owner reference tracking") == "true",
		IncludeManagedFields:    getConfigValueWithWarning("INCLUDE_MANAGED_FIELDS", "false", "managed fields") == "true",
		IncludeStatus:           getConfigValueWithWarning("INCLUDE_STATUS", "false", "resource status") == "true",
		OpenShiftMode:           getConfigValueWithWarning("OPENSHIFT_MODE", "auto-detect", "OpenShift detection"),
		IncludeOpenShiftRes:     getConfigValueWithWarning("INCLUDE_OPENSHIFT_RESOURCES", "true", "OpenShift resources") == "true",
		ValidateYAML:            getConfigValueWithWarning("VALIDATE_YAML", "true", "YAML validation") == "true",
		SkipInvalidResources:    getConfigValueWithWarning("SKIP_INVALID_RESOURCES", "true", "invalid resource handling") == "true",
		EnableCleanup:           getConfigValueWithWarning("ENABLE_CLEANUP", "true", "cleanup policy") == "true",
		CleanupOnStartup:        getConfigValueWithWarning("CLEANUP_ON_STARTUP", "false", "startup cleanup") == "true",
		RetentionDays:           7, // Will be parsed from RETENTION_DAYS
	}

	// Parse retention days with validation
	if retentionStr := getConfigValueWithWarning("RETENTION_DAYS", "7", "cleanup retention"); retentionStr != "" {
		if retention, err := strconv.Atoi(retentionStr); err == nil && retention > 0 && retention <= 365 {
			config.RetentionDays = retention
		} else {
			log.Printf("Warning: Invalid RETENTION_DAYS '%s', using default 7", retentionStr)
		}
	}

	// Validate BackupConfig for security and consistency
	if err := validateBackupConfig(config); err != nil {
		log.Printf("Warning: BackupConfig validation failed: %v, using safe defaults", err)
		return getDefaultBackupConfig(), nil
	}
	
	return config, nil
}


func getDefaultBackupConfig() *BackupConfig {
	return &BackupConfig{
		// FilteringMode removed - hardcoded to whitelist mode
		IncludeResources: []string{
			"deployments", "services", "configmaps", "persistentvolumeclaims",
			"routes", "buildconfigs", "imagestreams", "deploymentconfigs",
		},
		// IncludeCRDs removed - now fully dynamic via API discovery
		OpenShiftMode:         "auto-detect",
		IncludeOpenShiftRes:   true,
		ValidateYAML:          true,
		SkipInvalidResources:  true,
		FollowOwnerReferences: false,
		IncludeManagedFields:  false,
		IncludeStatus:         false,
		// Cleanup configuration defaults
		EnableCleanup:         true,
		RetentionDays:         7,
		CleanupOnStartup:      false,
	}
}

func parseCommaSeparated(input string) []string {
	var result []string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			parts := strings.Split(line, ",")
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					result = append(result, trimmed)
				}
			}
		}
	}
	return result
}

func NewClusterBackup(config *Config, backupConfig *BackupConfig, logger *StructuredLogger, ctx context.Context) (*ClusterBackup, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %v", err)
	}

	minioClient, err := minio.New(config.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinIOAccessKey, config.MinIOSecretKey, ""),
		Secure: config.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}

	metrics := &BackupMetrics{
		BackupDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "cluster_backup_duration_seconds",
			Help: "Duration of cluster backup operations in seconds",
		}),
		BackupErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cluster_backup_errors_total",
			Help: "Total number of backup errors",
		}),
		ResourcesBackedUp: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cluster_backup_resources_total",
			Help: "Total number of resources backed up",
		}),
		LastBackupTime: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "cluster_backup_last_success_timestamp",
			Help: "Timestamp of the last successful backup",
		}),
		NamespacesBackedUp: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "cluster_backup_namespaces_total",
			Help: "Number of namespaces backed up",
		}),
	}

	// Initialize priority manager
	priorityManager := NewPriorityManager(kubeClient, "backup-resource-priorities", "cluster-backup")
	
	// Load initial priority configuration
	if err := priorityManager.LoadConfig(); err != nil {
		logger.Warn("priority_config_load_failed", "Failed to load priority config, using defaults", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Initialize circuit breakers for fault tolerance
	minioCircuitBreaker := NewCircuitBreaker(5, 30*time.Second)  // 5 failures, 30s reset
	apiCircuitBreaker := NewCircuitBreaker(3, 15*time.Second)    // 3 failures, 15s reset

	logger.Info("circuit_breakers_initialized", "Circuit breakers initialized for fault tolerance", map[string]interface{}{
		"minio_max_failures": 5,
		"minio_reset_timeout": "30s",
		"api_max_failures": 3,
		"api_reset_timeout": "15s",
	})

	return &ClusterBackup{
		config:               config,
		backupConfig:         backupConfig,
		minioClient:          minioClient,
		kubeClient:           kubeClient,
		dynamicClient:        dynamicClient,
		discoveryClient:      discoveryClient,
		metrics:              metrics,
		ctx:                  ctx,
		logger:               logger,
		priorityManager:      priorityManager,
		minioCircuitBreaker:  minioCircuitBreaker,
		apiCircuitBreaker:    apiCircuitBreaker,
	}, nil
}

func (cb *ClusterBackup) Run() error {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		cb.metrics.BackupDuration.Observe(duration.Seconds())
		cb.logger.Info("backup_duration", "Backup operation completed", map[string]interface{}{
			"duration_ms": float64(duration.Nanoseconds()) / 1e6,
			"duration_seconds": duration.Seconds(),
		})
	}()

	cb.logger.Info("backup_start", "Starting backup operation", map[string]interface{}{
		"cluster": cb.config.ClusterName + "." + cb.config.ClusterDomain,
		"openshift_mode": cb.backupConfig.OpenShiftMode,
		"filtering_mode": "whitelist",
	})

	// Auto-detect OpenShift if needed
	if cb.backupConfig.OpenShiftMode == "auto-detect" {
		detectedMode := cb.detectOpenShift()
		cb.backupConfig.OpenShiftMode = detectedMode
		cb.logger.Info("openshift_detection", "OpenShift auto-detection completed", map[string]interface{}{
			"detected_mode": detectedMode,
		})
	}

	cb.logger.Info("minio_check", "Checking MinIO bucket existence", map[string]interface{}{
		"bucket": cb.config.MinIOBucket,
		"endpoint": cb.config.MinIOEndpoint,
	})

	// Advanced bucket management with auto-creation and fallback
	activeBucket, err := cb.ensureBucketAvailable()
	if err != nil {
		cb.metrics.BackupErrors.Inc()
		return fmt.Errorf("failed to ensure bucket availability: %v", err)
	}
	
	// Update config with active bucket (might be fallback)
	cb.config.MinIOBucket = activeBucket
	
	cb.logger.Info("minio_ready", "MinIO bucket verified successfully", map[string]interface{}{
		"bucket": activeBucket,
		"bucket_management": "advanced",
	})

	// Get all available API resources
	cb.logger.Info("api_discovery_start", "Starting API resource discovery", nil)
	apiResources, err := cb.getAPIResources()
	if err != nil {
		cb.metrics.BackupErrors.Inc()
		cb.logger.Error("api_discovery_failed", "Failed to get API resources", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to get API resources: %v", err)
	}

	cb.logger.Info("api_discovery_complete", "API resource discovery completed", map[string]interface{}{
		"resource_types_found": len(apiResources),
	})

	// Get namespaces to backup
	cb.logger.Info("namespace_discovery_start", "Starting namespace discovery", nil)
	namespaces, err := cb.getNamespacesToBackup()
	if err != nil {
		cb.metrics.BackupErrors.Inc()
		cb.logger.Error("namespace_discovery_failed", "Failed to get namespaces", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to get namespaces: %v", err)
	}

	cb.logger.Info("namespace_discovery_complete", "Namespace discovery completed", map[string]interface{}{
		"namespaces_to_backup": len(namespaces),
		"namespace_list": namespaces,
	})
	cb.metrics.NamespacesBackedUp.Set(float64(len(namespaces)))

	totalResources := 0
	namespaceResults := make([]map[string]interface{}, 0)
	
	for _, ns := range namespaces {
		nsStartTime := time.Now()
		count, err := cb.backupNamespace(ns, apiResources)
		nsDuration := time.Since(nsStartTime)
		
		nsResult := map[string]interface{}{
			"namespace": ns,
			"duration_ms": float64(nsDuration.Nanoseconds()) / 1e6,
			"resources_backed_up": count,
		}
		
		if err != nil {
			cb.logger.Error("namespace_backup_failed", "Error backing up namespace", map[string]interface{}{
				"namespace": ns,
				"error": err.Error(),
				"duration_ms": float64(nsDuration.Nanoseconds()) / 1e6,
			})
			cb.metrics.BackupErrors.Inc()
			nsResult["error"] = err.Error()
		} else {
			cb.logger.Info("namespace_backup_complete", "Namespace backup completed", map[string]interface{}{
				"namespace": ns,
				"resources_backed_up": count,
				"duration_ms": float64(nsDuration.Nanoseconds()) / 1e6,
			})
			totalResources += count
		}
		
		namespaceResults = append(namespaceResults, nsResult)
	}

	// Calculate success/failure statistics
	successfulNamespaces := 0
	failedNamespaces := 0
	for _, nsResult := range namespaceResults {
		if _, hasError := nsResult["error"]; hasError {
			failedNamespaces++
		} else {
			successfulNamespaces++
		}
	}
	
	// Create comprehensive backup status summary
	cb.logger.Info("backup_status_summary", "BACKUP OPERATION COMPLETED", map[string]interface{}{
		"status": func() string {
			if failedNamespaces == 0 {
				return "SUCCESS - All namespaces backed up"
			} else if successfulNamespaces > 0 {
				return "PARTIAL SUCCESS - Some namespaces failed"
			} else {
				return "FAILED - No namespaces backed up successfully"
			}
		}(),
		"cluster": cb.config.ClusterName,
		"total_resources_backed_up": totalResources,
		"namespaces_processed": len(namespaces),
		"namespaces_successful": successfulNamespaces,
		"namespaces_failed": failedNamespaces,
		"openshift_mode": cb.backupConfig.OpenShiftMode,
		"filtering_mode": "whitelist",
		"minio_bucket": cb.config.MinIOBucket,
		"backup_timestamp": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
	cb.metrics.LastBackupTime.SetToCurrentTime()
	return nil
}

func (cb *ClusterBackup) detectOpenShift() string {
	// Check cache first (valid for 1 hour)
	if cb.openShiftDetected != nil && time.Since(cb.openShiftCacheTime) < 1*time.Hour {
		cb.logger.Debug("openshift_detection_cached", "Using cached OpenShift detection result", map[string]interface{}{
			"cached_result": *cb.openShiftDetected,
			"cache_age_minutes": int(time.Since(cb.openShiftCacheTime).Minutes()),
		})
		return *cb.openShiftDetected
	}
	
	// Dynamic OpenShift detection via API groups
	cb.logger.Debug("openshift_dynamic_detection", "Performing dynamic OpenShift detection via API discovery", nil)
	
	// Get all available API groups dynamically
	apiGroups, err := cb.discoveryClient.ServerGroups()
	if err != nil {
		result := "disabled"
		cb.openShiftDetected = &result
		cb.openShiftCacheTime = time.Now()
		cb.logger.Info("kubernetes_detected", "Failed to get API groups, assuming standard Kubernetes", map[string]interface{}{
			"mode": "disabled",
			"error": err.Error(),
		})
		return result
	}
	
	// Look for any OpenShift-specific API groups dynamically (ending with .openshift.io)
	for _, group := range apiGroups.Groups {
		if strings.HasSuffix(group.Name, ".openshift.io") {
			result := "enabled"
			cb.openShiftDetected = &result
			cb.openShiftCacheTime = time.Now()
			cb.logger.Info("openshift_detected", "OpenShift detected via dynamic API group discovery", map[string]interface{}{
				"detection_method": "dynamic_api_groups",
				"detected_group": group.Name,
				"mode": "enabled",
			})
			return result
		}
	}
	
	result := "disabled"
	cb.openShiftDetected = &result
	cb.openShiftCacheTime = time.Now()
	cb.logger.Info("kubernetes_detected", "OpenShift API groups not found, using standard Kubernetes mode", map[string]interface{}{
		"mode": "disabled",
		"total_groups_checked": len(apiGroups.Groups),
	})
	return result
}

func (cb *ClusterBackup) getAPIResources() ([]metav1.APIResource, error) {
	var allResources []metav1.APIResource
	
	// Use cached discovery results if available and not expired (cache for 5 minutes)
	resourceLists := cb.getCachedDiscoveryResults()
	
	for _, list := range resourceLists {
		if list == nil {
			continue
		}
		
		// Debug log for each API resource list
		cb.logger.Debug("api_resource_list_debug", "Processing API resource list", map[string]interface{}{
			"group_version": list.GroupVersion,
			"resource_count": len(list.APIResources),
			"contains_slash": strings.Contains(list.GroupVersion, "/"),
		})
		
		for _, resource := range list.APIResources {
			// Debug log for each individual resource BEFORE processing
			cb.logger.Debug("api_resource_raw_debug", "Raw API resource from discovery", map[string]interface{}{
				"resource_name": resource.Name,
				"resource_group": resource.Group,
				"resource_version": resource.Version,
				"resource_namespaced": resource.Namespaced,
				"list_group_version": list.GroupVersion,
			})
			if cb.shouldIncludeResource(resource, list.GroupVersion) {
				// Create a lightweight resource copy - optimize memory usage
				resourceCopy := resource // Direct assignment for read-only operations
				
				// Parse GroupVersion using proper Kubernetes client-go method
				cb.logger.Debug("api_groupversion_parse_debug", "Parsing GroupVersion", map[string]interface{}{
					"raw_group_version": list.GroupVersion,
					"resource_name": resource.Name,
				})
				
				// The correct way: APIResourceList.GroupVersion contains the authoritative Group/Version info
				// Individual APIResource.Group and APIResource.Version are often empty and should NOT be used
				gv, err := schema.ParseGroupVersion(list.GroupVersion)
				if err != nil {
					cb.logger.Error("api_groupversion_parse_error", "Failed to parse GroupVersion", map[string]interface{}{
						"group_version": list.GroupVersion,
						"resource_name": resource.Name,
						"error": err.Error(),
					})
					// This should never happen with valid Kubernetes API responses
					continue
				}
				
				// Always use the parsed GroupVersion from the list, never the individual resource fields
				resourceCopy.Group = gv.Group
				resourceCopy.Version = gv.Version
				
				cb.logger.Debug("api_groupversion_assigned", "Assigned Group/Version from APIResourceList", map[string]interface{}{
					"resource_name": resource.Name,
					"list_group_version": list.GroupVersion,
					"assigned_group": gv.Group,
					"assigned_version": gv.Version,
					"original_resource_group": resource.Group,
					"original_resource_version": resource.Version,
				})
				
				// Debug log AFTER processing to see final values
				cb.logger.Debug("api_resource_processed_debug", "Final processed API resource", map[string]interface{}{
					"resource_name": resourceCopy.Name,
					"final_group": resourceCopy.Group,
					"final_version": resourceCopy.Version,
					"original_list_gv": list.GroupVersion,
					"namespaced": resourceCopy.Namespaced,
				})
				
				allResources = append(allResources, resourceCopy)
			}
		}
	}

	// Sort resources by priority using dynamic priority system
	prioritizedResources := make([]metav1.APIResource, 0, len(allResources))
	
	// Create a slice to hold resources with dynamic priority info
	type resourceWithPriority struct {
		metav1.APIResource
		priority int
	}
	
	// Get priorities for all resources using dynamic system
	var resourcesWithPriority []resourceWithPriority
	for _, resource := range allResources {
		// Get dynamic priority (default namespace for cluster-scoped resources)
		namespace := ""
		if resource.Namespaced {
			namespace = "default" // We'll adjust per namespace during actual backup
		}
		
		priority := cb.priorityManager.GetResourcePriority(resource.Name, namespace, nil)
		
		// Check if resource should be excluded
		groupVersion := ""
		if resource.Group != "" {
			groupVersion = resource.Group + "/" + resource.Version
		} else {
			groupVersion = resource.Version
		}
		
		if cb.priorityManager.ShouldExcludeResource(resource.Name, groupVersion) {
			cb.logger.Debug("resource_excluded_by_priority", "Resource excluded by priority manager", map[string]interface{}{
				"resource_name": resource.Name,
				"group_version": groupVersion,
			})
			continue
		}
		
		resourcesWithPriority = append(resourcesWithPriority, resourceWithPriority{
			APIResource: resource,
			priority: priority,
		})
		
		cb.logger.Debug("resource_priority_assigned", "Dynamic priority assigned", map[string]interface{}{
			"resource_name": resource.Name,
			"priority": priority,
			"group_version": groupVersion,
		})
	}
	
	// Sort all resources by dynamic priority
	sort.Slice(resourcesWithPriority, func(i, j int) bool {
		return resourcesWithPriority[i].priority < resourcesWithPriority[j].priority
	})
	
	// Add sorted resources to final list
	for _, res := range resourcesWithPriority {
		prioritizedResources = append(prioritizedResources, res.APIResource)
	}

	cb.logger.Info("resource_dynamic_ordering", "Resources ordered by dynamic priority system", map[string]interface{}{
		"prioritized_resources": len(resourcesWithPriority),
		"total_resources": len(prioritizedResources),
		"dynamic_priority_system": true,
		"excluded_resources": len(allResources) - len(resourcesWithPriority),
	})

	return prioritizedResources, nil
}

// getCachedDiscoveryResults returns cached API discovery results or fetches new ones
func (cb *ClusterBackup) getCachedDiscoveryResults() []*metav1.APIResourceList {
	// Cache expires after 5 minutes
	if cb.apiResourcesCache != nil && time.Since(cb.cacheTimestamp) < 5*time.Minute {
		cb.logger.Debug("api_discovery_cache_hit", "Using cached API discovery results", map[string]interface{}{
			"cache_age_seconds": int(time.Since(cb.cacheTimestamp).Seconds()),
		})
		return cb.apiResourcesCache
	}
	
	// Fetch fresh results and cache them using more reliable method
	cb.logger.Info("api_discovery_refresh", "Refreshing API discovery cache", nil)
	
	// First try ServerGroupsAndResources for complete API discovery
	groupList, resourceLists, err := cb.discoveryClient.ServerGroupsAndResources()
	if err != nil {
		cb.logger.Debug("api_discovery_fallback", "ServerGroupsAndResources failed, falling back to ServerPreferredResources", map[string]interface{}{
			"error": err.Error(),
		})
		// Fallback to ServerPreferredResources
		resourceLists, err = cb.discoveryClient.ServerPreferredResources()
		if err != nil {
			log.Printf("Warning: Some API resources may not be available: %v", err)
		}
	} else {
		cb.logger.Debug("api_discovery_groups_found", "Discovered API groups", map[string]interface{}{
			"groups_count": len(groupList),
			"resource_lists_count": len(resourceLists),
		})
	}
	
	// Update cache
	cb.apiResourcesCache = resourceLists
	cb.cacheTimestamp = time.Now()
	
	cb.logger.Info("api_discovery_cached", "API discovery results cached", map[string]interface{}{
		"resource_lists": len(resourceLists),
	})
	
	return resourceLists
}

// getCRDResources function removed - now fully dynamic via API discovery

func (cb *ClusterBackup) shouldIncludeResource(resource metav1.APIResource, groupVersion string) bool {
	resourceFullName := resource.Name
	if strings.Contains(groupVersion, "/") {
		groupPart := strings.Split(groupVersion, "/")[0]
		if groupPart != "" {
			resourceFullName = resource.Name + "." + groupPart
		}
	}

	// Must be listable and not a subresource - basic requirement
	if !containsVerb(resource.Verbs, "list") || strings.Contains(resource.Name, "/") {
		return false
	}

	// Only whitelist mode supported - include only resources in the include list
	return cb.isInIncludeList(resource.Name, resourceFullName)
}

func (cb *ClusterBackup) isInIncludeList(resourceName, resourceFullName string) bool {
	for _, included := range cb.backupConfig.IncludeResources {
		if strings.EqualFold(resourceName, included) || strings.EqualFold(resourceFullName, included) {
			return true
		}
	}
	return false
}

// Removed: isInExcludeList - using whitelist only

func (cb *ClusterBackup) getNamespacesToBackup() ([]string, error) {
	// Debug log for namespace filtering
	includeNamespacesRaw := getConfigValueWithWarning("INCLUDE_NAMESPACES", "", "namespace inclusion")
	cb.logger.Debug("namespace_filtering_debug", "Checking namespace filtering configuration", map[string]interface{}{
		"filtering_mode": "whitelist",
		"include_namespaces_raw": includeNamespacesRaw,
		"include_namespaces_count": len(cb.backupConfig.IncludeNamespaces),
		"include_namespaces": cb.backupConfig.IncludeNamespaces,
		// No exclude namespaces - whitelist only
	})
	
	// Only whitelist mode supported - return include namespaces or error
	if len(cb.backupConfig.IncludeNamespaces) > 0 {
		cb.logger.Info("namespace_filtering_whitelist", "Using whitelist mode - only specified namespaces", map[string]interface{}{
			"namespaces_to_backup": cb.backupConfig.IncludeNamespaces,
		})
		return cb.backupConfig.IncludeNamespaces, nil
	} else {
		cb.logger.Error("whitelist_mode_no_namespaces", "Whitelist mode set but no namespaces specified", map[string]interface{}{
			"parsing_result": cb.backupConfig.IncludeNamespaces,
			"raw_env": includeNamespacesRaw,
		})
		return nil, fmt.Errorf("whitelist mode requires INCLUDE_NAMESPACES to be specified")
	}
}

// Removed: shouldExcludeNamespace - using whitelist only

func (cb *ClusterBackup) backupNamespace(namespace string, apiResources []metav1.APIResource) (int, error) {
	cb.logger.Info("namespace_backup_start", "Starting namespace backup", map[string]interface{}{
		"namespace": namespace,
		"api_resources_available": len(apiResources),
	})
	resourceCount := 0
	resourceErrors := 0

	for _, resource := range apiResources {
		gvr := schema.GroupVersionResource{
			Group:    resource.Group,
			Version:  resource.Version,
			Resource: resource.Name,
		}

		// Debug log to check API resource data
		cb.logger.Debug("api_resource_debug", "Processing API resource", map[string]interface{}{
			"resource_name": resource.Name,
			"group": resource.Group,
			"version": resource.Version,
			"namespaced": resource.Namespaced,
			"gvr_constructed": fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource),
		})

		resourceStartTime := time.Now()
		count, err := cb.backupResource(namespace, gvr, resource)
		resourceDuration := time.Since(resourceStartTime)
		
		if err != nil {
			cb.logger.Error("resource_backup_failed", "Error backing up resource type", map[string]interface{}{
				"namespace": namespace,
				"resource_type": resource.Name,
				"group": gvr.Group,
				"version": gvr.Version,
				"error": err.Error(),
				"duration_ms": float64(resourceDuration.Nanoseconds()) / 1e6,
			})
			resourceErrors++
			continue
		}
		
		if count > 0 {
			cb.logger.Debug("resource_backup_success", "Resource backup completed", map[string]interface{}{
				"namespace": namespace,
				"resource_type": resource.Name,
				"count": count,
				"duration_ms": float64(resourceDuration.Nanoseconds()) / 1e6,
			})
		}
		
		resourceCount += count
	}

	cb.logger.Info("namespace_backup_summary", "Namespace backup completed", map[string]interface{}{
		"namespace": namespace,
		"total_resources": resourceCount,
		"resource_errors": resourceErrors,
		"api_types_processed": len(apiResources),
	})
	
	return resourceCount, nil
}

func (cb *ClusterBackup) backupResource(namespace string, gvr schema.GroupVersionResource, resource metav1.APIResource) (int, error) {
	var listOptions metav1.ListOptions
	
	if cb.backupConfig.LabelSelector != "" {
		listOptions.LabelSelector = cb.backupConfig.LabelSelector
	}
	
	// Implement pagination to prevent memory exhaustion
	listOptions.Limit = int64(cb.config.BatchSize)
	
	cb.logger.Debug("resource_list_start", "Starting resource listing with pagination", map[string]interface{}{
		"namespace": namespace,
		"resource_type": resource.Name,
		"group": gvr.Group,
		"version": gvr.Version,
		"namespaced": resource.Namespaced,
		"label_selector": cb.backupConfig.LabelSelector,
		"batch_size": cb.config.BatchSize,
	})

	count := 0
	skipped := 0
	invalid := 0
	totalProcessed := 0
	
	// Stream processing with pagination to prevent memory exhaustion
	for {
		// Create timeout context for each API call
		listCtx, cancel := context.WithTimeout(cb.ctx, 2*time.Minute)
		
		var resources *unstructured.UnstructuredList
		var err error

		// Use circuit breaker and retry logic for API calls
		err = cb.apiCircuitBreaker.Execute(func() error {
			return cb.retryWithExponentialBackoff(fmt.Sprintf("list-%s", resource.Name), func() error {
				if resource.Namespaced {
					resources, err = cb.dynamicClient.Resource(gvr).Namespace(namespace).List(listCtx, listOptions)
				} else {
					resources, err = cb.dynamicClient.Resource(gvr).List(listCtx, listOptions)
				}
				return err
			})
		})
		cancel()

		if err != nil {
			cb.logger.Error("resource_list_failed", "Failed to list resources after retries", map[string]interface{}{
				"namespace": namespace,
				"resource_type": resource.Name,
				"error": err.Error(),
				"continue_token": listOptions.Continue,
			})
			return count, fmt.Errorf("failed to list %s after %d retries: %v", resource.Name, cb.config.RetryAttempts, err)
		}

		if len(resources.Items) == 0 {
			break // No more resources
		}
		
		totalProcessed += len(resources.Items)
		
		cb.logger.Debug("resource_batch_processing", "Processing resource batch", map[string]interface{}{
			"namespace": namespace,
			"resource_type": resource.Name,
			"batch_items": len(resources.Items),
			"total_processed": totalProcessed,
		})
		
		for _, item := range resources.Items {
			if cb.shouldSkipResource(&item) {
				cb.logger.Debug("resource_skipped", "Resource skipped due to filters", map[string]interface{}{
					"namespace": namespace,
					"resource_type": resource.Name,
					"resource_name": item.GetName(),
					"reason": "annotation_or_owner_filter",
				})
				skipped++
				continue
			}

			cleaned := cb.cleanResource(&item)
			
			if cb.backupConfig.ValidateYAML {
				if err := cb.validateResource(cleaned); err != nil {
					if cb.backupConfig.SkipInvalidResources {
						cb.logger.Warn("resource_invalid_skipped", "Skipping invalid resource", map[string]interface{}{
							"namespace": namespace,
							"resource_type": resource.Name,
							"resource_name": item.GetName(),
							"validation_error": err.Error(),
						})
						invalid++
						continue
					}
					cb.logger.Error("resource_invalid_fatal", "Invalid resource causing backup failure", map[string]interface{}{
						"namespace": namespace,
						"resource_type": resource.Name,
						"resource_name": item.GetName(),
						"validation_error": err.Error(),
					})
					return count, fmt.Errorf("invalid resource %s/%s: %v", namespace, item.GetName(), err)
				}
			}

			// Use retry logic for MinIO uploads
			// Handle non-namespaced resources with special namespace
			uploadNamespace := namespace
			if !resource.Namespaced {
				uploadNamespace = "cluster-global"
			}
			
			uploadErr := cb.retryWithExponentialBackoff(fmt.Sprintf("upload-%s/%s", uploadNamespace, item.GetName()), func() error {
				return cb.uploadResource(uploadNamespace, gvr.Resource, item.GetName(), cleaned)
			})
			
			if uploadErr != nil {
				cb.logger.Error("resource_upload_failed", "Failed to upload resource to MinIO after retries", map[string]interface{}{
					"namespace": namespace,
					"resource_type": resource.Name,
					"resource_name": item.GetName(),
					"error": uploadErr.Error(),
					"retries_exhausted": cb.config.RetryAttempts,
				})
				return count, fmt.Errorf("failed to upload %s/%s after %d retries: %v", namespace, item.GetName(), cb.config.RetryAttempts, uploadErr)
			}

			count++
			cb.metrics.ResourcesBackedUp.Inc()
			
			cb.logger.Debug("resource_uploaded", "Resource successfully uploaded", map[string]interface{}{
				"namespace": namespace,
				"resource_type": resource.Name,
				"resource_name": item.GetName(),
				"path": fmt.Sprintf("clusterbackup/%s/%s/%s/%s.yaml", cb.config.ClusterName, namespace, gvr.Resource, item.GetName()),
			})
		}
		
		// Check for pagination continuation
		if resources.GetContinue() == "" {
			break // No more pages
		}
		listOptions.Continue = resources.GetContinue()
		
		cb.logger.Debug("resource_pagination", "Continuing to next page", map[string]interface{}{
			"namespace": namespace,
			"resource_type": resource.Name,
			"continue_token": listOptions.Continue,
			"current_count": count,
		})
	}

	cb.logger.Info("resource_type_summary", "Resource type backup completed", map[string]interface{}{
		"namespace": namespace,
		"resource_type": resource.Name,
		"backed_up": count,
		"skipped": skipped,
		"invalid": invalid,
		"total_processed": totalProcessed,
	})

	return count, nil
}

func (cb *ClusterBackup) shouldSkipResource(resource *unstructured.Unstructured) bool {
	// Skip resources with specific annotations if configured
	if cb.backupConfig.AnnotationSelector != "" {
		annotations := resource.GetAnnotations()
		if annotations == nil {
			return true
		}
		
		// Simple annotation check (could be enhanced with label selector parsing)
		parts := strings.Split(cb.backupConfig.AnnotationSelector, "=")
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if annotations[key] != value {
				return true
			}
		}
	}

	// Skip resources managed by operators if not following owner references
	if !cb.backupConfig.FollowOwnerReferences {
		if owners := resource.GetOwnerReferences(); len(owners) > 0 {
			for _, owner := range owners {
				if owner.Controller != nil && *owner.Controller {
					return true
				}
			}
		}
	}

	return false
}

func (cb *ClusterBackup) validateResource(resource map[string]interface{}) error {
	// Basic YAML validation
	_, err := yaml.Marshal(resource)
	return err
}

func (cb *ClusterBackup) cleanResource(resource *unstructured.Unstructured) map[string]interface{} {
	cleaned := make(map[string]interface{})
	for k, v := range resource.Object {
		cleaned[k] = v
	}

	// Always remove status unless specifically included
	if !cb.backupConfig.IncludeStatus {
		delete(cleaned, "status")
	}

	// Clean metadata
	if metadata, ok := cleaned["metadata"].(map[string]interface{}); ok {
		// Always remove these volatile fields
		delete(metadata, "uid")
		delete(metadata, "resourceVersion")
		delete(metadata, "generation")
		delete(metadata, "creationTimestamp")
		delete(metadata, "selfLink")
		
		if !cb.backupConfig.IncludeManagedFields {
			delete(metadata, "managedFields")
		}
	}

	return cleaned
}

func (cb *ClusterBackup) uploadResource(namespace, resourceType, name string, resource map[string]interface{}) error {
	yamlData, err := yaml.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource to YAML: %v", err)
	}
	
	// Check resource size if limit is configured
	if cb.backupConfig.MaxResourceSize != "" {
		maxSize := parseSize(cb.backupConfig.MaxResourceSize)
		if maxSize > 0 && len(yamlData) > maxSize {
			return fmt.Errorf("resource too large: %d bytes, max: %d bytes", len(yamlData), maxSize)
		}
	}

	// Multi-domain multi-cluster path structure: {domain}/{cluster-name}/{namespace}/{resource-type}/{resource-name}.yaml
	objectPath := fmt.Sprintf("%s/%s/%s/%s/%s.yaml",
		sanitizePath(cb.config.ClusterDomain),
		sanitizePath(cb.config.ClusterName),
		sanitizePath(namespace),
		sanitizePath(resourceType),
		sanitizePath(name),
	)

	// Use circuit breaker for MinIO operations
	err = cb.minioCircuitBreaker.Execute(func() error {
		_, putErr := cb.minioClient.PutObject(
			cb.ctx,
			cb.config.MinIOBucket,
			objectPath,
			strings.NewReader(string(yamlData)),
			int64(len(yamlData)),
			minio.PutObjectOptions{
				ContentType: "application/x-yaml",
			},
		)
		return putErr
	})

	return err
}

func containsVerb(verbs []string, verb string) bool {
	for _, v := range verbs {
		if v == verb {
			return true
		}
	}
	return false
}

func getSecretValue(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getConfigValue(key string) string {
	return os.Getenv(key)
}

func getConfigValueWithWarning(key, defaultValue, configType string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	
	// Log missing configuration with context
	log.Printf("Warning: %s not found in ConfigMap, using default value '%s' for %s", 
		key, defaultValue, configType)
	return defaultValue
}

func validateConfig(config *Config) error {
	// Critical MinIO configuration validation
	if config.MinIOEndpoint == "" {
		return fmt.Errorf("MINIO_ENDPOINT is required")
	}
	if config.MinIOAccessKey == "" {
		return fmt.Errorf("MINIO_ACCESS_KEY is required")
	}
	if config.MinIOSecretKey == "" {
		return fmt.Errorf("MINIO_SECRET_KEY is required")
	}
	
	// Validate endpoint format (basic injection protection)
	if strings.Contains(config.MinIOEndpoint, ";") || strings.Contains(config.MinIOEndpoint, "|") || 
	   strings.Contains(config.MinIOEndpoint, "&") || strings.Contains(config.MinIOEndpoint, "$") {
		return fmt.Errorf("invalid MINIO_ENDPOINT format: contains forbidden characters")
	}
	
	// Validate cluster name (prevent injection)
	if strings.ContainsAny(config.ClusterName, ";|&$`(){}[]") {
		return fmt.Errorf("invalid CLUSTER_NAME: contains forbidden characters")
	}
	
	// Validate bucket name format
	if strings.ContainsAny(config.MinIOBucket, ";|&$`(){}[]") {
		return fmt.Errorf("invalid MINIO_BUCKET: contains forbidden characters")
	}
	
	// Validate numeric ranges (already done in parsing, but double-check)
	if config.BatchSize < 1 || config.BatchSize > 1000 {
		return fmt.Errorf("BATCH_SIZE must be between 1-1000, got %d", config.BatchSize)
	}
	if config.RetryAttempts < 0 || config.RetryAttempts > 10 {
		return fmt.Errorf("RETRY_ATTEMPTS must be between 0-10, got %d", config.RetryAttempts)
	}
	if config.RetentionDays < 1 || config.RetentionDays > 365 {
		return fmt.Errorf("RETENTION_DAYS must be between 1-365, got %d", config.RetentionDays)
	}
	
	// Check for unknown environment variables (security audit)
	checkUnknownEnvVars()
	
	log.Printf("Configuration validation passed: cluster=%s, endpoint=%s, batch_size=%d", 
		config.ClusterName, config.MinIOEndpoint, config.BatchSize)
	
	return nil
}

func validateBackupConfig(config *BackupConfig) error {
	// FilteringMode validation removed - hardcoded to whitelist mode
	
	// Validate namespace names (injection protection)
	for _, ns := range config.IncludeNamespaces {
		if strings.ContainsAny(ns, ";|&$`(){}[]") || len(ns) > 253 {
			return fmt.Errorf("invalid namespace name '%s': contains forbidden characters or too long", ns)
		}
	}
	// Validate resource names (injection protection)
	for _, res := range config.IncludeResources {
		if strings.ContainsAny(res, ";|&$`(){}[]") || len(res) > 100 {
			return fmt.Errorf("invalid resource name '%s': contains forbidden characters or too long", res)
		}
	}
	
	// CRD validation removed - now fully dynamic via API discovery
	
	// Validate resource size format
	if config.MaxResourceSize != "" {
		if parseSize(config.MaxResourceSize) <= 0 {
			return fmt.Errorf("invalid max-resource-size '%s': must be valid size format (e.g., 10Mi)", config.MaxResourceSize)
		}
	}
	
	// Log successful validation with key parameters
	log.Printf("BackupConfig validation passed: filtering=whitelist, include_ns=%d, include_res=%d", 
		len(config.IncludeNamespaces), len(config.IncludeResources))
	
	return nil
}

func checkUnknownEnvVars() {
	// Build known environment variable list dynamically from configuration
	knownVars := make(map[string]bool)
	
	// Configuration-related environment variables
	configVars := []string{
		"CLUSTER_DOMAIN", "CLUSTER_NAME",
		"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY",
		"MINIO_BUCKET", "MINIO_USE_SSL",
		"BATCH_SIZE", "RETRY_ATTEMPTS", "RETRY_DELAY",
		"RETENTION_DAYS", "ENABLE_CLEANUP", "CLEANUP_ON_STARTUP",
		"FILTERING_MODE", "INCLUDE_NAMESPACES", "EXCLUDE_NAMESPACES",
		"INCLUDE_RESOURCES", "EXCLUDE_RESOURCES",
		"INCLUDE_OPENSHIFT_RESOURCES", "INCLUDE_MANAGED_FIELDS",
		"INCLUDE_STATUS", "VALIDATE_YAML", "SKIP_INVALID_RESOURCES",
		"MAX_RESOURCE_SIZE", "LOG_LEVEL", "METRICS_PORT",
	}
	for _, v := range configVars {
		knownVars[v] = true
	}
	
	// System environment variables (commonly found in Kubernetes pods)
	systemVars := []string{
		"POD_NAMESPACE", "HOSTNAME", "PATH", "HOME", "USER", "SHELL",
		"KUBERNETES_SERVICE_HOST", "KUBERNETES_SERVICE_PORT",
		"TERM", "PWD", "OLDPWD", "LANG", "LC_ALL",
	}
	for _, v := range systemVars {
		knownVars[v] = true
	}
	
	// Check all environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := parts[0]
		value := parts[1]
		
		// Skip system and known variables
		if knownVars[key] || strings.HasPrefix(key, "KUBERNETES_") || strings.HasPrefix(key, "OPENSHIFT_") {
			continue
		}
		
		// Log unknown environment variables as potential security risk
		if !strings.HasPrefix(key, "HOME") && !strings.HasPrefix(key, "PATH") && !strings.HasPrefix(key, "SHELL") {
			log.Printf("Warning: Unknown environment variable detected: %s (value length: %d)", key, len(value))
		}
	}
}


// parseSize converts size strings like "10Mi", "1Gi", "5M", "10K" to bytes
func parseSize(sizeStr string) int {
	if sizeStr == "" {
		return 0
	}
	
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) == 0 {
		return 0
	}
	
	// Handle pure numeric values (no unit)
	if v, err := strconv.Atoi(sizeStr); err == nil {
		return v
	}
	
	// Handle single character units (5M, 10K, 1G)
	if len(sizeStr) >= 2 {
		lastChar := strings.ToLower(string(sizeStr[len(sizeStr)-1:]))
		valueStr := sizeStr[:len(sizeStr)-1]
		
		if value, err := strconv.Atoi(valueStr); err == nil {
			switch lastChar {
			case "k":
				return value * 1024
			case "m":
				return value * 1024 * 1024
			case "g":
				return value * 1024 * 1024 * 1024
			}
		}
	}
	
	// Handle two character units (10Ki, 1Mi, 2Gi)
	if len(sizeStr) >= 3 {
		unit := strings.ToLower(sizeStr[len(sizeStr)-2:])
		valueStr := sizeStr[:len(sizeStr)-2]
		
		if value, err := strconv.Atoi(valueStr); err == nil {
			switch unit {
			case "ki":
				return value * 1024
			case "mi":
				return value * 1024 * 1024
			case "gi":
				return value * 1024 * 1024 * 1024
			}
		}
	}
	
	return 0
}

// retryWithExponentialBackoff executes a function with exponential backoff retry logic
func (cb *ClusterBackup) retryWithExponentialBackoff(operation string, fn func() error) error {
	if cb.config.RetryAttempts == 0 {
		return fn() // No retries
	}
	
	var lastErr error
	for attempt := 0; attempt <= cb.config.RetryAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			
			if attempt == cb.config.RetryAttempts {
				// Final attempt failed
				cb.logger.Error("retry_exhausted", "All retry attempts exhausted", map[string]interface{}{
					"operation": operation,
					"attempts": attempt + 1,
					"final_error": err.Error(),
				})
				break
			}
			
			// Calculate exponential backoff delay: base * 2^attempt
			backoffDelay := cb.config.RetryDelay * time.Duration(1<<uint(attempt))
			// Cap maximum delay at 2 minutes
			if backoffDelay > 2*time.Minute {
				backoffDelay = 2 * time.Minute
			}
			
			cb.logger.Warn("retry_attempt", "Operation failed, retrying with exponential backoff", map[string]interface{}{
				"operation": operation,
				"attempt": attempt + 1,
				"max_attempts": cb.config.RetryAttempts + 1,
				"backoff_delay_ms": backoffDelay.Milliseconds(),
				"error": err.Error(),
			})
			
			// Sleep with context cancellation support
			timer := time.NewTimer(backoffDelay)
			select {
			case <-cb.ctx.Done():
				timer.Stop()
				return cb.ctx.Err()
			case <-timer.C:
				// Continue to next attempt
			}
		} else {
			// Success
			if attempt > 0 {
				cb.logger.Info("retry_success", "Operation succeeded after retries", map[string]interface{}{
					"operation": operation,
					"attempts_used": attempt + 1,
				})
			}
			return nil
		}
	}
	
	return lastErr
}

// sanitizePath removes path traversal attempts and invalid characters
func sanitizePath(input string) string {
	// Remove path traversal attempts
	sanitized := strings.ReplaceAll(input, "..", "")
	sanitized = strings.ReplaceAll(sanitized, "\\", "")
	// Allow forward slashes for resource types like "deployments.apps"
	// but ensure no leading/trailing slashes
	sanitized = strings.Trim(sanitized, "/")
	return sanitized
}

func NewStructuredLogger(component, clusterName string) *StructuredLogger {
	return &StructuredLogger{
		clusterName: clusterName,
		logLevel:    getSecretValue("LOG_LEVEL", "info"),
	}
}

func (sl *StructuredLogger) log(level, operation, message string, data map[string]interface{}, err error) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Component: "backup",
		Cluster:   sl.clusterName,
		Operation: operation,
		Message:   message,
		Data:      data,
	}
	
	if err != nil {
		entry.Error = err.Error()
	}
	
	// Add namespace and resource from data if available
	if data != nil {
		if ns, ok := data["namespace"].(string); ok {
			entry.Namespace = ns
		}
		if res, ok := data["resource"].(string); ok {
			entry.Resource = res
		}
		if dur, ok := data["duration_ms"].(float64); ok {
			entry.Duration = dur
		}
	}
	
	logJSON, _ := json.Marshal(entry)
	fmt.Println(string(logJSON))
	
	// Also log to standard logger for backward compatibility
	if level == "error" || level == "fatal" {
		log.Printf("[%s] %s: %s", level, operation, message)
		if err != nil {
			log.Printf("Error details: %v", err)
		}
	}
}

func (sl *StructuredLogger) Debug(operation, message string, data map[string]interface{}) {
	if sl.logLevel == "debug" {
		sl.log("debug", operation, message, data, nil)
	}
}

func (sl *StructuredLogger) Info(operation, message string, data map[string]interface{}) {
	sl.log("info", operation, message, data, nil)
}

func (sl *StructuredLogger) Warn(operation, message string, data map[string]interface{}) {
	sl.log("warn", operation, message, data, nil)
}

func (sl *StructuredLogger) Error(operation, message string, data map[string]interface{}) {
	sl.log("error", operation, message, data, nil)
}

func (sl *StructuredLogger) ErrorWithErr(operation, message string, data map[string]interface{}, err error) {
	sl.log("error", operation, message, data, err)
}

func (sl *StructuredLogger) Fatal(operation, message string, data map[string]interface{}) {
	sl.log("fatal", operation, message, data, nil)
	os.Exit(1)
}

func startMetricsServer() error {
	// Use configurable port with default 8080 for backup service
	metricsPort := getSecretValue("METRICS_PORT", "8080")
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	log.Printf("Starting backup metrics server on :%s", metricsPort)
	return http.ListenAndServe(":"+metricsPort, nil)
}

// Cleanup functions for managing backup retention
func (cb *ClusterBackup) performCleanup() error {
	if !cb.backupConfig.EnableCleanup {
		cb.logger.Debug("cleanup_skip", "Cleanup disabled in configuration", nil)
		return nil
	}

	cb.logger.Info("cleanup_start", "Starting backup cleanup process", map[string]interface{}{
		"retention_days": cb.backupConfig.RetentionDays,
		"cluster": cb.config.ClusterName,
	})

	startTime := time.Now()
	cutoffTime := startTime.AddDate(0, 0, -cb.backupConfig.RetentionDays)
	
	// List objects for this cluster with pagination to prevent memory exhaustion
	prefix := fmt.Sprintf("%s/%s", cb.config.ClusterDomain, cb.config.ClusterName)
	
	var cleanedCount int
	var cleanedSize int64
	var errors []string
	objectsToDelete := make([]string, 0, cb.config.BatchSize)
	
	// Process objects in batches to prevent memory exhaustion
	listOptions := minio.ListObjectsOptions{
		Prefix:  prefix,
		MaxKeys: cb.config.BatchSize,
	}
	
	for {
		objects := cb.minioClient.ListObjects(cb.ctx, cb.config.MinIOBucket, listOptions)
		batchCount := 0
		
		for object := range objects {
			if object.Err != nil {
				cb.logger.Error("cleanup_list_error", "Error listing object during cleanup", map[string]interface{}{
					"error": object.Err.Error(),
				})
				continue
			}
			
			batchCount++

			// Check if object is older than retention period
			if object.LastModified.Before(cutoffTime) {
				objectsToDelete = append(objectsToDelete, object.Key)
				cleanedSize += object.Size
				
				// Process batch deletion when buffer is full
				if len(objectsToDelete) >= cb.config.BatchSize {
					deleted, errs := cb.batchDeleteObjects(objectsToDelete)
					cleanedCount += deleted
					errors = append(errors, errs...)
					objectsToDelete = objectsToDelete[:0] // Reset slice but keep capacity
				}
			}
		}
		
		// If we processed fewer objects than batch size, we're done
		if batchCount < cb.config.BatchSize {
			break
		}
	}
	
	// Process remaining objects
	if len(objectsToDelete) > 0 {
		deleted, errs := cb.batchDeleteObjects(objectsToDelete)
		cleanedCount += deleted
		errors = append(errors, errs...)
	}

	duration := time.Since(startTime)
	
	if len(errors) > 0 {
		cb.logger.Warn("cleanup_complete_with_errors", "Cleanup completed with some errors", map[string]interface{}{
			"cleaned_files": cleanedCount,
			"cleaned_size_bytes": cleanedSize,
			"errors_count": len(errors),
			"duration_ms": duration.Milliseconds(),
		})
	} else {
		cb.logger.Info("cleanup_complete", "Cleanup completed successfully", map[string]interface{}{
			"cleaned_files": cleanedCount,
			"cleaned_size_bytes": cleanedSize,
			"duration_ms": duration.Milliseconds(),
		})
	}

	return nil
}

// batchDeleteObjects deletes multiple objects efficiently with error handling
func (cb *ClusterBackup) batchDeleteObjects(objectKeys []string) (int, []string) {
	var errors []string
	deletedCount := 0
	
	// Use parallel deletion with limited concurrency (max 10 concurrent)
	maxConcurrency := 10
	if len(objectKeys) < maxConcurrency {
		maxConcurrency = len(objectKeys)
	}
	
	// Create channels for coordination
	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan struct {
		success bool
		key     string
		err     error
	}, len(objectKeys))
	
	// Start deletion workers
	for _, key := range objectKeys {
		go func(objectKey string) {
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore
			
			deleteCtx, cancel := context.WithTimeout(cb.ctx, 30*time.Second)
			defer cancel()
			
			err := cb.minioClient.RemoveObject(deleteCtx, cb.config.MinIOBucket, objectKey, minio.RemoveObjectOptions{})
			results <- struct {
				success bool
				key     string
				err     error
			}{err == nil, objectKey, err}
		}(key)
	}
	
	// Collect results
	for i := 0; i < len(objectKeys); i++ {
		result := <-results
		if result.success {
			deletedCount++
			cb.logger.Debug("cleanup_batch_delete_success", "Object deleted successfully", map[string]interface{}{
				"object_key": result.key,
			})
		} else {
			errorMsg := fmt.Sprintf("Failed to remove %s: %v", result.key, result.err)
			errors = append(errors, errorMsg)
			cb.logger.Error("cleanup_batch_delete_error", "Failed to delete object", map[string]interface{}{
				"object_key": result.key,
				"error": result.err.Error(),
			})
		}
	}
	
	cb.logger.Info("cleanup_batch_complete", "Batch deletion completed", map[string]interface{}{
		"requested": len(objectKeys),
		"deleted": deletedCount,
		"errors": len(errors),
	})
	
	return deletedCount, errors
}

func (cb *ClusterBackup) addBackupMetadata(obj *unstructured.Unstructured) {
	// Add backup metadata to the object
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	
	annotations["backup.cluster/timestamp"] = time.Now().Format(time.RFC3339)
	annotations["backup.cluster/cluster"] = cb.config.ClusterName
	annotations["backup.cluster/version"] = "1.0.0"
	
	obj.SetAnnotations(annotations)
}

func (cb *ClusterBackup) shouldCleanupOnStartup() bool {
	return cb.backupConfig.EnableCleanup && cb.backupConfig.CleanupOnStartup
}

// PriorityManager Implementation
func NewPriorityManager(clientset kubernetes.Interface, configMap, namespace string) *PriorityManager {
	return &PriorityManager{
		configMap: configMap,
		namespace: namespace,
		clientset: clientset,
	}
}

func (pm *PriorityManager) LoadConfig() error {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	
	// Get ConfigMap
	cm, err := pm.clientset.CoreV1().ConfigMaps(pm.namespace).Get(
		context.Background(), pm.configMap, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get priority ConfigMap: %v", err)
	}
	
	// Parse priorities.yaml
	prioritiesYAML, exists := cm.Data["priorities.yaml"]
	if !exists {
		return fmt.Errorf("priorities.yaml not found in ConfigMap")
	}
	
	var config ResourcePriorityConfig
	if err := yaml.Unmarshal([]byte(prioritiesYAML), &config); err != nil {
		return fmt.Errorf("failed to parse priorities.yaml: %v", err)
	}
	
	pm.config = &config
	pm.lastUpdate = time.Now()
	
	log.Printf("Loaded dynamic priority configuration with %d core resources", 
		len(config.CoreResources))
	return nil
}

func (pm *PriorityManager) GetResourcePriority(resourceName, namespace string, labels map[string]string) int {
	pm.lock.RLock()
	defer pm.lock.RUnlock()
	
	if pm.config == nil {
		return 1000 // Default low priority if config not loaded
	}
	
	// Check all priority categories in order
	priority := pm.getBasePriority(resourceName)
	
	// Apply namespace-specific overrides
	if nsOverride, exists := pm.config.SpecialHandling.NamespaceOverrides[namespace]; exists {
		priority += nsOverride.PriorityBoost
	}
	
	// Apply label-based dynamic rules
	for labelSelector, priorityAdjust := range pm.config.DynamicRules.LabelPriorities {
		parts := strings.Split(labelSelector, "=")
		if len(parts) == 2 {
			key, value := parts[0], parts[1]
			if labelValue, exists := labels[key]; exists && labelValue == value {
				priority += priorityAdjust
				break // Apply first matching rule
			}
		}
	}
	
	// Ensure priority is positive
	if priority < 1 {
		priority = 1
	}
	
	return priority
}

func (pm *PriorityManager) getBasePriority(resourceName string) int {
	// Check each category in priority order
	if priority, exists := pm.config.CoreResources[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.RBACResources[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.NetworkResources[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.WorkloadResources[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.OpenShiftCore[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.OpenShiftSecurity[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.StorageResources[resourceName]; exists {
		return priority
	}
	if priority, exists := pm.config.CustomResources[resourceName]; exists {
		return priority
	}
	
	// Default priority for unknown resources
	return 80
}

func (pm *PriorityManager) ShouldExcludeResource(resourceName, groupVersion string) bool {
	pm.lock.RLock()
	defer pm.lock.RUnlock()
	
	if pm.config == nil {
		return false
	}
	
	// Check exclude list
	for _, exclude := range pm.config.SpecialHandling.Exclude {
		if strings.Contains(groupVersion, exclude) || resourceName == exclude {
			return true
		}
	}
	
	return false
}

func (pm *PriorityManager) GetRetryConfig(priority int) RetryConfig {
	pm.lock.RLock()
	defer pm.lock.RUnlock()
	
	if pm.config == nil {
		// Default retry config
		return RetryConfig{
			MaxAttempts:  3,
			InitialDelay: "2s",
			MaxDelay:     "60s",
		}
	}
	
	// Determine retry category based on priority
	if priority <= 10 {
		if config, exists := pm.config.BackupConfig.RetryConfig["critical"]; exists {
			return config
		}
	} else if priority <= 50 {
		if config, exists := pm.config.BackupConfig.RetryConfig["normal"]; exists {
			return config
		}
	} else {
		if config, exists := pm.config.BackupConfig.RetryConfig["low"]; exists {
			return config
		}
	}
	
	// Fallback to normal config
	if config, exists := pm.config.BackupConfig.RetryConfig["normal"]; exists {
		return config
	}
	
	// Ultimate fallback
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: "2s",
		MaxDelay:     "60s",
	}
}

// Circuit Breaker Implementation
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

type CircuitBreaker struct {
	maxFailures   int
	resetTimeout  time.Duration
	state         CircuitState
	failures      int
	lastFailTime  time.Time
	mutex         sync.RWMutex
	successCount  int
	halfOpenLimit int
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:   maxFailures,
		resetTimeout:  resetTimeout,
		state:         CircuitClosed,
		halfOpenLimit: 3, // Allow 3 attempts in half-open state
	}
}

func (cb *CircuitBreaker) Execute(operation func() error) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Check if we can execute the operation
	if cb.state == CircuitOpen {
		// Check if reset timeout has passed
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.successCount = 0
			log.Printf("Circuit breaker moving to half-open state")
		} else {
			return fmt.Errorf("circuit breaker is open, operation blocked")
		}
	}

	// In half-open state, limit concurrent attempts
	if cb.state == CircuitHalfOpen && cb.successCount >= cb.halfOpenLimit {
		return fmt.Errorf("circuit breaker half-open limit reached, operation blocked")
	}

	// Execute the operation
	err := operation()
	
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailTime = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
		log.Printf("Circuit breaker opened after %d failures", cb.failures)
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	if cb.state == CircuitHalfOpen {
		cb.successCount++
		// If we've had enough successes in half-open, close the circuit
		if cb.successCount >= cb.halfOpenLimit {
			cb.state = CircuitClosed
			cb.failures = 0
			cb.successCount = 0
			log.Printf("Circuit breaker closed after successful recovery")
		}
	} else {
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) GetState() (CircuitState, int, time.Time) {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state, cb.failures, cb.lastFailTime
}

// Advanced Bucket Management Implementation
func (cb *ClusterBackup) ensureBucketAvailable() (string, error) {
	// Try primary bucket first
	bucket, err := cb.tryBucket(cb.config.MinIOBucket)
	if err == nil {
		cb.logger.Info("bucket_primary_available", "Primary bucket is available", map[string]interface{}{
			"bucket": bucket,
		})
		return bucket, nil
	}
	
	cb.logger.Warn("bucket_primary_failed", "Primary bucket failed, trying alternatives", map[string]interface{}{
		"primary_bucket": cb.config.MinIOBucket,
		"error": err.Error(),
	})
	
	// Try auto-creation if enabled
	if cb.config.AutoCreateBucket {
		if createErr := cb.createBucketWithRetry(cb.config.MinIOBucket); createErr == nil {
			cb.logger.Info("bucket_created", "Successfully created primary bucket", map[string]interface{}{
				"bucket": cb.config.MinIOBucket,
			})
			return cb.config.MinIOBucket, nil
		} else {
			cb.logger.Warn("bucket_creation_failed", "Failed to create primary bucket", map[string]interface{}{
				"bucket": cb.config.MinIOBucket,
				"error": createErr.Error(),
			})
		}
	}
	
	// Try fallback buckets
	for _, fallbackBucket := range cb.config.FallbackBuckets {
		bucket, err := cb.tryBucket(fallbackBucket)
		if err == nil {
			cb.logger.Info("bucket_fallback_success", "Fallback bucket is available", map[string]interface{}{
				"fallback_bucket": bucket,
				"original_bucket": cb.config.MinIOBucket,
			})
			return bucket, nil
		}
		
		// Try creating fallback bucket if auto-creation is enabled
		if cb.config.AutoCreateBucket {
			if createErr := cb.createBucketWithRetry(fallbackBucket); createErr == nil {
				cb.logger.Info("bucket_fallback_created", "Successfully created fallback bucket", map[string]interface{}{
					"bucket": fallbackBucket,
				})
				return fallbackBucket, nil
			}
		}
		
		cb.logger.Warn("bucket_fallback_failed", "Fallback bucket failed", map[string]interface{}{
			"fallback_bucket": fallbackBucket,
			"error": err.Error(),
		})
	}
	
	return "", fmt.Errorf("no available buckets: primary=%s failed, %d fallbacks failed", 
		cb.config.MinIOBucket, len(cb.config.FallbackBuckets))
}

func (cb *ClusterBackup) tryBucket(bucketName string) (string, error) {
	// Check if bucket exists using circuit breaker
	var exists bool
	err := cb.minioCircuitBreaker.Execute(func() error {
		var checkErr error
		exists, checkErr = cb.minioClient.BucketExists(cb.ctx, bucketName)
		return checkErr
	})
	if err != nil {
		return "", fmt.Errorf("failed to check bucket existence: %v", err)
	}
	
	if !exists {
		return "", fmt.Errorf("bucket %s does not exist", bucketName)
	}
	
	// Test bucket accessibility with a small operation
	if err := cb.testBucketAccess(bucketName); err != nil {
		return "", fmt.Errorf("bucket %s exists but is not accessible: %v", bucketName, err)
	}
	
	return bucketName, nil
}

func (cb *ClusterBackup) createBucketWithRetry(bucketName string) error {
	var lastErr error
	
	for attempt := 1; attempt <= cb.config.BucketRetryAttempts; attempt++ {
		cb.logger.Debug("bucket_creation_attempt", "Attempting bucket creation", map[string]interface{}{
			"bucket": bucketName,
			"attempt": attempt,
			"max_attempts": cb.config.BucketRetryAttempts,
		})
		
		err := cb.minioClient.MakeBucket(cb.ctx, bucketName, minio.MakeBucketOptions{})
		if err == nil {
			cb.logger.Info("bucket_creation_success", "Bucket created successfully", map[string]interface{}{
				"bucket": bucketName,
				"attempt": attempt,
			})
			return nil
		}
		
		lastErr = err
		cb.logger.Warn("bucket_creation_attempt_failed", "Bucket creation attempt failed", map[string]interface{}{
			"bucket": bucketName,
			"attempt": attempt,
			"error": err.Error(),
		})
		
		// Don't sleep on last attempt
		if attempt < cb.config.BucketRetryAttempts {
			time.Sleep(cb.config.BucketRetryDelay)
		}
	}
	
	return fmt.Errorf("failed to create bucket after %d attempts: %v", cb.config.BucketRetryAttempts, lastErr)
}

func (cb *ClusterBackup) testBucketAccess(bucketName string) error {
	// Test with a small list operation to verify read access
	objectCh := cb.minioClient.ListObjects(cb.ctx, bucketName, minio.ListObjectsOptions{
		MaxKeys: 1,
	})
	
	// Read first object or error
	select {
	case obj := <-objectCh:
		if obj.Err != nil {
			return fmt.Errorf("bucket list test failed: %v", obj.Err)
		}
		// Success - we can list objects
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("bucket access test timed out")
	}
}