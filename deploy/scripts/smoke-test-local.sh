#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/.local/contracts.env}"
API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
ADMIN_PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
USER_PRIVATE_KEY="${USER_PRIVATE_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"
LOCAL_CHAIN_ID="${LOCAL_CHAIN_ID:-31337}"

if [[ -f "$CHAIN_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$CHAIN_ENV_FILE"
  set +a
fi

if [[ "${BASE_RPC_URL:-}" == "http://anvil:8545" ]]; then
  BASE_RPC_URL="http://127.0.0.1:8545"
fi

: "${BASE_FACTORY_ADDRESS:?BASE_FACTORY_ADDRESS is required}"
: "${BASE_USDC_ADDRESS:?BASE_USDC_ADDRESS is required}"
: "${BASE_RPC_URL:?BASE_RPC_URL is required}"

USER_ADDRESS="$(cast wallet address --private-key "$USER_PRIVATE_KEY")"
RECIPIENT_ADDRESS="$(cast wallet address --private-key "$ADMIN_PRIVATE_KEY")"

request_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "${API_BASE_URL}${path}" -H 'Content-Type: application/json' -H "X-Trace-Id: smoke_$(date +%s)" -d "$body"
  else
    curl -fsS -X "$method" "${API_BASE_URL}${path}" -H "X-Trace-Id: smoke_$(date +%s)"
  fi
}

request_authed_json() {
  local method="$1"
  local path="$2"
  local token="$3"
  local body="${4:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "${API_BASE_URL}${path}" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${token}" \
      -H "X-Trace-Id: smoke_$(date +%s)" \
      -d "$body"
  else
    curl -fsS -X "$method" "${API_BASE_URL}${path}" \
      -H "Authorization: Bearer ${token}" \
      -H "X-Trace-Id: smoke_$(date +%s)"
  fi
}

extract_json() {
  local expr="$1"
  python3 -c '
import json
import sys

expr = sys.argv[1]
obj = json.load(sys.stdin)
value = obj
for part in expr.split("."):
    if not part:
        continue
    if part.isdigit():
        value = value[int(part)]
    else:
        value = value[part]
if isinstance(value, (dict, list)):
    print(json.dumps(value))
else:
    print(value)
' "$expr"
}

build_message() {
  local domain="$1"
  local chain_id="$2"
  local nonce="$3"
  printf 'RGPerp Login\nDomain: %s\nChain ID: %s\nNonce: %s' "$domain" "$chain_id" "$nonce"
}

wait_for_api() {
  for _ in $(seq 1 120); do
    if curl -fsS "${API_BASE_URL}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_router_address() {
  local token="$1"
  local expected="$2"
  local payload=""
  local current=""
  for _ in $(seq 1 60); do
    payload="$(request_authed_json GET '/api/v1/wallet/deposit-addresses' "$token")"
    current="$(printf '%s' "$payload" | python3 -c '
import json
import sys

expected = sys.argv[1].lower()
data = json.load(sys.stdin).get("data", [])
for item in data:
    address = str(item.get("address", ""))
    if address.lower() == expected:
        print(address)
        break
' "$expected")"
    if [[ "${current,,}" == "${expected,,}" ]]; then
      echo "$current"
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_balance() {
  local token="$1"
  for _ in $(seq 1 60); do
    local balances
    balances="$(request_authed_json GET '/api/v1/account/balances' "$token")"
    local wallet_balance
    wallet_balance="$(printf '%s' "$balances" | extract_json 'data.0.balance' || true)"
    if [[ -n "$wallet_balance" ]] && [[ "$wallet_balance" != "0" ]] && [[ "$wallet_balance" != "0.000000000000000000" ]]; then
      printf '%s\n' "$balances"
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_api

NONCE_PAYLOAD="$(request_json POST '/api/v1/auth/nonce' "{\"address\":\"${USER_ADDRESS}\",\"chain_id\":${LOCAL_CHAIN_ID}}")"
NONCE="$(printf '%s' "$NONCE_PAYLOAD" | extract_json 'data.nonce')"
DOMAIN="$(printf '%s' "$NONCE_PAYLOAD" | extract_json 'data.domain')"
MESSAGE="$(build_message "$DOMAIN" "${LOCAL_CHAIN_ID}" "$NONCE")"
SIGNATURE="$(cast wallet sign --private-key "$USER_PRIVATE_KEY" "$MESSAGE")"

LOGIN_PAYLOAD="$(request_json POST '/api/v1/auth/login' "{\"address\":\"${USER_ADDRESS}\",\"chain_id\":${LOCAL_CHAIN_ID},\"nonce\":\"${NONCE}\",\"signature\":\"${SIGNATURE}\",\"device_fingerprint\":\"smoke-test\"}")"
ACCESS_TOKEN="$(printf '%s' "$LOGIN_PAYLOAD" | extract_json 'data.access_token')"
USER_ID="$(printf '%s' "$LOGIN_PAYLOAD" | extract_json 'data.user.id')"

EXPECTED_ROUTER_ADDRESS="$(cast call "$BASE_FACTORY_ADDRESS" "routerOfUser(uint256)(address)" "$USER_ID" --rpc-url "$BASE_RPC_URL")"
if [[ "${EXPECTED_ROUTER_ADDRESS,,}" == "0x0000000000000000000000000000000000000000" ]]; then
  cast send "$BASE_FACTORY_ADDRESS" "createRouter(uint256,bytes32)" "$USER_ID" "$(cast keccak "rgperp:user:${USER_ID}")" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
  EXPECTED_ROUTER_ADDRESS="$(cast call "$BASE_FACTORY_ADDRESS" "routerOfUser(uint256)(address)" "$USER_ID" --rpc-url "$BASE_RPC_URL")"
fi
sleep 3
ROUTER_ADDRESS="$(wait_for_router_address "$ACCESS_TOKEN" "$EXPECTED_ROUTER_ADDRESS")"

cast send "$BASE_USDC_ADDRESS" "mint(address,uint256)" "$ROUTER_ADDRESS" "250000000" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
cast send "$ROUTER_ADDRESS" "forward()" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null

BALANCES="$(wait_for_balance "$ACCESS_TOKEN")"

WITHDRAW_RESPONSE="$(curl -fsS -X POST "${API_BASE_URL}/api/v1/wallet/withdrawals" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "X-Trace-Id: smoke_$(date +%s)" \
  -H "Idempotency-Key: smoke_withdraw_${USER_ID}_$(date +%s)" \
  -d "{\"chain_id\":${LOCAL_CHAIN_ID},\"asset\":\"USDC\",\"amount\":\"10\",\"to_address\":\"${RECIPIENT_ADDRESS}\"}")"
WITHDRAW_ID="$(printf '%s' "$WITHDRAW_RESPONSE" | extract_json 'data.withdraw_id')"

ADMIN_NONCE_PAYLOAD="$(request_json POST '/api/v1/auth/nonce' "{\"address\":\"${LOCAL_ANVIL_ADMIN_ADDRESS}\",\"chain_id\":${LOCAL_CHAIN_ID}}")"
ADMIN_NONCE="$(printf '%s' "$ADMIN_NONCE_PAYLOAD" | extract_json 'data.nonce')"
ADMIN_DOMAIN="$(printf '%s' "$ADMIN_NONCE_PAYLOAD" | extract_json 'data.domain')"
ADMIN_MESSAGE="$(build_message "$ADMIN_DOMAIN" "${LOCAL_CHAIN_ID}" "$ADMIN_NONCE")"
ADMIN_SIGNATURE="$(cast wallet sign --private-key "$ADMIN_PRIVATE_KEY" "$ADMIN_MESSAGE")"
ADMIN_LOGIN_PAYLOAD="$(request_json POST '/api/v1/auth/login' "{\"address\":\"${LOCAL_ANVIL_ADMIN_ADDRESS}\",\"chain_id\":${LOCAL_CHAIN_ID},\"nonce\":\"${ADMIN_NONCE}\",\"signature\":\"${ADMIN_SIGNATURE}\",\"device_fingerprint\":\"smoke-admin\"}")"
ADMIN_ACCESS_TOKEN="$(printf '%s' "$ADMIN_LOGIN_PAYLOAD" | extract_json 'data.access_token')"

APPROVE_RESPONSE="$(request_authed_json POST "/api/v1/admin/withdrawals/${WITHDRAW_ID}/approve" "$ADMIN_ACCESS_TOKEN")"

echo "login ok, user_id=${USER_ID}"
echo "router=${ROUTER_ADDRESS}"
echo "balances=${BALANCES}"
echo "withdraw_approved=${APPROVE_RESPONSE}"
