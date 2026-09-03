package integration

import (
	"context"
	"errors"
	"fmt"
	"time"

	dintegration "github.com/fullwa/fullwa/internal/domain/integration"
	"github.com/fullwa/fullwa/internal/domain/organization"
	"github.com/fullwa/fullwa/internal/domain/session"
	"github.com/fullwa/fullwa/internal/ports/channel"
	"github.com/fullwa/fullwa/internal/ports/repository"
	"github.com/fullwa/fullwa/internal/providers"
)

// ErrNotFound is returned when the requested integration does not exist
// (or belongs to a different organization). Callers translate this to a
// 404 at the REST edge.
var ErrNotFound = errors.New("integration not found")

// ErrValidation wraps a user-actionable validation problem — missing
// required config or secret keys, unknown provider, etc. Translated to
// 422 at the REST edge.
type ErrValidation struct{ Msg string }

// Error implements error.
func (e *ErrValidation) Error() string { return e.Msg }

// newValidation constructs an ErrValidation with fmt.Sprintf semantics.
func newValidation(format string, a ...any) error {
	return &ErrValidation{Msg: fmt.Sprintf(format, a...)}
}

// IntegrationSecretsRepo is the auxiliary read path that decrypts credential
// material on demand. It is intentionally a separate interface from the
// port's IntegrationRepo so callers that never need secrets don't reach
// for them by accident.
type IntegrationSecretsRepo interface {
	// GetWithSecrets returns the Integration together with a decrypted
	// secrets map. Never log the returned map.
	GetWithSecrets(ctx context.Context, orgID organization.ID, id dintegration.ID) (dintegration.Integration, map[string]string, error)

	// SaveSecrets envelope-encrypts and persists the secret map for an
	// integration. Called by Create right after the integration row is
	// inserted so the REST and CLI paths persist secrets the same way.
	SaveSecrets(ctx context.Context, orgID organization.ID, id dintegration.ID, secrets map[string]string) error
}

// BusinessEndpointUpserter is the write path used by Create to ensure a
// business endpoint row exists for the provider-native id
// (e.g. phone_number_id). Implemented by
// mysql.BusinessEndpoints.Upsert (Agent A).
type BusinessEndpointUpserter interface {
	// Upsert finds-or-creates the (org, provider, external_id) tuple,
	// linking it to the given integration.
	Upsert(ctx context.Context, ep repository.BusinessEndpoint) (session.BusinessEndpointID, error)
}

// ProviderResolver returns the channel.Provider adapter for a given
// integration, wired against decrypted credentials. Implemented in
// cmd/server (the only place that may import provider packages).
type ProviderResolver interface {
	// Resolve returns a live channel.Provider for the given Integration.
	Resolve(ctx context.Context, i dintegration.Integration, secrets map[string]string) (channel.Provider, error)
}

// Clock abstracts time.Now so tests can inject a deterministic value.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// IDGenerator produces new integration IDs. Injected so tests can pin them.
type IDGenerator interface {
	// NewID returns a fresh ULID (or provider-appropriate) integration ID.
	NewID() dintegration.ID
}

// Service is the use-case layer for tenant provider integrations.
type Service struct {
	repo      repository.IntegrationRepo
	secrets   IntegrationSecretsRepo
	endpoints BusinessEndpointUpserter
	resolver  ProviderResolver
	clock     Clock
	ids       IDGenerator
}

// Deps bundles the constructor arguments of Service.
type Deps struct {
	// Repo persists Integration rows (required).
	Repo repository.IntegrationRepo
	// Secrets exposes decrypted credential material (required for Test).
	Secrets IntegrationSecretsRepo
	// Endpoints upserts BusinessEndpoint rows (required for Create).
	Endpoints BusinessEndpointUpserter
	// Resolver returns a live provider adapter (required for Test).
	Resolver ProviderResolver
	// Clock overrides time.Now; defaults to systemClock{}.
	Clock Clock
	// IDs overrides the ID generator; defaults are supplied by callers.
	IDs IDGenerator
}

// NewService constructs a Service. Panics if the required deps are nil.
func NewService(d Deps) *Service {
	if d.Repo == nil {
		panic("integration.NewService: Repo required")
	}
	s := &Service{
		repo:      d.Repo,
		secrets:   d.Secrets,
		endpoints: d.Endpoints,
		resolver:  d.Resolver,
		clock:     d.Clock,
		ids:       d.IDs,
	}
	if s.clock == nil {
		s.clock = systemClock{}
	}
	return s
}

// systemClock is the default Clock backed by time.Now.
type systemClock struct{}

// Now returns time.Now.
func (systemClock) Now() time.Time { return time.Now().UTC() }

// providerSchema declares the required Config and Secret keys per
// registered provider. Add new providers here as they land; unknown
// providers are rejected at Create time.
type providerSchema struct {
	// Type is the provider kind expected for this key.
	Type dintegration.Type
	// Channel is the channel token stored on business_endpoints
	// (e.g. "whatsapp"). Empty for non-channel providers.
	Channel string
	// RequiredConfig lists Config keys that MUST be present.
	RequiredConfig []string
	// RequiredSecrets lists Secret keys that MUST be present.
	RequiredSecrets []string
	// ExternalIDKey is the Config key that becomes
	// business_endpoints.external_id (empty for non-channel providers).
	ExternalIDKey string
}

// schemas is the small per-provider validation table. Adding a provider
// here is the only step required to unlock CRUD for it.
var schemas = map[string]providerSchema{
	"whatsapp": {
		Type:            dintegration.TypeChannel,
		Channel:         "whatsapp",
		RequiredConfig:  []string{"phone_number_id", "waba_id"},
		RequiredSecrets: []string{"access_token", "app_secret", "verify_token"},
		ExternalIDKey:   "phone_number_id",
	},
}

// List returns every integration owned by orgID. Secrets are never
// carried on the returned values.
func (s *Service) List(ctx context.Context, orgID organization.ID) ([]PublicIntegration, error) {
	rows, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("integration list: %w", err)
	}
	out := make([]PublicIntegration, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPublic(r))
	}
	return out, nil
}

// Get fetches a single integration by (orgID, id). Secrets are stripped.
func (s *Service) Get(ctx context.Context, orgID organization.ID, id dintegration.ID) (PublicIntegration, error) {
	row, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return PublicIntegration{}, fmt.Errorf("integration get: %w", err)
	}
	return toPublic(row), nil
}

// Create validates input, persists a new Integration row (with encrypted
// secrets), and upserts a BusinessEndpoint row for the provider-native
// id. Returns the client-facing view.
func (s *Service) Create(ctx context.Context, orgID organization.ID, in CreateInput) (PublicIntegration, map[string]string, error) {
	if in.Name == "" {
		return PublicIntegration{}, nil, newValidation("name required")
	}
	if in.Provider == "" {
		return PublicIntegration{}, nil, newValidation("provider required")
	}
	schema, ok := schemas[in.Provider]
	if !ok {
		return PublicIntegration{}, nil, newValidation("unknown provider %q", in.Provider)
	}
	kind := providers.Kind(schema.Type)
	if _, ok := providers.Lookup(kind, in.Provider); !ok {
		return PublicIntegration{}, nil, newValidation("provider %q not registered", in.Provider)
	}
	if in.Type != "" && in.Type != schema.Type {
		return PublicIntegration{}, nil, newValidation("type %q does not match provider %q (want %q)", in.Type, in.Provider, schema.Type)
	}
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	for _, k := range schema.RequiredConfig {
		v, ok := in.Config[k]
		if !ok || v == nil || v == "" {
			return PublicIntegration{}, nil, newValidation("config.%s required", k)
		}
	}
	if in.Secrets == nil {
		return PublicIntegration{}, nil, newValidation("secrets required")
	}
	for _, k := range schema.RequiredSecrets {
		if v, ok := in.Secrets[k]; !ok || v == "" {
			return PublicIntegration{}, nil, newValidation("secrets.%s required", k)
		}
	}

	// Guard against secrets accidentally provided in Config.
	for k := range sensitiveConfigKeys {
		if _, present := in.Config[k]; present {
			return PublicIntegration{}, nil, newValidation("config must not carry secret %q; use secrets", k)
		}
	}

	now := s.clock.Now()
	var id dintegration.ID
	if s.ids != nil {
		id = s.ids.NewID()
	}
	row := dintegration.Integration{
		ID:           id,
		OrgID:        orgID,
		Type:         schema.Type,
		Provider:     in.Provider,
		Name:         in.Name,
		Status:       dintegration.StatusDisconnected, // Test() flips it to Connected.
		Config:       in.Config,
		Capabilities: map[string]bool{},
		Health:       map[string]any{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return PublicIntegration{}, nil, fmt.Errorf("integration create: %w", err)
	}

	// Persist secrets right after the integration row lands so the FK
	// (integration_credentials.integration_id → integrations.id) is
	// satisfied. Without this, the REST-created row would have no
	// credentials and the webhook verify handshake would 403 (Meta's
	// "Callback verification failed").
	if s.secrets != nil && len(in.Secrets) > 0 {
		if err := s.secrets.SaveSecrets(ctx, orgID, row.ID, in.Secrets); err != nil {
			return PublicIntegration{}, nil, fmt.Errorf("integration save secrets: %w", err)
		}
	}

	// Upsert the business endpoint for channel-kind providers.
	if schema.ExternalIDKey != "" && s.endpoints != nil {
		externalID, _ := in.Config[schema.ExternalIDKey].(string)
		if externalID != "" {
			ep := repository.BusinessEndpoint{
				OrgID:         orgID,
				Channel:       schema.Channel,
				Provider:      in.Provider,
				IntegrationID: string(row.ID),
				ExternalID:    externalID,
				Display:       in.Name,
				Metadata:      map[string]any{},
				CreatedAt:     now,
			}
			if _, err := s.endpoints.Upsert(ctx, ep); err != nil {
				return PublicIntegration{}, nil, fmt.Errorf("integration endpoint upsert: %w", err)
			}
		}
	}
	// Never return secrets to callers; the second return value is nil
	// on purpose (reserved for a future path that streams a one-shot
	// bootstrap token). Callers must strip.
	return toPublic(row), nil, nil
}

// Test resolves the adapter for id, invokes HealthCheck, updates the
// integration's Status + Health, and returns the outcome. HealthCheck
// dispatch is done via the ProviderResolver so this package never imports
// concrete adapter code.
func (s *Service) Test(ctx context.Context, orgID organization.ID, id dintegration.ID) (TestResult, error) {
	if s.secrets == nil || s.resolver == nil {
		return TestResult{}, fmt.Errorf("integration test: secrets or resolver not wired")
	}
	row, secrets, err := s.secrets.GetWithSecrets(ctx, orgID, id)
	if err != nil {
		return TestResult{}, fmt.Errorf("integration test: %w", err)
	}
	adapter, err := s.resolver.Resolve(ctx, row, secrets)
	if err != nil {
		return TestResult{}, fmt.Errorf("integration test resolve: %w", err)
	}
	h := adapter.HealthCheck(ctx)
	now := s.clock.Now()
	row.Health = map[string]any{
		"ok":         h.OK,
		"message":    h.Message,
		"checked_at": now.Format(time.RFC3339),
	}
	if h.OK {
		row.Status = dintegration.StatusConnected
	} else {
		row.Status = dintegration.StatusDegraded
	}
	row.UpdatedAt = now
	if err := s.repo.Update(ctx, row); err != nil {
		return TestResult{}, fmt.Errorf("integration test update: %w", err)
	}
	return TestResult{OK: h.OK, Message: h.Message, CheckedAt: now}, nil
}

// Delete soft-disconnects the integration. The row and its
// encrypted credentials remain in the audit trail for Phase 4; the
// Status field flips to disconnected so no downstream worker picks it up.
func (s *Service) Delete(ctx context.Context, orgID organization.ID, id dintegration.ID) error {
	row, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return fmt.Errorf("integration delete get: %w", err)
	}
	row.Status = dintegration.StatusDisconnected
	row.Health = map[string]any{"message": "disconnected by operator", "checked_at": s.clock.Now().Format(time.RFC3339)}
	row.UpdatedAt = s.clock.Now()
	if err := s.repo.Update(ctx, row); err != nil {
		return fmt.Errorf("integration delete update: %w", err)
	}
	return nil
}

// WebhookPath returns the canonical webhook path for a provider-kind
// integration ("/webhooks/whatsapp/<id>"). The REST handler prepends
// its configured public base URL.
func WebhookPath(providerKey string, id dintegration.ID) string {
	return "/webhooks/" + providerKey + "/" + string(id)
}

// Ensure the local helper is used to satisfy the linter when callers of
// this package want a typed OrgID without importing the domain package.
var _ = orgIDFrom
