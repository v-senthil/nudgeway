# fullWA

Open-source, multi-tenant customer-communication platform. Initially centred on the WhatsApp Business Cloud API, architected from day one for any communication / ticketing / CRM / AI / bot / calling provider to plug in as an adapter.

- **Backend:** Go modular monolith, single binary.
- **Storage:** MySQL (source of truth) + Redis (queues/cache/coordination) + HBase (high-volume message/event data).
- **Frontend:** React + TypeScript + Vite + TanStack Router/Query + Tailwind — built and embedded into the Go binary.
- **Real-time:** WebSockets for live inbox updates.
- **Async-first:** every external API call runs off the request path.
- **Provider-agnostic core:** WhatsApp / Zoho Desk / OpenAI / Anthropic / Zoho Zia / Dialogflow etc. all live behind port interfaces in `internal/providers/`.

## Quick start

**Prereqs (local, native — no Docker):** MySQL 8+, Redis 7+, HBase 2+ running on your machine.

```bash
cp config/example.yaml config/local.yaml   # then edit to point at your local services
make check-infra                            # verifies MySQL / Redis / HBase are reachable
make migrate up                             # runs schema migrations
make dev                                    # Go server + Vite frontend
```

Open http://localhost:8080.

## Repo layout

See [`docs/architecture.md`](docs/architecture.md) for the full picture. Key directories:

| Path | Purpose |
|------|---------|
| `cmd/server/` | Single-binary entrypoint — HTTP + WebSocket + workers + scheduler. |
| `cmd/cli/` | Admin CLI (`migrate`, `seed`, `tenant create`, `user invite`, `key rotate`). |
| `internal/domain/` | Pure Go domain model. Zero infra imports. Zero provider imports. |
| `internal/application/` | Use-cases; orchestrates domain + ports. |
| `internal/ports/` | Interfaces the application depends on. |
| `internal/providers/` | The **only** place third-party SDKs (Meta / Zoho / OpenAI / …) may live. |
| `internal/infrastructure/` | MySQL / Redis / HBase / HTTP / auth / observability implementations of ports. |
| `internal/events/` | Event bus (in-proc fan-out + Redis Streams). |
| `internal/workers/` | Background consumers. |
| `internal/webhook/` | Provider-agnostic webhook ingress. |
| `internal/api/` | REST + WebSocket handlers, OpenAPI spec. |
| `web/` | Vite + React frontend, embedded into the binary at build. |
| `migrations/` | `golang-migrate` SQL files. |
| `docs/` | Architecture, ADRs, phase notes, domain docs, flow docs, provider docs, runbooks. |

## Phase status

| Phase | Scope | Status |
|-------|-------|--------|
| 0 | Foundations — skeleton, CLAUDE.md harness, CI, gstack team-mode. | In progress |
| 1 | WhatsApp inbox MVP. | Pending |
| 2 | Tickets, templates, automation v1. | Pending |
| 3 | Campaigns, AI orchestration, calling. | Pending |
| 4 | Hardening, observability, compliance. | Pending |
| 5 | Provider expansion. | Ongoing |

Full delivery plan: [`docs/phases/`](docs/phases/).

## License

MIT — see [LICENSE](LICENSE).
