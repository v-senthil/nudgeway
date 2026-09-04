package integrationsettings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	dintegration "github.com/v-senthil/nudgeway/internal/domain/integration"
	"github.com/v-senthil/nudgeway/internal/domain/organization"
	"github.com/v-senthil/nudgeway/internal/ports/repository"
)

// ErrNotFound is returned when the requested integration does not exist
// (or belongs to a different organization). Callers translate this to
// a 404 at the REST edge.
var ErrNotFound = errors.New("integration not found")

// BusinessProfileDTO is the provider-agnostic view of a business
// profile edited from the settings drawer. Mirrors the WhatsApp
// adapter's BusinessProfile shape one-to-one for now — later providers
// map into this same set.
type BusinessProfileDTO struct {
	About             string   `json:"about,omitempty"`
	Address           string   `json:"address,omitempty"`
	Description       string   `json:"description,omitempty"`
	Email             string   `json:"email,omitempty"`
	ProfilePictureURL string   `json:"profile_picture_url,omitempty"`
	Vertical          string   `json:"vertical,omitempty"`
	Websites          []string `json:"websites,omitempty"`
}

// CallSettingsDTO is the provider-agnostic view of per-integration
// call settings. Currently modelled on Meta's `calling` sub-object.
type CallSettingsDTO struct {
	Status                   string        `json:"status,omitempty"`
	CallIconVisibility       string        `json:"call_icon_visibility,omitempty"`
	CallHours                *CallHoursDTO `json:"call_hours,omitempty"`
	CallbackPermissionStatus string        `json:"callback_permission_status,omitempty"`
}

// CallHoursDTO is the weekly operating window.
type CallHoursDTO struct {
	Status               string           `json:"status,omitempty"`
	TimezoneID           string           `json:"timezone_id,omitempty"`
	WeeklyOperatingHours []WeeklyHoursDTO `json:"weekly_operating_hours,omitempty"`
}

// WeeklyHoursDTO is a single day's open/close window. Times are
// carried as Meta's "HHMM" format; the frontend converts to/from
// "HH:MM" for display.
type WeeklyHoursDTO struct {
	DayOfWeek string `json:"day_of_week"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

// OBAStatusDTO is the compact view of an Official Business Account
// application. Values match Meta's enum: PENDING | APPROVED |
// REJECTED | CANCELLED | NOT_APPLIED.
type OBAStatusDTO struct {
	OBAStatus     string `json:"oba_status,omitempty"`
	StatusMessage string `json:"status_message,omitempty"`
}

// PhoneNumberDTO is the provider-neutral view of a channel phone number
// as returned by the settings drawer. Mirrors the WhatsApp adapter's
// PhoneNumber shape one-to-one; later channels map into the same set.
type PhoneNumberDTO struct {
	ID                        string `json:"id,omitempty"`
	DisplayPhoneNumber        string `json:"display_phone_number,omitempty"`
	VerifiedName              string `json:"verified_name,omitempty"`
	Status                    string `json:"status,omitempty"`
	QualityRating             string `json:"quality_rating,omitempty"`
	CountryCode               string `json:"country_code,omitempty"`
	CountryDialCode           string `json:"country_dial_code,omitempty"`
	CodeVerificationStatus    string `json:"code_verification_status,omitempty"`
	AccountMode               string `json:"account_mode,omitempty"`
	HostPlatform              string `json:"host_platform,omitempty"`
	MessagingLimitTier        string `json:"messaging_limit_tier,omitempty"`
	IsOfficialBusinessAccount bool   `json:"is_official_business_account,omitempty"`
}

// UsernameDTO is the provider-neutral view of a Business-Scoped Username.
// Empty Username means no username is currently adopted for the phone
// number — this is a valid steady state, not an error condition.
type UsernameDTO struct {
	Username string `json:"username,omitempty"`
	Status   string `json:"status,omitempty"` // "approved" | "reserved" | ""
}

// ProviderClient is the small surface each provider adapter must
// implement to be usable from the settings drawer. Implementations
// live in cmd/server so this package stays dependency-rule compliant.
type ProviderClient interface {
	GetBusinessProfile(ctx context.Context) (BusinessProfileDTO, error)
	UpdateBusinessProfile(ctx context.Context, bp BusinessProfileDTO) error
	GetCallSettings(ctx context.Context) (CallSettingsDTO, error)
	UpdateCallSettings(ctx context.Context, cs CallSettingsDTO) error
	GetOBAStatus(ctx context.Context) (OBAStatusDTO, error)
	ApplyOBA(ctx context.Context) (OBAStatusDTO, error)
	WithdrawOBA(ctx context.Context) (OBAStatusDTO, error)
	GetUsername(ctx context.Context) (UsernameDTO, error)
	SetUsername(ctx context.Context, username, transferAction string) (UsernameDTO, error)
	DeleteUsername(ctx context.Context) error
	GetUsernameSuggestions(ctx context.Context) ([]string, error)
	GetPhoneNumber(ctx context.Context) (PhoneNumberDTO, error)
	// SetWebhookOverride writes a provider-level webhook callback URL +
	// verify token. For WhatsApp this maps to Meta's
	// /{waba_id}?webhook_configuration override.
	SetWebhookOverride(ctx context.Context, callbackURL, verifyToken string) error
}

// WebhookConfigDTO is the outcome of a Set-webhook call: the fully-
// qualified provider webhook URL that was pushed to the provider, plus
// the verify token used. The verify token is returned as a convenience
// so operators can paste it into the provider console if the console
// UI wants it separately.
type WebhookConfigDTO struct {
	WebhookURL  string `json:"webhook_url"`
	VerifyToken string `json:"verify_token,omitempty"`
}

// Resolver builds a ProviderClient for a given integration + secrets
// combination. Implemented by cmd/server so provider imports never
// leak into internal/application.
type Resolver interface {
	Settings(ctx context.Context, providerKey string, integ dintegration.Integration, secrets map[string]string) (ProviderClient, error)
}

// CallPermission is the provider-agnostic view of a recipient's
// call-permission state. Mirrors ports/calling.Permission but stays local
// so the settings service does not import the calling port surface.
type CallPermission struct {
	Status         string `json:"status,omitempty"`
	ExpirationTime int64  `json:"expiration_time,omitempty"`
}

// CallPermissionLookup is the small port cmd/server implements to answer
// permission queries from the settings drawer. Kept as a callback so this
// package does not couple to ports/calling.
type CallPermissionLookup interface {
	LookupCallPermission(
		ctx context.Context,
		orgID organization.ID,
		id dintegration.ID,
		waID string,
	) (CallPermission, error)
}

// IntegrationSecretsRepo is the auxiliary read path that returns an
// integration together with its decrypted secrets. Duplicated locally
// (rather than importing the appintegration variant) so this package
// stays free of cross-service coupling.
type IntegrationSecretsRepo interface {
	repository.IntegrationRepo
	GetWithSecrets(ctx context.Context, orgID organization.ID, id dintegration.ID) (dintegration.Integration, map[string]string, error)
}

// Service is the use-case entry point for the settings drawer.
type Service struct {
	integrations IntegrationSecretsRepo
	providers    Resolver
	permissions  CallPermissionLookup
	logger       *slog.Logger
}

// Deps bundles the constructor arguments of Service.
type Deps struct {
	// Integrations exposes the row + decrypted secrets (required).
	Integrations IntegrationSecretsRepo
	// Providers resolves a ProviderClient for the integration
	// (required).
	Providers Resolver
	// Permissions is the optional call-permission lookup used by
	// GetCallPermission. Nil means the surface returns
	// ErrPermissionUnsupported.
	Permissions CallPermissionLookup
	// Logger receives handler-level warnings; nil falls back to slog.Default.
	Logger *slog.Logger
}

// ErrPermissionUnsupported is returned by GetCallPermission when no
// CallPermissionLookup is wired.
var ErrPermissionUnsupported = errors.New("integrationsettings: call permission lookup not configured")

// NewService constructs a Service. Panics if the required deps are nil.
func NewService(d Deps) *Service {
	if d.Integrations == nil {
		panic("integrationsettings.NewService: Integrations required")
	}
	if d.Providers == nil {
		panic("integrationsettings.NewService: Providers required")
	}
	return &Service{
		integrations: d.Integrations,
		providers:    d.Providers,
		permissions:  d.Permissions,
		logger:       d.Logger,
	}
}

// GetCallPermission returns the current WhatsApp user-call-permission for
// the given recipient waID (E.164). Delegates to the injected
// CallPermissionLookup so the settings package stays free of a direct
// dependency on ports/calling.
func (s *Service) GetCallPermission(
	ctx context.Context,
	orgID organization.ID,
	id dintegration.ID,
	waID string,
) (CallPermission, error) {
	if s.permissions == nil {
		return CallPermission{}, ErrPermissionUnsupported
	}
	return s.permissions.LookupCallPermission(ctx, orgID, id, waID)
}

// resolve loads the integration + decrypted secrets and hands them to
// the configured Resolver. The reserved "_integration_id" / "_org_id"
// keys mirror the send / read / attachment paths so the tracer can tag
// every emitted execution-log row.
func (s *Service) resolve(ctx context.Context, orgID organization.ID, id dintegration.ID) (ProviderClient, error) {
	row, secrets, err := s.integrations.GetWithSecrets(ctx, orgID, id)
	if err != nil {
		return nil, fmt.Errorf("integrationsettings: load integration: %w", err)
	}
	if row.ID == "" {
		return nil, ErrNotFound
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	// Enrich secrets with the config fields the whatsapp adapter reads
	// so callers get parity with the send-path wire-up.
	if v, ok := row.Config["phone_number_id"].(string); ok {
		secrets["phone_number_id"] = v
	}
	if v, ok := row.Config["waba_id"].(string); ok {
		secrets["waba_id"] = v
	}
	secrets["_integration_id"] = string(row.ID)
	secrets["_org_id"] = string(row.OrgID)
	return s.providers.Settings(ctx, row.Provider, row, secrets)
}

// GetBusinessProfile fetches the business profile for the integration.
func (s *Service) GetBusinessProfile(ctx context.Context, orgID organization.ID, id dintegration.ID) (BusinessProfileDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return BusinessProfileDTO{}, err
	}
	return pc.GetBusinessProfile(ctx)
}

// UpdateBusinessProfile writes the provided fields.
func (s *Service) UpdateBusinessProfile(ctx context.Context, orgID organization.ID, id dintegration.ID, bp BusinessProfileDTO) error {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return err
	}
	return pc.UpdateBusinessProfile(ctx, bp)
}

// GetCallSettings fetches call settings.
func (s *Service) GetCallSettings(ctx context.Context, orgID organization.ID, id dintegration.ID) (CallSettingsDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return CallSettingsDTO{}, err
	}
	return pc.GetCallSettings(ctx)
}

// UpdateCallSettings writes call settings.
func (s *Service) UpdateCallSettings(ctx context.Context, orgID organization.ID, id dintegration.ID, cs CallSettingsDTO) error {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return err
	}
	return pc.UpdateCallSettings(ctx, cs)
}

// GetOBAStatus fetches the Official Business Account status.
func (s *Service) GetOBAStatus(ctx context.Context, orgID organization.ID, id dintegration.ID) (OBAStatusDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return OBAStatusDTO{}, err
	}
	return pc.GetOBAStatus(ctx)
}

// ApplyOBA submits an OBA application.
func (s *Service) ApplyOBA(ctx context.Context, orgID organization.ID, id dintegration.ID) (OBAStatusDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return OBAStatusDTO{}, err
	}
	return pc.ApplyOBA(ctx)
}

// WithdrawOBA cancels an in-flight OBA application.
func (s *Service) WithdrawOBA(ctx context.Context, orgID organization.ID, id dintegration.ID) (OBAStatusDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return OBAStatusDTO{}, err
	}
	return pc.WithdrawOBA(ctx)
}

// GetUsername fetches the current business username for the integration.
// A zero-value UsernameDTO with nil error means no username is adopted —
// callers render an "adopt" form instead of an error.
func (s *Service) GetUsername(ctx context.Context, orgID organization.ID, id dintegration.ID) (UsernameDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return UsernameDTO{}, err
	}
	return pc.GetUsername(ctx)
}

// SetUsername adopts or changes the business username. transferAction is
// "none" (default) or "force_transfer" — the latter reclaims a username
// held on another of the operator's phone numbers (Meta error 147005).
func (s *Service) SetUsername(ctx context.Context, orgID organization.ID, id dintegration.ID, username, transferAction string) (UsernameDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return UsernameDTO{}, err
	}
	return pc.SetUsername(ctx, username, transferAction)
}

// DeleteUsername releases the current username.
func (s *Service) DeleteUsername(ctx context.Context, orgID organization.ID, id dintegration.ID) error {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return err
	}
	return pc.DeleteUsername(ctx)
}

// GetUsernameSuggestions returns Meta's suggested usernames. Fired only
// when the operator explicitly requests them (Meta throttles heavily).
func (s *Service) GetUsernameSuggestions(ctx context.Context, orgID organization.ID, id dintegration.ID) ([]string, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	return pc.GetUsernameSuggestions(ctx)
}

// SetWebhook pushes the current integration's webhook callback URL +
// verify token to the provider. The caller supplies the public base
// URL (e.g. "https://a1b2.ngrok-free.app"); this method appends the
// canonical webhook path for the integration and pulls the verify
// token from the stored secrets. Returns the fully-qualified URL that
// was pushed so the caller can echo it back to the operator.
//
// A blank publicBaseURL is rejected with ErrValidation — Meta requires
// an https URL. Trailing slashes on publicBaseURL are stripped.
func (s *Service) SetWebhook(ctx context.Context, orgID organization.ID, id dintegration.ID, publicBaseURL string) (WebhookConfigDTO, error) {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if publicBaseURL == "" {
		return WebhookConfigDTO{}, &ErrValidation{Msg: "public_url is required"}
	}
	if !strings.HasPrefix(publicBaseURL, "https://") && !strings.HasPrefix(publicBaseURL, "http://") {
		return WebhookConfigDTO{}, &ErrValidation{Msg: "public_url must start with https:// (or http:// for local dev)"}
	}
	row, secrets, err := s.integrations.GetWithSecrets(ctx, orgID, id)
	if err != nil {
		return WebhookConfigDTO{}, fmt.Errorf("integrationsettings: load integration: %w", err)
	}
	if row.ID == "" {
		return WebhookConfigDTO{}, ErrNotFound
	}
	verifyToken := secrets["verify_token"]
	if verifyToken == "" {
		return WebhookConfigDTO{}, &ErrValidation{Msg: "integration has no verify_token stored; recreate with a verify token"}
	}
	callbackURL := publicBaseURL + "/webhooks/" + row.Provider + "/" + string(row.ID)

	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return WebhookConfigDTO{}, err
	}
	if err := pc.SetWebhookOverride(ctx, callbackURL, verifyToken); err != nil {
		return WebhookConfigDTO{}, err
	}
	return WebhookConfigDTO{WebhookURL: callbackURL, VerifyToken: verifyToken}, nil
}

// ErrValidation is returned by SetWebhook when the caller-supplied input
// is missing / malformed. Translated to 422 at the REST edge.
type ErrValidation struct{ Msg string }

// Error implements error.
func (e *ErrValidation) Error() string { return e.Msg }

// GetPhoneNumber returns the Meta phone-number record for the
// integration's configured phone number id. A zero-value DTO with nil
// error means the id is not present on the WABA's phone-number list.
func (s *Service) GetPhoneNumber(ctx context.Context, orgID organization.ID, id dintegration.ID) (PhoneNumberDTO, error) {
	pc, err := s.resolve(ctx, orgID, id)
	if err != nil {
		return PhoneNumberDTO{}, err
	}
	return pc.GetPhoneNumber(ctx)
}
