package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fullwa/fullwa/internal/domain/events"
)

// ErrSignatureMissing is returned when the incoming request lacks the
// X-Hub-Signature-256 header.
var ErrSignatureMissing = errors.New("whatsapp: missing X-Hub-Signature-256")

// ErrSignatureMismatch is returned when the computed signature does not match
// the one delivered in the header.
var ErrSignatureMismatch = errors.New("whatsapp: signature mismatch")

// VerifySignature verifies Meta's X-Hub-Signature-256 header on a webhook
// request. It computes `sha256=` HMAC-SHA256 of rawBody using appSecret and
// compares to the delivered header in constant time.
//
// Meta's spec: see webhooks/create-webhook-endpoint.md in the local mirror.
func VerifySignature(headers http.Header, rawBody []byte, appSecret string) error {
	if appSecret == "" {
		return fmt.Errorf("whatsapp: verify signature: app secret not configured")
	}
	got := headers.Get("X-Hub-Signature-256")
	if got == "" {
		return ErrSignatureMissing
	}
	got = strings.TrimSpace(got)
	if !strings.HasPrefix(got, "sha256=") {
		return ErrSignatureMismatch
	}
	gotHex := strings.TrimPrefix(got, "sha256=")
	gotBytes, err := hex.DecodeString(gotHex)
	if err != nil {
		return ErrSignatureMismatch
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(rawBody)
	want := mac.Sum(nil)
	if !hmac.Equal(gotBytes, want) {
		return ErrSignatureMismatch
	}
	return nil
}

// EndpointResolver maps a Meta phone_number_id to the tenant that owns it.
// The webhook parser needs this to attach OrgID + BusinessEndpointExternalID
// to every emitted event.
type EndpointResolver interface {
	Resolve(phoneNumberID string) (orgID string, endpointExternalID string, ok bool)
}

// EndpointResolverFunc adapts a bare func to the EndpointResolver interface.
type EndpointResolverFunc func(phoneNumberID string) (orgID string, endpointExternalID string, ok bool)

// Resolve implements EndpointResolver.
func (f EndpointResolverFunc) Resolve(phoneNumberID string) (string, string, bool) {
	return f(phoneNumberID)
}

// ParseWebhook decodes a Meta whatsapp_business_account webhook body into
// zero or more canonical events. Signature verification is NOT performed
// here — call VerifySignature on the raw body first.
func ParseWebhook(rawBody []byte, resolver EndpointResolver) ([]events.Envelope, error) {
	var env metaWebhookEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return nil, fmt.Errorf("whatsapp: decode webhook: %w", err)
	}
	if env.Object != "" && env.Object != "whatsapp_business_account" {
		// Not for us; return empty so the caller ACKs 200.
		return nil, nil
	}
	out := make([]events.Envelope, 0, 4)
	for _, entry := range env.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue // Phase 1: only 'messages' field. Other fields ignored.
			}
			v := change.Value
			var phoneNumberID string
			if v.Metadata != nil {
				phoneNumberID = v.Metadata.PhoneNumberID
			}
			orgID, endpointExternalID, ok := "", phoneNumberID, true
			if resolver != nil {
				orgID, endpointExternalID, ok = resolver.Resolve(phoneNumberID)
				if !ok {
					// Unrouteable — skip this change; caller decides what to
					// log/persist. Never leak Meta types out; just move on.
					continue
				}
			}

			// Index contact profile names by wa_id so inbound events can
			// carry FromDisplayName without a second scan.
			names := map[string]string{}
			for _, c := range v.Contacts {
				names[c.WaID] = c.Profile.Name
			}

			for _, msg := range v.Messages {
				mtype, payload := canonicalize(msg)
				ts := parseTimestamp(msg.Timestamp)
				if ts.IsZero() {
					ts = time.Now().UTC()
				}
				received := events.MessageReceivedPayload{
					Provider:                   "whatsapp",
					Channel:                    "whatsapp",
					BusinessEndpointExternalID: endpointExternalID,
					ProviderMessageID:          msg.ID,
					From:                       "+" + strings.TrimPrefix(msg.From, "+"),
					FromDisplayName:            names[msg.From],
					To:                         phoneNumberID,
					MessageType:                string(mtype),
					Payload:                    payload,
					Timestamp:                  ts,
				}
				if msg.Context != nil {
					received.ContextProviderMessageID = msg.Context.ID
				}
				if mtype == "unknown" {
					// Preserve raw for future-proofing.
					received.Raw = map[string]any{"raw_type": msg.Type}
				}
				out = append(out, events.Envelope{
					Type:           events.MessageReceived,
					OrganizationID: orgID,
					OccurredAt:     ts,
					Payload:        received,
				})
			}
			for _, st := range v.Statuses {
				etype, ok := statusEventType(st.Status)
				if !ok {
					continue
				}
				ts := parseTimestamp(st.Timestamp)
				if ts.IsZero() {
					ts = time.Now().UTC()
				}
				payload := events.MessageStatusPayload{
					Provider:          "whatsapp",
					Channel:           "whatsapp",
					ProviderMessageID: st.ID,
					Recipient:         st.RecipientID,
					Status:            st.Status,
					Timestamp:         ts,
				}
				if len(st.Errors) > 0 {
					payload.ErrorCode = fmt.Sprintf("%d", st.Errors[0].Code)
					payload.ErrorMessage = st.Errors[0].Message
				}
				out = append(out, events.Envelope{
					Type:           etype,
					OrganizationID: orgID,
					OccurredAt:     ts,
					Payload:        payload,
				})
			}
		}
	}
	return out, nil
}

// statusEventType maps Meta status strings onto canonical event types.
func statusEventType(s string) (events.Type, bool) {
	switch s {
	case "sent":
		return events.MessageSent, true
	case "delivered":
		return events.MessageDelivered, true
	case "read":
		return events.MessageRead, true
	case "failed":
		return events.MessageFailed, true
	default:
		return "", false
	}
}
