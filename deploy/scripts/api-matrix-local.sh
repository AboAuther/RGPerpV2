#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/.local/contracts.env}"
API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"
USER_PRIVATE_KEY="${USER_PRIVATE_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"
LOCAL_CHAIN_ID="${LOCAL_CHAIN_ID:-31337}"

if [[ -f "$CHAIN_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$CHAIN_ENV_FILE"
  set +a
fi

BASE_RPC_URL="${BASE_RPC_URL_HOST:-${BASE_RPC_URL:-http://127.0.0.1:8545}}"
if [[ "${BASE_RPC_URL}" == "http://host.docker.internal:8545" ]]; then
  BASE_RPC_URL="http://127.0.0.1:8545"
fi

: "${LOCAL_ANVIL_ADMIN_PRIVATE_KEY:?LOCAL_ANVIL_ADMIN_PRIVATE_KEY is required}"
: "${LOCAL_ANVIL_ADMIN_ADDRESS:?LOCAL_ANVIL_ADMIN_ADDRESS is required}"
: "${BASE_USDC_ADDRESS:?BASE_USDC_ADDRESS is required}"
: "${BASE_RPC_URL:?BASE_RPC_URL is required}"

USER_ADDRESS="$(cast wallet address --private-key "$USER_PRIVATE_KEY")"

request_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  shift 3 || true

  local args=(
    -sS
    -X "$method"
    "${API_BASE_URL}${path}"
    -H 'Accept: application/json'
  )

  while [[ $# -gt 0 ]]; do
    args+=(-H "$1")
    shift
  done

  if [[ -n "$body" ]]; then
    args+=(-H 'Content-Type: application/json' --data "$body")
  fi

  curl "${args[@]}"
}

build_login_message() {
  local domain="$1"
  local chain_id="$2"
  local nonce="$3"
  printf 'RGPerp Login\nDomain: %s\nChain ID: %s\nNonce: %s' "$domain" "$chain_id" "$nonce"
}

login_and_get_token() {
  local address="$1"
  local private_key="$2"
  local nonce_payload nonce domain message signature login_payload

  nonce_payload="$(request_json POST '/api/v1/auth/challenge' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID}}" -H "X-Trace-Id: matrix_nonce_$(date +%s)")"
  nonce="$(printf '%s' "$nonce_payload" | jq -r '.data.nonce')"
  domain="$(printf '%s' "$nonce_payload" | jq -r '.data.domain')"
  message="$(printf '%s' "$nonce_payload" | jq -r '.data.message')"
  signature="$(cast wallet sign --private-key "$private_key" "$message")"
  login_payload="$(request_json POST '/api/v1/auth/login' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID},\"nonce\":\"${nonce}\",\"signature\":\"${signature}\"}" -H "X-Trace-Id: matrix_login_$(date +%s)")"
  printf '%s' "$login_payload" | jq -r '.data.access_token'
}

assert_ok() {
  local label="$1"
  local payload="$2"
  local code
  code="$(printf '%s' "$payload" | jq -r '.code')"
  if [[ "$code" != "OK" ]]; then
    echo "[FAIL] ${label}: ${payload}" >&2
    exit 1
  fi
  echo "[OK] ${label}"
}

wait_for_credited_deposit() {
  local token="$1"
  local previous_tx_hash="$2"

  for _ in $(seq 1 30); do
    local payload latest_hash latest_status
    payload="$(request_json GET '/api/v1/wallet/deposits' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: matrix_poll_$(date +%s)")"
    latest_hash="$(printf '%s' "$payload" | jq -r '.data[0].tx_hash // ""')"
    latest_status="$(printf '%s' "$payload" | jq -r '.data[0].status // ""')"
    if [[ "$latest_hash" != "$previous_tx_hash" && "$latest_status" == "CREDITED" ]]; then
      printf '%s' "$payload"
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for credited deposit" >&2
  exit 1
}

USER_TOKEN="$(login_and_get_token "$USER_ADDRESS" "$USER_PRIVATE_KEY")"
ADMIN_TOKEN="$(login_and_get_token "$LOCAL_ANVIL_ADMIN_ADDRESS" "$LOCAL_ANVIL_ADMIN_PRIVATE_KEY")"

for path in \
  /api/v1/account/summary \
  /api/v1/account/balances \
  /api/v1/account/risk \
  /api/v1/account/funding \
  /api/v1/account/transfers \
  /api/v1/wallet/deposit-addresses \
  /api/v1/wallet/deposits \
  /api/v1/wallet/withdrawals \
  /api/v1/markets/symbols \
  /api/v1/markets/tickers \
  /api/v1/orders \
  /api/v1/fills \
  /api/v1/positions \
  /api/v1/explorer/events
do
  payload="$(request_json GET "$path" '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: matrix_get_$(date +%s)")"
  assert_ok "$path" "$payload"
done

deposit_addresses_payload="$(request_json POST "/api/v1/wallet/deposit-addresses/${LOCAL_CHAIN_ID}/generate" '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: matrix_deposit_addresses_$(date +%s)")"
assert_ok '/api/v1/wallet/deposit-addresses/:chainId/generate' "$deposit_addresses_payload"
deposit_address="$(printf '%s' "$deposit_addresses_payload" | jq -r '.data.address')"

deposits_before_payload="$(request_json GET '/api/v1/wallet/deposits' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: matrix_deposits_before_$(date +%s)")"
previous_tx_hash="$(printf '%s' "$deposits_before_payload" | jq -r '.data[0].tx_hash // ""')"

cast send "$BASE_USDC_ADDRESS" "mint(address,uint256)" "$deposit_address" "20000000000" --rpc-url "$BASE_RPC_URL" --private-key "$LOCAL_ANVIL_ADMIN_PRIVATE_KEY" >/dev/null
cast send "$deposit_address" "forward()" --rpc-url "$BASE_RPC_URL" --private-key "$LOCAL_ANVIL_ADMIN_PRIVATE_KEY" >/dev/null

credited_deposit_payload="$(wait_for_credited_deposit "$USER_TOKEN" "$previous_tx_hash")"
assert_ok 'credited deposit after mint' "$credited_deposit_payload"

withdraw_payload="$(request_json POST '/api/v1/wallet/withdrawals' "{\"chain_id\":${LOCAL_CHAIN_ID},\"asset\":\"USDC\",\"amount\":\"10001\",\"to_address\":\"0x0000000000000000000000000000000000000001\"}" -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: matrix_withdraw_$(date +%s)" -H "Idempotency-Key: matrix_withdraw_${LOCAL_CHAIN_ID}_$(date +%s)")"
assert_ok '/api/v1/wallet/withdrawals' "$withdraw_payload"
withdraw_id="$(printf '%s' "$withdraw_payload" | jq -r '.data.withdraw_id')"

approve_payload="$(request_json POST "/api/v1/admin/withdrawals/${withdraw_id}/approve" '' -H "Authorization: Bearer ${ADMIN_TOKEN}" -H "X-Trace-Id: matrix_approve_$(date +%s)")"
assert_ok '/api/v1/admin/withdrawals/:withdrawId/approve' "$approve_payload"

echo
echo "api matrix passed"
echo "deposit_address=${deposit_address}"
echo "withdraw_id=${withdraw_id}"
