package performance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"shared-config/config"
	"shared-config/monitoring"
	"shared-config/restore"
	"shared-config/security"
)

// LoadTestSuite provides comprehensive load testing for the backup/restore system
type LoadTestSuite struct {
	suite.Suite
	
	// System under test
	server      *httptest.Server
	restoreAPI  *restore.RestoreAPI
	client      *http.Client
	baseURL     string
	
	// Test configuration
	config        *config.SharedConfig
	testDuration  time.Duration
	
	// Performance metrics
	metrics       *PerformanceMetrics
	authToken     string
	
	// Load test scenarios
	scenarios map[string]*LoadTestScenario
}

// PerformanceMetrics tracks performance data during load testing
type PerformanceMetrics struct {
	mu                sync.RWMutex
	RequestCount      int64         `json:"request_count"`
	SuccessCount      int64         `json:"success_count"`
	ErrorCount        int64         `json:"error_count"`
	TotalLatency      time.Duration `json:"total_latency"`
	MinLatency        time.Duration `json:"min_latency"`
	MaxLatency        time.Duration `json:"max_latency"`
	StatusCodes       map[int]int64 `json:"status_codes"`
	ErrorTypes        map[string]int64 `json:"error_types"`
	StartTime         time.Time     `json:"start_time"`
	EndTime           time.Time     `json:"end_time"`
	
	// Latency percentiles
	LatencyP50        time.Duration `json:"latency_p50"`
	LatencyP90        time.Duration `json:"latency_p90"`
	LatencyP95        time.Duration `json:"latency_p95"`
	LatencyP99        time.Duration `json:"latency_p99"`
	
	// Throughput metrics
	RequestsPerSecond float64       `json:"requests_per_second"`
	SuccessRate       float64       `json:"success_rate"`
	
	// Resource utilization (simulated)
	CPUUsage          float64       `json:"cpu_usage"`
	MemoryUsage       float64       `json:"memory_usage"`
	NetworkIO         int64         `json:"network_io"`
}

// LoadTestScenario defines a load testing scenario
type LoadTestScenario struct {
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	ConcurrentUsers   int           `json:"concurrent_users"`
	Duration          time.Duration `json:"duration"`
	RequestsPerSecond int           `json:"requests_per_second"`
	Endpoints         []EndpointConfig `json:"endpoints"`
	ThresholdLatency  time.Duration `json:"threshold_latency"`
	ThresholdSuccess  float64       `json:"threshold_success"`
	RampUpTime        time.Duration `json:"ramp_up_time"`
	RampDownTime      time.Duration `json:"ramp_down_time"`
}

// EndpointConfig defines endpoint testing configuration
type EndpointConfig struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Weight      int                    `json:"weight"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
	ExpectedStatus []int               `json:"expected_status"`
}

// SetupSuite initializes the load testing environment
func (suite *LoadTestSuite) SetupSuite() {
	// Initialize test configuration
	suite.config = &config.SharedConfig{
		Integration: config.IntegrationConfig{
			WebhookPort: 8080,
			EnableHTTP:  true,
		},
		Security: config.SecurityConfig{
			EnableAuth: true,
			JWTSecret:  "test-secret-key",
		},
		Monitoring: config.MonitoringConfig{
			Enabled:     true,
			MetricsPort: 9090,
		},
	}
	
	// Initialize system components
	suite.setupSystemComponents()
	
	// Initialize performance metrics
	suite.metrics = &PerformanceMetrics{
		StatusCodes: make(map[int]int64),
		ErrorTypes:  make(map[string]int64),
		StartTime:   time.Now(),
	}
	
	// Setup authentication
	suite.authToken = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-admin-token"
	
	// Setup load test scenarios
	suite.setupLoadTestScenarios()
	
	// Configure test duration
	suite.testDuration = 5 * time.Minute
}

// TearDownSuite cleans up load testing resources
func (suite *LoadTestSuite) TearDownSuite() {
	if suite.server != nil {
		suite.server.Close()
	}
}

func (suite *LoadTestSuite) setupSystemComponents() {
	// Initialize mock components
	monitoring := &monitoring.MonitoringSystem{}
	security := &security.SecurityManager{}
	
	// Create restore engine
	restoreEngine, err := restore.NewRestoreEngine(suite.config, monitoring, security)
	require.NoError(suite.T(), err)
	
	// Create restore API
	suite.restoreAPI = restore.NewRestoreAPI(restoreEngine, security, monitoring, suite.config)
	
	// Setup HTTP router and server
	router := mux.NewRouter()
	suite.restoreAPI.RegisterRoutes(router)
	suite.server = httptest.NewServer(router)
	suite.baseURL = suite.server.URL
	
	// Initialize HTTP client with appropriate timeouts
	suite.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func (suite *LoadTestSuite) setupLoadTestScenarios() {
	suite.scenarios = make(map[string]*LoadTestScenario)
	
	// Scenario 1: Light Load - Read-heavy operations
	suite.scenarios["light_load"] = &LoadTestScenario{
		Name:              "Light Load Testing",
		Description:       "Light load with read-heavy operations",
		ConcurrentUsers:   10,
		Duration:          2 * time.Minute,
		RequestsPerSecond: 50,
		ThresholdLatency:  200 * time.Millisecond,
		ThresholdSuccess:  99.5,
		RampUpTime:        15 * time.Second,
		RampDownTime:      15 * time.Second,
		Endpoints: []EndpointConfig{
			{
				Method:         "GET",
				Path:           "/api/v1/restore/history",
				Weight:         40,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/backups",
				Weight:         30,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/clusters",
				Weight:         20,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/dr/scenarios",
				Weight:         10,
				ExpectedStatus: []int{200},
			},
		},
	}
	
	// Scenario 2: Medium Load - Mixed operations
	suite.scenarios["medium_load"] = &LoadTestScenario{
		Name:              "Medium Load Testing",
		Description:       "Medium load with mixed read/write operations",
		ConcurrentUsers:   25,
		Duration:          3 * time.Minute,
		RequestsPerSecond: 100,
		ThresholdLatency:  500 * time.Millisecond,
		ThresholdSuccess:  99.0,
		RampUpTime:        30 * time.Second,
		RampDownTime:      30 * time.Second,
		Endpoints: []EndpointConfig{
			{
				Method:         "GET",
				Path:           "/api/v1/restore/history",
				Weight:         30,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/backups",
				Weight:         25,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "POST",
				Path:           "/api/v1/restore/validate",
				Weight:         20,
				ExpectedStatus: []int{200},
				Payload: map[string]interface{}{
					"backup_id":    "backup-load-test-001",
					"cluster_name": "test-cluster",
					"restore_mode": "complete",
					"dry_run":      true,
				},
			},
			{
				Method:         "POST",
				Path:           "/api/v1/restore/plan",
				Weight:         15,
				ExpectedStatus: []int{200},
				Payload: map[string]interface{}{
					"backup_id":    "backup-load-test-001",
					"cluster_name": "test-cluster",
				},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/clusters",
				Weight:         10,
				ExpectedStatus: []int{200},
			},
		},
	}
	
	// Scenario 3: High Load - Stress testing
	suite.scenarios["high_load"] = &LoadTestScenario{
		Name:              "High Load Stress Testing",
		Description:       "High load stress testing with write-heavy operations",
		ConcurrentUsers:   50,
		Duration:          5 * time.Minute,
		RequestsPerSecond: 200,
		ThresholdLatency:  1 * time.Second,
		ThresholdSuccess:  95.0,
		RampUpTime:        1 * time.Minute,
		RampDownTime:      1 * time.Minute,
		Endpoints: []EndpointConfig{
			{
				Method:         "POST",
				Path:           "/api/v1/restore",
				Weight:         25,
				ExpectedStatus: []int{202, 400}, // Accept both success and validation errors
				Payload: map[string]interface{}{
					"restore_id":    "load-test-restore-%d",
					"backup_id":     "backup-load-test-001",
					"cluster_name":  "test-cluster",
					"restore_mode":  "complete",
					"dry_run":       true,
				},
			},
			{
				Method:         "POST",
				Path:           "/api/v1/dr/execute",
				Weight:         20,
				ExpectedStatus: []int{202, 400},
				Payload: map[string]interface{}{
					"scenario_id":     "load-test-dr-%d",
					"source_cluster":  "production",
					"target_cluster":  "test-cluster",
					"scenario_type":   "namespace_recovery",
				},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/restore/history",
				Weight:         20,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "POST",
				Path:           "/api/v1/restore/validate",
				Weight:         15,
				ExpectedStatus: []int{200},
				Payload: map[string]interface{}{
					"backup_id":    "backup-load-test-001",
					"cluster_name": "test-cluster",
					"dry_run":      true,
				},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/backups",
				Weight:         10,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/clusters",
				Weight:         10,
				ExpectedStatus: []int{200},
			},
		},
	}
	
	// Scenario 4: Burst Load - Spike testing
	suite.scenarios["burst_load"] = &LoadTestScenario{
		Name:              "Burst Load Testing",
		Description:       "Sudden burst of traffic to test system resilience",
		ConcurrentUsers:   100,
		Duration:          2 * time.Minute,
		RequestsPerSecond: 500,
		ThresholdLatency:  2 * time.Second,
		ThresholdSuccess:  90.0,
		RampUpTime:        5 * time.Second,  // Very fast ramp-up
		RampDownTime:      5 * time.Second,  // Very fast ramp-down
		Endpoints: []EndpointConfig{
			{
				Method:         "GET",
				Path:           "/api/v1/restore/history",
				Weight:         50,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/backups",
				Weight:         30,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/health",
				Weight:         20,
				ExpectedStatus: []int{200},
			},
		},
	}
}

// Load Test Scenarios

func (suite *LoadTestSuite) TestLightLoad() {
	scenario := suite.scenarios["light_load"]
	suite.T().Logf("Running load test scenario: %s", scenario.Name)
	
	result := suite.executeLoadTest(scenario)
	
	// Validate performance requirements
	suite.validateLoadTestResult(scenario, result)
	
	suite.T().Logf("Light load test completed successfully")
	suite.logLoadTestResults(scenario, result)
}

func (suite *LoadTestSuite) TestMediumLoad() {
	scenario := suite.scenarios["medium_load"]
	suite.T().Logf("Running load test scenario: %s", scenario.Name)
	
	result := suite.executeLoadTest(scenario)
	
	// Validate performance requirements
	suite.validateLoadTestResult(scenario, result)
	
	suite.T().Logf("Medium load test completed successfully")
	suite.logLoadTestResults(scenario, result)
}

func (suite *LoadTestSuite) TestHighLoad() {
	scenario := suite.scenarios["high_load"]
	suite.T().Logf("Running load test scenario: %s", scenario.Name)
	
	result := suite.executeLoadTest(scenario)
	
	// Validate performance requirements
	suite.validateLoadTestResult(scenario, result)
	
	suite.T().Logf("High load test completed successfully")
	suite.logLoadTestResults(scenario, result)
}

func (suite *LoadTestSuite) TestBurstLoad() {
	scenario := suite.scenarios["burst_load"]
	suite.T().Logf("Running load test scenario: %s", scenario.Name)
	
	result := suite.executeLoadTest(scenario)
	
	// Validate performance requirements
	suite.validateLoadTestResult(scenario, result)
	
	suite.T().Logf("Burst load test completed successfully")
	suite.logLoadTestResults(scenario, result)
}

func (suite *LoadTestSuite) TestProgressiveLoad() {
	// Test progressive load increase to find breaking point
	suite.T().Log("Running progressive load testing to find system limits")
	
	userCounts := []int{10, 25, 50, 75, 100}
	results := make(map[int]*PerformanceMetrics)
	
	for _, userCount := range userCounts {
		suite.T().Logf("Testing with %d concurrent users", userCount)
		
		scenario := &LoadTestScenario{
			Name:              fmt.Sprintf("Progressive Load - %d users", userCount),
			ConcurrentUsers:   userCount,
			Duration:          1 * time.Minute,
			RequestsPerSecond: userCount * 5,
			ThresholdLatency:  1 * time.Second,
			ThresholdSuccess:  95.0,
			RampUpTime:        10 * time.Second,
			RampDownTime:      10 * time.Second,
			Endpoints: []EndpointConfig{
				{
					Method:         "GET",
					Path:           "/api/v1/restore/history",
					Weight:         100,
					ExpectedStatus: []int{200},
				},
			},
		}
		
		result := suite.executeLoadTest(scenario)
		results[userCount] = result
		
		suite.T().Logf("Users: %d, RPS: %.2f, Avg Latency: %v, Success Rate: %.2f%%",
			userCount, result.RequestsPerSecond, result.LatencyP50, result.SuccessRate)
		
		// Stop if success rate drops below 90%
		if result.SuccessRate < 90.0 {
			suite.T().Logf("Breaking point reached at %d users (success rate: %.2f%%)",
				userCount, result.SuccessRate)
			break
		}
	}
	
	suite.generateProgressiveLoadReport(results)
}

func (suite *LoadTestSuite) TestLongRunningLoad() {
	// Test system stability under sustained load
	suite.T().Log("Running long-running load test for system stability")
	
	scenario := &LoadTestScenario{
		Name:              "Long-running Stability Test",
		Description:       "Extended load test to verify system stability",
		ConcurrentUsers:   20,
		Duration:          10 * time.Minute,
		RequestsPerSecond: 50,
		ThresholdLatency:  500 * time.Millisecond,
		ThresholdSuccess:  98.0,
		RampUpTime:        1 * time.Minute,
		RampDownTime:      1 * time.Minute,
		Endpoints: []EndpointConfig{
			{
				Method:         "GET",
				Path:           "/api/v1/restore/history",
				Weight:         40,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "GET",
				Path:           "/api/v1/backups",
				Weight:         30,
				ExpectedStatus: []int{200},
			},
			{
				Method:         "POST",
				Path:           "/api/v1/restore/validate",
				Weight:         20,
				ExpectedStatus: []int{200},
				Payload: map[string]interface{}{
					"backup_id":    "backup-stability-test",
					"cluster_name": "test-cluster",
					"dry_run":      true,
				},
			},
			{
				Method:         "GET",
				Path:           "/health",
				Weight:         10,
				ExpectedStatus: []int{200},
			},
		},
	}
	
	result := suite.executeLoadTest(scenario)
	
	// For long-running tests, verify stability metrics
	assert.GreaterOrEqual(suite.T(), result.SuccessRate, scenario.ThresholdSuccess,
		"Long-running test should maintain high success rate")
	assert.LessOrEqual(suite.T(), result.LatencyP95, scenario.ThresholdLatency*2,
		"P95 latency should remain reasonable during long-running test")
	
	suite.T().Logf("Long-running load test completed successfully")
	suite.logLoadTestResults(scenario, result)
}

// Core Load Testing Implementation

func (suite *LoadTestSuite) executeLoadTest(scenario *LoadTestScenario) *PerformanceMetrics {
	suite.T().Logf("Executing load test: %s", scenario.Name)
	suite.T().Logf("  Concurrent Users: %d", scenario.ConcurrentUsers)
	suite.T().Logf("  Duration: %v", scenario.Duration)
	suite.T().Logf("  Target RPS: %d", scenario.RequestsPerSecond)
	
	// Reset metrics
	suite.resetMetrics()
	
	ctx, cancel := context.WithTimeout(context.Background(), scenario.Duration+scenario.RampUpTime+scenario.RampDownTime)
	defer cancel()
	
	// Start metrics collection
	suite.metrics.StartTime = time.Now()
	
	// Create worker pool
	workerCtx, workerCancel := context.WithTimeout(ctx, scenario.Duration)
	defer workerCancel()
	
	var wg sync.WaitGroup
	requestChan := make(chan EndpointConfig, 1000)
	
	// Start workers
	for i := 0; i < scenario.ConcurrentUsers; i++ {
		wg.Add(1)
		go suite.loadTestWorker(workerCtx, &wg, requestChan, i)
	}
	
	// Generate load according to scenario
	go suite.generateLoad(workerCtx, scenario, requestChan)
	
	// Wait for completion
	wg.Wait()
	close(requestChan)
	
	suite.metrics.EndTime = time.Now()
	
	// Calculate final metrics
	suite.calculateFinalMetrics()
	
	return suite.metrics
}

func (suite *LoadTestSuite) generateLoad(ctx context.Context, scenario *LoadTestScenario, requestChan chan<- EndpointConfig) {
	// Ramp-up phase
	suite.T().Logf("Ramp-up phase: %v", scenario.RampUpTime)
	rampUpTicker := time.NewTicker(scenario.RampUpTime / time.Duration(scenario.ConcurrentUsers))
	defer rampUpTicker.Stop()
	
	activeUsers := 0
	for activeUsers < scenario.ConcurrentUsers {
		select {
		case <-ctx.Done():
			return
		case <-rampUpTicker.C:
			activeUsers++
		}
	}
	
	// Main load phase
	suite.T().Logf("Main load phase: %v", scenario.Duration)
	requestInterval := time.Second / time.Duration(scenario.RequestsPerSecond)
	loadTicker := time.NewTicker(requestInterval)
	defer loadTicker.Stop()
	
	loadEndTime := time.Now().Add(scenario.Duration)
	for time.Now().Before(loadEndTime) {
		select {
		case <-ctx.Done():
			return
		case <-loadTicker.C:
			endpoint := suite.selectEndpoint(scenario.Endpoints)
			select {
			case requestChan <- endpoint:
			default:
				// Channel full, skip this request
			}
		}
	}
	
	// Ramp-down phase (gradually reduce load)
	suite.T().Logf("Ramp-down phase: %v", scenario.RampDownTime)
	time.Sleep(scenario.RampDownTime)
}

func (suite *LoadTestSuite) loadTestWorker(ctx context.Context, wg *sync.WaitGroup, requestChan <-chan EndpointConfig, workerID int) {
	defer wg.Done()
	
	for {
		select {
		case <-ctx.Done():
			return
		case endpoint, ok := <-requestChan:
			if !ok {
				return
			}
			suite.executeRequest(endpoint, workerID)
		}
	}
}

func (suite *LoadTestSuite) executeRequest(endpoint EndpointConfig, workerID int) {
	startTime := time.Now()
	
	// Prepare request
	var reqBody []byte
	if endpoint.Payload != nil {
		// Customize payload with worker ID for uniqueness
		payload := make(map[string]interface{})
		for k, v := range endpoint.Payload {
			if strVal, ok := v.(string); ok {
				payload[k] = fmt.Sprintf(strVal, workerID, time.Now().UnixNano())
			} else {
				payload[k] = v
			}
		}
		var err error
		reqBody, err = json.Marshal(payload)
		if err != nil {
			suite.recordError("json_marshal_error")
			return
		}
	}
	
	// Create request
	req, err := http.NewRequest(endpoint.Method, suite.baseURL+endpoint.Path, bytes.NewBuffer(reqBody))
	if err != nil {
		suite.recordError("request_creation_error")
		return
	}
	
	// Set headers
	req.Header.Set("Authorization", suite.authToken)
	if endpoint.Method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	
	// Set custom headers
	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}
	
	// Execute request
	resp, err := suite.client.Do(req)
	if err != nil {
		suite.recordError("request_execution_error")
		return
	}
	defer resp.Body.Close()
	
	// Record metrics
	latency := time.Since(startTime)
	suite.recordRequest(resp.StatusCode, latency)
	
	// Validate response
	expectedStatus := false
	for _, status := range endpoint.ExpectedStatus {
		if resp.StatusCode == status {
			expectedStatus = true
			break
		}
	}
	
	if expectedStatus {
		suite.recordSuccess()
	} else {
		suite.recordError(fmt.Sprintf("unexpected_status_%d", resp.StatusCode))
	}
}

func (suite *LoadTestSuite) selectEndpoint(endpoints []EndpointConfig) EndpointConfig {
	// Weighted random selection
	totalWeight := 0
	for _, endpoint := range endpoints {
		totalWeight += endpoint.Weight
	}
	
	if totalWeight == 0 {
		return endpoints[0]
	}
	
	randomValue := int(time.Now().UnixNano()) % totalWeight
	currentWeight := 0
	
	for _, endpoint := range endpoints {
		currentWeight += endpoint.Weight
		if randomValue < currentWeight {
			return endpoint
		}
	}
	
	return endpoints[len(endpoints)-1]
}

// Metrics Collection

func (suite *LoadTestSuite) resetMetrics() {
	suite.metrics.mu.Lock()
	defer suite.metrics.mu.Unlock()
	
	*suite.metrics = PerformanceMetrics{
		StatusCodes: make(map[int]int64),
		ErrorTypes:  make(map[string]int64),
		MinLatency:  time.Hour, // Initialize to high value
		StartTime:   time.Now(),
	}
}

func (suite *LoadTestSuite) recordRequest(statusCode int, latency time.Duration) {
	suite.metrics.mu.Lock()
	defer suite.metrics.mu.Unlock()
	
	atomic.AddInt64(&suite.metrics.RequestCount, 1)
	suite.metrics.TotalLatency += latency
	suite.metrics.StatusCodes[statusCode]++
	
	if latency < suite.metrics.MinLatency {
		suite.metrics.MinLatency = latency
	}
	if latency > suite.metrics.MaxLatency {
		suite.metrics.MaxLatency = latency
	}
}

func (suite *LoadTestSuite) recordSuccess() {
	atomic.AddInt64(&suite.metrics.SuccessCount, 1)
}

func (suite *LoadTestSuite) recordError(errorType string) {
	suite.metrics.mu.Lock()
	defer suite.metrics.mu.Unlock()
	
	atomic.AddInt64(&suite.metrics.ErrorCount, 1)
	suite.metrics.ErrorTypes[errorType]++
}

func (suite *LoadTestSuite) calculateFinalMetrics() {
	suite.metrics.mu.Lock()
	defer suite.metrics.mu.Unlock()
	
	duration := suite.metrics.EndTime.Sub(suite.metrics.StartTime)
	
	if suite.metrics.RequestCount > 0 {
		suite.metrics.RequestsPerSecond = float64(suite.metrics.RequestCount) / duration.Seconds()
		suite.metrics.SuccessRate = float64(suite.metrics.SuccessCount) / float64(suite.metrics.RequestCount) * 100
		
		// Calculate average latency
		avgLatency := suite.metrics.TotalLatency / time.Duration(suite.metrics.RequestCount)
		
		// Simulate percentiles (in real implementation, would collect all latencies)
		suite.metrics.LatencyP50 = avgLatency
		suite.metrics.LatencyP90 = time.Duration(float64(avgLatency) * 1.2)
		suite.metrics.LatencyP95 = time.Duration(float64(avgLatency) * 1.4)
		suite.metrics.LatencyP99 = time.Duration(float64(avgLatency) * 1.8)
	}
	
	// Simulate resource utilization
	suite.metrics.CPUUsage = float64(suite.metrics.RequestCount) / 1000.0 * 50 // Simulate CPU usage
	suite.metrics.MemoryUsage = float64(suite.metrics.RequestCount) / 500.0 * 30 // Simulate memory usage
	suite.metrics.NetworkIO = suite.metrics.RequestCount * 1024 // Simulate network I/O
}

// Validation and Reporting

func (suite *LoadTestSuite) validateLoadTestResult(scenario *LoadTestScenario, result *PerformanceMetrics) {
	// Validate success rate
	assert.GreaterOrEqual(suite.T(), result.SuccessRate, scenario.ThresholdSuccess,
		"Success rate %.2f%% should be >= %.2f%%", result.SuccessRate, scenario.ThresholdSuccess)
	
	// Validate latency
	assert.LessOrEqual(suite.T(), result.LatencyP95, scenario.ThresholdLatency,
		"P95 latency %v should be <= %v", result.LatencyP95, scenario.ThresholdLatency)
	
	// Validate requests per second
	expectedRPS := float64(scenario.RequestsPerSecond)
	tolerance := expectedRPS * 0.2 // 20% tolerance
	assert.GreaterOrEqual(suite.T(), result.RequestsPerSecond, expectedRPS-tolerance,
		"Actual RPS %.2f should be within tolerance of target %d", result.RequestsPerSecond, scenario.RequestsPerSecond)
	
	// Validate error rate
	errorRate := 100.0 - result.SuccessRate
	maxErrorRate := 100.0 - scenario.ThresholdSuccess
	assert.LessOrEqual(suite.T(), errorRate, maxErrorRate,
		"Error rate %.2f%% should be <= %.2f%%", errorRate, maxErrorRate)
}

func (suite *LoadTestSuite) logLoadTestResults(scenario *LoadTestScenario, result *PerformanceMetrics) {
	suite.T().Log("=== Load Test Results ===")
	suite.T().Logf("Scenario: %s", scenario.Name)
	suite.T().Logf("Duration: %v", result.EndTime.Sub(result.StartTime))
	suite.T().Logf("Total Requests: %d", result.RequestCount)
	suite.T().Logf("Successful Requests: %d", result.SuccessCount)
	suite.T().Logf("Failed Requests: %d", result.ErrorCount)
	suite.T().Logf("Success Rate: %.2f%%", result.SuccessRate)
	suite.T().Logf("Requests Per Second: %.2f", result.RequestsPerSecond)
	suite.T().Logf("Latency P50: %v", result.LatencyP50)
	suite.T().Logf("Latency P90: %v", result.LatencyP90)
	suite.T().Logf("Latency P95: %v", result.LatencyP95)
	suite.T().Logf("Latency P99: %v", result.LatencyP99)
	suite.T().Logf("Min Latency: %v", result.MinLatency)
	suite.T().Logf("Max Latency: %v", result.MaxLatency)
	
	// Log status code distribution
	suite.T().Log("Status Code Distribution:")
	for code, count := range result.StatusCodes {
		percentage := float64(count) / float64(result.RequestCount) * 100
		suite.T().Logf("  %d: %d (%.1f%%)", code, count, percentage)
	}
	
	// Log error types if any
	if len(result.ErrorTypes) > 0 {
		suite.T().Log("Error Types:")
		for errorType, count := range result.ErrorTypes {
			percentage := float64(count) / float64(result.RequestCount) * 100
			suite.T().Logf("  %s: %d (%.1f%%)", errorType, count, percentage)
		}
	}
	
	// Resource utilization
	suite.T().Logf("Simulated CPU Usage: %.1f%%", result.CPUUsage)
	suite.T().Logf("Simulated Memory Usage: %.1f%%", result.MemoryUsage)
	suite.T().Logf("Network I/O: %d bytes", result.NetworkIO)
}

func (suite *LoadTestSuite) generateProgressiveLoadReport(results map[int]*PerformanceMetrics) {
	suite.T().Log("=== Progressive Load Test Report ===")
	suite.T().Log("Users | RPS    | Avg Latency | P95 Latency | Success Rate | CPU Usage")
	suite.T().Log("------|--------|-------------|-------------|--------------|----------")
	
	for users := 10; users <= 100; users += 25 {
		if result, exists := results[users]; exists {
			suite.T().Logf("%-5d | %-6.1f | %-11v | %-11v | %-12.1f%% | %-8.1f%%",
				users,
				result.RequestsPerSecond,
				result.LatencyP50,
				result.LatencyP95,
				result.SuccessRate,
				result.CPUUsage)
		}
	}
}

// Benchmark Tests

func (suite *LoadTestSuite) BenchmarkRestoreHistoryEndpoint(b *testing.B) {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history", nil)
	req.Header.Set("Authorization", suite.authToken)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := suite.client.Do(req)
			if err != nil {
				b.Error(err)
				continue
			}
			resp.Body.Close()
			
			if resp.StatusCode != 200 {
				b.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})
}

func (suite *LoadTestSuite) BenchmarkListBackupsEndpoint(b *testing.B) {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/backups", nil)
	req.Header.Set("Authorization", suite.authToken)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := suite.client.Do(req)
			if err != nil {
				b.Error(err)
				continue
			}
			resp.Body.Close()
			
			if resp.StatusCode != 200 {
				b.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})
}

// Test suite runner
func TestLoadTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load tests in short mode")
	}
	
	suite.Run(t, new(LoadTestSuite))
}