package sharedconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LiveValidationService provides real-time validation and health monitoring
type LiveValidationService struct {
	validator           *EnhancedMultiClusterValidator
	config             *MultiClusterConfig
	
	// Service configuration
	enabled             bool
	validationInterval  time.Duration
	healthCheckInterval time.Duration
	httpServerPort      int
	
	// State management
	mutex                 sync.RWMutex
	lastValidationResult  *EnhancedValidationResult
	lastHealthCheck       *HealthCheckResult
	validationHistory     []*ValidationHistoryEntry
	maxHistoryEntries     int
	
	// Service lifecycle
	ctx                   context.Context
	cancel                context.CancelFunc
	httpServer            *http.Server
	
	// Event handling
	eventHandlers         map[string][]ValidationEventHandler
	eventMutex           sync.RWMutex
	
	// Metrics
	metrics              *ValidationServiceMetrics
}

// ValidationEventHandler handles validation events
type ValidationEventHandler func(event *ValidationEvent) error

// ValidationEvent represents a validation event
type ValidationEvent struct {
	Type        ValidationEventType `json:"type"`
	Timestamp   time.Time           `json:"timestamp"`
	ClusterName string              `json:"cluster_name,omitempty"`
	Data        interface{}         `json:"data"`
	Severity    EventSeverity       `json:"severity"`
}

// ValidationEventType represents the type of validation event
type ValidationEventType string

const (
	EventValidationStarted     ValidationEventType = "validation_started"
	EventValidationCompleted   ValidationEventType = "validation_completed"
	EventValidationFailed      ValidationEventType = "validation_failed"
	EventClusterUnreachable    ValidationEventType = "cluster_unreachable"
	EventClusterReconnected    ValidationEventType = "cluster_reconnected"
	EventTokenExpired          ValidationEventType = "token_expired"
	EventConfigurationChanged  ValidationEventType = "configuration_changed"
	EventHealthCheckFailed     ValidationEventType = "health_check_failed"
)

// EventSeverity represents the severity of a validation event
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityError    EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
)

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Timestamp          time.Time                      `json:"timestamp"`
	OverallHealthy     bool                           `json:"overall_healthy"`
	ClusterHealth      map[string]*ClusterHealth      `json:"cluster_health"`
	ServiceHealth      *ServiceHealth                 `json:"service_health"`
	ValidationSummary  *ValidationSummary             `json:"validation_summary"`
}

// ClusterHealth represents the health status of a single cluster
type ClusterHealth struct {
	ClusterName       string        `json:"cluster_name"`
	Healthy           bool          `json:"healthy"`
	LastValidated     time.Time     `json:"last_validated"`
	ResponseTime      time.Duration `json:"response_time"`
	ErrorCount        int           `json:"error_count"`
	ConsecutiveErrors int           `json:"consecutive_errors"`
	LastError         string        `json:"last_error,omitempty"`
	Availability      float64       `json:"availability"`
}

// ServiceHealth represents the health of the validation service itself
type ServiceHealth struct {
	Healthy                bool      `json:"healthy"`
	Uptime                 time.Duration `json:"uptime"`
	LastValidation         time.Time `json:"last_validation"`
	ValidationsPerformed   int64     `json:"validations_performed"`
	ErrorRate              float64   `json:"error_rate"`
	AverageValidationTime  time.Duration `json:"average_validation_time"`
}

// ValidationHistoryEntry represents a historical validation result
type ValidationHistoryEntry struct {
	Timestamp time.Time                   `json:"timestamp"`
	Result    *EnhancedValidationResult   `json:"result"`
	Duration  time.Duration              `json:"duration"`
}

// ValidationServiceMetrics tracks service performance metrics
type ValidationServiceMetrics struct {
	mutex                      sync.RWMutex
	TotalValidations           int64         `json:"total_validations"`
	SuccessfulValidations      int64         `json:"successful_validations"`
	FailedValidations          int64         `json:"failed_validations"`
	AverageValidationTime      time.Duration `json:"average_validation_time"`
	LastValidationTime         time.Time     `json:"last_validation_time"`
	ValidationTimeHistory      []time.Duration `json:"validation_time_history"`
	ClusterMetrics             map[string]*ClusterMetrics `json:"cluster_metrics"`
}

// ClusterMetrics tracks metrics for individual clusters
type ClusterMetrics struct {
	ClusterName           string        `json:"cluster_name"`
	SuccessfulValidations int64         `json:"successful_validations"`
	FailedValidations     int64         `json:"failed_validations"`
	AverageResponseTime   time.Duration `json:"average_response_time"`
	LastSuccessfulCheck   time.Time     `json:"last_successful_check"`
	ConsecutiveFailures   int           `json:"consecutive_failures"`
}

// LiveValidationServiceConfig configures the live validation service
type LiveValidationServiceConfig struct {
	Enabled               bool
	ValidationInterval    time.Duration
	HealthCheckInterval   time.Duration
	HTTPServerPort        int
	MaxHistoryEntries     int
	EnableEventHandlers   bool
	ValidationOptions     *EnhancedValidationOptions
}

// NewLiveValidationService creates a new live validation service
func NewLiveValidationService(config *MultiClusterConfig, serviceConfig *LiveValidationServiceConfig) *LiveValidationService {
	if serviceConfig == nil {
		serviceConfig = &LiveValidationServiceConfig{
			Enabled:             true,
			ValidationInterval:  30 * time.Second,
			HealthCheckInterval: 10 * time.Second,
			HTTPServerPort:      8090,
			MaxHistoryEntries:   100,
			EnableEventHandlers: true,
			ValidationOptions:   nil, // Use defaults
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &LiveValidationService{
		validator:           NewEnhancedMultiClusterValidator(serviceConfig.ValidationOptions),
		config:             config,
		enabled:            serviceConfig.Enabled,
		validationInterval: serviceConfig.ValidationInterval,
		healthCheckInterval: serviceConfig.HealthCheckInterval,
		httpServerPort:     serviceConfig.HTTPServerPort,
		ctx:                ctx,
		cancel:             cancel,
		validationHistory:  make([]*ValidationHistoryEntry, 0),
		maxHistoryEntries:  serviceConfig.MaxHistoryEntries,
		eventHandlers:      make(map[string][]ValidationEventHandler),
		metrics: &ValidationServiceMetrics{
			ClusterMetrics: make(map[string]*ClusterMetrics),
		},
	}

	return service
}

// Start starts the live validation service
func (lvs *LiveValidationService) Start() error {
	if !lvs.enabled {
		log.Println("Live validation service is disabled")
		return nil
	}

	log.Printf("Starting live validation service on port %d", lvs.httpServerPort)

	// Start HTTP server
	if err := lvs.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start validation routine
	go lvs.validationRoutine()

	// Start health check routine
	go lvs.healthCheckRoutine()

	log.Println("Live validation service started successfully")
	return nil
}

// Stop stops the live validation service
func (lvs *LiveValidationService) Stop() error {
	log.Println("Stopping live validation service")

	// Cancel context to stop routines
	lvs.cancel()

	// Stop HTTP server
	if lvs.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		if err := lvs.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}

	log.Println("Live validation service stopped")
	return nil
}

// startHTTPServer starts the HTTP API server
func (lvs *LiveValidationService) startHTTPServer() error {
	mux := http.NewServeMux()
	
	// API endpoints
	mux.HandleFunc("/health", lvs.handleHealth)
	mux.HandleFunc("/validation", lvs.handleValidation)
	mux.HandleFunc("/validation/status", lvs.handleValidationStatus)
	mux.HandleFunc("/validation/history", lvs.handleValidationHistory)
	mux.HandleFunc("/validation/trigger", lvs.handleTriggerValidation)
	mux.HandleFunc("/clusters", lvs.handleClusters)
	mux.HandleFunc("/clusters/", lvs.handleClusterDetail)
	mux.HandleFunc("/metrics", lvs.handleMetrics)
	mux.HandleFunc("/events", lvs.handleEvents)

	lvs.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", lvs.httpServerPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := lvs.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// validationRoutine runs periodic validations
func (lvs *LiveValidationService) validationRoutine() {
	ticker := time.NewTicker(lvs.validationInterval)
	defer ticker.Stop()

	log.Printf("Started validation routine with interval %v", lvs.validationInterval)

	// Run initial validation
	lvs.performValidation()

	for {
		select {
		case <-lvs.ctx.Done():
			log.Println("Validation routine stopped")
			return
		case <-ticker.C:
			lvs.performValidation()
		}
	}
}

// healthCheckRoutine runs periodic health checks
func (lvs *LiveValidationService) healthCheckRoutine() {
	ticker := time.NewTicker(lvs.healthCheckInterval)
	defer ticker.Stop()

	log.Printf("Started health check routine with interval %v", lvs.healthCheckInterval)

	for {
		select {
		case <-lvs.ctx.Done():
			log.Println("Health check routine stopped")
			return
		case <-ticker.C:
			lvs.performHealthCheck()
		}
	}
}

// performValidation performs a validation check
func (lvs *LiveValidationService) performValidation() {
	startTime := time.Now()
	
	log.Println("Performing scheduled validation")
	
	// Emit validation started event
	lvs.emitEvent(&ValidationEvent{
		Type:      EventValidationStarted,
		Timestamp: startTime,
		Severity:  SeverityInfo,
	})

	// Perform validation
	result := lvs.validator.ValidateMultiClusterConfigurationWithLiveChecks(lvs.config)
	result.GenerateSummary()
	
	duration := time.Since(startTime)

	// Update state
	lvs.mutex.Lock()
	lvs.lastValidationResult = result
	lvs.addToHistory(&ValidationHistoryEntry{
		Timestamp: startTime,
		Result:    result,
		Duration:  duration,
	})
	lvs.mutex.Unlock()

	// Update metrics
	lvs.updateValidationMetrics(result, duration)

	// Emit completion event
	eventType := EventValidationCompleted
	severity := SeverityInfo
	if !result.OverallValid {
		eventType = EventValidationFailed
		severity = SeverityError
	}

	lvs.emitEvent(&ValidationEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      result.Summary,
		Severity:  severity,
	})

	// Check for cluster-specific issues
	for clusterName, clusterResult := range result.ClusterResults {
		if !clusterResult.Valid {
			lvs.emitEvent(&ValidationEvent{
				Type:        EventClusterUnreachable,
				Timestamp:   time.Now(),
				ClusterName: clusterName,
				Data:        clusterResult.Errors,
				Severity:    SeverityError,
			})
		}
	}

	log.Printf("Validation completed in %v, overall valid: %v", duration, result.OverallValid)
}

// performHealthCheck performs a health check
func (lvs *LiveValidationService) performHealthCheck() {
	startTime := time.Now()
	
	healthResult := &HealthCheckResult{
		Timestamp:     startTime,
		ClusterHealth: make(map[string]*ClusterHealth),
		ServiceHealth: lvs.calculateServiceHealth(),
	}

	// Get cluster health from last validation
	lvs.mutex.RLock()
	lastValidation := lvs.lastValidationResult
	lvs.mutex.RUnlock()

	if lastValidation != nil {
		overallHealthy := true
		
		for clusterName, clusterResult := range lastValidation.ClusterResults {
			clusterHealth := &ClusterHealth{
				ClusterName:   clusterName,
				Healthy:       clusterResult.Valid,
				LastValidated: clusterResult.ValidatedAt,
				ErrorCount:    len(clusterResult.Errors),
			}
			
			if clusterResult.ConnectivityStatus != nil {
				clusterHealth.ResponseTime = clusterResult.ConnectivityStatus.ResponseTime
			}
			
			// Calculate availability from metrics
			if metrics, exists := lvs.metrics.ClusterMetrics[clusterName]; exists {
				total := metrics.SuccessfulValidations + metrics.FailedValidations
				if total > 0 {
					clusterHealth.Availability = float64(metrics.SuccessfulValidations) / float64(total)
				}
				clusterHealth.ConsecutiveErrors = metrics.ConsecutiveFailures
			}
			
			if !clusterHealth.Healthy {
				overallHealthy = false
			}
			
			healthResult.ClusterHealth[clusterName] = clusterHealth
		}
		
		healthResult.OverallHealthy = overallHealthy
		healthResult.ValidationSummary = lastValidation.Summary
	}

	// Update health check result
	lvs.mutex.Lock()
	lvs.lastHealthCheck = healthResult
	lvs.mutex.Unlock()

	// Emit health check events for failures
	if !healthResult.OverallHealthy {
		lvs.emitEvent(&ValidationEvent{
			Type:      EventHealthCheckFailed,
			Timestamp: startTime,
			Data:      healthResult,
			Severity:  SeverityWarning,
		})
	}
}

// calculateServiceHealth calculates service health metrics
func (lvs *LiveValidationService) calculateServiceHealth() *ServiceHealth {
	lvs.metrics.mutex.RLock()
	defer lvs.metrics.mutex.RUnlock()

	total := lvs.metrics.SuccessfulValidations + lvs.metrics.FailedValidations
	var errorRate float64
	if total > 0 {
		errorRate = float64(lvs.metrics.FailedValidations) / float64(total)
	}

	return &ServiceHealth{
		Healthy:               errorRate < 0.1, // Consider healthy if error rate < 10%
		Uptime:               time.Since(time.Now()), // This would be calculated from service start time
		LastValidation:       lvs.metrics.LastValidationTime,
		ValidationsPerformed: total,
		ErrorRate:            errorRate,
		AverageValidationTime: lvs.metrics.AverageValidationTime,
	}
}

// updateValidationMetrics updates validation metrics
func (lvs *LiveValidationService) updateValidationMetrics(result *EnhancedValidationResult, duration time.Duration) {
	lvs.metrics.mutex.Lock()
	defer lvs.metrics.mutex.Unlock()

	lvs.metrics.TotalValidations++
	if result.OverallValid {
		lvs.metrics.SuccessfulValidations++
	} else {
		lvs.metrics.FailedValidations++
	}

	// Update average validation time
	if lvs.metrics.TotalValidations == 1 {
		lvs.metrics.AverageValidationTime = duration
	} else {
		// Running average
		lvs.metrics.AverageValidationTime = time.Duration(
			(int64(lvs.metrics.AverageValidationTime)*int64(lvs.metrics.TotalValidations-1) + int64(duration)) /
			int64(lvs.metrics.TotalValidations))
	}

	lvs.metrics.LastValidationTime = time.Now()

	// Update cluster-specific metrics
	for clusterName, clusterResult := range result.ClusterResults {
		if _, exists := lvs.metrics.ClusterMetrics[clusterName]; !exists {
			lvs.metrics.ClusterMetrics[clusterName] = &ClusterMetrics{
				ClusterName: clusterName,
			}
		}

		metrics := lvs.metrics.ClusterMetrics[clusterName]
		
		if clusterResult.Valid {
			metrics.SuccessfulValidations++
			metrics.ConsecutiveFailures = 0
			metrics.LastSuccessfulCheck = time.Now()
		} else {
			metrics.FailedValidations++
			metrics.ConsecutiveFailures++
		}

		// Update response time
		if clusterResult.ConnectivityStatus != nil {
			responseTime := clusterResult.ConnectivityStatus.ResponseTime
			total := metrics.SuccessfulValidations + metrics.FailedValidations
			if total == 1 {
				metrics.AverageResponseTime = responseTime
			} else {
				// Running average
				metrics.AverageResponseTime = time.Duration(
					(int64(metrics.AverageResponseTime)*int64(total-1) + int64(responseTime)) / int64(total))
			}
		}
	}
}

// addToHistory adds a validation result to history
func (lvs *LiveValidationService) addToHistory(entry *ValidationHistoryEntry) {
	lvs.validationHistory = append(lvs.validationHistory, entry)
	
	// Keep only the last N entries
	if len(lvs.validationHistory) > lvs.maxHistoryEntries {
		lvs.validationHistory = lvs.validationHistory[1:]
	}
}

// Event handling

// RegisterEventHandler registers an event handler
func (lvs *LiveValidationService) RegisterEventHandler(eventType ValidationEventType, handler ValidationEventHandler) {
	lvs.eventMutex.Lock()
	defer lvs.eventMutex.Unlock()

	if _, exists := lvs.eventHandlers[string(eventType)]; !exists {
		lvs.eventHandlers[string(eventType)] = make([]ValidationEventHandler, 0)
	}
	
	lvs.eventHandlers[string(eventType)] = append(lvs.eventHandlers[string(eventType)], handler)
}

// emitEvent emits a validation event
func (lvs *LiveValidationService) emitEvent(event *ValidationEvent) {
	lvs.eventMutex.RLock()
	handlers, exists := lvs.eventHandlers[string(event.Type)]
	lvs.eventMutex.RUnlock()

	if !exists {
		return
	}

	// Execute handlers asynchronously
	for _, handler := range handlers {
		go func(h ValidationEventHandler) {
			if err := h(event); err != nil {
				log.Printf("Event handler error: %v", err)
			}
		}(handler)
	}
}

// HTTP API handlers

func (lvs *LiveValidationService) handleHealth(w http.ResponseWriter, r *http.Request) {
	lvs.mutex.RLock()
	health := lvs.lastHealthCheck
	lvs.mutex.RUnlock()

	if health == nil {
		http.Error(w, "Health check not available yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (lvs *LiveValidationService) handleValidation(w http.ResponseWriter, r *http.Request) {
	lvs.mutex.RLock()
	result := lvs.lastValidationResult
	lvs.mutex.RUnlock()

	if result == nil {
		http.Error(w, "Validation not available yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (lvs *LiveValidationService) handleValidationStatus(w http.ResponseWriter, r *http.Request) {
	lvs.mutex.RLock()
	result := lvs.lastValidationResult
	lvs.mutex.RUnlock()

	if result == nil {
		http.Error(w, "Validation not available yet", http.StatusServiceUnavailable)
		return
	}

	status := map[string]interface{}{
		"overall_valid":    result.OverallValid,
		"validation_time":  result.ValidationTime,
		"total_clusters":   len(result.ClusterResults),
		"valid_clusters":   0,
		"invalid_clusters": 0,
		"summary":         result.Summary,
	}

	for _, clusterResult := range result.ClusterResults {
		if clusterResult.Valid {
			status["valid_clusters"] = status["valid_clusters"].(int) + 1
		} else {
			status["invalid_clusters"] = status["invalid_clusters"].(int) + 1
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (lvs *LiveValidationService) handleValidationHistory(w http.ResponseWriter, r *http.Request) {
	lvs.mutex.RLock()
	history := make([]*ValidationHistoryEntry, len(lvs.validationHistory))
	copy(history, lvs.validationHistory)
	lvs.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (lvs *LiveValidationService) handleTriggerValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Trigger immediate validation
	go lvs.performValidation()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "triggered",
		"message": "Validation triggered successfully",
	})
}

func (lvs *LiveValidationService) handleClusters(w http.ResponseWriter, r *http.Request) {
	lvs.mutex.RLock()
	health := lvs.lastHealthCheck
	lvs.mutex.RUnlock()

	if health == nil {
		http.Error(w, "Health check not available yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health.ClusterHealth)
}

func (lvs *LiveValidationService) handleClusterDetail(w http.ResponseWriter, r *http.Request) {
	// Extract cluster name from URL path
	clusterName := strings.TrimPrefix(r.URL.Path, "/clusters/")
	if clusterName == "" {
		http.Error(w, "Cluster name required", http.StatusBadRequest)
		return
	}

	lvs.mutex.RLock()
	result := lvs.lastValidationResult
	lvs.mutex.RUnlock()

	if result == nil {
		http.Error(w, "Validation not available yet", http.StatusServiceUnavailable)
		return
	}

	clusterResult, exists := result.ClusterResults[clusterName]
	if !exists {
		http.Error(w, "Cluster not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clusterResult)
}

func (lvs *LiveValidationService) handleMetrics(w http.ResponseWriter, r *http.Request) {
	lvs.metrics.mutex.RLock()
	metrics := *lvs.metrics // Copy the metrics
	lvs.metrics.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (lvs *LiveValidationService) handleEvents(w http.ResponseWriter, r *http.Request) {
	// This could be enhanced to provide a event stream or webhook endpoint
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Event handling endpoint - could be enhanced for real-time events",
		"handlers": len(lvs.eventHandlers),
	})
}

// GetLastValidationResult returns the most recent validation result
func (lvs *LiveValidationService) GetLastValidationResult() *EnhancedValidationResult {
	lvs.mutex.RLock()
	defer lvs.mutex.RUnlock()
	return lvs.lastValidationResult
}

// GetHealthStatus returns the current health status
func (lvs *LiveValidationService) GetHealthStatus() *HealthCheckResult {
	lvs.mutex.RLock()
	defer lvs.mutex.RUnlock()
	return lvs.lastHealthCheck
}

// TriggerValidation triggers an immediate validation
func (lvs *LiveValidationService) TriggerValidation() {
	go lvs.performValidation()
}