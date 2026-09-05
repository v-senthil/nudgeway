// Package ticketing is the port for ticketing providers (
// Freshdesk, Zendesk, Salesforce, ServiceNow, …).
package ticketing

import "context"

// Ticket is the canonical ticket shape the port exchanges with adapters.
type Ticket struct {
	ID          string
	OrgID       string
	Title       string
	Description string
	Status      string
	Priority    string
	ContactID   string
}

// Provider is implemented by every ticketing adapter.
type Provider interface {
	CreateTicket(ctx context.Context, t Ticket) (externalID string, err error)
	UpdateTicket(ctx context.Context, externalID string, patch Ticket) error
	AddMessage(ctx context.Context, externalID, body string) error
	CloseTicket(ctx context.Context, externalID string) error
	ReopenTicket(ctx context.Context, externalID string) error
}
