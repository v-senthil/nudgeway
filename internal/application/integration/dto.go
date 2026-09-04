// Package integration contains use-cases for tenant provider integrations
// (WhatsApp, Zoho Desk, ...). It never imports concrete provider packages;
// dispatching to a specific adapter goes through the `providers` registry
// and the `channel.Provider` port.
package integration

import (
	"time"

	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// CreateInput is the operator-supplied payload for POST
// /api/v1/integrations. Config carries non-secret provider settings
// (e.g. `phone_number_id`, `waba_id`); Secrets carries envelope-encrypted
// material (e.g. `access_token`, `app_secret`, `verify_token`).
//
// Callers MUST populate Secrets even though the response strips them:
// the service passes them to the underlying repository which encrypts
// them at rest. Secrets never leave the process boundary as plaintext.
type CreateInput struct {
	// Type is the provider kind ("channel", "ticketing", ...). Must match
	// the registered provider's kind.
	Type dintegration.Type
	// Provider is the registry key (e.g. "whatsapp").
	Provider string
	// Name is the tenant-supplied label.
	Name string
	// Config carries non-secret provider configuration values.
	Config map[string]any
	// Secrets carries secret material. Never returned to clients.
	Secrets map[string]string
}

// TestResult is the response body of POST /api/v1/integrations/{id}/test.
type TestResult struct {
	// OK is true when the provider adapter reports a healthy state.
	OK bool `json:"ok"`
	// Message is a human-readable diagnostic (never contains credentials).
	Message string `json:"message"`
	// CheckedAt is the time the health check completed.
	CheckedAt time.Time `json:"checked_at"`
}

// PublicIntegration is the client-facing view of an Integration row.
// It omits every field carrying secret material (CredentialsRef is
// dropped; Config is passed through minus any keys the service marks
// sensitive) and adds the tenant-facing webhook URL to paste into the
// provider console.
type PublicIntegration struct {
	// ID is the ULID of the integration row.
	ID string `json:"id"`
	// OrgID is the owning organization's ULID.
	OrgID string `json:"org_id"`
	// Type is the provider kind ("channel", ...).
	Type string `json:"type"`
	// Provider is the registry key ("whatsapp", ...).
	Provider string `json:"provider"`
	// Name is the tenant-supplied label.
	Name string `json:"name"`
	// Status is the current lifecycle/health state.
	Status string `json:"status"`
	// Config carries non-secret configuration values only.
	Config map[string]any `json:"config"`
	// Capabilities is the adapter-declared capability map.
	Capabilities map[string]bool `json:"capabilities"`
	// Health is the last observed health snapshot (adapter-defined).
	Health map[string]any `json:"health,omitempty"`
	// WebhookURL is the fully-qualified URL the operator pastes into the
	// provider console. Derived at response time from PublicBaseURL and
	// the provider key.
	WebhookURL string `json:"webhook_url,omitempty"`
	// CreatedAt is the row insertion time.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last mutation time.
	UpdatedAt time.Time `json:"updated_at"`
}

// toPublic strips secret material from a domain Integration and returns
// the client-facing view. The webhook URL is left blank here; the caller
// (REST handler) fills it in based on request context.
func toPublic(i dintegration.Integration) PublicIntegration {
	cfg := map[string]any{}
	for k, v := range i.Config {
		// The domain Integration invariant already forbids secrets in Config,
		// but we defensively drop known-sensitive keys as a belt-and-braces.
		if _, sensitive := sensitiveConfigKeys[k]; sensitive {
			continue
		}
		cfg[k] = v
	}
	return PublicIntegration{
		ID:           string(i.ID),
		OrgID:        string(i.OrgID),
		Type:         string(i.Type),
		Provider:     i.Provider,
		Name:         i.Name,
		Status:       string(i.Status),
		Config:       cfg,
		Capabilities: i.Capabilities,
		Health:       i.Health,
		CreatedAt:    i.CreatedAt,
		UpdatedAt:    i.UpdatedAt,
	}
}

// sensitiveConfigKeys is the belt-and-braces set of keys that MUST NOT
// appear on the client-facing view even if they somehow leaked into Config.
var sensitiveConfigKeys = map[string]struct{}{
	"access_token":  {},
	"app_secret":    {},
	"verify_token":  {},
	"refresh_token": {},
	"api_key":       {},
	"client_secret": {},
}

// orgIDFrom is a helper reused by callers that receive OrgID as a plain
// string (e.g. the REST middleware's Principal). It exists so this
// package doesn't need to expose the domain type at every seam.
func orgIDFrom(s string) organization.ID { return organization.ID(s) }
