package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/fullwa/fullwa/internal/domain/contact"
	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/events"
	"github.com/fullwa/fullwa/internal/domain/identity"
	"github.com/fullwa/fullwa/internal/domain/integration"
	msgdom "github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/ports/channel"
	"github.com/fullwa/fullwa/internal/ports/eventbus"
	"github.com/fullwa/fullwa/internal/ports/queue"
	"github.com/fullwa/fullwa/internal/ports/repository"
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
	// bubble without dereferencing HBase (payload storage) later. Inbound
	// does the same in InboundService — outbound must match.
	if req.Type == string(msgdom.TypeText) && len(req.Payload) > 0 {
		var t struct {
			Body string `json:"body"`
		}
		if json.Unmarshal(req.Payload, &t) == nil && t.Body != "" {
			row.Metadata["text"] = t.Body
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

	// Success: update status → SENT and record provider_message_id via the
	// same UpdateStatus call (the underlying MySQL repo also persists the
	// provider_message_id when non-empty on the row; we update the id via
	// the repo's status write which stamps SentAt).
	if err := s.deps.Messages.UpdateStatus(ctx, orgID, msgdom.ID(job.MessageID), msgdom.StatusSent, now); err != nil {
		log.Error("message.send status=sent write failed",
			slog.Any("err", err),
		)
		return fmt.Errorf("mark sent: %w", err)
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
	if c.PrimaryIdentityID == nil {
		// No primary identity — fall back to any identity we can find
		// for this contact. Better than returning the display name,
		// which providers can't route to.
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

// Retryable is implemented by provider errors that want the send worker to
// requeue rather than mark the message FAILED. Provider adapters
// (whatsapp.APIError, etc.) satisfy this without the application layer
// needing to import their concrete types.
type Retryable interface {
	Retryable() bool
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
