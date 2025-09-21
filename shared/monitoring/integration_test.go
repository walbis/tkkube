package monitoring

import (
	"context"
	"testing"
	"time"

	sharedconfig "shared-config/config"
)

// TestBasicMonitoringSystemIntegration tests the basic monitoring system setup
func TestBasicMonitoringSystemIntegration(t *testing.T) {
	// Create a test configuration
	config := &sharedconfig.SharedConfig{
		SchemaVersion: "1.0",
		Performance: sharedconfig.PerformanceConfig{
			Limits: sharedconfig.LimitsConfig{
				MaxConcurrentOperations: 10,
			},
		},
		Pipeline: sharedconfig.PipelineConfig{
			Notifications: sharedconfig.NotificationsConfig{
				Enabled: true,
			},
		},
	}
	
	// Create logger
	logger := NewLogger("test")
	
	// Create monitoring system
	system := NewMonitoringSystem(config, logger)
	
	// Initialize components
	ctx := context.Background()
	err := system.InitializeComponents(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize components: %v", err)
	}
	
	// Start the system
	err = system.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start monitoring system: %v", err)
	}
	defer system.Stop()
	
	// Verify components are registered
	components := system.ListComponents()
	if len(components) == 0 {
		t.Error("No components registered")
	}
	
	// Check system health
	health := system.GetSystemHealth()
	if health.Status == HealthStatusUnknown {
		t.Error("System health is unknown")
	}
	
	// Get aggregated metrics
	metrics := system.GetAggregatedMetrics()
	if metrics.Timestamp.IsZero() {
		t.Error("Metrics timestamp is zero")
	}
	
	t.Logf("Monitoring system integration test passed with %d components", len(components))
}

// TestMetricsCollection tests basic metrics collection functionality
func TestMetricsCollection(t *testing.T) {
	config := &MonitoringConfig{
		MetricsEnabled: true,
		MaxMetricsBuffer: 100,
	}
	
	collector := NewMetricsCollector(config)
	
	// Test counter metric
	labels := map[string]string{"test": "value"}
	collector.IncCounter("test_counter", labels, 1)
	collector.IncCounter("test_counter", labels, 5)
	
	// Test gauge metric
	collector.SetGauge("test_gauge", labels, 42.5)
	
	// Test duration metric
	duration := 100 * time.Millisecond
	collector.RecordDuration("test_duration", labels, duration)
	
	// Test histogram
	collector.RecordHistogram("test_histogram", labels, 123.45)
	
	// Get metrics
	metrics := collector.GetMetrics()
	if len(metrics) == 0 {
		t.Error("No metrics collected")
	}
	
	// Verify metric types and values
	foundCounter := false
	foundGauge := false
	foundDuration := false
	
	for _, metric := range metrics {
		switch metric.Name {
		case "test_counter":
			if metric.Type != MetricTypeCounter {
				t.Errorf("Expected counter type, got %s", metric.Type)
			}
			foundCounter = true
		case "test_gauge":
			if metric.Type != MetricTypeGauge {
				t.Errorf("Expected gauge type, got %s", metric.Type)
			}
			if metric.Value != 42.5 {
				t.Errorf("Expected gauge value 42.5, got %f", metric.Value)
			}
			foundGauge = true
		case "test_duration":
			if metric.Type != MetricTypeHistogram {
				t.Errorf("Expected histogram type for duration, got %s", metric.Type)
			}
			if metric.Value != 100 { // 100ms
				t.Errorf("Expected duration value 100, got %f", metric.Value)
			}
			foundDuration = true
		}
	}
	
	if !foundCounter {
		t.Error("Counter metric not found")
	}
	if !foundGauge {
		t.Error("Gauge metric not found")
	}
	if !foundDuration {
		t.Error("Duration metric not found")
	}
	
	t.Logf("Metrics collection test passed with %d metrics", len(metrics))
}

// TestHealthMonitoring tests the health monitoring functionality
func TestHealthMonitoring(t *testing.T) {
	config := &MonitoringConfig{
		HealthEnabled: true,
		HealthInterval: 100 * time.Millisecond,
	}
	
	monitor := NewHealthMonitor(config)
	
	// Register a health check
	healthCheck := func(ctx context.Context) HealthStatus {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: "Test component is healthy",
		}
	}
	
	monitor.RegisterHealthCheck("test_component", healthCheck)
	
	// Get health status
	status := monitor.GetHealthStatus("test_component")
	if status.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy status, got %s", status.Status)
	}
	
	// Get overall health
	overall := monitor.GetOverallHealth()
	if overall.Status != HealthStatusHealthy {
		t.Errorf("Expected overall healthy status, got %s", overall.Status)
	}
	
	if overall.Summary.TotalComponents != 1 {
		t.Errorf("Expected 1 component, got %d", overall.Summary.TotalComponents)
	}
	
	if overall.Summary.HealthyComponents != 1 {
		t.Errorf("Expected 1 healthy component, got %d", overall.Summary.HealthyComponents)
	}
	
	t.Log("Health monitoring test passed")
}

// TestEventPublishing tests the event publishing functionality
func TestEventPublishing(t *testing.T) {
	logger := NewLogger("test")
	config := &MonitoringConfig{
		EventsEnabled: true,
		MaxEventsBuffer: 100,
	}
	
	publisher := NewEventPublisher(config, logger)
	
	// Test system event
	systemEvent := SystemEvent{
		ID:        "test_event_1",
		Timestamp: time.Now(),
		Type:      "test_system_event",
		Component: "test_component",
		Action:    "test_action",
		Status:    "success",
	}
	
	err := publisher.PublishSystemEvent(systemEvent)
	if err != nil {
		t.Errorf("Failed to publish system event: %v", err)
	}
	
	// Test business event
	businessEvent := BusinessEvent{
		ID:          "test_event_2",
		Timestamp:   time.Now(),
		Type:        "test_business_event",
		BusinessID:  "test_business_id",
		Description: "Test business event",
		Impact:      "positive",
	}
	
	err = publisher.PublishBusinessEvent(businessEvent)
	if err != nil {
		t.Errorf("Failed to publish business event: %v", err)
	}
	
	// Test error event
	testError := &TestError{message: "test error"}
	errorEvent := CreateErrorEvent("test_component", "test_operation", testError, "warning")
	
	err = publisher.PublishErrorEvent(errorEvent)
	if err != nil {
		t.Errorf("Failed to publish error event: %v", err)
	}
	
	// Get published events
	events := publisher.GetEvents()
	if len(events) == 0 {
		t.Error("No events were stored")
	}
	
	// Verify event types
	foundSystemEvent := false
	foundBusinessEvent := false
	foundErrorEvent := false
	
	for _, event := range events {
		switch event.GetType() {
		case "test_system_event":
			foundSystemEvent = true
		case "test_business_event":
			foundBusinessEvent = true
		case "error":
			foundErrorEvent = true
		}
	}
	
	if !foundSystemEvent {
		t.Error("System event not found")
	}
	if !foundBusinessEvent {
		t.Error("Business event not found")
	}
	if !foundErrorEvent {
		t.Error("Error event not found")
	}
	
	t.Logf("Event publishing test passed with %d events", len(events))
}

// TestMonitoringInitializer tests the monitoring initializer
func TestMonitoringInitializer(t *testing.T) {
	initializer := NewMonitoringInitializer()
	
	ctx := context.Background()
	system, err := initializer.StartWithAutoSetup(ctx)
	if err != nil {
		t.Fatalf("Failed to start monitoring with auto setup: %v", err)
	}
	defer initializer.Cleanup()
	
	// Verify system is running
	if !system.running {
		t.Error("Monitoring system is not running")
	}
	
	// Check components
	components := system.ListComponents()
	if len(components) == 0 {
		t.Error("No components initialized")
	}
	
	// Check health
	health := system.GetSystemHealth()
	if health.Timestamp.IsZero() {
		t.Error("Health timestamp is zero")
	}
	
	t.Logf("Monitoring initializer test passed with %d components", len(components))
}

// TestComponentRegistration tests component registration and monitoring
func TestComponentRegistration(t *testing.T) {
	logger := NewLogger("test")
	config := &MonitoringConfig{
		MetricsEnabled: true,
		HealthEnabled:  true,
		EventsEnabled:  true,
	}
	
	hub := NewMonitoringHub(config, logger)
	
	// Create test component
	component := &TestComponent{
		name:    "test_component",
		version: "1.0.0",
		healthy: true,
	}
	
	// Register component
	err := hub.RegisterComponent("test_component", component)
	if err != nil {
		t.Fatalf("Failed to register component: %v", err)
	}
	
	// Start hub
	ctx := context.Background()
	err = hub.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hub.Stop()
	
	// Check component is listed
	components := hub.ListComponents()
	found := false
	for _, name := range components {
		if name == "test_component" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Component not found in list")
	}
	
	// Check component health
	health := hub.GetSystemHealth()
	if componentHealth, exists := health.ComponentHealth["test_component"]; !exists {
		t.Error("Component health not found")
	} else if componentHealth.Status != HealthStatusHealthy {
		t.Errorf("Expected healthy component, got %s", componentHealth.Status)
	}
	
	// Get component metrics
	metrics := hub.GetAggregatedMetrics()
	if componentMetrics, exists := metrics.ComponentMetrics["test_component"]; !exists {
		t.Error("Component metrics not found")
	} else if len(componentMetrics) == 0 {
		t.Error("No component metrics collected")
	}
	
	t.Log("Component registration test passed")
}

// Test helper types

type TestError struct {
	message string
}

func (e *TestError) Error() string {
	return e.message
}

type TestComponent struct {
	name    string
	version string
	healthy bool
}

func (tc *TestComponent) GetComponentName() string {
	return tc.name
}

func (tc *TestComponent) GetComponentVersion() string {
	return tc.version
}

func (tc *TestComponent) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"test_metric": 42.0,
		"status":      "ok",
	}
}

// GetMetricsCollector implements the interface needed for metrics aggregation
func (tc *TestComponent) GetMetricsCollector() MetricsCollector {
	config := &MonitoringConfig{
		MetricsEnabled: true,
		MaxMetricsBuffer: 100,
	}
	collector := NewMetricsCollector(config)
	
	// Add some test metrics
	labels := map[string]string{"component": tc.name}
	collector.SetGauge("test_metric", labels, 42.0)
	collector.IncCounter("status_ok", labels, 1)
	
	return collector
}

func (tc *TestComponent) ResetMetrics() {
	// Test implementation
}

func (tc *TestComponent) HealthCheck(ctx context.Context) HealthStatus {
	if tc.healthy {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: "Test component is healthy",
		}
	}
	return HealthStatus{
		Status:  HealthStatusUnhealthy,
		Message: "Test component is unhealthy",
	}
}

func (tc *TestComponent) GetDependencies() []string {
	return []string{"test_dependency"}
}

func (tc *TestComponent) OnStart(ctx context.Context) error {
	return nil
}

func (tc *TestComponent) OnStop(ctx context.Context) error {
	return nil
}

// Benchmark tests

func BenchmarkMetricsCollection(b *testing.B) {
	config := &MonitoringConfig{
		MetricsEnabled: true,
		MaxMetricsBuffer: 10000,
	}
	
	collector := NewMetricsCollector(config)
	labels := map[string]string{"benchmark": "test"}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		collector.IncCounter("benchmark_counter", labels, 1)
	}
}

func BenchmarkEventPublishing(b *testing.B) {
	logger := NewNullLogger()
	config := &MonitoringConfig{
		EventsEnabled: true,
		MaxEventsBuffer: 10000,
	}
	
	publisher := NewEventPublisher(config, logger)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		event := SystemEvent{
			ID:        "benchmark_event",
			Timestamp: time.Now(),
			Type:      "benchmark",
			Component: "test",
		}
		publisher.PublishSystemEvent(event)
	}
}