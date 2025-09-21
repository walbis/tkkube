package monitoring

import (
	"context"
	"fmt"
	"log"
	"time"
)

// DefaultLogger provides a simple implementation of the Logger interface
type DefaultLogger struct {
	component string
	fields    map[string]interface{}
	prefix    string
}

// NewLogger creates a new logger instance
func NewLogger(component string) Logger {
	return &DefaultLogger{
		component: component,
		fields:    make(map[string]interface{}),
		prefix:    fmt.Sprintf("[%s] ", component),
	}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(message string, fields map[string]interface{}) {
	l.log("DEBUG", message, fields)
}

// Info logs an info message
func (l *DefaultLogger) Info(message string, fields map[string]interface{}) {
	l.log("INFO", message, fields)
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(message string, fields map[string]interface{}) {
	l.log("WARN", message, fields)
}

// Error logs an error message
func (l *DefaultLogger) Error(message string, fields map[string]interface{}) {
	l.log("ERROR", message, fields)
}

// Fatal logs a fatal message
func (l *DefaultLogger) Fatal(message string, fields map[string]interface{}) {
	l.log("FATAL", message, fields)
}

// WithContext returns a logger with context information
func (l *DefaultLogger) WithContext(ctx context.Context) Logger {
	newLogger := &DefaultLogger{
		component: l.component,
		fields:    make(map[string]interface{}),
		prefix:    l.prefix,
	}
	
	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	
	// Add context information if available
	if traceID := ctx.Value("trace_id"); traceID != nil {
		newLogger.fields["trace_id"] = traceID
	}
	if spanID := ctx.Value("span_id"); spanID != nil {
		newLogger.fields["span_id"] = spanID
	}
	
	return newLogger
}

// WithFields returns a logger with additional fields
func (l *DefaultLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := &DefaultLogger{
		component: l.component,
		fields:    make(map[string]interface{}),
		prefix:    l.prefix,
	}
	
	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	
	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	
	return newLogger
}

// log performs the actual logging
func (l *DefaultLogger) log(level, message string, fields map[string]interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	
	// Combine logger fields with message fields
	allFields := make(map[string]interface{})
	for k, v := range l.fields {
		allFields[k] = v
	}
	for k, v := range fields {
		allFields[k] = v
	}
	
	// Format the log message
	logMsg := fmt.Sprintf("%s %s%s %s", timestamp, l.prefix, level, message)
	
	// Add fields if present
	if len(allFields) > 0 {
		logMsg += " "
		for k, v := range allFields {
			logMsg += fmt.Sprintf("%s=%v ", k, v)
		}
	}
	
	// Use standard log package for output
	log.Println(logMsg)
}

// NullLogger provides a no-op logger implementation
type NullLogger struct{}

// NewNullLogger creates a logger that discards all messages
func NewNullLogger() Logger {
	return &NullLogger{}
}

func (nl *NullLogger) Debug(message string, fields map[string]interface{}) {}
func (nl *NullLogger) Info(message string, fields map[string]interface{})  {}
func (nl *NullLogger) Warn(message string, fields map[string]interface{})  {}
func (nl *NullLogger) Error(message string, fields map[string]interface{}) {}
func (nl *NullLogger) Fatal(message string, fields map[string]interface{}) {}

func (nl *NullLogger) WithContext(ctx context.Context) Logger {
	return nl
}

func (nl *NullLogger) WithFields(fields map[string]interface{}) Logger {
	return nl
}

// StructuredLogger provides JSON-structured logging
type StructuredLogger struct {
	component string
	fields    map[string]interface{}
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(component string) Logger {
	return &StructuredLogger{
		component: component,
		fields:    make(map[string]interface{}),
	}
}

// Debug logs a debug message in structured format
func (sl *StructuredLogger) Debug(message string, fields map[string]interface{}) {
	sl.logStructured("debug", message, fields)
}

// Info logs an info message in structured format
func (sl *StructuredLogger) Info(message string, fields map[string]interface{}) {
	sl.logStructured("info", message, fields)
}

// Warn logs a warning message in structured format
func (sl *StructuredLogger) Warn(message string, fields map[string]interface{}) {
	sl.logStructured("warn", message, fields)
}

// Error logs an error message in structured format
func (sl *StructuredLogger) Error(message string, fields map[string]interface{}) {
	sl.logStructured("error", message, fields)
}

// Fatal logs a fatal message in structured format
func (sl *StructuredLogger) Fatal(message string, fields map[string]interface{}) {
	sl.logStructured("fatal", message, fields)
}

// WithContext returns a structured logger with context information
func (sl *StructuredLogger) WithContext(ctx context.Context) Logger {
	newLogger := &StructuredLogger{
		component: sl.component,
		fields:    make(map[string]interface{}),
	}
	
	// Copy existing fields
	for k, v := range sl.fields {
		newLogger.fields[k] = v
	}
	
	// Add context information if available
	if traceID := ctx.Value("trace_id"); traceID != nil {
		newLogger.fields["trace_id"] = traceID
	}
	if spanID := ctx.Value("span_id"); spanID != nil {
		newLogger.fields["span_id"] = spanID
	}
	
	return newLogger
}

// WithFields returns a structured logger with additional fields
func (sl *StructuredLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := &StructuredLogger{
		component: sl.component,
		fields:    make(map[string]interface{}),
	}
	
	// Copy existing fields
	for k, v := range sl.fields {
		newLogger.fields[k] = v
	}
	
	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	
	return newLogger
}

// logStructured performs structured logging
func (sl *StructuredLogger) logStructured(level, message string, fields map[string]interface{}) {
	logEntry := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"level":     level,
		"component": sl.component,
		"message":   message,
	}
	
	// Add logger fields
	for k, v := range sl.fields {
		logEntry[k] = v
	}
	
	// Add message fields
	for k, v := range fields {
		logEntry[k] = v
	}
	
	// Format as JSON-like string (simplified)
	logMsg := fmt.Sprintf("{\"timestamp\":\"%s\",\"level\":\"%s\",\"component\":\"%s\",\"message\":\"%s\"",
		logEntry["timestamp"], level, sl.component, message)
	
	for k, v := range sl.fields {
		logMsg += fmt.Sprintf(",\"%s\":\"%v\"", k, v)
	}
	for k, v := range fields {
		logMsg += fmt.Sprintf(",\"%s\":\"%v\"", k, v)
	}
	
	logMsg += "}"
	
	log.Println(logMsg)
}

// LoggerFactory provides factory methods for creating loggers
type LoggerFactory struct {
	loggerType string
	baseFields map[string]interface{}
}

// NewLoggerFactory creates a new logger factory
func NewLoggerFactory(loggerType string) *LoggerFactory {
	return &LoggerFactory{
		loggerType: loggerType,
		baseFields: make(map[string]interface{}),
	}
}

// SetBaseFields sets base fields that will be included in all loggers
func (lf *LoggerFactory) SetBaseFields(fields map[string]interface{}) {
	lf.baseFields = fields
}

// CreateLogger creates a logger of the configured type
func (lf *LoggerFactory) CreateLogger(component string) Logger {
	var logger Logger
	
	switch lf.loggerType {
	case "structured":
		logger = NewStructuredLogger(component)
	case "null":
		logger = NewNullLogger()
	default:
		logger = NewLogger(component)
	}
	
	// Add base fields if any
	if len(lf.baseFields) > 0 {
		logger = logger.WithFields(lf.baseFields)
	}
	
	return logger
}