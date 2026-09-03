package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// ctxKey is the private context-key type — protects against key collisions
// with other packages.
type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxUserID
	ctxOrgID
	ctxPermissions
)

// HeaderRequestID is the response header carrying the request ID back to callers.
const HeaderRequestID = "X-Request-ID"

// RequestID assigns a random request ID unless the incoming request already
// carries one, stores it on the context, and echoes it on the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom returns the request ID stashed by RequestID, or "".
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID).(string)
	return v
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "req-fallback"
	}
	return "req_" + hex.EncodeToString(b)
}
