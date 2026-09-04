package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// SaveSecrets envelope-encrypts and persists the given secret map for an
// integration. Idempotent — on repeat calls the ciphertext is rotated
// via ON DUPLICATE KEY UPDATE. Requires the repository to have been
// constructed with a crypto.Envelope.
//
// Called by the Integrations application service on Create() so the
// REST path and the CLI path both persist secrets the same way.
func (r *Integrations) SaveSecrets(
	ctx context.Context,
	orgID organization.ID,
	id integration.ID,
	secrets map[string]string,
) error {
	if r.env == nil {
		return errors.New("integrations: envelope not configured — cannot persist secrets")
	}
	if len(secrets) == 0 {
		return nil
	}
	orgBytes, err := ulidToBytes(string(orgID))
	if err != nil {
		return fmt.Errorf("integrations save secrets bad org: %w", err)
	}
	idBytes, err := ulidToBytes(string(id))
	if err != nil {
		return fmt.Errorf("integrations save secrets bad id: %w", err)
	}
	pt, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("integrations save secrets marshal: %w", err)
	}
	ct, err := r.env.Encrypt(pt)
	if err != nil {
		return fmt.Errorf("integrations save secrets encrypt: %w", err)
	}
	credID := ulid.Make()
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO integration_credentials (id, org_id, integration_id, ciphertext, kek_ref)
		 VALUES (?, ?, ?, ?, 'auth.credential_kek_hex')
		 ON DUPLICATE KEY UPDATE ciphertext = VALUES(ciphertext)`,
		credID[:], orgBytes, idBytes, ct,
	); err != nil {
		return fmt.Errorf("integrations save secrets upsert: %w", err)
	}
	return nil
}
