package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/contact"
	"github.com/v-senthil/nudgeway/internal/domain/conversation"
	"github.com/v-senthil/nudgeway/internal/domain/events"
	"github.com/v-senthil/nudgeway/internal/domain/identity"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	msgdom "github.com/v-senthil/nudgeway/internal/domain/message"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/channel"
	"github.com/v-senthil/nudgeway/internal/ports/eventbus"
	"github.com/v-senthil/nudgeway/internal/ports/queue"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// SendLane is the queue lane that carries outbound send jobs to the send
// worker. Kept as a package-level const so the REST enqueue path and the
// worker consume path cannot drift.
const SendLane = "message.send"

// ErrConversationNotFound is returned by RequestSend when the target
// conversation does not exist for the caller's org.
var ErrConversationNotFound = errors.New("message: conversation not found")

// ErrInvalidPayload is returned by RequestSend when the request body cannot
// be interpreted as a canonical message payload.
var ErrInvalidPayload = errors.New("message: invalid payload")

// ErrEndpointNotFound is returned when the session's business endpoint row
// has been deleted while a conversation still points at it.
var ErrEndpointNotFound = errors.New("message: business endpoint not found")

// ErrSendIntegrationMissing is returned when the endpoint's integration row
// is missing or belongs to a different org.
var ErrSendIntegrationMissing = errors.New("message: send integration missing")

// IntegrationSecrets exposes decrypted integration secrets to the send
// service. The concrete infra implementation (mysql.Integrations) exposes a
// GetWithSecrets method; the service takes an interface wider than the pure
// repository.IntegrationRepo port so we can decrypt on demand from the
// worker path without leaking secrets through the domain Integration value.
type IntegrationSecrets interface {
	repository.IntegrationRepo

	// GetWithSecrets returns the Integration alongside a map of decrypted
	// secret material keyed by name ("access_token", "phone_number_id",
	// "waba_id" for WhatsApp).
	GetWithSecrets(
		ctx context.Context,
		orgID organization.ID,
		id integration.ID,
	) (integration.Integration, map[string]string, error)
}

// ProviderRegistry looks up a channel.Provider adapter by provider key,
// binding it to a specific integration's decrypted secrets. The application
// service depends on this interface so it never imports a provider package
// directly (the wire-up in cmd/server injects a concrete implementation
// that closes over the provider adapters).
type ProviderRegistry interface {
	// Channel returns a channel.Provider ready to serve SendMessage against
	// the given integration. Implementations construct or memoise the
	// adapter per (providerKey, integrationID) as needed.
	Channel(ctx context.Context, providerKey string, secrets map[string]string) (channel.Provider, error)
}

// IDGenerator mints new message IDs. Kept as a port so tests can inject a
// deterministic generator without importing ulid here.
type IDGenerator interface {
	// NewMessageID returns a fresh globally-unique message id.
	NewMessageID() string
}

// Clock returns the current time. Kept as a port for deterministic tests.
type Clock interface {
	// Now returns the current wall-clock time in UTC.
	Now() time.Time
}

// SendDeps bundles the dependencies of SendService.
type SendDeps struct {
	// Messages persists message rows.
	Messages repository.MessageRepo
	// Conversations resolves the target conversation and its session id.
	Conversations repository.ConversationRepo
	// Sessions resolves the session's business endpoint id and contact id.
	Sessions repository.SessionRepo
	// Endpoints resolves the business endpoint (channel + external id +
	// integration id).
	Endpoints repository.BusinessEndpointRepo
	// Contacts resolves the recipient identity for the outbound send.
	Contacts repository.ContactRepo
	// Identities loads the ContactIdentity behind Contact.PrimaryIdentityID
	// so the outbound "to" is the provider-native address (E.164 phone
	// for WhatsApp) rather than an internal ULID.
	Identities repository.IdentityRepo
	// Integrations resolves integration config + decrypts secrets.
	Integrations IntegrationSecrets
	// Templates is optional. When non-nil the send path looks up the
	// template definition on outbound template sends and stamps the
	// resolved (parameter-substituted) header / body / footer / button
	// text onto the outbound message's metadata.template.resolved field.
	// The frontend prefers this over the raw parameter list; missing
	// lookups are non-fatal (frontend falls back).
	Templates repository.TemplateRepo
	// Enqueuer places SendJobPayload on the "message.send" lane.
	Enqueuer queue.Enqueuer
	// Publisher publishes canonical events on the in-proc bus.
	Publisher eventbus.Publisher
	// Providers resolves the channel adapter for a given (provider, secrets).
	Providers ProviderRegistry
	// IDs mints new message ids.
	IDs IDGenerator
	// Clock returns the current time.
	Clock Clock
	// Logger receives structured records with org_id, request_id,
	// correlation_id, message_id.
	Logger *slog.Logger
}

// SendService implements the outbound send use-case. RequestSend is the REST
// entry point: it validates the request, persists a QUEUED Message row,
// enqueues a job on the "message.send" lane, publishes MessageSendRequested,
// and returns. The provider adapter is never called on this path.
//
// ProcessSend is the worker entry point: it loads the message, resolves the
// integration + provider adapter, calls SendMessage, updates the message
// status, and publishes MessageSent / MessageFailed accordingly. Transient
// errors bubble up so the queue can retry with backoff; permanent errors
// mark the message failed and return nil so the queue moves on.
type SendService struct {
	deps SendDeps
}

// NewSendService constructs a SendService. Panics on nil required deps —
// this is a wire-up bug and should fail loudly.
func NewSendService(deps SendDeps) *SendService {
	if deps.Messages == nil || deps.Conversations == nil || deps.Sessions == nil ||
		deps.Endpoints == nil || deps.Integrations == nil || deps.Enqueuer == nil ||
		deps.Publisher == nil || deps.Providers == nil || deps.IDs == nil ||
		deps.Clock == nil {
		panic("message.NewSendService: missing required dependency")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &SendService{deps: deps}
}

// RequestSend is the REST entry point. See SendService for the contract.
//
// The steps run in strict order so a crash between them is recoverable:
//  1. validate request shape;
//  2. resolve conversation → session → endpoint → integration (org-scoped);
//  3. INSERT Message(QUEUED);
//  4. enqueue on SendLane;
//  5. publish MessageSendRequested.
//
// Steps 4 and 5 fire after the row commits, so a crash between step 3 and
// step 4 leaves a queued row for a sweeper to re-enqueue.
func (s *SendService) RequestSend(ctx context.Context, req SendRequest) (SendResponse, error) {
	// 1. Validate.
	if req.OrgID == "" {
		return SendResponse{}, fmt.Errorf("%w: missing org", ErrInvalidPayload)
	}
	if req.ConversationID == "" {
		return SendResponse{}, fmt.Errorf("%w: missing conversation_id", ErrInvalidPayload)
	}
	if req.Type == "" {
		return SendResponse{}, fmt.Errorf("%w: missing type", ErrInvalidPayload)
	}
	if err := validatePayload(msgdom.Type(req.Type), req.Payload); err != nil {
		return SendResponse{}, err
	}

	orgID := organization.ID(req.OrgID)

	// 2. Resolve the target conversation → session → endpoint → integration.
	conv, err := s.deps.Conversations.Get(ctx, orgID, conversation.ID(req.ConversationID))
	if err != nil {
		return SendResponse{}, fmt.Errorf("%w: %w", ErrConversationNotFound, err)
	}
	sess, err := s.deps.Sessions.Get(ctx, orgID, conv.SessionID)
	if err != nil {
		return SendResponse{}, fmt.Errorf("resolve session: %w", err)
	}
	ep, err := s.deps.Endpoints.Get(ctx, orgID, sess.BusinessEndpointID)
	if err != nil {
		return SendResponse{}, fmt.Errorf("%w: %w", ErrEndpointNotFound, err)
	}
	if ep.IntegrationID == "" {
		return SendResponse{}, fmt.Errorf("%w: endpoint has no integration", ErrSendIntegrationMissing)
	}
	integ, err := s.deps.Integrations.Get(ctx, orgID, integration.ID(ep.IntegrationID))
	if err != nil {
		return SendResponse{}, fmt.Errorf("%w: %w", ErrSendIntegrationMissing, err)
	}

	// Recipient identity: derive from the Contact if available, else from
	// the session's contact id via ContactRepo. We fall back to the raw
	// contact id when no primary identity is set — the provider adapter
	// will reject an invalid To at send time.
	recipient, err := s.resolveRecipient(ctx, orgID, sess.ContactID)
	if err != nil {
		return SendResponse{}, fmt.Errorf("resolve recipient: %w", err)
	}

	// 3. Insert Message(QUEUED). The ID is minted here so the queue payload
	// can reference it.
	now := s.deps.Clock.Now().UTC()
	msgID := msgdom.ID(s.deps.IDs.NewMessageID())
	row := msgdom.Message{
		ID:                msgID,
		OrgID:             orgID,
		ContactID:         sess.ContactID,
		SessionID:         sess.ID,
		ConversationID:    conv.ID,
		Channel:           ep.Channel,
		Provider:          ep.Provider,
		Direction:         msgdom.DirectionOutbound,
		SenderIdentity:    ep.ExternalID,
		RecipientIdentity: recipient,
		MessageType:       msgdom.Type(req.Type),
		Status:            msgdom.StatusQueued,
		CreatedAt:         now,
		Metadata:          map[string]any{"idempotency_key": req.IdempotencyKey},
	}
	// Surface text / media_url into Metadata so the REST DTO can render the
	// outbound bubble without a re-fetch. Inbound does the same in
	// InboundService.
	if req.Type == string(msgdom.TypeText) && len(req.Payload) > 0 {
		var t struct {
			Body string `json:"body"`
		}
		if json.Unmarshal(req.Payload, &t) == nil && t.Body != "" {
			row.Metadata["text"] = t.Body
		}
	}
	// Persist the full template payload so the outbound bubble can render
	// the template name, language, and parameters without a re-fetch.
	if req.Type == string(msgdom.TypeTemplate) && len(req.Payload) > 0 {
		var tpl map[string]any
		if json.Unmarshal(req.Payload, &tpl) == nil && len(tpl) > 0 {
			// Best-effort: look up the stored template definition and
			// interpolate parameter values into the header/body/footer/
			// button text so the frontend can render an authentic
			// WhatsApp-style bubble. Failures are non-fatal — the
			// frontend falls back to the raw parameter list when
			// "resolved" is missing.
			if s.deps.Templates != nil {
				if resolved := s.buildResolvedTemplate(ctx, orgID, integration.ID(ep.IntegrationID), tpl); resolved != nil {
					tpl["resolved"] = resolved
				}
			}
			row.Metadata["template"] = tpl
		}
	}
	if isMediaType(req.Type) && len(req.Payload) > 0 {
		var mp struct {
			MediaID  string `json:"media_id"`
			URL      string `json:"url"`
			Caption  string `json:"caption"`
			FileName string `json:"filename"`
		}
		if json.Unmarshal(req.Payload, &mp) == nil {
			if mp.URL != "" {
				row.Metadata["media_url"] = mp.URL
			}
			if mp.MediaID != "" {
				row.Metadata["media_id"] = mp.MediaID
			}
			if mp.Caption != "" {
				row.Metadata["text"] = mp.Caption
			}
			if mp.FileName != "" {
				row.Metadata["filename"] = mp.FileName
			}
		}
	}
	if err := s.deps.Messages.Create(ctx, row); err != nil {
		return SendResponse{}, fmt.Errorf("persist message: %w", err)
	}

	// 4. Enqueue.
	corr := req.CorrelationID
	if corr == "" {
		corr = req.RequestID
	}
	job := SendJobPayload{
		MessageID:      string(msgID),
		OrgID:          string(orgID),
		IntegrationID:  string(integ.ID),
		ProviderKey:    integ.Provider,
		Recipient:      recipient,
		Type:           req.Type,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  corr,
		RequestID:      req.RequestID,
	}
	body, err := json.Marshal(job)
	if err != nil {
		return SendResponse{}, fmt.Errorf("encode send job: %w", err)
	}
	if _, err := s.deps.Enqueuer.Enqueue(ctx, queue.Job{
		Lane:    SendLane,
		Payload: body,
	}); err != nil {
		// The row is persisted with status=queued; a sweeper on
		// (org_id, status=queued) will re-enqueue. We still surface the
		// error so the caller sees the transient failure.
		s.deps.Logger.Error("message.send enqueue failed (row persisted)",
			slog.String("org_id", string(orgID)),
			slog.String("message_id", string(msgID)),
			slog.String("request_id", req.RequestID),
			slog.Any("err", err),
		)
		return SendResponse{}, fmt.Errorf("enqueue send job: %w", err)
	}

	// 5. Publish MessageSendRequested on the in-proc bus so anything else
	// (analytics, audit) can react without blocking the response.
	_ = s.deps.Publisher.Publish(ctx, events.Envelope{
		Type:           events.MessageSendRequested,
		OrganizationID: string(orgID),
		OccurredAt:     now,
		CorrelationID:  corr,
		CausationID:    req.RequestID,
		Payload: events.MessageStatusPayload{
			Provider:  integ.Provider,
			Channel:   ep.Channel,
			Recipient: recipient,
			Status:    string(msgdom.StatusQueued),
			Timestamp: now,
		},
	})

	s.deps.Logger.Info("message.send requested",
		slog.String("org_id", string(orgID)),
		slog.String("message_id", string(msgID)),
		slog.String("conversation_id", string(conv.ID)),
		slog.String("integration_id", string(integ.ID)),
		slog.String("provider", integ.Provider),
		slog.String("request_id", req.RequestID),
	)

	return SendResponse{MessageID: string(msgID), Status: string(msgdom.StatusQueued)}, nil
}

// ProcessSend is the worker entry point. It is invoked by the send worker
// once per queue.Job on the message.send lane. Transient errors (rate
// limits, transport failures) are returned so the queue retries with
// backoff. Permanent errors (auth, validation) mark the message failed and
// return nil.
func (s *SendService) ProcessSend(ctx context.Context, job SendJobPayload) error {
	orgID := organization.ID(job.OrgID)

	// Look up integration + secrets.
	integ, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integration.ID(job.IntegrationID))
	if err != nil {
		return fmt.Errorf("load integration: %w", err)
	}
	if integ.Provider != job.ProviderKey {
		return fmt.Errorf("integration provider mismatch: row=%q job=%q", integ.Provider, job.ProviderKey)
	}

	// Merge non-secret config keys the provider factory needs (e.g. WhatsApp
	// phone_number_id + waba_id live in integration.Config, not the
	// encrypted secrets bag). The factory takes a single flat map for
	// backwards compatibility with the port; do the merge here.
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)
	// Resolve provider adapter.
	provider, err := s.deps.Providers.Channel(ctx, integ.Provider, secrets)
	if err != nil {
		return fmt.Errorf("resolve provider %q: %w", integ.Provider, err)
	}

	// Call the adapter. Idempotency key = message id when the caller did
	// not supply one; provider adapters that support opaque callback data
	// forward this so downstream status webhooks can be reconciled without
	// a client round-trip.
	idem := job.IdempotencyKey
	if idem == "" {
		idem = job.MessageID
	}
	res, sendErr := provider.SendMessage(ctx, channel.SendRequest{
		OrganizationID: job.OrgID,
		IntegrationID:  job.IntegrationID,
		To:             job.Recipient,
		MessageType:    job.Type,
		Body:           job.Payload,
		IdempotencyKey: idem,
	})

	now := s.deps.Clock.Now().UTC()
	log := s.deps.Logger.With(
		slog.String("org_id", job.OrgID),
		slog.String("message_id", job.MessageID),
		slog.String("integration_id", job.IntegrationID),
		slog.String("provider", integ.Provider),
		slog.String("correlation_id", job.CorrelationID),
	)

	if sendErr != nil {
		// Classify: retryable => bubble up; permanent => mark FAILED.
		if isRetryable(sendErr) {
			log.Warn("message.send transient failure; will retry",
				slog.Any("err", sendErr),
			)
			return fmt.Errorf("send transient: %w", sendErr)
		}
		if err := s.deps.Messages.UpdateStatus(ctx, orgID, msgdom.ID(job.MessageID), msgdom.StatusFailed, now); err != nil {
			log.Error("message.send failed status write",
				slog.Any("err", err),
			)
		}
		_ = s.deps.Publisher.Publish(ctx, events.Envelope{
			Type:           events.MessageFailed,
			OrganizationID: job.OrgID,
			OccurredAt:     now,
			CorrelationID:  job.CorrelationID,
			CausationID:    job.RequestID,
			Payload: events.MessageStatusPayload{
				Provider:     integ.Provider,
				Recipient:    job.Recipient,
				Status:       string(msgdom.StatusFailed),
				Timestamp:    now,
				ErrorMessage: sendErr.Error(),
			},
		})
		log.Warn("message.send permanent failure",
			slog.Any("err", sendErr),
		)
		return nil // do not requeue permanent failures
	}

	// Success: update status → SENT then stamp the provider_message_id
	// (wamid) so later status callbacks (delivered / read / failed) can
	// route via UpdateStatusByProviderMessageID and advance the UI.
	if err := s.deps.Messages.UpdateStatus(ctx, orgID, msgdom.ID(job.MessageID), msgdom.StatusSent, now); err != nil {
		log.Error("message.send status=sent write failed",
			slog.Any("err", err),
		)
		return fmt.Errorf("mark sent: %w", err)
	}
	if setter, ok := s.deps.Messages.(interface {
		SetProviderMessageID(ctx context.Context, orgID organization.ID, id msgdom.ID, providerMessageID string) error
	}); ok && res.ProviderMessageID != "" {
		if err := setter.SetProviderMessageID(ctx, orgID, msgdom.ID(job.MessageID), res.ProviderMessageID); err != nil {
			log.Warn("message.send: SetProviderMessageID failed; delivered/read callbacks will not route",
				slog.String("provider_message_id", res.ProviderMessageID),
				slog.Any("err", err),
			)
		}
	}
	_ = s.deps.Publisher.Publish(ctx, events.Envelope{
		Type:           events.MessageSent,
		OrganizationID: job.OrgID,
		OccurredAt:     now,
		CorrelationID:  job.CorrelationID,
		CausationID:    job.RequestID,
		Payload: events.MessageStatusPayload{
			Provider:          integ.Provider,
			ProviderMessageID: res.ProviderMessageID,
			Recipient:         job.Recipient,
			Status:            string(msgdom.StatusSent),
			Timestamp:         now,
		},
	})
	log.Info("message.send sent",
		slog.String("provider_message_id", res.ProviderMessageID),
	)
	return nil
}

// resolveRecipient loads the contact and returns its recipient identity.
// The recipient string is provider-neutral (canonical E.164 / email / wa_id
// with plus). When the contact record has no PrimaryIdentityID set we fall
// back to the contact display name — the provider adapter validates the
// address at send time so this stays a non-fatal path.
func (s *SendService) resolveRecipient(ctx context.Context, orgID organization.ID, contactID contact.ID) (string, error) {
	if s.deps.Contacts == nil {
		return string(contactID), nil
	}
	c, err := s.deps.Contacts.Get(ctx, orgID, contactID)
	if err != nil {
		return "", err
	}
	// Preference order for the outbound "to" field:
	//   1. Phone / wa_id — universally accepted by Meta today.
	//   2. BSUID (once WhatsApp completes the username rollout Meta will
	//      accept this everywhere; for now some BSUIDs still 4xx, hence
	//      the fallback).
	//   3. The Contact's primary identity (may be BSUID after our
	//      InboundService promotes it).
	//   4. Display name — providers can't route to it, but safer than
	//      leaking an internal ULID.
	// TODO: promote BSUID to (1) once Meta portfolio-side send accepts
	// all BSUIDs. See ~/Documents/whatsapp_doc_tracker/docs/
	// business-scoped-user-ids.md.
	if s.deps.Identities != nil {
		list, err := s.deps.Identities.ListForContact(ctx, orgID, contactID)
		if err == nil {
			for _, id := range list {
				if (id.Type == identity.TypePhone || id.Type == identity.TypeWhatsApp) && id.NormalizedValue != "" {
					return id.NormalizedValue, nil
				}
			}
			for _, id := range list {
				if id.Type == identity.TypeBSUID && id.NormalizedValue != "" {
					return id.NormalizedValue, nil
				}
			}
		}
	}
	if c.PrimaryIdentityID == nil {
		if s.deps.Identities != nil {
			list, err := s.deps.Identities.ListForContact(ctx, orgID, contactID)
			if err == nil && len(list) > 0 {
				return list[0].NormalizedValue, nil
			}
		}
		return c.DisplayName, nil
	}
	if s.deps.Identities == nil {
		return string(*c.PrimaryIdentityID), nil
	}
	ident, err := s.deps.Identities.Get(ctx, orgID, identity.ID(*c.PrimaryIdentityID))
	if err != nil {
		return "", fmt.Errorf("load identity %q: %w", *c.PrimaryIdentityID, err)
	}
	return ident.NormalizedValue, nil
}

// validatePayload sanity-checks the JSON envelope for the given canonical
// message type. Full field validation is delegated to the provider adapter;
// this layer only rejects obviously malformed input (empty payloads, wrong
// shapes) so the queue does not accumulate poison messages.
func validatePayload(t msgdom.Type, raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: empty payload for type %q", ErrInvalidPayload, t)
	}
	// The payload must be a JSON object; adapters unmarshal into a shape
	// matching the type.
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}
	switch t {
	case msgdom.TypeText:
		if _, ok := probe["body"]; !ok {
			return fmt.Errorf("%w: text payload missing 'body'", ErrInvalidPayload)
		}
	case msgdom.TypeTemplate:
		if _, ok := probe["name"]; !ok {
			return fmt.Errorf("%w: template payload missing 'name'", ErrInvalidPayload)
		}
	case msgdom.TypeImage, msgdom.TypeVideo, msgdom.TypeAudio,
		msgdom.TypeDocument, msgdom.TypeSticker:
		_, hasID := probe["media_id"]
		_, hasURL := probe["url"]
		if !hasID && !hasURL {
			return fmt.Errorf("%w: media payload requires media_id or url", ErrInvalidPayload)
		}
	}
	return nil
}

// isMediaType reports whether the canonical Type carries a MediaPayload.
func isMediaType(t string) bool {
	switch msgdom.Type(t) {
	case msgdom.TypeImage, msgdom.TypeVideo, msgdom.TypeAudio,
		msgdom.TypeDocument, msgdom.TypeSticker:
		return true
	}
	return false
}

// Retryable is implemented by provider errors that want the send worker to
// requeue rather than mark the message FAILED. Provider adapters
// (whatsapp.APIError, etc.) satisfy this without the application layer
// needing to import their concrete types.
type Retryable interface {
	Retryable() bool
}

// buildResolvedTemplate looks up the stored template definition matching
// the outbound send payload's (name, language.code) and interpolates the
// send-time parameter values into the header / body / footer / button
// text. It returns a map shaped like:
//
//	{
//	  "header":  "Support of Order No: ON-12345",
//	  "body":    "Hi Senthil, welcome",
//	  "footer":  "Reply STOP",
//	  "buttons": [{"type":"URL","text":"View sale","url":"https://…"}]
//	}
//
// The result is stamped onto row.Metadata["template"]["resolved"] so the
// frontend can render a WhatsApp-style bubble instead of a bland
// parameter list. Any lookup or shape mismatch yields nil (best-effort;
// the frontend falls back to the raw parameter list).
func (s *SendService) buildResolvedTemplate(
	ctx context.Context,
	orgID organization.ID,
	integrationID integration.ID,
	tpl map[string]any,
) map[string]any {
	name, _ := tpl["name"].(string)
	if name == "" {
		return nil
	}
	langCode := ""
	if lang, ok := tpl["language"].(map[string]any); ok {
		langCode, _ = lang["code"].(string)
	}
	if langCode == "" {
		if lang, ok := tpl["language"].(string); ok {
			langCode = lang
		}
	}
	if langCode == "" {
		return nil
	}
	def, err := s.deps.Templates.FindByNameLanguage(ctx, orgID, integrationID, name, langCode)
	if err != nil {
		s.deps.Logger.Debug("template resolve: definition not found",
			slog.String("org_id", string(orgID)),
			slog.String("integration_id", string(integrationID)),
			slog.String("template_name", name),
			slog.String("language", langCode),
			slog.Any("err", err),
		)
		return nil
	}

	// Index the send-time components by type ("header" / "body" / "button")
	// so we can pull parameters when we walk the definition.
	sendComps := map[string][]map[string]any{}
	if raw, ok := tpl["components"].([]any); ok {
		for _, c := range raw {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			ct, _ := cm["type"].(string)
			ct = strings.ToLower(ct)
			if ct == "" {
				continue
			}
			sendComps[ct] = append(sendComps[ct], cm)
		}
	}

	// Build the resolved output by walking the definition components in
	// order and matching each to its send-time counterpart.
	resolved := map[string]any{}
	var buttons []map[string]any
	// A rolling index per type so the Nth button/header/body in the
	// definition matches the Nth in the payload.
	nextByType := map[string]int{}
	for _, dc := range def.Components {
		typ := strings.ToLower(dc.Type)
		switch typ {
		case "header":
			// Only interpolate TEXT headers — media headers surface a
			// placeholder label instead.
			if strings.EqualFold(dc.Format, "TEXT") || dc.Format == "" {
				params := pickSendParams(sendComps, "header", nextByType)
				resolved["header"] = substituteTemplate(dc.Text, params, dc.Example)
			} else {
				resolved["header"] = "[" + strings.ToLower(dc.Format) + " attachment]"
			}
		case "body":
			params := pickSendParams(sendComps, "body", nextByType)
			resolved["body"] = substituteTemplate(dc.Text, params, dc.Example)
		case "footer":
			// Footer never carries variables in Meta's schema, but keep
			// the substitution call for symmetry.
			resolved["footer"] = substituteTemplate(dc.Text, nil, dc.Example)
		case "buttons":
			for i, btn := range dc.Buttons {
				out := map[string]any{}
				for k, v := range btn {
					out[k] = v
				}
				// Buttons in the payload each declare their own component
				// with sub_type ("url" / "quick_reply" / "copy_code" / …)
				// and an index. We match by index when present; otherwise
				// fall back to positional order.
				if match := findButtonSendComp(sendComps["button"], i); match != nil {
					if url, ok := btn["url"].(string); ok && url != "" {
						out["url"] = substituteButtonURL(url, match)
					}
					if txt, ok := btn["text"].(string); ok {
						out["text"] = txt
					}
				}
				buttons = append(buttons, out)
			}
		}
	}
	if len(buttons) > 0 {
		resolved["buttons"] = buttons
	}
	return resolved
}

// pickSendParams returns the next send-time parameter list for the given
// component type ("header" / "body"). The nextByType map is mutated so
// successive calls advance through repeated components.
func pickSendParams(sendComps map[string][]map[string]any, typ string, nextByType map[string]int) []any {
	list := sendComps[typ]
	i := nextByType[typ]
	nextByType[typ] = i + 1
	if i >= len(list) {
		return nil
	}
	params, _ := list[i]["parameters"].([]any)
	return params
}

// findButtonSendComp returns the send-time button component matching the
// given definition index. Meta's send payload tags each button component
// with an "index" field; when absent we fall back to positional order.
func findButtonSendComp(comps []map[string]any, defIndex int) map[string]any {
	for _, c := range comps {
		switch idx := c["index"].(type) {
		case float64:
			if int(idx) == defIndex {
				return c
			}
		case string:
			if n, err := strconv.Atoi(idx); err == nil && n == defIndex {
				return c
			}
		}
	}
	if defIndex >= 0 && defIndex < len(comps) {
		return comps[defIndex]
	}
	return nil
}

// substituteButtonURL swaps the first {{1}} placeholder in a button URL
// with the first text parameter of the send-time button component. URL
// buttons in Meta's schema only support one dynamic path/query segment.
func substituteButtonURL(url string, sendBtn map[string]any) string {
	params, _ := sendBtn["parameters"].([]any)
	if len(params) == 0 {
		return url
	}
	first, _ := params[0].(map[string]any)
	val, _ := first["text"].(string)
	if val == "" {
		return url
	}
	return strings.ReplaceAll(url, "{{1}}", val)
}

// substituteTemplate replaces {{n}} (1-indexed positional) and
// {{name}} (named) placeholders in text with the corresponding
// parameter value. Positional is the primary path — Meta's send payload
// carries a parameters array whose Nth entry backs {{n+1}}. Named
// placeholders are resolved via the definition's Example map
// (body_text_named_params) when the send payload omits them.
func substituteTemplate(text string, params []any, example map[string]any) string {
	if text == "" {
		return ""
	}
	// Positional: build a slice of string values from the params array.
	positional := make([]string, 0, len(params))
	named := map[string]string{}
	for _, p := range params {
		pm, ok := p.(map[string]any)
		if !ok {
			positional = append(positional, "")
			continue
		}
		val, _ := pm["text"].(string)
		if val == "" {
			// Fall back to type hint for media parameters.
			if t, _ := pm["type"].(string); t != "" && t != "text" {
				val = "<" + t + ">"
			}
		}
		positional = append(positional, val)
		if name, ok := pm["parameter_name"].(string); ok && name != "" {
			named[name] = val
		}
	}
	// Seed named map from the template's example if not supplied at
	// send time. Meta stores named-parameter examples under
	// example.body_text_named_params = [{param_name, example}, …].
	if example != nil {
		if list, ok := example["body_text_named_params"].([]any); ok {
			for _, item := range list {
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				pn, _ := im["param_name"].(string)
				if pn == "" {
					continue
				}
				if _, seen := named[pn]; seen {
					continue
				}
				ex, _ := im["example"].(string)
				named[pn] = ex
			}
		}
	}
	return interpolatePlaceholders(text, positional, named)
}

// interpolatePlaceholders walks the string and replaces {{token}} where
// token is either a 1-based integer index into positional or a name in
// named. Unknown tokens are left untouched so the operator can spot the
// mismatch.
func interpolatePlaceholders(text string, positional []string, named map[string]string) string {
	var b strings.Builder
	b.Grow(len(text))
	i := 0
	for i < len(text) {
		if i+1 < len(text) && text[i] == '{' && text[i+1] == '{' {
			end := strings.Index(text[i+2:], "}}")
			if end < 0 {
				b.WriteString(text[i:])
				break
			}
			token := strings.TrimSpace(text[i+2 : i+2+end])
			replaced := false
			if n, err := strconv.Atoi(token); err == nil && n >= 1 && n <= len(positional) {
				b.WriteString(positional[n-1])
				replaced = true
			} else if v, ok := named[token]; ok {
				b.WriteString(v)
				replaced = true
			}
			if !replaced {
				b.WriteString(text[i : i+2+end+2])
			}
			i += 2 + end + 2
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

// isRetryable inspects err via errors.As for anything satisfying Retryable.
// Falls back to false so an unknown error defaults to permanent failure —
// safer than an infinite retry loop.
func isRetryable(err error) bool {
	var r Retryable
	if errors.As(err, &r) {
		return r.Retryable()
	}
	return false
}
