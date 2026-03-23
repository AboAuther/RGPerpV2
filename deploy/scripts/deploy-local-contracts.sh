#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
RPC_URL="${RPC_URL:-http://anvil:8545}"
OUTPUT_FILE="${OUTPUT_FILE:-/shared/contracts.env}"
OUTPUT_HOST_RPC_URL="${OUTPUT_HOST_RPC_URL:-$RPC_URL}"
OUTPUT_DOCKER_RPC_URL="${OUTPUT_DOCKER_RPC_URL:-$OUTPUT_HOST_RPC_URL}"
PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"

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

  if contract_exists "${BASE_USDC_ADDRESS:-}" && contract_exists "${BASE_VAULT_ADDRESS:-}" && contract_exists "${BASE_FACTORY_ADDRESS:-}"; then
    cat > "${OUTPUT_FILE}.tmp" <<EOF
export BASE_RPC_URL=$OUTPUT_HOST_RPC_URL
export BASE_RPC_URL_HOST=$OUTPUT_HOST_RPC_URL
export BASE_RPC_URL_DOCKER=$OUTPUT_DOCKER_RPC_URL
export BASE_CONFIRMATIONS=1
export BASE_USDC_ADDRESS=${BASE_USDC_ADDRESS}
export BASE_VAULT_ADDRESS=${BASE_VAULT_ADDRESS}
export BASE_FACTORY_ADDRESS=${BASE_FACTORY_ADDRESS}
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
  echo "$output" | awk '/Deployed to: /{print $3}' | tail -n 1
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

MOCK_USDC_ADDRESS="$(deploy_contract src/MockUSDC.sol:MockUSDC)"
VAULT_ADDRESS="$(deploy_contract src/Vault.sol:Vault --constructor-args "$ADMIN_ADDRESS")"
FACTORY_ADDRESS="$(deploy_contract src/DepositRouterFactory.sol:DepositRouterFactory --constructor-args "$ADMIN_ADDRESS" "$VAULT_ADDRESS" "$MOCK_USDC_ADDRESS")"

cast send "$VAULT_ADDRESS" "setTokenAllowed(address,bool)" "$MOCK_USDC_ADDRESS" true --rpc-url "$RPC_URL" --private-key "$PRIVATE_KEY" >/dev/null
# Intentionally do not pre-fund the vault here.
# Local test scripts may mint to user/router explicitly, but bootstrap must not create
# hidden platform assets that bypass the ledger model.

cat > "${OUTPUT_FILE}.tmp" <<EOF
export BASE_RPC_URL=$OUTPUT_HOST_RPC_URL
export BASE_RPC_URL_HOST=$OUTPUT_HOST_RPC_URL
export BASE_RPC_URL_DOCKER=$OUTPUT_DOCKER_RPC_URL
export BASE_CONFIRMATIONS=1
export BASE_USDC_ADDRESS=$MOCK_USDC_ADDRESS
export BASE_VAULT_ADDRESS=$VAULT_ADDRESS
export BASE_FACTORY_ADDRESS=$FACTORY_ADDRESS
export ADMIN_WALLETS=$ADMIN_ADDRESS
export LOCAL_ANVIL_ADMIN_ADDRESS=$ADMIN_ADDRESS
export LOCAL_ANVIL_ADMIN_PRIVATE_KEY=$PRIVATE_KEY
EOF

mv "${OUTPUT_FILE}.tmp" "$OUTPUT_FILE"
echo "contracts deployed and wrote $OUTPUT_FILE"
