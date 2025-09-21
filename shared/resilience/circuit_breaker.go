package resilience

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
)

// CircuitBreakerState represents the different states of a circuit breaker
type CircuitBreakerState int32

const (
	// Closed - normal operation, requests are allowed
	StateClosed CircuitBreakerState = iota
	// Open - circuit is tripped, requests are rejected
	StateOpen
	// HalfOpen - limited requests allowed to test if service has recovered
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig defines circuit breaker configuration
type CircuitBreakerConfig struct {
	// Name identifies the circuit breaker instance
	Name string
	// FailureThreshold number of failures before opening circuit
	FailureThreshold int
	// Timeout duration to keep circuit open
	Timeout time.Duration
	// RecoveryTime duration before transitioning from OPEN to HALF_OPEN
	RecoveryTime time.Duration
	// HalfOpenMaxRequests maximum requests allowed in HALF_OPEN state
	HalfOpenMaxRequests int
	// SuccessThreshold number of successes needed to close circuit from HALF_OPEN
	SuccessThreshold int
	// MonitoringEnabled enables metrics and logging
	MonitoringEnabled bool
	// OnStateChange callback for state transitions
	OnStateChange func(name string, from, to CircuitBreakerState)
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:                name,
		FailureThreshold:    5,
		Timeout:             60 * time.Second,
		RecoveryTime:        300 * time.Second,
		HalfOpenMaxRequests: 3,
		SuccessThreshold:    2,
		MonitoringEnabled:   true,
	}
}

// ConfigFromShared creates circuit breaker config from shared configuration
func ConfigFromShared(name string, retryConfig *sharedconfig.RetryConfig) *CircuitBreakerConfig {
	config := DefaultCircuitBreakerConfig(name)
	if retryConfig != nil {
		config.FailureThreshold = retryConfig.CircuitBreakerThreshold
		config.Timeout = retryConfig.CircuitBreakerTimeout
		config.RecoveryTime = retryConfig.CircuitBreakerRecoveryTime
	}
	return config
}

// CircuitBreakerMetrics tracks circuit breaker performance
type CircuitBreakerMetrics struct {
	// State tracking
	State          CircuitBreakerState
	StateChanges   int64
	LastStateChange time.Time
	
	// Request metrics
	TotalRequests    int64
	SuccessfulReqs   int64
	FailedReqs       int64
	RejectedReqs     int64
	
	// State-specific metrics
	ClosedRequests   int64
	OpenRequests     int64
	HalfOpenRequests int64
	
	// Timing metrics
	FailureStreak    int64
	SuccessStreak    int64
	LastFailureTime  time.Time
	LastSuccessTime  time.Time
	OpenedAt         time.Time
	LastRecoveryAt   time.Time
	
	mu sync.RWMutex
}

// CircuitBreaker implements the circuit breaker pattern for resilience
type CircuitBreaker struct {
	config        *CircuitBreakerConfig
	state         int32 // atomic access to CircuitBreakerState
	metrics       *CircuitBreakerMetrics
	failureCount  int64
	successCount  int64
	requestCount  int64
	lastFailure   int64 // unix timestamp
	lastSuccess   int64 // unix timestamp
	stateChanged  int64 // unix timestamp
	mu            sync.RWMutex
	monitoring    monitoring.MetricsCollector
}

// NewCircuitBreaker creates a new circuit breaker instance
func NewCircuitBreaker(config *CircuitBreakerConfig, monitoring monitoring.MetricsCollector) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig("default")
	}
	
	cb := &CircuitBreaker{
		config:     config,
		state:      int32(StateClosed),
		metrics:    &CircuitBreakerMetrics{State: StateClosed},
		monitoring: monitoring,
	}
	
	// Initialize metrics if monitoring is provided
	if cb.monitoring != nil && config.MonitoringEnabled {
		cb.initializeMetrics()
	}
	
	return cb
}

// Execute wraps a function call with circuit breaker logic
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func() error) error {
	// Check if request should be allowed
	if !cb.AllowRequest() {
		atomic.AddInt64(&cb.metrics.RejectedReqs, 1)
		cb.recordMetric("circuit_breaker_rejected_requests", 1)
		return NewCircuitBreakerError(cb.config.Name, cb.GetState())
	}
	
	// Execute the operation
	start := time.Now()
	err := operation()
	duration := time.Since(start)
	
	// Record the result
	if err != nil {
		cb.RecordFailure()
		cb.recordDuration("circuit_breaker_failed_request_duration", duration)
		return err
	}
	
	cb.RecordSuccess()
	cb.recordDuration("circuit_breaker_success_request_duration", duration)
	return nil
}

// AllowRequest determines if a request should be allowed based on current state
func (cb *CircuitBreaker) AllowRequest() bool {
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	
	switch state {
	case StateClosed:
		return true
	case StateOpen:
		return cb.shouldAttemptRecovery()
	case StateHalfOpen:
		return cb.canMakeHalfOpenRequest()
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	atomic.AddInt64(&cb.metrics.TotalRequests, 1)
	atomic.AddInt64(&cb.metrics.SuccessfulReqs, 1)
	atomic.StoreInt64(&cb.lastSuccess, time.Now().Unix())
	
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	
	switch state {
	case StateClosed:
		// Reset failure count on success
		atomic.StoreInt64(&cb.failureCount, 0)
		atomic.AddInt64(&cb.metrics.ClosedRequests, 1)
	case StateHalfOpen:
		// Track success count in half-open state
		successCount := atomic.AddInt64(&cb.successCount, 1)
		atomic.AddInt64(&cb.metrics.HalfOpenRequests, 1)
		
		// Check if we should close the circuit
		if int(successCount) >= cb.config.SuccessThreshold {
			cb.transitionToState(StateClosed)
		}
	}
	
	cb.updateMetrics()
	cb.recordMetric("circuit_breaker_successful_requests", 1)
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	atomic.AddInt64(&cb.metrics.TotalRequests, 1)
	atomic.AddInt64(&cb.metrics.FailedReqs, 1)
	atomic.StoreInt64(&cb.lastFailure, time.Now().Unix())
	
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	
	switch state {
	case StateClosed:
		failureCount := atomic.AddInt64(&cb.failureCount, 1)
		atomic.AddInt64(&cb.metrics.ClosedRequests, 1)
		
		// Check if we should open the circuit
		if int(failureCount) >= cb.config.FailureThreshold {
			cb.transitionToState(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open immediately opens circuit
		atomic.AddInt64(&cb.metrics.HalfOpenRequests, 1)
		cb.transitionToState(StateOpen)
	}
	
	cb.updateMetrics()
	cb.recordMetric("circuit_breaker_failed_requests", 1)
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return CircuitBreakerState(atomic.LoadInt32(&cb.state))
}

// GetMetrics returns current circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.metrics.mu.RLock()
	defer cb.metrics.mu.RUnlock()
	return *cb.metrics
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	atomic.StoreInt32(&cb.state, int32(StateClosed))
	atomic.StoreInt64(&cb.failureCount, 0)
	atomic.StoreInt64(&cb.successCount, 0)
	atomic.StoreInt64(&cb.requestCount, 0)
	
	cb.updateMetrics()
	cb.recordStateChange("manual_reset")
}

// ForceOpen forces the circuit breaker to open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.transitionToState(StateOpen)
	cb.recordStateChange("forced_open")
}

// Private methods

func (cb *CircuitBreaker) shouldAttemptRecovery() bool {
	lastFailureTime := atomic.LoadInt64(&cb.lastFailure)
	if lastFailureTime == 0 {
		return false
	}
	
	recoveryTime := time.Unix(lastFailureTime, 0).Add(cb.config.RecoveryTime)
	if time.Now().After(recoveryTime) {
		cb.transitionToState(StateHalfOpen)
		return true
	}
	
	return false
}

func (cb *CircuitBreaker) canMakeHalfOpenRequest() bool {
	// Check if we've exceeded max requests in half-open state
	currentRequests := atomic.LoadInt64(&cb.requestCount)
	if int(currentRequests) >= cb.config.HalfOpenMaxRequests {
		return false
	}
	
	atomic.AddInt64(&cb.requestCount, 1)
	return true
}

func (cb *CircuitBreaker) transitionToState(newState CircuitBreakerState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	oldState := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	if oldState == newState {
		return
	}
	
	atomic.StoreInt32(&cb.state, int32(newState))
	atomic.StoreInt64(&cb.stateChanged, time.Now().Unix())
	
	// Reset counters based on state transition
	switch newState {
	case StateClosed:
		atomic.StoreInt64(&cb.failureCount, 0)
		atomic.StoreInt64(&cb.successCount, 0)
		atomic.StoreInt64(&cb.requestCount, 0)
	case StateOpen:
		atomic.StoreInt64(&cb.successCount, 0)
		atomic.StoreInt64(&cb.requestCount, 0)
		cb.metrics.OpenedAt = time.Now()
	case StateHalfOpen:
		atomic.StoreInt64(&cb.successCount, 0)
		atomic.StoreInt64(&cb.requestCount, 0)
		cb.metrics.LastRecoveryAt = time.Now()
	}
	
	cb.updateMetrics()
	cb.recordStateTransition(oldState, newState)
	
	// Call state change callback if provided
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

func (cb *CircuitBreaker) updateMetrics() {
	cb.metrics.mu.Lock()
	defer cb.metrics.mu.Unlock()
	
	cb.metrics.State = CircuitBreakerState(atomic.LoadInt32(&cb.state))
	cb.metrics.TotalRequests = atomic.LoadInt64(&cb.metrics.TotalRequests)
	cb.metrics.SuccessfulReqs = atomic.LoadInt64(&cb.metrics.SuccessfulReqs)
	cb.metrics.FailedReqs = atomic.LoadInt64(&cb.metrics.FailedReqs)
	cb.metrics.RejectedReqs = atomic.LoadInt64(&cb.metrics.RejectedReqs)
	cb.metrics.FailureStreak = atomic.LoadInt64(&cb.failureCount)
	cb.metrics.SuccessStreak = atomic.LoadInt64(&cb.successCount)
	
	if lastFailure := atomic.LoadInt64(&cb.lastFailure); lastFailure > 0 {
		cb.metrics.LastFailureTime = time.Unix(lastFailure, 0)
	}
	if lastSuccess := atomic.LoadInt64(&cb.lastSuccess); lastSuccess > 0 {
		cb.metrics.LastSuccessTime = time.Unix(lastSuccess, 0)
	}
	if stateChanged := atomic.LoadInt64(&cb.stateChanged); stateChanged > 0 {
		cb.metrics.LastStateChange = time.Unix(stateChanged, 0)
	}
}

func (cb *CircuitBreaker) initializeMetrics() {
	if cb.monitoring == nil {
		return
	}
	
	// Initialize metric counters
	labels := map[string]string{"circuit_breaker": cb.config.Name}
	cb.monitoring.IncCounter("circuit_breaker_total_requests", labels, 0)
	cb.monitoring.IncCounter("circuit_breaker_successful_requests", labels, 0)
	cb.monitoring.IncCounter("circuit_breaker_failed_requests", labels, 0)
	cb.monitoring.IncCounter("circuit_breaker_rejected_requests", labels, 0)
	cb.monitoring.IncCounter("circuit_breaker_state_changes", labels, 0)
}

func (cb *CircuitBreaker) recordMetric(metricName string, value float64) {
	if cb.monitoring == nil || !cb.config.MonitoringEnabled {
		return
	}
	
	labels := map[string]string{
		"circuit_breaker": cb.config.Name,
		"state":          cb.GetState().String(),
	}
	cb.monitoring.IncCounter(metricName, labels, value)
}

func (cb *CircuitBreaker) recordDuration(metricName string, duration time.Duration) {
	if cb.monitoring == nil || !cb.config.MonitoringEnabled {
		return
	}
	
	labels := map[string]string{
		"circuit_breaker": cb.config.Name,
		"state":          cb.GetState().String(),
	}
	cb.monitoring.RecordDuration(metricName, labels, duration)
}

func (cb *CircuitBreaker) recordStateTransition(from, to CircuitBreakerState) {
	if cb.monitoring == nil || !cb.config.MonitoringEnabled {
		return
	}
	
	atomic.AddInt64(&cb.metrics.StateChanges, 1)
	
	labels := map[string]string{
		"circuit_breaker": cb.config.Name,
		"from_state":     from.String(),
		"to_state":       to.String(),
	}
	cb.monitoring.IncCounter("circuit_breaker_state_changes", labels, 1)
	cb.recordStateChange(fmt.Sprintf("transition_%s_to_%s", from.String(), to.String()))
}

func (cb *CircuitBreaker) recordStateChange(reason string) {
	if cb.monitoring == nil || !cb.config.MonitoringEnabled {
		return
	}
	
	labels := map[string]string{
		"circuit_breaker": cb.config.Name,
		"state":          cb.GetState().String(),
		"reason":         reason,
	}
	cb.monitoring.IncCounter("circuit_breaker_state_change_events", labels, 1)
}

// CircuitBreakerError represents an error when circuit breaker rejects a request
type CircuitBreakerError struct {
	CircuitName string
	State       CircuitBreakerState
}

func (e *CircuitBreakerError) Error() string {
	return fmt.Sprintf("circuit breaker '%s' is %s - request rejected", e.CircuitName, e.State.String())
}

func NewCircuitBreakerError(name string, state CircuitBreakerState) *CircuitBreakerError {
	return &CircuitBreakerError{
		CircuitName: name,
		State:       state,
	}
}

// IsCircuitBreakerError checks if error is a circuit breaker error
func IsCircuitBreakerError(err error) bool {
	_, ok := err.(*CircuitBreakerError)
	return ok
}