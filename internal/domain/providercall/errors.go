package providercall

import "errors"

// ErrNotFound is returned by repositories when a lookup for a single entry
// misses. The list surface returns an empty page rather than this error.
var ErrNotFound = errors.New("providercall: entry not found")
