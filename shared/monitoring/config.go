package monitoring

import (
	"context"
	"fmt"
	"time"

	sharedconfig "shared-config/config"
)

// MonitoringSystem provides a complete monitoring setup for the shared configuration system
type MonitoringSystem struct {
	hub              *DefaultMonitoringHub
	config           *MonitoringConfig
	logger           Logger
	sharedConfig     *sharedconfig.SharedConfig
	components       map[string]MonitoredComponent
	running          bool
}

// NewMonitoringSystem creates a new monitoring system
func NewMonitoringSystem(sharedConfig *sharedconfig.SharedConfig, logger Logger) *MonitoringSystem {
	monitoringConfig := createMonitoringConfig(sharedConfig)
	hub := NewMonitoringHub(monitoringConfig, logger)
	
	return &MonitoringSystem{
		hub:          hub,
		config:       monitoringConfig,
		logger:       logger.WithFields(map[string]interface{}{"component": "monitoring_system"}),
		sharedConfig: sharedConfig,
		components:   make(map[string]MonitoredComponent),
	}
}

// createMonitoringConfig creates monitoring configuration from shared config
func createMonitoringConfig(sharedConfig *sharedconfig.SharedConfig) *MonitoringConfig {
	config := &MonitoringConfig{
		// Enable all monitoring by default
		MetricsEnabled: true,
		HealthEnabled:  true,
		EventsEnabled:  true,
		
		// Set reasonable defaults
		MetricsInterval:  30 * time.Second,
		HealthInterval:   60 * time.Second,
		MetricsRetention: 24 * time.Hour,
		EventsRetention:  7 * 24 * time.Hour,
		
		// Export settings
		ExportEnabled:  true,
		ExportInterval: 5 * time.Minute,
		
		// Alerting disabled by default
		AlertingEnabled: false,
		
		// Buffer sizes
		MaxMetricsBuffer: 10000,
		MaxEventsBuffer:  5000,
	}
	
	// Override with shared config settings if available
	if sharedConfig != nil {
		// Use performance settings to inform monitoring configuration
		if sharedConfig.Performance.Limits.MaxConcurrentOperations > 0 {
			// Adjust buffer sizes based on concurrency
			config.MaxMetricsBuffer = sharedConfig.Performance.Limits.MaxConcurrentOperations * 100
			config.MaxEventsBuffer = sharedConfig.Performance.Limits.MaxConcurrentOperations * 50
		}
		
		// Use notification settings for export configuration
		if sharedConfig.Pipeline.Notifications.Enabled && sharedConfig.Pipeline.Notifications.Webhook.URL != "" {
			config.ExportEnabled = true
			config.ExportEndpoint = sharedConfig.Pipeline.Notifications.Webhook.URL
		}
		
		// Enable alerting if notifications are configured
		if sharedConfig.Pipeline.Notifications.Enabled {
			config.AlertingEnabled = true
			config.AlertThresholds = map[string]float64{
				"error_rate":           0.05, // 5% error rate threshold
				"response_time_p95":    5000,  // 5 second P95 response time
				"health_check_failures": 3,    // 3 consecutive health check failures
			}
		}
		
		// Adjust intervals based on automation settings
		if sharedConfig.Pipeline.Automation.Enabled {
			// More frequent monitoring for automated systems
			config.MetricsInterval = 15 * time.Second
			config.HealthInterval = 30 * time.Second
			config.ExportInterval = 2 * time.Minute
		}
	}
	
	return config
}

// InitializeComponents initializes and registers all monitorable components
func (ms *MonitoringSystem) InitializeComponents(ctx context.Context) error {
	ms.logger.Info("initializing_monitoring_components", nil)
	
	// Initialize core monitoring component (self-monitoring)
	if err := ms.initializeCoreComponents(); err != nil {
		return fmt.Errorf("failed to initialize core components: %v", err)
	}
	
	// Initialize HTTP clients
	if err := ms.initializeHTTPClients(); err != nil {
		return fmt.Errorf("failed to initialize HTTP clients: %v", err)
	}
	
	// Initialize config components
	if err := ms.initializeConfigComponents(); err != nil {
		return fmt.Errorf("failed to initialize config components: %v", err)
	}
	
	// Initialize trigger components
	if err := ms.initializeTriggerComponents(); err != nil {
		return fmt.Errorf("failed to initialize trigger components: %v", err)
	}
	
	ms.logger.Info("monitoring_components_initialized", map[string]interface{}{
		"component_count": len(ms.components),
	})
	
	return nil
}

// initializeCoreComponents creates and registers core monitoring components
func (ms *MonitoringSystem) initializeCoreComponents() error {
	// Create a monitoring system component that monitors itself
	systemComponent := &MonitoringSystemComponent{
		name:    "monitoring_system",
		version: "1.0.0",
		system:  ms,
	}
	
	// Register the component with the hub
	if err := ms.hub.RegisterComponent("monitoring_system", systemComponent); err != nil {
		return fmt.Errorf("failed to register monitoring system component: %v", err)
	}
	
	// Add to local components map
	ms.components["monitoring_system"] = systemComponent
	
	ms.logger.Info("core_monitoring_component_registered", map[string]interface{}{
		"component": "monitoring_system",
	})
	
	return nil
}

// initializeHTTPClients creates and registers monitored HTTP clients
func (ms *MonitoringSystem) initializeHTTPClients() error {
	ms.logger.Info("initializing_http_clients", map[string]interface{}{
		"component": "monitoring_system",
	})

	// Create monitoring-enabled HTTP client manager
	httpManager := &HTTPClientManager{
		monitoring: ms,
		clients:    make(map[string]*MonitoredHTTPClientWrapper),
		config:     ms.config,
		logger:     ms.logger,
	}

	// Register the HTTP client manager as a monitored component
	if err := ms.hub.RegisterComponent("http-client-manager", httpManager); err != nil {
		ms.logger.Warn("failed_to_register_http_manager", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	// Store reference for later use
	ms.components["http-client-manager"] = httpManager

	// Initialize default HTTP client profiles
	if err := httpManager.InitializeDefaultClients(); err != nil {
		ms.logger.Warn("failed_to_initialize_default_clients", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	ms.logger.Info("http_clients_initialized", map[string]interface{}{
		"component": "monitoring_system",
		"clients_count": len(httpManager.clients),
	})

	return nil
}

// initializeConfigComponents creates and registers config-related components
func (ms *MonitoringSystem) initializeConfigComponents() error {
	ms.logger.Info("initializing_config_components", map[string]interface{}{
		"component": "monitoring_system",
	})

	// Create config monitoring manager
	configManager := &ConfigManager{
		monitoring:     ms,
		config:         ms.config,
		logger:         ms.logger,
		validationRuns: 0,
		lastValidation: time.Now(),
		validationErrors: make(map[string]error),
	}

	// Register the config manager as a monitored component
	if err := ms.hub.RegisterComponent("config-manager", configManager); err != nil {
		ms.logger.Warn("failed_to_register_config_manager", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	// Store reference for later use
	ms.components["config-manager"] = configManager

	// Perform initial configuration validation
	if err := configManager.ValidateConfiguration(); err != nil {
		ms.logger.Warn("initial_config_validation_failed", map[string]interface{}{
			"error": err.Error(),
		})
		// Don't fail startup due to config validation issues
	}

	// Set up periodic configuration validation
	go configManager.StartPeriodicValidation(context.Background())

	ms.logger.Info("config_components_initialized", map[string]interface{}{
		"component": "monitoring_system",
		"validation_enabled": true,
	})

	return nil
}

// initializeTriggerComponents creates and registers trigger-related components
func (ms *MonitoringSystem) initializeTriggerComponents() error {
	ms.logger.Info("initializing_trigger_components", map[string]interface{}{
		"component": "monitoring_system",
	})

	// Create trigger monitoring manager
	triggerManager := &TriggerManager{
		monitoring:      ms,
		config:          ms.config,
		logger:          ms.logger,
		activeTriggers:  make(map[string]*TriggerStatus),
		triggerHistory:  make([]*TriggerExecution, 0),
		totalExecutions: 0,
		successfulExecutions: 0,
		failedExecutions: 0,
	}

	// Register the trigger manager as a monitored component
	if err := ms.hub.RegisterComponent("trigger-manager", triggerManager); err != nil {
		ms.logger.Warn("failed_to_register_trigger_manager", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	// Store reference for later use
	ms.components["trigger-manager"] = triggerManager

	// Initialize trigger monitoring
	if err := triggerManager.InitializeTriggerMonitoring(); err != nil {
		ms.logger.Warn("failed_to_initialize_trigger_monitoring", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	// Start trigger health monitoring
	go triggerManager.StartTriggerHealthMonitoring(context.Background())

	ms.logger.Info("trigger_components_initialized", map[string]interface{}{
		"component": "monitoring_system",
		"monitoring_enabled": true,
	})

	return nil
}

// Start starts the monitoring system
func (ms *MonitoringSystem) Start(ctx context.Context) error {
	if ms.running {
		return fmt.Errorf("monitoring system already running")
	}
	
	ms.logger.Info("starting_monitoring_system", map[string]interface{}{
		"components": len(ms.components),
	})
	
	// Start the monitoring hub
	if err := ms.hub.Start(ctx); err != nil {
		return fmt.Errorf("failed to start monitoring hub: %v", err)
	}
	
	// Configure event publishers with exporters
	if err := ms.configureEventExporters(); err != nil {
		ms.logger.Error("failed_to_configure_event_exporters", map[string]interface{}{
			"error": err.Error(),
		})
		// Non-fatal error, continue starting
	}
	
	ms.running = true
	
	ms.logger.Info("monitoring_system_started", map[string]interface{}{
		"metrics_enabled": ms.config.MetricsEnabled,
		"health_enabled":  ms.config.HealthEnabled,
		"events_enabled":  ms.config.EventsEnabled,
		"export_enabled":  ms.config.ExportEnabled,
	})
	
	return nil
}

// Stop stops the monitoring system
func (ms *MonitoringSystem) Stop() error {
	if !ms.running {
		return fmt.Errorf("monitoring system not running")
	}
	
	ms.logger.Info("stopping_monitoring_system", nil)
	
	// Stop the monitoring hub
	if err := ms.hub.Stop(); err != nil {
		ms.logger.Error("failed_to_stop_monitoring_hub", map[string]interface{}{
			"error": err.Error(),
		})
	}
	
	ms.running = false
	
	ms.logger.Info("monitoring_system_stopped", nil)
	
	return nil
}

// configureEventExporters sets up event exporters based on configuration
func (ms *MonitoringSystem) configureEventExporters() error {
	eventPublisher := ms.hub.GetEventPublisher()
	
	// Always add console exporter for development
	consoleExporter := NewConsoleEventExporter(ms.logger)
	if err := eventPublisher.SetExporter(consoleExporter); err != nil {
		return fmt.Errorf("failed to set console exporter: %v", err)
	}
	
	// Add webhook exporter if configured
	if ms.config.ExportEnabled && ms.config.ExportEndpoint != "" {
		webhookExporter := NewHTTPEventExporter(ms.config.ExportEndpoint)
		webhookExporter.Configure(ExporterConfig{
			Type:     "webhook",
			Endpoint: ms.config.ExportEndpoint,
		})
		
		if err := eventPublisher.SetExporter(webhookExporter); err != nil {
			return fmt.Errorf("failed to set webhook exporter: %v", err)
		}
	}
	
	// Add file exporter for persistence
	fileExporter := NewFileEventExporter("/var/log/backup-gitops-events.log")
	if err := eventPublisher.SetExporter(fileExporter); err != nil {
		return fmt.Errorf("failed to set file exporter: %v", err)
	}
	
	return nil
}

// GetSystemHealth returns overall system health
func (ms *MonitoringSystem) GetSystemHealth() SystemHealth {
	return ms.hub.GetSystemHealth()
}

// GetAggregatedMetrics returns aggregated metrics from all components
func (ms *MonitoringSystem) GetAggregatedMetrics() AggregatedMetrics {
	return ms.hub.GetAggregatedMetrics()
}

// GetComponent returns a specific monitored component
func (ms *MonitoringSystem) GetComponent(name string) (MonitoredComponent, bool) {
	component, exists := ms.components[name]
	return component, exists
}

// ListComponents returns names of all registered components
func (ms *MonitoringSystem) ListComponents() []string {
	names := make([]string, 0, len(ms.components))
	for name := range ms.components {
		names = append(names, name)
	}
	return names
}

// GetMonitoringHub returns the monitoring hub for advanced usage
func (ms *MonitoringSystem) GetMonitoringHub() *DefaultMonitoringHub {
	return ms.hub
}

// CreateDefaultMonitoringSystem creates a fully configured monitoring system
func CreateDefaultMonitoringSystem(sharedConfig *sharedconfig.SharedConfig) (*MonitoringSystem, error) {
	// Create logger
	logger := NewLogger("monitoring_system")
	
	// Create monitoring system
	ms := NewMonitoringSystem(sharedConfig, logger)
	
	// Initialize components
	ctx := context.Background()
	if err := ms.InitializeComponents(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize monitoring components: %v", err)
	}
	
	return ms, nil
}

// MonitoringInitializer provides helper functions for quick monitoring setup
type MonitoringInitializer struct {
	system *MonitoringSystem
	logger Logger
}

// NewMonitoringInitializer creates a new monitoring initializer
func NewMonitoringInitializer() *MonitoringInitializer {
	logger := NewLogger("monitoring_initializer")
	return &MonitoringInitializer{
		logger: logger,
	}
}

// InitializeFromConfig initializes monitoring from a shared config file
func (mi *MonitoringInitializer) InitializeFromConfig(configPath string) (*MonitoringSystem, error) {
	mi.logger.Info("initializing_monitoring_from_config", map[string]interface{}{
		"config_path": configPath,
	})
	
	// Load shared configuration
	loader := sharedconfig.NewConfigLoader(configPath)
	sharedConfig, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load shared config: %v", err)
	}
	
	// Create and initialize monitoring system
	system, err := CreateDefaultMonitoringSystem(sharedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring system: %v", err)
	}
	
	mi.system = system
	
	mi.logger.Info("monitoring_initialized_successfully", map[string]interface{}{
		"components": len(system.ListComponents()),
	})
	
	return system, nil
}

// StartWithAutoSetup starts monitoring with automatic configuration detection
func (mi *MonitoringInitializer) StartWithAutoSetup(ctx context.Context) (*MonitoringSystem, error) {
	mi.logger.Info("starting_monitoring_with_auto_setup", nil)
	
	// Try to find shared configuration automatically
	configPaths := []string{
		"shared-config.yaml",
		"./config/shared-config.yaml",
		"../shared/config/shared-config.yaml",
		"/etc/backup-gitops/config.yaml",
	}
	
	var sharedConfig *sharedconfig.SharedConfig
	var err error
	
	for _, path := range configPaths {
		loader := sharedconfig.NewConfigLoader(path)
		sharedConfig, err = loader.Load()
		if err == nil {
			mi.logger.Info("found_shared_config", map[string]interface{}{
				"path": path,
			})
			break
		}
	}
	
	// If no config found, use minimal defaults
	if sharedConfig == nil {
		mi.logger.Info("no_shared_config_found_using_defaults", nil)
		sharedConfig = &sharedconfig.SharedConfig{
			SchemaVersion: "1.0",
			Performance: sharedconfig.PerformanceConfig{
				Limits: sharedconfig.LimitsConfig{
					MaxConcurrentOperations: 10,
				},
			},
		}
	}
	
	// Create and start monitoring system
	system, err := CreateDefaultMonitoringSystem(sharedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring system: %v", err)
	}
	
	if err := system.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start monitoring system: %v", err)
	}
	
	mi.system = system
	
	mi.logger.Info("monitoring_started_successfully", map[string]interface{}{
		"components":      len(system.ListComponents()),
		"metrics_enabled": system.config.MetricsEnabled,
		"health_enabled":  system.config.HealthEnabled,
		"events_enabled":  system.config.EventsEnabled,
	})
	
	return system, nil
}

// GetSystem returns the initialized monitoring system
func (mi *MonitoringInitializer) GetSystem() *MonitoringSystem {
	return mi.system
}

// Cleanup performs cleanup when shutting down
func (mi *MonitoringInitializer) Cleanup() error {
	if mi.system != nil && mi.system.running {
		return mi.system.Stop()
	}
	return nil
}

// MonitoringSystemComponent implements MonitoredComponent for the monitoring system itself
type MonitoringSystemComponent struct {
	name    string
	version string
	system  *MonitoringSystem
}

func (msc *MonitoringSystemComponent) GetComponentName() string {
	return msc.name
}

func (msc *MonitoringSystemComponent) GetComponentVersion() string {
	return msc.version
}

func (msc *MonitoringSystemComponent) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})
	
	if msc.system != nil {
		// Get basic system metrics
		aggregated := msc.system.GetAggregatedMetrics()
		health := msc.system.GetSystemHealth()
		
		metrics["total_components"] = len(msc.system.ListComponents())
		metrics["healthy_components"] = health.OverallHealth.Summary.HealthyComponents
		metrics["degraded_components"] = health.OverallHealth.Summary.DegradedComponents
		metrics["unhealthy_components"] = health.OverallHealth.Summary.UnhealthyComponents
		metrics["system_uptime_seconds"] = health.SystemMetrics.Uptime.Seconds()
		metrics["memory_usage_percent"] = health.SystemMetrics.MemoryUsage
		metrics["cpu_usage_percent"] = health.SystemMetrics.CPUUsage
		metrics["total_metrics"] = len(aggregated.SystemMetrics)
		metrics["is_running"] = msc.system.running
	}
	
	return metrics
}

func (msc *MonitoringSystemComponent) GetMetricsCollector() MetricsCollector {
	if msc.system != nil && msc.system.hub != nil {
		return msc.system.hub.GetMetricsCollector()
	}
	
	// Return a default collector if system is not available
	config := &MonitoringConfig{
		MetricsEnabled: true,
		MaxMetricsBuffer: 1000,
	}
	return NewMetricsCollector(config)
}

func (msc *MonitoringSystemComponent) ResetMetrics() {
	if msc.system != nil && msc.system.hub != nil {
		collector := msc.system.hub.GetMetricsCollector()
		collector.ResetMetrics()
	}
}

func (msc *MonitoringSystemComponent) HealthCheck(ctx context.Context) HealthStatus {
	if msc.system == nil {
		return HealthStatus{
			Status:  HealthStatusUnhealthy,
			Message: "Monitoring system not initialized",
		}
	}
	
	if !msc.system.running {
		return HealthStatus{
			Status:  HealthStatusDegraded,
			Message: "Monitoring system not running",
		}
	}
	
	// Check if core components are working
	health := msc.system.GetSystemHealth()
	if health.OverallHealth.Summary.UnhealthyComponents > 0 {
		return HealthStatus{
			Status:  HealthStatusDegraded,
			Message: fmt.Sprintf("System has %d unhealthy components", health.OverallHealth.Summary.UnhealthyComponents),
			Metrics: map[string]interface{}{
				"unhealthy_components": health.OverallHealth.Summary.UnhealthyComponents,
				"total_components": health.OverallHealth.Summary.TotalComponents,
			},
		}
	}
	
	return HealthStatus{
		Status:  HealthStatusHealthy,
		Message: "Monitoring system operating normally",
		Metrics: map[string]interface{}{
			"total_components": health.OverallHealth.Summary.TotalComponents,
			"healthy_components": health.OverallHealth.Summary.HealthyComponents,
		},
	}
}

func (msc *MonitoringSystemComponent) GetDependencies() []string {
	// Monitoring system doesn't have external dependencies by default
	return []string{}
}

func (msc *MonitoringSystemComponent) OnStart(ctx context.Context) error {
	// Nothing special to do on start
	return nil
}

func (msc *MonitoringSystemComponent) OnStop(ctx context.Context) error {
	// Nothing special to do on stop
	return nil
}

// HTTPClientManager manages HTTP client monitoring
type HTTPClientManager struct {
	monitoring *MonitoringSystem
	clients    map[string]*MonitoredHTTPClientWrapper
	config     *MonitoringConfig
	logger     Logger
}

// MonitoredHTTPClientWrapper wraps HTTP client with monitoring
type MonitoredHTTPClientWrapper struct {
	name           string
	requestCount   int64
	errorCount     int64
	totalDuration  time.Duration
	lastUsed       time.Time
	healthStatus   string
}

func (hcm *HTTPClientManager) GetComponentName() string {
	return "http-client-manager"
}

func (hcm *HTTPClientManager) GetComponentVersion() string {
	return "1.0.0"
}

func (hcm *HTTPClientManager) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})
	metrics["total_clients"] = len(hcm.clients)
	
	var totalRequests, totalErrors int64
	for _, client := range hcm.clients {
		totalRequests += client.requestCount
		totalErrors += client.errorCount
	}
	
	metrics["total_requests"] = totalRequests
	metrics["total_errors"] = totalErrors
	if totalRequests > 0 {
		metrics["error_rate"] = float64(totalErrors) / float64(totalRequests)
	}
	
	return metrics
}

func (hcm *HTTPClientManager) GetMetricsCollector() MetricsCollector {
	return hcm.monitoring.hub.GetMetricsCollector()
}

func (hcm *HTTPClientManager) ResetMetrics() {
	for _, client := range hcm.clients {
		client.requestCount = 0
		client.errorCount = 0
		client.totalDuration = 0
	}
}

func (hcm *HTTPClientManager) HealthCheck(ctx context.Context) HealthStatus {
	healthyClients := 0
	for _, client := range hcm.clients {
		if client.healthStatus == "healthy" {
			healthyClients++
		}
	}
	
	if len(hcm.clients) == 0 {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: "No HTTP clients configured",
		}
	}
	
	if healthyClients == len(hcm.clients) {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: fmt.Sprintf("All %d HTTP clients healthy", len(hcm.clients)),
		}
	}
	
	return HealthStatus{
		Status:  HealthStatusDegraded,
		Message: fmt.Sprintf("%d/%d HTTP clients healthy", healthyClients, len(hcm.clients)),
	}
}

func (hcm *HTTPClientManager) GetDependencies() []string {
	return []string{}
}

func (hcm *HTTPClientManager) OnStart(ctx context.Context) error {
	return nil
}

func (hcm *HTTPClientManager) OnStop(ctx context.Context) error {
	return nil
}

func (hcm *HTTPClientManager) InitializeDefaultClients() error {
	// Initialize common HTTP client profiles
	defaultClient := &MonitoredHTTPClientWrapper{
		name:         "default",
		healthStatus: "healthy",
		lastUsed:     time.Now(),
	}
	hcm.clients["default"] = defaultClient
	
	webhookClient := &MonitoredHTTPClientWrapper{
		name:         "webhook",
		healthStatus: "healthy", 
		lastUsed:     time.Now(),
	}
	hcm.clients["webhook"] = webhookClient
	
	return nil
}

// ConfigManager manages configuration monitoring
type ConfigManager struct {
	monitoring       *MonitoringSystem
	config           *MonitoringConfig
	logger           Logger
	validationRuns   int64
	lastValidation   time.Time
	validationErrors map[string]error
}

func (cm *ConfigManager) GetComponentName() string {
	return "config-manager"
}

func (cm *ConfigManager) GetComponentVersion() string {
	return "1.0.0"
}

func (cm *ConfigManager) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})
	metrics["validation_runs"] = cm.validationRuns
	metrics["validation_errors"] = len(cm.validationErrors)
	metrics["last_validation"] = cm.lastValidation.Unix()
	
	return metrics
}

func (cm *ConfigManager) GetMetricsCollector() MetricsCollector {
	return cm.monitoring.hub.GetMetricsCollector()
}

func (cm *ConfigManager) ResetMetrics() {
	cm.validationRuns = 0
	cm.validationErrors = make(map[string]error)
}

func (cm *ConfigManager) HealthCheck(ctx context.Context) HealthStatus {
	if len(cm.validationErrors) > 0 {
		return HealthStatus{
			Status:  HealthStatusDegraded,
			Message: fmt.Sprintf("Configuration has %d validation errors", len(cm.validationErrors)),
			Metrics: map[string]interface{}{
				"validation_errors": len(cm.validationErrors),
			},
		}
	}
	
	return HealthStatus{
		Status:  HealthStatusHealthy,
		Message: "Configuration validation successful",
		Metrics: map[string]interface{}{
			"validation_runs": cm.validationRuns,
		},
	}
}

func (cm *ConfigManager) GetDependencies() []string {
	return []string{}
}

func (cm *ConfigManager) OnStart(ctx context.Context) error {
	return nil
}

func (cm *ConfigManager) OnStop(ctx context.Context) error {
	return nil
}

func (cm *ConfigManager) ValidateConfiguration() error {
	cm.validationRuns++
	cm.lastValidation = time.Now()
	
	// Perform basic configuration validation
	if cm.config == nil {
		err := fmt.Errorf("monitoring config is nil")
		cm.validationErrors["config_nil"] = err
		return err
	}
	
	// Validate buffer sizes
	if cm.config.MaxMetricsBuffer <= 0 {
		err := fmt.Errorf("invalid metrics buffer size: %d", cm.config.MaxMetricsBuffer)
		cm.validationErrors["metrics_buffer"] = err
		return err
	}
	
	if cm.config.MaxEventsBuffer <= 0 {
		err := fmt.Errorf("invalid events buffer size: %d", cm.config.MaxEventsBuffer)
		cm.validationErrors["events_buffer"] = err
		return err
	}
	
	// Clear previous errors if validation passes
	cm.validationErrors = make(map[string]error)
	return nil
}

func (cm *ConfigManager) StartPeriodicValidation(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cm.ValidateConfiguration(); err != nil {
				cm.logger.Warn("periodic_config_validation_failed", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}
}

// TriggerManager manages trigger monitoring
type TriggerManager struct {
	monitoring           *MonitoringSystem
	config               *MonitoringConfig
	logger               Logger
	activeTriggers       map[string]*TriggerStatus
	triggerHistory       []*TriggerExecution
	totalExecutions      int64
	successfulExecutions int64
	failedExecutions     int64
}

type TriggerStatus struct {
	Name         string
	Status       string
	LastExecution time.Time
	ExecutionCount int64
}

type TriggerExecution struct {
	TriggerName string
	StartTime   time.Time
	Duration    time.Duration
	Success     bool
	Error       string
}

func (tm *TriggerManager) GetComponentName() string {
	return "trigger-manager"
}

func (tm *TriggerManager) GetComponentVersion() string {
	return "1.0.0"
}

func (tm *TriggerManager) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})
	metrics["active_triggers"] = len(tm.activeTriggers)
	metrics["total_executions"] = tm.totalExecutions
	metrics["successful_executions"] = tm.successfulExecutions
	metrics["failed_executions"] = tm.failedExecutions
	
	if tm.totalExecutions > 0 {
		metrics["success_rate"] = float64(tm.successfulExecutions) / float64(tm.totalExecutions)
	}
	
	return metrics
}

func (tm *TriggerManager) GetMetricsCollector() MetricsCollector {
	return tm.monitoring.hub.GetMetricsCollector()
}

func (tm *TriggerManager) ResetMetrics() {
	tm.totalExecutions = 0
	tm.successfulExecutions = 0
	tm.failedExecutions = 0
	tm.triggerHistory = make([]*TriggerExecution, 0)
}

func (tm *TriggerManager) HealthCheck(ctx context.Context) HealthStatus {
	healthyTriggers := 0
	for _, trigger := range tm.activeTriggers {
		if trigger.Status == "healthy" {
			healthyTriggers++
		}
	}
	
	if len(tm.activeTriggers) == 0 {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: "No triggers configured",
		}
	}
	
	if healthyTriggers == len(tm.activeTriggers) {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: fmt.Sprintf("All %d triggers healthy", len(tm.activeTriggers)),
		}
	}
	
	return HealthStatus{
		Status:  HealthStatusDegraded,
		Message: fmt.Sprintf("%d/%d triggers healthy", healthyTriggers, len(tm.activeTriggers)),
	}
}

func (tm *TriggerManager) GetDependencies() []string {
	return []string{}
}

func (tm *TriggerManager) OnStart(ctx context.Context) error {
	return nil
}

func (tm *TriggerManager) OnStop(ctx context.Context) error {
	return nil
}

func (tm *TriggerManager) InitializeTriggerMonitoring() error {
	// Initialize basic trigger monitoring
	tm.activeTriggers["backup_completion"] = &TriggerStatus{
		Name:           "backup_completion",
		Status:         "healthy",
		LastExecution:  time.Now(),
		ExecutionCount: 0,
	}
	
	tm.activeTriggers["gitops_generation"] = &TriggerStatus{
		Name:           "gitops_generation",
		Status:         "healthy",
		LastExecution:  time.Now(),
		ExecutionCount: 0,
	}
	
	return nil
}

func (tm *TriggerManager) StartTriggerHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.checkTriggerHealth()
		}
	}
}

func (tm *TriggerManager) checkTriggerHealth() {
	now := time.Now()
	for name, trigger := range tm.activeTriggers {
		// Mark trigger as unhealthy if no execution in last 10 minutes
		if now.Sub(trigger.LastExecution) > 10*time.Minute && trigger.ExecutionCount > 0 {
			trigger.Status = "degraded"
		} else {
			trigger.Status = "healthy"
		}
		tm.activeTriggers[name] = trigger
	}
}