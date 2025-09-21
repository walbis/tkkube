package priority

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"cluster-backup/internal/resilience"
)

// ResourcePriorityConfig holds the priority configuration for different resource types
type ResourcePriorityConfig struct {
	CoreResources     map[string]int            `yaml:"core_resources"`
	RBACResources     map[string]int            `yaml:"rbac_resources"`
	NetworkResources  map[string]int            `yaml:"network_resources"`
	WorkloadResources map[string]int            `yaml:"workload_resources"`
	OpenShiftCore     map[string]int            `yaml:"openshift_core"`
	OpenShiftSecurity map[string]int            `yaml:"openshift_security"`
	StorageResources  map[string]int            `yaml:"storage_resources"`
	CustomResources   map[string]int            `yaml:"custom_resources"`
	SpecialHandling   SpecialHandlingConfig     `yaml:"special_handling"`
	DynamicRules      DynamicRulesConfig        `yaml:"dynamic_rules"`
	BackupConfig      BackupBehaviorConfig      `yaml:"backup_config"`
}

// SpecialHandlingConfig defines special handling rules for certain resources
type SpecialHandlingConfig struct {
	Events             EventsConfig                     `yaml:"events"`
	Exclude            []string                         `yaml:"exclude"`
	NamespaceOverrides map[string]NamespaceOverride     `yaml:"namespace_overrides"`
}

// EventsConfig defines how events should be handled
type EventsConfig struct {
	Priority       int `yaml:"priority"`
	RetentionHours int `yaml:"retention_hours"`
}

// NamespaceOverride allows overriding priority for specific namespaces
type NamespaceOverride struct {
	PriorityBoost int `yaml:"priority_boost"`
}

// DynamicRulesConfig defines dynamic priority rules based on labels and size
type DynamicRulesConfig struct {
	LabelPriorities map[string]int  `yaml:"label_priorities"`
	SizeRules       SizeRulesConfig `yaml:"size_rules"`
}

// SizeRulesConfig defines rules based on resource size
type SizeRulesConfig struct {
	LargeResources LargeResourceConfig `yaml:"large_resources"`
}

// LargeResourceConfig defines how large resources should be handled
type LargeResourceConfig struct {
	SizeThreshold   string `yaml:"size_threshold"`
	PriorityPenalty int    `yaml:"priority_penalty"`
}

// BackupBehaviorConfig defines backup behavior settings
type BackupBehaviorConfig struct {
	MaxConcurrentPerType int                    `yaml:"max_concurrent_per_type"`
	RetryConfig          map[string]RetryConfig `yaml:"retry_config"`
}

// RetryConfig defines retry behavior for different priority levels
type RetryConfig struct {
	MaxAttempts  int    `yaml:"max_attempts"`
	InitialDelay string `yaml:"initial_delay"`
	MaxDelay     string `yaml:"max_delay"`
}

// Manager handles resource priority management
type Manager struct {
	config      *ResourcePriorityConfig
	lock        sync.RWMutex
	lastUpdate  time.Time
	configMap   string
	namespace   string
	clientset   kubernetes.Interface
}

// NewManager creates a new priority manager
func NewManager(clientset kubernetes.Interface, configMap, namespace string) *Manager {
	return &Manager{
		clientset: clientset,
		configMap: configMap,
		namespace: namespace,
		config:    getDefaultPriorityConfig(),
	}
}

// LoadConfig loads priority configuration from a ConfigMap
func (pm *Manager) LoadConfig() error {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	if pm.configMap == "" {
		log.Printf("No priority config map specified, using defaults")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cm, err := pm.clientset.CoreV1().ConfigMaps(pm.namespace).Get(ctx, pm.configMap, metav1.GetOptions{})
	if err != nil {
		log.Printf("Failed to load priority config map %s/%s: %v", pm.namespace, pm.configMap, err)
		return err
	}

	if data, exists := cm.Data["priority-config.yaml"]; exists {
		var config ResourcePriorityConfig
		if err := yaml.Unmarshal([]byte(data), &config); err != nil {
			return fmt.Errorf("failed to parse priority configuration: %v", err)
		}

		pm.config = &config
		pm.lastUpdate = time.Now()
		log.Printf("Successfully loaded priority configuration from %s/%s", pm.namespace, pm.configMap)
		return nil
	}

	return fmt.Errorf("priority-config.yaml not found in ConfigMap %s/%s", pm.namespace, pm.configMap)
}

// GetResourcePriority calculates the priority for a given resource
func (pm *Manager) GetResourcePriority(resourceName, namespace string, labels map[string]string) int {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	basePriority := pm.getBasePriority(resourceName)

	// Apply namespace-specific overrides
	if nsOverride, exists := pm.config.SpecialHandling.NamespaceOverrides[namespace]; exists {
		basePriority += nsOverride.PriorityBoost
	}

	// Apply label-based priority adjustments
	for labelKey, priorityBoost := range pm.config.DynamicRules.LabelPriorities {
		if labelValue, exists := labels[labelKey]; exists && labelValue != "" {
			basePriority += priorityBoost
		}
	}

	return basePriority
}

// getBasePriority returns the base priority for a resource type
func (pm *Manager) getBasePriority(resourceName string) int {
	// Check all priority categories
	priorityMaps := []map[string]int{
		pm.config.CoreResources,
		pm.config.RBACResources,
		pm.config.NetworkResources,
		pm.config.WorkloadResources,
		pm.config.OpenShiftCore,
		pm.config.OpenShiftSecurity,
		pm.config.StorageResources,
		pm.config.CustomResources,
	}

	for _, priorityMap := range priorityMaps {
		if priority, exists := priorityMap[resourceName]; exists {
			return priority
		}
	}

	// Default priority for unknown resources
	return 50
}

// ShouldExcludeResource checks if a resource should be excluded from backup
func (pm *Manager) ShouldExcludeResource(resourceName, groupVersion string) bool {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	// Check exclude list
	for _, excluded := range pm.config.SpecialHandling.Exclude {
		if resourceName == excluded || strings.Contains(groupVersion+"/"+resourceName, excluded) {
			return true
		}
	}

	return false
}

// GetRetryConfig returns the retry configuration for a given priority level
func (pm *Manager) GetRetryConfig(priority int) resilience.RetryConfig {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	// Determine retry configuration based on priority
	var retryConfigKey string
	switch {
	case priority >= 90:
		retryConfigKey = "critical"
	case priority >= 70:
		retryConfigKey = "high"
	case priority >= 50:
		retryConfigKey = "medium"
	default:
		retryConfigKey = "low"
	}

	if retryConfig, exists := pm.config.BackupConfig.RetryConfig[retryConfigKey]; exists {
		return convertToResilienceRetryConfig(retryConfig)
	}

	// Default configuration
	return resilience.DefaultRetryConfig()
}

// convertToResilienceRetryConfig converts the priority RetryConfig to resilience.RetryConfig
func convertToResilienceRetryConfig(config RetryConfig) resilience.RetryConfig {
	resConfig := resilience.DefaultRetryConfig()
	
	resConfig.MaxAttempts = config.MaxAttempts
	
	if initialDelay, err := time.ParseDuration(config.InitialDelay); err == nil {
		resConfig.InitialDelay = initialDelay
	}
	
	if maxDelay, err := time.ParseDuration(config.MaxDelay); err == nil {
		resConfig.MaxDelay = maxDelay
	}
	
	return resConfig
}

// GetMaxConcurrentPerType returns the maximum concurrent operations per resource type
func (pm *Manager) GetMaxConcurrentPerType() int {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	return pm.config.BackupConfig.MaxConcurrentPerType
}

// IsLargeResource checks if a resource is considered large based on size threshold
func (pm *Manager) IsLargeResource(sizeStr string) bool {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	threshold := pm.config.DynamicRules.SizeRules.LargeResources.SizeThreshold
	if threshold == "" {
		return false
	}

	resourceSize := parseSize(sizeStr)
	thresholdSize := parseSize(threshold)

	return resourceSize > thresholdSize
}

// GetLargResourcePriorityPenalty returns the priority penalty for large resources
func (pm *Manager) GetLargeResourcePriorityPenalty() int {
	pm.lock.RLock()
	defer pm.lock.RUnlock()

	return pm.config.DynamicRules.SizeRules.LargeResources.PriorityPenalty
}

// parseSize parses size strings like "10Mi", "5Gi" into bytes
func parseSize(sizeStr string) int {
	if sizeStr == "" {
		return 0
	}

	// Remove any spaces
	sizeStr = strings.TrimSpace(sizeStr)
	
	// Handle unit suffixes
	multiplier := 1
	if strings.HasSuffix(sizeStr, "Ki") {
		multiplier = 1024
		sizeStr = strings.TrimSuffix(sizeStr, "Ki")
	} else if strings.HasSuffix(sizeStr, "Mi") {
		multiplier = 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "Mi")
	} else if strings.HasSuffix(sizeStr, "Gi") {
		multiplier = 1024 * 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "Gi")
	} else if strings.HasSuffix(sizeStr, "Ti") {
		multiplier = 1024 * 1024 * 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "Ti")
	}

	if value, err := strconv.Atoi(sizeStr); err == nil {
		return value * multiplier
	}

	return 0
}

// getDefaultPriorityConfig returns a sensible default priority configuration
func getDefaultPriorityConfig() *ResourcePriorityConfig {
	return &ResourcePriorityConfig{
		CoreResources: map[string]int{
			"namespaces":                100,
			"nodes":                     95,
			"persistentvolumes":         90,
			"persistentvolumeclaims":    85,
			"configmaps":               80,
			"secrets":                  85,
			"serviceaccounts":          75,
			"services":                 70,
			"endpoints":                65,
			"pods":                     60,
		},
		RBACResources: map[string]int{
			"clusterroles":             95,
			"clusterrolebindings":      95,
			"roles":                    85,
			"rolebindings":             85,
		},
		NetworkResources: map[string]int{
			"networkpolicies":          80,
			"ingresses":                75,
		},
		WorkloadResources: map[string]int{
			"deployments":              85,
			"statefulsets":             90,
			"daemonsets":               85,
			"replicasets":              70,
			"jobs":                     65,
			"cronjobs":                 75,
		},
		StorageResources: map[string]int{
			"storageclasses":           95,
			"volumeattachments":        80,
		},
		CustomResources: map[string]int{
			// Will be populated dynamically
		},
		SpecialHandling: SpecialHandlingConfig{
			Events: EventsConfig{
				Priority:       30,
				RetentionHours: 24,
			},
			Exclude: []string{
				"events",
				"componentstatuses",
				"bindings",
			},
			NamespaceOverrides: map[string]NamespaceOverride{
				"kube-system":    {PriorityBoost: 20},
				"kube-public":    {PriorityBoost: 15},
				"openshift-*":    {PriorityBoost: 15},
				"default":        {PriorityBoost: 10},
			},
		},
		DynamicRules: DynamicRulesConfig{
			LabelPriorities: map[string]int{
				"app.kubernetes.io/component": 10,
				"backup.priority":             0, // Will use label value as priority boost
			},
			SizeRules: SizeRulesConfig{
				LargeResources: LargeResourceConfig{
					SizeThreshold:   "10Mi",
					PriorityPenalty: 10,
				},
			},
		},
		BackupConfig: BackupBehaviorConfig{
			MaxConcurrentPerType: 5,
			RetryConfig: map[string]RetryConfig{
				"critical": {
					MaxAttempts:  5,
					InitialDelay: "1s",
					MaxDelay:     "30s",
				},
				"high": {
					MaxAttempts:  3,
					InitialDelay: "2s",
					MaxDelay:     "20s",
				},
				"medium": {
					MaxAttempts:  3,
					InitialDelay: "3s",
					MaxDelay:     "15s",
				},
				"low": {
					MaxAttempts:  2,
					InitialDelay: "5s",
					MaxDelay:     "10s",
				},
			},
		},
	}
}