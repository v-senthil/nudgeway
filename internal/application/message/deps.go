package message

import (
	"context"
	"time"

	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/message"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/ports/channel"
	"github.com/fullwa/fullwa/internal/ports/eventbus"
	"github.com/fullwa/fullwa/internal/ports/repository"
)

// IntegrationSecretsRepo is the subset of the integration repository that
// the inbound service needs — the "read the integration row plus its
// decrypted secret material" call. The concrete implementation lives in
// internal/infrastructure/mysql alongside repository.IntegrationRepo.
// Declaring it here keeps the application service testable with a fake
// without dragging the full mutation surface in.
type IntegrationSecretsRepo interface {
	// GetWithSecrets fetches an integration by ID (no org scoping — the
	// caller is a webhook worker that only has an integration_id in hand)
	// and populates the envelope-decrypted secret material referenced by
	// CredentialsRef. Returns a not-found error when the row is missing.
	GetWithSecrets(ctx context.Context, id integration.ID) (row integration.Integration, secrets map[string]string, err error)
}

// MessageStatusByProviderID advances a message's Status by looking it up
// via (org_id, provider_message_id) instead of the internal row ID.
//
// The baseline MessageRepo.UpdateStatus expects the internal message.ID,
// which the inbound flow does not have; provider status callbacks arrive
// keyed only by wamid. Implementations live alongside the primary
// MessageRepo in internal/infrastructure/mysql and rely on the
// UNIQUE(org_id, provider_message_id) index to resolve the row.
type MessageStatusByProviderID interface {
	// UpdateStatusByProviderMessageID advances a message's status by
	// (org, provider_message_id). Idempotent: applying the same terminal
	// status twice returns nil. Missing rows return an error the caller
	// classifies as permanent — Meta occasionally delivers status
	// callbacks before the original send is durably recorded, and the
	// reconciler picks those up on a later pass.
	UpdateStatusByProviderMessageID(
		ctx context.Context,
		orgID organization.ID,
		providerMessageID string,
		next message.Status,
		at time.Time,
	) error
}

// ChannelProviderLookup resolves a provider-registry key to its runtime
// channel.Provider implementation. The webhook package owns the concrete
// map; the application service takes the lookup as a plain function so it
// does not import any provider package (dependency-rule compliant).
type ChannelProviderLookup func(providerKey string) (channel.Provider, bool)

// IDGen mints new IDs for freshly-allocated Contacts and Messages. Kept
// as a plain function so the application layer does not depend on a
// specific ULID / UUID vendor.
type IDGen func() string

// Deps bundles every port the InboundService depends on. Wire it once at
// cmd/server boot; the service holds a copy and never mutates it.
type Deps struct {
	// Integrations reads an integration row plus its decrypted secrets so
	// the service can derive org_id + the provider adapter.
	Integrations IntegrationSecretsRepo
	// WebhookEvents marks the delivery row processed / failed after
	// application-side persistence completes.
	WebhookEvents repository.WebhookEventRepo
	// BusinessEndpoints resolves phone_number_id → the tenant's endpoint row.
	BusinessEndpoints repository.BusinessEndpointRepo
	// Contacts + Identities own the customer-side identity graph.
	Contacts   repository.ContactRepo
	Identities repository.IdentityRepo
	// Sessions + Conversations own the customer-service context.
	Sessions      repository.SessionRepo
	Conversations repository.ConversationRepo
	// Messages persists per-message metadata; payloads live in HBase and
	// are handled by a separate repo once that infra lands.
	Messages repository.MessageRepo
	// MessageStatusByPMI advances status updates keyed by provider_message_id.
	MessageStatusByPMI MessageStatusByProviderID
	// Bus fans out canonical events (MessageReceived / MessageDelivered /
	// MessageRead / MessageFailed) to real-time + automation subscribers.
	Bus eventbus.Publisher
	// LookupProvider resolves providerKey → channel.Provider.
	LookupProvider ChannelProviderLookup
	// NewID mints IDs for freshly-allocated Contacts and Messages.
	NewID IDGen
	// Now returns the current instant. Nil defaults to time.Now().UTC().
	Now func() time.Time
}
