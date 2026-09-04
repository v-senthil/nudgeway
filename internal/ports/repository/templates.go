package repository

import (
	"context"
	"time"

	"github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/template"
)

// TemplateListFilter narrows a Template listing.
//
// IntegrationID is a pointer so the zero value is "any integration". Status
// is an empty string to disable the filter. Cursor is opaque base64 minted
// by the infra layer.
type TemplateListFilter struct {
	IntegrationID *integration.ID
	Status        template.Status
	Cursor        string
	Limit         int
}

// TemplatePage is one page of templates newest-updated-first.
type TemplatePage struct {
	Templates  []template.Template
	NextCursor string
}

// TemplateRepo persists Template rows.
//
// Upsert is the sync path: the infra implementation keys on
// (org_id, integration_id, name, language) and treats a row already
// carrying a provider_template_id as an update rather than an insert.
type TemplateRepo interface {
	// Create inserts a fresh Template row. Callers pre-populate the ID.
	// Implementations return a duplicate error when the (org, integration,
	// name, language) unique index rejects the insert; the application
	// service maps that into template.ErrInvalid.
	Create(ctx context.Context, t template.Template) error

	// Get returns a single Template by (org, id). Missing rows surface as
	// template.ErrNotFound.
	Get(ctx context.Context, orgID organization.ID, id template.ID) (template.Template, error)

	// FindByNameLanguage returns the Template row matching the natural key
	// (org, integration, name, language). Used by the send path to look up
	// the definition so outbound message metadata can be enriched with the
	// resolved header/body/footer/button text. Returns template.ErrNotFound
	// when no row matches.
	FindByNameLanguage(
		ctx context.Context,
		orgID organization.ID,
		integrationID integration.ID,
		name string,
		language string,
	) (template.Template, error)

	// List returns templates for one org filtered by the supplied filter.
	// Ordered by updated_at DESC — the sync path stamps updated_at on
	// every reconcile so the freshest data floats to the top.
	List(ctx context.Context, orgID organization.ID, filter TemplateListFilter) (TemplatePage, error)

	// Upsert inserts or updates a Template by its (org, integration, name,
	// language) natural key. Used by the sync path to reconcile the local
	// mirror against the provider.
	Upsert(ctx context.Context, t template.Template) error

	// UpdateStatus advances a Template's Status (and stamps last_synced_at)
	// without touching the components / variables. Idempotent: setting the
	// same status twice is a no-op.
	UpdateStatus(ctx context.Context, orgID organization.ID, id template.ID, status template.Status, syncedAt time.Time) error

	// Delete removes a Template row by (org, id).
	Delete(ctx context.Context, orgID organization.ID, id template.ID) error
}
