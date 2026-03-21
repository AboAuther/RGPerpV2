#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
RPC_URL="${RPC_URL:-http://127.0.0.1:8545}"
OUTPUT_FILE="${OUTPUT_FILE:-$ROOT_DIR/.local/contracts.env}"

mkdir -p "$(dirname "$OUTPUT_FILE")"

export RPC_URL
export OUTPUT_FILE
export OUTPUT_HOST_RPC_URL="${OUTPUT_HOST_RPC_URL:-$RPC_URL}"
export OUTPUT_DOCKER_RPC_URL="${OUTPUT_DOCKER_RPC_URL:-http://host.docker.internal:8545}"

exec sh "$ROOT_DIR/deploy/scripts/deploy-local-contracts.sh"
