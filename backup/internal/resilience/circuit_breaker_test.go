package resilience

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_BasicOperation(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	
	// Test successful operation
	err := cb.Execute(func() error {
		return nil
	})
	
	if err != nil {
		t.Fatalf("Expected no error for successful operation, got: %v", err)
	}
	
	state, failures, _ := cb.GetState()
	if state != CircuitClosed {
		t.Errorf("Expected circuit to be closed, got: %v", state)
	}
	if failures != 0 {
		t.Errorf("Expected 0 failures, got: %d", failures)
	}
}

func TestCircuitBreaker_FailureHandling(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	
	testError := errors.New("test error")
	
	// First failure
	err := cb.Execute(func() error {
		return testError
	})
	
	if err != testError {
		t.Fatalf("Expected test error, got: %v", err)
	}
	
	state, failures, _ := cb.GetState()
	if state != CircuitClosed {
		t.Errorf("Expected circuit to be closed after 1 failure, got: %v", state)
	}
	if failures != 1 {
		t.Errorf("Expected 1 failure, got: %d", failures)
	}
	
	// Second failure should open the circuit
	err = cb.Execute(func() error {
		return testError
	})
	
	if err != testError {
		t.Fatalf("Expected test error, got: %v", err)
	}
	
	state, failures, _ = cb.GetState()
	if state != CircuitOpen {
		t.Errorf("Expected circuit to be open after 2 failures, got: %v", state)
	}
	if failures != 2 {
		t.Errorf("Expected 2 failures, got: %d", failures)
	}
}

func TestCircuitBreaker_OpenState(t *testing.T) {
	cb := NewCircuitBreaker(1, 100*time.Millisecond)
	
	// Trigger failure to open circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})
	
	// Next call should be rejected
	err := cb.Execute(func() error {
		t.Error("Operation should not be executed when circuit is open")
		return nil
	})
	
	if !IsCircuitBreakerError(err) {
		t.Errorf("Expected circuit breaker error, got: %v", err)
	}
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)
	
	// Open the circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})
	
	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)
	
	// Next call should succeed and close the circuit
	successCount := 0
	for i := 0; i < 3; i++ {
		err := cb.Execute(func() error {
			successCount++
			return nil
		})
		
		if err != nil {
			t.Fatalf("Expected success in half-open state, got: %v", err)
		}
	}
	
	state, failures, _ := cb.GetState()
	if state != CircuitClosed {
		t.Errorf("Expected circuit to be closed after successful calls, got: %v", state)
	}
	if failures != 0 {
		t.Errorf("Expected failures to be reset, got: %d", failures)
	}
	if successCount != 3 {
		t.Errorf("Expected 3 successful executions, got: %d", successCount)
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	
	stats := cb.GetStats()
	
	if stats.State != CircuitClosed {
		t.Errorf("Expected initial state to be closed, got: %v", stats.State)
	}
	if stats.MaxFailures != 3 {
		t.Errorf("Expected max failures to be 3, got: %d", stats.MaxFailures)
	}
	if stats.ResetTimeout != 100*time.Millisecond {
		t.Errorf("Expected reset timeout to be 100ms, got: %v", stats.ResetTimeout)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(1, 100*time.Millisecond)
	
	// Open the circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})
	
	state, failures, _ := cb.GetState()
	if state != CircuitOpen {
		t.Errorf("Expected circuit to be open, got: %v", state)
	}
	
	// Reset the circuit
	cb.Reset()
	
	state, failures, _ = cb.GetState()
	if state != CircuitClosed {
		t.Errorf("Expected circuit to be closed after reset, got: %v", state)
	}
	if failures != 0 {
		t.Errorf("Expected failures to be reset to 0, got: %d", failures)
	}
}