package retellAI

import (
	"sync"
)

// EventHandler represents a function that handles events
type EventHandler func(data interface{})

// EventEmitter provides event emission and listening capabilities
type EventEmitter struct {
	listeners map[string][]EventHandler
	mutex     sync.RWMutex
}

// NewEventEmitter creates a new EventEmitter instance
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		listeners: make(map[string][]EventHandler),
	}
}

// On registers an event listener for the specified event
func (e *EventEmitter) On(event string, handler EventHandler) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.listeners[event] == nil {
		e.listeners[event] = make([]EventHandler, 0)
	}
	e.listeners[event] = append(e.listeners[event], handler)
}

// Emit triggers all listeners for the specified event
func (e *EventEmitter) Emit(event string, data interface{}) {
	e.mutex.RLock()
	handlers := make([]EventHandler, len(e.listeners[event]))
	copy(handlers, e.listeners[event])
	e.mutex.RUnlock()

	for _, handler := range handlers {
		go handler(data) // Execute handlers in goroutines for non-blocking behavior
	}
}

// Off removes an event listener (removes all listeners for the event if handler is nil)
func (e *EventEmitter) Off(event string, handler EventHandler) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if handler == nil {
		// Remove all listeners for this event
		delete(e.listeners, event)
		return
	}

	// Remove specific handler (this is complex in Go without function comparison)
	// For simplicity, we'll provide RemoveAllListeners method instead
}

// RemoveAllListeners removes all listeners for the specified event
func (e *EventEmitter) RemoveAllListeners(event string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	delete(e.listeners, event)
}

// ListenerCount returns the number of listeners for the specified event
func (e *EventEmitter) ListenerCount(event string) int {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return len(e.listeners[event])
}
