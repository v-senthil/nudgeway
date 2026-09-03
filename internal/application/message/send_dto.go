package message

// SendRequest is the canonical DTO consumed by SendService.RequestSend.
// It is populated by the REST handler after validating the JSON body against
// the OpenAPI schema. The application layer is provider-agnostic — nothing
// in this shape mentions WhatsApp / Meta / Twilio / etc.
type SendRequest struct {
	// OrgID is the authenticated caller's organization (from the session
	// principal). NEVER trusted from the client body.
	OrgID string
	// ActorUserID is the authenticated caller. Logged for audit.
	ActorUserID string
	// ConversationID names the conversation to append the message to. The
	// service resolves session/contact/endpoint/integration from this ID and
	// enforces the (OrgID, ConversationID) match key.
	ConversationID string
	// Type is a canonical message.Type value ("text", "template", "image", ...).
	Type string
	// Payload is a JSON-encoded blob matching one of the shapes in
	// internal/domain/message/payload.go (TextPayload, MediaPayload,
	// TemplatePayload, ...). It is forwarded to the provider adapter verbatim.
	Payload []byte
	// IdempotencyKey optionally deduplicates concurrent send attempts and is
	// echoed to the provider (e.g. Meta's biz_opaque_callback_data) so status
	// webhooks can be correlated without a client round-trip.
	IdempotencyKey string
	// RequestID is the incoming HTTP request id, propagated for logging.
	RequestID string
	// CorrelationID is the correlation id for downstream events. Defaults to
	// the request id when empty.
	CorrelationID string
}

// SendResponse is the DTO returned by SendService.RequestSend on success.
type SendResponse struct {
	// MessageID is the canonical id assigned to the newly-created row.
	MessageID string
	// Status is always "queued" for a successful RequestSend — the message
	// is durably persisted and enqueued but not yet dispatched to the
	// provider.
	Status string
}

// SendJobPayload is the JSON envelope written to the queue.Enqueuer lane
// "message.send" by RequestSend and decoded by the worker before calling
// SendService.ProcessSend.
type SendJobPayload struct {
	MessageID      string `json:"message_id"`
	OrgID          string `json:"org_id"`
	IntegrationID  string `json:"integration_id"`
	ProviderKey    string `json:"provider_key"`
	Recipient      string `json:"recipient"`
	Type           string `json:"type"`
	Payload        []byte `json:"payload"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	CorrelationID  string `json:"correlation_id,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
}
