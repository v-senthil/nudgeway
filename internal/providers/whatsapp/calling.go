package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/events"
	"github.com/v-senthil/nudgeway/internal/ports/calling"
)

// ---------- Provider port implementation ----------
//
// The methods below satisfy calling.Provider by shape, but *Provider also
// implements channel.Provider whose Capabilities() returns a different
// type, so we cannot satisfy both interfaces on the same receiver. The
// disambiguation is done via CallingProvider() below which returns an
// adapter whose Capabilities() returns calling.Capabilities.

// Compile-time check that the calling adapter satisfies calling.Provider.
var _ calling.Provider = callingProviderAdapter{}

// InitiateCall places a business-initiated call via Meta's Calling API.
// See ~/Documents/whatsapp_doc_tracker/docs/calling/reference.md
// (§ "Initiate a new call" — POST /<PHONE_NUMBER_ID>/calls with
// action=connect and an SDP offer).
//
// The WebRTC SDP handshake is intentionally omitted from this adapter.
// A real WebRTC handshake requires a call agent (RTP/DTLS/ICE), which is
// out of scope for the REST-only slice we ship here. We surface the
// InitiateCall shape so orchestration is provider-agnostic; the concrete
// Meta SDP orchestration belongs in a follow-up worker.
func (p *Provider) InitiateCall(ctx context.Context, req calling.CallRequest) (calling.CallResult, error) {
	if req.To == "" && req.ToUserID == "" {
		return calling.CallResult{}, fmt.Errorf("whatsapp: InitiateCall: recipient required")
	}
	if p.cfg.PhoneNumberID == "" {
		return calling.CallResult{}, fmt.Errorf("whatsapp: InitiateCall: phone_number_id not configured")
	}

	body := map[string]any{
		"messaging_product": "whatsapp",
		"action":            "connect",
	}
	if req.To != "" {
		body["to"] = req.To
	}
	if req.ToUserID != "" {
		body["to_user_id"] = req.ToUserID
	}
	if req.IdempotencyKey != "" {
		body["biz_opaque_callback_data"] = req.IdempotencyKey
	}
	if rec := req.Recording; rec != nil {
		body["recording"] = recordingJSON(rec)
	}
	if tr := req.Transcription; tr != nil {
		body["transcription"] = transcriptionJSON(tr)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return calling.CallResult{}, fmt.Errorf("whatsapp: encode initiate call: %w", err)
	}
	url := fmt.Sprintf("%s/%s/%s/calls", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var resp metaCallInitiateResponse
	if err := p.client.doJSON(ctx, "initiate_call", http.MethodPost, url, raw, &resp); err != nil {
		return calling.CallResult{}, err
	}
	if resp.CallID == "" && len(resp.Calls) > 0 {
		resp.CallID = resp.Calls[0].ID
	}
	if resp.CallID == "" {
		return calling.CallResult{}, errors.New("whatsapp: Meta returned no call id")
	}
	return calling.CallResult{
		ProviderCallID: resp.CallID,
		AcceptedAt:     time.Now().UTC().Unix(),
	}, nil
}

// AnswerCall accepts an inbound call by posting action=accept to the
// /<phone_number_id>/calls endpoint. See docs/calling/reference.md §
// "Accept call".
//
// When opts is nil this is a bare accept (legacy behaviour). When opts
// carries an AnswerSDP the browser-side WebRTC answer is included in the
// session block; recording / transcription options ride along when set.
func (p *Provider) AnswerCall(ctx context.Context, providerCallID string, opts *calling.AnswerOptions) error {
	if providerCallID == "" {
		return fmt.Errorf("whatsapp: AnswerCall: providerCallID required")
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"call_id":           providerCallID,
		"action":            "accept",
	}
	if opts != nil {
		if opts.AnswerSDP != "" {
			body["session"] = map[string]any{
				"sdp_type": "answer",
				"sdp":      opts.AnswerSDP,
			}
		}
		if opts.Recording != nil {
			body["recording"] = recordingJSON(opts.Recording)
		}
		if opts.Transcription != nil {
			body["transcription"] = transcriptionJSON(opts.Transcription)
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("whatsapp: encode accept: %w", err)
	}
	url := fmt.Sprintf("%s/%s/%s/calls", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var resp map[string]any
	return p.client.doJSON(ctx, "accept_call", http.MethodPost, url, raw, &resp)
}

// RejectCall declines an inbound call by posting action=reject.
func (p *Provider) RejectCall(ctx context.Context, providerCallID, reason string) error {
	if providerCallID == "" {
		return fmt.Errorf("whatsapp: RejectCall: providerCallID required")
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"call_id":           providerCallID,
		"action":            "reject",
	}
	if reason != "" {
		body["biz_opaque_callback_data"] = reason
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("whatsapp: encode reject: %w", err)
	}
	url := fmt.Sprintf("%s/%s/%s/calls", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var resp map[string]any
	return p.client.doJSON(ctx, "reject_call", http.MethodPost, url, raw, &resp)
}

// EndCall terminates an in-progress call by posting action=terminate.
func (p *Provider) EndCall(ctx context.Context, providerCallID string) error {
	if providerCallID == "" {
		return fmt.Errorf("whatsapp: EndCall: providerCallID required")
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"call_id":           providerCallID,
		"action":            "terminate",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("whatsapp: encode terminate: %w", err)
	}
	url := fmt.Sprintf("%s/%s/%s/calls", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var resp map[string]any
	return p.client.doJSON(ctx, "terminate_call", http.MethodPost, url, raw, &resp)
}

// GetRecording streams the finished recording bytes. Meta ships the
// recording as a media asset delivered by the call_recording_available
// webhook; this method takes the same providerCallID and resolves the
// most recent recording via /<call_id>/recording (Graph shape) followed
// by the standard media-download two-step.
func (p *Provider) GetRecording(ctx context.Context, providerCallID string) (io.ReadCloser, string, error) {
	if providerCallID == "" {
		return nil, "", fmt.Errorf("whatsapp: GetRecording: providerCallID required")
	}
	// Meta's call recording webhook already includes the media asset id;
	// in the current REST-only slice we defer the resolution to the
	// Media API. The caller is expected to pass the media asset id
	// (not the wacid) when a persisted recording_url has already been
	// resolved to a media_id. The adapter treats providerCallID as a
	// media_id when it does not begin with "wacid.".
	if len(providerCallID) > 6 && providerCallID[:6] == "wacid." {
		return nil, "", fmt.Errorf("whatsapp: GetRecording: pass the media asset id, not the wacid")
	}
	lookup, err := p.client.getMediaURL(ctx, providerCallID)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp: lookup recording url: %w", err)
	}
	body, ctype, err := p.client.downloadMedia(ctx, lookup.URL)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp: download recording: %w", err)
	}
	if ctype == "" {
		ctype = lookup.MimeType
	}
	return body, ctype, nil
}

// Transcript is a raw JSON transcript payload returned by GetTranscript.
// Kept as an alias to json.RawMessage so the port stays provider-neutral
// while the adapter can return Meta's document verbatim.
type Transcript = json.RawMessage

// GetTranscript resolves a transcript document by its Meta media id and
// returns the raw JSON bytes. Two-hop: first resolve the short-lived
// media URL via GET /{mediaID}, then download it under the Bearer
// token. Reference:
// ~/Documents/whatsapp_doc_tracker/docs/calling/call-transcription.md.
func (p *Provider) GetTranscript(ctx context.Context, mediaID string) (Transcript, error) {
	if mediaID == "" {
		return nil, fmt.Errorf("whatsapp: GetTranscript: mediaID required")
	}
	// Hop 1: resolve the download URL. getMediaURL already goes through
	// doJSON under op "get_media_url"; we re-use it verbatim so metrics
	// stay uniform with the rest of the media pipeline. (The task's
	// op-name suggestion "get_transcript_url" is subsumed by the shared
	// get_media_url op — Meta uses the same GET /{id} endpoint for every
	// media asset regardless of mime.)
	lookup, err := p.client.getMediaURL(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: lookup transcript url: %w", err)
	}
	// Hop 2: download the bytes. downloadMedia handles auth, tracing,
	// and error classification uniformly with recording + image
	// downloads, so we re-use it rather than adding a parallel op.
	body, _, err := p.client.downloadMedia(ctx, lookup.URL)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: download transcript: %w", err)
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: read transcript: %w", err)
	}
	return Transcript(raw), nil
}

// CallPermission is the adapter-local shape of Meta's user-call-permission
// response. Meta returns a top-level envelope {permission:{status,expiration_time}};
// the adapter unwraps the sub-object and exposes it verbatim. Status values
// documented by Meta include "temporary", "permanent", "no_permission".
// See ~/Documents/whatsapp_doc_tracker/docs/calling/user-call-permissions.md.
type CallPermission struct {
	// Status is Meta's permission enum ("temporary" | "permanent" |
	// "no_permission" | ...). Empty when Meta did not include a permission
	// block.
	Status string `json:"status,omitempty"`
	// ExpirationTime is the unix seconds when a "temporary" permission lapses.
	// Zero for "permanent" / "no_permission".
	ExpirationTime int64 `json:"expiration_time,omitempty"`
}

// metaCallPermissionEnvelope mirrors Meta's response shape:
//
//	{"messaging_product":"whatsapp",
//	 "permission":{"status":"<STATUS>","expiration_time":<UNIX>},
//	 "actions":[...]}
type metaCallPermissionEnvelope struct {
	MessagingProduct string         `json:"messaging_product,omitempty"`
	Permission       CallPermission `json:"permission"`
	Actions          []any          `json:"actions,omitempty"`
}

// GetCallPermission fetches the current call-permission state for a
// recipient. Either waID (E.164) or recipient (BSUID) must be non-empty;
// waID wins when both are supplied. The Meta envelope's `.permission`
// sub-object is unwrapped and returned. See docs/calling/user-call-permissions.md
// (§ "Get the current permission").
func (p *Provider) GetCallPermission(ctx context.Context, waID, recipient string) (CallPermission, error) {
	if waID == "" && recipient == "" {
		return CallPermission{}, fmt.Errorf("whatsapp: GetCallPermission: waID or recipient required")
	}
	if p.cfg.PhoneNumberID == "" {
		return CallPermission{}, fmt.Errorf("whatsapp: GetCallPermission: phone_number_id not configured")
	}
	q := ""
	if waID != "" {
		q = "user_wa_id=" + waID
	} else {
		q = "recipient=" + recipient
	}
	url := fmt.Sprintf("%s/%s/%s/call_permissions?%s",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID, q,
	)
	var env metaCallPermissionEnvelope
	if err := p.client.doJSON(ctx, "get_call_permission", http.MethodGet, url, nil, &env); err != nil {
		return CallPermission{}, err
	}
	return env.Permission, nil
}

// SendCallPermissionRequest sends an interactive call_permission_request
// message to prompt the user to grant call permission. Returns the wamid.
// See docs/calling/user-call-permissions.md (§ "Send free-form permission
// request").
func (p *Provider) SendCallPermissionRequest(ctx context.Context, waID, prompt string) (string, error) {
	if waID == "" {
		return "", fmt.Errorf("whatsapp: SendCallPermissionRequest: waID required")
	}
	if p.cfg.PhoneNumberID == "" {
		return "", fmt.Errorf("whatsapp: SendCallPermissionRequest: phone_number_id not configured")
	}
	if prompt == "" {
		prompt = "We'd like to call you regarding your recent conversation."
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                waID,
		"type":              "interactive",
		"interactive": map[string]any{
			"type":   "call_permission_request",
			"action": map[string]any{"name": "call_permission_request"},
			"body":   map[string]any{"text": prompt},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("whatsapp: encode permission request: %w", err)
	}
	url := fmt.Sprintf("%s/%s/%s/messages", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var resp metaSendResponse
	if err := p.client.doJSON(ctx, "send_call_permission_request", http.MethodPost, url, raw, &resp); err != nil {
		return "", err
	}
	if len(resp.Messages) == 0 {
		return "", errors.New("whatsapp: Meta returned no wamid on permission request")
	}
	return resp.Messages[0].ID, nil
}

// Capabilities reports the calling capabilities of the WhatsApp adapter.
// Kept as a separate method so the calling.Provider interface stays
// decoupled from the channel.Provider capability shape.
func (p *Provider) CallingCapabilities() calling.Capabilities {
	return calling.Capabilities{
		InitiateOutbound: true,
		AnswerInbound:    true,
		Reject:           true,
		Terminate:        true,
		Recording:        true,
		Transcription:    true,
	}
}

// Capabilities on the calling.Provider interface — resolves the naming
// clash with the channel.Provider.Capabilities method by delegating.
//
// Go dispatches the method on the interface, so consumers of
// calling.Provider see this variant while channel consumers keep the
// existing SendText/ReceiveMessages shape returned from capabilities.go.
// (Both methods coexist because the two ports return different types;
// Go allows the same receiver to satisfy both.)
//
// NOTE: capabilities.go defines Capabilities() channel.Capabilities. To
// avoid a duplicate method name on the same receiver, we expose the
// calling capabilities via the wrapper returned by CallingProvider().
// See the CallingProvider adapter type below.

// CallingProvider returns a value that satisfies calling.Provider by
// wrapping the WhatsApp *Provider. The wrapper's Capabilities() returns
// calling.Capabilities rather than channel.Capabilities.
func (p *Provider) CallingProvider() calling.Provider { return callingProviderAdapter{p: p} }

// callingProviderAdapter shim exists to disambiguate the Capabilities()
// method between the channel.Provider and calling.Provider interfaces.
// Its InitiateCall / Answer / Reject / End / GetRecording methods delegate
// verbatim to the underlying *Provider; Capabilities() returns the
// calling capability shape.
type callingProviderAdapter struct{ p *Provider }

// InitiateCall delegates to the underlying provider.
func (a callingProviderAdapter) InitiateCall(ctx context.Context, req calling.CallRequest) (calling.CallResult, error) {
	return a.p.InitiateCall(ctx, req)
}

// AnswerCall delegates to the underlying provider.
func (a callingProviderAdapter) AnswerCall(ctx context.Context, providerCallID string, opts *calling.AnswerOptions) error {
	return a.p.AnswerCall(ctx, providerCallID, opts)
}

// RejectCall delegates to the underlying provider.
func (a callingProviderAdapter) RejectCall(ctx context.Context, providerCallID, reason string) error {
	return a.p.RejectCall(ctx, providerCallID, reason)
}

// EndCall delegates to the underlying provider.
func (a callingProviderAdapter) EndCall(ctx context.Context, providerCallID string) error {
	return a.p.EndCall(ctx, providerCallID)
}

// GetRecording delegates to the underlying provider.
func (a callingProviderAdapter) GetRecording(ctx context.Context, providerCallID string) (io.ReadCloser, string, error) {
	return a.p.GetRecording(ctx, providerCallID)
}

// GetTranscript delegates to the underlying provider.
func (a callingProviderAdapter) GetTranscript(ctx context.Context, mediaID string) (json.RawMessage, error) {
	return a.p.GetTranscript(ctx, mediaID)
}

// GetPermission delegates to GetCallPermission and translates the shape
// into the port-neutral calling.Permission.
func (a callingProviderAdapter) GetPermission(ctx context.Context, waID, recipient string) (calling.Permission, error) {
	pm, err := a.p.GetCallPermission(ctx, waID, recipient)
	if err != nil {
		return calling.Permission{}, err
	}
	return calling.Permission{Status: pm.Status, ExpirationTime: pm.ExpirationTime}, nil
}

// SendPermissionRequest delegates to SendCallPermissionRequest.
func (a callingProviderAdapter) SendPermissionRequest(ctx context.Context, waID, prompt string) (string, error) {
	return a.p.SendCallPermissionRequest(ctx, waID, prompt)
}

// Capabilities returns calling.Capabilities.
func (a callingProviderAdapter) Capabilities() calling.Capabilities {
	return a.p.CallingCapabilities()
}

// ---------- Meta wire types (adapter-local; NEVER leak) ----------

type metaCallInitiateResponse struct {
	// CallID is the Meta wacid.* returned on success.
	CallID string             `json:"call_id,omitempty"`
	Calls  []metaCallInitEntry `json:"calls,omitempty"`
}

type metaCallInitEntry struct {
	ID string `json:"id"`
}

// metaWebhookCall is the shape of an entry in the "calls" array on inbound
// call webhooks. Its field vocabulary is documented in
// ~/Documents/whatsapp_doc_tracker/docs/calling/reference.md
// (§ "Webhook values for calls").
type metaWebhookCall struct {
	ID              string          `json:"id"`
	To              string          `json:"to,omitempty"`
	ToUserID        string          `json:"to_user_id,omitempty"`
	ToParentUserID  string          `json:"to_parent_user_id,omitempty"`
	From            string          `json:"from,omitempty"`
	FromUserID      string          `json:"from_user_id,omitempty"`
	FromParentID    string          `json:"from_parent_user_id,omitempty"`
	Event           string          `json:"event"`
	Timestamp       string          `json:"timestamp,omitempty"`
	Direction       string          `json:"direction,omitempty"`
	Status          string          `json:"status,omitempty"`
	StartTime       string          `json:"start_time,omitempty"`
	EndTime         string          `json:"end_time,omitempty"`
	Duration        int             `json:"duration,omitempty"`
	BizOpaque       string          `json:"biz_opaque_callback_data,omitempty"`
	CallRecording   *metaMediaSlot  `json:"call_recording,omitempty"`
	CallTranscript  *metaDocSlot    `json:"call_transcript,omitempty"`
	Session         *metaCallSession `json:"session,omitempty"`
}

// metaCallSession carries the SDP offer/answer that Meta ships on the
// `connect` webhook (event=connect ⇒ sdp_type="offer" + sdp="<RFC 8866>").
// See ~/Documents/whatsapp_doc_tracker/docs/calling/user-initiated-calls.md
// ("Inbound call webhook shape"). The adapter stashes the SDP verbatim
// into CallEventPayload.Raw so the application layer can persist it
// without widening the canonical payload.
type metaCallSession struct {
	SDPType string `json:"sdp_type,omitempty"`
	SDP     string `json:"sdp,omitempty"`
}

type metaMediaSlot struct {
	Type  string           `json:"type,omitempty"`
	Audio *metaMediaAsset  `json:"audio,omitempty"`
}

type metaDocSlot struct {
	Document *metaMediaAsset `json:"document,omitempty"`
}

type metaMediaAsset struct {
	ID       string `json:"id,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	URL      string `json:"url,omitempty"`
}

// metaCallWebhookEnvelope mirrors the top-level shape but only decodes
// the "calls" array — this keeps the calling parser independent of the
// message parser in mapper.go.
type metaCallWebhookEnvelope struct {
	Object string              `json:"object"`
	Entry  []metaCallEntryWrap `json:"entry"`
}

type metaCallEntryWrap struct {
	ID      string           `json:"id"`
	Changes []metaCallChange `json:"changes"`
}

type metaCallChange struct {
	Field string        `json:"field"`
	Value metaCallValue `json:"value"`
}

type metaCallValue struct {
	MessagingProduct string             `json:"messaging_product,omitempty"`
	Metadata         *metaValueMetadata `json:"metadata,omitempty"`
	Calls            []metaWebhookCall  `json:"calls,omitempty"`
	Statuses         []metaCallStatus   `json:"statuses,omitempty"`
	Errors           []metaError        `json:"errors,omitempty"`
}

// metaCallStatus is the shape of an entry in the "statuses" array on
// the Call status webhook (RINGING / ACCEPTED / REJECTED).
type metaCallStatus struct {
	ID                     string `json:"id"`
	Timestamp              string `json:"timestamp"`
	Type                   string `json:"type"`
	Status                 string `json:"status"`
	RecipientID            string `json:"recipient_id,omitempty"`
	RecipientUserID        string `json:"recipient_user_id,omitempty"`
	RecipientParentUserID  string `json:"recipient_parent_user_id,omitempty"`
	BizOpaqueCallbackData  string `json:"biz_opaque_callback_data,omitempty"`
}

// ---------- Webhook parser ----------

// ParseCallWebhook decodes a Meta whatsapp_business_account webhook whose
// changes[].field is "calls" into canonical events. It is invoked in
// parallel with ParseWebhook by the ingest wire-up so both message and
// call events are emitted from a single POST body.
//
// Signature verification is the caller's responsibility. Failures to
// resolve the tenant (unknown phone_number_id) result in an empty return
// so the caller ACKs 200 — Meta retries otherwise.
func ParseCallWebhook(rawBody []byte, resolver EndpointResolver) ([]events.Envelope, error) {
	var env metaCallWebhookEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return nil, fmt.Errorf("whatsapp: decode call webhook: %w", err)
	}
	if env.Object != "" && env.Object != "whatsapp_business_account" {
		return nil, nil
	}
	var out []events.Envelope
	for _, entry := range env.Entry {
		for _, ch := range entry.Changes {
			if ch.Field != "calls" {
				continue
			}
			phoneID := ""
			if ch.Value.Metadata != nil {
				phoneID = ch.Value.Metadata.PhoneNumberID
			}
			orgID := ""
			endpointExternalID := phoneID
			if resolver != nil {
				o, ex, ok := resolver.Resolve(phoneID)
				if ok {
					orgID = o
					endpointExternalID = ex
				}
			}
			// When no resolver is wired (the default whatsapp adapter case
			// used by webhook.ProviderLookup), leave orgID empty and let
			// the InboundService stamp it from the integration lookup.
			// Do NOT drop the envelope — mirroring the message parser's
			// behavior avoids losing every inbound call in the default
			// wire-up. Cross-tenant safety comes from the integration_id
			// scoping already applied upstream at the webhook route.
			// Emit an envelope for each call entry.
			for _, wc := range ch.Value.Calls {
				out = append(out, envelopeFromCall(orgID, endpointExternalID, wc))
			}
			// And for each status entry (RINGING/ACCEPTED/REJECTED).
			for _, st := range ch.Value.Statuses {
				out = append(out, envelopeFromCallStatus(orgID, endpointExternalID, st))
			}
		}
	}
	return out, nil
}

// envelopeFromCall maps one metaWebhookCall into a canonical Envelope.
// Event type selection:
//
//	event=connect                     → CallRinging (connect webhook is
//	                                    fired when the callee side is ready)
//	event=call_created                → CallInitiated
//	event=terminate + status=COMPLETED → CallEnded
//	event=terminate + status=FAILED   → CallFailed
//	event=call_recording_available    → CallRecordingCreated
//	event=call_transcription_available → CallEnded (transcript attaches
//	                                    onto an already-terminal row)
func envelopeFromCall(orgID, endpointExternalID string, wc metaWebhookCall) events.Envelope {
	direction := "outbound"
	if wc.Direction == "USER_INITIATED" {
		direction = "inbound"
	}
	payload := events.CallEventPayload{
		Provider:                   providerKey,
		ProviderCallID:             wc.ID,
		BusinessEndpointExternalID: endpointExternalID,
		Direction:                  direction,
		From:                       wc.From,
		To:                         wc.To,
		FromUserID:                 wc.FromUserID,
		ToUserID:                   wc.ToUserID,
		HangupReason:               "",
		DurationSeconds:            wc.Duration,
		Timestamp:                  parseUnix(wc.Timestamp),
	}
	if wc.StartTime != "" {
		payload.StartedAt = parseUnix(wc.StartTime)
	}
	if wc.EndTime != "" {
		payload.EndedAt = parseUnix(wc.EndTime)
	}
	if wc.CallRecording != nil && wc.CallRecording.Audio != nil {
		payload.RecordingURL = wc.CallRecording.Audio.URL
		payload.Raw = map[string]any{"recording_media_id": wc.CallRecording.Audio.ID}
	}
	if wc.CallTranscript != nil && wc.CallTranscript.Document != nil {
		payload.TranscriptionRef = wc.CallTranscript.Document.ID
	}
	if wc.BizOpaque != "" {
		if payload.Raw == nil {
			payload.Raw = map[string]any{}
		}
		payload.Raw["biz_opaque_callback_data"] = wc.BizOpaque
	}
	// Capture the SDP offer Meta ships on the `connect` webhook so the
	// application layer can persist it and the operator's browser can
	// build a WebRTC answer against it.
	if wc.Session != nil && wc.Session.SDP != "" {
		if payload.Raw == nil {
			payload.Raw = map[string]any{}
		}
		payload.Raw["session_sdp"] = wc.Session.SDP
		if wc.Session.SDPType != "" {
			payload.Raw["session_sdp_type"] = wc.Session.SDPType
		} else {
			payload.Raw["session_sdp_type"] = "offer"
		}
	}

	var eventType events.Type
	switch wc.Event {
	case "connect":
		eventType = events.CallRinging
		payload.Status = "ringing"
	case "call_created":
		eventType = events.CallInitiated
		payload.Status = "queued"
	case "terminate":
		if wc.Status == "COMPLETED" {
			eventType = events.CallEnded
			payload.Status = "completed"
		} else {
			eventType = events.CallFailed
			payload.Status = "failed"
			payload.HangupReason = wc.Status
		}
	case "call_recording_available":
		eventType = events.CallRecordingCreated
		payload.Status = "completed"
	// Meta ships both `call_transcript_available` (observed in the wild)
	// and `call_transcription_available` (documented). Accept both so a
	// spec drift in either direction doesn't silently drop the transcript.
	case "call_transcript_available", "call_transcription_available":
		eventType = events.CallEnded
		payload.Status = "completed"
	default:
		eventType = events.CallEnded
	}

	return events.Envelope{
		Type:           eventType,
		OrganizationID: orgID,
		OccurredAt:     payload.Timestamp,
		Payload:        payload,
	}
}

// envelopeFromCallStatus maps a metaCallStatus into a canonical Envelope.
func envelopeFromCallStatus(orgID, endpointExternalID string, st metaCallStatus) events.Envelope {
	payload := events.CallEventPayload{
		Provider:                   providerKey,
		ProviderCallID:             st.ID,
		BusinessEndpointExternalID: endpointExternalID,
		Direction:                  "outbound", // status webhooks fire on business-initiated calls
		To:                         st.RecipientID,
		ToUserID:                   st.RecipientUserID,
		Timestamp:                  parseUnix(st.Timestamp),
	}
	var eventType events.Type
	switch st.Status {
	case "RINGING":
		eventType = events.CallRinging
		payload.Status = "ringing"
	case "ACCEPTED":
		eventType = events.CallAnswered
		payload.Status = "answered"
		payload.AnsweredAt = payload.Timestamp
	case "REJECTED":
		eventType = events.CallFailed
		payload.Status = "declined"
		payload.HangupReason = "declined"
	default:
		eventType = events.CallRinging
		payload.Status = "ringing"
	}
	if st.BizOpaqueCallbackData != "" {
		payload.Raw = map[string]any{"biz_opaque_callback_data": st.BizOpaqueCallbackData}
	}
	return events.Envelope{
		Type:           eventType,
		OrganizationID: orgID,
		OccurredAt:     payload.Timestamp,
		Payload:        payload,
	}
}

// parseUnix parses a Meta unix timestamp string into a time.Time.
func parseUnix(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

// ---------- payload helpers ----------

// recordingJSON serializes RecordingOptions into Meta's shape. When
// Enabled is true Meta REQUIRES both purpose and announcement_language —
// omitting either is rejected with 400 "JSON schema constraint 'required'".
// Fill sensible defaults when the caller didn't specify one so a bare
// "record this call" checkbox in the UI still succeeds. Reference:
// ~/Documents/whatsapp_doc_tracker/docs/calling/call-recording.md
// § "recording object reference".
func recordingJSON(r *calling.RecordingOptions) map[string]any {
	m := map[string]any{"status": "DISABLED"}
	if r.Enabled {
		purpose := r.Purpose
		if purpose == "" {
			purpose = "Quality assurance"
		}
		lang := r.AnnouncementLanguage
		if lang == "" {
			lang = "en_US"
		}
		m["status"] = "ENABLED"
		m["purpose"] = purpose
		m["announcement_language"] = lang
	}
	return m
}

// transcriptionJSON mirrors recordingJSON. Meta enforces the same
// required-when-enabled contract on the transcription sub-object.
func transcriptionJSON(t *calling.TranscriptionOptions) map[string]any {
	m := map[string]any{"status": "DISABLED"}
	if t.Enabled {
		purpose := t.Purpose
		if purpose == "" {
			purpose = "Quality assurance"
		}
		lang := t.AnnouncementLanguage
		if lang == "" {
			lang = "en_US"
		}
		m["status"] = "ENABLED"
		m["purpose"] = purpose
		m["announcement_language"] = lang
	}
	return m
}
