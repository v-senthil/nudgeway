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
	// May become empty once WhatsApp completes the BSUID / username
	// rollout — always prefer FromUserID when populated.
	From string
	// FromUserID is the provider-native durable identity (WhatsApp BSUID:
	// business-scoped user id). Present when the provider supports it;
	// this is the identity to key on going forward. Format
	// <CC>.<alnum-up-to-128>.
	FromUserID string
	// FromParentUserID is the WhatsApp parent BSUID for managed
	// businesses enrolled in a parent BSUID account. Usable across every
	// portfolio in the parent account.
	FromParentUserID string
	// FromUsername is the WhatsApp username the customer has adopted, if
	// any. Optional display-only field.
	FromUsername string
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
	// ConversationID is the canonical conversation id set by the
	// InboundService right before publish (the parser cannot know it —
	// it's assigned when we upsert the domain Conversation row). Enables
	// downstream subscribers (WebSocket bridge, automation engine) to
	// route updates to a specific conversation without a second DB hit.
	ConversationID string
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
	// RecipientUserID is the WhatsApp BSUID Meta echoes back on delivered
	// and read status callbacks. Present regardless of whether the
	// original send targeted a phone number or a BSUID (except failed
	// status callbacks, which omit it when the send was phone-addressed).
	RecipientUserID string
	Status          string    // "sent" | "delivered" | "read" | "failed"
	Timestamp       time.Time
	// ErrorCode / ErrorMessage populated when Status == "failed".
	ErrorCode    string
	ErrorMessage string
	// Raw preserves the source payload for debug/analytics.
	Raw map[string]any
}
