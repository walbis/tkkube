package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DefaultEventPublisher provides a thread-safe implementation of EventPublisher
type DefaultEventPublisher struct {
	events    []Event
	exporters []EventExporter
	config    *MonitoringConfig
	logger    Logger
	mu        sync.RWMutex
	buffer    chan Event
	running   bool
	stopChan  chan struct{}
}

// NewEventPublisher creates a new event publisher
func NewEventPublisher(config *MonitoringConfig, logger Logger) *DefaultEventPublisher {
	bufferSize := 1000
	if config != nil && config.MaxEventsBuffer > 0 {
		bufferSize = config.MaxEventsBuffer
	}
	
	return &DefaultEventPublisher{
		events:    make([]Event, 0),
		exporters: make([]EventExporter, 0),
		config:    config,
		logger:    logger,
		buffer:    make(chan Event, bufferSize),
		stopChan:  make(chan struct{}),
	}
}

// PublishSystemEvent publishes a system event
func (ep *DefaultEventPublisher) PublishSystemEvent(event SystemEvent) error {
	return ep.PublishEvent(event)
}

// PublishBusinessEvent publishes a business event
func (ep *DefaultEventPublisher) PublishBusinessEvent(event BusinessEvent) error {
	return ep.PublishEvent(event)
}

// PublishErrorEvent publishes an error event
func (ep *DefaultEventPublisher) PublishErrorEvent(event ErrorEvent) error {
	return ep.PublishEvent(event)
}

// PublishEvent publishes a generic event
func (ep *DefaultEventPublisher) PublishEvent(event Event) error {
	if ep.config != nil && !ep.config.EventsEnabled {
		return nil // Events disabled
	}
	
	// Add to buffer for processing
	select {
	case ep.buffer <- event:
		// Successfully added to buffer
	default:
		// Buffer full, try to process immediately or drop
		ep.logger.Warn("event_buffer_full", map[string]interface{}{
			"event_id":   event.GetID(),
			"event_type": event.GetType(),
		})
		return fmt.Errorf("event buffer full")
	}
	
	// Add to local storage
	ep.mu.Lock()
	ep.events = append(ep.events, event)
	
	// Apply retention if configured
	if ep.config != nil && ep.config.EventsRetention > 0 {
		cutoff := time.Now().Add(-ep.config.EventsRetention)
		ep.events = ep.filterEventsByTime(ep.events, cutoff)
	}
	ep.mu.Unlock()
	
	return nil
}

// SetExporter sets an event exporter
func (ep *DefaultEventPublisher) SetExporter(exporter EventExporter) error {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	ep.exporters = append(ep.exporters, exporter)
	return nil
}

// Close closes the event publisher
func (ep *DefaultEventPublisher) Close() error {
	if ep.running {
		close(ep.stopChan)
		ep.running = false
	}
	
	// Close all exporters
	var errors []error
	for _, exporter := range ep.exporters {
		if err := exporter.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("errors closing exporters: %v", errors)
	}
	
	return nil
}

// Start starts the event processing loop
func (ep *DefaultEventPublisher) Start(ctx context.Context) error {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	if ep.running {
		return fmt.Errorf("event publisher already running")
	}
	
	ep.running = true
	go ep.eventProcessingLoop(ctx)
	
	return nil
}

// eventProcessingLoop processes events from the buffer
func (ep *DefaultEventPublisher) eventProcessingLoop(ctx context.Context) {
	exportInterval := 10 * time.Second
	if ep.config != nil && ep.config.ExportInterval > 0 {
		exportInterval = ep.config.ExportInterval
	}
	
	ticker := time.NewTicker(exportInterval)
	defer ticker.Stop()
	
	var eventBatch []Event
	
	for {
		select {
		case <-ctx.Done():
			// Export remaining events before shutdown
			if len(eventBatch) > 0 {
				ep.exportEvents(ctx, eventBatch)
			}
			return
		case <-ep.stopChan:
			// Export remaining events before shutdown
			if len(eventBatch) > 0 {
				ep.exportEvents(ctx, eventBatch)
			}
			return
		case event := <-ep.buffer:
			eventBatch = append(eventBatch, event)
			
			// Export batch if it reaches a certain size
			if len(eventBatch) >= 100 {
				ep.exportEvents(ctx, eventBatch)
				eventBatch = nil
			}
		case <-ticker.C:
			// Export batch on timer
			if len(eventBatch) > 0 {
				ep.exportEvents(ctx, eventBatch)
				eventBatch = nil
			}
		}
	}
}

// exportEvents exports a batch of events to all configured exporters
func (ep *DefaultEventPublisher) exportEvents(ctx context.Context, events []Event) {
	if len(ep.exporters) == 0 || !ep.shouldExport() {
		return
	}
	
	for _, exporter := range ep.exporters {
		if err := exporter.ExportEvents(ctx, events); err != nil {
			ep.logger.Error("event_export_failed", map[string]interface{}{
				"exporter": fmt.Sprintf("%T", exporter),
				"error":    err.Error(),
				"events_count": len(events),
			})
		}
	}
}

// shouldExport checks if events should be exported based on configuration
func (ep *DefaultEventPublisher) shouldExport() bool {
	return ep.config == nil || ep.config.ExportEnabled
}

// filterEventsByTime filters events to keep only those after the cutoff time
func (ep *DefaultEventPublisher) filterEventsByTime(events []Event, cutoff time.Time) []Event {
	filtered := make([]Event, 0)
	for _, event := range events {
		if event.GetTimestamp().After(cutoff) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetEvents returns all stored events
func (ep *DefaultEventPublisher) GetEvents() []Event {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	
	events := make([]Event, len(ep.events))
	copy(events, ep.events)
	return events
}

// GetEventsByType returns events of a specific type
func (ep *DefaultEventPublisher) GetEventsByType(eventType string) []Event {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	
	var filtered []Event
	for _, event := range ep.events {
		if event.GetType() == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetEventsByTimeRange returns events within a time range
func (ep *DefaultEventPublisher) GetEventsByTimeRange(start, end time.Time) []Event {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	
	var filtered []Event
	for _, event := range ep.events {
		timestamp := event.GetTimestamp()
		if timestamp.After(start) && timestamp.Before(end) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// ConsoleEventExporter exports events to console/logs
type ConsoleEventExporter struct {
	logger Logger
	config ExporterConfig
}

// NewConsoleEventExporter creates a new console event exporter
func NewConsoleEventExporter(logger Logger) *ConsoleEventExporter {
	return &ConsoleEventExporter{
		logger: logger,
	}
}

// ExportEvents exports events to console
func (ce *ConsoleEventExporter) ExportEvents(ctx context.Context, events []Event) error {
	for _, event := range events {
		ce.logger.Info("exported_event", map[string]interface{}{
			"event_id":   event.GetID(),
			"event_type": event.GetType(),
			"timestamp":  event.GetTimestamp(),
			"metadata":   event.GetMetadata(),
		})
	}
	return nil
}

// Configure configures the console exporter
func (ce *ConsoleEventExporter) Configure(config ExporterConfig) error {
	ce.config = config
	return nil
}

// Close closes the console exporter
func (ce *ConsoleEventExporter) Close() error {
	return nil
}

// FileEventExporter exports events to a file
type FileEventExporter struct {
	filePath string
	config   ExporterConfig
	mu       sync.Mutex
}

// NewFileEventExporter creates a new file event exporter
func NewFileEventExporter(filePath string) *FileEventExporter {
	return &FileEventExporter{
		filePath: filePath,
	}
}

// ExportEvents exports events to file
func (fe *FileEventExporter) ExportEvents(ctx context.Context, events []Event) error {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	
	// This would typically write to a file
	// For now, just return success
	return nil
}

// Configure configures the file exporter
func (fe *FileEventExporter) Configure(config ExporterConfig) error {
	fe.config = config
	if endpoint, ok := config.Settings["file_path"].(string); ok {
		fe.filePath = endpoint
	}
	return nil
}

// Close closes the file exporter
func (fe *FileEventExporter) Close() error {
	return nil
}

// HTTPEventExporter exports events to an HTTP endpoint
type HTTPEventExporter struct {
	endpoint string
	config   ExporterConfig
}

// NewHTTPEventExporter creates a new HTTP event exporter
func NewHTTPEventExporter(endpoint string) *HTTPEventExporter {
	return &HTTPEventExporter{
		endpoint: endpoint,
	}
}

// ExportEvents exports events to HTTP endpoint
func (he *HTTPEventExporter) ExportEvents(ctx context.Context, events []Event) error {
	// This would typically POST events to an HTTP endpoint
	// For now, just return success
	return nil
}

// Configure configures the HTTP exporter
func (he *HTTPEventExporter) Configure(config ExporterConfig) error {
	he.config = config
	he.endpoint = config.Endpoint
	return nil
}

// Close closes the HTTP exporter
func (he *HTTPEventExporter) Close() error {
	return nil
}

// Event factory functions for common event types

// CreateSystemStartEvent creates a system start event
func CreateSystemStartEvent(component string) SystemEvent {
	return SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "system_start",
		Component: component,
		Action:    "start",
		Status:    "success",
	}
}

// CreateSystemStopEvent creates a system stop event
func CreateSystemStopEvent(component string) SystemEvent {
	return SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "system_stop",
		Component: component,
		Action:    "stop",
		Status:    "success",
	}
}

// CreateConfigurationLoadedEvent creates a configuration loaded event
func CreateConfigurationLoadedEvent(component string, duration time.Duration) SystemEvent {
	return SystemEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Type:      "configuration_loaded",
		Component: component,
		Action:    "load_config",
		Status:    "success",
		Duration:  duration,
	}
}

// CreateBackupCompletedEvent creates a backup completed business event
func CreateBackupCompletedEvent(backupID string, resourceCount int, sizeBytes int64) BusinessEvent {
	return BusinessEvent{
		ID:          generateEventID(),
		Timestamp:   time.Now(),
		Type:        "backup_completed",
		BusinessID:  backupID,
		Description: "Kubernetes backup completed successfully",
		Impact:      "positive",
		Metrics: map[string]float64{
			"resource_count": float64(resourceCount),
			"size_bytes":     float64(sizeBytes),
		},
		Tags: map[string]string{
			"operation": "backup",
			"status":    "completed",
		},
	}
}

// CreateGitOpsGenerationEvent creates a GitOps generation business event
func CreateGitOpsGenerationEvent(generationID string, commitHash string) BusinessEvent {
	return BusinessEvent{
		ID:          generateEventID(),
		Timestamp:   time.Now(),
		Type:        "gitops_generation",
		BusinessID:  generationID,
		Description: "GitOps manifests generated and committed",
		Impact:      "positive",
		Tags: map[string]string{
			"operation":   "gitops_generation",
			"commit_hash": commitHash,
		},
	}
}

// CreateErrorEvent creates an error event
func CreateErrorEvent(component, operation string, err error, severity string) ErrorEvent {
	return ErrorEvent{
		ID:          generateEventID(),
		Timestamp:   time.Now(),
		Component:   component,
		Operation:   operation,
		Error:       err,
		Severity:    severity,
		Recoverable: severity != "critical" && severity != "fatal",
		Context: map[string]interface{}{
			"error_message": err.Error(),
		},
	}
}