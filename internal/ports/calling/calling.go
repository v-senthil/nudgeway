// Package calling is the port for voice/IVR providers.
package calling

import "context"

// Call is the canonical call shape.
type Call struct {
	ID        string
	OrgID     string
	ContactID string
	Direction string
	Status    string
}

// Provider is implemented by every calling adapter.
type Provider interface {
	InitiateCall(ctx context.Context, c Call) (externalID string, err error)
	EndCall(ctx context.Context, externalID string) error
}
