# CLI (nudgeway-cli)

`nudgeway-cli` is the admin CLI for bootstrap tasks — creating tenants, users, and provider integrations, and running migrations. It talks to MySQL directly using the same config file the server reads; it does not go through the REST API.

## Build

```bash
go build -o bin/nudgeway-cli ./cmd/cli
```

## Configuration

The CLI reads the same `config/local.yaml` your server uses. In particular it needs:

- `mysql.dsn` — the Go-driver DSN (`user:pass@tcp(host:port)/db?parseTime=true`).
- `auth.credential_kek_hex` — the 32-byte hex KEK used to envelope-encrypt integration secrets. Required for `integration create`.
- `auth.argon2.*` — parameters for hashing user passwords (`user create` uses these).

Point the CLI at a non-default config with the standard config-loader search path.

## Commands

### `tenant create`

Create an organization row.

```bash
./bin/nudgeway-cli tenant create --slug acme --name "Acme Co"
```

Flags:

- `--slug` (required) — URL-safe org slug used by every subsequent `--org-slug` lookup.
- `--name` (required) — display name.

### `user create`

Create a platform user under an existing tenant.

```bash
./bin/nudgeway-cli user create \
  --org-slug acme \
  --email you@acme.com \
  --password password123 \
  --admin
```

Flags:

- `--org-slug` (required) — the tenant to attach the user to.
- `--email` (required).
- `--password` (required) — argon2id-hashed on insert per `auth.argon2.*`.
- `--admin` — grant admin scopes. Omit for a regular user.

### `integration create`

Seed a provider integration and its envelope-encrypted secrets.

```bash
./bin/nudgeway-cli integration create \
  --org-slug acme \
  --provider whatsapp \
  --name "Acme India" \
  --phone-number-id 1234567890 \
  --waba-id 0987654321 \
  --access-token "EAAB..." \
  --app-secret "..." \
  --verify-token "pick-any-string"
```

Flags:

- `--org-slug`, `--name` (required for every provider).
- `--provider` — provider registry key, default `whatsapp`.
- For `whatsapp`: `--phone-number-id`, `--waba-id`, `--access-token`, `--app-secret`, `--verify-token` are all required.

On success the CLI prints the integration ID and the webhook URL path:

```
integration created: id=01JC5..., webhook_url=/webhooks/whatsapp/01JC5...
```

Prepend your public tunnel origin (cloudflared / ngrok) to that path and paste into Meta App → WhatsApp → Configuration. See [Connect a WhatsApp integration](/#/integrations/connect-whatsapp).

### `migrate up | down | status`

Shells out to the standard `migrate` binary using the DSN from your config.

```bash
./bin/nudgeway-cli migrate up
./bin/nudgeway-cli migrate down --steps 1
./bin/nudgeway-cli migrate status
```

Flags:

- `--steps N` — apply / revert exactly N migrations.

Requires the [`golang-migrate`](https://github.com/golang-migrate/migrate) CLI on `$PATH`:

```bash
brew install golang-migrate
# or:
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

If `migrate` is not on `$PATH` the CLI prints a message and exits non-zero.

## Environment variables

`nudgeway-cli` does not require any special env vars beyond what your config loader honours. In particular:

- Nothing like `NUDGEWAY_API_URL` / `NUDGEWAY_API_TOKEN` — those are for the MCP server, not the CLI. The CLI writes directly to MySQL.
- `PATH` must contain the `migrate` binary for the `migrate` subcommand.

## Related

- [First run](/#/getting-started/first-run) — the tenant + user + integration bootstrap walkthrough.
- [Connect a WhatsApp integration](/#/integrations/connect-whatsapp) — the UI flow the `integration create` command mirrors.
- [MCP server](/#/developer/mcp-server) — programmatic access via HTTP (not the CLI).
