package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// statusRecorder wraps [http.ResponseWriter] to capture the response
// status code so the middleware can label the histogram/counter. It
// defaults to 200 (per net/http's implicit WriteHeader behaviour).
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

// WriteHeader records the status code before delegating to the wrapped
// writer.
func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write ensures a status is captured for handlers that skip WriteHeader.
func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.status = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}

// HTTPMiddleware returns a middleware that records HTTP request counts
// and latencies onto the fullWA metrics registry. The `route` argument is
// the route template (not the raw URL) — pass it in from the router so
// paths remain low cardinality. When empty, `r.URL.Path` is used as a
// last-resort label; prefer to mount this per-route.
func (m *Metrics) HTTPMiddleware(route string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			path := route
			if path == "" {
				path = r.URL.Path
			}
			status := strconv.Itoa(rec.status)
			m.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			m.HTTPRequestDurationSeconds.
				WithLabelValues(r.Method, path, status).
				Observe(time.Since(start).Seconds())
		})
	}
}
