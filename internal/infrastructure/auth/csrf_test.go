package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRF_RoundTrip(t *testing.T) {
	t.Parallel()
	tok, err := NewCSRFToken()
	if err != nil {
		t.Fatalf("NewCSRFToken: %v", err)
	}
	w := httptest.NewRecorder()
	SetCSRFCookie(w, tok, CookieOptions{Name: "nudgeway_csrf"})
	cookie := w.Result().Cookies()[0]
	if cookie.HttpOnly {
		t.Error("csrf cookie must NOT be HttpOnly (frontend must read it)")
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", tok)
	if !VerifyCSRF(req, "nudgeway_csrf") {
		t.Errorf("VerifyCSRF returned false for matching token")
	}
}

func TestCSRF_Mismatch(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "nudgeway_csrf", Value: "aaaa"})
	req.Header.Set("X-CSRF-Token", "bbbb")
	if VerifyCSRF(req, "nudgeway_csrf") {
		t.Errorf("VerifyCSRF returned true for mismatch")
	}
}

func TestCSRF_MissingHeaderOrCookie(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if VerifyCSRF(req, "nudgeway_csrf") {
		t.Errorf("VerifyCSRF returned true with no cookie")
	}
}
