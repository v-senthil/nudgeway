package organization

import "time"

// ID is a tenant identifier. Stored as VARBINARY(16) in MySQL; exchanged as
// ULID or UUID string across API + logs.
type ID string

// Status enumerates organization lifecycle states.
type Status string

// Status values.
const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

// Organization is the tenant boundary of the platform.
type Organization struct {
	ID        ID
	Slug      string
	Name      string
	Status    Status
	Settings  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Active reports whether the tenant is currently allowed to operate.
func (o Organization) Active() bool { return o.Status == StatusActive }
