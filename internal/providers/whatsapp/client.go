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
	if err := c.doJSON(ctx, "send_message", http.MethodPost, url, body, &resp); err != nil {
		return metaSendResponse{}, err
	}
	return resp, nil
}

// markAsRead POSTs a mark-as-read status update to /messages. Meta returns
// {"success":true} on success; the body is not otherwise interesting to us.
//
// Reference: ~/Documents/whatsapp_doc_tracker/docs/messages/mark-message-as-read.md.
func (c *client) markAsRead(ctx context.Context, providerMessageID string) error {
	url := fmt.Sprintf("%s/%s/%s/messages", c.cfg.baseURL(), c.cfg.version(), c.cfg.PhoneNumberID)
	body, err := json.Marshal(map[string]string{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        providerMessageID,
	})
	if err != nil {
		return fmt.Errorf("whatsapp: encode mark-as-read: %w", err)
	}
	var resp map[string]any
	if err := c.doJSON(ctx, "mark_as_read", http.MethodPost, url, body, &resp); err != nil {
		return err
	}
	return nil
}

// getMediaURL resolves a Meta media ID to a short-lived download URL.
// See /messages/audio-messages.md, /image-messages.md, etc. — the pattern
// is GET /<media_id> returning a JSON object with a `url` field.
func (c *client) getMediaURL(ctx context.Context, mediaID string) (mediaLookupResponse, error) {
	url := fmt.Sprintf("%s/%s/%s", c.cfg.baseURL(), c.cfg.version(), mediaID)
	var resp mediaLookupResponse
	if err := c.doJSON(ctx, "get_media_url", http.MethodGet, url, nil, &resp); err != nil {
		return mediaLookupResponse{}, err
	}
	return resp, nil
}

// downloadMedia GETs a Meta-provided media URL with the Bearer token. Meta
// returns the raw bytes; caller is responsible for closing the returned body.
//
// The tracer sees an event with status_code + latency but NEVER the response
// body — the raw bytes are the media itself, potentially large binary blobs,
// and not useful for debugging.
func (c *client) downloadMedia(ctx context.Context, url string) (io.ReadCloser, string, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.trace(ctx, TraceEvent{
			Operation: "download_media", Method: http.MethodGet, URL: url,
			LatencyMs: msSince(start), ErrClass: "permanent",
			ErrMessage: err.Error(),
		})
		return nil, "", fmt.Errorf("whatsapp: build media download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	res, err := c.cfg.httpClient().Do(req)
	if err != nil {
		c.trace(ctx, TraceEvent{
			Operation: "download_media", Method: http.MethodGet, URL: url,
			LatencyMs: msSince(start), ErrClass: string(ClassTransient),
			ErrMessage: err.Error(),
		})
		return nil, "", fmt.Errorf("whatsapp: media download: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		class := classifyStatus(res.StatusCode, 0, "")
		c.trace(ctx, TraceEvent{
			Operation: "download_media", Method: http.MethodGet, URL: url,
			StatusCode: res.StatusCode, LatencyMs: msSince(start),
			ErrClass:   string(class),
			ErrMessage: "media download failed",
			TraceID:    res.Header.Get("x-fb-trace-id"),
		})
		return nil, "", &APIError{
			Class:      class,
			StatusCode: res.StatusCode,
			Message:    "media download failed",
			Raw:        raw,
		}
	}
	// Successful media download: log the status_code + latency but do
	// NOT read or persist the body — the caller streams it straight
	// through to the attachment store.
	c.trace(ctx, TraceEvent{
		Operation: "download_media", Method: http.MethodGet, URL: url,
		StatusCode: res.StatusCode, LatencyMs: msSince(start),
		TraceID: res.Header.Get("x-fb-trace-id"),
	})
	return res.Body, res.Header.Get("Content-Type"), nil
}

// listTemplates returns templates registered on the WABA.
func (c *client) listTemplates(ctx context.Context) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s/message_templates", c.cfg.baseURL(), c.cfg.version(), c.cfg.WABAID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, "list_templates", http.MethodGet, url, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// createTemplate submits a new template for Meta review.
func (c *client) createTemplate(ctx context.Context, body []byte) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s/message_templates", c.cfg.baseURL(), c.cfg.version(), c.cfg.WABAID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, "create_template", http.MethodPost, url, body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// getTemplateStatus fetches a single template by id.
func (c *client) getTemplateStatus(ctx context.Context, templateID string) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s", c.cfg.baseURL(), c.cfg.version(), templateID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, "get_template_status", http.MethodGet, url, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// doJSON is the retrying JSON request core. `op` names the adapter
// operation for the tracer / log line — every callsite must supply one so
// the operator UI can filter on it.
func (c *client) doJSON(ctx context.Context, op, method, url string, body []byte, out any) error {
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
		err := c.doOnce(ctx, op, method, url, body, out)
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

// doOnce performs a single request/response round-trip and emits exactly
// one tracer event to c.cfg.Tracer. On retry, each attempt emits its own
// event so the operator UI shows the full attempt history.
func (c *client) doOnce(ctx context.Context, op, method, url string, body []byte, out any) error {
	start := time.Now()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		c.trace(ctx, TraceEvent{
			Operation: op, Method: method, URL: url,
			RequestBody: body, LatencyMs: msSince(start),
			ErrClass: "permanent", ErrMessage: err.Error(),
		})
		return fmt.Errorf("whatsapp: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.cfg.httpClient().Do(req)
	if err != nil {
		c.trace(ctx, TraceEvent{
			Operation: op, Method: method, URL: url,
			RequestBody: body, LatencyMs: msSince(start),
			ErrClass: string(ClassTransient), ErrMessage: err.Error(),
		})
		return &APIError{Class: ClassTransient, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		c.trace(ctx, TraceEvent{
			Operation: op, Method: method, URL: url,
			RequestBody: body, StatusCode: res.StatusCode,
			LatencyMs: msSince(start),
			ErrClass:  "transient", ErrMessage: err.Error(),
			TraceID: res.Header.Get("x-fb-trace-id"),
		})
		return fmt.Errorf("whatsapp: read body: %w", err)
	}
	if res.StatusCode >= 400 {
		// Diagnostic: dump exact URL + payload + Meta response so operators
		// can debug 100/33 GraphMethodException without adding print
		// statements. Truncate secrets never leave this scope.
		safeBody := string(body)
		if len(safeBody) > 2048 {
			safeBody = safeBody[:2048] + "…"
		}
		fmt.Fprintf(io.Discard, "") // keep import
		// Use standard fmt to stderr — slog isn't wired inside the adapter
		// package. Line-based so it's easy to grep.
		fmt.Fprintf(getDebugSink(),
			"[whatsapp] %s %s\n  request:  %s\n  response: %d %s\n",
			method, url, safeBody, res.StatusCode, string(raw),
		)
		apiErr := parseErrorResponse(res.StatusCode, raw, res.Header.Get("x-fb-trace-id"))
		errClass := ""
		errMsg := ""
		traceID := res.Header.Get("x-fb-trace-id")
		if a := AsAPIError(apiErr); a != nil {
			errClass = string(a.Class)
			errMsg = a.Message
			if a.TraceID != "" {
				traceID = a.TraceID
			}
		}
		c.trace(ctx, TraceEvent{
			Operation: op, Method: method, URL: url,
			RequestBody: body, ResponseBody: raw,
			StatusCode: res.StatusCode, LatencyMs: msSince(start),
			ErrClass: errClass, ErrMessage: errMsg,
			TraceID: traceID,
		})
		return apiErr
	}
	// Success: emit the event before decoding so a JSON-decode error
	// downstream still has an entry attributing the successful HTTP round
	// trip.
	c.trace(ctx, TraceEvent{
		Operation: op, Method: method, URL: url,
		RequestBody: body, ResponseBody: raw,
		StatusCode: res.StatusCode, LatencyMs: msSince(start),
		TraceID: res.Header.Get("x-fb-trace-id"),
	})
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

// trace hands the event to the configured tracer, stamping the IntegrationID
// / OrgID from Config so downstream persistence knows the tenant. A panic in
// the tracer is recovered so a broken bookkeeping wire-up cannot break the
// outbound send path.
func (c *client) trace(ctx context.Context, evt TraceEvent) {
	if evt.IntegrationID == "" {
		evt.IntegrationID = c.cfg.IntegrationID
	}
	if evt.OrgID == "" {
		evt.OrgID = c.cfg.OrgID
	}
	defer func() { _ = recover() }()
	c.cfg.tracer().OnCall(ctx, evt)
}

// msSince returns the wall-clock elapsed since start in milliseconds.
func msSince(start time.Time) int64 {
	return time.Since(start).Milliseconds()
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
