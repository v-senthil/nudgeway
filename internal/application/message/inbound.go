package message

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/events"
	"github.com/fullwa/fullwa/internal/domain/identity"
	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
)

// InboundService is the application-layer entry point for processing a
// provider webhook delivery once the ingress layer has verified the
// signature, persisted the raw body, and enqueued the job.
//
// Responsibilities:
//   - Resolve the tenant from integration_id.
//   - Dispatch parsing to the provider adapter (via a registry lookup so
//     no provider package is imported here).
//   - For each canonical events.Envelope emitted by the parser:
//     upsert Contact/Identity/Session/Conversation, persist the Message
//     row (inbound) or advance status (status callback), and publish the
//     canonical event on the bus.
//   - Mark the webhook_events row processed/failed for idempotency +
//     audit.
//
// The service never opens a DB transaction across the provider call —
// persistence is per-envelope, and idempotency comes from the
// UNIQUE(org, provider_message_id) index on messages plus the
// UNIQUE(integration_id, external_event_id) index on webhook_events.
type InboundService struct {
	deps           Deps
	mediaWarnOnce  sync.Once
}

// NewInboundService constructs an InboundService with the injected deps.
// Deps.Now defaults to time.Now().UTC() when nil.
func NewInboundService(deps Deps) *InboundService {
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	return &InboundService{deps: deps}
}

// ProcessRaw is the worker entry point. providerKey is the registry key of
// the adapter that should parse the body ("whatsapp" for Meta). integrationID
// is the integration row whose secrets + org_id anchor the delivery.
// rawBody is the exact bytes that were signature-verified at ingress —
// re-serialising here would break MAC parity for downstream replays.
//
// Errors are classified via IsPermanent. Permanent errors mark the
// webhook_events row failed and return nil so the queue commits the job;
// transient errors return the error so the consumer redelivers.
//
// eventID is the WebhookEvent row id that the ingress layer already
// persisted (via WebhookEventRepo.Insert). Pass empty to skip the
// mark-processed / mark-failed bookkeeping — useful for tests or for
// direct-invocation smoke paths where no webhook_events row exists.
func (s *InboundService) ProcessRaw(
	ctx context.Context,
	providerKey string,
	integrationID integration.ID,
	eventID integration.WebhookEventID,
	rawBody []byte,
) error {
	if err := s.process(ctx, providerKey, integrationID, rawBody); err != nil {
		s.markFailed(ctx, eventID, err)
		if IsPermanent(err) {
			return nil
		}
		return err
	}
	s.markProcessed(ctx, eventID)
	return nil
}

// process runs the load-parse-persist pipeline. Kept small so ProcessRaw
// can wrap it with the WebhookEvent bookkeeping.
func (s *InboundService) process(
	ctx context.Context,
	providerKey string,
	integrationID integration.ID,
	rawBody []byte,
) error {
	integ, _, err := s.deps.Integrations.GetWithSecrets(ctx, integrationID)
	if err != nil {
		if isNotFound(err) {
			return Permanent(fmt.Errorf("%w: %s", ErrIntegrationNotFound, integrationID))
		}
		return fmt.Errorf("inbound: load integration %s: %w", integrationID, err)
	}
	orgID := integ.OrgID

	provider, ok := s.deps.LookupProvider(providerKey)
	if !ok {
		return Permanent(fmt.Errorf("%w: %s", ErrProviderNotRegistered, providerKey))
	}

	envelopes, err := provider.ParseWebhook(ctx, nil, rawBody)
	if err != nil {
		return Permanent(fmt.Errorf("inbound: parse webhook (provider=%s): %w", providerKey, err))
	}

	for i := range envelopes {
		// stamp org_id on every envelope so downstream subscribers see the
		// tenant regardless of whether the parser had a resolver wired.
		if envelopes[i].OrganizationID == "" {
			envelopes[i].OrganizationID = string(orgID)
		}
		if err := s.handleEnvelope(ctx, providerKey, integrationID, orgID, envelopes[i]); err != nil {
			// Any envelope error aborts the batch — the raw event will be
			// marked failed and the queue policy applies. Envelope-level
			// permanence bubbles up as-is.
			return err
		}
	}
	return nil
}

// handleEnvelope routes each envelope by Type.
func (s *InboundService) handleEnvelope(
	ctx context.Context,
	providerKey string,
	integrationID integration.ID,
	orgID organization.ID,
	env events.Envelope,
) error {
	switch env.Type {
	case events.MessageReceived:
		payload, ok := env.Payload.(events.MessageReceivedPayload)
		if !ok {
			return Permanent(fmt.Errorf("%w: MessageReceived payload=%T", ErrUnknownEnvelope, env.Payload))
		}
		return s.handleInbound(ctx, providerKey, integrationID, orgID, env, payload)
	case events.MessageSent, events.MessageDelivered, events.MessageRead, events.MessageFailed:
		payload, ok := env.Payload.(events.MessageStatusPayload)
		if !ok {
			return Permanent(fmt.Errorf("%w: %s payload=%T", ErrUnknownEnvelope, env.Type, env.Payload))
		}
		return s.handleStatus(ctx, orgID, env, payload)
	default:
		// Unknown envelope types are non-fatal — we simply don't have a
		// handler yet. Log via error return classified permanent so we
		// don't retry forever, but do not fail the whole batch.
		return nil
	}
}

// handleInbound persists a newly-received customer message.
func (s *InboundService) handleInbound(
	ctx context.Context,
	providerKey string,
	integrationID integration.ID,
	orgID organization.ID,
	env events.Envelope,
	payload events.MessageReceivedPayload,
) error {
	endpoint, err := s.deps.BusinessEndpoints.FindByExternalID(ctx, orgID, providerKey, payload.BusinessEndpointExternalID)
	if err != nil {
		if isNotFound(err) {
			// Integration must be provisioned first. Skipping is safe —
			// the raw body is already in webhook_events for replay.
			return Permanent(fmt.Errorf("%w: org=%s provider=%s endpoint=%s: %w",
				ErrEndpointNotProvisioned, orgID, providerKey, payload.BusinessEndpointExternalID, err))
		}
		return fmt.Errorf("inbound: resolve endpoint org=%s provider=%s external=%s: %w",
			orgID, providerKey, payload.BusinessEndpointExternalID, err)
	}

	normalizedPhone, err := identity.NormalizePhoneE164(payload.From)
	if err != nil {
		return Permanent(fmt.Errorf("inbound: normalize sender %q: %w", payload.From, err))
	}

	contactID := contact.ID(s.deps.NewID())
	// The schema has a mutual FK between contacts and contact_identities:
	// identity.contact_id → contacts.id (NOT NULL), and
	// contact.primary_identity_id → contact_identities.id (nullable).
	// Bootstrap by inserting the Contact first with PrimaryIdentityID=nil,
	// then the Identity, then re-upsert the Contact with the primary set.
	display := payload.FromDisplayName
	if display == "" {
		display = fallbackDisplayName(normalizedPhone)
	}
	if norm, err := contact.NormalizeDisplayName(display); err == nil {
		display = norm
	}
	now := s.deps.Now()
	if err := s.deps.Contacts.Upsert(ctx, contact.Contact{
		ID:          contactID,
		OrgID:       orgID,
		DisplayName: display,
		LastSeenAt:  &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return fmt.Errorf("inbound: upsert contact (pre-identity): %w", err)
	}

	ident, created, err := s.deps.Identities.FindOrCreate(
		ctx, orgID, contactID, identityTypeForProvider(providerKey), providerKey, payload.From, normalizedPhone,
	)
	if err != nil {
		return fmt.Errorf("inbound: find-or-create identity: %w", err)
	}
	// FindOrCreate returns the pre-existing contact_id when the identity
	// row already existed; adopt it so downstream writes hit the right row.
	contactID = ident.ContactID

	// BSUID (WhatsApp business-scoped user id) upsert. Meta now ships this
	// alongside every message and will eventually stop sending wa_id — it
	// is the durable identity. When present, upsert as a BSUID identity
	// bound to the same Contact and promote it to primary.
	primaryIdentityID := contact.IdentityID(ident.ID)
	if payload.FromUserID != "" {
		bsuidIdent, _, bErr := s.deps.Identities.FindOrCreate(
			ctx, orgID, contactID, identity.TypeBSUID, providerKey,
			payload.FromUserID, payload.FromUserID,
		)
		if bErr != nil {
			slog.Default().Warn("inbound: bsuid identity upsert failed (continuing with phone identity)",
				slog.String("provider", providerKey),
				slog.String("org_id", string(orgID)),
				slog.String("bsuid", payload.FromUserID),
				slog.Any("err", bErr),
			)
		} else {
			// FindOrCreate may return an identity bound to a different
			// contact if the BSUID row pre-existed under an older contact
			// row — adopt whichever contact_id it points at.
			contactID = bsuidIdent.ContactID
			primaryIdentityID = contact.IdentityID(bsuidIdent.ID)
		}
	}

	if created || payload.FromUserID != "" {
		if err := s.deps.Contacts.Upsert(ctx, contact.Contact{
			ID:                contactID,
			OrgID:             orgID,
			DisplayName:       display,
			PrimaryIdentityID: &primaryIdentityID,
			LastSeenAt:        &now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			return fmt.Errorf("inbound: upsert contact (post-identity): %w", err)
		}
	}

	sess, err := s.deps.Sessions.FindOrCreateActive(ctx, orgID, endpoint.ID, contactID)
	if err != nil {
		return fmt.Errorf("inbound: find-or-create session: %w", err)
	}

	conv, err := s.deps.Conversations.FindOrCreateOpen(ctx, orgID, sess.ID, contactID)
	if err != nil {
		return fmt.Errorf("inbound: find-or-create conversation: %w", err)
	}

	msg := message.Message{
		ID:                message.ID(s.deps.NewID()),
		OrgID:             orgID,
		ContactID:         contactID,
		SessionID:         sess.ID,
		ConversationID:    conv.ID,
		Channel:           payload.Channel,
		Provider:          payload.Provider,
		Direction:         message.DirectionInbound,
		SenderIdentity:    normalizedPhone,
		RecipientIdentity: payload.To,
		MessageType:       message.Type(payload.MessageType),
		ProviderMessageID: payload.ProviderMessageID,
		Status:            message.StatusDelivered, // inbound: skip QUEUED/SENT
		CreatedAt:         payload.Timestamp,
		DeliveredAt:       timePtr(payload.Timestamp),
	}
	if payload.Raw != nil {
		msg.Metadata = payload.Raw
	}
	// Surface the message body / caption into metadata.text so the REST DTO
	// can render it without dereferencing HBase. Real payload storage lands
	// with the HBase infra work.
	if msg.Metadata == nil {
		msg.Metadata = map[string]any{}
	}
	if t, ok := payload.Payload.(message.TextPayload); ok && t.Body != "" {
		msg.Metadata["text"] = t.Body
	}
	if m, ok := payload.Payload.(message.MediaPayload); ok {
		if m.Caption != "" {
			msg.Metadata["text"] = m.Caption
		}
		if m.URL != "" {
			msg.Metadata["media_url"] = m.URL
		}
		if m.MediaID != "" {
			s.downloadInboundMedia(ctx, providerKey, orgID, integrationID, msg.Metadata, m)
		}
	}
	if l, ok := payload.Payload.(message.LocationPayload); ok {
		loc := map[string]any{
			"latitude":  l.Latitude,
			"longitude": l.Longitude,
		}
		if l.Name != "" {
			loc["name"] = l.Name
		}
		if l.Address != "" {
			loc["address"] = l.Address
		}
		if l.URL != "" {
			loc["url"] = l.URL
		}
		msg.Metadata["location"] = loc
	}
	if c, ok := payload.Payload.(message.ContactsPayload); ok {
		cards := make([]map[string]any, 0, len(c.Contacts))
		for _, card := range c.Contacts {
			out := map[string]any{}
			name := ""
			if card.Name != nil {
				if v, ok := card.Name["formatted_name"].(string); ok && v != "" {
					name = v
				} else if v, ok := card.Name["first_name"].(string); ok {
					name = v
					if last, ok := card.Name["last_name"].(string); ok && last != "" {
						name = strings.TrimSpace(name + " " + last)
					}
				}
			}
			out["name"] = name
			if len(card.Phones) > 0 {
				phones := make([]string, 0, len(card.Phones))
				for _, p := range card.Phones {
					if s, ok := p["phone"].(string); ok && s != "" {
						phones = append(phones, s)
					} else if s, ok := p["wa_id"].(string); ok && s != "" {
						phones = append(phones, s)
					}
				}
				if len(phones) > 0 {
					out["phones"] = phones
				}
			}
			if len(card.Emails) > 0 {
				emails := make([]string, 0, len(card.Emails))
				for _, e := range card.Emails {
					if s, ok := e["email"].(string); ok && s != "" {
						emails = append(emails, s)
					}
				}
				if len(emails) > 0 {
					out["emails"] = emails
				}
			}
			cards = append(cards, out)
		}
		msg.Metadata["contacts"] = cards
	}
	if r, ok := payload.Payload.(message.ReactionPayload); ok {
		msg.Metadata["reaction"] = map[string]any{
			"emoji":      r.Emoji,
			"message_id": r.MessageID,
		}
	}
	if it, ok := payload.Payload.(message.InteractivePayload); ok {
		interactive := map[string]any{"kind": it.Kind}
		if it.ButtonReply != nil {
			interactive["id"] = it.ButtonReply.ID
			interactive["title"] = it.ButtonReply.Title
		} else if it.ListReply != nil {
			interactive["id"] = it.ListReply.ID
			interactive["title"] = it.ListReply.Title
			if it.ListReply.Description != "" {
				interactive["description"] = it.ListReply.Description
			}
		}
		msg.Metadata["interactive"] = interactive
	}
	if payload.ContextProviderMessageID != "" {
		msg.Metadata["reply_to_wamid"] = payload.ContextProviderMessageID
	}

	if err := s.deps.Messages.Create(ctx, msg); err != nil {
		if IsDuplicateMessage(err) {
			// Duplicate Meta redelivery — treat as success, do not
			// re-publish (subscribers already saw the first delivery).
			return nil
		}
		return fmt.Errorf("inbound: persist message: %w", err)
	}

	// Enrich the payload with the canonical conversation_id so downstream
	// subscribers (WebSocket bridge → browser) can route to the right
	// thread without a second DB round-trip.
	payload.ConversationID = string(conv.ID)
	env.Payload = payload
	if err := s.deps.Bus.Publish(ctx, env); err != nil {
		return fmt.Errorf("inbound: publish MessageReceived: %w", err)
	}
	return nil
}

// handleStatus advances a message's status based on a provider status
// callback. Idempotent — the underlying repo swallows terminal-state
// re-applications.
func (s *InboundService) handleStatus(
	ctx context.Context,
	orgID organization.ID,
	env events.Envelope,
	payload events.MessageStatusPayload,
) error {
	next, ok := domainStatusFor(env.Type)
	if !ok {
		return nil
	}
	err := s.deps.MessageStatusByPMI.UpdateStatusByProviderMessageID(ctx, orgID, payload.ProviderMessageID, next, payload.Timestamp)
	if err != nil {
		if isNotFound(err) {
			// Race: Meta delivered a status callback before we persisted
			// the send row. Permanent for this delivery; the send-status
			// reconciler picks these up on a later pass.
			return Permanent(fmt.Errorf("inbound: status for unknown message provider_id=%s: %w", payload.ProviderMessageID, err))
		}
		return fmt.Errorf("inbound: update status org=%s pmid=%s: %w", orgID, payload.ProviderMessageID, err)
	}
	if err := s.deps.Bus.Publish(ctx, env); err != nil {
		return fmt.Errorf("inbound: publish %s: %w", env.Type, err)
	}
	return nil
}

// markProcessed transitions the webhook_events row to processed. Best
// effort: errors are swallowed because the primary work already succeeded
// and the reconciler picks up stuck rows.
func (s *InboundService) markProcessed(ctx context.Context, eventID integration.WebhookEventID) {
	if eventID == "" {
		return
	}
	_ = s.deps.WebhookEvents.MarkProcessed(ctx, eventID)
}

// markFailed transitions the webhook_events row to failed and stamps the
// error message for debugging. Best effort for the same reason as
// markProcessed.
func (s *InboundService) markFailed(ctx context.Context, eventID integration.WebhookEventID, cause error) {
	if eventID == "" || cause == nil {
		return
	}
	msg := cause.Error()
	if len(msg) > 1024 {
		msg = msg[:1024]
	}
	_ = s.deps.WebhookEvents.MarkFailed(ctx, eventID, msg)
}

// domainStatusFor maps a canonical event Type onto the corresponding
// message.Status. Returns false for events that do not carry a status.
func domainStatusFor(t events.Type) (message.Status, bool) {
	switch t {
	case events.MessageSent:
		return message.StatusSent, true
	case events.MessageDelivered:
		return message.StatusDelivered, true
	case events.MessageRead:
		return message.StatusRead, true
	case events.MessageFailed:
		return message.StatusFailed, true
	default:
		return "", false
	}
}

// identityTypeForProvider maps a provider key onto its canonical identity
// type. WhatsApp identities are E.164 phone numbers under the hood — kept
// as identity.TypePhone so cross-channel merge works out of the box.
func identityTypeForProvider(providerKey string) identity.Type {
	switch providerKey {
	case "whatsapp":
		return identity.TypePhone
	default:
		return identity.TypeExternal
	}
}

// fallbackDisplayName produces a human-readable label when the provider
// didn't supply a profile name. Uses the last 4 digits of the phone.
func fallbackDisplayName(phoneE164 string) string {
	digits := strings.TrimPrefix(phoneE164, "+")
	if len(digits) >= 4 {
		return "Customer " + digits[len(digits)-4:]
	}
	return "Customer"
}

// timePtr returns a pointer to t so caller code can assign optional
// timestamps without introducing an anonymous variable.
func timePtr(t time.Time) *time.Time { return &t }

// isNotFound reports whether err represents a "row does not exist"
// signal from an infrastructure repository. We match by string to avoid
// importing the mysql package from the application layer (dependency
// rule: application → domain + ports only).
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Fast paths: common sentinel patterns.
	if errors.Is(err, errNotFoundSentinel) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no rows")
}

// errNotFoundSentinel is an unexported sentinel wrapped by nothing in this
// package; it exists so isNotFound has an errors.Is target when the
// infrastructure layer starts publishing a shared sentinel. Kept
// unexported to avoid inviting cross-layer coupling in the meantime.
//
// Consumers who want the strict sentinel behaviour can wrap their
// infrastructure error with errors.Join(err, errNotFoundSentinel).
var errNotFoundSentinel = errors.New("row not found")

// downloadInboundMedia streams the provider media bytes into the
// attachment store and stamps the resulting key + content-type + size on
// the message metadata. Failures are logged (WARN) but never returned —
// the message row still persists and the frontend falls back to the
// "attachment unavailable" placeholder. This mirrors the guarantee that a
// transient Meta CDN outage never blocks the inbox from advancing.
//
// When either the Downloader or Store dep is nil the feature is silently
// disabled after emitting a one-shot boot WARN so operators know the
// media pipeline is off.
func (s *InboundService) downloadInboundMedia(
	ctx context.Context,
	providerKey string,
	orgID organization.ID,
	integrationID integration.ID,
	metadata map[string]any,
	payload message.MediaPayload,
) {
	if s.deps.Downloader == nil || s.deps.Attachments == nil {
		s.mediaWarnOnce.Do(func() {
			slog.Default().Warn("inbound media download disabled — Attachments or Downloader dep is nil",
				slog.String("provider", providerKey),
				slog.String("org_id", string(orgID)),
			)
		})
		return
	}
	body, ctype, err := s.deps.Downloader.Download(ctx, providerKey, integrationID, payload.MediaID, payload.URL)
	if err != nil {
		slog.Default().Warn("inbound media download failed",
			slog.String("provider", providerKey),
			slog.String("org_id", string(orgID)),
			slog.String("integration_id", string(integrationID)),
			slog.String("media_id", payload.MediaID),
			slog.Any("err", err),
		)
		return
	}
	defer func() { _ = body.Close() }()

	// Fall back to the payload's MIME type when the transport didn't
	// surface one. Meta's media-download endpoint always sets
	// Content-Type but tests + non-Meta providers may not.
	if ctype == "" {
		ctype = payload.MIMEType
	}
	key, size, _, err := s.deps.Attachments.Put(ctx, ctype, body)
	if err != nil {
		slog.Default().Warn("inbound media store failed",
			slog.String("provider", providerKey),
			slog.String("org_id", string(orgID)),
			slog.String("integration_id", string(integrationID)),
			slog.String("media_id", payload.MediaID),
			slog.Any("err", err),
		)
		return
	}
	metadata["attachment_key"] = key
	metadata["content_type"] = ctype
	metadata["file_size"] = size
}

