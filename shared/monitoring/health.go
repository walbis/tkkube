package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// DefaultHealthMonitor provides a thread-safe implementation of HealthMonitor
type DefaultHealthMonitor struct {
	healthChecks   map[string]HealthCheckConfig
	healthStatus   map[string]HealthStatus
	dependencies   map[string]DependencyConfig
	depStatus      map[string]DependencyHealth
	mu             sync.RWMutex
	config         *MonitoringConfig
	running        bool
	stopChan       chan struct{}
	ticker         *time.Ticker
}

// HealthCheckConfig defines configuration for a health check
type HealthCheckConfig struct {
	Name        string
	Description string
	Check       HealthCheck
	Interval    time.Duration
	Timeout     time.Duration
	Critical    bool
}

// DependencyConfig defines configuration for a dependency health check
type DependencyConfig struct {
	Name        string
	Description string
	Endpoint    string
	Type        string // http, tcp, database, queue
	Check       func(ctx context.Context) DependencyHealth
	Interval    time.Duration
	Timeout     time.Duration
	Critical    bool
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(config *MonitoringConfig) *DefaultHealthMonitor {
	return &DefaultHealthMonitor{
		healthChecks: make(map[string]HealthCheckConfig),
		healthStatus: make(map[string]HealthStatus),
		dependencies: make(map[string]DependencyConfig),
		depStatus:    make(map[string]DependencyHealth),
		config:       config,
		stopChan:     make(chan struct{}),
	}
}

// RegisterHealthCheck registers a component health check
func (hm *DefaultHealthMonitor) RegisterHealthCheck(name string, check HealthCheck) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	interval := 30 * time.Second
	if hm.config != nil && hm.config.HealthInterval > 0 {
		interval = hm.config.HealthInterval
	}
	
	hm.healthChecks[name] = HealthCheckConfig{
		Name:        name,
		Check:       check,
		Interval:    interval,
		Timeout:     10 * time.Second,
		Critical:    true,
	}
	
	// Execute initial health check immediately
	ctx := context.Background()
	status := check(ctx)
	status.LastCheck = time.Now()
	status.CheckCount = 1
	hm.healthStatus[name] = status
}

// GetHealthStatus returns the health status of a specific component
func (hm *DefaultHealthMonitor) GetHealthStatus(component string) HealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	
	if status, exists := hm.healthStatus[component]; exists {
		return status
	}
	
	return HealthStatus{
		Status:  HealthStatusUnknown,
		Message: "Component not found",
	}
}

// GetOverallHealth returns the overall system health status
func (hm *DefaultHealthMonitor) GetOverallHealth() OverallHealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	
	var healthyCount, degradedCount, unhealthyCount int
	components := make(map[string]HealthStatus)
	
	for name, status := range hm.healthStatus {
		components[name] = status
		switch status.Status {
		case HealthStatusHealthy:
			healthyCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnhealthy:
			unhealthyCount++
		}
	}
	
	dependencies := make(map[string]DependencyHealth)
	var healthyDeps int
	for name, dep := range hm.depStatus {
		dependencies[name] = dep
		if dep.Status == HealthStatusHealthy {
			healthyDeps++
		}
	}
	
	// Determine overall status
	var overallStatus HealthStatusType
	if unhealthyCount > 0 {
		overallStatus = HealthStatusUnhealthy
	} else if degradedCount > 0 {
		overallStatus = HealthStatusDegraded
	} else if healthyCount > 0 {
		overallStatus = HealthStatusHealthy
	} else {
		overallStatus = HealthStatusUnknown
	}
	
	return OverallHealthStatus{
		Status:       overallStatus,
		Components:   components,
		Dependencies: dependencies,
		Summary: HealthSummary{
			TotalComponents:     len(hm.healthStatus),
			HealthyComponents:   healthyCount,
			DegradedComponents:  degradedCount,
			UnhealthyComponents: unhealthyCount,
			TotalDependencies:   len(hm.depStatus),
			HealthyDependencies: healthyDeps,
		},
		Timestamp: time.Now(),
	}
}

// CheckDependency performs a health check on an external dependency
func (hm *DefaultHealthMonitor) CheckDependency(name string, endpoint string) DependencyHealth {
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	start := time.Now()
	var status HealthStatusType
	var errorMsg string
	
	// HTTP dependency check
	if endpoint != "" {
		resp, err := http.Get(endpoint)
		if err != nil {
			status = HealthStatusUnhealthy
			errorMsg = err.Error()
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				status = HealthStatusHealthy
			} else {
				status = HealthStatusDegraded
				errorMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
		}
	} else {
		status = HealthStatusUnknown
		errorMsg = "No endpoint configured"
	}
	
	responseTime := time.Since(start)
	
	dependency := DependencyHealth{
		Name:         name,
		Status:       status,
		ResponseTime: responseTime,
		Endpoint:     endpoint,
		ErrorMessage: errorMsg,
	}
	
	if status == HealthStatusHealthy {
		dependency.LastSuccess = time.Now()
	} else {
		dependency.LastFailure = time.Now()
		hm.incrementConsecutiveFails(name)
	}
	
	hm.mu.Lock()
	hm.depStatus[name] = dependency
	hm.mu.Unlock()
	
	return dependency
}

// GetDependencyStatus returns the status of all dependencies
func (hm *DefaultHealthMonitor) GetDependencyStatus() map[string]DependencyHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	
	status := make(map[string]DependencyHealth)
	for name, dep := range hm.depStatus {
		status[name] = dep
	}
	return status
}

// StartHealthChecks starts the health checking process
func (hm *DefaultHealthMonitor) StartHealthChecks(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	if hm.running {
		return fmt.Errorf("health checks already running")
	}
	
	interval := 30 * time.Second
	if hm.config != nil && hm.config.HealthInterval > 0 {
		interval = hm.config.HealthInterval
	}
	
	hm.ticker = time.NewTicker(interval)
	hm.running = true
	
	go hm.healthCheckLoop(ctx)
	
	return nil
}

// StopHealthChecks stops the health checking process
func (hm *DefaultHealthMonitor) StopHealthChecks() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	if !hm.running {
		return fmt.Errorf("health checks not running")
	}
	
	close(hm.stopChan)
	if hm.ticker != nil {
		hm.ticker.Stop()
	}
	hm.running = false
	
	return nil
}

// healthCheckLoop runs the health check loop
func (hm *DefaultHealthMonitor) healthCheckLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-hm.stopChan:
			return
		case <-hm.ticker.C:
			hm.runHealthChecks(ctx)
		}
	}
}

// runHealthChecks executes all registered health checks
func (hm *DefaultHealthMonitor) runHealthChecks(ctx context.Context) {
	hm.mu.RLock()
	checks := make(map[string]HealthCheckConfig)
	for name, config := range hm.healthChecks {
		checks[name] = config
	}
	hm.mu.RUnlock()
	
	// Run health checks concurrently
	var wg sync.WaitGroup
	for name, config := range checks {
		wg.Add(1)
		go func(n string, c HealthCheckConfig) {
			defer wg.Done()
			hm.executeHealthCheck(ctx, n, c)
		}(name, config)
	}
	
	wg.Wait()
	
	// Run dependency checks
	hm.runDependencyChecks(ctx)
}

// executeHealthCheck executes a single health check
func (hm *DefaultHealthMonitor) executeHealthCheck(ctx context.Context, name string, config HealthCheckConfig) {
	checkCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	
	start := time.Now()
	status := config.Check(checkCtx)
	duration := time.Since(start)
	
	status.Duration = duration
	status.LastCheck = time.Now()
	
	hm.mu.Lock()
	if existingStatus, exists := hm.healthStatus[name]; exists {
		status.CheckCount = existingStatus.CheckCount + 1
	} else {
		status.CheckCount = 1
	}
	hm.healthStatus[name] = status
	hm.mu.Unlock()
}

// runDependencyChecks executes dependency health checks
func (hm *DefaultHealthMonitor) runDependencyChecks(ctx context.Context) {
	hm.mu.RLock()
	deps := make(map[string]DependencyConfig)
	for name, config := range hm.dependencies {
		deps[name] = config
	}
	hm.mu.RUnlock()
	
	var wg sync.WaitGroup
	for name, config := range deps {
		wg.Add(1)
		go func(n string, c DependencyConfig) {
			defer wg.Done()
			if c.Check != nil {
				checkCtx, cancel := context.WithTimeout(ctx, c.Timeout)
				defer cancel()
				result := c.Check(checkCtx)
				
				hm.mu.Lock()
				hm.depStatus[n] = result
				hm.mu.Unlock()
			} else if c.Endpoint != "" {
				hm.CheckDependency(n, c.Endpoint)
			}
		}(name, config)
	}
	
	wg.Wait()
}

// incrementConsecutiveFails increments the consecutive failures count for a dependency
func (hm *DefaultHealthMonitor) incrementConsecutiveFails(name string) {
	if dep, exists := hm.depStatus[name]; exists {
		dep.ConsecutiveFails++
		hm.depStatus[name] = dep
	}
}

// RegisterDependency registers a dependency for health monitoring
func (hm *DefaultHealthMonitor) RegisterDependency(name, endpoint string, depType string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	interval := 60 * time.Second
	if hm.config != nil && hm.config.HealthInterval > 0 {
		interval = hm.config.HealthInterval * 2 // Dependencies checked less frequently
	}
	
	hm.dependencies[name] = DependencyConfig{
		Name:        name,
		Endpoint:    endpoint,
		Type:        depType,
		Interval:    interval,
		Timeout:     15 * time.Second,
		Critical:    false,
	}
	
	// Initialize status
	hm.depStatus[name] = DependencyHealth{
		Name:   name,
		Status: HealthStatusUnknown,
	}
}

// RegisterCustomDependencyCheck registers a custom dependency check function
func (hm *DefaultHealthMonitor) RegisterCustomDependencyCheck(name string, check func(ctx context.Context) DependencyHealth) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	interval := 60 * time.Second
	if hm.config != nil && hm.config.HealthInterval > 0 {
		interval = hm.config.HealthInterval * 2
	}
	
	hm.dependencies[name] = DependencyConfig{
		Name:     name,
		Check:    check,
		Interval: interval,
		Timeout:  15 * time.Second,
		Critical: false,
	}
	
	// Initialize status
	hm.depStatus[name] = DependencyHealth{
		Name:   name,
		Status: HealthStatusUnknown,
	}
}

// Common health check functions

// HTTPHealthCheck creates a health check for HTTP endpoints
func HTTPHealthCheck(endpoint string) HealthCheck {
	return func(ctx context.Context) HealthStatus {
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return HealthStatus{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("Failed to create request: %v", err),
			}
		}
		
		resp, err := client.Do(req)
		if err != nil {
			return HealthStatus{
				Status:  HealthStatusUnhealthy,
				Message: fmt.Sprintf("HTTP request failed: %v", err),
			}
		}
		defer resp.Body.Close()
		
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return HealthStatus{
				Status:  HealthStatusHealthy,
				Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
			}
		}
		
		return HealthStatus{
			Status:  HealthStatusDegraded,
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}
}

// BasicHealthCheck creates a simple always-healthy health check
func BasicHealthCheck(componentName string) HealthCheck {
	return func(ctx context.Context) HealthStatus {
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: fmt.Sprintf("%s is running", componentName),
		}
	}
}

// MemoryHealthCheck creates a health check based on memory usage
func MemoryHealthCheck(threshold float64) HealthCheck {
	return func(ctx context.Context) HealthStatus {
		// This would typically integrate with runtime memory stats
		// For now, return a mock implementation
		return HealthStatus{
			Status:  HealthStatusHealthy,
			Message: "Memory usage within limits",
			Metrics: map[string]interface{}{
				"memory_threshold": threshold,
			},
		}
	}
}