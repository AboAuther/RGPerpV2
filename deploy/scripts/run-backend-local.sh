#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
SERVICE_BIN="${1:-${SERVICE_BIN:-}}"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-$ROOT_DIR/.local/contracts.env}"

if [ -z "$SERVICE_BIN" ]; then
  echo "usage: $0 <api-server|indexer|market-data|funding-worker|migrator>" >&2
  exit 1
fi

if [ -s "$CHAIN_ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$CHAIN_ENV_FILE"
  set +a
fi

if [ -n "${BASE_RPC_URL_HOST:-}" ]; then
  export BASE_RPC_URL="${BASE_RPC_URL_HOST}"
fi

export APP_ENV="${APP_ENV:-dev}"
export LOG_LEVEL="${LOG_LEVEL:-debug}"
export TZ="${TZ:-UTC}"
export AUTH_DOMAIN="${AUTH_DOMAIN:-localhost}"
export JWT_ACCESS_SECRET="${JWT_ACCESS_SECRET:-local-access-secret}"
export JWT_REFRESH_SECRET="${JWT_REFRESH_SECRET:-local-refresh-secret}"
export RUNTIME_CONFIG_PATH="${RUNTIME_CONFIG_PATH:-$ROOT_DIR/deploy/config/runtime/dev.yaml}"
export MYSQL_DSN="${MYSQL_DSN:-root:root@tcp(127.0.0.1:3306)/rgperp?charset=utf8mb4&parseTime=True&loc=UTC}"
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export REDIS_PASSWORD="${REDIS_PASSWORD:-}"
export REDIS_DB="${REDIS_DB:-0}"
export RABBITMQ_URL="${RABBITMQ_URL:-amqp://guest:guest@127.0.0.1:5672/}"

case "$SERVICE_BIN" in
  api-server)
    export APP_PORT="${APP_PORT:-8080}"
    ;;
  indexer)
    export APP_PORT="${APP_PORT:-8081}"
    ;;
  market-data)
    export APP_PORT="${APP_PORT:-8082}"
    ;;
  funding-worker)
    export APP_PORT="${APP_PORT:-8084}"
    ;;
  migrator)
    export APP_PORT="${APP_PORT:-8083}"
    ;;
  *)
    echo "unsupported service: $SERVICE_BIN" >&2
    exit 1
    ;;
esac

cd "$ROOT_DIR/backend"
exec go run "./cmd/${SERVICE_BIN}"
