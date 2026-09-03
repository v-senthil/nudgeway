package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecover_CatchesPanic(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Recover(logger)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "problem+json") {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestRecover_PassesThrough(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q", rec.Body.String())
	}
}
