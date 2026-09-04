package middleware

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// tokenBodyCap bounds the number of body bytes retained per direction
// for persistence. Anything larger is dropped from the recorded copy;
// the true wire size is still counted in the *_bytes fields.
const tokenBodyCap = 8 << 10 // 8 KiB

// TokenUsageEvent is the recording payload the middleware hands off to
// the TokenUsageRecorder. The middleware declares its own type (rather
// than importing the domain package) so infrastructure stays free of
// domain / application imports (see CLAUDE.md §4 dependency rule).
type TokenUsageEvent struct {
	// OrgID is the tenant of the bearer principal.
	OrgID string
	// UserID is the user tied to the bearer token.
	UserID string
	// TokenID is the api_tokens ULID that authenticated the request.
	TokenID string
	// RequestID stitches the row back to the request-scoped log lines.
	RequestID string
	// OccurredAt is the wall-clock time the request completed.
	OccurredAt time.Time
	// Method is the HTTP method.
	Method string
	// Path is the request URL path (no query string).
	Path string
	// StatusCode is the HTTP status the handler wrote.
	StatusCode int
	// LatencyMs is the handler duration in milliseconds.
	LatencyMs int
	// RemoteIP is the caller's IP as observed by the server.
	RemoteIP string
	// UserAgent is the caller's User-Agent header.
	UserAgent string
	// RequestBody is the captured request body (truncated to tokenBodyCap).
	RequestBody []byte
	// ResponseBody is the captured response body (truncated to tokenBodyCap).
	ResponseBody []byte
	// RequestBytes is the true wire size of the request body.
	RequestBytes int
	// ResponseBytes is the true wire size of the response body.
	ResponseBytes int
	// ErrorMessage carries a short failure summary for 4xx/5xx responses.
	ErrorMessage string
}

// TokenUsageRecorder is the small port the recording middleware calls
// into. Implemented in cmd/server by a thin adapter over the
// application-layer apitokenusage.Service.
type TokenUsageRecorder interface {
	// RecordUsage persists one event. Implementations MUST NOT block
	// the caller — the middleware spawns a detached goroutine for
	// every call so a slow persist never delays the HTTP response.
	RecordUsage(ctx context.Context, e TokenUsageEvent)
}

// TokenRecording wraps every downstream handler with a per-request
// recorder. When the request is bearer-authenticated (IsBearer(ctx) is
// true), a detached goroutine sends a TokenUsageEvent to recorder after
// the handler returns.
//
// The middleware reads + restores the request body so downstream
// handlers still see the full payload. Response bytes are captured
// through a wrapped ResponseWriter. Both directions are capped at
// tokenBodyCap; the true wire sizes are always preserved.
//
// If recorder is nil the middleware degrades to a pass-through — no
// body capture, no goroutine — so slim deploys can skip usage tracking
// entirely by leaving the dependency nil.
func TokenRecording(recorder TokenUsageRecorder, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if recorder == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read + replace the request body so the handler sees it
			// unchanged. Cap capture at tokenBodyCap; the true size is
			// the length of the original read.
			var (
				reqCaptured []byte
				reqBytes    int
			)
			if r.Body != nil && r.Body != http.NoBody {
				raw, err := io.ReadAll(r.Body)
				_ = r.Body.Close()
				if err == nil {
					reqBytes = len(raw)
					if len(raw) > tokenBodyCap {
						reqCaptured = append([]byte(nil), raw[:tokenBodyCap]...)
					} else {
						reqCaptured = append([]byte(nil), raw...)
					}
					r.Body = io.NopCloser(bytes.NewReader(raw))
				} else {
					// If reading failed, log and continue without capture
					// — the handler will see EOF and surface its own error.
					if logger != nil {
						logger.Warn("token_recording: request body read failed",
							slog.String("request_id", RequestIDFrom(r.Context())),
							slog.Any("err", err),
						)
					}
				}
			}

			start := time.Now()
			cw := &captureWriter{ResponseWriter: w, status: http.StatusOK, cap: tokenBodyCap}
			next.ServeHTTP(cw, r)

			if !IsBearer(r.Context()) {
				return
			}
			pr, ok := PrincipalFrom(r.Context())
			if !ok {
				return
			}
			ev := TokenUsageEvent{
				OrgID:         pr.OrgID,
				UserID:        pr.UserID,
				TokenID:       APITokenIDFrom(r.Context()),
				RequestID:     RequestIDFrom(r.Context()),
				OccurredAt:    time.Now().UTC(),
				Method:        r.Method,
				Path:          r.URL.Path,
				StatusCode:    cw.status,
				LatencyMs:     int(time.Since(start).Milliseconds()),
				RemoteIP:      remoteIP(r),
				UserAgent:     r.Header.Get("User-Agent"),
				RequestBody:   reqCaptured,
				ResponseBody:  cw.captured(),
				RequestBytes:  reqBytes,
				ResponseBytes: cw.total,
			}
			if cw.status >= 400 {
				ev.ErrorMessage = extractErrorMessage(cw.captured())
			}
			// Detached goroutine: use a fresh context so shutdown of
			// the request context doesn't cancel the persist. The
			// recording path is fire-and-forget by design.
			go recorder.RecordUsage(context.Background(), ev)
		})
	}
}

// captureWriter wraps an http.ResponseWriter to capture the status code
// and the first `cap` bytes of the response body while still forwarding
// every write to the underlying writer.
type captureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
	cap    int
	total  int
}

// WriteHeader records the status and forwards.
func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

// Write forwards to the underlying writer and captures a bounded copy.
func (c *captureWriter) Write(b []byte) (int, error) {
	c.total += len(b)
	if c.buf.Len() < c.cap {
		room := c.cap - c.buf.Len()
		if room > len(b) {
			room = len(b)
		}
		c.buf.Write(b[:room])
	}
	return c.ResponseWriter.Write(b)
}

// captured returns the buffered slice.
func (c *captureWriter) captured() []byte {
	if c.buf.Len() == 0 {
		return nil
	}
	out := make([]byte, c.buf.Len())
	copy(out, c.buf.Bytes())
	return out
}

// Hijack forwards to the underlying Hijacker so WS upgrades still work
// through the recording middleware. If the underlying writer does not
// support Hijacker (rare), an error surfaces to the caller.
func (c *captureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := c.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("captureWriter: upstream does not implement http.Hijacker")
	}
	return h.Hijack()
}

// Flush forwards to the underlying Flusher when present so SSE / chunked
// responses still stream.
func (c *captureWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// remoteIP returns the caller's IP address, preferring the first hop
// of X-Forwarded-For when present. Bounded at 45 chars (fits IPv6).
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First hop; trim any surrounding whitespace.
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		xff = strings.TrimSpace(xff)
		if xff != "" {
			if len(xff) > 45 {
				xff = xff[:45]
			}
			return xff
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(host) > 45 {
		host = host[:45]
	}
	return host
}

// extractErrorMessage does a best-effort scrape of the response body
// for a "detail" field (RFC 7807) so operators see a useful summary in
// the usage log without opening the raw body. Falls back to a byte-
// bounded copy of the buffer.
func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	// Cheap scan: look for `"detail":"..."` — full JSON unmarshal would
	// churn on every 4xx and this middleware runs on the hot path.
	const needle = `"detail":"`
	idx := bytes.Index(body, []byte(needle))
	if idx < 0 {
		if len(body) > 200 {
			return string(body[:200])
		}
		return string(body)
	}
	start := idx + len(needle)
	end := bytes.IndexByte(body[start:], '"')
	if end < 0 {
		return ""
	}
	msg := string(body[start : start+end])
	if len(msg) > 512 {
		msg = msg[:512]
	}
	return msg
}
