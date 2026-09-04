package template

import "errors"

// ErrNotFound is returned by ports + services when a Template lookup misses.
var ErrNotFound = errors.New("template: not found")

// ErrInvalid is returned when a Template fails validation (missing name,
// unknown category, no body component, etc.). Callers wrap with more
// context for the REST edge to translate into a 400.
var ErrInvalid = errors.New("template: invalid")

// ErrIntegrationMissing is returned when the target integration for a
// create/sync call cannot be resolved for the caller's org.
var ErrIntegrationMissing = errors.New("template: integration missing")

// ErrNotSubmittable is returned when SubmitForReview is called on a
// Template that is not in DRAFT state — a second submission of a PENDING
// or APPROVED row is a no-op the caller should recognise as such.
var ErrNotSubmittable = errors.New("template: not submittable")

// ErrNotEditable is returned when Update is called on a Template that is
// not in DRAFT state. Only local drafts are editable — anything the
// provider has already seen must be edited by cloning into a new draft.
var ErrNotEditable = errors.New("template: not editable")
