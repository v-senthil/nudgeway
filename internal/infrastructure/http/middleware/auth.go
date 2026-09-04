package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/auth"
)

// Principal is the authenticated caller resolved from the session cookie.
type Principal struct {
	UserID      string
	OrgID       string
	Permissions rbac.PermissionSet
}

// PrincipalFrom returns the current principal, or false if unauthenticated.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	uid, _ := ctx.Value(ctxUserID).(string)
	oid, _ := ctx.Value(ctxOrgID).(string)
	perms, _ := ctx.Value(ctxPermissions).(rbac.PermissionSet)
	if uid == "" || oid == "" {
		return Principal{}, false
	}
	return Principal{UserID: uid, OrgID: oid, Permissions: perms}, true
}

// OrgIDFrom returns the caller's organization ID or "".
func OrgIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxOrgID).(string)
	return v
}

// PermissionResolver returns the permission set granted to a user in an org.
// Implemented by application/rbac in later phases; Phase 0 uses a no-op that
// grants everything to keep the walking skeleton demoable.
type PermissionResolver interface {
	Resolve(ctx context.Context, orgID, userID string) (rbac.PermissionSet, error)
}

// SessionAuth wires the session cookie to a Principal on the request context.
// On unauthenticated requests it does NOT reject — RequireAuth does that
// downstream. This lets public endpoints share the same middleware chain.
func SessionAuth(
	store auth.SessionStore,
	cookieName string,
	slideEvery time.Duration,
	resolver PermissionResolver,
	logger *slog.Logger,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			sess, err := store.Get(r.Context(), auth.SessionID(c.Value))
			if err != nil {
				if !errors.Is(err, auth.ErrSessionNotFound) {
					logger.Warn("session lookup failed",
						slog.String("request_id", RequestIDFrom(r.Context())),
						slog.Any("err", err),
					)
				}
				next.ServeHTTP(w, r)
				return
			}
			// slide the expiry.
			if time.Since(sess.LastSeenAt) > slideEvery {
				_ = store.Touch(r.Context(), sess.ID, time.Now())
			}
			perms, err := resolver.Resolve(r.Context(), sess.OrgID, sess.UserID)
			if err != nil {
				logger.Warn("permission resolve failed",
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.Any("err", err),
				)
				perms = rbac.PermissionSet{}
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxUserID, sess.UserID)
			ctx = context.WithValue(ctx, ctxOrgID, sess.OrgID)
			ctx = context.WithValue(ctx, ctxPermissions, perms)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth rejects unauthenticated requests with 401.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PrincipalFrom(r.Context()); !ok {
			writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission gates a handler on a specific permission key.
func RequirePermission(p rbac.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pr, ok := PrincipalFrom(r.Context())
			if !ok {
				writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
				return
			}
			if !pr.Permissions.Has(p) {
				writeProblem(w, r, http.StatusForbidden, "forbidden", "missing permission "+string(p))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireCSRF enforces the double-submit cookie for state-changing methods.
func RequireCSRF(cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if !auth.VerifyCSRF(r, cookieName) {
				writeProblem(w, r, http.StatusForbidden, "csrf_failed", "csrf token mismatch")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "https://nudgeway.dev/errors/" + title,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"request_id": RequestIDFrom(r.Context()),
	})
}
