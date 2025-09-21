package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/http"
	"shared-config/monitoring"
	"shared-config/security"
	"shared-config/triggers"
	"shared-config/restore"
)

// IntegrationBridge provides unified coordination between backup, GitOps, restore, and shared components
type IntegrationBridge struct {
	config           *sharedconfig.SharedConfig
	monitoringSystem *monitoring.MonitoringSystem
	securityManager  *security.SecurityManager
	httpClient       *http.MonitoredHTTPClient
	triggerSystem    *triggers.MonitoredAutoTrigger
	
	// Component systems
	restoreEngine    *restore.RestoreEngine
	restoreAPI       *restore.RestoreAPI
	httpServer       *HTTPServer
	
	// Component status tracking
	backupStatus   ComponentStatus
	gitopsStatus   ComponentStatus
	restoreStatus  ComponentStatus
	bridgeStatus   ComponentStatus
	
	// Event coordination
	eventBus       *EventBus
	webhookHandler *WebhookHandler
	
	// Integrated monitoring
	monitoringIntegration *MonitoringIntegration
	
	// Lifecycle management
	running bool
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// ComponentStatus represents the operational status of a component
type ComponentStatus struct {
	Name         string                 `json:"name"`
	Status       string                 `json:"status"` // healthy, degraded, unhealthy, unknown
	LastCheck    time.Time              `json:"last_check"`
	Version      string                 `json:"version"`
	Metadata     map[string]interface{} `json:"metadata"`
	Dependencies []string               `json:"dependencies"`
}


// EventHandler function type for processing events
type EventHandler func(ctx context.Context, event *IntegrationEvent) error

// IntegrationEvent represents communication between components
type IntegrationEvent struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Source      string                 `json:"source"`
	Target      string                 `json:"target"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// BackupCompletionEvent represents backup operation completion
type BackupCompletionEvent struct {
	BackupID      string    `json:"backup_id"`
	ClusterName   string    `json:"cluster_name"`
	Timestamp     time.Time `json:"timestamp"`
	ResourceCount int       `json:"resource_count"`
	Size          int64     `json:"size_bytes"`
	Success       bool      `json:"success"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	MinIOPath     string    `json:"minio_path"`
}

// GitOpsGenerationRequest represents request for GitOps generation
type GitOpsGenerationRequest struct {
	RequestID     string                 `json:"request_id"`
	BackupID      string                 `json:"backup_id"`
	ClusterName   string                 `json:"cluster_name"`
	SourcePath    string                 `json:"source_path"`
	TargetRepo    string                 `json:"target_repo"`
	TargetBranch  string                 `json:"target_branch"`
	Configuration map[string]interface{} `json:"configuration"`
}

// NewIntegrationBridge creates a new integration bridge
func NewIntegrationBridge(config *sharedconfig.SharedConfig) (*IntegrationBridge, error) {
	if config == nil {
		return nil, fmt.Errorf("shared configuration is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize monitoring system
	logger := monitoring.NewLogger("integration_bridge")
	monitoringSystem := monitoring.NewMonitoringSystem(config, logger)

	// Initialize security manager
	securityManager := security.NewSecurityManager(config)

	// Initialize HTTP client
	httpClient := http.NewMonitoredHTTPClientFromConfig(config, "integration_bridge", logger)

	// Initialize trigger system
	triggerSystem := triggers.NewMonitoredAutoTrigger(config, logger)

	// Initialize restore engine
	restoreEngine, err := restore.NewRestoreEngine(config, monitoringSystem, securityManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create restore engine: %v", err)
	}

	// Initialize restore API
	restoreAPI := restore.NewRestoreAPI(restoreEngine, securityManager, monitoringSystem, config)

	bridge := &IntegrationBridge{
		config:           config,
		monitoringSystem: monitoringSystem,
		securityManager:  securityManager,
		httpClient:       httpClient,
		triggerSystem:    triggerSystem,
		restoreEngine:    restoreEngine,
		restoreAPI:       restoreAPI,
		eventBus:         NewEventBus(config),
		ctx:              ctx,
		cancel:           cancel,
		bridgeStatus: ComponentStatus{
			Name:      "integration-bridge",
			Status:    "initializing",
			LastCheck: time.Now(),
			Version:   "1.0.0",
			Metadata:  make(map[string]interface{}),
		},
		restoreStatus: ComponentStatus{
			Name:      "restore-engine",
			Status:    "initializing",
			LastCheck: time.Now(),
			Version:   "1.0.0",
			Metadata:  make(map[string]interface{}),
		},
	}

	// Initialize webhook handler
	bridge.webhookHandler = NewWebhookHandler(bridge)

	// Initialize monitoring integration
	bridge.monitoringIntegration = NewMonitoringIntegration(bridge)

	// Initialize HTTP server
	bridge.httpServer = NewHTTPServer(bridge, config)

	// Register event handlers
	bridge.registerEventHandlers()

	return bridge, nil
}

// Start initializes and starts the integration bridge
func (ib *IntegrationBridge) Start(ctx context.Context) error {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	if ib.running {
		return fmt.Errorf("integration bridge is already running")
	}

	log.Printf("Starting integration bridge...")

	// Start monitoring system
	if err := ib.monitoringSystem.Start(ctx); err != nil {
		return fmt.Errorf("failed to start monitoring system: %v", err)
	}

	// Register bridge component with monitoring
	if err := ib.monitoringSystem.GetMonitoringHub().RegisterComponent("integration-bridge", ib); err != nil {
		log.Printf("Warning: failed to register bridge with monitoring: %v", err)
	}

	// Start webhook handler
	if err := ib.webhookHandler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start webhook handler: %v", err)
	}

	// Start monitoring integration
	if err := ib.monitoringIntegration.Start(ctx); err != nil {
		log.Printf("Warning: failed to start monitoring integration: %v", err)
	}

	// Start HTTP server
	if err := ib.httpServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %v", err)
	}

	// Update status
	ib.bridgeStatus.Status = "healthy"
	ib.bridgeStatus.LastCheck = time.Now()
	ib.restoreStatus.Status = "healthy"
	ib.restoreStatus.LastCheck = time.Now()
	ib.running = true

	// Start background health monitoring
	go ib.monitorComponentHealth(ctx)

	log.Printf("Integration bridge with restore capabilities started successfully")
	return nil
}

// Stop gracefully shuts down the integration bridge
func (ib *IntegrationBridge) Stop() error {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	if !ib.running {
		return nil
	}

	log.Printf("Stopping integration bridge...")

	// Stop webhook handler
	if err := ib.webhookHandler.Stop(); err != nil {
		log.Printf("Warning: error stopping webhook handler: %v", err)
	}

	// Stop HTTP server
	if err := ib.httpServer.Stop(ib.ctx); err != nil {
		log.Printf("Warning: error stopping HTTP server: %v", err)
	}

	// Stop monitoring integration
	if err := ib.monitoringIntegration.Stop(); err != nil {
		log.Printf("Warning: error stopping monitoring integration: %v", err)
	}

	// Stop monitoring system
	if err := ib.monitoringSystem.Stop(); err != nil {
		log.Printf("Warning: error stopping monitoring system: %v", err)
	}

	// Cancel context
	ib.cancel()

	ib.bridgeStatus.Status = "stopped"
	ib.bridgeStatus.LastCheck = time.Now()
	ib.restoreStatus.Status = "stopped"
	ib.restoreStatus.LastCheck = time.Now()
	ib.running = false

	log.Printf("Integration bridge stopped")
	return nil
}

// TriggerGitOpsGeneration handles backup completion and triggers GitOps generation
func (ib *IntegrationBridge) TriggerGitOpsGeneration(ctx context.Context, event *BackupCompletionEvent) error {
	if !event.Success {
		log.Printf("Backup failed, skipping GitOps generation: %s", event.ErrorMessage)
		return nil
	}

	log.Printf("Triggering GitOps generation for backup %s", event.BackupID)

	// Create GitOps generation request
	request := &GitOpsGenerationRequest{
		RequestID:     fmt.Sprintf("gitops-%s-%d", event.BackupID, time.Now().Unix()),
		BackupID:      event.BackupID,
		ClusterName:   event.ClusterName,
		SourcePath:    event.MinIOPath,
		TargetRepo:    ib.config.GitOps.Repository.URL,
		TargetBranch:  ib.config.GitOps.Repository.Branch,
		Configuration: map[string]interface{}{
			"timestamp":       event.Timestamp,
			"resource_count":  event.ResourceCount,
			"size_bytes":      event.Size,
		},
	}

	// Publish integration event
	integrationEvent := &IntegrationEvent{
		ID:        request.RequestID,
		Type:      "gitops_generation_requested",
		Source:    "backup-tool",
		Target:    "gitops-generator",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"backup_completion": event,
			"generation_request": request,
		},
		Metadata: map[string]interface{}{
			"component": "integration-bridge",
			"version":   ib.bridgeStatus.Version,
		},
	}

	if err := ib.eventBus.Publish(ctx, integrationEvent); err != nil {
		return fmt.Errorf("failed to publish GitOps generation event: %v", err)
	}

	// Trigger using the shared trigger system

	_, err := ib.triggerSystem.TriggerGitOpsGeneration(ctx, &triggers.BackupCompletionEvent{
		BackupID:        event.BackupID,
		ClusterName:     event.ClusterName,
		Timestamp:       event.Timestamp,
		Success:         event.Success,
		MinIOBucket:     ib.config.Storage.Bucket,
		ResourcesCount:  event.ResourceCount,
		BackupSize:      event.Size,
		Errors:          []string{event.ErrorMessage},
	})
	if err != nil {
		return fmt.Errorf("failed to trigger GitOps generation: %v", err)
	}

	// Update metrics
	ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("gitops_triggers_total", 
		map[string]string{"cluster": event.ClusterName, "status": "success"}, 1)

	return nil
}

// GetComponentStatus returns the current status of all components
func (ib *IntegrationBridge) GetComponentStatus() map[string]ComponentStatus {
	ib.mu.RLock()
	defer ib.mu.RUnlock()

	return map[string]ComponentStatus{
		"backup":            ib.backupStatus,
		"gitops":            ib.gitopsStatus,
		"restore":           ib.restoreStatus,
		"integration-bridge": ib.bridgeStatus,
	}
}

// RegisterBackupTool registers the backup tool component
func (ib *IntegrationBridge) RegisterBackupTool(endpoint string, version string) error {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	ib.backupStatus = ComponentStatus{
		Name:      "backup-tool",
		Status:    "registered",
		LastCheck: time.Now(),
		Version:   version,
		Metadata: map[string]interface{}{
			"endpoint": endpoint,
		},
		Dependencies: []string{"kubernetes", "minio"},
	}

	log.Printf("Backup tool registered: %s (version %s)", endpoint, version)
	return nil
}

// RegisterGitOpsTool registers the GitOps generator component
func (ib *IntegrationBridge) RegisterGitOpsTool(endpoint string, version string) error {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	ib.gitopsStatus = ComponentStatus{
		Name:      "gitops-generator",
		Status:    "registered",
		LastCheck: time.Now(),
		Version:   version,
		Metadata: map[string]interface{}{
			"endpoint": endpoint,
		},
		Dependencies: []string{"minio", "git"},
	}

	log.Printf("GitOps generator registered: %s (version %s)", endpoint, version)
	return nil
}

// StartRestore initiates a restore operation through the integration bridge
func (ib *IntegrationBridge) StartRestore(ctx context.Context, request *restore.RestoreRequest) (*restore.RestoreOperation, error) {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	if !ib.running {
		return nil, fmt.Errorf("integration bridge is not running")
	}

	log.Printf("Starting restore operation %s for backup %s", request.RestoreID, request.BackupID)

	// Start restore operation
	operation, err := ib.restoreEngine.StartRestore(ctx, *request)
	if err != nil {
		// Update metrics
		ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter(
			"restore_operations_failed",
			map[string]string{"cluster": request.ClusterName, "reason": "start_failed"},
			1,
		)
		return nil, fmt.Errorf("failed to start restore operation: %v", err)
	}

	// Publish integration event
	integrationEvent := &IntegrationEvent{
		ID:        request.RestoreID,
		Type:      "restore_started",
		Source:    "integration-bridge",
		Target:    "restore-engine",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"restore_request": request,
			"operation_id":    operation.Request.RestoreID,
		},
		Metadata: map[string]interface{}{
			"component": "integration-bridge",
			"version":   ib.bridgeStatus.Version,
		},
	}

	if err := ib.eventBus.Publish(ctx, integrationEvent); err != nil {
		log.Printf("Warning: failed to publish restore start event: %v", err)
	}

	// Update metrics
	ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter(
		"restore_operations_started",
		map[string]string{"cluster": request.ClusterName, "mode": string(request.RestoreMode)},
		1,
	)

	return operation, nil
}

// GetRestoreStatus returns the current status of a restore operation
func (ib *IntegrationBridge) GetRestoreStatus(restoreID string) (*restore.RestoreOperation, error) {
	return ib.restoreEngine.GetRestoreStatus(restoreID)
}

// CancelRestore cancels an active restore operation
func (ib *IntegrationBridge) CancelRestore(restoreID string) error {
	log.Printf("Cancelling restore operation %s", restoreID)
	
	err := ib.restoreEngine.CancelRestore(restoreID)
	if err != nil {
		ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter(
			"restore_operations_cancel_failed",
			map[string]string{"restore_id": restoreID},
			1,
		)
		return err
	}

	// Update metrics
	ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter(
		"restore_operations_cancelled",
		map[string]string{"restore_id": restoreID},
		1,
	)

	return nil
}

// ListActiveRestores returns all currently active restore operations
func (ib *IntegrationBridge) ListActiveRestores() []*restore.RestoreOperation {
	return ib.restoreEngine.ListActiveRestores()
}

// GetRestoreHistory returns historical restore records
func (ib *IntegrationBridge) GetRestoreHistory(limit int) []*restore.RestoreRecord {
	return ib.restoreEngine.GetRestoreHistory(limit)
}

// GetRestoreAPI returns the restore API handler for HTTP routing
func (ib *IntegrationBridge) GetRestoreAPI() *restore.RestoreAPI {
	return ib.restoreAPI
}

// monitorComponentHealth runs background health checks
func (ib *IntegrationBridge) monitorComponentHealth(ctx context.Context) {
	ticker := time.NewTicker(ib.config.Timeouts.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ib.checkComponentHealth(ctx)
		}
	}
}

// checkComponentHealth performs health checks on all components
func (ib *IntegrationBridge) checkComponentHealth(ctx context.Context) {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	// Check backup tool health
	if ib.backupStatus.Name != "" {
		if endpoint, ok := ib.backupStatus.Metadata["endpoint"].(string); ok {
			if err := ib.pingComponent(ctx, endpoint+"/health"); err != nil {
				ib.backupStatus.Status = "unhealthy"
				ib.backupStatus.Metadata["error"] = err.Error()
			} else {
				ib.backupStatus.Status = "healthy"
				delete(ib.backupStatus.Metadata, "error")
			}
			ib.backupStatus.LastCheck = time.Now()
		}
	}

	// Check GitOps tool health
	if ib.gitopsStatus.Name != "" {
		if endpoint, ok := ib.gitopsStatus.Metadata["endpoint"].(string); ok {
			if err := ib.pingComponent(ctx, endpoint+"/health"); err != nil {
				ib.gitopsStatus.Status = "unhealthy"
				ib.gitopsStatus.Metadata["error"] = err.Error()
			} else {
				ib.gitopsStatus.Status = "healthy"
				delete(ib.gitopsStatus.Metadata, "error")
			}
			ib.gitopsStatus.LastCheck = time.Now()
		}
	}

	// Check restore engine health
	if ib.restoreEngine != nil {
		// Check if restore engine is responsive
		activeRestores := len(ib.restoreEngine.ListActiveRestores())
		ib.restoreStatus.Metadata["active_restores"] = activeRestores
		ib.restoreStatus.Status = "healthy"
		ib.restoreStatus.LastCheck = time.Now()
	}

	// Update bridge status
	ib.bridgeStatus.LastCheck = time.Now()
	if ib.running {
		ib.bridgeStatus.Status = "healthy"
	}
}

// pingComponent performs a health check ping to a component
func (ib *IntegrationBridge) pingComponent(ctx context.Context, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, ib.config.Timeouts.PermissionCheckTimeout)
	defer cancel()

	_, err := ib.httpClient.Get(ctx, endpoint)
	return err
}

// registerEventHandlers sets up event handling for the bridge
func (ib *IntegrationBridge) registerEventHandlers() {
	// Handle backup completion events
	ib.eventBus.Subscribe("backup_completed", func(ctx context.Context, event *IntegrationEvent) error {
		if backupData, ok := event.Data["backup_completion"]; ok {
			var backupEvent BackupCompletionEvent
			if data, err := json.Marshal(backupData); err == nil {
				if err := json.Unmarshal(data, &backupEvent); err == nil {
					return ib.TriggerGitOpsGeneration(ctx, &backupEvent)
				}
			}
		}
		return fmt.Errorf("invalid backup completion event data")
	})

	// Handle GitOps generation completion events
	ib.eventBus.Subscribe("gitops_completed", func(ctx context.Context, event *IntegrationEvent) error {
		log.Printf("GitOps generation completed: %s", event.ID)
		
		// Update metrics
		status := "success"
		if errorMsg, ok := event.Data["error"]; ok && errorMsg != nil {
			status = "failure"
		}
		
		ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("gitops_completions_total",
			map[string]string{"status": status}, 1)
		
		return nil
	})

	// Handle restore completion events
	ib.eventBus.Subscribe("restore_completed", func(ctx context.Context, event *IntegrationEvent) error {
		log.Printf("Restore operation completed: %s", event.ID)
		
		// Update metrics
		status := "success"
		if errorMsg, ok := event.Data["error"]; ok && errorMsg != nil {
			status = "failure"
		}
		
		ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("restore_completions_total",
			map[string]string{"status": status}, 1)
		
		return nil
	})

	// Handle restore failure events
	ib.eventBus.Subscribe("restore_failed", func(ctx context.Context, event *IntegrationEvent) error {
		log.Printf("Restore operation failed: %s", event.ID)
		
		// Update metrics
		ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("restore_failures_total",
			map[string]string{"restore_id": event.ID}, 1)
		
		return nil
	})

	// Handle disaster recovery events
	ib.eventBus.Subscribe("disaster_recovery_initiated", func(ctx context.Context, event *IntegrationEvent) error {
		log.Printf("Disaster recovery initiated: %s", event.ID)
		
		// Update metrics
		ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("disaster_recovery_total",
			map[string]string{"scenario": event.Data["scenario"].(string)}, 1)
		
		return nil
	})
}

// Implement MonitoredComponent interface

// GetComponentName returns the component name
func (ib *IntegrationBridge) GetComponentName() string {
	return "integration-bridge"
}

// GetComponentVersion returns the component version
func (ib *IntegrationBridge) GetComponentVersion() string {
	return ib.bridgeStatus.Version
}

// GetMetrics returns component metrics
func (ib *IntegrationBridge) GetMetrics() map[string]interface{} {
	ib.mu.RLock()
	defer ib.mu.RUnlock()

	metrics := make(map[string]interface{})
	
	// Component counts
	componentCount := 0
	healthyCount := 0
	for _, status := range ib.GetComponentStatus() {
		if status.Name != "" {
			componentCount++
			if status.Status == "healthy" {
				healthyCount++
			}
		}
	}

	metrics["total_components"] = componentCount
	metrics["healthy_components"] = healthyCount
	metrics["bridge_running"] = ib.running
	metrics["bridge_uptime_seconds"] = time.Since(ib.bridgeStatus.LastCheck).Seconds()
	
	// Add restore engine metrics
	if ib.restoreEngine != nil {
		activeRestores := len(ib.restoreEngine.ListActiveRestores())
		restoreHistory := len(ib.restoreEngine.GetRestoreHistory(0))
		metrics["active_restores"] = activeRestores
		metrics["total_restore_history"] = restoreHistory
	}

	return metrics
}

// GetMetricsCollector returns the metrics collector
func (ib *IntegrationBridge) GetMetricsCollector() monitoring.MetricsCollector {
	return ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector()
}

// ResetMetrics resets component metrics
func (ib *IntegrationBridge) ResetMetrics() {
	ib.monitoringSystem.GetMonitoringHub().GetMetricsCollector().ResetMetrics()
}

// HealthCheck performs component health check
func (ib *IntegrationBridge) HealthCheck(ctx context.Context) monitoring.HealthStatus {
	ib.mu.RLock()
	defer ib.mu.RUnlock()

	if !ib.running {
		return monitoring.HealthStatus{
			Status:  monitoring.HealthStatusUnhealthy,
			Message: "Integration bridge is not running",
		}
	}

	// Check component health
	statuses := ib.GetComponentStatus()
	unhealthyCount := 0
	for _, status := range statuses {
		if status.Name != "" && status.Status == "unhealthy" {
			unhealthyCount++
		}
	}

	if unhealthyCount > 0 {
		return monitoring.HealthStatus{
			Status:  monitoring.HealthStatusDegraded,
			Message: fmt.Sprintf("%d components are unhealthy", unhealthyCount),
			Metrics: map[string]interface{}{
				"unhealthy_components": unhealthyCount,
			},
		}
	}

	return monitoring.HealthStatus{
		Status:  monitoring.HealthStatusHealthy,
		Message: "All components are healthy",
		Metrics: map[string]interface{}{
			"total_components":   len(statuses),
			"healthy_components": len(statuses) - unhealthyCount,
		},
	}
}

// GetDependencies returns component dependencies
func (ib *IntegrationBridge) GetDependencies() []string {
	return []string{"monitoring", "http-client", "trigger-system", "restore-engine", "security-manager"}
}

// OnStart handles component start lifecycle
func (ib *IntegrationBridge) OnStart(ctx context.Context) error {
	return ib.Start(ctx)
}

// OnStop handles component stop lifecycle
func (ib *IntegrationBridge) OnStop(ctx context.Context) error {
	return ib.Stop()
}

// GetIntegratedMetrics returns comprehensive metrics across all components
func (ib *IntegrationBridge) GetIntegratedMetrics() *IntegratedMetricsAggregator {
	if ib.monitoringIntegration != nil {
		return ib.monitoringIntegration.GetAggregatedMetrics()
	}
	return &IntegratedMetricsAggregator{}
}

// GetComponentHealth returns health information for a specific component
func (ib *IntegrationBridge) GetComponentHealth(componentName string) (*ComponentMetrics, error) {
	if ib.monitoringIntegration != nil {
		return ib.monitoringIntegration.GetComponentHealth(componentName)
	}
	return nil, fmt.Errorf("monitoring integration not available")
}

// GetOverallHealth returns the overall system health status
func (ib *IntegrationBridge) GetOverallHealth() HealthSummary {
	if ib.monitoringIntegration != nil {
		return ib.monitoringIntegration.GetOverallHealth()
	}
	return HealthSummary{
		OverallStatus: "unknown",
	}
}

// RecordIntegrationEvent records an integration flow event for monitoring
func (ib *IntegrationBridge) RecordIntegrationEvent(eventType string, duration time.Duration, success bool) {
	if ib.monitoringIntegration != nil {
		ib.monitoringIntegration.RecordIntegrationEvent(eventType, duration, success)
	}
}