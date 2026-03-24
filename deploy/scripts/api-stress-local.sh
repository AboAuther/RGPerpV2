#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"
if [[ ! -f "$CHAIN_ENV_FILE" && -f "${ROOT_DIR}/.local/contracts.env" ]]; then
  CHAIN_ENV_FILE="${ROOT_DIR}/.local/contracts.env"
fi

if [[ -f "$CHAIN_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$CHAIN_ENV_FILE"
  set +a
fi

export API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"
export STRESS_PRIVATE_KEY="${STRESS_PRIVATE_KEY:-${USER_PRIVATE_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}}"
export STRESS_CHAIN_ID="${STRESS_CHAIN_ID:-${LOCAL_CHAIN_ID:-31337}}"

cd "${ROOT_DIR}/backend"
exec go run ./cmd/api-stress "$@"
