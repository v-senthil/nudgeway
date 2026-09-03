package whatsapp

import (
	"net/http"
	"time"
)

// DefaultGraphVersion is the Meta Graph API version pinned by the adapter.
// Bump this in a single ADR-noted commit when Meta releases a new version.
const DefaultGraphVersion = "v20.0"

// DefaultBaseURL is the Meta Graph host. Overridable for tests/mock servers.
const DefaultBaseURL = "https://graph.facebook.com"

// DefaultTimeout is the per-request HTTP timeout applied when the caller does
// not inject a client of their own.
const DefaultTimeout = 30 * time.Second

// Config declares the credentials + tunables required to talk to one Meta
// WhatsApp Business Cloud API integration. One Config per BusinessEndpoint.
type Config struct {
	// PhoneNumberID is the WhatsApp Business phone number ID used as the
	// path parameter on send calls (e.g. .../<phone_number_id>/messages).
	PhoneNumberID string

	// WABAID is the WhatsApp Business Account ID that owns the phone number.
	// Used for template management calls.
	WABAID string

	// AccessToken is the system / business integration user token used as
	// a Bearer credential. Never log.
	AccessToken string

	// AppSecret is the Meta app secret used to verify X-Hub-Signature-256
	// headers on incoming webhooks.
	AppSecret string

	// GraphVersion overrides DefaultGraphVersion when set.
	GraphVersion string

	// BaseURL overrides DefaultBaseURL when set (used by tests + mock servers).
	BaseURL string

	// HTTPClient overrides the internally-constructed default client.
	HTTPClient *http.Client
}

// version returns the effective Graph API version.
func (c Config) version() string {
	if c.GraphVersion == "" {
		return DefaultGraphVersion
	}
	return c.GraphVersion
}

// baseURL returns the effective Graph base URL.
func (c Config) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return c.BaseURL
}

// httpClient returns the HTTP client to use for outbound calls. Callers get
// a shared default with DefaultTimeout when they do not inject their own.
func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: DefaultTimeout}
}
