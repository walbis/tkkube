package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// TracingSystem provides distributed tracing capabilities
type TracingSystem struct {
	config    *TracingConfig
	spans     map[string]*Span
	traces    map[string]*Trace
	exporter  TraceExporter
	sampler   Sampler
	mu        sync.RWMutex
	running   bool
	stopChan  chan struct{}
}

// TracingConfig defines configuration for distributed tracing
type TracingConfig struct {
	// Sampling configuration
	SamplingRate    float64       `json:"sampling_rate"`
	SamplingType    SamplingType  `json:"sampling_type"`
	
	// Span limits
	MaxSpansPerTrace    int           `json:"max_spans_per_trace"`
	MaxAttributesPerSpan int          `json:"max_attributes_per_span"`
	MaxEventsPerSpan    int           `json:"max_events_per_span"`
	MaxLinksPerSpan     int           `json:"max_links_per_span"`
	
	// Retention and export
	SpanRetention       time.Duration `json:"span_retention"`
	ExportInterval      time.Duration `json:"export_interval"`
	ExportBatchSize     int           `json:"export_batch_size"`
	
	// Propagation
	PropagationFormat   string        `json:"propagation_format"` // "w3c", "b3", "jaeger"
	
	// Service identification
	ServiceName         string        `json:"service_name"`
	ServiceVersion      string        `json:"service_version"`
	Environment         string        `json:"environment"`
}

// SamplingType defines the sampling strategy
type SamplingType string

const (
	SamplingAlwaysOn     SamplingType = "always_on"
	SamplingAlwaysOff    SamplingType = "always_off"
	SamplingProbability  SamplingType = "probability"
	SamplingRateLimiting SamplingType = "rate_limiting"
	SamplingAdaptive     SamplingType = "adaptive"
	SamplingTailBased    SamplingType = "tail_based"
)

// Span represents a unit of work in a distributed trace
type Span struct {
	TraceID      string                 `json:"trace_id"`
	SpanID       string                 `json:"span_id"`
	ParentSpanID string                 `json:"parent_span_id,omitempty"`
	Name         string                 `json:"name"`
	Kind         SpanKind               `json:"kind"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      *time.Time             `json:"end_time,omitempty"`
	Duration     *time.Duration         `json:"duration,omitempty"`
	Status       SpanStatus             `json:"status"`
	Attributes   map[string]interface{} `json:"attributes"`
	Events       []SpanEvent            `json:"events"`
	Links        []SpanLink             `json:"links"`
	Resource     map[string]interface{} `json:"resource"`
	
	// Internal
	mu           sync.Mutex
	isRecording  bool
	hasEnded     bool
}

// SpanKind represents the type of span
type SpanKind string

const (
	SpanKindInternal SpanKind = "internal"
	SpanKindServer   SpanKind = "server"
	SpanKindClient   SpanKind = "client"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
)

// SpanStatus represents the status of a span
type SpanStatus struct {
	Code        StatusCode `json:"code"`
	Description string     `json:"description,omitempty"`
}

// StatusCode represents the status code of a span
type StatusCode string

const (
	StatusCodeUnset StatusCode = "unset"
	StatusCodeOK    StatusCode = "ok"
	StatusCodeError StatusCode = "error"
)

// SpanEvent represents an event within a span
type SpanEvent struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes"`
}

// SpanLink represents a link to another span
type SpanLink struct {
	TraceID    string                 `json:"trace_id"`
	SpanID     string                 `json:"span_id"`
	Attributes map[string]interface{} `json:"attributes"`
}

// Trace represents a complete distributed trace
type Trace struct {
	TraceID   string    `json:"trace_id"`
	Spans     []*Span   `json:"spans"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	
	// Trace-level metadata
	Service    string                 `json:"service"`
	Operation  string                 `json:"operation"`
	Status     StatusCode             `json:"status"`
	Attributes map[string]interface{} `json:"attributes"`
	
	// Statistics
	SpanCount        int     `json:"span_count"`
	ErrorCount       int     `json:"error_count"`
	CriticalPath     []*Span `json:"critical_path,omitempty"`
	TotalNetworkTime time.Duration `json:"total_network_time,omitempty"`
}

// NewTracingSystem creates a new distributed tracing system
func NewTracingSystem(config *TracingConfig) *TracingSystem {
	ts := &TracingSystem{
		config:   config,
		spans:    make(map[string]*Span),
		traces:   make(map[string]*Trace),
		sampler:  NewSampler(config.SamplingType, config.SamplingRate),
		stopChan: make(chan struct{}),
	}
	
	// Initialize exporter
	ts.exporter = NewJaegerExporter(config)
	
	return ts
}

// Start begins the tracing system
func (ts *TracingSystem) Start(ctx context.Context) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	if ts.running {
		return fmt.Errorf("tracing system already running")
	}
	
	ts.running = true
	
	// Start export goroutine
	go ts.runExport(ctx)
	
	// Start cleanup goroutine
	go ts.runCleanup(ctx)
	
	return nil
}

// Stop halts the tracing system
func (ts *TracingSystem) Stop() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	if !ts.running {
		return nil
	}
	
	close(ts.stopChan)
	ts.running = false
	
	// Export remaining spans
	ts.exportAllSpans()
	
	return nil
}

// StartSpan starts a new span
func (ts *TracingSystem) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	// Check sampling decision
	if !ts.sampler.ShouldSample(name) {
		return ctx, nil
	}
	
	// Create span
	span := &Span{
		TraceID:     ts.getOrCreateTraceID(ctx),
		SpanID:      ts.generateSpanID(),
		Name:        name,
		Kind:        SpanKindInternal,
		StartTime:   time.Now(),
		Status:      SpanStatus{Code: StatusCodeUnset},
		Attributes:  make(map[string]interface{}),
		Events:      []SpanEvent{},
		Links:       []SpanLink{},
		Resource:    ts.getResourceAttributes(),
		isRecording: true,
	}
	
	// Apply options
	for _, opt := range opts {
		opt(span)
	}
	
	// Get parent span from context
	if parentSpan := SpanFromContext(ctx); parentSpan != nil {
		span.ParentSpanID = parentSpan.SpanID
		span.TraceID = parentSpan.TraceID
	}
	
	// Store span
	ts.mu.Lock()
	ts.spans[span.SpanID] = span
	
	// Add to trace
	if trace, exists := ts.traces[span.TraceID]; exists {
		trace.Spans = append(trace.Spans, span)
		trace.SpanCount++
	} else {
		ts.traces[span.TraceID] = &Trace{
			TraceID:    span.TraceID,
			Spans:      []*Span{span},
			StartTime:  span.StartTime,
			Service:    ts.config.ServiceName,
			Operation:  name,
			Status:     StatusCodeUnset,
			Attributes: make(map[string]interface{}),
			SpanCount:  1,
		}
	}
	ts.mu.Unlock()
	
	// Add span to context
	ctx = ContextWithSpan(ctx, span)
	
	return ctx, span
}

// EndSpan ends a span
func (ts *TracingSystem) EndSpan(span *Span) {
	if span == nil || !span.isRecording {
		return
	}
	
	span.mu.Lock()
	defer span.mu.Unlock()
	
	if span.hasEnded {
		return
	}
	
	now := time.Now()
	span.EndTime = &now
	duration := now.Sub(span.StartTime)
	span.Duration = &duration
	span.hasEnded = true
	span.isRecording = false
	
	// Update trace
	ts.mu.Lock()
	if trace, exists := ts.traces[span.TraceID]; exists {
		// Update trace end time
		if span.EndTime.After(trace.EndTime) {
			trace.EndTime = *span.EndTime
			trace.Duration = trace.EndTime.Sub(trace.StartTime)
		}
		
		// Update error count
		if span.Status.Code == StatusCodeError {
			trace.ErrorCount++
			trace.Status = StatusCodeError
		}
	}
	ts.mu.Unlock()
}

// RecordError records an error on a span
func (ts *TracingSystem) RecordError(span *Span, err error) {
	if span == nil || !span.isRecording {
		return
	}
	
	span.mu.Lock()
	defer span.mu.Unlock()
	
	// Set error status
	span.Status = SpanStatus{
		Code:        StatusCodeError,
		Description: err.Error(),
	}
	
	// Add error event
	event := SpanEvent{
		Name:      "error",
		Timestamp: time.Now(),
		Attributes: map[string]interface{}{
			"error.type":    fmt.Sprintf("%T", err),
			"error.message": err.Error(),
		},
	}
	
	if len(span.Events) < ts.config.MaxEventsPerSpan {
		span.Events = append(span.Events, event)
	}
	
	// Add error attributes
	span.Attributes["error"] = true
	span.Attributes["error.message"] = err.Error()
}

// AddEvent adds an event to a span
func (ts *TracingSystem) AddEvent(span *Span, name string, attributes map[string]interface{}) {
	if span == nil || !span.isRecording {
		return
	}
	
	span.mu.Lock()
	defer span.mu.Unlock()
	
	if len(span.Events) >= ts.config.MaxEventsPerSpan {
		return
	}
	
	event := SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attributes,
	}
	
	span.Events = append(span.Events, event)
}

// SetAttributes sets attributes on a span
func (ts *TracingSystem) SetAttributes(span *Span, attributes map[string]interface{}) {
	if span == nil || !span.isRecording {
		return
	}
	
	span.mu.Lock()
	defer span.mu.Unlock()
	
	for k, v := range attributes {
		if len(span.Attributes) < ts.config.MaxAttributesPerSpan {
			span.Attributes[k] = v
		}
	}
}

// getOrCreateTraceID gets or creates a trace ID
func (ts *TracingSystem) getOrCreateTraceID(ctx context.Context) string {
	// Check if trace ID exists in context
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		return traceID
	}
	
	// Generate new trace ID
	return ts.generateTraceID()
}

// generateTraceID generates a new trace ID
func (ts *TracingSystem) generateTraceID() string {
	// Generate 128-bit trace ID
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// generateSpanID generates a new span ID
func (ts *TracingSystem) generateSpanID() string {
	// Generate 64-bit span ID
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// getResourceAttributes gets resource attributes for spans
func (ts *TracingSystem) getResourceAttributes() map[string]interface{} {
	return map[string]interface{}{
		"service.name":    ts.config.ServiceName,
		"service.version": ts.config.ServiceVersion,
		"environment":     ts.config.Environment,
	}
}

// runExport handles periodic span export
func (ts *TracingSystem) runExport(ctx context.Context) {
	ticker := time.NewTicker(ts.config.ExportInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ts.stopChan:
			return
		case <-ticker.C:
			ts.exportSpans()
		}
	}
}

// exportSpans exports completed spans
func (ts *TracingSystem) exportSpans() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	// Find completed spans
	completedSpans := []*Span{}
	for spanID, span := range ts.spans {
		if span.hasEnded {
			completedSpans = append(completedSpans, span)
			delete(ts.spans, spanID)
		}
	}
	
	if len(completedSpans) == 0 {
		return
	}
	
	// Export spans
	if err := ts.exporter.ExportSpans(completedSpans); err != nil {
		fmt.Printf("Failed to export spans: %v\n", err)
	}
}

// exportAllSpans exports all remaining spans
func (ts *TracingSystem) exportAllSpans() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	allSpans := []*Span{}
	for _, span := range ts.spans {
		allSpans = append(allSpans, span)
	}
	
	if len(allSpans) > 0 {
		ts.exporter.ExportSpans(allSpans)
	}
}

// runCleanup handles periodic cleanup of old traces
func (ts *TracingSystem) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ts.stopChan:
			return
		case <-ticker.C:
			ts.cleanupOldTraces()
		}
	}
}

// cleanupOldTraces removes old traces from memory
func (ts *TracingSystem) cleanupOldTraces() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	cutoff := time.Now().Add(-ts.config.SpanRetention)
	
	for traceID, trace := range ts.traces {
		if trace.EndTime.Before(cutoff) {
			delete(ts.traces, traceID)
		}
	}
}

// GetTrace returns a complete trace
func (ts *TracingSystem) GetTrace(traceID string) *Trace {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	return ts.traces[traceID]
}

// GetActiveSpans returns currently active spans
func (ts *TracingSystem) GetActiveSpans() []*Span {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	activeSpans := []*Span{}
	for _, span := range ts.spans {
		if span.isRecording {
			activeSpans = append(activeSpans, span)
		}
	}
	
	return activeSpans
}

// SpanOption is an option for creating spans
type SpanOption func(*Span)

// WithSpanKind sets the span kind
func WithSpanKind(kind SpanKind) SpanOption {
	return func(s *Span) {
		s.Kind = kind
	}
}

// WithAttributes sets initial attributes
func WithAttributes(attrs map[string]interface{}) SpanOption {
	return func(s *Span) {
		for k, v := range attrs {
			s.Attributes[k] = v
		}
	}
}

// WithLinks adds links to other spans
func WithLinks(links []SpanLink) SpanOption {
	return func(s *Span) {
		s.Links = links
	}
}

// Context management functions

type contextKey string

const (
	spanContextKey  contextKey = "span"
	traceContextKey contextKey = "trace_id"
)

// ContextWithSpan adds a span to context
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey, span)
}

// SpanFromContext extracts a span from context
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanContextKey).(*Span); ok {
		return span
	}
	return nil
}

// TraceIDFromContext extracts a trace ID from context
func TraceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceContextKey).(string); ok {
		return traceID
	}
	return ""
}

// Sampler interface for trace sampling decisions
type Sampler interface {
	ShouldSample(spanName string) bool
}

// ProbabilitySampler samples based on probability
type ProbabilitySampler struct {
	probability float64
}

// NewSampler creates a sampler based on type
func NewSampler(samplingType SamplingType, rate float64) Sampler {
	switch samplingType {
	case SamplingAlwaysOn:
		return &AlwaysOnSampler{}
	case SamplingAlwaysOff:
		return &AlwaysOffSampler{}
	case SamplingProbability:
		return &ProbabilitySampler{probability: rate}
	case SamplingRateLimiting:
		return NewRateLimitingSampler(rate)
	case SamplingAdaptive:
		return NewAdaptiveSampler(rate)
	default:
		return &ProbabilitySampler{probability: rate}
	}
}

// ShouldSample implements Sampler
func (s *ProbabilitySampler) ShouldSample(spanName string) bool {
	return rand.Float64() < s.probability
}

// AlwaysOnSampler always samples
type AlwaysOnSampler struct{}

// ShouldSample implements Sampler
func (s *AlwaysOnSampler) ShouldSample(spanName string) bool {
	return true
}

// AlwaysOffSampler never samples
type AlwaysOffSampler struct{}

// ShouldSample implements Sampler
func (s *AlwaysOffSampler) ShouldSample(spanName string) bool {
	return false
}

// RateLimitingSampler limits sampling rate
type RateLimitingSampler struct {
	rateLimit float64
	bucket    float64
	lastCheck time.Time
	mu        sync.Mutex
}

// NewRateLimitingSampler creates a rate limiting sampler
func NewRateLimitingSampler(rateLimit float64) *RateLimitingSampler {
	return &RateLimitingSampler{
		rateLimit: rateLimit,
		bucket:    rateLimit,
		lastCheck: time.Now(),
	}
}

// ShouldSample implements Sampler
func (s *RateLimitingSampler) ShouldSample(spanName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	now := time.Now()
	elapsed := now.Sub(s.lastCheck).Seconds()
	s.lastCheck = now
	
	// Refill bucket
	s.bucket += elapsed * s.rateLimit
	if s.bucket > s.rateLimit {
		s.bucket = s.rateLimit
	}
	
	// Check if we can sample
	if s.bucket >= 1 {
		s.bucket--
		return true
	}
	
	return false
}

// AdaptiveSampler adjusts sampling rate based on load
type AdaptiveSampler struct {
	targetRate    float64
	currentRate   float64
	spanCount     int64
	sampleCount   int64
	lastAdjust    time.Time
	mu            sync.Mutex
}

// NewAdaptiveSampler creates an adaptive sampler
func NewAdaptiveSampler(targetRate float64) *AdaptiveSampler {
	return &AdaptiveSampler{
		targetRate:  targetRate,
		currentRate: targetRate,
		lastAdjust:  time.Now(),
	}
}

// ShouldSample implements Sampler
func (s *AdaptiveSampler) ShouldSample(spanName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.spanCount++
	
	// Adjust rate every 100 spans
	if s.spanCount%100 == 0 {
		s.adjustRate()
	}
	
	// Sample based on current rate
	if rand.Float64() < s.currentRate {
		s.sampleCount++
		return true
	}
	
	return false
}

// adjustRate adjusts the sampling rate
func (s *AdaptiveSampler) adjustRate() {
	actualRate := float64(s.sampleCount) / float64(s.spanCount)
	
	// Adjust towards target
	if actualRate < s.targetRate {
		s.currentRate = s.currentRate * 1.1
		if s.currentRate > 1.0 {
			s.currentRate = 1.0
		}
	} else if actualRate > s.targetRate {
		s.currentRate = s.currentRate * 0.9
		if s.currentRate < 0.001 {
			s.currentRate = 0.001
		}
	}
}

// TraceExporter interface for exporting traces
type TraceExporter interface {
	ExportSpans(spans []*Span) error
	Shutdown() error
}

// JaegerExporter exports traces to Jaeger
type JaegerExporter struct {
	config   *TracingConfig
	endpoint string
}

// NewJaegerExporter creates a new Jaeger exporter
func NewJaegerExporter(config *TracingConfig) *JaegerExporter {
	return &JaegerExporter{
		config:   config,
		endpoint: "http://localhost:14268/api/traces",
	}
}

// ExportSpans exports spans to Jaeger
func (e *JaegerExporter) ExportSpans(spans []*Span) error {
	// Convert spans to Jaeger format
	// This is simplified - in production, use the Jaeger client library
	
	for _, span := range spans {
		// Convert and send span
		jaegerSpan := e.convertToJaegerFormat(span)
		// Send to Jaeger (simplified)
		_ = jaegerSpan
	}
	
	return nil
}

// convertToJaegerFormat converts a span to Jaeger format
func (e *JaegerExporter) convertToJaegerFormat(span *Span) map[string]interface{} {
	jaegerSpan := map[string]interface{}{
		"traceID":       span.TraceID,
		"spanID":        span.SpanID,
		"parentSpanID":  span.ParentSpanID,
		"operationName": span.Name,
		"startTime":     span.StartTime.UnixMicro(),
		"tags":          span.Attributes,
		"logs":          e.convertEvents(span.Events),
		"process": map[string]interface{}{
			"serviceName": e.config.ServiceName,
			"tags":        span.Resource,
		},
	}
	
	if span.EndTime != nil {
		jaegerSpan["duration"] = span.Duration.Microseconds()
	}
	
	return jaegerSpan
}

// convertEvents converts span events to Jaeger logs
func (e *JaegerExporter) convertEvents(events []SpanEvent) []map[string]interface{} {
	logs := []map[string]interface{}{}
	
	for _, event := range events {
		log := map[string]interface{}{
			"timestamp": event.Timestamp.UnixMicro(),
			"fields": []map[string]interface{}{
				{
					"key":   "event",
					"value": event.Name,
				},
			},
		}
		
		for k, v := range event.Attributes {
			log["fields"] = append(log["fields"].([]map[string]interface{}), map[string]interface{}{
				"key":   k,
				"value": v,
			})
		}
		
		logs = append(logs, log)
	}
	
	return logs
}

// Shutdown shuts down the exporter
func (e *JaegerExporter) Shutdown() error {
	// Clean shutdown
	return nil
}

// TracingMetrics provides metrics about the tracing system
type TracingMetrics struct {
	SpansCreated   int64         `json:"spans_created"`
	SpansCompleted int64         `json:"spans_completed"`
	SpansExported  int64         `json:"spans_exported"`
	TracesActive   int           `json:"traces_active"`
	SamplingRate   float64       `json:"sampling_rate"`
	ErrorRate      float64       `json:"error_rate"`
	AverageLatency time.Duration `json:"average_latency"`
}

// GetMetrics returns tracing system metrics
func (ts *TracingSystem) GetMetrics() *TracingMetrics {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	metrics := &TracingMetrics{
		TracesActive: len(ts.traces),
	}
	
	// Calculate metrics from traces
	totalSpans := 0
	completedSpans := 0
	errorSpans := 0
	totalDuration := time.Duration(0)
	
	for _, trace := range ts.traces {
		totalSpans += trace.SpanCount
		if trace.Status == StatusCodeError {
			errorSpans += trace.ErrorCount
		}
		if trace.Duration > 0 {
			completedSpans++
			totalDuration += trace.Duration
		}
	}
	
	metrics.SpansCreated = int64(totalSpans)
	metrics.SpansCompleted = int64(completedSpans)
	
	if totalSpans > 0 {
		metrics.ErrorRate = float64(errorSpans) / float64(totalSpans)
	}
	
	if completedSpans > 0 {
		metrics.AverageLatency = totalDuration / time.Duration(completedSpans)
	}
	
	return metrics
}