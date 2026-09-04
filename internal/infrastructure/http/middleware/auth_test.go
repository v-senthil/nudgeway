package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/v-senthil/nudgeway/internal/domain/rbac"
	"github.com/v-senthil/nudgeway/internal/infrastructure/auth"
)

type stubResolver struct{ perms rbac.PermissionSet }

func (s stubResolver) Resolve(_ context.Context, _, _ string) (rbac.PermissionSet, error) {
	return s.perms, nil
}

func TestSessionAuth_AttachesPrincipal(t *testing.T) {
	t.Parallel()
	store := auth.NewMemStore()
	id, _ := auth.NewSessionID()
	_ = store.Create(context.Background(), auth.Session{
		ID: id, UserID: "u1", OrgID: "o1", ExpiresAt: time.Now().Add(time.Hour),
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	perms := rbac.NewSet(rbac.PermContactsRead)
	mw := SessionAuth(store, "s", time.Minute, stubResolver{perms}, logger)

	var seen bool
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		pr, ok := PrincipalFrom(r.Context())
		if !ok {
			t.Errorf("principal not set")
			return
		}
		if pr.OrgID != "o1" || !pr.Permissions.Has(rbac.PermContactsRead) {
			t.Errorf("principal = %+v", pr)
		}
		seen = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "s", Value: string(id)})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !seen {
		t.Errorf("handler not called")
	}
}

func TestRequireAuth_Rejects(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	RequireAuth(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("should not reach handler")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestRequirePermission_ForbidsMissing(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxUserID, "u")
	ctx = context.WithValue(ctx, ctxOrgID, "o")
	ctx = context.WithValue(ctx, ctxPermissions, rbac.NewSet())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	RequirePermission(rbac.PermMessagesSend)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("should not reach handler")
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("code = %d, want 403", rec.Code)
	}
}

func TestRequirePermission_AllowsGranted(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxUserID, "u")
	ctx = context.WithValue(ctx, ctxOrgID, "o")
	ctx = context.WithValue(ctx, ctxPermissions, rbac.NewSet(rbac.PermMessagesSend))
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	called := false
	RequirePermission(rbac.PermMessagesSend)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	if !called {
		t.Errorf("handler not called")
	}
}

func TestRequireCSRF_AllowsGetBlocksBadPost(t *testing.T) {
	t.Parallel()
	mw := RequireCSRF("csrf")
	// GET passes.
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET code = %d", rec.Code)
	}
	// POST without header is rejected.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "tok"})
	mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { t.Error("should not reach") })).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("bad POST code = %d, want 403", rec.Code)
	}
}
