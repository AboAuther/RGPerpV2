#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_FILE="${OUTPUT_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"
MANIFEST_DIR="${MANIFEST_DIR:-${ROOT_DIR}/.local/manifest/dev}"
ADMIN_PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$(dirname "$OUTPUT_FILE")" "$MANIFEST_DIR"

ETH_HOST_RPC_URL="${ETH_HOST_RPC_URL:-http://127.0.0.1:8545}"
ETH_DEPLOY_RPC_URL="${ETH_DEPLOY_RPC_URL:-${ETH_HOST_RPC_URL}}"
ETH_DOCKER_RPC_URL="${ETH_DOCKER_RPC_URL:-http://host.docker.internal:8545}"

ARB_HOST_RPC_URL="${ARB_HOST_RPC_URL:-http://127.0.0.1:8546}"
ARB_DEPLOY_RPC_URL="${ARB_DEPLOY_RPC_URL:-${ARB_HOST_RPC_URL}}"
ARB_DOCKER_RPC_URL="${ARB_DOCKER_RPC_URL:-http://host.docker.internal:8546}"

BASE_HOST_RPC_URL="${BASE_HOST_RPC_URL:-http://127.0.0.1:8547}"
BASE_DEPLOY_RPC_URL="${BASE_DEPLOY_RPC_URL:-${BASE_HOST_RPC_URL}}"
BASE_DOCKER_RPC_URL="${BASE_DOCKER_RPC_URL:-http://host.docker.internal:8547}"

deploy_chain() {
  local prefix="$1"
  local key="$2"
  local display_name="$3"
  local chain_id="$4"
  local deploy_rpc_url="$5"
  local host_rpc_url="$6"
  local docker_rpc_url="$7"
  local chain_env_file="${TMP_DIR}/${prefix}.env"
  local manifest_file="${MANIFEST_DIR}/${key}.json"

  RPC_URL="$deploy_rpc_url" \
  OUTPUT_FILE="$chain_env_file" \
  OUTPUT_HOST_RPC_URL="$host_rpc_url" \
  OUTPUT_DOCKER_RPC_URL="$docker_rpc_url" \
  ADMIN_PRIVATE_KEY="$ADMIN_PRIVATE_KEY" \
  CHAIN_ENV_PREFIX="$prefix" \
  CHAIN_KEY="$key" \
  CHAIN_DISPLAY_NAME="$display_name" \
  CHAIN_CONFIG_CHAIN_ID="$chain_id" \
  CHAIN_LOCAL_TESTNET=true \
  sh "${ROOT_DIR}/deploy/scripts/deploy-local-contracts.sh"

  set -a
  # shellcheck disable=SC1090
  source "$chain_env_file"
  set +a

  local token_address vault_address factory_address deploy_block deploy_tx_hash admin_address created_at
  token_address="$(eval "printf '%s' \"\${${prefix}_USDC_ADDRESS}\"")"
  vault_address="$(eval "printf '%s' \"\${${prefix}_VAULT_ADDRESS}\"")"
  factory_address="$(eval "printf '%s' \"\${${prefix}_FACTORY_ADDRESS}\"")"
  deploy_block="$(eval "printf '%s' \"\${${prefix}_DEPLOY_BLOCK:-}\"")"
  deploy_tx_hash="$(eval "printf '%s' \"\${${prefix}_FACTORY_DEPLOY_TX_HASH:-}\"")"
  admin_address="${LOCAL_ANVIL_ADMIN_ADDRESS}"
  created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  python3 - "$manifest_file" "$key" "$chain_id" "$display_name" "$host_rpc_url" "$token_address" "$vault_address" "$factory_address" "$deploy_block" "$deploy_tx_hash" "$admin_address" "$created_at" <<'PY'
import json
import sys

path, chain_key, chain_id, display_name, rpc_url, token_address, vault_address, factory_address, deploy_block, deploy_tx_hash, admin_address, created_at = sys.argv[1:]

payload = {
    "env": "dev",
    "chain_id": int(chain_id),
    "chain_key": chain_key,
    "display_name": display_name,
    "rpc_url": rpc_url,
    "token_address": token_address,
    "vault_address": vault_address,
    "factory_address": factory_address,
    "deploy_block": int(deploy_block) if deploy_block else None,
    "deploy_tx_hash": deploy_tx_hash or None,
    "admin_address": admin_address,
    "roles": {
        "admin": admin_address,
        "withdraw_signer": admin_address,
        "hot_wallet": admin_address,
        "warm_wallet": admin_address,
        "cold_wallet": admin_address,
    },
    "created_at": created_at,
}

with open(path, "w", encoding="utf-8") as fh:
    json.dump(payload, fh, ensure_ascii=True, indent=2)
    fh.write("\n")
PY
}

deploy_chain ETH ethereum "Local Ethereum" 31337 "$ETH_DEPLOY_RPC_URL" "$ETH_HOST_RPC_URL" "$ETH_DOCKER_RPC_URL"
deploy_chain ARB arbitrum "Local Arbitrum" 31338 "$ARB_DEPLOY_RPC_URL" "$ARB_HOST_RPC_URL" "$ARB_DOCKER_RPC_URL"
deploy_chain BASE base "Local Base" 31339 "$BASE_DEPLOY_RPC_URL" "$BASE_HOST_RPC_URL" "$BASE_DOCKER_RPC_URL"

cat > "${OUTPUT_FILE}.tmp" <<EOF
# RGPerp local multichain config
# Canonical local chain startup config consumed by backend/services/scripts.
# Update this file when you want to change local chain ids, rpc urls,
# confirmations, or deployed contract addresses for integration testing.

# Ethereum
export ETH_ENABLED=true
export ETH_CHAIN_ID=${ETH_CHAIN_ID}
export ETH_DISPLAY_NAME="${ETH_DISPLAY_NAME}"
export ETH_LOCAL_TESTNET=${ETH_LOCAL_TESTNET}
export ETH_RPC_URL=${ETH_RPC_URL}
export ETH_RPC_URL_HOST=${ETH_RPC_URL_HOST}
export ETH_RPC_URL_DOCKER=${ETH_RPC_URL_DOCKER}
export ETH_CONFIRMATIONS=${ETH_CONFIRMATIONS}
export ETH_USDC_ADDRESS=${ETH_USDC_ADDRESS}
export ETH_VAULT_ADDRESS=${ETH_VAULT_ADDRESS}
export ETH_FACTORY_ADDRESS=${ETH_FACTORY_ADDRESS}
export ETH_DEPLOY_BLOCK=${ETH_DEPLOY_BLOCK:-}
export ETH_USDC_DEPLOY_TX_HASH=${ETH_USDC_DEPLOY_TX_HASH:-}
export ETH_VAULT_DEPLOY_TX_HASH=${ETH_VAULT_DEPLOY_TX_HASH:-}
export ETH_FACTORY_DEPLOY_TX_HASH=${ETH_FACTORY_DEPLOY_TX_HASH:-}

# Arbitrum
export ARB_ENABLED=true
export ARB_CHAIN_ID=${ARB_CHAIN_ID}
export ARB_DISPLAY_NAME="${ARB_DISPLAY_NAME}"
export ARB_LOCAL_TESTNET=${ARB_LOCAL_TESTNET}
export ARB_RPC_URL=${ARB_RPC_URL}
export ARB_RPC_URL_HOST=${ARB_RPC_URL_HOST}
export ARB_RPC_URL_DOCKER=${ARB_RPC_URL_DOCKER}
export ARB_CONFIRMATIONS=${ARB_CONFIRMATIONS}
export ARB_USDC_ADDRESS=${ARB_USDC_ADDRESS}
export ARB_VAULT_ADDRESS=${ARB_VAULT_ADDRESS}
export ARB_FACTORY_ADDRESS=${ARB_FACTORY_ADDRESS}
export ARB_DEPLOY_BLOCK=${ARB_DEPLOY_BLOCK:-}
export ARB_USDC_DEPLOY_TX_HASH=${ARB_USDC_DEPLOY_TX_HASH:-}
export ARB_VAULT_DEPLOY_TX_HASH=${ARB_VAULT_DEPLOY_TX_HASH:-}
export ARB_FACTORY_DEPLOY_TX_HASH=${ARB_FACTORY_DEPLOY_TX_HASH:-}

# Base
export BASE_ENABLED=true
export BASE_CHAIN_ID=${BASE_CHAIN_ID}
export BASE_DISPLAY_NAME="${BASE_DISPLAY_NAME}"
export BASE_LOCAL_TESTNET=${BASE_LOCAL_TESTNET}
export BASE_RPC_URL=${BASE_RPC_URL}
export BASE_RPC_URL_HOST=${BASE_RPC_URL_HOST}
export BASE_RPC_URL_DOCKER=${BASE_RPC_URL_DOCKER}
export BASE_CONFIRMATIONS=${BASE_CONFIRMATIONS}
export BASE_USDC_ADDRESS=${BASE_USDC_ADDRESS}
export BASE_VAULT_ADDRESS=${BASE_VAULT_ADDRESS}
export BASE_FACTORY_ADDRESS=${BASE_FACTORY_ADDRESS}
export BASE_DEPLOY_BLOCK=${BASE_DEPLOY_BLOCK:-}
export BASE_USDC_DEPLOY_TX_HASH=${BASE_USDC_DEPLOY_TX_HASH:-}
export BASE_VAULT_DEPLOY_TX_HASH=${BASE_VAULT_DEPLOY_TX_HASH:-}
export BASE_FACTORY_DEPLOY_TX_HASH=${BASE_FACTORY_DEPLOY_TX_HASH:-}

# Shared local dev accounts
export ADMIN_WALLETS=${ADMIN_WALLETS}
export LOCAL_ANVIL_ADMIN_ADDRESS=${LOCAL_ANVIL_ADMIN_ADDRESS}
export LOCAL_ANVIL_ADMIN_PRIVATE_KEY=${LOCAL_ANVIL_ADMIN_PRIVATE_KEY}
export LOCAL_MULTICHAIN_MANIFEST_DIR=${MANIFEST_DIR}
EOF

mv "${OUTPUT_FILE}.tmp" "$OUTPUT_FILE"
echo "multichain contracts ready and wrote ${OUTPUT_FILE}"
echo "manifest dir: ${MANIFEST_DIR}"
