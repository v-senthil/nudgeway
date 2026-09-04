# Docker Compose

The repo ships a [`docker-compose.yml`](https://github.com/v-senthil/nudgeway/blob/main/docker-compose.yml) at the root that starts all four backing services with sane dev defaults. Use this when you don't want MySQL / Redis / Kafka / HBase scattered across your OS, for CI, or for a quick evaluation. The Go server and Vite dev server still run natively via `make dev` — the compose file only replaces the infra.

## Prerequisites

- **Docker Desktop** (macOS, Windows, Linux), **Podman** (Linux + macOS, no Desktop app), or **Apple Containers** (see the [Apple Containers](/#/getting-started/apple-containers) page for the macOS 26+ native runtime).
- Do not mix with native services — pick one path, ports collide otherwise.

## Bring the stack up

```bash
docker compose up -d          # start MySQL + Redis + Kafka + HBase
docker compose logs -f        # tail logs
docker compose ps             # status
docker compose down           # stop (volumes persist)
docker compose down --volumes # stop + wipe data
```

Ports bound to `127.0.0.1`:

| Service | Port |
|---|---|
| MySQL | `3306` |
| Redis | `6379` |
| Kafka (PLAINTEXT) | `9092` |
| HBase ZooKeeper | `2181` |
| HBase Master UI | `16010` |

`config/example.yaml` already targets these ports — no edits needed for a fresh checkout.

## Podman

Same compose subcommands, no Docker Desktop:

```bash
brew install podman           # macOS; use your package manager on Linux
podman machine init && podman machine start   # macOS only
podman compose up -d
```

## First-boot timing

HBase takes ~60 seconds to fully warm up on first boot. The compose healthcheck reflects that — `docker compose ps` will show HBase as `starting` until the master is up. `make check-infra` waits for readiness.

## Next steps

```bash
make check-infra              # confirm all four are reachable
make migrate up               # apply SQL migrations
./bin/nudgeway-cli tenant create --slug acme --name "Acme Co"
make dev                      # backend on :8080, Vite on :5173
```

See [First run](/#/getting-started/first-run) for the full walkthrough.

## Troubleshooting

- **`bind: address already in use`** — a native install (Homebrew / apt) is still holding the port. `brew services stop mysql redis kafka hbase` or the systemd equivalent.
- **HBase master exits after 30s** — hostname resolution. The compose file sets `hostname: hbase` for exactly this reason; if you renamed the compose project, the alias breaks.
- **Kafka broker unreachable** — the advertised listener is `127.0.0.1:9092`. If you exposed the container on a different host, override `KAFKA_CFG_ADVERTISED_LISTENERS`.
- **MySQL data survives `down`** — that's intentional. Use `down --volumes` to nuke.
- **Compose plugin missing on Linux** — `sudo apt install docker-compose-plugin` (Debian/Ubuntu) or `sudo dnf install docker-compose-plugin` (Fedora).

## Related

- [Install services (native)](/#/getting-started/install-services) — Homebrew / apt / dnf path.
- [Apple Containers](/#/getting-started/apple-containers) — macOS 26+ native OCI runtime.
- [First run](/#/getting-started/first-run) — org + admin + first message.
