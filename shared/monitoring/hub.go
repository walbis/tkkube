package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DefaultMonitoringHub provides a centralized monitoring coordination implementation
type DefaultMonitoringHub struct {
	components       map[string]MonitoredComponent
	metricsCollector MetricsCollector
	healthMonitor    HealthMonitor
	eventPublisher   EventPublisher
	config           *MonitoringConfig
	logger           Logger
	mu               sync.RWMutex
	running          bool
	stopChan         chan struct{}
	aggregator       *MetricsAggregator
}

// NewMonitoringHub creates a new monitoring hub
func NewMonitoringHub(config *MonitoringConfig, logger Logger) *DefaultMonitoringHub {
	hub := &DefaultMonitoringHub{
		components:       make(map[string]MonitoredComponent),
		metricsCollector: NewMetricsCollector(config),
		healthMonitor:    NewHealthMonitor(config),
		eventPublisher:   NewEventPublisher(config, logger),
		config:           config,
		logger:           logger,
		stopChan:         make(chan struct{}),
		aggregator:       NewMetricsAggregator(),
	}
	
	// Register the hub's own metrics collector
	hub.aggregator.RegisterCollector("monitoring_hub", hub.metricsCollector)
	
	return hub
}

// RegisterComponent registers a component for monitoring
func (mh *DefaultMonitoringHub) RegisterComponent(name string, component MonitoredComponent) error {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	
	if _, exists := mh.components[name]; exists {
		return fmt.Errorf("component '%s' already registered", name)
	}
	
	mh.components[name] = component
	
	// Register component's health check
	healthCheck := func(ctx context.Context) HealthStatus {
		return component.HealthCheck(ctx)
	}
	mh.healthMonitor.RegisterHealthCheck(name, healthCheck)
	
	// Register component's metrics collector if it implements the interface
	if metricsProvider, ok := component.(interface{ GetMetricsCollector() MetricsCollector }); ok {
		mh.aggregator.RegisterCollector(name, metricsProvider.GetMetricsCollector())
	}
	
	// Register component dependencies
	for _, dep := range component.GetDependencies() {
		if hm, ok := mh.healthMonitor.(*DefaultHealthMonitor); ok {
			hm.RegisterDependency(dep, "", "component_dependency")
		}
	}
	
	mh.logger.Info("component_registered", map[string]interface{}{
		"component": name,
		"version":   component.GetComponentVersion(),
	})
	
	// Publish component registration event
	event := SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "component_registered",
		Component: name,
		Action:    "register",
		Status:    "success",
		Metadata: map[string]interface{}{
			"version": component.GetComponentVersion(),
		},
	}
	mh.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

// UnregisterComponent removes a component from monitoring
func (mh *DefaultMonitoringHub) UnregisterComponent(name string) error {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	
	if _, exists := mh.components[name]; !exists {
		return fmt.Errorf("component '%s' not found", name)
	}
	
	delete(mh.components, name)
	mh.aggregator.UnregisterCollector(name)
	
	mh.logger.Info("component_unregistered", map[string]interface{}{
		"component": name,
	})
	
	// Publish component unregistration event
	event := SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "component_unregistered",
		Component: name,
		Action:    "unregister",
		Status:    "success",
	}
	mh.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

// GetAggregatedMetrics returns aggregated metrics from all components
func (mh *DefaultMonitoringHub) GetAggregatedMetrics() AggregatedMetrics {
	return mh.aggregator.GetAggregatedMetrics()
}

// GetSystemHealth returns overall system health
func (mh *DefaultMonitoringHub) GetSystemHealth() SystemHealth {
	overallHealth := mh.healthMonitor.GetOverallHealth()
	
	// Add system-level metrics
	systemMetrics := SystemHealthMetrics{
		Uptime:             mh.getUptime(),
		MemoryUsage:        mh.getMemoryUsage(),
		CPUUsage:           mh.getCPUUsage(),
		DiskUsage:          mh.getDiskUsage(),
		NetworkLatency:     mh.getNetworkLatency(),
		ActiveConnections:  mh.getActiveConnections(),
		GoroutineCount:     mh.getGoroutineCount(),
	}
	
	return SystemHealth{
		Status:             overallHealth.Status,
		OverallHealth:      overallHealth,
		ComponentHealth:    overallHealth.Components,
		DependencyHealth:   overallHealth.Dependencies,
		SystemMetrics:      systemMetrics,
		Timestamp:          time.Now(),
	}
}

// BroadcastEvent broadcasts an event to all registered components
func (mh *DefaultMonitoringHub) BroadcastEvent(event Event) error {
	mh.mu.RLock()
	components := make(map[string]MonitoredComponent)
	for name, comp := range mh.components {
		components[name] = comp
	}
	mh.mu.RUnlock()
	
	// Publish the event
	err := mh.eventPublisher.PublishEvent(event)
	if err != nil {
		mh.logger.Error("failed_to_publish_event", map[string]interface{}{
			"event_id":   event.GetID(),
			"event_type": event.GetType(),
			"error":      err.Error(),
		})
		return err
	}
	
	// Notify components that implement event handling
	for name, component := range components {
		if eventHandler, ok := component.(interface{ HandleEvent(Event) error }); ok {
			if err := eventHandler.HandleEvent(event); err != nil {
				mh.logger.Error("component_event_handling_failed", map[string]interface{}{
					"component":  name,
					"event_id":   event.GetID(),
					"event_type": event.GetType(),
					"error":      err.Error(),
				})
			}
		}
	}
	
	return nil
}

// Configure updates the monitoring hub configuration
func (mh *DefaultMonitoringHub) Configure(config MonitoringConfig) error {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	
	mh.config = &config
	
	mh.logger.Info("monitoring_hub_configured", map[string]interface{}{
		"metrics_enabled": config.MetricsEnabled,
		"health_enabled":  config.HealthEnabled,
		"events_enabled":  config.EventsEnabled,
	})
	
	return nil
}

// Start starts the monitoring hub
func (mh *DefaultMonitoringHub) Start(ctx context.Context) error {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	
	if mh.running {
		return fmt.Errorf("monitoring hub already running")
	}
	
	// Start health monitoring if enabled
	if mh.config == nil || mh.config.HealthEnabled {
		if err := mh.healthMonitor.StartHealthChecks(ctx); err != nil {
			return fmt.Errorf("failed to start health checks: %v", err)
		}
	}
	
	// Start components
	for name, component := range mh.components {
		if err := component.OnStart(ctx); err != nil {
			mh.logger.Error("component_start_failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
		}
	}
	
	mh.running = true
	
	// Start background monitoring tasks
	go mh.monitoringLoop(ctx)
	
	mh.logger.Info("monitoring_hub_started", map[string]interface{}{
		"components_count": len(mh.components),
	})
	
	// Publish hub start event
	event := SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "monitoring_hub_started",
		Component: "monitoring_hub",
		Action:    "start",
		Status:    "success",
		Metadata: map[string]interface{}{
			"components_count": len(mh.components),
		},
	}
	mh.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

// Stop stops the monitoring hub
func (mh *DefaultMonitoringHub) Stop() error {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	
	if !mh.running {
		return fmt.Errorf("monitoring hub not running")
	}
	
	// Stop health monitoring
	if err := mh.healthMonitor.StopHealthChecks(); err != nil {
		mh.logger.Error("failed_to_stop_health_checks", map[string]interface{}{
			"error": err.Error(),
		})
	}
	
	// Stop components
	ctx := context.Background()
	for name, component := range mh.components {
		if err := component.OnStop(ctx); err != nil {
			mh.logger.Error("component_stop_failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
		}
	}
	
	// Stop background tasks
	close(mh.stopChan)
	
	// Close event publisher
	if err := mh.eventPublisher.Close(); err != nil {
		mh.logger.Error("failed_to_close_event_publisher", map[string]interface{}{
			"error": err.Error(),
		})
	}
	
	mh.running = false
	
	mh.logger.Info("monitoring_hub_stopped", nil)
	
	// Publish hub stop event (best effort)
	event := SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "monitoring_hub_stopped",
		Component: "monitoring_hub",
		Action:    "stop",
		Status:    "success",
	}
	mh.eventPublisher.PublishSystemEvent(event)
	
	return nil
}

// monitoringLoop runs background monitoring tasks
func (mh *DefaultMonitoringHub) monitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mh.stopChan:
			return
		case <-ticker.C:
			mh.performPeriodicTasks(ctx)
		}
	}
}

// performPeriodicTasks executes periodic monitoring tasks
func (mh *DefaultMonitoringHub) performPeriodicTasks(ctx context.Context) {
	// Collect system metrics
	if mh.config == nil || mh.config.MetricsEnabled {
		mh.collectSystemMetrics()
	}
	
	// Check for component health changes
	mh.checkHealthChanges()
	
	// Clean up old metrics if configured
	if mh.config != nil && mh.config.MetricsRetention > 0 {
		mh.cleanupOldMetrics()
	}
}

// collectSystemMetrics collects system-level metrics
func (mh *DefaultMonitoringHub) collectSystemMetrics() {
	_ = time.Now()
	
	// Memory usage
	memUsage := mh.getMemoryUsage()
	mh.metricsCollector.SetGauge(ResourceMemoryUsage, nil, memUsage)
	
	// CPU usage
	cpuUsage := mh.getCPUUsage()
	mh.metricsCollector.SetGauge(ResourceCPUUsage, nil, cpuUsage)
	
	// Component count
	mh.mu.RLock()
	componentCount := float64(len(mh.components))
	mh.mu.RUnlock()
	mh.metricsCollector.SetGauge("monitoring_hub_components_total", nil, componentCount)
	
	// Health status metrics
	health := mh.healthMonitor.GetOverallHealth()
	mh.metricsCollector.SetGauge("monitoring_hub_healthy_components", nil, float64(health.Summary.HealthyComponents))
	mh.metricsCollector.SetGauge("monitoring_hub_degraded_components", nil, float64(health.Summary.DegradedComponents))
	mh.metricsCollector.SetGauge("monitoring_hub_unhealthy_components", nil, float64(health.Summary.UnhealthyComponents))
	
	// Uptime
	uptimeSeconds := mh.getUptime().Seconds()
	mh.metricsCollector.SetGauge(ComponentUptime, map[string]string{"component": "monitoring_hub"}, uptimeSeconds)
}

// checkHealthChanges monitors for health status changes
func (mh *DefaultMonitoringHub) checkHealthChanges() {
	health := mh.healthMonitor.GetOverallHealth()
	
	// Check for unhealthy components and publish alerts
	for name, status := range health.Components {
		if status.Status == HealthStatusUnhealthy {
			event := ErrorEvent{
				ID:        generateEventID(),
				Timestamp: time.Now(),
				Component: name,
				Operation: "health_check",
				Severity:  "critical",
				Context: map[string]interface{}{
					"health_status": status.Status,
					"health_message": status.Message,
				},
			}
			mh.eventPublisher.PublishErrorEvent(event)
		}
	}
}

// cleanupOldMetrics removes old metrics based on retention policy
func (mh *DefaultMonitoringHub) cleanupOldMetrics() {
	// This would typically involve cleaning up metrics older than the retention period
	// For now, just reset metrics if they're too old
	mh.metricsCollector.ResetMetrics()
}

// System metrics collection helpers (mock implementations)
func (mh *DefaultMonitoringHub) getUptime() time.Duration {
	// This would integrate with actual system uptime
	return time.Hour * 24 // Mock: 24 hours uptime
}

func (mh *DefaultMonitoringHub) getMemoryUsage() float64 {
	// This would integrate with runtime.MemStats
	return 45.5 // Mock: 45.5% memory usage
}

func (mh *DefaultMonitoringHub) getCPUUsage() float64 {
	// This would integrate with system CPU monitoring
	return 23.2 // Mock: 23.2% CPU usage
}

func (mh *DefaultMonitoringHub) getDiskUsage() float64 {
	// This would integrate with disk usage monitoring
	return 67.8 // Mock: 67.8% disk usage
}

func (mh *DefaultMonitoringHub) getNetworkLatency() time.Duration {
	// This would integrate with network latency monitoring
	return time.Millisecond * 15 // Mock: 15ms latency
}

func (mh *DefaultMonitoringHub) getActiveConnections() int64 {
	// This would integrate with connection monitoring
	return 42 // Mock: 42 active connections
}

func (mh *DefaultMonitoringHub) getGoroutineCount() int64 {
	// This would integrate with runtime.NumGoroutine()
	return 25 // Mock: 25 goroutines
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

// GetMetricsCollector returns the hub's metrics collector
func (mh *DefaultMonitoringHub) GetMetricsCollector() MetricsCollector {
	return mh.metricsCollector
}

// GetHealthMonitor returns the hub's health monitor
func (mh *DefaultMonitoringHub) GetHealthMonitor() HealthMonitor {
	return mh.healthMonitor
}

// GetEventPublisher returns the hub's event publisher
func (mh *DefaultMonitoringHub) GetEventPublisher() EventPublisher {
	return mh.eventPublisher
}

// ListComponents returns a list of registered component names
func (mh *DefaultMonitoringHub) ListComponents() []string {
	mh.mu.RLock()
	defer mh.mu.RUnlock()
	
	components := make([]string, 0, len(mh.components))
	for name := range mh.components {
		components = append(components, name)
	}
	return components
}

// GetComponent returns a specific component by name
func (mh *DefaultMonitoringHub) GetComponent(name string) (MonitoredComponent, bool) {
	mh.mu.RLock()
	defer mh.mu.RUnlock()
	
	component, exists := mh.components[name]
	return component, exists
}