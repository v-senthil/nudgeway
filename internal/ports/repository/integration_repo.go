package repository

import (
	"context"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// IntegrationRepo persists Integration rows.
//
// Secret material (access tokens, app secrets, verify tokens) is NEVER
// carried on the returned Integration value — only the opaque
// CredentialsRef pointer. Concrete infrastructure implementations expose
// an auxiliary method (see mysql.Integrations.GetWithSecrets) that
// decrypts on demand; callers that need secrets ask for them explicitly.
type IntegrationRepo interface {
	// Get fetches an Integration by (OrgID, ID). Returns a not-found
	// sentinel from the infrastructure layer when missing.
	Get(ctx context.Context, orgID organization.ID, id integration.ID) (integration.Integration, error)

	// List returns every Integration owned by the org, ordered by
	// CreatedAt ascending.
	List(ctx context.Context, orgID organization.ID) ([]integration.Integration, error)

	// Create inserts a new Integration. CreatedAt/UpdatedAt are stamped
	// by the storage layer if left zero.
	Create(ctx context.Context, i integration.Integration) error

	// Update persists mutable fields (Status, Config, Capabilities,
	// Health, Name). The (OrgID, ID) tuple is the match key.
	Update(ctx context.Context, i integration.Integration) error

	// Delete removes an Integration row.
	Delete(ctx context.Context, orgID organization.ID, id integration.ID) error
}
