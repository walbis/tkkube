package resilience

import (
	"sync"
	"time"
	sharedErrors "shared-errors"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// String returns the string representation of CircuitState
func (cs CircuitState) String() string {
	switch cs {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}


// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	maxFailures   int
	resetTimeout  time.Duration
	state         CircuitState
	failures      int
	lastFailTime  time.Time
	mutex         sync.RWMutex
	successCount  int
	halfOpenLimit int
}

// NewCircuitBreaker creates a new circuit breaker with the specified parameters
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:   maxFailures,
		resetTimeout:  resetTimeout,
		state:         CircuitClosed,
		halfOpenLimit: 3, // Allow 3 attempts in half-open state
	}
}

// Execute runs the given operation with circuit breaker protection
func (cb *CircuitBreaker) Execute(operation func() error) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Check if we should move from open to half-open
	if cb.state == CircuitOpen && time.Since(cb.lastFailTime) > cb.resetTimeout {
		cb.state = CircuitHalfOpen
		cb.successCount = 0
	}

	// Reject operation if circuit is open
	if cb.state == CircuitOpen {
		return NewCircuitBreakerError("circuit_breaker", cb.state, cb.failures, cb.lastFailTime)
	}

	// Execute the operation
	err := operation()
	
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// recordFailure handles failure recording and state transitions
func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailTime = time.Now()

	if cb.state == CircuitHalfOpen || cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

// recordSuccess handles success recording and state transitions
func (cb *CircuitBreaker) recordSuccess() {
	if cb.state == CircuitClosed {
		// Reset failure count on success in closed state
		cb.failures = 0
		return
	}

	if cb.state == CircuitHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.halfOpenLimit {
			// Move to closed state after enough successes
			cb.state = CircuitClosed
			cb.failures = 0
			cb.successCount = 0
		}
	}
}

// GetState returns the current state and metrics of the circuit breaker
func (cb *CircuitBreaker) GetState() (CircuitState, int, time.Time) {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state, cb.failures, cb.lastFailTime
}

// GetStats returns detailed statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	
	return CircuitBreakerStats{
		State:         cb.state,
		Failures:      cb.failures,
		SuccessCount:  cb.successCount,
		LastFailTime:  cb.lastFailTime,
		MaxFailures:   cb.maxFailures,
		ResetTimeout:  cb.resetTimeout,
		HalfOpenLimit: cb.halfOpenLimit,
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successCount = 0
	cb.lastFailTime = time.Time{}
}

// CircuitBreakerStats contains statistics about a circuit breaker
type CircuitBreakerStats struct {
	State         CircuitState
	Failures      int
	SuccessCount  int
	LastFailTime  time.Time
	MaxFailures   int
	ResetTimeout  time.Duration
	HalfOpenLimit int
}

// CircuitBreakerError is returned when a circuit breaker is open
// NewCircuitBreakerError creates a circuit breaker error with context
func NewCircuitBreakerError(name string, state CircuitState, failures int, lastFailTime time.Time) *sharedErrors.StandardError {
	return sharedErrors.New(sharedErrors.ErrCodeCircuitBreaker, "resilience", "circuit_breaker", 
		"circuit breaker is open due to repeated failures").
		WithContext("circuit_breaker_name", name).
		WithContext("state", state.String()).
		WithContext("failures", failures).
		WithContext("last_fail_time", lastFailTime).
		WithUserMessage("Service temporarily unavailable due to repeated failures. Please try again later.")
}

// IsCircuitBreakerError checks if an error is a circuit breaker error
func IsCircuitBreakerError(err error) bool {
	return sharedErrors.IsCode(err, sharedErrors.ErrCodeCircuitBreaker)
}