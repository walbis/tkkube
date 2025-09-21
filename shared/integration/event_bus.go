package integration

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	sharedconfig "shared-config/config"
)

// EventBus handles event coordination between components
type EventBus struct {
	subscribers map[string][]EventHandler
	mu          sync.RWMutex
	config      *sharedconfig.SharedConfig
}

// NewEventBus creates a new event bus
func NewEventBus(config *sharedconfig.SharedConfig) *EventBus {
	return &EventBus{
		subscribers: make(map[string][]EventHandler),
		config:      config,
	}
}

// Subscribe registers an event handler for a specific event type
func (eb *EventBus) Subscribe(eventType string, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.subscribers[eventType] == nil {
		eb.subscribers[eventType] = make([]EventHandler, 0)
	}
	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)

	log.Printf("Event handler registered for event type: %s", eventType)
}

// Publish sends an event to all registered handlers
func (eb *EventBus) Publish(ctx context.Context, event *IntegrationEvent) error {
	eb.mu.RLock()
	handlers := eb.subscribers[event.Type]
	eb.mu.RUnlock()

	if len(handlers) == 0 {
		log.Printf("No handlers registered for event type: %s", event.Type)
		return nil
	}

	// Execute handlers concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for _, handler := range handlers {
		wg.Add(1)
		go func(h EventHandler) {
			defer wg.Done()
			
			timeout := 30 * time.Second // default fallback
			if eb.config != nil {
				timeout = eb.config.Timeouts.EventHandlerTimeout
			}
			handlerCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			if err := h(handlerCtx, event); err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
				log.Printf("Event handler error for %s: %v", event.Type, err)
			}
		}(handler)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("event handling errors: %v", errors)
	}

	log.Printf("Event published successfully: %s (ID: %s)", event.Type, event.ID)
	return nil
}

// GetSubscriberCount returns the number of subscribers for an event type
func (eb *EventBus) GetSubscriberCount(eventType string) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers[eventType])
}

// GetEventTypes returns all registered event types
func (eb *EventBus) GetEventTypes() []string {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	types := make([]string, 0, len(eb.subscribers))
	for eventType := range eb.subscribers {
		types = append(types, eventType)
	}
	return types
}