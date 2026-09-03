// Package events defines the canonical, provider-agnostic event types that
// flow through the internal event bus. Providers translate their native
// payloads into these events at the adapter boundary; nothing downstream
// knows which provider produced them.
package events
