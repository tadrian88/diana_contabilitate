// Package llmusagetest provides an in-memory usage recorder for tests.
package llmusagetest

import (
	"context"
	"sync"

	"diana-contabilitate/backend/internal/llmusage"
)

type MemoryRecorder struct {
	mu     sync.Mutex
	events []llmusage.Event
}

func (m *MemoryRecorder) RecordLLMCall(_ context.Context, event llmusage.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *MemoryRecorder) Events() []llmusage.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]llmusage.Event(nil), m.events...)
}
