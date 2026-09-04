package template

import (
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
)

// ID identifies a Template row.
type ID string

// Category is the provider-neutral template category. Values mirror the
// the WhatsApp provider vocabulary because the platform grew up around WhatsApp; other
// providers map their own categories onto these three at the adapter
// boundary.
type Category string

// Category values.
const (
	// CategoryMarketing is promotional / brand-awareness content.
	CategoryMarketing Category = "MARKETING"
	// CategoryUtility is transactional content (order updates, receipts,
	// account notifications).
	CategoryUtility Category = "UTILITY"
	// CategoryAuthentication is one-time password + verification content.
	CategoryAuthentication Category = "AUTHENTICATION"
)

// Status enumerates the review lifecycle of a Template. DRAFT is our
// pre-submission state — the WhatsApp provider never sees these. Everything else mirrors
// the WhatsApp provider's review vocabulary so a Sync round-trip does not lose fidelity.
type Status string

// Status values.
const (
	// StatusDraft is the tenant-local pre-submission state.
	StatusDraft Status = "DRAFT"
	// StatusPending means the template is under provider review.
	StatusPending Status = "PENDING"
	// StatusApproved means the provider approved the template for send.
	StatusApproved Status = "APPROVED"
	// StatusRejected means the provider rejected the template; edit + resubmit.
	StatusRejected Status = "REJECTED"
	// StatusPaused means the provider temporarily paused the template
	// (usually due to quality signal issues).
	StatusPaused Status = "PAUSED"
	// StatusDisabled means the provider permanently disabled the template.
	StatusDisabled Status = "DISABLED"
)

// Component is a provider-neutral template component. The the WhatsApp provider component
// vocabulary is the widest we've mapped so far — buttons, headers, footers,
// bodies, carousels — so Component keeps the union shape flat with
// optional fields the caller populates per Type.
//
// Reference: ~/Documents/whatsapp_doc_tracker/docs/templates/components.md.
type Component struct {
	// Type is the component kind: "HEADER", "BODY", "FOOTER", "BUTTONS", ...
	Type string `json:"type"`
	// Format is the header format when Type=HEADER: "TEXT", "IMAGE",
	// "VIDEO", "DOCUMENT", "LOCATION". Empty otherwise.
	Format string `json:"format,omitempty"`
	// Text is the rendered body text with {{n}} placeholders.
	Text string `json:"text,omitempty"`
	// Example carries the sample values the WhatsApp provider requires when the component
	// uses variables (header_text / body_text / body_text_named_params).
	Example map[string]any `json:"example,omitempty"`
	// Buttons is the button list when Type=BUTTONS. Each button is a
	// free-form map because the WhatsApp provider's button vocabulary (URL, PHONE_NUMBER,
	// QUICK_REPLY, COPY_CODE, OTP, ...) grows frequently.
	Buttons []map[string]any `json:"buttons,omitempty"`
	// Cards is the carousel card list when Type=CAROUSEL.
	Cards []map[string]any `json:"cards,omitempty"`
	// Extra preserves any provider-specific fields that don't map cleanly
	// onto the named fields above. Marshaled through untouched.
	Extra map[string]any `json:"extra,omitempty"`
}

// Template is the tenant-scoped provider-neutral message template.
//
// A Template is minted DRAFT locally, submitted to the provider via
// SubmitForReview which flips it to PENDING and populates
// ProviderTemplateID, and thereafter mirrors the provider's status via
// Sync.
type Template struct {
	ID                 ID
	OrgID              organization.ID
	IntegrationID      integration.ID
	ProviderTemplateID string
	Name               string
	Language           string
	Category           Category
	Status             Status
	Components         []Component
	// Variables is a snapshot of the {{name}} → example-value map extracted
	// from Components at persist time so the composer can render preview
	// values without walking the component tree again.
	Variables      map[string]string
	LastSyncedAt   *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
