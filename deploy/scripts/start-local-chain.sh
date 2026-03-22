#!/bin/sh
set -eu

HOST="${HOST:-0.0.0.0}"
PORT="${PORT:-8545}"
CHAIN_ID="${CHAIN_ID:-31337}"
MNEMONIC="${MNEMONIC:-test test test test test test test test test test test junk}"

exec anvil --host "$HOST" --port "$PORT" --chain-id "$CHAIN_ID" --mnemonic "$MNEMONIC"
