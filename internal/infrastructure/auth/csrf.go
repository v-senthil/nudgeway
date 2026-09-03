package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// CSRFTokenBytes is the length of a raw CSRF token.
const CSRFTokenBytes = 32

// NewCSRFToken returns a base64url-encoded random token.
func NewCSRFToken() (string, error) {
	b := make([]byte, CSRFTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetCSRFCookie writes the CSRF cookie. Unlike the session cookie, this is
// readable by JavaScript so the frontend can echo it in the X-CSRF-Token header.
func SetCSRFCookie(w http.ResponseWriter, token string, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     defaultStr(opts.Name, "fullwa_csrf"),
		Value:    token,
		Path:     defaultStr(opts.Path, "/"),
		Domain:   opts.Domain,
		MaxAge:   int(opts.MaxAge.Seconds()),
		HttpOnly: false,
		Secure:   opts.Secure,
		SameSite: nonZeroSameSite(opts.SameSite),
	})
}

// VerifyCSRF returns true when the cookie and header carry the same token
// (double-submit cookie pattern). Constant-time comparison.
func VerifyCSRF(r *http.Request, cookieName string) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	header := r.Header.Get("X-CSRF-Token")
	if header == "" || c.Value == "" {
		return false
	}
	if len(header) != len(c.Value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(c.Value)) == 1
}
