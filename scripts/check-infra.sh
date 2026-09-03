#!/usr/bin/env bash
# scripts/check-infra.sh — verify the user's local MySQL, Redis, and HBase are reachable
# per config/local.yaml. Prints a red/green summary and exits non-zero on any failure.
#
# Usage:
#   ./scripts/check-infra.sh                 # reads config/local.yaml (or config/example.yaml as fallback)
#   ./scripts/check-infra.sh path/to.yaml    # reads a specific config

set -euo pipefail

CFG="${1:-config/local.yaml}"
if [[ ! -f "$CFG" ]]; then
  echo "[warn] $CFG not found; falling back to config/example.yaml" >&2
  CFG="config/example.yaml"
fi
if [[ ! -f "$CFG" ]]; then
  echo "[fail] no config file to read" >&2
  exit 2
fi

RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; RST=$'\033[0m'
fail=0

# Minimal YAML value extraction (single-level keys under top-level sections).
yaml_val() {
  local section="$1" key="$2"
  awk -v s="$section" -v k="$key" '
    $0 ~ "^"s":"           { in_s=1; next }
    in_s && $0 ~ "^[a-zA-Z_]+:" && $1 != k":" { in_s=0 }
    in_s && $1 == k":"     { $1=""; sub(/^[ \t]+/, ""); gsub(/^"|"$/, ""); print; exit }
  ' "$CFG"
}

# --- MySQL ------------------------------------------------------------------
DSN="$(yaml_val mysql dsn || true)"
if [[ -z "$DSN" ]]; then
  echo "${YEL}[skip]${RST} MySQL — no dsn in $CFG"
else
  # Parse user:pass@tcp(host:port)/db from DSN
  host="$(echo "$DSN" | sed -n 's#.*tcp(\([^:]*\):\([0-9]*\)).*#\1#p')"
  port="$(echo "$DSN" | sed -n 's#.*tcp(\([^:]*\):\([0-9]*\)).*#\2#p')"
  user="$(echo "$DSN" | sed -n 's#^\([^:]*\):.*#\1#p')"
  db="$(echo "$DSN"   | sed -n 's#.*/\([^?]*\)?.*#\1#p')"
  host="${host:-127.0.0.1}"; port="${port:-3306}"
  if command -v mysqladmin >/dev/null 2>&1; then
    if mysqladmin ping -h "$host" -P "$port" -u "$user" --silent 2>/dev/null; then
      echo "${GRN}[ok]${RST}  MySQL  reachable at $host:$port (user=$user, db=$db)"
    else
      echo "${RED}[fail]${RST} MySQL  not reachable at $host:$port (user=$user)"
      fail=1
    fi
  else
    # Fallback: raw TCP connect
    if (echo > /dev/tcp/"$host"/"$port") 2>/dev/null; then
      echo "${GRN}[ok]${RST}  MySQL  TCP open at $host:$port (mysqladmin not installed, cannot auth-check)"
    else
      echo "${RED}[fail]${RST} MySQL  TCP closed at $host:$port"
      fail=1
    fi
  fi
fi

# --- Redis ------------------------------------------------------------------
r_addr="$(yaml_val redis addr || true)"
r_addr="${r_addr:-127.0.0.1:6379}"
r_host="${r_addr%:*}"; r_port="${r_addr##*:}"
if command -v redis-cli >/dev/null 2>&1; then
  if [[ "$(redis-cli -h "$r_host" -p "$r_port" ping 2>/dev/null)" == "PONG" ]]; then
    echo "${GRN}[ok]${RST}  Redis  reachable at $r_addr"
  else
    echo "${RED}[fail]${RST} Redis  no PONG from $r_addr"
    fail=1
  fi
else
  if (echo > /dev/tcp/"$r_host"/"$r_port") 2>/dev/null; then
    echo "${GRN}[ok]${RST}  Redis  TCP open at $r_addr (redis-cli not installed)"
  else
    echo "${RED}[fail]${RST} Redis  TCP closed at $r_addr"
    fail=1
  fi
fi

# --- HBase ------------------------------------------------------------------
# Prefer Zookeeper quorum; fall back to Thrift addr.
zk_line="$(awk '/^hbase:/,/^[a-zA-Z_]+:/' "$CFG" | grep -E 'zookeeper_quorum:' || true)"
zk_targets=()
if [[ -n "$zk_line" ]]; then
  # Extract host:port entries from inline array like ["127.0.0.1:2181"]
  while read -r hp; do zk_targets+=("$hp"); done < <(echo "$zk_line" | grep -oE '[a-zA-Z0-9._-]+:[0-9]+' || true)
fi
thrift_addr="$(yaml_val hbase thrift_addr || true)"

if [[ ${#zk_targets[@]} -gt 0 ]]; then
  hb_ok=0
  for hp in "${zk_targets[@]}"; do
    zh="${hp%:*}"; zp="${hp##*:}"
    if (echo > /dev/tcp/"$zh"/"$zp") 2>/dev/null; then
      echo "${GRN}[ok]${RST}  HBase  Zookeeper reachable at $hp"
      hb_ok=1
    else
      echo "${RED}[fail]${RST} HBase  Zookeeper not reachable at $hp"
    fi
  done
  [[ $hb_ok -eq 1 ]] || fail=1
elif [[ -n "$thrift_addr" ]]; then
  th="${thrift_addr%:*}"; tp="${thrift_addr##*:}"
  if (echo > /dev/tcp/"$th"/"$tp") 2>/dev/null; then
    echo "${GRN}[ok]${RST}  HBase  Thrift reachable at $thrift_addr"
  else
    echo "${RED}[fail]${RST} HBase  Thrift not reachable at $thrift_addr"
    fail=1
  fi
else
  echo "${YEL}[skip]${RST} HBase — no zookeeper_quorum or thrift_addr in $CFG"
fi

if [[ $fail -ne 0 ]]; then
  echo ""
  echo "${RED}infra check failed${RST} — bring up the failing service(s) locally and re-run."
  exit 1
fi
echo ""
echo "${GRN}all infra reachable${RST}"
