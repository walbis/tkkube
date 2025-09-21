package integration

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	sharedconfig "shared-config/config"
)

// ConfigManager provides unified configuration management across all components
type ConfigManager struct {
	loader *sharedconfig.ConfigLoader
	config *sharedconfig.SharedConfig
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPaths ...string) *ConfigManager {
	// If no paths provided, use default locations
	if len(configPaths) == 0 {
		configPaths = getDefaultConfigPaths()
	}

	loader := sharedconfig.NewConfigLoader(configPaths...)
	
	return &ConfigManager{
		loader: loader,
	}
}

// LoadConfig loads and validates the unified configuration
func (cm *ConfigManager) LoadConfig() (*sharedconfig.SharedConfig, error) {
	config, err := cm.loader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	cm.config = config

	// Ensure integration configuration is properly initialized
	if err := cm.initializeIntegrationDefaults(); err != nil {
		return nil, fmt.Errorf("failed to initialize integration defaults: %v", err)
	}

	return cm.config, nil
}

// GetConfig returns the loaded configuration
func (cm *ConfigManager) GetConfig() *sharedconfig.SharedConfig {
	return cm.config
}

// GetBackupConfig returns backup-specific configuration
func (cm *ConfigManager) GetBackupConfig() sharedconfig.BackupConfig {
	if cm.config == nil {
		return sharedconfig.BackupConfig{}
	}
	return cm.config.Backup
}

// GetGitOpsConfig returns GitOps-specific configuration
func (cm *ConfigManager) GetGitOpsConfig() sharedconfig.GitOpsConfig {
	if cm.config == nil {
		return sharedconfig.GitOpsConfig{}
	}
	return cm.config.GitOps
}

// GetIntegrationConfig returns integration-specific configuration
func (cm *ConfigManager) GetIntegrationConfig() sharedconfig.IntegrationConfig {
	if cm.config == nil {
		return sharedconfig.IntegrationConfig{}
	}
	return cm.config.Integration
}

// GetStorageConfig returns storage-specific configuration
func (cm *ConfigManager) GetStorageConfig() sharedconfig.StorageConfig {
	if cm.config == nil {
		return sharedconfig.StorageConfig{}
	}
	return cm.config.Storage
}

// GetClusterConfig returns cluster-specific configuration
func (cm *ConfigManager) GetClusterConfig() sharedconfig.ClusterConfig {
	if cm.config == nil {
		return sharedconfig.ClusterConfig{}
	}
	return cm.config.Cluster
}

// initializeIntegrationDefaults sets up default values for integration configuration
func (cm *ConfigManager) initializeIntegrationDefaults() error {
	if cm.config == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Initialize integration config if not present
	if cm.config.Integration.WebhookPort == 0 {
		cm.config.Integration.WebhookPort = 8080
	}

	// Initialize bridge defaults
	if !cm.config.Integration.Bridge.Enabled {
		cm.config.Integration.Bridge.Enabled = true
	}
	if cm.config.Integration.Bridge.HealthInterval == 0 {
		cm.config.Integration.Bridge.HealthInterval = 30000000000 // 30 seconds in nanoseconds
	}
	if cm.config.Integration.Bridge.EventBufferSize == 0 {
		cm.config.Integration.Bridge.EventBufferSize = 1000
	}
	if cm.config.Integration.Bridge.MaxConcurrency == 0 {
		cm.config.Integration.Bridge.MaxConcurrency = 10
	}

	// Initialize communication defaults
	if cm.config.Integration.Communication.Method == "" {
		cm.config.Integration.Communication.Method = "webhook"
	}
	if cm.config.Integration.Communication.Endpoints.BackupTool == "" {
		cm.config.Integration.Communication.Endpoints.BackupTool = "http://backup-tool:8080"
	}
	if cm.config.Integration.Communication.Endpoints.GitOpsGenerator == "" {
		cm.config.Integration.Communication.Endpoints.GitOpsGenerator = "http://gitops-generator:8081"
	}
	if cm.config.Integration.Communication.Endpoints.IntegrationBridge == "" {
		cm.config.Integration.Communication.Endpoints.IntegrationBridge = "http://integration-bridge:8080"
	}

	// Initialize retry defaults
	if cm.config.Integration.Communication.Retry.MaxAttempts == 0 {
		cm.config.Integration.Communication.Retry.MaxAttempts = 3
	}
	if cm.config.Integration.Communication.Retry.InitialDelay == 0 {
		cm.config.Integration.Communication.Retry.InitialDelay = 1000000000 // 1 second
	}
	if cm.config.Integration.Communication.Retry.MaxDelay == 0 {
		cm.config.Integration.Communication.Retry.MaxDelay = 30000000000 // 30 seconds
	}
	if cm.config.Integration.Communication.Retry.Multiplier == 0 {
		cm.config.Integration.Communication.Retry.Multiplier = 2.0
	}

	// Initialize trigger integration defaults
	if !cm.config.Integration.Triggers.AutoTrigger {
		cm.config.Integration.Triggers.AutoTrigger = true
	}
	if cm.config.Integration.Triggers.DelayAfterBackup == 0 {
		cm.config.Integration.Triggers.DelayAfterBackup = 30000000000 // 30 seconds
	}

	// Initialize fallback methods if empty
	if len(cm.config.Integration.Triggers.FallbackMethods) == 0 {
		cm.config.Integration.Triggers.FallbackMethods = []string{"webhook", "process"}
	}

	return nil
}

// ValidateConfiguration validates the loaded configuration
func (cm *ConfigManager) ValidateConfiguration() error {
	if cm.config == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Validate storage configuration
	if err := cm.validateStorageConfig(); err != nil {
		return fmt.Errorf("storage configuration invalid: %v", err)
	}

	// Validate cluster configuration
	if err := cm.validateClusterConfig(); err != nil {
		return fmt.Errorf("cluster configuration invalid: %v", err)
	}

	// Validate GitOps configuration
	if err := cm.validateGitOpsConfig(); err != nil {
		return fmt.Errorf("GitOps configuration invalid: %v", err)
	}

	// Validate integration configuration
	if err := cm.validateIntegrationConfig(); err != nil {
		return fmt.Errorf("integration configuration invalid: %v", err)
	}

	return nil
}

// validateStorageConfig validates storage configuration
func (cm *ConfigManager) validateStorageConfig() error {
	storage := cm.config.Storage
	
	if storage.Endpoint == "" {
		return fmt.Errorf("storage endpoint is required")
	}
	if storage.AccessKey == "" {
		return fmt.Errorf("storage access key is required")
	}
	if storage.SecretKey == "" {
		return fmt.Errorf("storage secret key is required")
	}
	if storage.Bucket == "" {
		return fmt.Errorf("storage bucket is required")
	}

	return nil
}

// validateClusterConfig validates cluster configuration
func (cm *ConfigManager) validateClusterConfig() error {
	cluster := cm.config.Cluster
	
	if cluster.Name == "" {
		return fmt.Errorf("cluster name is required")
	}

	return nil
}

// validateGitOpsConfig validates GitOps configuration
func (cm *ConfigManager) validateGitOpsConfig() error {
	gitops := cm.config.GitOps
	
	if gitops.Repository.URL == "" {
		return fmt.Errorf("GitOps repository URL is required")
	}
	if gitops.Repository.Branch == "" {
		return fmt.Errorf("GitOps repository branch is required")
	}

	return nil
}

// validateIntegrationConfig validates integration configuration
func (cm *ConfigManager) validateIntegrationConfig() error {
	integration := cm.config.Integration
	
	if integration.WebhookPort <= 0 || integration.WebhookPort > 65535 {
		return fmt.Errorf("invalid webhook port: %d", integration.WebhookPort)
	}
	
	if integration.Communication.Method == "" {
		return fmt.Errorf("communication method is required")
	}
	
	validMethods := map[string]bool{
		"webhook":   true,
		"event-bus": true,
		"file":      true,
	}
	if !validMethods[integration.Communication.Method] {
		return fmt.Errorf("invalid communication method: %s", integration.Communication.Method)
	}

	return nil
}

// CreateBackupToolConfig creates configuration specific to the backup tool
func (cm *ConfigManager) CreateBackupToolConfig() map[string]interface{} {
	if cm.config == nil {
		return make(map[string]interface{})
	}

	return map[string]interface{}{
		"cluster":     cm.config.Cluster,
		"storage":     cm.config.Storage,
		"backup":      cm.config.Backup,
		"integration": map[string]interface{}{
			"bridge_endpoint": cm.config.Integration.Communication.Endpoints.IntegrationBridge,
			"webhook_enabled": cm.config.Integration.Triggers.AutoTrigger,
		},
		"observability": cm.config.Observability,
		"security":      cm.config.Security,
	}
}

// CreateGitOpsConfig creates configuration specific to the GitOps generator
func (cm *ConfigManager) CreateGitOpsConfig() map[string]interface{} {
	if cm.config == nil {
		return make(map[string]interface{})
	}

	return map[string]interface{}{
		"storage":     cm.config.Storage,
		"gitops":      cm.config.GitOps,
		"cluster":     cm.config.Cluster,
		"integration": map[string]interface{}{
			"bridge_endpoint": cm.config.Integration.Communication.Endpoints.IntegrationBridge,
			"auto_trigger":    cm.config.Integration.Triggers.AutoTrigger,
			"delay_after_backup": cm.config.Integration.Triggers.DelayAfterBackup,
		},
		"observability": cm.config.Observability,
		"security":      cm.config.Security,
	}
}

// CreateIntegrationBridgeConfig creates configuration specific to the integration bridge
func (cm *ConfigManager) CreateIntegrationBridgeConfig() map[string]interface{} {
	if cm.config == nil {
		return make(map[string]interface{})
	}

	return map[string]interface{}{
		"integration":   cm.config.Integration,
		"observability": cm.config.Observability,
		"security":      cm.config.Security,
		"storage":       cm.config.Storage,
		"gitops":        cm.config.GitOps,
		"cluster":       cm.config.Cluster,
	}
}

// GetEnvironmentOverrides returns environment variable overrides
func (cm *ConfigManager) GetEnvironmentOverrides() map[string]string {
	overrides := make(map[string]string)
	
	// Storage overrides
	if val := os.Getenv("MINIO_ENDPOINT"); val != "" {
		overrides["storage.endpoint"] = val
	}
	if val := os.Getenv("MINIO_ACCESS_KEY"); val != "" {
		overrides["storage.access_key"] = val
	}
	if val := os.Getenv("MINIO_SECRET_KEY"); val != "" {
		overrides["storage.secret_key"] = val
	}
	if val := os.Getenv("MINIO_BUCKET"); val != "" {
		overrides["storage.bucket"] = val
	}

	// Cluster overrides
	if val := os.Getenv("CLUSTER_NAME"); val != "" {
		overrides["cluster.name"] = val
	}
	if val := os.Getenv("CLUSTER_DOMAIN"); val != "" {
		overrides["cluster.domain"] = val
	}

	// GitOps overrides
	if val := os.Getenv("GITOPS_REPO"); val != "" {
		overrides["gitops.repository"] = val
	}
	if val := os.Getenv("GITOPS_BRANCH"); val != "" {
		overrides["gitops.branch"] = val
	}

	// Integration overrides
	if val := os.Getenv("BACKUP_TOOL_ENDPOINT"); val != "" {
		overrides["integration.communication.endpoints.backup_tool"] = val
	}
	if val := os.Getenv("GITOPS_GENERATOR_ENDPOINT"); val != "" {
		overrides["integration.communication.endpoints.gitops_generator"] = val
	}
	if val := os.Getenv("INTEGRATION_BRIDGE_ENDPOINT"); val != "" {
		overrides["integration.communication.endpoints.integration_bridge"] = val
	}

	return overrides
}

// getDefaultConfigPaths returns the default configuration file paths
func getDefaultConfigPaths() []string {
	paths := []string{}

	// Current directory
	if _, err := os.Stat("config.yaml"); err == nil {
		paths = append(paths, "config.yaml")
	}
	if _, err := os.Stat("integration.yaml"); err == nil {
		paths = append(paths, "integration.yaml")
	}

	// Config directory
	configDir := "config"
	if _, err := os.Stat(configDir); err == nil {
		configFiles := []string{
			"integration.yaml",
			"config.yaml", 
			"backup.yaml",
			"gitops.yaml",
		}
		
		for _, file := range configFiles {
			path := filepath.Join(configDir, file)
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
			}
		}
	}

	// Home directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeConfigPath := filepath.Join(homeDir, ".backup-integration", "config.yaml")
		if _, err := os.Stat(homeConfigPath); err == nil {
			paths = append(paths, homeConfigPath)
		}
	}

	// System configuration
	systemPaths := []string{
		"/etc/backup-integration/config.yaml",
		"/etc/backup-integration/integration.yaml",
	}
	for _, path := range systemPaths {
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}

	// If no config files found, use the integration template
	if len(paths) == 0 {
		// Look for the integration template
		templatePath := "integration/config_integration.yaml"
		if _, err := os.Stat(templatePath); err == nil {
			paths = append(paths, templatePath)
		}
	}

	return paths
}

// SaveConfiguration saves the current configuration to a file
func (cm *ConfigManager) SaveConfiguration(filePath string) error {
	if cm.config == nil {
		return fmt.Errorf("no configuration loaded to save")
	}

	// Marshal configuration to YAML
	data, err := yaml.Marshal(cm.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %v", err)
	}
	
	// Write to file
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	
	return nil
}

// ReloadConfiguration reloads configuration from files
func (cm *ConfigManager) ReloadConfiguration() error {
	config, err := cm.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to reload configuration: %v", err)
	}

	cm.config = config
	return nil
}