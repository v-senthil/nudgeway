package webhook

import (
	"sync"

	"github.com/fullwa/fullwa/internal/ports/channel"
)

// providerMu guards the channel-provider registry. Registration is expected
// to happen once at process boot from cmd/server; runtime writes are safe
// but discouraged.
var (
	providerMu       sync.RWMutex
	channelProviders = map[string]channel.Provider{}
)

// RegisterProvider makes a channel.Provider instance discoverable by its
// registry key. cmd/server calls this once per configured adapter after
// constructing the concrete provider. Re-registering the same key
// overwrites — useful when swapping instances in tests, harmless in
// production where the key is stable.
func RegisterProvider(key string, p channel.Provider) {
	if key == "" || p == nil {
		return
	}
	providerMu.Lock()
	channelProviders[key] = p
	providerMu.Unlock()
}

// UnregisterProvider removes a provider from the runtime registry. Intended
// for test cleanup; production wiring keeps providers for the process
// lifetime.
func UnregisterProvider(key string) {
	providerMu.Lock()
	delete(channelProviders, key)
	providerMu.Unlock()
}

// ProviderLookup resolves a provider-registry key ("whatsapp", "twilio", ...)
// to its runtime channel.Provider implementation. Returns ok=false when the
// key has not been registered — callers must treat that as a permanent
// failure for the current delivery.
//
// This function is dependency-safe for the application layer: it exposes
// only the port type channel.Provider, so importers do not gain a
// transitive dependency on any provider adapter package.
func ProviderLookup(key string) (channel.Provider, bool) {
	providerMu.RLock()
	defer providerMu.RUnlock()
	p, ok := channelProviders[key]
	return p, ok
}
