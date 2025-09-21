package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStructuredLogger(t *testing.T) {
	tests := []struct {
		name            string
		service         string
		clusterName     string
		expectedService string
		expectedCluster string
	}{
		{
			name:            "normal_values",
			service:         "backup-service",
			clusterName:     "test-cluster",
			expectedService: "backup-service",
			expectedCluster: "test-cluster",
		},
		{
			name:            "empty_service",
			service:         "",
			clusterName:     "test-cluster",
			expectedService: "backup",
			expectedCluster: "test-cluster",
		},
		{
			name:            "empty_cluster",
			service:         "backup-service",
			clusterName:     "",
			expectedService: "backup-service",
			expectedCluster: "unknown",
		},
		{
			name:            "both_empty",
			service:         "",
			clusterName:     "",
			expectedService: "backup",
			expectedCluster: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewStructuredLogger(tt.service, tt.clusterName)
			
			assert.Equal(t, tt.expectedService, logger.GetService())
			assert.Equal(t, tt.expectedCluster, logger.GetClusterName())
		})
	}
}

func TestStructuredLogger_Log(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := NewStructuredLogger("test-service", "test-cluster")
	
	testData := map[string]interface{}{
		"namespace": "default",
		"resource":  "deployment",
		"count":     5,
	}

	// Test Info logging
	logger.Info("test_operation", "Test message", testData)

	// Parse the JSON log output
	logOutput := buf.String()
	assert.Contains(t, logOutput, "test_operation")
	assert.Contains(t, logOutput, "Test message")
	
	// Parse JSON to verify structure
	var logEntry LogEntry
	jsonStr := strings.TrimSpace(strings.TrimPrefix(logOutput, time.Now().Format("2006/01/02 15:04:05 ")))
	err := json.Unmarshal([]byte(jsonStr), &logEntry)
	require.NoError(t, err)
	
	assert.Equal(t, "INFO", logEntry.Level)
	assert.Equal(t, "test-service", logEntry.Service)
	assert.Equal(t, "test-cluster", logEntry.Cluster)
	assert.Equal(t, "test_operation", logEntry.Operation)
	assert.Equal(t, "Test message", logEntry.Message)
	assert.Equal(t, "default", logEntry.Data["namespace"])
	assert.Equal(t, "deployment", logEntry.Data["resource"])
	assert.Equal(t, float64(5), logEntry.Data["count"]) // JSON unmarshals numbers as float64
}

func TestStructuredLogger_AllLevels(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := NewStructuredLogger("test-service", "test-cluster")

	// Test all log levels
	levels := []struct {
		level string
		fn    func(string, string, map[string]interface{})
	}{
		{"INFO", logger.Info},
		{"ERROR", logger.Error},
		{"WARNING", logger.Warning},
		{"DEBUG", logger.Debug},
	}

	for _, level := range levels {
		buf.Reset()
		level.fn("test_op", "test message", nil)
		
		logOutput := buf.String()
		assert.Contains(t, logOutput, level.level)
		assert.Contains(t, logOutput, "test_op")
		assert.Contains(t, logOutput, "test message")
	}
}

func TestStructuredLogger_SetClusterName(t *testing.T) {
	logger := NewStructuredLogger("test-service", "initial-cluster")
	
	// Test setting valid cluster name
	logger.SetClusterName("new-cluster")
	assert.Equal(t, "new-cluster", logger.GetClusterName())
	
	// Test setting empty cluster name (should not change)
	logger.SetClusterName("")
	assert.Equal(t, "new-cluster", logger.GetClusterName())
}

func TestStructuredLogger_SetService(t *testing.T) {
	logger := NewStructuredLogger("initial-service", "test-cluster")
	
	// Test setting valid service name
	logger.SetService("new-service")
	assert.Equal(t, "new-service", logger.GetService())
	
	// Test setting empty service name (should not change)
	logger.SetService("")
	assert.Equal(t, "new-service", logger.GetService())
}

func TestStructuredLogger_WithContext(t *testing.T) {
	logger := NewStructuredLogger("test-service", "test-cluster")
	
	// Create logger with context
	contextLogger := logger.WithContext("namespace", "kube-system")
	
	// Verify the new logger has the same base properties
	assert.Equal(t, "test-service", contextLogger.GetService())
	assert.Equal(t, "test-cluster", contextLogger.GetClusterName())
	
	// Verify original logger is unchanged
	assert.Equal(t, "test-service", logger.GetService())
	assert.Equal(t, "test-cluster", logger.GetClusterName())
}

func TestStructuredLogger_LogDuration(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := NewStructuredLogger("test-service", "test-cluster")
	
	startTime := time.Now().Add(-100 * time.Millisecond) // Simulate 100ms operation
	
	logger.LogDuration("backup_operation", startTime, "Backup completed", map[string]interface{}{
		"namespace": "default",
	})

	logOutput := buf.String()
	assert.Contains(t, logOutput, "backup_operation")
	assert.Contains(t, logOutput, "Backup completed")
	assert.Contains(t, logOutput, "duration_seconds")
	assert.Contains(t, logOutput, "duration_ms")
	assert.Contains(t, logOutput, "namespace")
}

func TestStructuredLogger_LogDurationWithNilData(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := NewStructuredLogger("test-service", "test-cluster")
	
	startTime := time.Now().Add(-50 * time.Millisecond)
	
	// Test with nil data
	logger.LogDuration("test_operation", startTime, "Operation completed", nil)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "duration_seconds")
	assert.Contains(t, logOutput, "duration_ms")
}

func TestIsValidLogLevel(t *testing.T) {
	tests := []struct {
		level string
		valid bool
	}{
		{"DEBUG", true},
		{"INFO", true},
		{"WARNING", true},
		{"ERROR", true},
		{"FATAL", true},
		{"TRACE", false},
		{"INVALID", false},
		{"info", false}, // Case sensitive
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			result := IsValidLogLevel(tt.level)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestLogEntry_JSONMarshaling(t *testing.T) {
	entry := LogEntry{
		Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     "INFO",
		Service:   "test-service",
		Cluster:   "test-cluster",
		Operation: "test_op",
		Message:   "test message",
		Data: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
			"key3": true,
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(entry)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaled LogEntry
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, entry.Level, unmarshaled.Level)
	assert.Equal(t, entry.Service, unmarshaled.Service)
	assert.Equal(t, entry.Cluster, unmarshaled.Cluster)
	assert.Equal(t, entry.Operation, unmarshaled.Operation)
	assert.Equal(t, entry.Message, unmarshaled.Message)
	assert.Equal(t, "value1", unmarshaled.Data["key1"])
	assert.Equal(t, float64(123), unmarshaled.Data["key2"]) // JSON numbers become float64
	assert.Equal(t, true, unmarshaled.Data["key3"])
}

// Benchmark tests
func BenchmarkStructuredLogger_Info(b *testing.B) {
	// Discard log output for benchmarking
	log.SetOutput(bytes.NewBuffer(nil))
	defer log.SetOutput(os.Stderr)

	logger := NewStructuredLogger("bench-service", "bench-cluster")
	data := map[string]interface{}{
		"namespace": "default",
		"resource":  "deployment",
		"count":     10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("bench_operation", "Benchmark message", data)
	}
}

func BenchmarkStructuredLogger_LogDuration(b *testing.B) {
	// Discard log output for benchmarking
	log.SetOutput(bytes.NewBuffer(nil))
	defer log.SetOutput(os.Stderr)

	logger := NewStructuredLogger("bench-service", "bench-cluster")
	startTime := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.LogDuration("bench_operation", startTime, "Benchmark duration", nil)
	}
}