// Package message — read.go implements the mark-as-read use-case.
//
// The operator's UI opens a conversation → the frontend calls
// POST /api/v1/messages/{id}/read (or the batch variant), which drives
// ReadService.MarkRead / MarkConversationRead. For each inbound message
// with a provider_message_id (Meta's wamid), we call the channel adapter's
// MarkAsRead — Meta then flips the customer's client to blue-tick.
//
// Meta does not fan a status callback back to us for a business-side read
// (the read event exists only on the customer's side), so we stamp
// read_at locally when the mark succeeds. This gives the inbox a durable
// "you have already replied / acknowledged this" indicator.
package message

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fullwa/fullwa/internal/domain/conversation"
	"github.com/fullwa/fullwa/internal/domain/integration"
	msgdom "github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// ErrMessageNotFound is returned by MarkRead when the target message row
// does not exist for the caller's org.
var ErrMessageNotFound = errors.New("message: message not found")

// ErrReadIntegrationMissing is returned when the message's owning
// integration cannot be resolved (deleted / cross-tenant).
var ErrReadIntegrationMissing = errors.New("message: read integration missing")

// ReadDeps bundles the dependencies of ReadService. All fields except
// Logger are required.
type ReadDeps struct {
	// Messages loads the target message + writes the local read_at stamp.
	Messages repository.MessageRepo
	// Conversations resolves conversation → session id (batch variant).
	Conversations repository.ConversationRepo
	// Sessions resolves session → business endpoint id.
	Sessions repository.SessionRepo
	// Endpoints resolves the business endpoint → integration id.
	Endpoints repository.BusinessEndpointRepo
	// Integrations resolves integration + decrypts secrets so the adapter
	// can authenticate to the provider.
	Integrations IntegrationSecrets
	// Providers resolves the channel adapter for a given (provider, secrets).
	Providers ProviderRegistry
	// Clock returns the current time; stamped into local read_at.
	Clock Clock
	// Logger receives structured records with org_id, message_id.
	Logger *slog.Logger
}

// ReadService implements the mark-as-read use-case. See package doc.
type ReadService struct {
	deps ReadDeps
}

// NewReadService constructs a ReadService. Panics on missing required
// dependencies — wire-up misconfiguration should fail loudly.
func NewReadService(deps ReadDeps) *ReadService {
	if deps.Messages == nil || deps.Conversations == nil || deps.Sessions == nil ||
		deps.Endpoints == nil || deps.Integrations == nil || deps.Providers == nil ||
		deps.Clock == nil {
		panic("message.NewReadService: missing required dependency")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &ReadService{deps: deps}
}

// MarkRead marks a single inbound message as read to the provider. It is a
// no-op (nil error) when:
//   - the message is outbound (only inbound can be read-receipted);
//   - the message has no provider_message_id yet;
//   - the message already has a local read_at stamp (idempotent).
//
// Returns ErrMessageNotFound for missing rows.
func (s *ReadService) MarkRead(ctx context.Context, orgID organization.ID, messageID msgdom.ID) error {
	msg, err := s.deps.Messages.Get(ctx, orgID, messageID)
	if err != nil {
		if isReadNotFound(err) {
			return fmt.Errorf("%w: %s", ErrMessageNotFound, messageID)
		}
		return fmt.Errorf("mark read: load message %s: %w", messageID, err)
	}
	return s.markOne(ctx, orgID, msg)
}

// MarkConversationRead marks every unread inbound message in the
// conversation as read, capped at the newest `cap` rows (default 50).
// Failures on individual messages are logged but do not abort the batch;
// the first infrastructure error (adapter resolution, DB write) is
// returned so the caller can surface it.
func (s *ReadService) MarkConversationRead(ctx context.Context, orgID organization.ID, convID conversation.ID, cap int) (int, error) {
	if cap <= 0 || cap > 100 {
		cap = 50
	}
	page, err := s.deps.Messages.ListByConversation(ctx, orgID, convID, repository.MessageListFilter{Limit: cap})
	if err != nil {
		return 0, fmt.Errorf("mark conversation read: list: %w", err)
	}
	var firstErr error
	count := 0
	for _, msg := range page.Messages {
		if msg.Direction != msgdom.DirectionInbound {
			continue
		}
		if msg.ProviderMessageID == "" {
			continue
		}
		if msg.ReadAt != nil {
			continue
		}
		if err := s.markOne(ctx, orgID, msg); err != nil {
			s.deps.Logger.Warn("mark conversation read: per-message failure",
				slog.String("org_id", string(orgID)),
				slog.String("conversation_id", string(convID)),
				slog.String("message_id", string(msg.ID)),
				slog.Any("err", err),
			)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		count++
	}
	return count, firstErr
}

// markOne resolves the provider + calls MarkAsRead + stamps read_at.
// Skips outbound / no-wamid / already-read messages as documented on
// MarkRead.
func (s *ReadService) markOne(ctx context.Context, orgID organization.ID, msg msgdom.Message) error {
	if msg.Direction != msgdom.DirectionInbound {
		return nil
	}
	if msg.ProviderMessageID == "" {
		return nil
	}
	if msg.ReadAt != nil {
		return nil
	}

	// Resolve conversation → session → endpoint → integration.
	conv, err := s.deps.Conversations.Get(ctx, orgID, msg.ConversationID)
	if err != nil {
		return fmt.Errorf("mark read: load conversation %s: %w", msg.ConversationID, err)
	}
	sess, err := s.deps.Sessions.Get(ctx, orgID, conv.SessionID)
	if err != nil {
		return fmt.Errorf("mark read: load session %s: %w", conv.SessionID, err)
	}
	ep, err := s.deps.Endpoints.Get(ctx, orgID, sess.BusinessEndpointID)
	if err != nil {
		return fmt.Errorf("mark read: load endpoint %s: %w", sess.BusinessEndpointID, err)
	}
	if ep.IntegrationID == "" {
		return fmt.Errorf("%w: endpoint has no integration", ErrReadIntegrationMissing)
	}
	integ, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integration.ID(ep.IntegrationID))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrReadIntegrationMissing, err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	// Mirror send.go: phone_number_id + waba_id live in Integration.Config,
	// not the envelope-encrypted secrets bag.
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)
	provider, err := s.deps.Providers.Channel(ctx, integ.Provider, secrets)
	if err != nil {
		return fmt.Errorf("mark read: resolve provider %q: %w", integ.Provider, err)
	}

	if err := provider.MarkAsRead(ctx, msg.ProviderMessageID); err != nil {
		return fmt.Errorf("mark read: provider call: %w", err)
	}

	// Local stamp. Meta does not deliver a "you-marked-it-read" status
	// callback, so we record the timestamp ourselves for idempotency and
	// for the inbox unread-count computation.
	now := s.deps.Clock.Now().UTC()
	if err := s.deps.Messages.UpdateStatus(ctx, orgID, msg.ID, msgdom.StatusRead, now); err != nil {
		s.deps.Logger.Warn("mark read: local read_at write failed",
			slog.String("org_id", string(orgID)),
			slog.String("message_id", string(msg.ID)),
			slog.Any("err", err),
		)
	}
	return nil
}

// isReadNotFound classifies a MessageRepo.Get error as row-missing without
// importing the mysql package (dependency rule).
func isReadNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "not found") || contains(msg, "no rows")
}

