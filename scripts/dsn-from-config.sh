#!/usr/bin/env bash
# scripts/dsn-from-config.sh — extract the MySQL DSN from the config file and
# rewrite it for the golang-migrate driver, which expects `mysql://` prefix.
set -euo pipefail
CFG="${1:-config/local.yaml}"
[[ -f "$CFG" ]] || CFG="config/example.yaml"
dsn=$(awk '/^mysql:/,/^[a-zA-Z_]+:/' "$CFG" | awk -F': *' '/dsn:/ {gsub(/^"|"$/,"",$2); print $2; exit}')
# golang-migrate expects: mysql://user:pass@tcp(host:port)/db?params  (no leading protocol on the go-sql-driver DSN either)
# but the migrate binary parses `mysql://` prefix. Convert if not present.
if [[ "$dsn" != mysql://* ]]; then
  dsn="mysql://${dsn}"
fi
echo "$dsn"
