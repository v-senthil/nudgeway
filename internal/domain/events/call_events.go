package events

import "time"

// Call event types. The events.Type constants CallStarted / CallEnded /
// CallRecordingCreated already exist in events.go for schema stability;
// this file adds the more granular event kinds the WhatsApp Calling
// pipeline emits per webhook.
//
// New Type constants live here (additive to the events.Type namespace) so
// consumers can subscribe to specific phases of the call lifecycle without
// destructuring the payload.
const (
	// CallInitiated fires when an outbound call has been persisted and
	// enqueued but not yet placed with the provider.
	CallInitiated Type = "call.initiated"
	// CallRinging fires when the provider reports the callee's device is
	// ringing (business-initiated) or when a new inbound call has been
	// surfaced by webhook.
	CallRinging Type = "call.ringing"
	// CallAnswered fires when the callee accepts the call.
	CallAnswered Type = "call.answered"
	// CallEnded fires when a call transitions to a terminal COMPLETED status.
	// (The pre-existing CallEnded const in events.go is retained as an
	// alias; new subscribers should prefer this file's constants for
	// discoverability. Both point at the same string value.)
	// Left named "call.ended.v2" here to avoid a duplicate declaration.
	CallEndedDetailed Type = "call.ended.detailed"
	// CallFailed fires when the call transitions to a terminal FAILED /
	// MISSED / DECLINED / NO_ANSWER status.
	CallFailed Type = "call.failed"
)

// CallEventPayload is the canonical body of every Call* envelope. The
// payload is intentionally flat — subscribers pick the fields they care
// about without destructuring nested structs.
type CallEventPayload struct {
	// CallID is the canonical Call.ID (ULID string).
	CallID string
	// Provider is the registry key of the adapter ("whatsapp", ...).
	Provider string
	// ProviderCallID is the provider-native call id.
	ProviderCallID string
	// BusinessEndpointExternalID is the phone number / endpoint on our side.
	BusinessEndpointExternalID string
	// Direction is "inbound" | "outbound".
	Direction string
	// Status is the current canonical call status.
	Status string
	// From is the caller identity (E.164 phone or BSUID).
	From string
	// To is the callee identity.
	To string
	// FromUserID is the caller's BSUID when known.
	FromUserID string
	// ToUserID is the callee's BSUID when known.
	ToUserID string
	// StartedAt is when the call was placed / rang.
	StartedAt time.Time
	// AnsweredAt is when the call was picked up. Zero when not answered.
	AnsweredAt time.Time
	// EndedAt is when the call ended. Zero when still in progress.
	EndedAt time.Time
	// DurationSeconds is the wall-clock duration in seconds; zero when the
	// call never connected.
	DurationSeconds int
	// HangupReason is a short provider-specific hangup code / reason.
	HangupReason string
	// RecordingURL points at the recording (short-lived provider URL or
	// self-hosted /api/v1/media key).
	RecordingURL string
	// TranscriptionRef is the provider media id for the transcript JSON.
	TranscriptionRef string
	// ErrorCode / ErrorMessage populated when Status is a failed variant.
	ErrorCode    string
	ErrorMessage string
	// Timestamp is the wall-clock timestamp reported by the provider for
	// the underlying webhook event.
	Timestamp time.Time
	// Raw preserves the provider payload for debug / analytics.
	Raw map[string]any
}
