# Install services (native)

Nudgeway needs four backing services running on the local machine before `make dev`: **MySQL 8+**, **Redis 7+**, **Kafka 3+**, and **HBase 2+**. Native install is the recommended day-to-day dev path — every service becomes a normal OS service, restarts survive reboots, and there is no VM overhead.

> **Note.** This page is for the operator setting Nudgeway up on their own machine — command-line steps are unavoidable here. Once the app is running, the rest of the help pages walk you through the browser UI.

The full, canonical instructions live in [`docs/install-services.md`](https://github.com/v-senthil/nudgeway/blob/main/docs/install-services.md). The essentials are inline below.

## macOS (Homebrew)

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

Homebrew's Kafka formula ships with KRaft mode (no ZooKeeper). HBase runs standalone with an embedded ZooKeeper on `:2181`.

## Debian / Ubuntu

```bash
# MySQL 8
sudo apt update
sudo apt install -y mysql-server
sudo mysql_secure_installation

# Redis 7
sudo apt install -y redis-server
sudo systemctl enable --now redis-server
```

Kafka + HBase have no distro package with a recent enough version. Grab the official Apache tarballs (`kafka_2.13-3.7.1.tgz`, `hbase-2.5.10-bin.tar.gz`), untar under `/opt/`, and start each with its bundled script. Full commands in [`docs/install-services.md`](https://github.com/v-senthil/nudgeway/blob/main/docs/install-services.md#linux--debian--ubuntu).

## Fedora / RHEL

```bash
sudo dnf install -y community-mysql-server redis
sudo systemctl enable --now mysqld redis
# Kafka + HBase: use the tarball steps from the Debian/Ubuntu section.
```

## Windows

WSL2 is the least painful path — install Ubuntu from the Microsoft Store, then follow the Debian/Ubuntu section inside it. Native Windows works for MySQL + Redis via Chocolatey (`choco install mysql redis-64`), but Kafka and HBase need WSL2 or the [Docker Compose](#/getting-started/docker-compose) path.

## Verify

```bash
make check-infra
# expected: MySQL ✓  Redis ✓  Kafka ✓  HBase ✓
```

## Troubleshooting

- **Port already in use** — another Nudgeway service (native or containerised) is listening. Pick one path, not both. `lsof -i :<port>` finds the culprit.
- **HBase master exits immediately** — usually a hostname resolution issue. Ensure `127.0.0.1 localhost` is in `/etc/hosts`.
- **Kafka can't find broker** — KRaft mode needs `advertised.listeners=PLAINTEXT://127.0.0.1:9092`. Check `$(brew --prefix)/etc/kafka/server.properties` on macOS.
- **MySQL 8 caching_sha2 errors** — Nudgeway's `go-sql-driver/mysql` handles it fine; verify the driver version in `go.mod` if you see `Access denied` with the right password.
- **Homebrew HBase silently fails** — Java version drift. `brew install openjdk@17` and re-run `brew services restart hbase`.

## Related

- [Docker Compose](#/getting-started/docker-compose) — one-command alternative.
- [Apple Containers](#/getting-started/apple-containers) — macOS 26+ native OCI runtime.
- [First run](#/getting-started/first-run) — org + admin + first message.
