#!/bin/sh
set -eu

HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-8545}"
CHAIN_ID="${CHAIN_ID:-31337}"

exec anvil --host "$HOST" --port "$PORT" --chain-id "$CHAIN_ID"
