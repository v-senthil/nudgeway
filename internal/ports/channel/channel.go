// Package channel is the port for communication providers (WhatsApp,
// Telegram, Instagram, Messenger, LINE, WeChat, SMS, Email …).
//
// Every ChannelProvider adapter maps its native surface into the canonical
// domain via inbound events + canonical send requests. Nothing in the
// application layer imports adapter packages directly.
package channel

import (
	"context"

	"github.com/fullwa/fullwa/internal/domain/events"
)

// Capabilities declares what a specific ChannelProvider instance can do,
// so the UI can enable/disable features per integration.
type Capabilities struct {
	SendText        bool
	SendMedia       bool
	SendTemplate    bool
	ReceiveMessages bool
	Templates       bool
	Groups          bool
	Calls           bool
	Flows           bool
}

// SendRequest is the canonical outbound message shape.
type SendRequest struct {
	OrganizationID string
	IntegrationID  string
	To             string
	MessageType    string
	Body           []byte
	IdempotencyKey string
}

// SendResult carries the provider's acknowledgement of a send.
type SendResult struct {
	ProviderMessageID string
	AcceptedAt        int64
}

// HealthStatus describes the provider's current health from the integration's POV.
type HealthStatus struct {
	OK      bool
	Message string
}

// Provider is implemented by every ChannelProvider adapter.
type Provider interface {
	SendMessage(ctx context.Context, req SendRequest) (SendResult, error)
	ParseWebhook(ctx context.Context, headers map[string][]string, body []byte) ([]events.Envelope, error)
	Capabilities() Capabilities
	HealthCheck(ctx context.Context) HealthStatus
	// MarkAsRead asks the provider to signal to the customer that the
	// business has read the inbound message identified by providerMessageID
	// (e.g. Meta's wamid). Adapters that do not support a read receipt
	// return a nil error — the caller treats absence of support as a no-op.
	MarkAsRead(ctx context.Context, providerMessageID string) error
}
