package main

import (
	"context"
	"fmt"
	"log"
	"time"

	sharedconfig "shared-config/config"
	httplib "shared-config/http"
)

func main() {
	// Example 1: Basic HTTP client usage
	fmt.Println("=== Basic HTTP Client Usage ===")
	basicHTTPClientExample()

	// Example 2: HTTP client manager with different profiles
	fmt.Println("\n=== HTTP Client Manager Usage ===")
	httpClientManagerExample()

	// Example 3: Integration with shared configuration
	fmt.Println("\n=== Shared Configuration Integration ===")
	sharedConfigurationExample()

	// Example 4: Metrics and monitoring
	fmt.Println("\n=== Metrics and Monitoring ===")
	metricsExample()
}

func basicHTTPClientExample() {
	// Create HTTP client with default configuration
	client := httplib.NewHTTPClient(httplib.DefaultHTTPClientConfig())
	defer client.Close()

	ctx := context.Background()

	// Make a GET request
	resp, err := client.Get(ctx, "https://httpbin.org/get")
	if err != nil {
		log.Printf("GET request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("GET Response: %d %s\n", resp.StatusCode, resp.Status)

	// Make a POST request with JSON payload
	payload := map[string]interface{}{
		"message": "Hello from optimized HTTP client",
		"timestamp": time.Now().Unix(),
	}

	resp, err = client.PostJSON(ctx, "https://httpbin.org/post", payload)
	if err != nil {
		log.Printf("POST request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("POST Response: %d %s\n", resp.StatusCode, resp.Status)

	// Get client metrics
	metrics := client.GetMetrics()
	fmt.Printf("Client Metrics: %d requests, %d successful, avg response time: %v\n",
		metrics.TotalRequests, metrics.SuccessfulReqs, metrics.AvgResponseTime)
}

func httpClientManagerExample() {
	// Create HTTP client manager
	config := &sharedconfig.SharedConfig{} // Use default config
	manager := httplib.NewHTTPClientManager(config)
	defer manager.Close()

	ctx := context.Background()

	// Use webhook client for webhook calls
	webhookClient := manager.GetWebhookClient()
	resp, err := webhookClient.Get(ctx, "https://httpbin.org/delay/1")
	if err != nil {
		log.Printf("Webhook request failed: %v", err)
	} else {
		resp.Body.Close()
		fmt.Printf("Webhook client request: %d %s\n", resp.StatusCode, resp.Status)
	}

	// Use storage client for large responses
	storageClient := manager.GetStorageClient()
	resp, err = storageClient.Get(ctx, "https://httpbin.org/bytes/1024")
	if err != nil {
		log.Printf("Storage request failed: %v", err)
	} else {
		resp.Body.Close()
		fmt.Printf("Storage client request: %d %s\n", resp.StatusCode, resp.Status)
	}

	// Use API client for general purpose
	apiClient := manager.GetAPIClient()
	resp, err = apiClient.Get(ctx, "https://httpbin.org/json")
	if err != nil {
		log.Printf("API request failed: %v", err)
	} else {
		resp.Body.Close()
		fmt.Printf("API client request: %d %s\n", resp.StatusCode, resp.Status)
	}

	// Use monitoring client for health checks
	monitoringClient := manager.GetMonitoringClient()
	resp, err = monitoringClient.Get(ctx, "https://httpbin.org/status/200")
	if err != nil {
		log.Printf("Monitoring request failed: %v", err)
	} else {
		resp.Body.Close()
		fmt.Printf("Monitoring client request: %d %s\n", resp.StatusCode, resp.Status)
	}

	// Get pool metrics
	poolMetrics := manager.GetMetrics()
	fmt.Printf("Pool Metrics: %d clients, %d total requests\n",
		poolMetrics.TotalClients, poolMetrics.AggregatedMetrics.TotalRequests)
}

func sharedConfigurationExample() {
	// Create shared configuration with HTTP settings
	config := &sharedconfig.SharedConfig{
		Performance: sharedconfig.PerformanceConfig{
			Limits: sharedconfig.LimitsConfig{
				MaxConcurrentOperations: 50,
			},
			Optimization: sharedconfig.OptimizationConfig{
				Compression: true,
			},
		},
		Pipeline: sharedconfig.PipelineConfig{
			ErrorHandling: sharedconfig.ErrorHandlingConfig{
				MaxRetries: 5,
				RetryDelay: 2 * time.Second,
			},
		},
	}

	// Create HTTP client with production profile
	client := httplib.NewHTTPClientFromConfig(config, "production")
	defer client.Close()

	ctx := context.Background()

	// Make requests that will use the configured settings
	for i := 0; i < 5; i++ {
		resp, err := client.Get(ctx, "https://httpbin.org/delay/0.1")
		if err != nil {
			log.Printf("Request %d failed: %v", i+1, err)
		} else {
			resp.Body.Close()
			fmt.Printf("Request %d: %d %s\n", i+1, resp.StatusCode, resp.Status)
		}
	}

	// Show how configuration affected the client
	clientConfig := client.GetConfig()
	fmt.Printf("Client configured with MaxRetries: %d, Compression: %t\n",
		clientConfig.MaxRetries, clientConfig.CompressionEnabled)
}

func metricsExample() {
	// Create client manager
	manager := httplib.NewHTTPClientManager(&sharedconfig.SharedConfig{})
	defer manager.Close()

	ctx := context.Background()

	// Make some requests to generate metrics
	clients := []string{"webhook", "storage", "api", "monitoring"}
	for _, clientType := range clients {
		client, _ := manager.GetClient(clientType)
		for i := 0; i < 3; i++ {
			resp, err := client.Get(ctx, "https://httpbin.org/delay/0.1")
			if resp != nil {
				resp.Body.Close()
			}
			if err != nil {
				log.Printf("Request failed: %v", err)
			}
		}
	}

	// Get detailed connection statistics
	stats := manager.GetConnectionStats()
	fmt.Printf("Connection Statistics:\n")
	fmt.Printf("  Total Clients: %v\n", stats["total_clients"])
	fmt.Printf("  Total Requests: %v\n", stats["total_requests"])
	fmt.Printf("  Success Rate: %.2f%%\n", stats["success_rate"])
	fmt.Printf("  Average Response Time: %v ms\n", stats["avg_response_time"])
	fmt.Printf("  Retry Rate: %.2f%%\n", stats["retry_rate"])

	// Get per-client metrics
	if clientStats, ok := stats["clients"].(map[string]map[string]interface{}); ok {
		fmt.Printf("\nPer-Client Statistics:\n")
		for clientType, metrics := range clientStats {
			fmt.Printf("  %s:\n", clientType)
			fmt.Printf("    Requests: %v\n", metrics["requests"])
			fmt.Printf("    Success Rate: %.2f%%\n", metrics["success_rate"])
			fmt.Printf("    Avg Response Time: %v ms\n", metrics["avg_response_time"])
		}
	}

	// Perform health check
	healthResults := manager.HealthCheck(ctx)
	fmt.Printf("\nHealth Check Results:\n")
	for profile, err := range healthResults {
		if err != nil {
			fmt.Printf("  %s: UNHEALTHY (%v)\n", profile, err)
		} else {
			fmt.Printf("  %s: HEALTHY\n", profile)
		}
	}

	// Reset metrics if needed
	manager.ResetMetrics()
	fmt.Printf("\nMetrics reset successfully\n")
}

// Example of custom HTTP client configuration
func customHTTPClientExample() {
	// Create custom configuration for specific use case
	config := &httplib.HTTPClientConfig{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     25,
		IdleConnTimeout:     60 * time.Second,
		KeepAlive:           15 * time.Second,
		DialTimeout:         5 * time.Second,
		RequestTimeout:      15 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		CompressionEnabled:  true,
		UserAgent:           "my-custom-client/1.0",
		MaxResponseSize:     5 * 1024 * 1024, // 5MB
		MaxRetries:          2,
		RetryDelay:          500 * time.Millisecond,
		BackoffFactor:       1.5,
		JitterEnabled:       true,
		RetryableErrors: []string{
			"connection refused",
			"timeout",
			"network unreachable",
		},
		CircuitBreakerEnabled:   true,
		FailureThreshold:        3,
		CircuitBreakerResetTime: 30 * time.Second,
	}

	client := httplib.NewHTTPClient(config)
	defer client.Close()

	fmt.Printf("Custom client created with %d max connections per host\n", config.MaxConnsPerHost)

	// Use the custom client
	ctx := context.Background()
	resp, err := client.Get(ctx, "https://httpbin.org/get")
	if err != nil {
		log.Printf("Custom client request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Custom client response: %d %s\n", resp.StatusCode, resp.Status)
}