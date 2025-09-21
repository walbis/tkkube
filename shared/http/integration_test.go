package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sharedconfig "shared-config/config"
)

// TestHTTPClientBasicFunctionality tests basic HTTP client operations
func TestHTTPClientBasicFunctionality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "success"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(DefaultHTTPClientConfig())
	defer client.Close()

	t.Run("GET Request", func(t *testing.T) {
		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("GET request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST Request", func(t *testing.T) {
		ctx := context.Background()
		payload := map[string]string{"test": "data"}
		resp, err := client.PostJSON(ctx, server.URL, payload)
		if err != nil {
			t.Fatalf("POST request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Verify metrics are being collected
	metrics := client.GetMetrics()
	if metrics.TotalRequests < 2 {
		t.Errorf("Expected at least 2 requests, got %d", metrics.TotalRequests)
	}
	if metrics.SuccessfulReqs < 2 {
		t.Errorf("Expected at least 2 successful requests, got %d", metrics.SuccessfulReqs)
	}
}

// TestHTTPClientRetryLogic tests retry functionality
func TestHTTPClientRetryLogic(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 1 {
			// Fail first request only
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error"))
		} else {
			// Succeed on 2nd request
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		}
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.MaxRetries = 3
	config.RetryDelay = 10 * time.Millisecond
	client := NewHTTPClient(config)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Request should have succeeded after retries: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	metrics := client.GetMetrics()
	if metrics.RetryAttempts == 0 {
		t.Error("Expected retry attempts to be recorded")
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests (1 failure + 1 success), got %d", requestCount)
	}
}

// TestHTTPClientCircuitBreaker tests circuit breaker functionality
func TestHTTPClientCircuitBreaker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always fail to trigger circuit breaker
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error"))
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.CircuitBreakerEnabled = true
	config.FailureThreshold = 3
	config.MaxRetries = 0 // No retries to speed up test
	client := NewHTTPClient(config)
	defer client.Close()

	ctx := context.Background()

	// Make requests until circuit breaker opens
	var lastErr error
	for i := 0; i < 10; i++ {
		resp, err := client.Get(ctx, server.URL)
		if resp != nil {
			resp.Body.Close()
		}
		lastErr = err
		
		// Check if circuit breaker is open
		if err != nil && err.Error() == "circuit breaker is open" {
			break
		}
	}

	if lastErr == nil || lastErr.Error() != "circuit breaker is open" {
		t.Error("Expected circuit breaker to open after failures")
	}

	metrics := client.GetMetrics()
	if metrics.CircuitBreakerHits == 0 {
		t.Error("Expected circuit breaker hits to be recorded")
	}
}

// TestHTTPClientTimeout tests timeout functionality
func TestHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.RequestTimeout = 50 * time.Millisecond // Shorter than server response time
	client := NewHTTPClient(config)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if resp != nil {
		resp.Body.Close()
	}

	if err == nil {
		t.Error("Expected request to timeout")
	}

	metrics := client.GetMetrics()
	if metrics.TimeoutErrors == 0 {
		t.Error("Expected timeout errors to be recorded")
	}
}

// TestHTTPClientPoolIntegration tests client pool functionality
func TestHTTPClientPoolIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &sharedconfig.SharedConfig{}
	manager := NewHTTPClientManager(config)
	defer manager.Close()

	t.Run("GetWebhookClient", func(t *testing.T) {
		client := manager.GetWebhookClient()
		if client == nil {
			t.Fatal("Expected webhook client, got nil")
		}

		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("Webhook client request failed: %v", err)
		}
		defer resp.Body.Close()
	})

	t.Run("GetStorageClient", func(t *testing.T) {
		client := manager.GetStorageClient()
		if client == nil {
			t.Fatal("Expected storage client, got nil")
		}

		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("Storage client request failed: %v", err)
		}
		defer resp.Body.Close()
	})

	t.Run("GetAPIClient", func(t *testing.T) {
		client := manager.GetAPIClient()
		if client == nil {
			t.Fatal("Expected API client, got nil")
		}

		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("API client request failed: %v", err)
		}
		defer resp.Body.Close()
	})

	t.Run("GetMonitoringClient", func(t *testing.T) {
		client := manager.GetMonitoringClient()
		if client == nil {
			t.Fatal("Expected monitoring client, got nil")
		}

		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("Monitoring client request failed: %v", err)
		}
		defer resp.Body.Close()
	})

	// Test pool metrics
	poolMetrics := manager.GetMetrics()
	if poolMetrics.TotalClients == 0 {
		t.Error("Expected clients to be created in pool")
	}

	if poolMetrics.AggregatedMetrics.TotalRequests == 0 {
		t.Error("Expected requests to be recorded in pool metrics")
	}

	// Test connection stats
	stats := manager.GetConnectionStats()
	if stats["total_clients"] == 0 {
		t.Error("Expected connection stats to show created clients")
	}
}

// TestHTTPClientCompression tests compression functionality
func TestHTTPClientCompression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client sent compression headers
		acceptEncoding := r.Header.Get("Accept-Encoding")
		if acceptEncoding == "" {
			t.Error("Expected Accept-Encoding header")
		}

		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		
		// Send uncompressed data - the test server doesn't actually compress
		w.Write([]byte("Uncompressed data"))
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.CompressionEnabled = true
	client := NewHTTPClient(config)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestHTTPClientWithSharedConfig tests integration with shared configuration
func TestHTTPClientWithSharedConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Create mock shared config
	config := &sharedconfig.SharedConfig{
		Storage: sharedconfig.StorageConfig{
			Connection: sharedconfig.ConnectionConfig{
				Timeout:    10,
				MaxRetries: 2,
				RetryDelay: 1 * time.Second,
			},
		},
		Pipeline: sharedconfig.PipelineConfig{
			ErrorHandling: sharedconfig.ErrorHandlingConfig{
				MaxRetries: 3,
				RetryDelay: 2 * time.Second,
			},
		},
		Performance: sharedconfig.PerformanceConfig{
			Limits: sharedconfig.LimitsConfig{
				MaxConcurrentOperations: 20,
			},
			Optimization: sharedconfig.OptimizationConfig{
				Compression: true,
			},
		},
		Security: sharedconfig.SecurityConfig{
			Network: sharedconfig.NetworkConfig{
				VerifySSL: true,
			},
		},
	}

	t.Run("ProductionProfile", func(t *testing.T) {
		client := NewHTTPClientFromConfig(config, "production")
		defer client.Close()

		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Verify config was applied
		clientConfig := client.GetConfig()
		if clientConfig.MaxRetries != 3 { // Should use pipeline error handling
			t.Errorf("Expected MaxRetries=3, got %d", clientConfig.MaxRetries)
		}
		// Storage timeout (10s) should override production profile (30s) - this is correct behavior
		if clientConfig.RequestTimeout != 10*time.Second {
			t.Errorf("Expected RequestTimeout=10s (from storage config), got %v", clientConfig.RequestTimeout)
		}
	})

	t.Run("DevelopmentProfile", func(t *testing.T) {
		client := NewHTTPClientFromConfig(config, "development")
		defer client.Close()

		ctx := context.Background()
		resp, err := client.Get(ctx, server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Verify development profile settings
		clientConfig := client.GetConfig()
		if clientConfig.CircuitBreakerEnabled {
			t.Error("Expected circuit breaker to be disabled in development")
		}
		if clientConfig.RequestTimeout != 10*time.Second {
			t.Errorf("Expected RequestTimeout=10s, got %v", clientConfig.RequestTimeout)
		}
	})
}

// TestHTTPClientHealthCheck tests health check functionality
func TestHTTPClientHealthCheck(t *testing.T) {
	config := &sharedconfig.SharedConfig{}
	manager := NewHTTPClientManager(config)
	defer manager.Close()

	ctx := context.Background()
	healthResults := manager.HealthCheck(ctx)

	if len(healthResults) == 0 {
		t.Error("Expected health check results for clients")
	}

	for profile, err := range healthResults {
		if err != nil {
			t.Logf("Health check for %s: %v", profile, err)
		} else {
			t.Logf("Health check for %s: OK", profile)
		}
	}
}

// TestHTTPClientMetricsAccuracy tests metrics accuracy
func TestHTTPClientMetricsAccuracy(t *testing.T) {
	successCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		successCount++
		if successCount <= 3 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error"))
		}
	}))
	defer server.Close()

	config := DefaultHTTPClientConfig()
	config.MaxRetries = 0 // No retries for precise counting
	client := NewHTTPClient(config)
	defer client.Close()

	ctx := context.Background()

	// Make 5 requests (3 success, 2 failure)
	for i := 0; i < 5; i++ {
		resp, err := client.Get(ctx, server.URL)
		if resp != nil {
			resp.Body.Close()
		}
		_ = err // Ignore errors for this test
	}

	metrics := client.GetMetrics()

	if metrics.TotalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", metrics.TotalRequests)
	}
	if metrics.SuccessfulReqs != 3 {
		t.Errorf("Expected 3 successful requests, got %d", metrics.SuccessfulReqs)
	}
	if metrics.FailedReqs != 2 {
		t.Errorf("Expected 2 failed requests, got %d", metrics.FailedReqs)
	}
}

// TestHTTPClientConcurrencySafety tests thread safety
func TestHTTPClientConcurrencySafety(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := NewHTTPClient(DefaultHTTPClientConfig())
	defer client.Close()

	numGoroutines := 50
	numRequestsPerGoroutine := 10

	// Start multiple goroutines making requests
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numRequestsPerGoroutine; j++ {
				ctx := context.Background()
				resp, err := client.Get(ctx, server.URL)
				if err != nil {
					t.Errorf("Concurrent request failed: %v", err)
				} else {
					resp.Body.Close()
				}
			}
		}()
	}

	// Wait a bit for all requests to complete
	time.Sleep(2 * time.Second)

	metrics := client.GetMetrics()
	expectedRequests := int64(numGoroutines * numRequestsPerGoroutine)

	// Allow some tolerance for timing issues
	if metrics.TotalRequests < expectedRequests-10 {
		t.Errorf("Expected around %d requests, got %d", expectedRequests, metrics.TotalRequests)
	}
}