package v1

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
)

// providerErrRe matches the wrapped error string emitted by
// providers/whatsapp.APIError.Error() so the REST layer can surface a
// clean, human-readable detail message + structured extras (Meta error
// code, trace id) in the RFC 7807 problem+json body — without importing
// any provider package (dependency-rule compliant).
//
// Pattern targets the whatsapp adapter's format:
//
//	whatsapp: <class> (status=<n> code=<n> subcode=<n> type=<T> trace=<id>): <message>
//
// The wrapping from application services ("create group: whatsapp: ...")
// does not interfere — regex anchors on the fields, not the prefix.
var providerErrRe = regexp.MustCompile(
	`(?s)status=(\d+)\s+code=(\d+)\s+subcode=(\d+)\s+type=(\S+)\s+trace=(\S+?)\):\s*(.+)$`,
)

// providerErrorDetail extracts the friendly detail message + provider
// extras from err. When err does not look like a provider API error the
// returned message is err.Error() and extras is nil.
func providerErrorDetail(err error) (string, map[string]any) {
	if err == nil {
		return "", nil
	}
	m := providerErrRe.FindStringSubmatch(err.Error())
	if m == nil {
		return err.Error(), nil
	}
	extras := map[string]any{
		"provider_status_code": m[1],
		"provider_error_code":  m[2],
	}
	if m[3] != "0" {
		extras["provider_error_subcode"] = m[3]
	}
	if m[4] != "" {
		extras["provider_error_type"] = m[4]
	}
	if m[5] != "" {
		extras["provider_trace_id"] = m[5]
	}
	return m[6], extras
}

// writeProblemExtras is writeProblem with additional top-level fields
// merged into the JSON body. Used for provider errors so the operator
// UI can render Meta's error code / trace id alongside the message.
//
// Reserved keys ("type", "title", "status", "detail", "request_id") in
// extras are dropped — the standard fields always win.
func writeProblemExtras(w http.ResponseWriter, r *http.Request, status int, title, detail string, extras map[string]any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	body := map[string]any{
		"type":       "https://fullwa.dev/errors/" + title,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"request_id": middleware.RequestIDFrom(r.Context()),
	}
	for k, v := range extras {
		switch k {
		case "type", "title", "status", "detail", "request_id":
			continue
		}
		body[k] = v
	}
	_ = json.NewEncoder(w).Encode(body)
}

// writeProviderProblem is the common exit for handlers that hit a
// provider (Meta / Zoho / OpenAI / ...). It reduces the wrapped error to
// its human-readable message + the provider's diagnostic extras. Falls
// back to plain writeProblem when the error isn't a provider error.
func writeProviderProblem(w http.ResponseWriter, r *http.Request, status int, title string, err error) {
	detail, extras := providerErrorDetail(err)
	if extras == nil {
		writeProblem(w, r, status, title, detail)
		return
	}
	writeProblemExtras(w, r, status, title, detail, extras)
}
