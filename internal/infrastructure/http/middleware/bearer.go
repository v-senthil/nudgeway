package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/v-senthil/nudgeway/internal/domain/rbac"
)

// BearerPrincipal is the identity resolved from a bearer API token —
// declared locally to preserve the layered dependency rule
// (infrastructure must not import application).
type BearerPrincipal struct {
	OrgID   string
	UserID  string
	TokenID string
}

// ErrInvalidBearer is returned by BearerVerifier when a plaintext token
// does not resolve to an active principal (bad format, unknown prefix,
// wrong secret, revoked, expired). Any other error is treated as a
// transient failure and logged.
var ErrInvalidBearer = errors.New("invalid bearer token")

// BearerVerifier resolves an opaque bearer token into a BearerPrincipal.
// Implemented by a thin adapter over application/apitoken.Service wired
// in cmd/server.
type BearerVerifier interface {
	// VerifyBearer returns the principal for a valid token, or
	// ErrInvalidBearer when the caller supplied an invalid one.
	VerifyBearer(ctx context.Context, plaintext string) (BearerPrincipal, error)
}

// BearerAuth inspects the Authorization header for a Bearer token. On
// success it injects the resolved principal (org, user, api-token id) plus
// the permission set for that (org, user) onto the request context, and
// marks the request as bearer-authenticated so downstream CSRF gates
// short-circuit.
//
// The middleware NEVER rejects unauthenticated or invalid-token requests
// — it simply chains to next. This lets it compose freely with the
// existing SessionAuth middleware: bearer wins when present, session
// takes over when absent, and RequireAuth downstream produces the 401.
func BearerAuth(
	verifier BearerVerifier,
	resolver PermissionResolver,
	logger *slog.Logger,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}
			const bearerScheme = "bearer "
			if len(raw) < len(bearerScheme) || !strings.EqualFold(raw[:len(bearerScheme)], bearerScheme) {
				next.ServeHTTP(w, r)
				return
			}
			token := strings.TrimSpace(raw[len(bearerScheme):])
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			principal, err := verifier.VerifyBearer(r.Context(), token)
			if err != nil {
				if !errors.Is(err, ErrInvalidBearer) && logger != nil {
					logger.Warn("bearer verify failed",
						slog.String("request_id", RequestIDFrom(r.Context())),
						slog.Any("err", err),
					)
				}
				next.ServeHTTP(w, r)
				return
			}
			perms, err := resolver.Resolve(r.Context(), principal.OrgID, principal.UserID)
			if err != nil {
				if logger != nil {
					logger.Warn("bearer permission resolve failed",
						slog.String("request_id", RequestIDFrom(r.Context())),
						slog.Any("err", err),
					)
				}
				perms = rbac.PermissionSet{}
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxUserID, principal.UserID)
			ctx = context.WithValue(ctx, ctxOrgID, principal.OrgID)
			ctx = context.WithValue(ctx, ctxPermissions, perms)
			ctx = context.WithValue(ctx, ctxBearer, true)
			ctx = context.WithValue(ctx, ctxAPITokenID, principal.TokenID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
