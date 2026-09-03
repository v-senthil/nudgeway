package whatsapp

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorClass classifies a Graph API failure so callers can decide whether to
// retry, back off, refresh credentials, or fail permanently.
type ErrorClass string

// ErrorClass values.
const (
	ClassTransient   ErrorClass = "transient"    // 5xx, network — retry with backoff
	ClassRateLimited ErrorClass = "rate_limited" // 429 — retry after longer delay
	ClassAuth        ErrorClass = "auth"         // 401/403 or Meta OAuth errors — refresh token
	ClassPermanent   ErrorClass = "permanent"    // 4xx business errors — do not retry
	ClassUnknown     ErrorClass = "unknown"
)

// APIError is the canonical error returned by the client.
type APIError struct {
	Class      ErrorClass
	StatusCode int
	Code       int    // Meta error code
	Subcode    int    // Meta error subcode
	Type       string // Meta error type (OAuthException, GraphMethodException, …)
	Message    string
	TraceID    string // fbtrace_id
	Raw        []byte
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("whatsapp: %s (status=%d code=%d subcode=%d type=%s trace=%s): %s",
		e.Class, e.StatusCode, e.Code, e.Subcode, e.Type, e.TraceID, e.Message)
}

// Retryable reports whether the caller should retry the request.
func (e *APIError) Retryable() bool {
	return e.Class == ClassTransient || e.Class == ClassRateLimited
}

// classifyStatus turns an HTTP status + Meta error body into an ErrorClass.
func classifyStatus(status int, code int, mtype string) ErrorClass {
	switch {
	case status == http.StatusTooManyRequests:
		return ClassRateLimited
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ClassAuth
	case mtype == "OAuthException":
		return ClassAuth
	case status >= 500 && status <= 599:
		return ClassTransient
	case code == 4 || code == 80007 || code == 130429: // rate limit codes
		return ClassRateLimited
	case status >= 400 && status <= 499:
		return ClassPermanent
	default:
		return ClassUnknown
	}
}

// AsAPIError unwraps err into *APIError, or nil.
func AsAPIError(err error) *APIError {
	var a *APIError
	if errors.As(err, &a) {
		return a
	}
	return nil
}
