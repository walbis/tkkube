package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// StructuredLogger provides structured logging capabilities
type StructuredLogger struct {
	service     string
	clusterName string
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       string                 `json:"level"`
	Service     string                 `json:"service"`
	Cluster     string                 `json:"cluster"`
	Operation   string                 `json:"operation"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(service, clusterName string) *StructuredLogger {
	if service == "" {
		service = "backup"
	}
	if clusterName == "" {
		clusterName = "unknown"
	}
	
	return &StructuredLogger{
		service:     service,
		clusterName: clusterName,
	}
}

// Info logs an info level message
func (sl *StructuredLogger) Info(operation, message string, data map[string]interface{}) {
	sl.log("INFO", operation, message, data)
}

// Error logs an error level message
func (sl *StructuredLogger) Error(operation, message string, data map[string]interface{}) {
	sl.log("ERROR", operation, message, data)
}

// Warning logs a warning level message
func (sl *StructuredLogger) Warning(operation, message string, data map[string]interface{}) {
	sl.log("WARNING", operation, message, data)
}

// Debug logs a debug level message
func (sl *StructuredLogger) Debug(operation, message string, data map[string]interface{}) {
	sl.log("DEBUG", operation, message, data)
}

// log writes a structured log entry
func (sl *StructuredLogger) log(level, operation, message string, data map[string]interface{}) {
	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Service:   sl.service,
		Cluster:   sl.clusterName,
		Operation: operation,
		Message:   message,
		Data:      data,
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple logging if JSON marshaling fails
		log.Printf("[%s] %s - %s: %s (marshal error: %v)", level, sl.service, operation, message, err)
		return
	}

	log.Printf("%s", string(jsonData))
}

// SetClusterName updates the cluster name for the logger
func (sl *StructuredLogger) SetClusterName(clusterName string) {
	if clusterName != "" {
		sl.clusterName = clusterName
	}
}

// SetService updates the service name for the logger
func (sl *StructuredLogger) SetService(service string) {
	if service != "" {
		sl.service = service
	}
}

// GetClusterName returns the current cluster name
func (sl *StructuredLogger) GetClusterName() string {
	return sl.clusterName
}

// GetService returns the current service name
func (sl *StructuredLogger) GetService() string {
	return sl.service
}

// WithContext creates a new logger with additional context
func (sl *StructuredLogger) WithContext(key, value string) *StructuredLogger {
	newLogger := &StructuredLogger{
		service:     sl.service,
		clusterName: sl.clusterName,
	}
	
	// For simplicity, we'll just return a copy. In a more advanced implementation,
	// you might want to store additional context in the logger.
	return newLogger
}

// LogDuration logs the duration of an operation
func (sl *StructuredLogger) LogDuration(operation string, startTime time.Time, message string, data map[string]interface{}) {
	duration := time.Since(startTime)
	
	if data == nil {
		data = make(map[string]interface{})
	}
	data["duration_seconds"] = duration.Seconds()
	data["duration_ms"] = duration.Milliseconds()
	
	sl.Info(operation, fmt.Sprintf("%s (took %v)", message, duration), data)
}

// IsValidLogLevel checks if a log level is valid
func IsValidLogLevel(level string) bool {
	validLevels := map[string]bool{
		"DEBUG":   true,
		"INFO":    true,
		"WARNING": true,
		"ERROR":   true,
		"FATAL":   true,
	}
	return validLevels[level]
}