#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"

if [ -s "$CHAIN_ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  source "$CHAIN_ENV_FILE"
  set +a
fi

CHAIN_KEY="${1:-eth}"
BLOCKS="${2:-1}"

case "${CHAIN_KEY,,}" in
  eth|ethereum)
    RPC_URL="${ETH_RPC_URL_HOST:-http://127.0.0.1:8545}"
    ;;
  arb|arbitrum)
    RPC_URL="${ARB_RPC_URL_HOST:-http://127.0.0.1:8546}"
    ;;
  base)
    RPC_URL="${BASE_RPC_URL_HOST:-http://127.0.0.1:8547}"
    ;;
  *)
    echo "usage: $0 <eth|arb|base> [blocks]" >&2
    exit 1
    ;;
esac

if ! [[ "$BLOCKS" =~ ^[0-9]+$ ]] || [ "$BLOCKS" -le 0 ]; then
  echo "blocks must be a positive integer" >&2
  exit 1
fi

before="$(cast block-number --rpc-url "$RPC_URL")"
for _ in $(seq 1 "$BLOCKS"); do
  cast rpc --rpc-url "$RPC_URL" evm_mine >/dev/null
done
after="$(cast block-number --rpc-url "$RPC_URL")"

echo "rpc_url=${RPC_URL}"
echo "mined_blocks=${BLOCKS}"
echo "block_before=${before}"
echo "block_after=${after}"
