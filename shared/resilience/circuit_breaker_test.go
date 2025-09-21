package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// MockMetricsCollector for testing
type MockMetricsCollector struct {
	counters map[string]float64
	gauges   map[string]float64
	durations map[string]time.Duration
	mu       sync.RWMutex
}

func NewMockMetricsCollector() *MockMetricsCollector {
	return &MockMetricsCollector{
		counters:  make(map[string]float64),
		gauges:    make(map[string]float64),
		durations: make(map[string]time.Duration),
	}
}

func (m *MockMetricsCollector) IncCounter(name string, labels map[string]string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	if len(labels) > 0 {
		for k, v := range labels {
			key += ":" + k + "=" + v
		}
	}
	m.counters[key] += value
}

func (m *MockMetricsCollector) SetGauge(name string, labels map[string]string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	if len(labels) > 0 {
		for k, v := range labels {
			key += ":" + k + "=" + v
		}
	}
	m.gauges[key] = value
}

func (m *MockMetricsCollector) RecordDuration(name string, labels map[string]string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	if len(labels) > 0 {
		for k, v := range labels {
			key += ":" + k + "=" + v
		}
	}
	m.durations[key] = duration
}

func (m *MockMetricsCollector) GetCounter(key string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counters[key]
}

func (m *MockMetricsCollector) GetGauge(key string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gauges[key]
}

func (m *MockMetricsCollector) GetDuration(key string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.durations[key]
}

func TestCircuitBreakerBasicFunctionality(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 3
	config.Timeout = 100 * time.Millisecond
	config.RecoveryTime = 200 * time.Millisecond
	
	cb := NewCircuitBreaker(config, monitoring)
	
	// Test initial state
	if cb.GetState() != StateClosed {
		t.Errorf("Expected initial state to be CLOSED, got %v", cb.GetState())
	}
	
	// Test successful operations
	ctx := context.Background()
	err := cb.Execute(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful operation, got error: %v", err)
	}
	
	metrics := cb.GetMetrics()
	if metrics.SuccessfulReqs != 1 {
		t.Errorf("Expected 1 successful request, got %d", metrics.SuccessfulReqs)
	}
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 3
	config.Timeout = 50 * time.Millisecond
	config.RecoveryTime = 100 * time.Millisecond
	config.HalfOpenMaxRequests = 2
	config.SuccessThreshold = 1
	
	stateChanges := make([]string, 0)
	config.OnStateChange = func(name string, from, to CircuitBreakerState) {
		stateChanges = append(stateChanges, from.String()+"->"+to.String())
	}
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Test transition to OPEN state
	for i := 0; i < config.FailureThreshold; i++ {
		err := cb.Execute(ctx, func() error {
			return errors.New("test failure")
		})
		if err == nil {
			t.Errorf("Expected error on failure %d", i+1)
		}
	}
	
	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be OPEN after %d failures, got %v", config.FailureThreshold, cb.GetState())
	}
	
	// Test rejection in OPEN state
	err := cb.Execute(ctx, func() error {
		return nil
	})
	if !IsCircuitBreakerError(err) {
		t.Errorf("Expected circuit breaker error in OPEN state, got: %v", err)
	}
	
	// Wait for recovery time and test transition to HALF_OPEN
	time.Sleep(config.RecoveryTime + 10*time.Millisecond)
	
	err = cb.Execute(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful operation in HALF_OPEN state, got: %v", err)
	}
	
	if cb.GetState() != StateClosed {
		t.Errorf("Expected state to be CLOSED after successful operation in HALF_OPEN, got %v", cb.GetState())
	}
	
	// Verify state transitions
	expectedTransitions := []string{"CLOSED->OPEN", "OPEN->HALF_OPEN", "HALF_OPEN->CLOSED"}
	if len(stateChanges) < len(expectedTransitions) {
		t.Errorf("Expected at least %d state transitions, got %d: %v", len(expectedTransitions), len(stateChanges), stateChanges)
	}
}

func TestCircuitBreakerConcurrency(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 10
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	var wg sync.WaitGroup
	numGoroutines := 100
	numOperationsPerGoroutine := 10
	
	// Test concurrent successful operations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				cb.Execute(ctx, func() error {
					return nil
				})
			}
		}()
	}
	wg.Wait()
	
	metrics := cb.GetMetrics()
	expectedSuccessful := int64(numGoroutines * numOperationsPerGoroutine)
	if metrics.SuccessfulReqs != expectedSuccessful {
		t.Errorf("Expected %d successful requests, got %d", expectedSuccessful, metrics.SuccessfulReqs)
	}
	
	// Test concurrent operations with some failures
	cb.Reset()
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				cb.Execute(ctx, func() error {
					if (id+j)%3 == 0 {
						return errors.New("test failure")
					}
					return nil
				})
			}
		}(i)
	}
	wg.Wait()
	
	metrics = cb.GetMetrics()
	if metrics.TotalRequests != expectedSuccessful {
		t.Errorf("Expected %d total requests, got %d", expectedSuccessful, metrics.TotalRequests)
	}
	
	if metrics.SuccessfulReqs+metrics.FailedReqs != metrics.TotalRequests {
		t.Errorf("Success + Failed should equal Total: %d + %d != %d", 
			metrics.SuccessfulReqs, metrics.FailedReqs, metrics.TotalRequests)
	}
}

func TestCircuitBreakerHalfOpenBehavior(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	config.RecoveryTime = 100 * time.Millisecond
	config.HalfOpenMaxRequests = 3
	config.SuccessThreshold = 2
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Trigger OPEN state
	for i := 0; i < config.FailureThreshold; i++ {
		cb.Execute(ctx, func() error {
			return errors.New("test failure")
		})
	}
	
	if cb.GetState() != StateOpen {
		t.Errorf("Expected OPEN state, got %v", cb.GetState())
	}
	
	// Wait for recovery time
	time.Sleep(config.RecoveryTime + 10*time.Millisecond)
	
	// Test limited requests in HALF_OPEN
	successCount := 0
	for i := 0; i < config.HalfOpenMaxRequests+2; i++ {
		err := cb.Execute(ctx, func() error {
			return nil
		})
		if err == nil {
			successCount++
		}
	}
	
	// Should only allow up to HalfOpenMaxRequests successful requests
	if successCount > config.HalfOpenMaxRequests {
		t.Errorf("Expected at most %d successful requests in HALF_OPEN, got %d", 
			config.HalfOpenMaxRequests, successCount)
	}
}

func TestCircuitBreakerMetrics(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.MonitoringEnabled = true
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Perform various operations
	cb.Execute(ctx, func() error { return nil })
	cb.Execute(ctx, func() error { return errors.New("failure") })
	cb.Execute(ctx, func() error { return nil })
	
	metrics := cb.GetMetrics()
	if metrics.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", metrics.TotalRequests)
	}
	if metrics.SuccessfulReqs != 2 {
		t.Errorf("Expected 2 successful requests, got %d", metrics.SuccessfulReqs)
	}
	if metrics.FailedReqs != 1 {
		t.Errorf("Expected 1 failed request, got %d", metrics.FailedReqs)
	}
	
	// Check if monitoring metrics were recorded
	successCounter := monitoring.GetCounter("circuit_breaker_successful_requests:circuit_breaker=test:state=CLOSED")
	if successCounter != 2 {
		t.Errorf("Expected 2 successful requests in metrics, got %f", successCounter)
	}
}

func TestCircuitBreakerTimeout(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	config.RecoveryTime = 100 * time.Millisecond
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Trigger OPEN state
	for i := 0; i < config.FailureThreshold; i++ {
		cb.Execute(ctx, func() error {
			return errors.New("test failure")
		})
	}
	
	if cb.GetState() != StateOpen {
		t.Errorf("Expected OPEN state, got %v", cb.GetState())
	}
	
	// Test immediate rejection
	err := cb.Execute(ctx, func() error {
		return nil
	})
	if !IsCircuitBreakerError(err) {
		t.Errorf("Expected circuit breaker error immediately after opening, got: %v", err)
	}
	
	// Wait for recovery time and verify transition
	time.Sleep(config.RecoveryTime + 10*time.Millisecond)
	
	// Should allow request now (transitions to HALF_OPEN)
	err = cb.Execute(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful operation after recovery time, got: %v", err)
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 2
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Trigger some failures
	cb.Execute(ctx, func() error { return errors.New("failure") })
	cb.Execute(ctx, func() error { return errors.New("failure") })
	
	if cb.GetState() != StateOpen {
		t.Errorf("Expected OPEN state, got %v", cb.GetState())
	}
	
	// Reset circuit breaker
	cb.Reset()
	
	if cb.GetState() != StateClosed {
		t.Errorf("Expected CLOSED state after reset, got %v", cb.GetState())
	}
	
	// Should allow operations immediately
	err := cb.Execute(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful operation after reset, got: %v", err)
	}
}

func TestCircuitBreakerForceOpen(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Force open
	cb.ForceOpen()
	
	if cb.GetState() != StateOpen {
		t.Errorf("Expected OPEN state after force open, got %v", cb.GetState())
	}
	
	// Should reject operations
	err := cb.Execute(ctx, func() error {
		return nil
	})
	if !IsCircuitBreakerError(err) {
		t.Errorf("Expected circuit breaker error after force open, got: %v", err)
	}
}

func TestCircuitBreakerError(t *testing.T) {
	err := NewCircuitBreakerError("test", StateOpen)
	
	if !IsCircuitBreakerError(err) {
		t.Errorf("IsCircuitBreakerError should return true for CircuitBreakerError")
	}
	
	if !IsCircuitBreakerError(err) {
		t.Errorf("IsCircuitBreakerError should return true for CircuitBreakerError")
	}
	
	normalError := errors.New("normal error")
	if IsCircuitBreakerError(normalError) {
		t.Errorf("IsCircuitBreakerError should return false for normal error")
	}
	
	expectedMessage := "circuit breaker 'test' is OPEN - request rejected"
	if err.Error() != expectedMessage {
		t.Errorf("Expected error message '%s', got '%s'", expectedMessage, err.Error())
	}
}

func TestCircuitBreakerConfigFromShared(t *testing.T) {
	retryConfig := &RetryConfig{
		CircuitBreakerThreshold:    10,
		CircuitBreakerTimeout:      2 * time.Minute,
		CircuitBreakerRecoveryTime: 5 * time.Minute,
	}
	
	config := ConfigFromShared("test", retryConfig)
	
	if config.FailureThreshold != 10 {
		t.Errorf("Expected failure threshold 10, got %d", config.FailureThreshold)
	}
	if config.Timeout != 2*time.Minute {
		t.Errorf("Expected timeout 2m, got %v", config.Timeout)
	}
	if config.RecoveryTime != 5*time.Minute {
		t.Errorf("Expected recovery time 5m, got %v", config.RecoveryTime)
	}
}

// Benchmark tests

func BenchmarkCircuitBreakerSuccessfulOperations(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("benchmark")
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Execute(ctx, func() error {
				return nil
			})
		}
	})
}

func BenchmarkCircuitBreakerFailedOperations(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("benchmark")
	config.FailureThreshold = 1000000 // Prevent opening during benchmark
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Execute(ctx, func() error {
				return errors.New("benchmark failure")
			})
		}
	})
}

func BenchmarkCircuitBreakerOpenState(b *testing.B) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("benchmark")
	config.FailureThreshold = 1
	cb := NewCircuitBreaker(config, monitoring)
	ctx := context.Background()
	
	// Trigger open state
	cb.Execute(ctx, func() error {
		return errors.New("trigger failure")
	})
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Execute(ctx, func() error {
				return nil
			})
		}
	})
}

// Integration test with context cancellation
func TestCircuitBreakerContextCancellation(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	cb := NewCircuitBreaker(config, monitoring)
	
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	err := cb.Execute(ctx, func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

// Test circuit breaker with slow operations
func TestCircuitBreakerSlowOperations(t *testing.T) {
	monitoring := NewMockMetricsCollector()
	config := DefaultCircuitBreakerConfig("test")
	cb := NewCircuitBreaker(config, monitoring)
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	start := time.Now()
	err := cb.Execute(ctx, func() error {
		time.Sleep(100 * time.Millisecond) // Longer than context timeout
		return nil
	})
	duration := time.Since(start)
	
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded error, got: %v", err)
	}
	
	if duration > 100*time.Millisecond {
		t.Errorf("Operation should have been cancelled by context timeout, took %v", duration)
	}
}