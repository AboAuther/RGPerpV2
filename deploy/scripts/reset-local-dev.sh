#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ANVIL_PID_FILE="${ANVIL_PID_FILE:-$ROOT_DIR/.local/anvil.pid}"
MULTICHAIN_PID_FILES=(
  "$ROOT_DIR/.local/anvil-ethereum.pid"
  "$ROOT_DIR/.local/anvil-arbitrum.pid"
  "$ROOT_DIR/.local/anvil-base.pid"
)

kill_pattern() {
  local pattern="$1"
  pkill -f "$pattern" >/dev/null 2>&1 || true
}

echo "stopping local app services"
kill_pattern '/exe/api-server'
kill_pattern '/exe/indexer'
kill_pattern '/exe/market-data'
kill_pattern '/exe/order-executor-worker'
kill_pattern '/exe/risk-engine-worker'
kill_pattern '/exe/funding-worker'
kill_pattern '/exe/liquidator-worker'
kill_pattern 'vite/bin/vite.js --host 127.0.0.1 --port 5173'
kill_pattern 'pnpm dev --host 127.0.0.1 --port 5173'

echo "stopping local chain"
if [[ -f "$ANVIL_PID_FILE" ]]; then
  kill "$(cat "$ANVIL_PID_FILE")" >/dev/null 2>&1 || true
fi
for pid_file in "${MULTICHAIN_PID_FILES[@]}"; do
  if [[ -f "$pid_file" ]]; then
    kill "$(cat "$pid_file")" >/dev/null 2>&1 || true
  fi
done
kill_pattern 'anvil --host 0.0.0.0 --port 8545 --chain-id 31337'
kill_pattern 'anvil --host 0.0.0.0 --port 8546 --chain-id 31338'
kill_pattern 'anvil --host 0.0.0.0 --port 8547 --chain-id 31339'
kill_pattern 'anvil --host 127.0.0.1 --port 8545 --chain-id 31337'
kill_pattern 'anvil --host 127.0.0.1 --port 8546 --chain-id 31338'
kill_pattern 'anvil --host 127.0.0.1 --port 8547 --chain-id 31339'

echo "dropping docker data volumes"
cd "$ROOT_DIR"
docker compose down -v --remove-orphans

echo "removing generated local state"
rm -rf "$ROOT_DIR/.local"
rm -f "$ROOT_DIR/deploy/env/local-chains.env"
rm -f "$ROOT_DIR/frontend/.env.local"

echo "local state cleared"
