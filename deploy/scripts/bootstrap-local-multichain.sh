#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_FILE="${OUTPUT_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"
FRONTEND_ENV_FILE="${FRONTEND_ENV_FILE:-${ROOT_DIR}/frontend/.env.local}"
MANIFEST_DIR="${MANIFEST_DIR:-${ROOT_DIR}/.local/manifest/dev}"

mkdir -p "$(dirname "$OUTPUT_FILE")" "$(dirname "$FRONTEND_ENV_FILE")" "$MANIFEST_DIR"

echo "starting local multichain anvil"
bash "${ROOT_DIR}/deploy/scripts/start-local-multichain.sh"

OUTPUT_FILE="$OUTPUT_FILE" MANIFEST_DIR="$MANIFEST_DIR" bash "${ROOT_DIR}/deploy/scripts/deploy-multichain-contracts.sh"

set -a
# shellcheck disable=SC1090
source "$OUTPUT_FILE"
set +a

cat > "${FRONTEND_ENV_FILE}.tmp" <<EOF
VITE_APP_ENV=dev
VITE_API_BASE_URL=http://127.0.0.1:8080
VITE_WS_BASE_URL=ws://127.0.0.1:8080/ws
VITE_ADMIN_WALLETS=${ADMIN_WALLETS:-}
EOF

mv "${FRONTEND_ENV_FILE}.tmp" "$FRONTEND_ENV_FILE"
echo "frontend env wrote ${FRONTEND_ENV_FILE}"
