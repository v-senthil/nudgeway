package audit

import (
	"net"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/domain/user"
)

// Action is a stable, human-readable verb identifying the mutation that
// was recorded. Action values are stored verbatim in the audit_logs table
// and referenced by admins searching the trail — treat them as an API.
type Action string

// Well-known audit actions emitted by Phase 1 + Phase 2 mutation paths.
// Additions land alongside the code that emits them; renames require a
// migration since old rows carry the old string.
const (
	// IntegrationCreated is emitted when a provider integration is
	// created by POST /api/v1/integrations.
	IntegrationCreated Action = "integration.created"
	// IntegrationDeleted is emitted when an integration is soft-disconnected
	// by DELETE /api/v1/integrations/{id}.
	IntegrationDeleted Action = "integration.deleted"
	// IntegrationTested is emitted when an operator runs the health check
	// via POST /api/v1/integrations/{id}/test.
	IntegrationTested Action = "integration.tested"
	// MessageSent is emitted when POST /api/v1/messages accepts an outbound
	// send. Recorded before the async worker actually contacts the provider.
	MessageSent Action = "message.sent"
	// MessageMarkedRead is emitted when POST /api/v1/messages/{id}/read
	// succeeds against the channel provider.
	MessageMarkedRead Action = "message.marked_read"
	// ConversationMarkedRead is emitted when the batch variant
	// POST /api/v1/conversations/{id}/read completes.
	ConversationMarkedRead Action = "conversation.marked_read"
	// AttachmentUploaded is emitted after POST /api/v1/attachments finishes
	// persisting and (when applicable) uploading to the provider.
	AttachmentUploaded Action = "attachment.uploaded"
	// BusinessProfileUpdated is emitted after PUT
	// /api/v1/integrations/{id}/business-profile commits the change to
	// the provider (Meta).
	BusinessProfileUpdated Action = "integration.business_profile.updated"
	// CallSettingsUpdated is emitted after PUT
	// /api/v1/integrations/{id}/call-settings.
	CallSettingsUpdated Action = "integration.call_settings.updated"
	// OBAApplied is emitted after POST
	// /api/v1/integrations/{id}/oba-status/apply.
	OBAApplied Action = "integration.oba.applied"
	// OBAWithdrawn is emitted after POST
	// /api/v1/integrations/{id}/oba-status/withdraw.
	OBAWithdrawn Action = "integration.oba.withdrawn"
	// UsernameUpdated is emitted after PUT
	// /api/v1/integrations/{id}/username adopts or changes the
	// business-scoped username on the provider (Meta).
	UsernameUpdated Action = "integration.username.updated"
	// UsernameDeleted is emitted after DELETE
	// /api/v1/integrations/{id}/username releases the username.
	UsernameDeleted Action = "integration.username.deleted"
	// WebhookConfigured is emitted after POST
	// /api/v1/integrations/{id}/webhook pushes a webhook callback
	// override to the provider (Meta).
	WebhookConfigured Action = "integration.webhook.configured"
	// CallPermissionRequested is emitted after POST
	// /api/v1/calls/permission-request sends an interactive
	// call_permission_request message to prompt the user to grant call
	// permission.
	CallPermissionRequested Action = "call.permission_request.sent"
	// UserLoggedIn is emitted when POST /api/v1/auth/login issues a session.
	UserLoggedIn Action = "user.logged_in"
	// UserLoggedOut is emitted when POST /api/v1/auth/logout invalidates one.
	UserLoggedOut Action = "user.logged_out"
)

// Entry is a single audit trail row. Every field except ActorUserID and IP
// is required; the zero value of ActorUserID (*user.ID nil) means the
// action was performed by the system (e.g. a scheduler job), and a nil IP
// means the action was performed off-request (e.g. a background worker
// with no HTTP context).
type Entry struct {
	// ID is the auto-increment primary key. Zero before Record returns.
	ID uint64
	// OrgID is the tenant that owns the audited resource.
	OrgID organization.ID
	// ActorUserID is the operator who performed the action, or nil for
	// system-driven mutations.
	ActorUserID *user.ID
	// Action is the verb — one of the exported constants above, or a new
	// value declared alongside the emitting code.
	Action Action
	// ResourceType is the domain entity kind ("integration", "message",
	// "conversation", "attachment", "session").
	ResourceType string
	// ResourceID is the affected entity's identifier as a string. Empty
	// when the action is not scoped to a single row (e.g. bulk import).
	ResourceID string
	// IP is the client IP address recorded from the request. Nil for
	// off-request writes.
	IP net.IP
	// Metadata holds arbitrary JSON-serialisable context (e.g. previous
	// value on an update, message type on a send). Persisted verbatim.
	Metadata map[string]any
	// OccurredAt is the wall-clock time the mutation happened. Populated
	// server-side on Record when zero.
	OccurredAt time.Time
}
