package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	sharedconfig "shared-config/config"
)

// BenchmarkHTTPClient tests the performance of our optimized HTTP client
func BenchmarkHTTPClient(b *testing.B) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate processing time
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "timestamp": "` + time.Now().Format(time.RFC3339) + `"}`))
	}))
	defer server.Close()

	b.Run("OptimizedClient", func(b *testing.B) {
		client := NewHTTPClient(DefaultHTTPClientConfig())
		defer client.Close()

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				resp, err := client.Get(ctx, server.URL)
				if err != nil {
					b.Errorf("Request failed: %v", err)
				} else {
					resp.Body.Close()
				}
				cancel()
			}
		})
	})

	b.Run("StandardClient", func(b *testing.B) {
		client := &http.Client{Timeout: 30 * time.Second}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				resp, err := client.Get(server.URL)
				if err != nil {
					b.Errorf("Request failed: %v", err)
				} else {
					resp.Body.Close()
				}
			}
		})
	})
}

// BenchmarkConnectionPooling tests connection reuse performance
func BenchmarkConnectionPooling(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	b.Run("WithPooling", func(b *testing.B) {
		config := DefaultHTTPClientConfig()
		config.MaxIdleConnsPerHost = 50
		config.MaxConnsPerHost = 100
		client := NewHTTPClient(config)
		defer client.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			resp, err := client.Get(ctx, server.URL)
			if err != nil {
				b.Errorf("Request failed: %v", err)
			} else {
				resp.Body.Close()
			}
		}
	})

	b.Run("WithoutPooling", func(b *testing.B) {
		config := DefaultHTTPClientConfig()
		config.MaxIdleConnsPerHost = 0 // Disable pooling
		client := NewHTTPClient(config)
		defer client.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			resp, err := client.Get(ctx, server.URL)
			if err != nil {
				b.Errorf("Request failed: %v", err)
			} else {
				resp.Body.Close()
			}
		}
	})
}

// BenchmarkClientPool tests the performance of the client pool
func BenchmarkClientPool(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &sharedconfig.SharedConfig{}
	manager := NewHTTPClientManager(config)
	defer manager.Close()

	b.Run("WebhookClient", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				client := manager.GetWebhookClient()
				ctx := context.Background()
				resp, err := client.Get(ctx, server.URL)
				if err != nil {
					b.Errorf("Request failed: %v", err)
				} else {
					resp.Body.Close()
				}
			}
		})
	})

	b.Run("StorageClient", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				client := manager.GetStorageClient()
				ctx := context.Background()
				resp, err := client.Get(ctx, server.URL)
				if err != nil {
					b.Errorf("Request failed: %v", err)
				} else {
					resp.Body.Close()
				}
			}
		})
	})
}

// BenchmarkRetryLogic tests retry performance under failure conditions
func BenchmarkRetryLogic(b *testing.B) {
	failureCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failureCount++
		// Fail first 2 requests, then succeed
		if failureCount%3 == 0 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error"))
		}
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.MaxRetries = 3
	config.RetryDelay = 10 * time.Millisecond
	client := NewHTTPClient(config)
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err == nil {
			resp.Body.Close()
		}
	}
}

// BenchmarkCircuitBreaker tests circuit breaker performance
func BenchmarkCircuitBreaker(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always fail to test circuit breaker
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error"))
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.CircuitBreakerEnabled = true
	config.FailureThreshold = 5
	config.MaxRetries = 1
	client := NewHTTPClient(config)
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err == nil {
			resp.Body.Close()
		}
	}
}

// BenchmarkConcurrentRequests tests performance under high concurrency
func BenchmarkConcurrentRequests(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond) // Simulate processing
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.MaxConnsPerHost = 100
	client := NewHTTPClient(config)
	defer client.Close()

	concurrency := []int{1, 10, 50, 100}

	for _, c := range concurrency {
		b.Run(fmt.Sprintf("Concurrency%d", c), func(b *testing.B) {
			b.SetParallelism(c)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					ctx := context.Background()
					resp, err := client.Get(ctx, server.URL)
					if err != nil {
						b.Errorf("Request failed: %v", err)
					} else {
						resp.Body.Close()
					}
				}
			})
		})
	}
}

// Performance test that measures actual metrics
func TestHTTPClientPerformanceMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := NewHTTPClient(DefaultHTTPClientConfig())
	defer client.Close()

	// Warm up
	for i := 0; i < 10; i++ {
		ctx := context.Background()
		resp, _ := client.Get(ctx, server.URL)
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Reset metrics before measurement
	client.ResetMetrics()

	// Perform test requests
	numRequests := 100
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			resp, err := client.Get(ctx, server.URL)
			if err != nil {
				t.Errorf("Request failed: %v", err)
			} else {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	duration := time.Since(start)
	metrics := client.GetMetrics()

	t.Logf("Performance Test Results:")
	t.Logf("  Total Requests: %d", metrics.TotalRequests)
	t.Logf("  Successful Requests: %d", metrics.SuccessfulReqs)
	t.Logf("  Failed Requests: %d", metrics.FailedReqs)
	t.Logf("  Success Rate: %.2f%%", float64(metrics.SuccessfulReqs)/float64(metrics.TotalRequests)*100)
	t.Logf("  Average Response Time: %v", metrics.AvgResponseTime)
	t.Logf("  Total Duration: %v", duration)
	t.Logf("  Requests/Second: %.2f", float64(numRequests)/duration.Seconds())
	t.Logf("  Retry Attempts: %d", metrics.RetryAttempts)
	t.Logf("  Timeout Errors: %d", metrics.TimeoutErrors)
	t.Logf("  Connection Errors: %d", metrics.ConnectionErrors)

	// Verify performance expectations
	if metrics.SuccessfulReqs != int64(numRequests) {
		t.Errorf("Expected %d successful requests, got %d", numRequests, metrics.SuccessfulReqs)
	}

	if metrics.AvgResponseTime > 50*time.Millisecond {
		t.Errorf("Average response time too high: %v", metrics.AvgResponseTime)
	}

	requestsPerSecond := float64(numRequests) / duration.Seconds()
	if requestsPerSecond < 500 { // Expect at least 500 RPS
		t.Errorf("Requests per second too low: %.2f", requestsPerSecond)
	}
}

// Test client pool performance under load
func TestHTTPClientPoolPerformance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &sharedconfig.SharedConfig{}
	manager := NewHTTPClientManager(config)
	defer manager.Close()

	// Test different client types
	clientTypes := []string{"webhook", "storage", "api", "monitoring"}
	numRequestsPerType := 50

	start := time.Now()
	var wg sync.WaitGroup

	for _, clientType := range clientTypes {
		for i := 0; i < numRequestsPerType; i++ {
			wg.Add(1)
			go func(ct string) {
				defer wg.Done()
				client, err := manager.GetClient(ct)
				if err != nil {
					t.Errorf("Failed to get client %s: %v", ct, err)
					return
				}
				ctx := context.Background()
				resp, err := client.Get(ctx, server.URL)
				if err != nil {
					t.Errorf("Request failed for %s: %v", ct, err)
				} else {
					resp.Body.Close()
				}
			}(clientType)
		}
	}

	wg.Wait()
	duration := time.Since(start)

	poolMetrics := manager.GetMetrics()
	connectionStats := manager.GetConnectionStats()

	t.Logf("Pool Performance Test Results:")
	t.Logf("  Total Clients: %d", poolMetrics.TotalClients)
	t.Logf("  Total Requests: %d", poolMetrics.AggregatedMetrics.TotalRequests)
	t.Logf("  Success Rate: %.2f%%", connectionStats["success_rate"])
	t.Logf("  Average Response Time: %v ms", connectionStats["avg_response_time"])
	t.Logf("  Total Duration: %v", duration)
	t.Logf("  Requests/Second: %.2f", float64(len(clientTypes)*numRequestsPerType)/duration.Seconds())

	// Verify pool created expected number of clients
	if poolMetrics.TotalClients != len(clientTypes) {
		t.Errorf("Expected %d clients, got %d", len(clientTypes), poolMetrics.TotalClients)
	}

	// Verify all requests succeeded
	expectedRequests := int64(len(clientTypes) * numRequestsPerType)
	if poolMetrics.AggregatedMetrics.TotalRequests != expectedRequests {
		t.Errorf("Expected %d total requests, got %d", expectedRequests, poolMetrics.AggregatedMetrics.TotalRequests)
	}
}

// Stress test for connection pooling limits
func TestConnectionPoolingLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response to test connection limits
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.MaxConnsPerHost = 10 // Limit connections
	config.RequestTimeout = 1 * time.Second
	client := NewHTTPClient(config)
	defer client.Close()

	// Start many concurrent requests (more than connection limit)
	numRequests := 50
	var wg sync.WaitGroup
	errors := make([]error, numRequests)

	start := time.Now()
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ctx := context.Background()
			resp, err := client.Get(ctx, server.URL)
			errors[index] = err
			if resp != nil {
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// Count successful requests
	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}

	metrics := client.GetMetrics()

	t.Logf("Connection Pooling Limits Test:")
	t.Logf("  Max Connections Per Host: %d", config.MaxConnsPerHost)
	t.Logf("  Total Requests: %d", numRequests)
	t.Logf("  Successful Requests: %d", successCount)
	t.Logf("  Success Rate: %.2f%%", float64(successCount)/float64(numRequests)*100)
	t.Logf("  Total Duration: %v", duration)
	t.Logf("  Timeout Errors: %d", metrics.TimeoutErrors)
	t.Logf("  Connection Errors: %d", metrics.ConnectionErrors)

	// Verify that connection pooling works (should complete all requests eventually)
	if successCount < numRequests/2 {
		t.Errorf("Success rate too low: %d/%d", successCount, numRequests)
	}
}