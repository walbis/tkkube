package http

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	sharedconfig "shared-config/config"
)

// HTTPClientConfig defines HTTP client configuration
type HTTPClientConfig struct {
	// Connection pooling
	MaxIdleConns        int           `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost     int           `yaml:"max_conns_per_host"`
	IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout"`
	KeepAlive           time.Duration `yaml:"keep_alive"`

	// Timeouts
	DialTimeout         time.Duration `yaml:"dial_timeout"`
	RequestTimeout      time.Duration `yaml:"request_timeout"`
	ResponseTimeout     time.Duration `yaml:"response_timeout"`
	TLSHandshakeTimeout time.Duration `yaml:"tls_handshake_timeout"`

	// Performance options
	CompressionEnabled bool   `yaml:"compression_enabled"`
	UserAgent          string `yaml:"user_agent"`
	MaxResponseSize    int64  `yaml:"max_response_size"`

	// Retry configuration
	MaxRetries      int           `yaml:"max_retries"`
	RetryDelay      time.Duration `yaml:"retry_delay"`
	BackoffFactor   float64       `yaml:"backoff_factor"`
	JitterEnabled   bool          `yaml:"jitter_enabled"`
	RetryableErrors []string      `yaml:"retryable_errors"`

	// Circuit breaker
	CircuitBreakerEnabled   bool          `yaml:"circuit_breaker_enabled"`
	FailureThreshold        int           `yaml:"failure_threshold"`
	CircuitBreakerTimeout   time.Duration `yaml:"circuit_breaker_timeout"`
	CircuitBreakerResetTime time.Duration `yaml:"circuit_breaker_reset_time"`

	// TLS configuration
	TLSConfig *tls.Config `yaml:"-"`
}

// DefaultHTTPClientConfig returns default HTTP client configuration
func DefaultHTTPClientConfig() *HTTPClientConfig {
	return &HTTPClientConfig{
		// Connection pooling defaults
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,
		KeepAlive:           30 * time.Second,

		// Timeout defaults
		DialTimeout:         10 * time.Second,
		RequestTimeout:      30 * time.Second,
		ResponseTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,

		// Performance defaults
		CompressionEnabled: true,
		UserAgent:          "shared-config-http-client/1.0",
		MaxResponseSize:    10 * 1024 * 1024, // 10MB

		// Retry defaults
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		BackoffFactor: 2.0,
		JitterEnabled: true,
		RetryableErrors: []string{
			"connection refused",
			"connection reset",
			"timeout",
			"temporary failure",
			"network unreachable",
			"no route to host",
			"service unavailable",
			"gateway timeout",
			"too many requests",
		},

		// Circuit breaker defaults
		CircuitBreakerEnabled:   true,
		FailureThreshold:        5,
		CircuitBreakerTimeout:   30 * time.Second,
		CircuitBreakerResetTime: 60 * time.Second,
	}
}

// HTTPClient represents an optimized HTTP client with connection pooling
type HTTPClient struct {
	client         *http.Client
	config         *HTTPClientConfig
	metrics        *HTTPMetrics
	circuitBreaker *CircuitBreaker
	mu             sync.RWMutex
}

// HTTPMetrics tracks HTTP client performance metrics
type HTTPMetrics struct {
	// Request metrics
	TotalRequests     int64         `json:"total_requests"`
	SuccessfulReqs    int64         `json:"successful_requests"`
	FailedReqs        int64         `json:"failed_requests"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
	TotalResponseTime time.Duration `json:"total_response_time"`

	// Connection pool metrics
	ActiveConns     int `json:"active_connections"`
	IdleConns       int `json:"idle_connections"`
	ConnectionReuse int `json:"connection_reuse_count"`

	// Error metrics
	TimeoutErrors     int64 `json:"timeout_errors"`
	ConnectionErrors  int64 `json:"connection_errors"`
	CircuitBreakerHits int64 `json:"circuit_breaker_hits"`
	RetryAttempts     int64 `json:"retry_attempts"`

	// Response size metrics
	TotalBytesRead    int64 `json:"total_bytes_read"`
	TotalBytesWritten int64 `json:"total_bytes_written"`

	mu sync.RWMutex
}

// CircuitBreakerState represents circuit breaker state
type CircuitBreakerState int

const (
	Closed CircuitBreakerState = iota
	Open
	HalfOpen
)

// CircuitBreaker implements circuit breaker pattern for HTTP requests
type CircuitBreaker struct {
	config           *HTTPClientConfig
	state            CircuitBreakerState
	failureCount     int64
	lastFailureTime  time.Time
	lastSuccessTime  time.Time
	mu               sync.RWMutex
}

// NewHTTPClient creates a new optimized HTTP client
func NewHTTPClient(config *HTTPClientConfig) *HTTPClient {
	if config == nil {
		config = DefaultHTTPClientConfig()
	}

	// Create custom transport with optimized settings
	transport := &http.Transport{
		// Connection pooling settings
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,

		// Dial settings
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: config.KeepAlive,
		}).DialContext,

		// TLS settings
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,
		TLSClientConfig:     config.TLSConfig,

		// Performance optimizations
		ForceAttemptHTTP2:     true,
		DisableKeepAlives:     false,
		DisableCompression:    !config.CompressionEnabled,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Create HTTP client
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.RequestTimeout,
	}

	client := &HTTPClient{
		client:  httpClient,
		config:  config,
		metrics: &HTTPMetrics{},
	}

	// Initialize circuit breaker if enabled
	if config.CircuitBreakerEnabled {
		client.circuitBreaker = &CircuitBreaker{
			config: config,
			state:  Closed,
		}
	}

	return client
}

// NewHTTPClientFromConfig creates HTTP client from shared configuration
func NewHTTPClientFromConfig(sharedConfig *sharedconfig.SharedConfig, profile string) *HTTPClient {
	config := DefaultHTTPClientConfig()

	// Apply configuration overrides based on profile
	switch profile {
	case "development":
		config.RequestTimeout = 10 * time.Second
		config.MaxRetries = 1
		config.CircuitBreakerEnabled = false
	case "production":
		config.RequestTimeout = 30 * time.Second
		config.MaxRetries = 5
		config.CircuitBreakerEnabled = true
		config.MaxIdleConnsPerHost = 50
		config.MaxConnsPerHost = 100
	case "testing":
		config.RequestTimeout = 5 * time.Second
		config.MaxRetries = 0
		config.CircuitBreakerEnabled = false
		config.CompressionEnabled = false
	default:
		// Use defaults
	}

	// Override with any specific HTTP settings from shared config
	if sharedConfig != nil {
		// Apply storage connection settings
		if sharedConfig.Storage.Connection.Timeout > 0 {
			config.RequestTimeout = time.Duration(sharedConfig.Storage.Connection.Timeout) * time.Second
		}
		if sharedConfig.Storage.Connection.MaxRetries > 0 {
			config.MaxRetries = sharedConfig.Storage.Connection.MaxRetries
		}
		if sharedConfig.Storage.Connection.RetryDelay > 0 {
			config.RetryDelay = sharedConfig.Storage.Connection.RetryDelay
		}

		// Apply pipeline error handling settings
		if sharedConfig.Pipeline.ErrorHandling.MaxRetries > 0 {
			config.MaxRetries = sharedConfig.Pipeline.ErrorHandling.MaxRetries
		}
		if sharedConfig.Pipeline.ErrorHandling.RetryDelay > 0 {
			config.RetryDelay = sharedConfig.Pipeline.ErrorHandling.RetryDelay
		}

		// Apply performance settings
		if sharedConfig.Performance.Limits.MaxConcurrentOperations > 0 {
			config.MaxConnsPerHost = sharedConfig.Performance.Limits.MaxConcurrentOperations
		}
		if sharedConfig.Performance.Optimization.Compression {
			config.CompressionEnabled = true
		}

		// Apply security settings
		if !sharedConfig.Security.Network.VerifySSL {
			config.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		}
	}

	return NewHTTPClient(config)
}

// Do performs HTTP request with retry logic and circuit breaker
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if c.circuitBreaker != nil {
		if !c.circuitBreaker.Allow() {
			c.recordCircuitBreakerHit()
			return nil, fmt.Errorf("circuit breaker is open")
		}
	}

	return c.doWithRetry(ctx, req)
}

// doWithRetry performs HTTP request with retry logic
func (c *HTTPClient) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay
			delay := c.calculateBackoffDelay(attempt)
			c.recordRetryAttempt()

			// Wait with context cancellation support
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Clone request for retry safety
		reqClone := c.cloneRequest(ctx, req)

		// Perform the request
		resp, err := c.doSingleRequest(reqClone)
		
		if err == nil && c.isSuccessStatusCode(resp.StatusCode) {
			// Success - record metrics and return
			duration := time.Since(startTime)
			c.recordSuccess(duration)
			
			if c.circuitBreaker != nil {
				c.circuitBreaker.RecordSuccess()
			}
			
			return resp, nil
		}

		lastErr = err
		if resp != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		}

		// Check if error is retryable
		// If we have a response, check status code; otherwise check error pattern
		if resp != nil {
			if !c.isRetryableStatusCode(resp) {
				break
			}
		} else if !c.isRetryableError(lastErr) {
			break
		}
	}

	// All retries exhausted - record failure
	duration := time.Since(startTime)
	c.recordFailure(duration, lastErr)
	
	if c.circuitBreaker != nil {
		c.circuitBreaker.RecordFailure()
	}

	return nil, fmt.Errorf("request failed after %d retries: %v", c.config.MaxRetries, lastErr)
}

// doSingleRequest performs a single HTTP request with optimizations
func (c *HTTPClient) doSingleRequest(req *http.Request) (*http.Response, error) {
	// Add compression headers if enabled
	if c.config.CompressionEnabled {
		req.Header.Set("Accept-Encoding", "gzip, deflate")
	}

	// Set User-Agent if not already set
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}

	// Perform request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle response compression
	if c.config.CompressionEnabled {
		resp = c.handleCompression(resp)
	}

	// Limit response size if configured
	if c.config.MaxResponseSize > 0 {
		resp.Body = &limitedReader{
			ReadCloser: resp.Body,
			limit:      c.config.MaxResponseSize,
		}
	}

	return resp, nil
}

// Get performs HTTP GET request
func (c *HTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// Post performs HTTP POST request
func (c *HTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(ctx, req)
}

// PostJSON performs HTTP POST request with JSON payload
func (c *HTTPClient) PostJSON(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}

	return c.Post(ctx, url, "application/json", strings.NewReader(string(jsonData)))
}

// GetMetrics returns current HTTP client metrics
func (c *HTTPClient) GetMetrics() HTTPMetrics {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	// Update connection pool metrics from transport
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		// Note: Go's http.Transport doesn't expose detailed pool metrics
		// These would need to be tracked manually or via reflection
		_ = transport
	}

	return *c.metrics
}

// GetConfig returns the HTTP client configuration
func (c *HTTPClient) GetConfig() *HTTPClientConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// ResetMetrics resets all metrics counters
func (c *HTTPClient) ResetMetrics() {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.TotalRequests = 0
	c.metrics.SuccessfulReqs = 0
	c.metrics.FailedReqs = 0
	c.metrics.AvgResponseTime = 0
	c.metrics.TotalResponseTime = 0
	c.metrics.TimeoutErrors = 0
	c.metrics.ConnectionErrors = 0
	c.metrics.CircuitBreakerHits = 0
	c.metrics.RetryAttempts = 0
	c.metrics.TotalBytesRead = 0
	c.metrics.TotalBytesWritten = 0
}

// Close cleanly shuts down the HTTP client
func (c *HTTPClient) Close() error {
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}

// Helper methods

func (c *HTTPClient) calculateBackoffDelay(attempt int) time.Duration {
	delay := float64(c.config.RetryDelay) * 
		math.Pow(c.config.BackoffFactor, float64(attempt-1))

	if c.config.JitterEnabled {
		// Add jitter (±25%)
		jitter := delay * 0.25 * (2*rand.Float64() - 1)
		delay += jitter
	}

	return time.Duration(delay)
}

func (c *HTTPClient) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	for _, pattern := range c.config.RetryableErrors {
		if strings.Contains(errorStr, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func (c *HTTPClient) isRetryableStatusCode(resp *http.Response) bool {
	if resp == nil {
		return true
	}

	retryableCodes := []int{408, 429, 500, 502, 503, 504}
	for _, code := range retryableCodes {
		if resp.StatusCode == code {
			return true
		}
	}
	return false
}

func (c *HTTPClient) isSuccessStatusCode(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func (c *HTTPClient) cloneRequest(ctx context.Context, req *http.Request) *http.Request {
	clone := req.Clone(ctx)
	if req.Body != nil {
		// For retries, we need to handle body cloning
		// This is a simplified approach - production code might need more sophisticated handling
		if req.GetBody != nil {
			body, _ := req.GetBody()
			clone.Body = body
		}
	}
	return clone
}

func (c *HTTPClient) handleCompression(resp *http.Response) *http.Response {
	encoding := resp.Header.Get("Content-Encoding")
	if encoding == "gzip" {
		if gzipReader, err := gzip.NewReader(resp.Body); err == nil {
			resp.Body = gzipReader
			resp.Header.Del("Content-Encoding")
		}
	}
	return resp
}

// Metrics recording methods

func (c *HTTPClient) recordSuccess(duration time.Duration) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.TotalRequests++
	c.metrics.SuccessfulReqs++
	c.metrics.TotalResponseTime += duration
	c.metrics.AvgResponseTime = c.metrics.TotalResponseTime / time.Duration(c.metrics.TotalRequests)
}

func (c *HTTPClient) recordFailure(duration time.Duration, err error) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.TotalRequests++
	c.metrics.FailedReqs++
	c.metrics.TotalResponseTime += duration
	c.metrics.AvgResponseTime = c.metrics.TotalResponseTime / time.Duration(c.metrics.TotalRequests)

	if err != nil {
		errorStr := strings.ToLower(err.Error())
		if strings.Contains(errorStr, "timeout") {
			c.metrics.TimeoutErrors++
		} else if strings.Contains(errorStr, "connection") {
			c.metrics.ConnectionErrors++
		}
	}
}

func (c *HTTPClient) recordRetryAttempt() {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()
	c.metrics.RetryAttempts++
}

func (c *HTTPClient) recordCircuitBreakerHit() {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()
	c.metrics.CircuitBreakerHits++
}

// Circuit breaker methods

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case Closed:
		return true
	case Open:
		return time.Since(cb.lastFailureTime) > cb.config.CircuitBreakerResetTime
	case HalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastSuccessTime = time.Now()
	if cb.state == HalfOpen {
		cb.state = Closed
		cb.failureCount = 0
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= int64(cb.config.FailureThreshold) {
		cb.state = Open
	}
}

// limitedReader limits the amount of data read from a response
type limitedReader struct {
	io.ReadCloser
	limit int64
	read  int64
}

func (lr *limitedReader) Read(p []byte) (n int, err error) {
	if lr.read >= lr.limit {
		return 0, fmt.Errorf("response size limit exceeded")
	}

	if int64(len(p)) > lr.limit-lr.read {
		p = p[:lr.limit-lr.read]
	}

	n, err = lr.ReadCloser.Read(p)
	lr.read += int64(n)
	return n, err
}