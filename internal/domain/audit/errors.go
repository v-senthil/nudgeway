package audit

import "errors"

// ErrInvalidEntry is returned by Record when the entry is missing
// mandatory fields (OrgID or Action). Callers should never surface this
// as a user-facing error — audit writes are internal.
var ErrInvalidEntry = errors.New("audit: invalid entry")

// ErrInvalidCursor is returned by List when the caller-supplied cursor
// cannot be decoded. Surfaces as 400 Bad Request at the REST edge.
var ErrInvalidCursor = errors.New("audit: invalid cursor")
