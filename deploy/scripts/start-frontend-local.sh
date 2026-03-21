#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-$ROOT_DIR/.local/contracts.env}"

if [ -s "$CHAIN_ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$CHAIN_ENV_FILE"
  set +a
fi

export VITE_APP_ENV="${VITE_APP_ENV:-dev}"
export VITE_API_BASE_URL="${VITE_API_BASE_URL:-http://127.0.0.1:8080}"
export VITE_WS_BASE_URL="${VITE_WS_BASE_URL:-ws://127.0.0.1:8080/ws}"
export VITE_SUPPORTED_CHAINS="${VITE_SUPPORTED_CHAINS:-local}"
export VITE_LOCAL_CHAIN_ID="${VITE_LOCAL_CHAIN_ID:-31337}"
export VITE_LOCAL_USDC_ADDRESS="${VITE_LOCAL_USDC_ADDRESS:-${BASE_USDC_ADDRESS:-}}"

cd "$ROOT_DIR/frontend"
exec pnpm dev --host 127.0.0.1 --port "${FRONTEND_PORT:-5173}"
