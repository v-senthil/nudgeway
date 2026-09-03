package workers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Runner is the tiny surface a worker exposes to Pool: a blocking Run
// that returns when ctx is cancelled or an unrecoverable error occurs.
type Runner interface {
	// Run blocks until ctx is done or the runner errors out.
	Run(ctx context.Context) error
}

// RunnerFunc adapts a plain function to the Runner interface.
type RunnerFunc func(ctx context.Context) error

// Run implements Runner.
func (f RunnerFunc) Run(ctx context.Context) error { return f(ctx) }

// Pool spawns a fixed number of goroutines, each running the provided
// Runner. It is the ONLY sanctioned place in the tree to launch background
// goroutines (see CLAUDE.md §11 — anti-patterns).
//
// Concurrency caps the goroutine count; values <=0 fall back to 1 so
// misconfiguration never disables the worker entirely. Name is used for
// logging + telemetry attribution.
type Pool struct {
	// Name identifies the pool in logs / metrics ("webhook", "message.send", ...).
	Name string
	// Concurrency is the number of goroutines to spawn. Bounded — no
	// unbounded fan-out is permitted anywhere else in the codebase.
	Concurrency int
	// Runner is the per-goroutine unit of work.
	Runner Runner
	// Log receives lifecycle events. Required.
	Log *slog.Logger
}

// Run spawns Concurrency goroutines and blocks until ctx is cancelled and
// every goroutine has returned. The first non-nil error from any runner
// is returned; other errors are logged.
func (p *Pool) Run(ctx context.Context) error {
	if p.Runner == nil {
		return fmt.Errorf("workers: Pool.Run: runner is required")
	}
	if p.Log == nil {
		return fmt.Errorf("workers: Pool.Run: logger is required")
	}
	n := p.Concurrency
	if n <= 0 {
		n = 1
	}

	p.Log.InfoContext(ctx, "worker pool starting",
		slog.String("pool", p.Name),
		slog.Int("concurrency", n),
	)

	var (
		wg       sync.WaitGroup
		errMu    sync.Mutex
		firstErr error
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := p.Runner.Run(ctx); err != nil {
				p.Log.ErrorContext(ctx, "worker exited with error",
					slog.String("pool", p.Name),
					slog.Int("worker_index", idx),
					slog.Any("err", err),
				)
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	p.Log.InfoContext(ctx, "worker pool stopped",
		slog.String("pool", p.Name),
		slog.Int("concurrency", n),
	)
	return firstErr
}
