# ADR 0001 — Language, runtime, and dependency posture

Status: Accepted (2026-09-03)

## Context

fullWA needs a language that supports high-concurrency networking, cheap goroutines, first-class WebSocket + webhook workloads, a good single-binary story, and a small deployment footprint. It also needs to be pleasant to write with AI assistance and to review.

## Decision

- **Go** (latest stable) for the backend.
- **Minimal dependencies.** No ORM. No DI framework. No microservice framework. Standard library first. Third-party libs only where the stdlib is genuinely absent.
- **TypeScript strict** for the frontend, built with Vite, embedded via `//go:embed`.

## Consequences

- Fast compile, fast tests, small binaries.
- We hand-write a bit more infrastructure (config loader, event bus, retry) than a framework-heavy stack. Acceptable tradeoff — this code is small, testable, and doesn't age poorly.
- New Go engineers can read the whole tree in an afternoon.
