package analytics

import "errors"

// ErrInvalidRange is returned by the read-side application service when
// the caller supplies a from > to range or an unbounded range beyond
// the supported window.
var ErrInvalidRange = errors.New("analytics: invalid range")

// ErrUnknownSeries is returned by Service.Series when the caller
// requests a SeriesKind the service does not recognise. Surfaces as a
// 400 Bad Request at the REST edge.
var ErrUnknownSeries = errors.New("analytics: unknown series kind")

// ErrInvalidRollupDay is returned by Service.Rollup when the caller
// supplies a zero day. Rollup is always scoped to a specific UTC day.
var ErrInvalidRollupDay = errors.New("analytics: invalid rollup day")
