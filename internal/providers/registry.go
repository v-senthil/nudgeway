// Package providers wires each provider adapter into the runtime via a
// self-registering registry. Nothing in internal/domain or
// internal/application imports this package; wiring happens in cmd/server.
package providers

import (
	"fmt"
	"sync"
)

// Kind categorises what a registered provider does. A single provider may
// register under multiple kinds when appropriate (e.g. WhatsApp registers
// under both Channel and Calling once the calling adapter lands).
type Kind string

// Registered provider kinds.
const (
	KindChannel   Kind = "channel"
	KindTicketing Kind = "ticketing"
	KindBot       Kind = "bot"
	KindAI        Kind = "ai"
	KindCalling   Kind = "calling"
)

// Descriptor is what an adapter registers with the registry at init time.
type Descriptor struct {
	Kind Kind
	Key  string // stable identifier: "whatsapp", "zoho_desk", "openai", ...
	Name string // human-readable
}

var (
	mu       sync.RWMutex
	registry = map[Kind]map[string]Descriptor{}
)

// Register makes a provider discoverable. Panics on duplicate (kind, key).
func Register(d Descriptor) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[d.Kind]; !ok {
		registry[d.Kind] = map[string]Descriptor{}
	}
	if _, dup := registry[d.Kind][d.Key]; dup {
		panic(fmt.Sprintf("provider already registered: %s/%s", d.Kind, d.Key))
	}
	registry[d.Kind][d.Key] = d
}

// Lookup returns a Descriptor or false.
func Lookup(kind Kind, key string) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := registry[kind][key]
	return d, ok
}

// List returns all descriptors for a kind.
func List(kind Kind) []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Descriptor, 0, len(registry[kind]))
	for _, d := range registry[kind] {
		out = append(out, d)
	}
	return out
}
