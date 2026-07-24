package pxpipe

import (
	"sync"
	"time"
)

type Event struct {
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

type Manager struct {
	mu      sync.RWMutex
	loaded  bool
	events  []Event
	install string
}

func New() *Manager { return &Manager{} }

func (m *Manager) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]any{"installed": false, "loaded": m.loaded, "available": false, "module": "go-fail-open"}
}

func (m *Manager) Start() map[string]any {
	m.mu.Lock()
	m.loaded = true
	m.mu.Unlock()
	return m.Status()
}

func (m *Manager) Stop() bool {
	m.mu.Lock()
	wasLoaded := m.loaded
	m.loaded = false
	m.mu.Unlock()
	return wasLoaded
}

func (m *Manager) AddEvent(data map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, Event{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Data: data})
	if len(m.events) > 1000 {
		m.events = m.events[len(m.events)-1000:]
	}
}

func (m *Manager) Events(limit int) []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit < 1 || limit > len(m.events) {
		limit = len(m.events)
	}
	result := append([]Event(nil), m.events[len(m.events)-limit:]...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}
