#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
RPC_URL="${RPC_URL:-http://127.0.0.1:8545}"
ANVIL_PID_FILE="${ANVIL_PID_FILE:-$ROOT_DIR/.local/anvil.pid}"
ANVIL_LOG_FILE="${ANVIL_LOG_FILE:-$ROOT_DIR/.local/anvil.log}"

mkdir -p "$ROOT_DIR/.local"

wait_for_rpc() {
  i=0
  while [ "$i" -lt 60 ]; do
    if cast block-number --rpc-url "$RPC_URL" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

if ! cast block-number --rpc-url "$RPC_URL" >/dev/null 2>&1; then
  echo "starting local anvil on $RPC_URL"
  nohup sh "$ROOT_DIR/deploy/scripts/start-local-chain.sh" </dev/null >"$ANVIL_LOG_FILE" 2>&1 &
  echo $! >"$ANVIL_PID_FILE"
  wait_for_rpc
else
  echo "local anvil already running on $RPC_URL"
fi

sh "$ROOT_DIR/deploy/scripts/bootstrap-local-chain.sh"
docker compose up -d --build mysql mysql-init redis rabbitmq api-server indexer market-data

echo
echo "local dev stack is ready"
echo "frontend env: $ROOT_DIR/frontend/.env.local"
echo "frontend start: cd $ROOT_DIR/frontend && pnpm dev"
