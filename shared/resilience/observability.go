package resilience

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"shared-config/monitoring"
)

// ObservabilityConfig defines configuration for circuit breaker observability
type ObservabilityConfig struct {
	// Metrics collection
	MetricsEnabled    bool          `yaml:"metrics_enabled"`
	MetricsInterval   time.Duration `yaml:"metrics_interval"`
	MetricsRetention  time.Duration `yaml:"metrics_retention"`
	
	// Logging configuration
	LoggingEnabled    bool   `yaml:"logging_enabled"`
	LogLevel          string `yaml:"log_level"`
	LogStateChanges   bool   `yaml:"log_state_changes"`
	LogFailures       bool   `yaml:"log_failures"`
	LogRecoveries     bool   `yaml:"log_recoveries"`
	
	// Alerting configuration
	AlertingEnabled   bool          `yaml:"alerting_enabled"`
	AlertThreshold    int           `yaml:"alert_threshold"`
	AlertCooldown     time.Duration `yaml:"alert_cooldown"`
	
	// Health check configuration
	HealthCheckEnabled bool          `yaml:"health_check_enabled"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
}

// DefaultObservabilityConfig returns sensible defaults
func DefaultObservabilityConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		MetricsEnabled:      true,
		MetricsInterval:     30 * time.Second,
		MetricsRetention:    24 * time.Hour,
		LoggingEnabled:      true,
		LogLevel:           "info",
		LogStateChanges:    true,
		LogFailures:        true,
		LogRecoveries:      true,
		AlertingEnabled:    true,
		AlertThreshold:     3, // Alert after 3 consecutive failures
		AlertCooldown:      5 * time.Minute,
		HealthCheckEnabled: true,
		HealthCheckInterval: 1 * time.Minute,
	}
}

// CircuitBreakerObserver monitors and observes circuit breaker behavior
type CircuitBreakerObserver struct {
	config             *ObservabilityConfig
	circuitBreakers    map[string]*CircuitBreaker
	monitoring         monitoring.MetricsCollector
	alertManager       *AlertManager
	healthMonitor      *HealthMonitor
	metricsAggregator  *MetricsAggregator
	eventLogger        *EventLogger
	mu                 sync.RWMutex
}

// NewCircuitBreakerObserver creates a new circuit breaker observer
func NewCircuitBreakerObserver(
	config *ObservabilityConfig,
	monitoring monitoring.MetricsCollector,
) *CircuitBreakerObserver {
	if config == nil {
		config = DefaultObservabilityConfig()
	}
	
	observer := &CircuitBreakerObserver{
		config:            config,
		circuitBreakers:   make(map[string]*CircuitBreaker),
		monitoring:        monitoring,
		eventLogger:       NewEventLogger(config),
		metricsAggregator: NewMetricsAggregator(config, monitoring),
		alertManager:      NewAlertManager(config),
		healthMonitor:     NewHealthMonitor(config),
	}
	
	return observer
}

// RegisterCircuitBreaker registers a circuit breaker for monitoring
func (o *CircuitBreakerObserver) RegisterCircuitBreaker(name string, cb *CircuitBreaker) {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	o.circuitBreakers[name] = cb
	o.eventLogger.LogEvent("circuit_breaker_registered", map[string]interface{}{
		"name": name,
		"timestamp": time.Now(),
	})
	
	// Set up state change callback
	if cb.config.OnStateChange == nil {
		cb.config.OnStateChange = o.handleStateChange
	} else {
		// Wrap existing callback
		originalCallback := cb.config.OnStateChange
		cb.config.OnStateChange = func(name string, from, to CircuitBreakerState) {
			originalCallback(name, from, to)
			o.handleStateChange(name, from, to)
		}
	}
}

// UnregisterCircuitBreaker removes a circuit breaker from monitoring
func (o *CircuitBreakerObserver) UnregisterCircuitBreaker(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	delete(o.circuitBreakers, name)
	o.eventLogger.LogEvent("circuit_breaker_unregistered", map[string]interface{}{
		"name": name,
		"timestamp": time.Now(),
	})
}

// StartMonitoring starts background monitoring of all registered circuit breakers
func (o *CircuitBreakerObserver) StartMonitoring(ctx context.Context) {
	// Start metrics collection
	if o.config.MetricsEnabled {
		go o.startMetricsCollection(ctx)
	}
	
	// Start health monitoring
	if o.config.HealthCheckEnabled {
		go o.startHealthMonitoring(ctx)
	}
	
	// Start alert processing
	if o.config.AlertingEnabled {
		go o.startAlertProcessing(ctx)
	}
	
	o.eventLogger.LogEvent("monitoring_started", map[string]interface{}{
		"metrics_enabled": o.config.MetricsEnabled,
		"health_check_enabled": o.config.HealthCheckEnabled,
		"alerting_enabled": o.config.AlertingEnabled,
		"timestamp": time.Now(),
	})
}

// GetSystemHealth returns overall system health based on circuit breaker states
func (o *CircuitBreakerObserver) GetSystemHealth() *SystemHealthReport {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	report := &SystemHealthReport{
		Timestamp:           time.Now(),
		TotalCircuitBreakers: len(o.circuitBreakers),
		Services:            make(map[string]*ServiceHealth),
	}
	
	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0
	
	for name, cb := range o.circuitBreakers {
		state := cb.GetState()
		metrics := cb.GetMetrics()
		
		serviceHealth := &ServiceHealth{
			Name:            name,
			State:           state,
			Healthy:         state != StateOpen,
			TotalRequests:   metrics.TotalRequests,
			SuccessRate:     o.calculateSuccessRate(metrics),
			FailureStreak:   metrics.FailureStreak,
			LastFailure:     metrics.LastFailureTime,
			LastStateChange: metrics.LastStateChange,
		}
		
		report.Services[name] = serviceHealth
		
		switch state {
		case StateClosed:
			if serviceHealth.SuccessRate >= 95.0 {
				healthyCount++
			} else {
				degradedCount++
			}
		case StateHalfOpen:
			degradedCount++
		case StateOpen:
			unhealthyCount++
		}
	}
	
	report.HealthyServices = healthyCount
	report.DegradedServices = degradedCount
	report.UnhealthyServices = unhealthyCount
	
	// Calculate overall health score
	if report.TotalCircuitBreakers > 0 {
		report.OverallHealthScore = float64(healthyCount) / float64(report.TotalCircuitBreakers) * 100
	} else {
		report.OverallHealthScore = 100.0
	}
	
	// Determine overall status
	if unhealthyCount == 0 && degradedCount == 0 {
		report.OverallStatus = "healthy"
	} else if unhealthyCount == 0 {
		report.OverallStatus = "degraded"
	} else {
		report.OverallStatus = "unhealthy"
	}
	
	return report
}

// GetDetailedMetrics returns detailed metrics for all circuit breakers
func (o *CircuitBreakerObserver) GetDetailedMetrics() map[string]*DetailedMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	result := make(map[string]*DetailedMetrics)
	
	for name, cb := range o.circuitBreakers {
		metrics := cb.GetMetrics()
		
		detailed := &DetailedMetrics{
			CircuitBreakerName: name,
			State:              metrics.State,
			Timestamp:          time.Now(),
			
			// Request metrics
			TotalRequests:    metrics.TotalRequests,
			SuccessfulReqs:   metrics.SuccessfulReqs,
			FailedReqs:       metrics.FailedReqs,
			RejectedReqs:     metrics.RejectedReqs,
			
			// State metrics
			ClosedRequests:   metrics.ClosedRequests,
			OpenRequests:     metrics.OpenRequests,
			HalfOpenRequests: metrics.HalfOpenRequests,
			
			// Performance metrics
			SuccessRate:      o.calculateSuccessRate(metrics),
			FailureRate:      o.calculateFailureRate(metrics),
			RejectionRate:    o.calculateRejectionRate(metrics),
			
			// Timing metrics
			FailureStreak:    metrics.FailureStreak,
			SuccessStreak:    metrics.SuccessStreak,
			StateChanges:     metrics.StateChanges,
			LastFailure:      metrics.LastFailureTime,
			LastSuccess:      metrics.LastSuccessTime,
			LastStateChange:  metrics.LastStateChange,
			OpenedAt:         metrics.OpenedAt,
			LastRecoveryAt:   metrics.LastRecoveryAt,
		}
		
		result[name] = detailed
	}
	
	return result
}

// GetAggregatedMetrics returns system-wide aggregated metrics
func (o *CircuitBreakerObserver) GetAggregatedMetrics() *AggregatedMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	agg := &AggregatedMetrics{
		Timestamp: time.Now(),
	}
	
	for _, cb := range o.circuitBreakers {
		metrics := cb.GetMetrics()
		state := cb.GetState()
		
		agg.TotalRequests += metrics.TotalRequests
		agg.TotalSuccessful += metrics.SuccessfulReqs
		agg.TotalFailed += metrics.FailedReqs
		agg.TotalRejected += metrics.RejectedReqs
		agg.TotalStateChanges += metrics.StateChanges
		
		switch state {
		case StateClosed:
			agg.ClosedCircuitBreakers++
		case StateOpen:
			agg.OpenCircuitBreakers++
		case StateHalfOpen:
			agg.HalfOpenCircuitBreakers++
		}
	}
	
	agg.TotalCircuitBreakers = len(o.circuitBreakers)
	
	// Calculate rates
	if agg.TotalRequests > 0 {
		agg.OverallSuccessRate = float64(agg.TotalSuccessful) / float64(agg.TotalRequests) * 100
		agg.OverallFailureRate = float64(agg.TotalFailed) / float64(agg.TotalRequests) * 100
		agg.OverallRejectionRate = float64(agg.TotalRejected) / float64(agg.TotalRequests) * 100
	}
	
	// Calculate health percentage
	if agg.TotalCircuitBreakers > 0 {
		healthyCount := agg.ClosedCircuitBreakers + agg.HalfOpenCircuitBreakers
		agg.HealthPercentage = float64(healthyCount) / float64(agg.TotalCircuitBreakers) * 100
	} else {
		agg.HealthPercentage = 100.0
	}
	
	return agg
}

// GetAlerts returns current active alerts
func (o *CircuitBreakerObserver) GetAlerts() []*Alert {
	return o.alertManager.GetActiveAlerts()
}

// GetEventHistory returns recent circuit breaker events
func (o *CircuitBreakerObserver) GetEventHistory(limit int) []*CircuitBreakerEvent {
	return o.eventLogger.GetRecentEvents(limit)
}

// ExportMetrics exports metrics in various formats
func (o *CircuitBreakerObserver) ExportMetrics(format string) ([]byte, error) {
	metrics := o.GetDetailedMetrics()
	
	switch format {
	case "json":
		return json.MarshalIndent(metrics, "", "  ")
	case "prometheus":
		return o.exportPrometheusMetrics(metrics)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// Private methods

func (o *CircuitBreakerObserver) handleStateChange(name string, from, to CircuitBreakerState) {
	event := &CircuitBreakerEvent{
		Type:            EventStateChange,
		CircuitBreaker:  name,
		Timestamp:       time.Now(),
		FromState:       from,
		ToState:         to,
		Message:         fmt.Sprintf("Circuit breaker '%s' transitioned from %s to %s", name, from.String(), to.String()),
	}
	
	// Log the event
	if o.config.LogStateChanges {
		o.eventLogger.LogEvent("state_change", map[string]interface{}{
			"circuit_breaker": name,
			"from_state":      from.String(),
			"to_state":        to.String(),
			"timestamp":       event.Timestamp,
		})
	}
	
	// Record event
	o.eventLogger.RecordEvent(event)
	
	// Check for alerts
	if o.config.AlertingEnabled {
		o.alertManager.CheckForAlert(name, from, to)
	}
	
	// Record metrics
	if o.monitoring != nil {
		labels := map[string]string{
			"circuit_breaker": name,
			"from_state":     from.String(),
			"to_state":       to.String(),
		}
		o.monitoring.IncCounter("circuit_breaker_state_transitions", labels, 1)
	}
}

func (o *CircuitBreakerObserver) startMetricsCollection(ctx context.Context) {
	ticker := time.NewTicker(o.config.MetricsInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.collectAndRecordMetrics()
		}
	}
}

func (o *CircuitBreakerObserver) startHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(o.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.performHealthChecks()
		}
	}
}

func (o *CircuitBreakerObserver) startAlertProcessing(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check for alerts every 30 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.processAlerts()
		}
	}
}

func (o *CircuitBreakerObserver) collectAndRecordMetrics() {
	aggregated := o.GetAggregatedMetrics()
	
	if o.monitoring == nil {
		return
	}
	
	// Record aggregated metrics
	labels := map[string]string{"observer": "circuit_breaker"}
	o.monitoring.SetGauge("circuit_breaker_total_count", labels, float64(aggregated.TotalCircuitBreakers))
	o.monitoring.SetGauge("circuit_breaker_closed_count", labels, float64(aggregated.ClosedCircuitBreakers))
	o.monitoring.SetGauge("circuit_breaker_open_count", labels, float64(aggregated.OpenCircuitBreakers))
	o.monitoring.SetGauge("circuit_breaker_half_open_count", labels, float64(aggregated.HalfOpenCircuitBreakers))
	o.monitoring.SetGauge("circuit_breaker_overall_success_rate", labels, aggregated.OverallSuccessRate)
	o.monitoring.SetGauge("circuit_breaker_health_percentage", labels, aggregated.HealthPercentage)
	
	// Record individual circuit breaker metrics
	detailed := o.GetDetailedMetrics()
	for name, metrics := range detailed {
		cbLabels := map[string]string{"circuit_breaker": name}
		o.monitoring.SetGauge("circuit_breaker_state", cbLabels, float64(metrics.State))
		o.monitoring.SetGauge("circuit_breaker_success_rate", cbLabels, metrics.SuccessRate)
		o.monitoring.SetGauge("circuit_breaker_failure_streak", cbLabels, float64(metrics.FailureStreak))
		o.monitoring.SetGauge("circuit_breaker_total_requests", cbLabels, float64(metrics.TotalRequests))
	}
}

func (o *CircuitBreakerObserver) performHealthChecks() {
	systemHealth := o.GetSystemHealth()
	
	// Log health status if configured
	if o.config.LoggingEnabled {
		log.Printf("System Health: %s (%.1f%% - %d/%d services healthy)",
			systemHealth.OverallStatus,
			systemHealth.OverallHealthScore,
			systemHealth.HealthyServices,
			systemHealth.TotalCircuitBreakers)
	}
	
	// Record health metrics
	if o.monitoring != nil {
		labels := map[string]string{"health_monitor": "circuit_breaker"}
		o.monitoring.SetGauge("system_health_score", labels, systemHealth.OverallHealthScore)
		o.monitoring.SetGauge("healthy_services_count", labels, float64(systemHealth.HealthyServices))
		o.monitoring.SetGauge("degraded_services_count", labels, float64(systemHealth.DegradedServices))
		o.monitoring.SetGauge("unhealthy_services_count", labels, float64(systemHealth.UnhealthyServices))
	}
}

func (o *CircuitBreakerObserver) processAlerts() {
	alerts := o.alertManager.ProcessAlerts()
	
	for _, alert := range alerts {
		if o.config.LoggingEnabled {
			log.Printf("ALERT: %s - %s", alert.Severity, alert.Message)
		}
		
		// Record alert metric
		if o.monitoring != nil {
			labels := map[string]string{
				"circuit_breaker": alert.CircuitBreaker,
				"severity":       alert.Severity,
				"type":          alert.Type,
			}
			o.monitoring.IncCounter("circuit_breaker_alerts", labels, 1)
		}
	}
}

func (o *CircuitBreakerObserver) calculateSuccessRate(metrics CircuitBreakerMetrics) float64 {
	if metrics.TotalRequests == 0 {
		return 100.0
	}
	return float64(metrics.SuccessfulReqs) / float64(metrics.TotalRequests) * 100
}

func (o *CircuitBreakerObserver) calculateFailureRate(metrics CircuitBreakerMetrics) float64 {
	if metrics.TotalRequests == 0 {
		return 0.0
	}
	return float64(metrics.FailedReqs) / float64(metrics.TotalRequests) * 100
}

func (o *CircuitBreakerObserver) calculateRejectionRate(metrics CircuitBreakerMetrics) float64 {
	if metrics.TotalRequests == 0 {
		return 0.0
	}
	return float64(metrics.RejectedReqs) / float64(metrics.TotalRequests) * 100
}

func (o *CircuitBreakerObserver) exportPrometheusMetrics(metrics map[string]*DetailedMetrics) ([]byte, error) {
	var output []string
	
	for name, m := range metrics {
		// State metric
		output = append(output, fmt.Sprintf("circuit_breaker_state{name=\"%s\"} %d", name, int(m.State)))
		
		// Request metrics
		output = append(output, fmt.Sprintf("circuit_breaker_total_requests{name=\"%s\"} %d", name, m.TotalRequests))
		output = append(output, fmt.Sprintf("circuit_breaker_successful_requests{name=\"%s\"} %d", name, m.SuccessfulReqs))
		output = append(output, fmt.Sprintf("circuit_breaker_failed_requests{name=\"%s\"} %d", name, m.FailedReqs))
		output = append(output, fmt.Sprintf("circuit_breaker_rejected_requests{name=\"%s\"} %d", name, m.RejectedReqs))
		
		// Rate metrics
		output = append(output, fmt.Sprintf("circuit_breaker_success_rate{name=\"%s\"} %.2f", name, m.SuccessRate))
		output = append(output, fmt.Sprintf("circuit_breaker_failure_rate{name=\"%s\"} %.2f", name, m.FailureRate))
		
		// Streak metrics
		output = append(output, fmt.Sprintf("circuit_breaker_failure_streak{name=\"%s\"} %d", name, m.FailureStreak))
		output = append(output, fmt.Sprintf("circuit_breaker_success_streak{name=\"%s\"} %d", name, m.SuccessStreak))
	}
	
	return []byte(fmt.Sprintf("%s\n", fmt.Sprintf("%v", output))), nil
}