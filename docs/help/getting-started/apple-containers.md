# Apple Containers (macOS 26+)

macOS 26 ships `container` — Apple's native OCI runtime built on Virtualization.framework. It reads the same OCI images and Compose files as Docker, but every container gets its own lightweight VM, no Docker Desktop background VM to babysit, and no license worries. The [`docker-compose.yml`](https://github.com/v-senthil/nudgeway/blob/main/docker-compose.yml) at the repo root works as-is.

## Prerequisites

- macOS 26 or newer (Apple Silicon or Intel).
- The `container` CLI (bundled with macOS 26; verify with `container --version`).

## One-time setup

```bash
softwareupdate --install --all      # ensure macOS 26 is current
container system start              # brings up the container VM daemon
```

## Bring the stack up (compose)

```bash
container compose up -d
container compose logs -f
container compose ps
container compose down
```

If your macOS 26 install doesn't have `container compose` yet (the plugin ships separately in some builds):

```bash
brew install container-compose
# or: container plugin install compose
```

## Alternative — run each service directly

No compose plugin? Same result with four `container run` invocations:

```bash
container run -d --name nudgeway-mysql \
  -p 3306:3306 -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=nudgeway \
  mysql:8.0

container run -d --name nudgeway-redis \
  -p 6379:6379 redis:7-alpine

container run -d --name nudgeway-kafka \
  -p 9092:9092 \
  -e KAFKA_ENABLE_KRAFT=yes \
  -e KAFKA_CFG_PROCESS_ROLES=broker,controller \
  -e KAFKA_CFG_NODE_ID=1 \
  -e KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=1@localhost:9093 \
  -e KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
  -e KAFKA_CFG_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:9092 \
  -e KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER \
  -e KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
  -e ALLOW_PLAINTEXT_LISTENER=yes \
  bitnami/kafka:3.7

container run -d --name nudgeway-hbase \
  -p 2181:2181 -p 16000:16000 -p 16010:16010 -p 16020:16020 -p 16030:16030 \
  -e HBASE_MANAGES_ZK=true --hostname hbase \
  harisekhon/hbase:2.1
```

Everything binds `127.0.0.1` on the host so `config/example.yaml` works as-is; the Go server + Vite still run natively via `make dev`.

## Verify

```bash
make check-infra
# expected: MySQL ✓  Redis ✓  Kafka ✓  HBase ✓
```

## Troubleshooting

- **`container: command not found`** — you're on macOS < 26. Use [Docker Compose](/#/getting-started/docker-compose) instead.
- **`container system start` hangs** — the Virtualization.framework needs Full Disk Access. Grant it in System Settings → Privacy & Security.
- **Port collision with Docker Desktop** — Apple Containers and Docker Desktop can both listen on `127.0.0.1:3306`. Quit Docker Desktop or pick different host ports.
- **HBase container restarts** — hostname must resolve. `--hostname hbase` is required; the compose file already sets it.
- **Slow first boot** — each container is a fresh VM. Warm-up is ~10-15s per service; HBase adds another ~60s to reach master-ready.

## Related

- [Install services (native)](/#/getting-started/install-services) — Homebrew path.
- [Docker Compose](/#/getting-started/docker-compose) — same compose file, Docker Desktop / Podman.
- [First run](/#/getting-started/first-run) — org + admin + first message.
