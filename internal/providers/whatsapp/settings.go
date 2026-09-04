package whatsapp

// This file adds the Meta Graph surfaces for editing WhatsApp Business
// integration settings: the business profile, calling settings, and
// Official Business Account (OBA) status.
//
// References (source of truth, do NOT invent):
//   - ~/Documents/whatsapp_doc_tracker/docs/business-profiles.md
//   - ~/Documents/whatsapp_doc_tracker/docs/calling/call-settings.md
//   - ~/Documents/whatsapp_doc_tracker/docs/reference/whatsapp-business-phone-number/
//     whatsapp-business-account-official-business-account-status-api.md
//
// Every HTTP round-trip is routed through client.doJSON so the tracer
// records it under a distinct `op` string. Follow-ups deferred to a
// later pass:
//   - SIP configuration on call settings.
//   - holiday_schedule / restrict_to_user_countries on call settings.
//   - profile picture upload (multipart /photos endpoint).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// BusinessProfile mirrors the editable subset of Meta's
// whatsapp_business_profile object. Fields left empty on update are
// omitted from the outbound payload (Meta treats missing keys as
// "leave unchanged"), matching the semantics operators expect from an
// edit form.
type BusinessProfile struct {
	About             string   `json:"about,omitempty"`
	Address           string   `json:"address,omitempty"`
	Description       string   `json:"description,omitempty"`
	Email             string   `json:"email,omitempty"`
	ProfilePictureURL string   `json:"profile_picture_url,omitempty"`
	Vertical          string   `json:"vertical,omitempty"`
	Websites          []string `json:"websites,omitempty"`
}

// businessProfileEnvelope is the {"data": [ { ... } ]} shape Meta wraps
// GET responses in. Only the first entry is meaningful — there is one
// profile per phone number.
type businessProfileEnvelope struct {
	Data []BusinessProfile `json:"data"`
}

// GetBusinessProfile returns the current business profile for the
// integration's phone number.
func (p *Provider) GetBusinessProfile(ctx context.Context) (BusinessProfile, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/whatsapp_business_profile?fields=about,address,description,email,profile_picture_url,websites,vertical",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	var env businessProfileEnvelope
	if err := p.client.doJSON(ctx, "get_business_profile", http.MethodGet, url, nil, &env); err != nil {
		return BusinessProfile{}, err
	}
	if len(env.Data) == 0 {
		return BusinessProfile{}, nil
	}
	return env.Data[0], nil
}

// UpdateBusinessProfile writes the supplied fields to Meta. Meta requires
// the "messaging_product":"whatsapp" preamble on every profile POST.
func (p *Provider) UpdateBusinessProfile(ctx context.Context, bp BusinessProfile) error {
	url := fmt.Sprintf(
		"%s/%s/%s/whatsapp_business_profile",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	payload := map[string]any{"messaging_product": "whatsapp"}
	if bp.About != "" {
		payload["about"] = bp.About
	}
	if bp.Address != "" {
		payload["address"] = bp.Address
	}
	if bp.Description != "" {
		payload["description"] = bp.Description
	}
	if bp.Email != "" {
		payload["email"] = bp.Email
	}
	if bp.ProfilePictureURL != "" {
		payload["profile_picture_url"] = bp.ProfilePictureURL
	}
	if bp.Vertical != "" {
		payload["vertical"] = bp.Vertical
	}
	if bp.Websites != nil {
		payload["websites"] = bp.Websites
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("whatsapp: encode business profile: %w", err)
	}
	var resp map[string]any
	return p.client.doJSON(ctx, "update_business_profile", http.MethodPost, url, body, &resp)
}

// CallSettings mirrors Meta's `calling` sub-object nested under
// /{phone_number_id}/settings. SIP config, holiday_schedule, and
// restrict_to_user_countries are intentionally omitted — they get a
// follow-up pass once the base surface is verified.
type CallSettings struct {
	Status                   string     `json:"status,omitempty"`
	CallIconVisibility       string     `json:"call_icon_visibility,omitempty"`
	CallHours                *CallHours `json:"call_hours,omitempty"`
	CallbackPermissionStatus string     `json:"callback_permission_status,omitempty"`
}

// CallHours is the weekly operating window Meta uses to decide when to
// surface the in-app call button on the customer's device.
type CallHours struct {
	Status               string        `json:"status,omitempty"`
	TimezoneID           string        `json:"timezone_id,omitempty"`
	WeeklyOperatingHours []WeeklyHours `json:"weekly_operating_hours,omitempty"`
}

// WeeklyHours is a single day's open/close window. OpenTime / CloseTime
// are Meta's "HHMM" format (e.g. "0900", "1730").
type WeeklyHours struct {
	DayOfWeek string `json:"day_of_week"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

// callSettingsEnvelope is the shape /{pnid}/settings returns —
// call settings sit under a nested `calling` object alongside other
// per-phone settings (sip, etc.) which we currently ignore.
type callSettingsEnvelope struct {
	Calling *CallSettings `json:"calling,omitempty"`
}

// GetCallSettings extracts the .calling block from Meta's /settings
// response. A missing block returns a zero-value CallSettings — Meta
// treats this as "defaults" on the read side.
func (p *Provider) GetCallSettings(ctx context.Context) (CallSettings, error) {
	url := fmt.Sprintf("%s/%s/%s/settings", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	var env callSettingsEnvelope
	if err := p.client.doJSON(ctx, "get_call_settings", http.MethodGet, url, nil, &env); err != nil {
		return CallSettings{}, err
	}
	if env.Calling == nil {
		return CallSettings{}, nil
	}
	return *env.Calling, nil
}

// UpdateCallSettings POSTs the supplied CallSettings under the `calling`
// key. Meta requires the messaging_product preamble here too.
//
// Meta rejects any `call_hours` sub-object that omits weekly_operating_hours
// (even when call_hours.status is DISABLED). If the caller supplied a
// CallHours with no schedule rows, drop the whole sub-object from the
// outbound payload — Meta treats the missing key as "leave existing
// call_hours configuration untouched", which is what the operator meant
// when they turned the toggle off without adding hours.
func (p *Provider) UpdateCallSettings(ctx context.Context, cs CallSettings) error {
	url := fmt.Sprintf("%s/%s/%s/settings", p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID)
	if cs.CallHours != nil && len(cs.CallHours.WeeklyOperatingHours) == 0 {
		cs.CallHours = nil
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"calling":           cs,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("whatsapp: encode call settings: %w", err)
	}
	var resp map[string]any
	return p.client.doJSON(ctx, "update_call_settings", http.MethodPost, url, body, &resp)
}

// OBAStatus is the compact view of Meta's Official Business Account
// application status. Values: PENDING | APPROVED | REJECTED |
// CANCELLED | NOT_APPLIED. See docs reference above for the state
// transitions Meta permits.
type OBAStatus struct {
	OBAStatus     string `json:"oba_status,omitempty"`
	StatusMessage string `json:"status_message,omitempty"`
}

// GetOBAStatus reads the current OBA state for the phone number.
func (p *Provider) GetOBAStatus(ctx context.Context) (OBAStatus, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/official_business_account",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	var out OBAStatus
	if err := p.client.doJSON(ctx, "get_oba_status", http.MethodGet, url, nil, &out); err != nil {
		return OBAStatus{}, err
	}
	return out, nil
}

// ApplyOBA submits an OBA application (POST with action=apply).
func (p *Provider) ApplyOBA(ctx context.Context) (OBAStatus, error) {
	return p.postOBAAction(ctx, "apply")
}

// WithdrawOBA cancels an in-flight OBA application (POST action=withdraw).
func (p *Provider) WithdrawOBA(ctx context.Context) (OBAStatus, error) {
	return p.postOBAAction(ctx, "withdraw")
}

// postOBAAction is the shared POST helper for /official_business_account.
func (p *Provider) postOBAAction(ctx context.Context, action string) (OBAStatus, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/official_business_account",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	body, err := json.Marshal(map[string]string{"action": action})
	if err != nil {
		return OBAStatus{}, fmt.Errorf("whatsapp: encode oba action: %w", err)
	}
	var out OBAStatus
	if err := p.client.doJSON(ctx, "update_oba_status", http.MethodPost, url, body, &out); err != nil {
		return OBAStatus{}, err
	}
	return out, nil
}

// Username is the compact view of Meta's Business-Scoped Username surface.
// When the phone number has no username, both fields come back empty —
// callers treat the zero-value as "no username adopted yet" rather than
// an error. See ~/Documents/whatsapp_doc_tracker/docs/business-scoped-user-ids.md.
type Username struct {
	// Username is the current handle (lowercase; a-z 0-9 . _; 3–35 chars).
	Username string `json:"username,omitempty"`
	// Status is Meta's lifecycle state: "approved" (in use) or "reserved"
	// (transiently held during a change). Empty when no username set.
	Status string `json:"status,omitempty"`
}

// usernameSuggestionsEnvelope accepts both shapes Meta has returned from
// /{pnid}/username_suggestions:
//
//  1. Documented wrapper: {"data":[{"username_suggestions":[...]}]}
//  2. Actual (observed):  {"username_suggestions":[...]} at the top level
//
// Both fields are decoded in one pass; the callsite prefers the top-level
// list when present and falls back to the first data entry.
type usernameSuggestionsEnvelope struct {
	UsernameSuggestions []string `json:"username_suggestions"`
	Data                []struct {
		UsernameSuggestions []string `json:"username_suggestions"`
	} `json:"data"`
}

// GetUsername returns the current business username for the integration's
// phone number. A phone number with no username returns Username{} nil-err
// (Meta responds with an empty object / omitted fields — not a 404).
func (p *Provider) GetUsername(ctx context.Context) (Username, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/username",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	var out Username
	if err := p.client.doJSON(ctx, "get_username", http.MethodGet, url, nil, &out); err != nil {
		return Username{}, err
	}
	return out, nil
}

// SetUsername adopts or changes the business username. transferAction is
// "none" or "force_transfer"; the latter is required when the desired
// username is already reserved/approved on another phone number in the
// same portfolio (Meta returns error 147005 in that case).
func (p *Provider) SetUsername(ctx context.Context, username, transferAction string) (Username, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/username",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	payload := map[string]any{"username": username}
	if transferAction != "" {
		payload["transfer_action"] = transferAction
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Username{}, fmt.Errorf("whatsapp: encode username: %w", err)
	}
	var out Username
	if err := p.client.doJSON(ctx, "set_username", http.MethodPost, url, body, &out); err != nil {
		return Username{}, err
	}
	// Meta's POST response echoes only {status}; carry the requested name
	// forward so callers see the reconciled state without a follow-up GET.
	if out.Username == "" {
		out.Username = username
	}
	return out, nil
}

// DeleteUsername releases the current username. Meta returns {success:bool};
// a false success surfaces as an *APIError from doJSON when the HTTP status
// signals failure, so a nil error here is treated as success.
func (p *Provider) DeleteUsername(ctx context.Context) error {
	url := fmt.Sprintf(
		"%s/%s/%s/username",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	var resp map[string]any
	return p.client.doJSON(ctx, "delete_username", http.MethodDelete, url, nil, &resp)
}

// PhoneNumber is the compact view of Meta's phone-number object returned
// by GET /{waba_id}/phone_numbers. Fields mirror the wire names one-to-
// one; empty means Meta omitted the field for this account tier (e.g.
// throughput on legacy accounts).
type PhoneNumber struct {
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

// phoneNumbersEnvelope is the {"data":[...]} wrapper Meta returns for
// GET /{waba_id}/phone_numbers.
type phoneNumbersEnvelope struct {
	Data []PhoneNumber `json:"data"`
}

// GetPhoneNumber returns the Meta phone-number record whose id matches
// the integration's configured PhoneNumberID. Returns a zero-value
// PhoneNumber with nil error when the id is not present in the list —
// callers interpret empty as "not part of this WABA yet".
func (p *Provider) GetPhoneNumber(ctx context.Context) (PhoneNumber, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/phone_numbers?fields=id,display_phone_number,verified_name,status,quality_rating,country_code,country_dial_code,code_verification_status,account_mode,host_platform,messaging_limit_tier,is_official_business_account,throughput",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.WABAID,
	)
	var env phoneNumbersEnvelope
	if err := p.client.doJSON(ctx, "get_phone_number", http.MethodGet, url, nil, &env); err != nil {
		return PhoneNumber{}, err
	}
	for _, pn := range env.Data {
		if pn.ID == p.cfg.PhoneNumberID {
			return pn, nil
		}
	}
	return PhoneNumber{}, nil
}

// GetUsernameSuggestions returns Meta's suggested usernames (0-N entries)
// for the phone number. Meta throttles this endpoint aggressively, so
// callers only fire it when the operator explicitly asks for suggestions.
func (p *Provider) GetUsernameSuggestions(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf(
		"%s/%s/%s/username_suggestions",
		p.cfg.baseURL(), p.cfg.version(), p.cfg.PhoneNumberID,
	)
	var env usernameSuggestionsEnvelope
	if err := p.client.doJSON(ctx, "get_username_suggestions", http.MethodGet, url, nil, &env); err != nil {
		return nil, err
	}
	// Prefer the top-level list Meta actually returns; fall back to the
	// documented wrapper shape if that's what we get.
	if len(env.UsernameSuggestions) > 0 {
		return env.UsernameSuggestions, nil
	}
	if len(env.Data) > 0 {
		return env.Data[0].UsernameSuggestions, nil
	}
	return []string{}, nil
}
