package repository

import (
	"context"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/session"
)

// BusinessEndpoint is a channel-specific address owned by the tenant
// (e.g. a WhatsApp phone_number_id). Kept here in the ports layer as a
// simple record type so we avoid a dedicated domain package until the
// endpoint aggregate grows behaviour.
type BusinessEndpoint struct {
	ID            session.BusinessEndpointID
	OrgID         organization.ID
	Channel       string // "whatsapp", "sms", "email", ...
	Provider      string // "whatsapp", "twilio", ...
	IntegrationID string
	ExternalID    string // provider-native id (e.g. phone_number_id)
	Display       string // human-friendly label
	Metadata      map[string]any
	CreatedAt     time.Time
}

// BusinessEndpointRepo persists BusinessEndpoint rows.
type BusinessEndpointRepo interface {
	// Get fetches a business endpoint by (OrgID, ID).
	Get(ctx context.Context, orgID organization.ID, id session.BusinessEndpointID) (BusinessEndpoint, error)

	// FindByExternalID resolves the (org, provider, external_id) tuple —
	// the primary lookup used by webhook parsers to route inbound events.
	FindByExternalID(ctx context.Context, orgID organization.ID, provider, externalID string) (BusinessEndpoint, error)

	// List returns all endpoints for an org.
	List(ctx context.Context, orgID organization.ID) ([]BusinessEndpoint, error)
}
