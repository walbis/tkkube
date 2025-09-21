package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// RestoreAPITestSuite provides comprehensive testing for the Restore API
type RestoreAPITestSuite struct {
	suite.Suite
	server      *httptest.Server
	restoreAPI  *restore.RestoreAPI
	client      *http.Client
	baseURL     string
	testData    *TestData
	authHeaders map[string]string
}

// TestData contains test fixtures and mock data
type TestData struct {
	ValidRestoreRequest   restore.RestoreAPIRequest
	InvalidRestoreRequest restore.RestoreAPIRequest
	ValidDRRequest        restore.DisasterRecoveryRequest
	AdminToken            string
	OperatorToken         string
	ReadOnlyToken         string
	ExpiredToken          string
}

// SetupSuite initializes the test suite with mock services and test data
func (suite *RestoreAPITestSuite) SetupSuite() {
	// Initialize test configuration
	config := &config.SharedConfig{
		Integration: config.IntegrationConfig{
			WebhookPort: 8080,
		},
		Security: config.SecurityConfig{
			EnableAuth: true,
			JWTSecret:  "test-secret-key",
		},
	}

	// Initialize mock monitoring and security
	monitoring := &monitoring.MonitoringSystem{}
	security := &security.SecurityManager{}

	// Create mock restore engine
	restoreEngine, err := restore.NewRestoreEngine(config, monitoring, security)
	require.NoError(suite.T(), err)

	// Create restore API
	suite.restoreAPI = restore.NewRestoreAPI(restoreEngine, security, monitoring, config)

	// Setup HTTP router and server
	router := mux.NewRouter()
	suite.restoreAPI.RegisterRoutes(router)
	suite.server = httptest.NewServer(router)
	suite.baseURL = suite.server.URL

	// Initialize HTTP client
	suite.client = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Setup test data
	suite.setupTestData()
}

// TearDownSuite cleans up test resources
func (suite *RestoreAPITestSuite) TearDownSuite() {
	if suite.server != nil {
		suite.server.Close()
	}
}

// SetupTest runs before each test
func (suite *RestoreAPITestSuite) SetupTest() {
	// Reset any state between tests if needed
}

// setupTestData initializes test fixtures
func (suite *RestoreAPITestSuite) setupTestData() {
	suite.testData = &TestData{
		ValidRestoreRequest: restore.RestoreAPIRequest{
			RestoreID:        "test-restore-001",
			BackupID:         "backup-2024-01-15-001",
			ClusterName:      "test-cluster",
			TargetNamespaces: []string{"test-app"},
			RestoreMode:      restore.RestoreModeComplete,
			ValidationMode:   restore.ValidationModeStrict,
			ConflictStrategy: restore.ConflictStrategyOverwrite,
			DryRun:           false,
			Configuration: map[string]interface{}{
				"timeout": 1800,
			},
			Metadata: map[string]interface{}{
				"user":   "test-user",
				"reason": "integration-test",
			},
		},
		InvalidRestoreRequest: restore.RestoreAPIRequest{
			// Missing required fields for validation testing
			RestoreID:   "",
			BackupID:    "",
			ClusterName: "",
		},
		ValidDRRequest: restore.DisasterRecoveryRequest{
			ScenarioID:      "dr-test-001",
			SourceCluster:   "production",
			TargetCluster:   "production-recovery",
			BackupID:        "backup-2024-01-15-large",
			ScenarioType:    "cluster_rebuild",
			AutomationLevel: "assisted",
			ValidationLevel: "strict",
		},
		AdminToken:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-admin-token",
		OperatorToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-operator-token",
		ReadOnlyToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-readonly-token",
		ExpiredToken:  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.expired-token",
	}

	suite.authHeaders = map[string]string{
		"admin":     fmt.Sprintf("Bearer %s", suite.testData.AdminToken),
		"operator":  fmt.Sprintf("Bearer %s", suite.testData.OperatorToken),
		"readonly":  fmt.Sprintf("Bearer %s", suite.testData.ReadOnlyToken),
		"expired":   fmt.Sprintf("Bearer %s", suite.testData.ExpiredToken),
		"invalid":   "Bearer invalid-token",
		"malformed": "InvalidFormat token",
	}
}

// Test Cases for Restore Operations

func (suite *RestoreAPITestSuite) TestStartRestore_Success() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["admin"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusAccepted, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.Contains(suite.T(), response.Message, "started successfully")
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestStartRestore_ValidationError() {
	body, _ := json.Marshal(suite.testData.InvalidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["admin"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.False(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Error)
	assert.Equal(suite.T(), "validation_error", response.Error.Code)
}

func (suite *RestoreAPITestSuite) TestStartRestore_AuthenticationError() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["invalid"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

func (suite *RestoreAPITestSuite) TestStartRestore_InsufficientPermissions() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

func (suite *RestoreAPITestSuite) TestGetRestoreStatus_Success() {
	// First start a restore
	suite.TestStartRestore_Success()

	// Then get its status
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/test-restore-001", nil)
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestGetRestoreStatus_NotFound() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/nonexistent-restore", nil)
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.False(suite.T(), response.Success)
	assert.Equal(suite.T(), "not_found", response.Error.Code)
}

func (suite *RestoreAPITestSuite) TestCancelRestore_Success() {
	// First start a restore
	suite.TestStartRestore_Success()

	// Then cancel it
	req, _ := http.NewRequest("DELETE", suite.baseURL+"/api/v1/restore/test-restore-001", nil)
	req.Header.Set("Authorization", suite.authHeaders["admin"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.Contains(suite.T(), response.Message, "cancelled successfully")
}

func (suite *RestoreAPITestSuite) TestListActiveRestores_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore", nil)
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestGetRestoreHistory_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestGetRestoreHistory_WithLimit() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history?limit=10", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
}

func (suite *RestoreAPITestSuite) TestValidateRestore_Success() {
	// Set DryRun to true for validation
	validateRequest := suite.testData.ValidRestoreRequest
	validateRequest.DryRun = true

	body, _ := json.Marshal(validateRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestCreateRestorePlan_Success() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore/plan", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)

	// Verify plan structure
	plan, ok := response.Data.(map[string]interface{})
	assert.True(suite.T(), ok)
	assert.Contains(suite.T(), plan, "restore_id")
	assert.Contains(suite.T(), plan, "phases")
	assert.Contains(suite.T(), plan, "estimated_time")
}

// Test Cases for Disaster Recovery

func (suite *RestoreAPITestSuite) TestExecuteDRScenario_Success() {
	body, _ := json.Marshal(suite.testData.ValidDRRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/dr/execute", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["admin"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusAccepted, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.Contains(suite.T(), response.Message, "started successfully")
}

func (suite *RestoreAPITestSuite) TestListDRScenarios_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/dr/scenarios", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)

	// Verify scenarios structure
	scenarios, ok := response.Data.([]map[string]interface{})
	assert.True(suite.T(), ok)
	assert.Greater(suite.T(), len(scenarios), 0)
}

func (suite *RestoreAPITestSuite) TestGetDRScenarioStatus_Success() {
	// First execute a DR scenario
	suite.TestExecuteDRScenario_Success()

	// Then get its status
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/dr/scenarios/dr-test-001", nil)
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

// Test Cases for Backup Management

func (suite *RestoreAPITestSuite) TestListAvailableBackups_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/backups", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestGetBackupDetails_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/backups/backup-2024-01-15-001", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestValidateBackup_Success() {
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/backups/backup-2024-01-15-001/validate", nil)
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

// Test Cases for Cluster Management

func (suite *RestoreAPITestSuite) TestListClusters_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/clusters", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestValidateCluster_Success() {
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/clusters/test-cluster/validate", nil)
	req.Header.Set("Authorization", suite.authHeaders["operator"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

func (suite *RestoreAPITestSuite) TestCheckClusterReadiness_Success() {
	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/clusters/test-cluster/readiness", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), response.Success)
	assert.NotNil(suite.T(), response.Data)
}

// Test Cases for Error Scenarios

func (suite *RestoreAPITestSuite) TestInvalidJSONRequest() {
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBufferString("invalid-json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["admin"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode)

	var response restore.APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	assert.False(suite.T(), response.Success)
	assert.Equal(suite.T(), "invalid_request", response.Error.Code)
}

func (suite *RestoreAPITestSuite) TestMissingAuthorizationHeader() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

func (suite *RestoreAPITestSuite) TestMalformedAuthorizationHeader() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["malformed"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

func (suite *RestoreAPITestSuite) TestExpiredToken() {
	body, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", suite.authHeaders["expired"])

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
}

// Test Cases for Rate Limiting and Performance

func (suite *RestoreAPITestSuite) TestConcurrentRequests() {
	concurrency := 10
	results := make(chan *http.Response, concurrency)
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history", nil)
			req.Header.Set("Authorization", suite.authHeaders["readonly"])

			resp, err := suite.client.Do(req)
			if err != nil {
				errors <- err
			} else {
				results <- resp
			}
		}()
	}

	successCount := 0
	for i := 0; i < concurrency; i++ {
		select {
		case resp := <-results:
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				successCount++
			}
		case err := <-errors:
			suite.T().Errorf("Request failed: %v", err)
		case <-time.After(30 * time.Second):
			suite.T().Error("Request timeout")
		}
	}

	assert.Greater(suite.T(), successCount, concurrency/2, "At least half of concurrent requests should succeed")
}

func (suite *RestoreAPITestSuite) TestRequestTimeout() {
	// Create a client with very short timeout
	shortTimeoutClient := &http.Client{
		Timeout: 1 * time.Millisecond,
	}

	req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history", nil)
	req.Header.Set("Authorization", suite.authHeaders["readonly"])

	_, err := shortTimeoutClient.Do(req)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "timeout")
}

// Benchmark Tests

func (suite *RestoreAPITestSuite) BenchmarkGetRestoreHistory() {
	b := suite.T().(*testing.T)
	
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history", nil)
		req.Header.Set("Authorization", suite.authHeaders["readonly"])

		start := time.Now()
		resp, err := suite.client.Do(req)
		duration := time.Since(start)

		require.NoError(b, err)
		resp.Body.Close()
		assert.Equal(b, http.StatusOK, resp.StatusCode)
		assert.Less(b, duration, 500*time.Millisecond, "Request should complete within 500ms")
	}
}

func (suite *RestoreAPITestSuite) BenchmarkListBackups() {
	b := suite.T().(*testing.T)
	
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/backups", nil)
		req.Header.Set("Authorization", suite.authHeaders["readonly"])

		start := time.Now()
		resp, err := suite.client.Do(req)
		duration := time.Since(start)

		require.NoError(b, err)
		resp.Body.Close()
		assert.Equal(b, http.StatusOK, resp.StatusCode)
		assert.Less(b, duration, 500*time.Millisecond, "Request should complete within 500ms")
	}
}

// Helper function to run all tests
func TestRestoreAPITestSuite(t *testing.T) {
	suite.Run(t, new(RestoreAPITestSuite))
}

// Additional test for comprehensive coverage
func (suite *RestoreAPITestSuite) TestCompleteAPIWorkflow() {
	ctx := context.Background()
	
	// 1. List available backups
	req1, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/backups", nil)
	req1.Header.Set("Authorization", suite.authHeaders["operator"])
	resp1, err := suite.client.Do(req1)
	require.NoError(suite.T(), err)
	resp1.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp1.StatusCode)

	// 2. Validate a backup
	req2, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/backups/backup-2024-01-15-001/validate", nil)
	req2.Header.Set("Authorization", suite.authHeaders["operator"])
	resp2, err := suite.client.Do(req2)
	require.NoError(suite.T(), err)
	resp2.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp2.StatusCode)

	// 3. Validate target cluster
	req3, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/clusters/test-cluster/validate", nil)
	req3.Header.Set("Authorization", suite.authHeaders["operator"])
	resp3, err := suite.client.Do(req3)
	require.NoError(suite.T(), err)
	resp3.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp3.StatusCode)

	// 4. Create restore plan
	body4, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req4, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore/plan", bytes.NewBuffer(body4))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Authorization", suite.authHeaders["operator"])
	resp4, err := suite.client.Do(req4)
	require.NoError(suite.T(), err)
	resp4.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp4.StatusCode)

	// 5. Validate restore request
	validateReq := suite.testData.ValidRestoreRequest
	validateReq.DryRun = true
	body5, _ := json.Marshal(validateReq)
	req5, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore/validate", bytes.NewBuffer(body5))
	req5.Header.Set("Content-Type", "application/json")
	req5.Header.Set("Authorization", suite.authHeaders["operator"])
	resp5, err := suite.client.Do(req5)
	require.NoError(suite.T(), err)
	resp5.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp5.StatusCode)

	// 6. Start actual restore
	body6, _ := json.Marshal(suite.testData.ValidRestoreRequest)
	req6, _ := http.NewRequest("POST", suite.baseURL+"/api/v1/restore", bytes.NewBuffer(body6))
	req6.Header.Set("Content-Type", "application/json")
	req6.Header.Set("Authorization", suite.authHeaders["admin"])
	resp6, err := suite.client.Do(req6)
	require.NoError(suite.T(), err)
	resp6.Body.Close()
	assert.Equal(suite.T(), http.StatusAccepted, resp6.StatusCode)

	// 7. Monitor restore progress
	for i := 0; i < 5; i++ {
		req7, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/test-restore-001", nil)
		req7.Header.Set("Authorization", suite.authHeaders["operator"])
		resp7, err := suite.client.Do(req7)
		require.NoError(suite.T(), err)
		resp7.Body.Close()
		assert.Equal(suite.T(), http.StatusOK, resp7.StatusCode)
		
		time.Sleep(1 * time.Second)
	}

	// 8. Check restore history
	req8, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/restore/history", nil)
	req8.Header.Set("Authorization", suite.authHeaders["operator"])
	resp8, err := suite.client.Do(req8)
	require.NoError(suite.T(), err)
	resp8.Body.Close()
	assert.Equal(suite.T(), http.StatusOK, resp8.StatusCode)

	_ = ctx // Avoid unused variable warning
}