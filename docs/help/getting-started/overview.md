# Getting started

Welcome to Nudgeway — an open-source, multi-tenant WhatsApp Business Platform. This section takes you from zero to sending your first message.

## Path picker

Nudgeway needs four backing services: **MySQL 8+**, **Redis 7+**, **Kafka 3+**, **HBase 2+**. Three ways to bring them up:

- **[Native install](install-services)** — Homebrew / apt / dnf / WSL2. Recommended for day-to-day dev.
- **[Docker Compose](docker-compose)** — one command, works with Docker Desktop, Podman, and Apple Containers.
- **[Apple Containers](apple-containers)** — macOS 26+, no Docker Desktop VM.

Then follow the shared steps:

1. Clone the repo + generate the credential-encryption KEK.
2. Apply MySQL migrations.
3. Create an organization + an admin user via the CLI.
4. `make dev` — backend on `:8080`, Vite frontend on `:5173`.
5. Log in, connect a WhatsApp integration, send yourself a message.

See [First run](first-run) for the full walkthrough.

## What you'll need

- A **Meta App** with the WhatsApp product added (developer.facebook.com).
- A **WhatsApp Business Account (WABA)** with a live phone number.
- A **System User Access Token** with `whatsapp_business_messaging` + `whatsapp_business_management` scopes.
- A public HTTPS tunnel for local dev (cloudflared / ngrok) — needed only to expose your webhook URL to Meta.

## Next

- [Install services (native)](install-services)
- [Docker Compose](docker-compose)
- [First run](first-run)
