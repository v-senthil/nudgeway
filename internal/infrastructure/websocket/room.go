package websocket

import "sync"

// Room is the fan-out target for a single organization. It holds the set of
// live Client connections currently subscribed on this node and offers
// concurrent-safe add / remove / broadcast primitives.
//
// A Room is created lazily by Hub.Room and lives until the Hub is closed. An
// empty Room is retained rather than deleted so pointer identity remains
// stable for callers that hold a Room reference across reconnects.
type Room struct {
	orgID string

	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// newRoom constructs an empty Room bound to orgID.
func newRoom(orgID string) *Room {
	return &Room{
		orgID:   orgID,
		clients: map[*Client]struct{}{},
	}
}

// OrgID returns the organization ID this Room belongs to.
func (r *Room) OrgID() string { return r.orgID }

// Add registers c as a member of the Room. Safe to call concurrently.
func (r *Room) Add(c *Client) {
	r.mu.Lock()
	r.clients[c] = struct{}{}
	r.mu.Unlock()
}

// Remove deregisters c from the Room. It is safe to call for a client that
// is not currently a member.
func (r *Room) Remove(c *Client) {
	r.mu.Lock()
	delete(r.clients, c)
	r.mu.Unlock()
}

// Len returns the current number of subscribed clients.
func (r *Room) Len() int {
	r.mu.RLock()
	n := len(r.clients)
	r.mu.RUnlock()
	return n
}

// snapshot returns a copy of the current membership. Used by broadcast so the
// send loop does not hold the room lock while pushing bytes onto client
// channels (which may briefly block on the per-client mutex).
func (r *Room) snapshot() []*Client {
	r.mu.RLock()
	out := make([]*Client, 0, len(r.clients))
	for c := range r.clients {
		out = append(out, c)
	}
	r.mu.RUnlock()
	return out
}
