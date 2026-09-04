# fullWA

Open-source, multi-tenant WhatsApp Business Platform. Provider-agnostic core — WhatsApp today, Zoho / OpenAI / Anthropic / Freshdesk / others plug in as adapters.

- **Backend:** Go modular monolith, single binary. MySQL + Redis + Kafka + HBase, all local + native.
- **Frontend:** React 18 + TypeScript + Vite + TanStack Router/Query + Tailwind.
- **Real-time:** WebSocket inbox + live incoming-call popup with WebRTC accept.
- **Storage:** MySQL (source of truth), Redis (cache/locks/rate limits), Kafka (durable event log + job queues), HBase (attachments + call recordings + transcripts).

## What works today (Phase 1 → Phase 3)

- Inbound + outbound WhatsApp messages (text, media, location, contacts, interactive, template, reactions).
- Live inbox with WebSocket updates. WhatsApp-style status ticks (queued → sent → delivered → read → failed).
- Media upload / download via HBase (durable — no per-request Meta round-trip).
- Message templates: CRUD + submit-for-review + Meta sync + WhatsApp-style rendering.
- WhatsApp Business Calling: business-initiated calls with WebRTC accept in-browser (recording + transcription optional), inbound-call popup with Accept/Reject, calls shown inline in the conversation thread.
- Meta OBA status, business profile, call settings, business username CRUD + QR-code display.
- Meta phone-number metadata rendered per integration.
- Groups (list / sync / create when your number is OBA-eligible).
- Full audit log + Meta API execution log (`/settings/audit`, `/settings/provider-calls`).
- Analytics rollups (messages, delivery rate, response time, calls) with sparklines.

## Prerequisites

Run these natively on your machine — **no Docker**.

| Service | Version | Notes |
|---|---|---|
| **MySQL** | 8.0+ | The transactional source of truth. |
| **Redis** | 7+ | Cache, locks, rate limiters, WS presence. |
| **Kafka** | 3+ | Durable event log + per-conversation-ordered job queues. |
| **HBase** | 2+ | Attachment + recording + transcript blob store. |
| **Go** | 1.21+ | Build tool + runtime. |
| **Node.js** | 20+ | Vite dev server. |
| **npm** | 10+ | Frontend deps. |
| **golang-migrate** | latest | Schema migrations. `brew install golang-migrate`. |

Optionally: a public HTTPS tunnel (cloudflared / ngrok) — needed only to expose your local webhook URL to Meta.

## 1. Clone + configure

```bash
git clone https://github.com/v-senthil/whatsapp-cloud-api.git fullWA
cd fullWA

# Copy the example config; edit for your local services.
cp config/example.yaml config/local.yaml

# Generate a 32-byte KEK for envelope-encrypted integration credentials.
openssl rand -hex 32
# Paste the output into config/local.yaml → auth.credential_kek_hex.
```

Point the DSNs at your local services (defaults assume `127.0.0.1` with default ports):

- `mysql.dsn` — `root:root@tcp(127.0.0.1:3306)/fullwa?parseTime=true&multiStatements=true`
- `redis.addr` — `127.0.0.1:6379`
- `kafka.brokers` — `["127.0.0.1:9092"]`
- `hbase.zookeeper_quorum` — `["127.0.0.1:2181"]`

Verify everything is reachable:

```bash
make check-infra
```

## 2. Create the database + apply migrations

```bash
# Create the database.
mysql -u root -proot -e 'CREATE DATABASE IF NOT EXISTS fullwa CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;'

# Apply schema migrations.
migrate -path migrations \
  -database 'mysql://root:root@tcp(127.0.0.1:3306)/fullwa?parseTime=true&multiStatements=true' \
  up
```

## 3. Create an organization + admin user

Build the CLI:

```bash
go build -o bin/fullwa-cli ./cmd/cli
```

Then:

```bash
# Create an organization.
./bin/fullwa-cli tenant create --slug acme --name "Acme Co"

# Create the first admin user (grants the built-in admin role — every permission).
./bin/fullwa-cli user create \
  --org-slug acme --email you@acme.com --password password123 --admin
```

## 4. Start backend + frontend

Easiest — run both in one terminal:

```bash
make dev
```

That launches the Go server on `:8080` and the Vite dev server on `:5173`. Ctrl-C stops both.

Or run them separately:

```bash
# terminal 1 — backend
FULLWA_CONFIG=config/local.yaml go run ./cmd/server

# terminal 2 — frontend
cd web && npm install && npm run dev
```

Open **http://localhost:5173** and log in with the user you just created.

## 5. Connect a WhatsApp phone number

Meta prerequisites you'll need from https://developers.facebook.com:

- A Meta App with the **WhatsApp** product added.
- A **WhatsApp Business Account (WABA)** — grab the `WABA ID`.
- A **Business phone number** — grab the `Phone Number ID` + `Display phone number`.
- A **System User Access Token** with `whatsapp_business_messaging` + `whatsapp_business_management` scopes.
- The App's **App Secret** (Settings → Basic → App Secret).
- A **Verify Token** you pick yourself (any string; you'll paste it into both Meta and fullWA).

In the fullWA UI:

1. Go to **Settings → Integrations → Connect WhatsApp**.
2. Paste the six fields (Name, Phone Number ID, WABA ID, Access Token, App Secret, Verify Token).
3. Save. fullWA envelope-encrypts the secrets and stores them.
4. Copy the **Webhook URL** shown on the integration row (e.g. `https://<your-tunnel>.trycloudflare.com/webhooks/whatsapp/<integration_id>`).

In the Meta App dashboard → **WhatsApp → Configuration**:

1. Paste the webhook URL into **Callback URL**.
2. Paste the Verify Token you chose in step 2.
3. Click **Verify and save** — fullWA responds with the `hub.challenge`.
4. Subscribe to at least: `messages`, `message_status`, `calls`, `call_settings_update`.

### Sanity checks

- **Outbound:** open the Inbox in fullWA → pick a conversation with a customer who has messaged you (a 24-hour open session is required for non-template sends) → type a message → Send. It should arrive on the customer's WhatsApp within a second.
- **Inbound:** send a WhatsApp message from your phone → the conversation appears in the Inbox, message renders live via WebSocket.
- **Calls:** trigger an inbound call → an accept/reject popup appears bottom-right. Clicking Accept prompts for mic permission, negotiates WebRTC, and connects audio.

### Dev-mode webhook signature

For local development the webhook accepts payloads that match the configured `phone_number_id` + `waba_id` (Meta App Secret is unreliable across some networks). Set `FULLWA_REQUIRE_SIGNATURE=1` in the env to force HMAC-SHA256 `X-Hub-Signature-256` verification (recommended for production).

## Repo layout

| Path | Purpose |
|---|---|
| `cmd/server/` | Single-binary entrypoint (HTTP + WS + workers + scheduler). |
| `cmd/cli/` | Admin CLI: `tenant create`, `user create`, `integration create`, `migrate`. |
| `internal/domain/` | Pure Go domain model. Zero infra / provider imports. |
| `internal/application/` | Use-cases. Orchestrates domain + ports. |
| `internal/ports/` | Interfaces the application depends on. |
| `internal/providers/` | The **only** place third-party SDKs (Meta / Zoho / OpenAI / ...) live. |
| `internal/infrastructure/` | MySQL / Redis / Kafka / HBase / WebSocket / auth / observability implementations. |
| `internal/webhook/` | Provider-agnostic webhook ingress. |
| `internal/workers/` | Background consumers (webhook process, message send, analytics rollup). |
| `internal/events/` | Event bus (in-proc + Kafka). |
| `internal/api/` | REST + WebSocket handlers, OpenAPI spec. |
| `web/` | Vite + React + TypeScript SPA. |
| `migrations/` | `golang-migrate` SQL files. |
| `config/` | `example.yaml` committed; `local.yaml` git-ignored. |
| `docs/` | Architecture, ADRs, phase notes, domain docs, flow docs, provider docs. |

## Commands cheat sheet

```bash
make check-infra              # verifies MySQL / Redis / Kafka / HBase reachable
make migrate ARGS="up"        # apply schema migrations
make dev                      # run backend + Vite dev server together
make build                    # build production binaries (embeds the frontend)
make test                     # unit tests
make test-int                 # integration tests (real MySQL/Redis/HBase)
make verify                   # fmt + vet + lint + arch-lint + all tests

# CLI
./bin/fullwa-cli tenant create --slug X --name Y
./bin/fullwa-cli user create --org-slug X --email E --password P [--admin]
./bin/fullwa-cli integration create --org-slug X --provider whatsapp \
    --name N --phone-number-id P --waba-id W --access-token T \
    --app-secret A --verify-token V
```

## Troubleshooting

**MySQL migrations fail with "duplicate migration file"** — check `ls migrations/ | sort` for two files with the same numeric prefix; renumber the offending pair.

**Webhook verification returns 502 in Meta App dashboard** — your tunnel (cloudflared / ngrok) isn't forwarding to `localhost:8080`. Test with `curl <public-url>/healthz` first.

**Outbound message returns "conversation not open"** — WhatsApp enforces a 24-hour customer-service window. Send an approved template to open a session, or wait for the customer to message you first.

**Incoming call popup doesn't appear** — log out + log back in. Session-scoped permissions are cached; new deploys with new permissions need a fresh login.

**Analytics cards show 0** — the rollup worker runs every 15 minutes. Restart the server to trigger an immediate roll-up on boot, or wait for the next tick.

## Docs

- [`docs/architecture.md`](docs/architecture.md) — the full picture.
- [`docs/phases/`](docs/phases/) — what shipped per phase.
- [`docs/providers/whatsapp.md`](docs/providers/whatsapp.md) — WhatsApp adapter capability matrix + BSUID / call permissions / templates / OBA.
- [`docs/domain/`](docs/domain/) — one file per canonical entity (Contact, Session, Conversation, Message, Call, Template, Group, ...).
- [`docs/flows/`](docs/flows/) — sequence diagrams for the async pipelines.
- [`docs/api/CHANGELOG.md`](docs/api/CHANGELOG.md) — OpenAPI history.
- [`CHANGELOG.md`](CHANGELOG.md) — reverse-chronological release notes.

## License

See [LICENSE](LICENSE).
