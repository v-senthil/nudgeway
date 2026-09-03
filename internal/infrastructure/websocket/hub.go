package websocket

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"nhooyr.io/websocket"
)

// Hub is the per-node WebSocket fan-out. It owns a set of Rooms keyed by
// org ID; each Room in turn owns the Clients currently connected for that
// tenant. All exported methods are safe for concurrent use.
//
// The Hub is intentionally node-local. Cross-node fan-out (Redis pub/sub or
// Kafka) is a later concern that will publish into every node's Hub; the
// contract exposed to the rest of the app stays the same.
type Hub struct {
	logger *slog.Logger

	mu    sync.RWMutex
	rooms map[string]*Room

	broadcasts atomic.Uint64
	drops      atomic.Uint64

	closed atomic.Bool
}

// NewHub returns an empty Hub. logger may be nil, in which case the default
// slog logger is used.
func NewHub(logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		logger: logger,
		rooms:  map[string]*Room{},
	}
}

// Logger returns the logger the Hub was constructed with. Handy for handlers
// that want to reuse the Hub's logger without wiring their own.
func (h *Hub) Logger() *slog.Logger { return h.logger }

// Room returns the Room for orgID, creating it if it does not exist. Empty
// rooms are retained for pointer stability.
func (h *Hub) Room(orgID string) *Room {
	h.mu.RLock()
	r, ok := h.rooms[orgID]
	h.mu.RUnlock()
	if ok {
		return r
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok = h.rooms[orgID]; ok {
		return r
	}
	r = newRoom(orgID)
	h.rooms[orgID] = r
	return r
}

// Broadcast fans payload out to every Client currently in the orgID's Room.
// The send is non-blocking per client: if a client's send queue is full the
// frame is dropped and the Hub-level drop counter is incremented. This is
// the mechanism that stops one slow consumer from stalling the whole fan-out
// path.
//
// No goroutine is spawned per broadcast. The Room membership is snapshotted
// under a read lock, then the send loop runs on the caller's goroutine.
func (h *Hub) Broadcast(orgID string, payload []byte) {
	if h.closed.Load() {
		return
	}
	h.broadcasts.Add(1)
	r := h.Room(orgID)
	for _, c := range r.snapshot() {
		before := c.Dropped()
		c.Enqueue(payload)
		if c.Dropped() > before {
			h.drops.Add(1)
			h.logger.Warn("ws broadcast dropped: client send queue full",
				slog.String("org_id", orgID),
				slog.String("user_id", c.UserID()),
			)
		}
	}
}

// Stats snapshots hub-wide counters. Useful for /metrics and diagnostics.
func (h *Hub) Stats() (broadcasts, drops uint64, rooms int) {
	h.mu.RLock()
	rooms = len(h.rooms)
	h.mu.RUnlock()
	return h.broadcasts.Load(), h.drops.Load(), rooms
}

// Close gracefully terminates every Client in every Room. Once Close returns,
// subsequent Broadcast calls are no-ops. The passed ctx is only advisory:
// individual Client.Close calls do not honour it (they must always shut down
// their conn), but a cancelled ctx short-circuits the shutdown loop.
func (h *Hub) Close(ctx context.Context) {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	h.mu.RUnlock()

	for _, r := range rooms {
		if ctx.Err() != nil {
			return
		}
		for _, c := range r.snapshot() {
			c.Close(websocket.StatusGoingAway, "hub shutdown")
		}
	}
}
