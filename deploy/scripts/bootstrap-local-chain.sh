#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
RPC_URL="${RPC_URL:-http://127.0.0.1:8545}"
OUTPUT_FILE="${OUTPUT_FILE:-$ROOT_DIR/.local/contracts.env}"
FRONTEND_ENV_FILE="${FRONTEND_ENV_FILE:-$ROOT_DIR/frontend/.env.local}"

mkdir -p "$(dirname "$OUTPUT_FILE")"
mkdir -p "$(dirname "$FRONTEND_ENV_FILE")"

export RPC_URL
export OUTPUT_FILE
export OUTPUT_HOST_RPC_URL="${OUTPUT_HOST_RPC_URL:-$RPC_URL}"
export OUTPUT_DOCKER_RPC_URL="${OUTPUT_DOCKER_RPC_URL:-http://host.docker.internal:8545}"

sh "$ROOT_DIR/deploy/scripts/deploy-local-contracts.sh"

set -a
# shellcheck disable=SC1090
. "$OUTPUT_FILE"
set +a

cat > "${FRONTEND_ENV_FILE}.tmp" <<EOF
VITE_APP_ENV=dev
VITE_API_BASE_URL=http://127.0.0.1:8080
VITE_WS_BASE_URL=ws://127.0.0.1:8080/ws
VITE_ADMIN_WALLETS=${ADMIN_WALLETS:-}
EOF

mv "${FRONTEND_ENV_FILE}.tmp" "$FRONTEND_ENV_FILE"
echo "frontend env wrote $FRONTEND_ENV_FILE"
