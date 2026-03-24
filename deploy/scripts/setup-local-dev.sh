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
docker compose up -d mysql mysql-init redis rabbitmq

echo
echo "local dev stack is ready"
echo "reset local state: sh $ROOT_DIR/deploy/scripts/reset-local-dev.sh"
echo "frontend env: $ROOT_DIR/frontend/.env.local"
echo "api-server start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh api-server"
echo "indexer start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh indexer"
echo "market-data start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh market-data"
echo "order-executor-worker start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh order-executor-worker"
echo "risk-engine-worker start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh risk-engine-worker"
echo "funding-worker start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh funding-worker"
echo "liquidator-worker start: sh $ROOT_DIR/deploy/scripts/run-backend-local.sh liquidator-worker"
echo "frontend start: sh $ROOT_DIR/deploy/scripts/start-frontend-local.sh"
