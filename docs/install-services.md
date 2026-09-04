# Install the four backing services

Nudgeway needs **MySQL 8+**, **Redis 7+**, **Kafka 3+**, and **HBase 2+** running on the local machine before `make dev`.

Two paths:

1. [Native install](#native-install) — recommended for day-to-day dev. Every service becomes a normal OS service, so restarts survive reboots and there's no VM overhead.
2. [Docker Compose](#docker-compose) — one command, everything in a container. Good for CI or when you don't want the services scattered across your OS.

Pick one. Don't mix native + Docker for the same service or ports collide.

## Native install

### macOS (Homebrew)

```bash
# Install
brew install mysql redis kafka hbase

# Start (auto-restart on login)
brew services start mysql
brew services start redis
brew services start kafka
brew services start hbase

# One-time MySQL setup — matches config/example.yaml
mysql -uroot -e "ALTER USER 'root'@'localhost' IDENTIFIED WITH caching_sha2_password BY 'root';"
mysql -uroot -proot -e "CREATE DATABASE IF NOT EXISTS nudgeway CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
```

Homebrew's Kafka formula ships with KRaft mode (no separate ZooKeeper). HBase runs standalone with an embedded ZooKeeper on `:2181`.

### Linux — Debian / Ubuntu

```bash
# MySQL 8
sudo apt update
sudo apt install -y mysql-server
sudo mysql_secure_installation

# Redis 7
sudo apt install -y redis-server
sudo systemctl enable --now redis-server

# Kafka 3 — official Apache tarball (no distro package with a recent enough version)
KAFKA_VERSION=3.7.1
wget https://downloads.apache.org/kafka/${KAFKA_VERSION}/kafka_2.13-${KAFKA_VERSION}.tgz
sudo tar -xzf kafka_2.13-${KAFKA_VERSION}.tgz -C /opt/
sudo ln -s /opt/kafka_2.13-${KAFKA_VERSION} /opt/kafka
# KRaft single-node bring-up
sudo /opt/kafka/bin/kafka-storage.sh format \
  --config /opt/kafka/config/kraft/server.properties \
  --cluster-id $(sudo /opt/kafka/bin/kafka-storage.sh random-uuid) --ignore-formatted
sudo /opt/kafka/bin/kafka-server-start.sh -daemon /opt/kafka/config/kraft/server.properties

# HBase 2 standalone
HBASE_VERSION=2.5.10
wget https://downloads.apache.org/hbase/${HBASE_VERSION}/hbase-${HBASE_VERSION}-bin.tar.gz
sudo tar -xzf hbase-${HBASE_VERSION}-bin.tar.gz -C /opt/
sudo ln -s /opt/hbase-${HBASE_VERSION} /opt/hbase
sudo /opt/hbase/bin/start-hbase.sh
```

### Linux — Fedora / RHEL

```bash
sudo dnf install -y community-mysql-server redis
sudo systemctl enable --now mysqld redis
# Kafka + HBase: use the tarball steps from the Debian/Ubuntu section.
```

### Windows

WSL2 is the least painful path — install Ubuntu from the Microsoft Store, then follow the [Debian/Ubuntu section](#linux--debian--ubuntu) inside it.

If you insist on native Windows, install [Chocolatey](https://chocolatey.org/install), then:

```powershell
choco install mysql redis-64
# Kafka + HBase have no working Chocolatey packages;
# download the Apache tarballs into WSL2 or use Docker Compose below.
```

## Docker Compose

Nudgeway ships a [`docker-compose.yml`](../docker-compose.yml) at the repo root that starts all four services with sane dev defaults (ports 3306 / 6379 / 9092 / 2181 / 16010).

Works with:
- **Docker Desktop** (macOS, Windows, Linux)
- **Podman** (`podman compose up -d`) — Docker-compatible on Linux + macOS
- **Apple Containers** — the OCI runtime that ships with macOS 26+, speaks the Compose spec

Bring everything up:

```bash
docker compose up -d          # start
docker compose logs -f        # tail
docker compose ps             # status
docker compose down           # stop (volumes persist)
docker compose down --volumes # nuke data
```

Everything binds to `127.0.0.1` so `config/example.yaml` values work as-is. HBase takes ~60 seconds to fully warm up on first boot — the healthcheck reflects that.

The Go server + Vite dev server still run natively via `make dev`. This compose file only replaces the four backing services.

## Verify

Once the services are up (either path):

```bash
make check-infra
# expected: MySQL ✓  Redis ✓  Kafka ✓  HBase ✓
```

Then continue with the [README](../README.md) — apply migrations, create an org + admin, `make dev`.

## Troubleshooting

- **Port already in use** — another Nudgeway service (native or containerised) is listening. Pick one path, not both. `lsof -i :<port>` finds the culprit.
- **HBase master exits immediately** — usually a hostname resolution issue. Ensure `127.0.0.1 localhost` is in `/etc/hosts`. Under Docker, the compose file sets `hostname: hbase` to work around this.
- **Kafka can't find broker** — with the KRaft compose config, the advertised listener is `127.0.0.1:9092`; if you renamed the compose project, edit the value.
- **MySQL 8 caching_sha2 vs legacy client** — some old Go drivers demand `mysql_native_password`. Nudgeway's `go-sql-driver/mysql` handles `caching_sha2` fine; if you see auth errors, verify the driver version in `go.mod`.
