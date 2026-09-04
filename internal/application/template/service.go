package template

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	tmpldom "github.com/v-senthil/nudgeway/internal/domain/template"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ProviderTemplateSummary is the provider-neutral view of one row returned
// by a provider's list-templates call. Provider adapters map their
// native shape into this before the application service sees it.
type ProviderTemplateSummary struct {
	ID         string
	Name       string
	Language   string
	Status     string
	Category   string
	Components []tmpldom.Component
}

// ProviderCreateRequest is the provider-neutral submission payload the
// application service hands to the TemplateProvider port. The port
// implementation is responsible for translating this into the provider's
// wire shape.
type ProviderCreateRequest struct {
	Name                string
	Language            string
	Category            string
	Components          []tmpldom.Component
	AllowCategoryChange bool
}

// ProviderCreateResult is the provider-neutral response of the create call.
type ProviderCreateResult struct {
	ProviderTemplateID string
	Status             string
	Category           string
}

// TemplateProvider is the port the application layer depends on to talk to
// a concrete provider (WhatsApp today). It is intentionally narrow so a
// non-channel provider (e.g. a future SMS template store) can implement
// the same shape without pulling in every message-send concern.
//
// Implementations live in cmd/server (they close over the concrete
// adapter). The application package NEVER imports a provider package.
type TemplateProvider interface {
	// ListTemplates returns every template registered on the provider for
	// the integration the resolver bound this instance to.
	ListTemplates(ctx context.Context) ([]ProviderTemplateSummary, error)
	// CreateTemplate submits a new template for provider review.
	CreateTemplate(ctx context.Context, req ProviderCreateRequest) (ProviderCreateResult, error)
	// GetTemplateStatus fetches a single template's current review state.
	GetTemplateStatus(ctx context.Context, providerTemplateID string) (ProviderTemplateSummary, error)
}

// ProviderRegistry resolves a (providerKey, integrationID) pair to a live
// TemplateProvider bound to the integration's decrypted secrets. Kept as a
// separate port from message.ProviderRegistry because that one produces a
// channel.Provider — templates and channels are orthogonal capabilities.
type ProviderRegistry interface {
	// Template returns a TemplateProvider ready to serve the given
	// integration.
	Template(ctx context.Context, providerKey string, integ integration.Integration, secrets map[string]string) (TemplateProvider, error)
}

// IntegrationSecrets is the read side of the integration repository the
// template service needs — the row plus its decrypted secret material.
// Kept as a narrow interface (superset of repository.IntegrationRepo)
// because the create + sync paths need to bind the provider adapter to
// live credentials.
type IntegrationSecrets interface {
	repository.IntegrationRepo
	// GetWithSecrets returns the Integration alongside its decrypted
	// secrets map. Never log the returned map.
	GetWithSecrets(ctx context.Context, orgID organization.ID, id integration.ID) (integration.Integration, map[string]string, error)
}

// IDGenerator mints new template IDs. Kept as a port for deterministic tests.
type IDGenerator interface {
	// NewTemplateID returns a fresh globally-unique template id.
	NewTemplateID() string
}

// Clock returns the current time. Kept as a port for deterministic tests.
type Clock interface {
	// Now returns the current wall-clock time in UTC.
	Now() time.Time
}

// Deps bundles the dependencies of Service.
type Deps struct {
	// Repo persists Template rows.
	Repo repository.TemplateRepo
	// Integrations resolves the target integration + decrypts its secrets.
	Integrations IntegrationSecrets
	// Providers resolves the provider adapter bound to an integration.
	Providers ProviderRegistry
	// IDs mints new template ids.
	IDs IDGenerator
	// Clock returns the current time.
	Clock Clock
	// Logger receives structured records; nil defaults to slog.Default.
	Logger *slog.Logger
}

// Service is the use-case layer for template management.
type Service struct {
	deps Deps
}

// NewService constructs a Service. Panics on nil required deps.
func NewService(deps Deps) *Service {
	if deps.Repo == nil || deps.Integrations == nil || deps.Providers == nil ||
		deps.IDs == nil || deps.Clock == nil {
		panic("template.NewService: missing required dependency")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Service{deps: deps}
}

// CreateInput is the operator-supplied payload for POST /api/v1/templates.
//
// Callers optionally supply Submit=true to skip the local DRAFT stop and
// call the provider in the same request; the composer usually splits
// these so the operator can preview + tweak first.
type CreateInput struct {
	IntegrationID       string
	Name                string
	Language            string
	Category            tmpldom.Category
	Components          []tmpldom.Component
	Submit              bool
	AllowCategoryChange bool
}

// nameRe is the character class WhatsApp accepts for template names: lowercase
// alphanum + underscore. We enforce the same at our REST edge so the DRAFT
// row can be submitted later without a rename round-trip.
var nameRe = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)

// Create persists a new template in DRAFT state. When Submit=true the
// service immediately calls the provider and advances the row to PENDING
// (or the provider-returned status).
func (s *Service) Create(ctx context.Context, orgID organization.ID, in CreateInput) (tmpldom.Template, error) {
	if err := validateCreate(in); err != nil {
		return tmpldom.Template{}, err
	}

	// Resolve the target integration.
	integ, err := s.deps.Integrations.Get(ctx, orgID, integration.ID(in.IntegrationID))
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("%w: %w", tmpldom.ErrIntegrationMissing, err)
	}

	now := s.deps.Clock.Now().UTC()
	row := tmpldom.Template{
		ID:            tmpldom.ID(s.deps.IDs.NewTemplateID()),
		OrgID:         orgID,
		IntegrationID: integration.ID(in.IntegrationID),
		Name:          in.Name,
		Language:      in.Language,
		Category:      in.Category,
		Status:        tmpldom.StatusDraft,
		Components:    in.Components,
		Variables:     extractVariables(in.Components),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.deps.Repo.Create(ctx, row); err != nil {
		return tmpldom.Template{}, fmt.Errorf("persist template: %w", err)
	}

	if !in.Submit {
		return row, nil
	}

	// Same request submit — resolve the provider adapter now and hit the
	// provider. Persisted DRAFT stays if the submit call fails so the
	// operator can retry.
	submitted, err := s.callProviderCreate(ctx, orgID, integ, row, in.AllowCategoryChange)
	if err != nil {
		return row, err
	}
	return submitted, nil
}

// SubmitForReview transitions an existing DRAFT template to PENDING by
// calling the provider. Idempotent: a row already in PENDING or APPROVED
// is returned unchanged with ErrNotSubmittable so the REST edge can
// surface the state clearly.
func (s *Service) SubmitForReview(ctx context.Context, orgID organization.ID, id tmpldom.ID) (tmpldom.Template, error) {
	row, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		return tmpldom.Template{}, err
	}
	if row.Status != tmpldom.StatusDraft {
		return row, tmpldom.ErrNotSubmittable
	}
	integ, err := s.deps.Integrations.Get(ctx, orgID, row.IntegrationID)
	if err != nil {
		return tmpldom.Template{}, fmt.Errorf("%w: %w", tmpldom.ErrIntegrationMissing, err)
	}
	return s.callProviderCreate(ctx, orgID, integ, row, false)
}

// callProviderCreate resolves the provider adapter, calls CreateTemplate,
// and reconciles the local row's status + provider id.
func (s *Service) callProviderCreate(
	ctx context.Context,
	orgID organization.ID,
	integ integration.Integration,
	row tmpldom.Template,
	allowCategoryChange bool,
) (tmpldom.Template, error) {
	_, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integ.ID)
	if err != nil {
		return row, fmt.Errorf("load secrets: %w", err)
	}
	// Fold non-secret config the provider factory needs (WABA id, etc.)
	// into the secrets bag — mirrors the pattern message.SendService uses.
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)

	prov, err := s.deps.Providers.Template(ctx, integ.Provider, integ, secrets)
	if err != nil {
		return row, fmt.Errorf("resolve provider %q: %w", integ.Provider, err)
	}
	res, err := prov.CreateTemplate(ctx, ProviderCreateRequest{
		Name:                row.Name,
		Language:            row.Language,
		Category:            string(row.Category),
		Components:          row.Components,
		AllowCategoryChange: allowCategoryChange,
	})
	if err != nil {
		return row, fmt.Errorf("provider create: %w", err)
	}

	now := s.deps.Clock.Now().UTC()
	next := deriveStatus(res.Status)
	row.ProviderTemplateID = res.ProviderTemplateID
	row.Status = next
	if res.Category != "" {
		row.Category = tmpldom.Category(res.Category)
	}
	row.LastSyncedAt = &now
	row.UpdatedAt = now
	if err := s.deps.Repo.Upsert(ctx, row); err != nil {
		return row, fmt.Errorf("persist post-submit: %w", err)
	}
	return row, nil
}

// SyncResult reports the counts of a Sync run.
type SyncResult struct {
	Fetched  int
	Upserted int
}

// Sync reconciles the local template mirror against one integration's
// provider-side template list. Every row returned by the provider is
// upserted; local DRAFT rows the provider does not know about are left
// alone (they haven't been submitted yet).
func (s *Service) Sync(ctx context.Context, orgID organization.ID, integrationID integration.ID) (SyncResult, error) {
	integ, err := s.deps.Integrations.Get(ctx, orgID, integrationID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %w", tmpldom.ErrIntegrationMissing, err)
	}
	_, secrets, err := s.deps.Integrations.GetWithSecrets(ctx, orgID, integrationID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("load secrets: %w", err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	if v, ok := integ.Config["waba_id"].(string); ok && v != "" {
		secrets["waba_id"] = v
	}
	if v, ok := integ.Config["phone_number_id"].(string); ok && v != "" {
		secrets["phone_number_id"] = v
	}
	secrets["_integration_id"] = string(integ.ID)
	secrets["_org_id"] = string(integ.OrgID)

	prov, err := s.deps.Providers.Template(ctx, integ.Provider, integ, secrets)
	if err != nil {
		return SyncResult{}, fmt.Errorf("resolve provider %q: %w", integ.Provider, err)
	}
	items, err := prov.ListTemplates(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("provider list: %w", err)
	}
	now := s.deps.Clock.Now().UTC()
	upserted := 0
	for _, it := range items {
		row := tmpldom.Template{
			ID:                 tmpldom.ID(s.deps.IDs.NewTemplateID()),
			OrgID:              orgID,
			IntegrationID:      integrationID,
			ProviderTemplateID: it.ID,
			Name:               it.Name,
			Language:           it.Language,
			Category:           tmpldom.Category(it.Category),
			Status:             deriveStatus(it.Status),
			Components:         it.Components,
			Variables:          extractVariables(it.Components),
			LastSyncedAt:       &now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.deps.Repo.Upsert(ctx, row); err != nil {
			s.deps.Logger.Warn("template sync upsert failed",
				slog.String("org_id", string(orgID)),
				slog.String("integration_id", string(integrationID)),
				slog.String("name", it.Name),
				slog.String("language", it.Language),
				slog.Any("err", err),
			)
			continue
		}
		upserted++
	}
	// Second pass: refresh the status of every locally-persisted template
	// whose provider_template_id is set but which Meta didn't return in
	// its list. This covers the "submitted-but-Meta-hasn't-indexed-yet"
	// window that leaves fresh PENDING submissions stuck as DRAFT in the
	// list-based sync path. Rows without a provider_template_id are pure
	// local drafts and are skipped.
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.ID != "" {
			seen[it.ID] = struct{}{}
		}
	}
	page, listErr := s.deps.Repo.List(ctx, orgID, repository.TemplateListFilter{
		IntegrationID: &integrationID,
		Limit:         200,
	})
	polled := 0
	if listErr != nil {
		s.deps.Logger.Warn("template sync: local list failed; skipping status refresh",
			slog.String("org_id", string(orgID)),
			slog.String("integration_id", string(integrationID)),
			slog.Any("err", listErr),
		)
	} else {
		for _, t := range page.Templates {
			if t.ProviderTemplateID == "" {
				continue
			}
			if _, ok := seen[t.ProviderTemplateID]; ok {
				continue // Already refreshed by the list-based upsert above.
			}
			st, err := prov.GetTemplateStatus(ctx, t.ProviderTemplateID)
			if err != nil {
				s.deps.Logger.Warn("template sync: status poll failed",
					slog.String("org_id", string(orgID)),
					slog.String("integration_id", string(integrationID)),
					slog.String("template_id", string(t.ID)),
					slog.String("provider_template_id", t.ProviderTemplateID),
					slog.Any("err", err),
				)
				continue
			}
			newStatus := deriveStatus(st.Status)
			if newStatus == "" || newStatus == t.Status {
				continue
			}
			t.Status = newStatus
			t.LastSyncedAt = &now
			t.UpdatedAt = now
			if err := s.deps.Repo.Upsert(ctx, t); err != nil {
				s.deps.Logger.Warn("template sync: status refresh upsert failed",
					slog.String("template_id", string(t.ID)),
					slog.Any("err", err),
				)
				continue
			}
			polled++
		}
	}
	s.deps.Logger.Info("template sync complete",
		slog.String("org_id", string(orgID)),
		slog.String("integration_id", string(integrationID)),
		slog.Int("fetched", len(items)),
		slog.Int("upserted", upserted),
		slog.Int("status_refreshed", polled),
	)
	return SyncResult{Fetched: len(items), Upserted: upserted + polled}, nil
}

// Get returns a single template by (org, id).
func (s *Service) Get(ctx context.Context, orgID organization.ID, id tmpldom.ID) (tmpldom.Template, error) {
	return s.deps.Repo.Get(ctx, orgID, id)
}

// List returns a page of templates matching the filter.
func (s *Service) List(ctx context.Context, orgID organization.ID, filter repository.TemplateListFilter) (repository.TemplatePage, error) {
	return s.deps.Repo.List(ctx, orgID, filter)
}

// UpdateInput is the operator-supplied patch for PUT /api/v1/templates/{id}.
//
// Only DRAFT rows are editable. Name / language / integration_id are the
// natural key on the provider side and stay pinned to the original row —
// the caller must delete + recreate if any of those needs to change.
type UpdateInput struct {
	Category   tmpldom.Category
	Components []tmpldom.Component
}

// Update mutates an existing DRAFT template's category + components.
// Returns tmpldom.ErrNotEditable if the row is not in DRAFT — the REST
// edge surfaces that as 409 not_editable so the operator can clone instead.
func (s *Service) Update(ctx context.Context, orgID organization.ID, id tmpldom.ID, in UpdateInput) (tmpldom.Template, error) {
	row, err := s.deps.Repo.Get(ctx, orgID, id)
	if err != nil {
		return tmpldom.Template{}, err
	}
	if row.Status != tmpldom.StatusDraft {
		return row, tmpldom.ErrNotEditable
	}
	switch in.Category {
	case tmpldom.CategoryMarketing, tmpldom.CategoryUtility, tmpldom.CategoryAuthentication:
	default:
		return row, fmt.Errorf("%w: unknown category %q", tmpldom.ErrInvalid, in.Category)
	}
	hasBody := false
	for _, c := range in.Components {
		if c.Type == "BODY" || c.Type == "body" {
			hasBody = true
			break
		}
	}
	if !hasBody {
		return row, fmt.Errorf("%w: at least one BODY component is required", tmpldom.ErrInvalid)
	}
	row.Category = in.Category
	row.Components = in.Components
	row.Variables = extractVariables(in.Components)
	row.UpdatedAt = s.deps.Clock.Now().UTC()
	if err := s.deps.Repo.Upsert(ctx, row); err != nil {
		return row, fmt.Errorf("persist update: %w", err)
	}
	return row, nil
}

// Delete removes a template row. The provider mirror is NOT touched — a
// second endpoint (out of Phase 2 scope) will handle provider-side
// deletes. Local delete is what the composer wants when the operator is
// abandoning an unsubmitted DRAFT.
func (s *Service) Delete(ctx context.Context, orgID organization.ID, id tmpldom.ID) error {
	return s.deps.Repo.Delete(ctx, orgID, id)
}

// validateCreate enforces the shape invariants for Create.
func validateCreate(in CreateInput) error {
	if in.IntegrationID == "" {
		return fmt.Errorf("%w: integration_id required", tmpldom.ErrInvalid)
	}
	if in.Name == "" || !nameRe.MatchString(in.Name) {
		return fmt.Errorf("%w: name must be lowercase alphanumeric + underscore", tmpldom.ErrInvalid)
	}
	if in.Language == "" {
		return fmt.Errorf("%w: language required", tmpldom.ErrInvalid)
	}
	switch in.Category {
	case tmpldom.CategoryMarketing, tmpldom.CategoryUtility, tmpldom.CategoryAuthentication:
	default:
		return fmt.Errorf("%w: unknown category %q", tmpldom.ErrInvalid, in.Category)
	}
	hasBody := false
	for _, c := range in.Components {
		if c.Type == "BODY" || c.Type == "body" {
			hasBody = true
			break
		}
	}
	if !hasBody {
		return fmt.Errorf("%w: at least one BODY component is required", tmpldom.ErrInvalid)
	}
	return nil
}

// deriveStatus maps a provider-reported status string to the domain enum.
// Unknown values collapse to StatusPending so the sync path never loses a
// row — the operator sees "under review" and the reconciler fixes it on
// the next round-trip.
func deriveStatus(raw string) tmpldom.Status {
	switch raw {
	case "APPROVED", "approved":
		return tmpldom.StatusApproved
	case "REJECTED", "rejected":
		return tmpldom.StatusRejected
	case "PENDING", "pending", "IN_APPEAL", "PENDING_DELETION":
		return tmpldom.StatusPending
	case "PAUSED", "paused":
		return tmpldom.StatusPaused
	case "DISABLED", "disabled", "DELETED":
		return tmpldom.StatusDisabled
	case "DRAFT", "draft", "":
		return tmpldom.StatusDraft
	}
	return tmpldom.StatusPending
}

// varRe matches {{name}} placeholders in body text.
var varRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// extractVariables walks the components and returns a map of variable
// name → example value (from Component.Example when present, else "").
// Unnamed positional variables surface as "1", "2", ...
func extractVariables(components []tmpldom.Component) map[string]string {
	out := map[string]string{}
	for _, c := range components {
		if c.Text == "" {
			continue
		}
		matches := varRe.FindAllStringSubmatch(c.Text, -1)
		for _, m := range matches {
			if len(m) >= 2 {
				out[m[1]] = ""
			}
		}
	}
	// Best-effort: read named-parameter examples if present.
	for _, c := range components {
		if c.Example == nil {
			continue
		}
		if named, ok := c.Example["body_text_named_params"].([]any); ok {
			for _, p := range named {
				pm, _ := p.(map[string]any)
				name, _ := pm["param_name"].(string)
				val, _ := pm["example"].(string)
				if name != "" {
					out[name] = val
				}
			}
		}
	}
	return out
}

// Ensure the standard errors are importable through this package (some
// callers want to use errors.Is without a second import of the domain).
var (
	// ErrInvalid re-exports the domain sentinel.
	ErrInvalid = tmpldom.ErrInvalid
	// ErrNotFound re-exports the domain sentinel.
	ErrNotFound = tmpldom.ErrNotFound
	// ErrIntegrationMissing re-exports the domain sentinel.
	ErrIntegrationMissing = tmpldom.ErrIntegrationMissing
	// ErrNotSubmittable re-exports the domain sentinel.
	ErrNotSubmittable = tmpldom.ErrNotSubmittable
	// ErrNotEditable re-exports the domain sentinel.
	ErrNotEditable = tmpldom.ErrNotEditable
)

// Compile-time assertion that our re-exported sentinels stay wired.
var _ = errors.Is
