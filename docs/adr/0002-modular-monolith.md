# ADR 0002 — Modular monolith, not microservices

Status: Accepted (2026-09-03)

## Context

Multiple "provider-agnostic communication platform" projects have died by premature service-splitting. We want strict module boundaries but a single deployable.

## Decision

- One binary hosts REST + WS + workers + scheduler + webhook ingress.
- Internal packages are strictly layered via `.go-arch-lint.yml`:
  `domain → application → ports; infrastructure/providers → ports; cmd → wires`.
- We only extract a service when we have measured, unavoidable scaling or isolation reasons — not because of team topology or aesthetics.

## Consequences

- Zero cross-service networking overhead.
- Deployment story is `scp binary && restart`.
- Boundary discipline is enforced by tooling, not culture — if arch-lint is green, the boundary holds.
- When a service does need to split out, the port interfaces are the natural extraction seam.
