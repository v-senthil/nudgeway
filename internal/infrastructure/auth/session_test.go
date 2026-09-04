package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSessionID_UniqueAndURLSafe(t *testing.T) {
	t.Parallel()
	seen := map[SessionID]bool{}
	for i := 0; i < 100; i++ {
		id, err := NewSessionID()
		if err != nil {
			t.Fatalf("NewSessionID: %v", err)
		}
		if len(id) < 40 {
			t.Errorf("id too short: %s", id)
		}
		if seen[id] {
			t.Errorf("duplicate id: %s", id)
		}
		seen[id] = true
	}
}

func TestSetSessionCookie_FlagsAreSafe(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	SetSessionCookie(w, SessionID("abc"), CookieOptions{
		Name: "nudgeway_session", MaxAge: time.Hour, Secure: true,
	})
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	c := cookies[0]
	if !c.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if !c.Secure {
		t.Error("cookie must be Secure when opts.Secure=true")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Value != "abc" {
		t.Errorf("Value = %q", c.Value)
	}
}

func TestClearSessionCookie_ExpiresImmediately(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ClearSessionCookie(w, CookieOptions{Name: "nudgeway_session"})
	c := w.Result().Cookies()[0]
	if c.MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", c.MaxAge)
	}
}
