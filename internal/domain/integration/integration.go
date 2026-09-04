// Package integration models the persisted Integration entity — the
// tenant-scoped configuration + credentials + declared capabilities for a
// concrete provider instance.
package integration

import (
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID identifies an Integration row.
type ID string

// Type is the coarse category the Integration belongs to. Matches the
// provider-kind vocabulary used by the provider registry.
type Type string

// Type values.
const (
	// TypeChannel is a customer-communication channel (WhatsApp, SMS, ...).
	TypeChannel Type = "channel"
	// TypeTicketing is a support/CRM ticketing backend (Zoho Desk, ...).
	TypeTicketing Type = "ticketing"
	// TypeBot is a conversational bot backend.
	TypeBot Type = "bot"
	// TypeAI is an LLM / AI backend (OpenAI, Anthropic, ...).
	TypeAI Type = "ai"
	// TypeCalling is a voice-calling backend.
	TypeCalling Type = "calling"
)

// Status enumerates Integration lifecycle / health states as understood by
// the application layer. The MySQL schema stores a slightly narrower ENUM
// ('pending','active','error','disabled') — infrastructure code maps
// between the two so the domain vocabulary stays expressive.
type Status string

// Status values.
const (
	// StatusConnected is the healthy steady state.
	StatusConnected Status = "connected"
	// StatusDisconnected means the tenant has intentionally disabled the row.
	StatusDisconnected Status = "disconnected"
	// StatusDegraded means partial functionality (e.g. some capabilities failing).
	StatusDegraded Status = "degraded"
	// StatusAuthFailed means the last provider call returned a 401/403.
	StatusAuthFailed Status = "auth_failed"
	// StatusRateLimited means the provider is currently throttling us.
	StatusRateLimited Status = "rate_limited"
)

// Integration is the tenant-scoped configuration for a concrete provider
// instance (e.g. "the production WhatsApp phone number").
//
// Non-secret configuration (phone_number_id, WABA id, business_id) lives in
// Config as JSON. Secret material (access_token, app_secret, verify_token,
// refresh_token) lives envelope-encrypted in a companion
// integration_credentials row referenced by CredentialsRef — never in
// Config, never in logs.
type Integration struct {
	ID             ID
	OrgID          organization.ID
	Type           Type
	Provider       string // registry key, e.g. "whatsapp", "zoho_desk"
	Name           string
	Status         Status
	Config         map[string]any
	CredentialsRef []byte // opaque ref (integration_credentials.id bytes) or nil
	Capabilities   map[string]bool
	Health         map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
