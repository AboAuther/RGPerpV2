#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOCAL_DIR="${ROOT_DIR}/.local"

CHAIN_KEYS=(ethereum arbitrum base)
CHAIN_PORTS=(8545 8546 8547)
CHAIN_IDS=(31337 31338 31339)

mkdir -p "$LOCAL_DIR"

wait_for_rpc() {
  local rpc_url="$1"
  local i=0
  while [[ "$i" -lt 60 ]]; do
    if cast block-number --rpc-url "$rpc_url" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

start_chain() {
  local key="$1"
  local port="$2"
  local chain_id="$3"
  local rpc_url="http://127.0.0.1:${port}"
  local pid_file="${LOCAL_DIR}/anvil-${key}.pid"
  local log_file="${LOCAL_DIR}/anvil-${key}.log"

  if cast block-number --rpc-url "$rpc_url" >/dev/null 2>&1; then
    echo "${key}: already running on ${rpc_url}"
    return 0
  fi

  echo "${key}: starting anvil on ${rpc_url} (chain_id=${chain_id})"
  HOST=0.0.0.0 PORT="$port" CHAIN_ID="$chain_id" nohup sh "${ROOT_DIR}/deploy/scripts/start-local-chain.sh" </dev/null >"$log_file" 2>&1 &
  echo $! >"$pid_file"
  wait_for_rpc "$rpc_url"
}

for idx in "${!CHAIN_KEYS[@]}"; do
  start_chain "${CHAIN_KEYS[$idx]}" "${CHAIN_PORTS[$idx]}" "${CHAIN_IDS[$idx]}"
done

cp "${LOCAL_DIR}/anvil-ethereum.pid" "${LOCAL_DIR}/anvil.pid" 2>/dev/null || true

echo
echo "local multichain anvil is ready"
echo "ethereum  http://127.0.0.1:8545  chain_id=31337"
echo "arbitrum http://127.0.0.1:8546  chain_id=31338"
echo "base      http://127.0.0.1:8547  chain_id=31339"
