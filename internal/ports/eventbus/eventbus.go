// Package eventbus is the port for publishing canonical domain events.
// Implementations live in internal/events (in-proc fan-out) and
// internal/infrastructure/redis (Redis Streams for cross-node fan-out).
package eventbus

import (
	"context"

	"github.com/v-senthil/nudgeway/internal/domain/events"
)

// Publisher accepts events and returns after the event has been durably
// enqueued (for cross-node) or synchronously dispatched (for in-proc).
type Publisher interface {
	Publish(ctx context.Context, evt events.Envelope) error
}

// Subscriber registers a Handler for a specific event Type. Handlers should
// return quickly and offload heavy work to workers.
type Subscriber interface {
	Subscribe(t events.Type, h events.Handler)
}
