// Package calling is the provider-agnostic port for voice-calling backends.
// The core domain and application layers depend on this package; concrete
// adapters live under internal/providers/<name>/. No provider SDK, no
// provider-specific vocabulary leaks past this file.
package calling

import (
	"context"
	"encoding/json"
	"io"
)

// CallRequest is the canonical outbound-call request. Providers translate
// the fields they support and ignore the rest (see Capabilities for what
// they advertise).
type CallRequest struct {
	// OrganizationID is the tenant id (ULID string). Required.
	OrganizationID string

	// IntegrationID is the ULID of the integration row supplying credentials.
	// Optional at this layer — the Registry may resolve credentials separately.
	IntegrationID string

	// BusinessEndpointExternalID is the caller identity (e.g. WhatsApp
	// phone_number_id). Required for outbound.
	BusinessEndpointExternalID string

	// To is the callee address. Provider-neutral shape (E.164 phone, BSUID,
	// email). Required.
	To string

	// ToUserID is the WhatsApp BSUID / provider-native durable identity
	// when known. Adapters prefer this over To when the provider accepts it.
	ToUserID string

	// IdempotencyKey is the caller-supplied idempotency token echoed back
	// on subsequent status callbacks. Optional.
	IdempotencyKey string

	// CorrelationID stitches the outbound call back to the originating
	// request or job for observability.
	CorrelationID string

	// Recording opts the outbound call into recording. Optional.
	Recording *RecordingOptions

	// Transcription opts the outbound call into transcription. Optional.
	Transcription *TranscriptionOptions

	// Extras carries free-form provider-specific fields the adapter
	// passes through. Kept as a bag so this port stays provider-agnostic.
	Extras map[string]any
}

// RecordingOptions selects call recording behaviour on outbound calls.
type RecordingOptions struct {
	// Enabled toggles recording. When false, the provider is instructed to
	// explicitly opt out.
	Enabled bool
	// Purpose is a short human-readable purpose spoken to both participants
	// as part of the pre-recording announcement (provider-specific).
	Purpose string
	// AnnouncementLanguage is the locale for the spoken announcement (e.g.
	// "en_US").
	AnnouncementLanguage string
}

// TranscriptionOptions selects call transcription behaviour on outbound
// calls. Mirrors RecordingOptions — the two features are independent.
type TranscriptionOptions struct {
	Enabled              bool
	Purpose              string
	AnnouncementLanguage string
}

// AnswerOptions carries the browser-side WebRTC answer (SDP) plus
// optional recording / transcription selections when the operator accepts
// an inbound call. All fields are optional; a nil *AnswerOptions is a
// bare accept (legacy behaviour).
type AnswerOptions struct {
	// AnswerSDP is the local answer SDP produced by the operator's browser
	// after applying the offer received on the `connect` webhook.
	AnswerSDP string
	// Recording opts this call into recording. Nil leaves recording off.
	Recording *RecordingOptions
	// Transcription opts this call into transcription. Nil leaves it off.
	Transcription *TranscriptionOptions
}

// CallResult is the outcome of an InitiateCall invocation.
type CallResult struct {
	// ProviderCallID is the durable id the provider assigns (e.g. WhatsApp
	// wacid.*).
	ProviderCallID string
	// AcceptedAt is the Unix time (seconds) the provider accepted the
	// initiate. Zero when the provider does not disclose one.
	AcceptedAt int64
}

// Capabilities advertises the subset of the Provider interface a concrete
// adapter actually implements. The application layer inspects this before
// invoking optional methods so unsupported operations surface as validation
// errors rather than opaque provider 5xx.
type Capabilities struct {
	// InitiateOutbound reports whether the provider can place outbound calls.
	InitiateOutbound bool
	// AnswerInbound reports whether the provider exposes a server-side
	// answer/pre-accept path for inbound calls.
	AnswerInbound bool
	// Reject reports whether the provider exposes a server-side reject.
	Reject bool
	// Terminate reports whether the provider exposes a server-side hangup.
	Terminate bool
	// Recording reports whether the provider supports call recording.
	Recording bool
	// Transcription reports whether the provider supports transcription.
	Transcription bool
}

// Permission is the provider-agnostic view of a recipient's current
// call-permission state. Status values are provider-defined but include
// well-known "temporary", "permanent", "no_permission" from WhatsApp.
// ExpirationTime is unix seconds; zero when not applicable.
type Permission struct {
	// Status is the free-form permission enum ("temporary" | "permanent"
	// | "no_permission" | ...). Empty when the provider did not return a
	// permission block.
	Status string
	// ExpirationTime is the unix seconds when a temporary permission
	// lapses. Zero when the status is not time-boxed.
	ExpirationTime int64
}

// Provider is the port every calling adapter satisfies. Contexts MUST be
// propagated end-to-end. No method holds a DB transaction.
type Provider interface {
	// InitiateCall places a new outbound call.
	InitiateCall(ctx context.Context, req CallRequest) (CallResult, error)

	// AnswerCall accepts an inbound call previously surfaced via webhook.
	// opts may be nil (bare accept) or carry an SDP answer + recording /
	// transcription selections when the operator's browser has completed
	// the WebRTC handshake.
	AnswerCall(ctx context.Context, providerCallID string, opts *AnswerOptions) error

	// RejectCall declines an inbound call. reason is a short free-form
	// string carried to the provider when supported.
	RejectCall(ctx context.Context, providerCallID, reason string) error

	// EndCall terminates an in-progress call from the server side.
	EndCall(ctx context.Context, providerCallID string) error

	// GetRecording streams the finished recording bytes. The second return
	// is the Content-Type (e.g. "audio/ogg"). The caller MUST close the
	// returned io.ReadCloser.
	GetRecording(ctx context.Context, providerCallID string) (io.ReadCloser, string, error)

	// GetTranscript resolves the transcript document identified by
	// mediaID (Meta's document.id from the call_transcription_available
	// webhook) and returns the raw JSON payload. Adapters perform the
	// provider-specific two-step (media-url lookup + authenticated
	// download) and return the bytes verbatim so the caller can either
	// re-serve them or parse the shape.
	GetTranscript(ctx context.Context, mediaID string) (json.RawMessage, error)

	// GetPermission fetches the current call-permission for a recipient.
	// Either waID (E.164 or provider-native address) or recipient (BSUID)
	// must be non-empty; adapters use the appropriate lookup key.
	GetPermission(ctx context.Context, waID, recipient string) (Permission, error)

	// SendPermissionRequest prompts the user to grant call permission via
	// the channel (e.g. WhatsApp interactive call_permission_request).
	// Returns the provider-native message id (wamid).
	//
	// This helper stays on the calling port so the call surface is
	// self-contained; adapters dispatch through the channel under the hood.
	SendPermissionRequest(ctx context.Context, waID, prompt string) (string, error)

	// Capabilities describes what the adapter actually supports.
	Capabilities() Capabilities
}

// Registry resolves a Provider for a given provider key + decrypted secrets
// bag. The application service depends on this so it never imports a
// concrete provider package — the wire-up in cmd/server injects a factory
// that closes over the adapters registered at init() time.
type Registry interface {
	// Calling returns a calling.Provider ready to serve requests against the
	// integration whose decrypted secrets are supplied.
	Calling(ctx context.Context, providerKey string, secrets map[string]string) (Provider, error)
}
