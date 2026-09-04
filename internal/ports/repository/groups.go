package repository

import (
	"context"

	"github.com/v-senthil/nudgeway/internal/domain/group"
	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// GroupListFilter is an org-scoped list filter for Groups.
type GroupListFilter struct {
	// IntegrationID scopes the list to a single integration when set.
	IntegrationID integration.ID
	// Query is a case-insensitive substring match on Subject; empty means
	// no text filter.
	Query string
	// Cursor is an opaque paging token; empty for first page.
	Cursor string
	// Limit caps returned rows; implementations enforce a hard maximum.
	Limit int
}

// GroupMemberKey identifies a member row inside a group without pinning the
// auto-increment id. Repositories match on (GroupID, WaID, BSUID) — either
// key alone is sufficient when the other is empty, mirroring the unique
// index on group_members.
type GroupMemberKey struct {
	WaID  string
	BSUID string
}

// GroupRepo persists Group aggregates and their members. All methods are
// org-scoped — implementations MUST NOT read across tenants.
type GroupRepo interface {
	// Upsert inserts or updates a Group identified by
	// (OrgID, IntegrationID, ProviderGroupID). When the row does not exist
	// it is inserted with a freshly-minted ID and CreatedAt=now; when it
	// exists Subject / Description / Size / IsAdmin / Metadata are refreshed.
	// The returned Group carries the persisted ID + timestamps.
	Upsert(ctx context.Context, g group.Group) (group.Group, error)

	// Get fetches a Group by (OrgID, ID). Returns group.ErrNotFound when
	// missing.
	Get(ctx context.Context, orgID organization.ID, id group.ID) (group.Group, error)

	// GetByProviderID fetches a Group by its provider-native id under a
	// specific integration. Returns group.ErrNotFound when missing.
	GetByProviderID(ctx context.Context, orgID organization.ID, integrationID integration.ID, providerGroupID string) (group.Group, error)

	// List returns groups for the org, filtered + paginated. The second
	// return is the opaque next-page cursor, empty when the page is final.
	List(ctx context.Context, orgID organization.ID, filter GroupListFilter) ([]group.Group, string, error)

	// Delete removes a Group and cascades to its members. Idempotent —
	// deleting a missing row returns nil.
	Delete(ctx context.Context, orgID organization.ID, id group.ID) error

	// AddMember upserts a member row keyed by (GroupID, WaID, BSUID).
	// Existing rows have ContactID / Role / LeftAt refreshed.
	AddMember(ctx context.Context, m group.Member) error

	// RemoveMember stamps LeftAt on the member row identified by key.
	// Idempotent when the row is already gone.
	RemoveMember(ctx context.Context, orgID organization.ID, groupID group.ID, key GroupMemberKey) error

	// ListMembers returns the active + historical member roster for the
	// group. Rows with LeftAt != nil are included so operators can see the
	// full history; call sites filter as needed.
	ListMembers(ctx context.Context, orgID organization.ID, groupID group.ID) ([]group.Member, error)
}
