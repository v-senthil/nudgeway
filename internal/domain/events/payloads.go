package events

import "time"

// MessageReceivedPayload is the canonical body of a MessageReceived envelope.
// It is provider-agnostic — Meta / Twilio / Telegram all get flattened onto
// this shape by their respective adapters.
type MessageReceivedPayload struct {
	// Provider is the stable provider key ("whatsapp", "twilio", …).
	Provider string
	// Channel is the channel family ("whatsapp", "sms", "email", …).
	Channel string
	// BusinessEndpointExternalID identifies which of the tenant's endpoints
	// received the message (e.g. WhatsApp phone_number_id).
	BusinessEndpointExternalID string
	// ProviderMessageID is the provider-native message id (e.g. wamid).
	// Idempotency on receive uses this.
	ProviderMessageID string
	// From is the canonical sender identity (E.164 phone, email, wa_id).
	From string
	// FromDisplayName is the profile name reported by the provider.
	FromDisplayName string
	// To is the canonical recipient identity (the business endpoint address).
	To string
	// MessageType is a canonical message.Type value (kept as string here
	// to avoid a cyclical import: events → message → events).
	MessageType string
	// Payload is one of the payload shapes from internal/domain/message
	// (TextPayload, MediaPayload, InteractivePayload, LocationPayload, …).
	// Consumers type-switch on the concrete type.
	Payload any
	// Timestamp is when the provider says the customer sent the message.
	Timestamp time.Time
	// ContextProviderMessageID references a prior message this one replies to
	// (e.g. a button tap or list reply). Empty when not a reply.
	ContextProviderMessageID string
	// Raw preserves the exact provider payload for TypeUnknown / debug.
	Raw map[string]any
}

// MessageStatusPayload is the body of MessageSent / MessageDelivered /
// MessageRead / MessageFailed envelopes.
type MessageStatusPayload struct {
	Provider          string
	Channel           string
	ProviderMessageID string
	Recipient         string
	Status            string    // "sent" | "delivered" | "read" | "failed"
	Timestamp         time.Time
	// ErrorCode / ErrorMessage populated when Status == "failed".
	ErrorCode    string
	ErrorMessage string
	// Raw preserves the source payload for debug/analytics.
	Raw map[string]any
}
