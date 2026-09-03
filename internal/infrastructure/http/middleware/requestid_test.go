package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID_SetsHeaderAndContext(t *testing.T) {
	t.Parallel()
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.HasPrefix(seen, "req_") {
		t.Errorf("ctx id = %q", seen)
	}
	if rec.Header().Get(HeaderRequestID) != seen {
		t.Errorf("response header %q != ctx %q", rec.Header().Get(HeaderRequestID), seen)
	}
}

func TestRequestID_HonoursIncomingHeader(t *testing.T) {
	t.Parallel()
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "req_incoming")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(HeaderRequestID); got != "req_incoming" {
		t.Errorf("preserved incoming id? got %q", got)
	}
}
