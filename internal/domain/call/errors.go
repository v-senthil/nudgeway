package call

import "errors"

// ErrNotFound is returned by CallRepo.Get / repo.UpdateStatus when the
// (org_id, id) or (org_id, provider, provider_call_id) tuple does not
// exist.
var ErrNotFound = errors.New("call: not found")

// ErrInvalid is returned when a mutation would produce a row that fails
// domain invariants (e.g. missing OrgID, missing Provider, empty
// ProviderCallID).
var ErrInvalid = errors.New("call: invalid")

// ErrProviderUnsupported is returned when the application layer is asked
// to invoke a capability the resolved provider does not advertise (e.g.
// Initiate on a receive-only provider).
var ErrProviderUnsupported = errors.New("call: provider does not support operation")
