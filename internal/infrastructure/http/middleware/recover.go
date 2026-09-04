package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover catches panics from downstream handlers, logs them with the stack,
// and returns an RFC 7807 500 response so we never expose internals.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rv := recover(); rv != nil {
					logger.Error("panic in handler",
						slog.String("request_id", RequestIDFrom(r.Context())),
						slog.Any("panic", rv),
						slog.String("stack", string(debug.Stack())),
					)
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"type":       "https://nudgeway.dev/errors/internal",
						"title":      "internal server error",
						"status":     http.StatusInternalServerError,
						"request_id": RequestIDFrom(r.Context()),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
