// Package health composes readiness probes for the /readyz endpoint.
// Each probe runs with a short timeout; any failure marks the process
// not-ready. Liveness (/healthz) is always 200 while the process is alive.
package health

import (
	"context"
	"errors"
	"time"
)

// Probe is one readiness check. Name is used in the response body; Check
// returns nil when the dependency is reachable.
type Probe struct {
	Name  string
	Check func(context.Context) error
}

// Result is the outcome of a single probe.
type Result struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
}

// Ready runs every probe with the supplied per-probe timeout and returns
// a combined result. The overall ok is true only when every probe passes.
func Ready(ctx context.Context, timeout time.Duration, probes []Probe) (ok bool, results []Result) {
	ok = true
	for _, p := range probes {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		err := p.Check(cctx)
		cancel()
		r := Result{Name: p.Name, OK: err == nil}
		if err != nil {
			r.Err = err.Error()
			ok = false
		}
		results = append(results, r)
	}
	return ok, results
}

// ErrProbeTimeout is returned by probes that hit the context deadline.
var ErrProbeTimeout = errors.New("probe timeout")
