#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
RPC_URL="${RPC_URL:-http://anvil:8545}"
OUTPUT_FILE="${OUTPUT_FILE:-/shared/contracts.env}"
OUTPUT_HOST_RPC_URL="${OUTPUT_HOST_RPC_URL:-$RPC_URL}"
OUTPUT_DOCKER_RPC_URL="${OUTPUT_DOCKER_RPC_URL:-$OUTPUT_HOST_RPC_URL}"
PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
CHAIN_ENV_PREFIX="${CHAIN_ENV_PREFIX:-BASE}"
CHAIN_KEY="${CHAIN_KEY:-base}"
CHAIN_DISPLAY_NAME="${CHAIN_DISPLAY_NAME:-Local Anvil}"
CHAIN_CONFIG_CHAIN_ID="${CHAIN_CONFIG_CHAIN_ID:-31337}"
CHAIN_LOCAL_TESTNET="${CHAIN_LOCAL_TESTNET:-true}"

LAST_DEPLOY_ADDRESS=""
LAST_DEPLOY_TX_HASH=""

wait_for_rpc() {
  i=0
  while [ "$i" -lt 120 ]; do
    if cast block-number --rpc-url "$RPC_URL" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

contract_exists() {
  address="$1"
  if [ -z "$address" ]; then
    return 1
  fi
  code="$(cast code "$address" --rpc-url "$RPC_URL" 2>/dev/null || true)"
  [ -n "$code" ] && [ "$code" != "0x" ]
}

reuse_existing_contracts() {
  if [ ! -s "$OUTPUT_FILE" ]; then
    return 1
  fi
  set -a
  # shellcheck disable=SC1090
  . "$OUTPUT_FILE"
  set +a

  prefix="${CHAIN_ENV_PREFIX}"
  eval "usdc_address=\${${prefix}_USDC_ADDRESS:-}"
  eval "vault_address=\${${prefix}_VAULT_ADDRESS:-}"
  eval "factory_address=\${${prefix}_FACTORY_ADDRESS:-}"
  eval "deploy_block=\${${prefix}_DEPLOY_BLOCK:-}"
  eval "usdc_deploy_tx_hash=\${${prefix}_USDC_DEPLOY_TX_HASH:-}"
  eval "vault_deploy_tx_hash=\${${prefix}_VAULT_DEPLOY_TX_HASH:-}"
  eval "factory_deploy_tx_hash=\${${prefix}_FACTORY_DEPLOY_TX_HASH:-}"

  if contract_exists "${usdc_address:-}" && contract_exists "${vault_address:-}" && contract_exists "${factory_address:-}"; then
    cat > "${OUTPUT_FILE}.tmp" <<EOF
export ${CHAIN_ENV_PREFIX}_ENABLED=true
export ${CHAIN_ENV_PREFIX}_CHAIN_ID=${CHAIN_CONFIG_CHAIN_ID}
export ${CHAIN_ENV_PREFIX}_DISPLAY_NAME="${CHAIN_DISPLAY_NAME}"
export ${CHAIN_ENV_PREFIX}_LOCAL_TESTNET=${CHAIN_LOCAL_TESTNET}
export ${CHAIN_ENV_PREFIX}_RPC_URL=$OUTPUT_HOST_RPC_URL
export ${CHAIN_ENV_PREFIX}_RPC_URL_HOST=$OUTPUT_HOST_RPC_URL
export ${CHAIN_ENV_PREFIX}_RPC_URL_DOCKER=$OUTPUT_DOCKER_RPC_URL
export ${CHAIN_ENV_PREFIX}_CONFIRMATIONS=1
export ${CHAIN_ENV_PREFIX}_USDC_ADDRESS=${usdc_address}
export ${CHAIN_ENV_PREFIX}_VAULT_ADDRESS=${vault_address}
export ${CHAIN_ENV_PREFIX}_FACTORY_ADDRESS=${factory_address}
export ${CHAIN_ENV_PREFIX}_DEPLOY_BLOCK=${deploy_block:-}
export ${CHAIN_ENV_PREFIX}_USDC_DEPLOY_TX_HASH=${usdc_deploy_tx_hash:-}
export ${CHAIN_ENV_PREFIX}_VAULT_DEPLOY_TX_HASH=${vault_deploy_tx_hash:-}
export ${CHAIN_ENV_PREFIX}_FACTORY_DEPLOY_TX_HASH=${factory_deploy_tx_hash:-}
export ADMIN_WALLETS=${ADMIN_WALLETS:-$ADMIN_ADDRESS}
export LOCAL_ANVIL_ADMIN_ADDRESS=${LOCAL_ANVIL_ADMIN_ADDRESS:-$ADMIN_ADDRESS}
export LOCAL_ANVIL_ADMIN_PRIVATE_KEY=$PRIVATE_KEY
EOF
    mv "${OUTPUT_FILE}.tmp" "$OUTPUT_FILE"
    echo "reused existing contracts from $OUTPUT_FILE"
    return 0
  fi
  return 1
}

deploy_contract() {
  contract="$1"
  shift
  output="$(forge create "$contract" --broadcast --rpc-url "$RPC_URL" --private-key "$PRIVATE_KEY" "$@")"
  echo "$output" >&2
  LAST_DEPLOY_ADDRESS="$(echo "$output" | awk '/Deployed to: /{print $3}' | tail -n 1)"
  LAST_DEPLOY_TX_HASH="$(echo "$output" | awk '/Transaction hash: /{print $3}' | tail -n 1)"
  if [ -z "$LAST_DEPLOY_ADDRESS" ]; then
    echo "failed to parse deployed address for $contract" >&2
    exit 1
  fi
}

mkdir -p "$(dirname "$OUTPUT_FILE")"

echo "waiting for anvil at $RPC_URL"
wait_for_rpc

cd "$ROOT_DIR/contracts"
forge build >/dev/null

ADMIN_ADDRESS="$(cast wallet address --private-key "$PRIVATE_KEY")"

if reuse_existing_contracts; then
  exit 0
fi

deploy_contract src/MockUSDC.sol:MockUSDC
MOCK_USDC_ADDRESS="$LAST_DEPLOY_ADDRESS"
USDC_DEPLOY_TX_HASH="$LAST_DEPLOY_TX_HASH"
deploy_contract src/Vault.sol:Vault --constructor-args "$ADMIN_ADDRESS"
VAULT_ADDRESS="$LAST_DEPLOY_ADDRESS"
VAULT_DEPLOY_TX_HASH="$LAST_DEPLOY_TX_HASH"
deploy_contract src/DepositRouterFactory.sol:DepositRouterFactory --constructor-args "$ADMIN_ADDRESS" "$VAULT_ADDRESS" "$MOCK_USDC_ADDRESS"
FACTORY_ADDRESS="$LAST_DEPLOY_ADDRESS"
FACTORY_DEPLOY_TX_HASH="$LAST_DEPLOY_TX_HASH"
DEPLOY_BLOCK="$(cast block-number --rpc-url "$RPC_URL")"

cast send "$VAULT_ADDRESS" "setTokenAllowed(address,bool)" "$MOCK_USDC_ADDRESS" true --rpc-url "$RPC_URL" --private-key "$PRIVATE_KEY" >/dev/null
# Intentionally do not pre-fund the vault here.
# Local test scripts may mint to user/router explicitly, but bootstrap must not create
# hidden platform assets that bypass the ledger model.

cat > "${OUTPUT_FILE}.tmp" <<EOF
export ${CHAIN_ENV_PREFIX}_ENABLED=true
export ${CHAIN_ENV_PREFIX}_CHAIN_ID=${CHAIN_CONFIG_CHAIN_ID}
export ${CHAIN_ENV_PREFIX}_DISPLAY_NAME="${CHAIN_DISPLAY_NAME}"
export ${CHAIN_ENV_PREFIX}_LOCAL_TESTNET=${CHAIN_LOCAL_TESTNET}
export ${CHAIN_ENV_PREFIX}_RPC_URL=$OUTPUT_HOST_RPC_URL
export ${CHAIN_ENV_PREFIX}_RPC_URL_HOST=$OUTPUT_HOST_RPC_URL
export ${CHAIN_ENV_PREFIX}_RPC_URL_DOCKER=$OUTPUT_DOCKER_RPC_URL
export ${CHAIN_ENV_PREFIX}_CONFIRMATIONS=1
export ${CHAIN_ENV_PREFIX}_USDC_ADDRESS=$MOCK_USDC_ADDRESS
export ${CHAIN_ENV_PREFIX}_VAULT_ADDRESS=$VAULT_ADDRESS
export ${CHAIN_ENV_PREFIX}_FACTORY_ADDRESS=$FACTORY_ADDRESS
export ${CHAIN_ENV_PREFIX}_DEPLOY_BLOCK=$DEPLOY_BLOCK
export ${CHAIN_ENV_PREFIX}_USDC_DEPLOY_TX_HASH=$USDC_DEPLOY_TX_HASH
export ${CHAIN_ENV_PREFIX}_VAULT_DEPLOY_TX_HASH=$VAULT_DEPLOY_TX_HASH
export ${CHAIN_ENV_PREFIX}_FACTORY_DEPLOY_TX_HASH=$FACTORY_DEPLOY_TX_HASH
export ADMIN_WALLETS=$ADMIN_ADDRESS
export LOCAL_ANVIL_ADMIN_ADDRESS=$ADMIN_ADDRESS
export LOCAL_ANVIL_ADMIN_PRIVATE_KEY=$PRIVATE_KEY
EOF

mv "${OUTPUT_FILE}.tmp" "$OUTPUT_FILE"
echo "contracts deployed and wrote $OUTPUT_FILE"
