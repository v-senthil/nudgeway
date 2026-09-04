// Package whatsapp adapts the Meta WhatsApp Business Cloud API to the
// canonical channel port. It is the ONLY place in the tree that speaks
// the Meta Graph API vocabulary.
//
// Reference (source of truth): ~/Documents/whatsapp_doc_tracker/docs/.
// Nothing in this package invents APIs that aren't documented there.
package whatsapp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/events"
	"github.com/v-senthil/nudgeway/internal/ports/channel"
	"github.com/v-senthil/nudgeway/internal/providers"
)

// providerKey is the stable identifier used in the registry + integration rows.
const providerKey = "whatsapp"

func init() {
	providers.Register(providers.Descriptor{
		Kind: providers.KindChannel,
		Key:  providerKey,
		Name: "WhatsApp Business Cloud API",
	})
}

// Provider implements the channel.Provider port over Meta's Cloud API.
type Provider struct {
	cfg      Config
	client   *client
	resolver EndpointResolver
	healthy  atomic.Bool
}

// New constructs a Provider. The resolver is optional at construction time;
// wire it before serving webhooks via WithEndpointResolver.
func New(cfg Config) *Provider {
	p := &Provider{cfg: cfg, client: newClient(cfg)}
	p.healthy.Store(true)
	return p
}

// WithEndpointResolver injects the resolver used to map inbound
// phone_number_id → tenant. Safe to call at wire-up time; do not call
// concurrently with ParseWebhook.
func (p *Provider) WithEndpointResolver(r EndpointResolver) *Provider {
	p.resolver = r
	return p
}

// WithTracer sets the Tracer the HTTP client emits execution-log events
// to and returns the Provider so wire-up can chain. Safe to call at
// wire-up time; do not call concurrently with in-flight adapter methods.
//
// This is the sanctioned wire-up point for the operator-facing execution
// log — cmd/server closes over providercall.Service.Record and passes it
// here after constructing the Provider. Tracing stays provider-internal
// so the channel.Provider port never grows a bookkeeping method.
func (p *Provider) WithTracer(t Tracer, integrationID, orgID string) *Provider {
	p.cfg.Tracer = t
	p.cfg.IntegrationID = integrationID
	p.cfg.OrgID = orgID
	p.client = newClient(p.cfg)
	return p
}

// Key returns the provider's registry key.
func (p *Provider) Key() string { return providerKey }

// SendMessage translates a canonical SendRequest into a Meta /messages POST.
func (p *Provider) SendMessage(ctx context.Context, req channel.SendRequest) (channel.SendResult, error) {
	if req.To == "" {
		return channel.SendResult{}, fmt.Errorf("whatsapp: SendMessage: recipient required")
	}
	if req.MessageType == "" {
		return channel.SendResult{}, fmt.Errorf("whatsapp: SendMessage: message type required")
	}
	body, err := canonicalSendToMeta(req)
	if err != nil {
		return channel.SendResult{}, err
	}
	resp, err := p.client.sendMessage(ctx, body)
	if err != nil {
		if apiErr := AsAPIError(err); apiErr != nil && apiErr.Class == ClassAuth {
			p.healthy.Store(false)
		}
		return channel.SendResult{}, err
	}
	if len(resp.Messages) == 0 {
		return channel.SendResult{}, fmt.Errorf("whatsapp: Meta returned no message id")
	}
	p.healthy.Store(true)
	return channel.SendResult{
		ProviderMessageID: resp.Messages[0].ID,
		AcceptedAt:        time.Now().UTC().Unix(),
	}, nil
}

// ParseWebhook implements channel.Provider. Signature verification is the
// caller's responsibility (call VerifySignature on the raw body first).
//
// Meta multiplexes messages and calls onto the same webhook endpoint —
// changes[].field is either "messages" or "calls". Run both parsers and
// concatenate the envelopes so downstream dispatch sees both streams from
// a single POST body. If the call parser errs we still return whatever
// message envelopes were extracted so a malformed call sub-payload never
// hides a valid inbound message.
func (p *Provider) ParseWebhook(ctx context.Context, headers map[string][]string, body []byte) ([]events.Envelope, error) {
	_ = ctx // parsing is CPU-only; ctx kept for interface parity.
	msgs, err := ParseWebhook(body, p.resolver)
	if err != nil {
		return nil, err
	}
	calls, cerr := ParseCallWebhook(body, p.resolver)
	if cerr != nil {
		// Don't lose valid message envelopes to a call-side parse error.
		// Log via slog so operators still see call-parse regressions.
		slog.Default().Warn("whatsapp: parse call webhook",
			slog.Any("err", cerr),
		)
		return msgs, nil
	}
	if len(calls) == 0 {
		return msgs, nil
	}
	return append(msgs, calls...), nil
}

// MarkAsRead flips a received message to "read" on the customer's device
// (Meta shows the blue double-tick). providerMessageID is Meta's wamid from
// the inbound webhook. Meta must be called within 30 days of receipt.
func (p *Provider) MarkAsRead(ctx context.Context, providerMessageID string) error {
	if providerMessageID == "" {
		return fmt.Errorf("whatsapp: MarkAsRead: provider_message_id required")
	}
	if err := p.client.markAsRead(ctx, providerMessageID); err != nil {
		if apiErr := AsAPIError(err); apiErr != nil && apiErr.Class == ClassAuth {
			p.healthy.Store(false)
		}
		return err
	}
	p.healthy.Store(true)
	return nil
}

// DownloadMedia resolves a Meta media ID to a short-lived download URL,
// then streams the bytes back to the caller. It is the two-step Meta
// pattern (GET /<media_id> → JSON with a `url` field, then GET on that
// url with the Bearer token) packaged as a single streaming call.
//
// The returned io.ReadCloser MUST be closed by the caller. The second
// return value is the Content-Type Meta served the bytes with — the
// InboundService persists it as the attachment sidecar so the browser
// can render <img>/<video>/<audio> without re-sniffing.
func (p *Provider) DownloadMedia(ctx context.Context, mediaID string) (io.ReadCloser, string, error) {
	if mediaID == "" {
		return nil, "", fmt.Errorf("whatsapp: DownloadMedia: mediaID required")
	}
	lookup, err := p.client.getMediaURL(ctx, mediaID)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp: lookup media url: %w", err)
	}
	body, ctype, err := p.client.downloadMedia(ctx, lookup.URL)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp: download media bytes: %w", err)
	}
	if ctype == "" {
		ctype = lookup.MimeType
	}
	return body, ctype, nil
}

// UploadMedia uploads the raw bytes of a media asset to Meta and returns
// the opaque media_id the outbound send API expects under {"id": "..."}.
// This is the preferred outbound path — a media_id skips the "resolvable
// URL" contract Meta imposes on the {"link": "..."} variant.
//
// contentType is the MIME type of the blob (e.g. "image/jpeg"). filename
// is optional metadata carried in the multipart body — Meta ignores it
// for most types but uses it as the document filename when type=document
// and no caption filename is supplied at send time.
//
// Reference: business-phone-numbers/media.md.
func (p *Provider) UploadMedia(ctx context.Context, contentType, filename string, r io.Reader) (string, error) {
	if contentType == "" {
		return "", fmt.Errorf("whatsapp: UploadMedia: contentType required")
	}
	id, err := p.client.uploadMedia(ctx, contentType, filename, r)
	if err != nil {
		if apiErr := AsAPIError(err); apiErr != nil && apiErr.Class == ClassAuth {
			p.healthy.Store(false)
		}
		return "", err
	}
	p.healthy.Store(true)
	return id, nil
}

// DownloadMediaByURL streams bytes from a URL Meta already gave us in the
// webhook envelope (image/video/audio/document/sticker `url` field). It
// skips the /v20.0/{media_id} lookup step — the URL is already signed +
// hash-scoped; we just need the Bearer token.
//
// Prefer this path when a URL is present in the payload: it's one HTTPS
// round-trip instead of two.
func (p *Provider) DownloadMediaByURL(ctx context.Context, url string) (io.ReadCloser, string, error) {
	if url == "" {
		return nil, "", fmt.Errorf("whatsapp: DownloadMediaByURL: url required")
	}
	return p.client.downloadMedia(ctx, url)
}

// HealthCheck reports the last-known outbound health. A dedicated ping call
// is intentionally not made here — Meta rate-limits the app-review pings and
// day-to-day sends are the truest signal.
func (p *Provider) HealthCheck(ctx context.Context) channel.HealthStatus {
	_ = ctx
	if p.healthy.Load() {
		return channel.HealthStatus{OK: true, Message: "ok"}
	}
	return channel.HealthStatus{OK: false, Message: "last send failed auth or transport"}
}
