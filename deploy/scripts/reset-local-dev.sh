#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ANVIL_PID_FILE="${ANVIL_PID_FILE:-$ROOT_DIR/.local/anvil.pid}"

kill_pattern() {
  local pattern="$1"
  pkill -f "$pattern" >/dev/null 2>&1 || true
}

echo "stopping local app services"
kill_pattern '/exe/api-server'
kill_pattern '/exe/indexer'
kill_pattern '/exe/market-data'
kill_pattern '/exe/funding-worker'
kill_pattern 'vite/bin/vite.js --host 127.0.0.1 --port 5173'
kill_pattern 'pnpm dev --host 127.0.0.1 --port 5173'

echo "stopping local chain"
if [[ -f "$ANVIL_PID_FILE" ]]; then
  kill "$(cat "$ANVIL_PID_FILE")" >/dev/null 2>&1 || true
fi
kill_pattern 'anvil --host 0.0.0.0 --port 8545 --chain-id 31337'

echo "dropping docker data volumes"
cd "$ROOT_DIR"
docker compose down -v --remove-orphans

echo "removing generated local state"
rm -rf "$ROOT_DIR/.local"
rm -f "$ROOT_DIR/frontend/.env.local"

echo "local state cleared"
