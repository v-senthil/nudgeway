# fullWA runbook

Operational procedures for the fullWA platform. Expands per phase — Phase 0 lands the skeleton.

## Local dev bring-up

1. Start MySQL, Redis, HBase (all natively — `brew services start mysql redis`; HBase via `start-hbase.sh`).
2. `make check-infra` — verifies reachability.
3. `make migrate up` — applies schema.
4. `make dev` — runs backend + frontend.

## Health probes

- `GET /healthz` — 200 if the process is alive.
- `GET /readyz` — 200 only if MySQL, Redis, and HBase are reachable (Phase 0 Task 2+). 503 otherwise; caller should retry.

## Runbook items (added per feature)

- (Phase 0 Task 2) Rotating the credential KEK.
- (Phase 1) Replaying a stuck webhook event.
- (Phase 1) Draining and restarting a worker pool.
- (Phase 2) Recovering a `PENDING_SYNC` ticket after a provider outage.
- (Phase 3) Pausing / resuming a campaign.
- (Phase 4) Backup + restore procedures (MySQL logical dump + HBase snapshot).
