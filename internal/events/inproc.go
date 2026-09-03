package events

import (
	"context"
	"sync"

	dev "github.com/fullwa/fullwa/internal/domain/events"
)

// InProc is a synchronous, in-process implementation of the eventbus port.
// It fans out to all registered handlers for the event's Type. Errors from
// individual handlers are collected but do not stop other handlers from
// running — the bus's job is delivery, not policy.
type InProc struct {
	mu       sync.RWMutex
	handlers map[dev.Type][]dev.Handler
}

// NewInProc returns a ready InProc bus.
func NewInProc() *InProc { return &InProc{handlers: map[dev.Type][]dev.Handler{}} }

// Subscribe registers a Handler for a Type.
func (b *InProc) Subscribe(t dev.Type, h dev.Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[t] = append(b.handlers[t], h)
}

// Publish fans out to every registered handler. Callers should ensure the
// context is not aborted mid-fanout for events that must reach all handlers.
func (b *InProc) Publish(_ context.Context, evt dev.Envelope) error {
	b.mu.RLock()
	hs := append([]dev.Handler(nil), b.handlers[evt.Type]...)
	b.mu.RUnlock()
	var firstErr error
	for _, h := range hs {
		if err := h(evt); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
