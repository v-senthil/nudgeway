package whatsapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// client is a thin HTTP wrapper around the subset of the Meta Graph API used
// by the WhatsApp adapter. Retries live here rather than at the caller.
type client struct {
	cfg Config
}

func newClient(cfg Config) *client { return &client{cfg: cfg} }

// sendMessage POSTs the given canonical Meta outbound body to /messages.
func (c *client) sendMessage(ctx context.Context, body []byte) (metaSendResponse, error) {
	url := fmt.Sprintf("%s/%s/%s/messages", c.cfg.baseURL(), c.cfg.version(), c.cfg.PhoneNumberID)
	var resp metaSendResponse
	if err := c.doJSON(ctx, http.MethodPost, url, body, &resp); err != nil {
		return metaSendResponse{}, err
	}
	return resp, nil
}

// getMediaURL resolves a Meta media ID to a short-lived download URL.
// See /messages/audio-messages.md, /image-messages.md, etc. — the pattern
// is GET /<media_id> returning a JSON object with a `url` field.
func (c *client) getMediaURL(ctx context.Context, mediaID string) (mediaLookupResponse, error) {
	url := fmt.Sprintf("%s/%s/%s", c.cfg.baseURL(), c.cfg.version(), mediaID)
	var resp mediaLookupResponse
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return mediaLookupResponse{}, err
	}
	return resp, nil
}

// downloadMedia GETs a Meta-provided media URL with the Bearer token. Meta
// returns the raw bytes; caller is responsible for closing the returned body.
func (c *client) downloadMedia(ctx context.Context, url string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp: build media download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	res, err := c.cfg.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("whatsapp: media download: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, "", &APIError{
			Class:      classifyStatus(res.StatusCode, 0, ""),
			StatusCode: res.StatusCode,
			Message:    "media download failed",
			Raw:        raw,
		}
	}
	return res.Body, res.Header.Get("Content-Type"), nil
}

// listTemplates returns templates registered on the WABA.
func (c *client) listTemplates(ctx context.Context) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s/message_templates", c.cfg.baseURL(), c.cfg.version(), c.cfg.WABAID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// createTemplate submits a new template for Meta review.
func (c *client) createTemplate(ctx context.Context, body []byte) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s/message_templates", c.cfg.baseURL(), c.cfg.version(), c.cfg.WABAID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodPost, url, body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// getTemplateStatus fetches a single template by id.
func (c *client) getTemplateStatus(ctx context.Context, templateID string) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s", c.cfg.baseURL(), c.cfg.version(), templateID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// doJSON is the retrying JSON request core.
func (c *client) doJSON(ctx context.Context, method, url string, body []byte, out any) error {
	const maxAttempts = 4
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := backoff(attempt)
			select {
			case <-ctx.Done():
				return fmt.Errorf("whatsapp: context cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}
		err := c.doOnce(ctx, method, url, body, out)
		if err == nil {
			return nil
		}
		last = err
		apiErr := AsAPIError(err)
		if apiErr == nil || !apiErr.Retryable() {
			return err
		}
	}
	return last
}

func (c *client) doOnce(ctx context.Context, method, url string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("whatsapp: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.cfg.httpClient().Do(req)
	if err != nil {
		return &APIError{Class: ClassTransient, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("whatsapp: read body: %w", err)
	}
	if res.StatusCode >= 400 {
		return parseErrorResponse(res.StatusCode, raw, res.Header.Get("x-fb-trace-id"))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if rawMsg, ok := out.(*json.RawMessage); ok {
		*rawMsg = append((*rawMsg)[:0], raw...)
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("whatsapp: decode response: %w", err)
	}
	return nil
}

// parseErrorResponse unmarshals a Graph API error body into an *APIError.
func parseErrorResponse(status int, raw []byte, traceID string) error {
	var env struct {
		Error struct {
			Code       int    `json:"code"`
			Subcode    int    `json:"error_subcode"`
			Type       string `json:"type"`
			Message    string `json:"message"`
			FBTraceID  string `json:"fbtrace_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)
	tid := env.Error.FBTraceID
	if tid == "" {
		tid = traceID
	}
	return &APIError{
		Class:      classifyStatus(status, env.Error.Code, env.Error.Type),
		StatusCode: status,
		Code:       env.Error.Code,
		Subcode:    env.Error.Subcode,
		Type:       env.Error.Type,
		Message:    env.Error.Message,
		TraceID:    tid,
		Raw:        raw,
	}
}

// backoff returns a jittered exponential backoff duration for attempt N
// (1-indexed). Base is 250ms, cap 5s.
func backoff(attempt int) time.Duration {
	const base = 250 * time.Millisecond
	const cap = 5 * time.Second
	d := base << (attempt - 1)
	if d > cap {
		d = cap
	}
	return d + jitter(d/2)
}

// jitter returns a uniformly random duration in [0, max) using crypto/rand
// (avoids importing math/rand and its global state pitfalls).
func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	n := binary.BigEndian.Uint64(b[:])
	return time.Duration(n % uint64(max))
}

// contentLength is a small helper for the mock server to log payload sizes.
// Retained here so nothing outside the package needs to know Graph's shape.
func contentLength(h http.Header) int {
	s := h.Get("Content-Length")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
