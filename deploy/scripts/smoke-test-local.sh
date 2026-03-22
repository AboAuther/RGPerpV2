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

BASE_RPC_URL="${BASE_RPC_URL_HOST:-${BASE_RPC_URL:-http://127.0.0.1:8545}}"
if [[ "${BASE_RPC_URL}" == "http://host.docker.internal:8545" ]]; then
  BASE_RPC_URL="http://127.0.0.1:8545"
fi

: "${BASE_FACTORY_ADDRESS:?BASE_FACTORY_ADDRESS is required}"
: "${BASE_USDC_ADDRESS:?BASE_USDC_ADDRESS is required}"
: "${BASE_RPC_URL:?BASE_RPC_URL is required}"

USER_ADDRESS="$(cast wallet address --private-key "$USER_PRIVATE_KEY")"
RECIPIENT_ADDRESS="$(cast wallet address --private-key "$ADMIN_PRIVATE_KEY")"

json_field() {
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
  json_field "$1"
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

wait_for_balance() {
  local token="$1"
  local minimum="$2"
  for _ in $(seq 1 90); do
    local balances
    balances="$(request_authed_json GET '/api/v1/account/balances' "$token")"
    if printf '%s' "$balances" | python3 -c '
import json, sys
from decimal import Decimal

minimum = Decimal(sys.argv[1])
data = json.load(sys.stdin)["data"]
wallet = next((item for item in data if item["account_code"] == "USER_WALLET" and item["asset"] == "USDC"), None)
balance = Decimal(wallet["balance"]) if wallet else Decimal("0")
sys.exit(0 if balance >= minimum else 1)
' "$minimum"; then
      printf '%s\n' "$balances"
      return 0
    fi
    sleep 1
  done
  return 1
}

login() {
  local address="$1"
  local private_key="$2"
  local fingerprint="$3"
  local challenge
  challenge="$(request_json POST '/api/v1/auth/challenge' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID}}")"
  local nonce
  nonce="$(printf '%s' "$challenge" | extract_json 'data.nonce')"
  local message
  message="$(printf '%s' "$challenge" | extract_json 'data.message')"
  local signature
  signature="$(cast wallet sign --private-key "$private_key" "$message")"
  request_json POST '/api/v1/auth/login' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID},\"nonce\":\"${nonce}\",\"signature\":\"${signature}\",\"device_fingerprint\":\"${fingerprint}\"}"
}

current_user_balance() {
  local token="$1"
  request_authed_json GET '/api/v1/account/balances' "$token" | python3 -c '
import json, sys
from decimal import Decimal
data = json.load(sys.stdin)["data"]
wallet = next((item for item in data if item["account_code"] == "USER_WALLET" and item["asset"] == "USDC"), None)
print(Decimal(wallet["balance"]) if wallet else Decimal("0"))
'
}

wait_for_withdraw_status() {
  local token="$1"
  local withdraw_id="$2"
  local expected_status="$3"
  for _ in $(seq 1 120); do
    local payload
    payload="$(request_authed_json GET '/api/v1/wallet/withdrawals' "$token")"
    local matched
    matched="$(printf '%s' "$payload" | python3 -c '
import json, sys
withdraw_id = sys.argv[1]
expected = sys.argv[2]
items = json.load(sys.stdin)["data"]
item = next((x for x in items if x["withdraw_id"] == withdraw_id), None)
if not item:
    sys.exit(1)
if item["status"] != expected:
    sys.exit(1)
print(json.dumps(item))
' "$withdraw_id" "$expected_status" 2>/dev/null || true)"
    if [[ -n "$matched" ]]; then
      printf '%s\n' "$matched"
      return 0
    fi
    sleep 1
  done
  return 1
}

ensure_explorer_has_tx() {
  local token="$1"
  local tx_hash="$2"
  for _ in $(seq 1 60); do
    local events
    events="$(request_authed_json GET '/api/v1/explorer/events' "$token")"
    if printf '%s' "$events" | python3 -c '
import json, sys
tx_hash = sys.argv[1].lower()
items = json.load(sys.stdin)["data"]
matched = [item for item in items if str(item.get("chain_tx_hash", "")).lower() == tx_hash]
sys.exit(0 if matched else 1)
' "$tx_hash"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_api

LOGIN_PAYLOAD="$(login "$USER_ADDRESS" "$USER_PRIVATE_KEY" "smoke-test")"
ACCESS_TOKEN="$(printf '%s' "$LOGIN_PAYLOAD" | extract_json 'data.access_token')"
USER_ID="$(printf '%s' "$LOGIN_PAYLOAD" | extract_json 'data.user.id')"
INITIAL_BALANCE="$(current_user_balance "$ACCESS_TOKEN")"
RECIPIENT_BALANCE_BEFORE="$(cast call "$BASE_USDC_ADDRESS" 'balanceOf(address)(uint256)' "$RECIPIENT_ADDRESS" --rpc-url "$BASE_RPC_URL" | awk '{print $1}')"

EXPECTED_ROUTER_ADDRESS="$(cast call "$BASE_FACTORY_ADDRESS" "routerOfUser(uint256)(address)" "$USER_ID" --rpc-url "$BASE_RPC_URL")"
if [[ "${EXPECTED_ROUTER_ADDRESS,,}" == "0x0000000000000000000000000000000000000000" ]]; then
  cast send "$BASE_FACTORY_ADDRESS" "createRouter(uint256,bytes32)" "$USER_ID" "$(cast keccak "rgperp:user:${USER_ID}")" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
  EXPECTED_ROUTER_ADDRESS="$(cast call "$BASE_FACTORY_ADDRESS" "routerOfUser(uint256)(address)" "$USER_ID" --rpc-url "$BASE_RPC_URL")"
fi
GENERATE_PAYLOAD="$(request_authed_json POST "/api/v1/wallet/deposit-addresses/${LOCAL_CHAIN_ID}/generate" "$ACCESS_TOKEN")"
ROUTER_ADDRESS="$(printf '%s' "$GENERATE_PAYLOAD" | extract_json 'data.address')"
if [[ "${ROUTER_ADDRESS,,}" != "${EXPECTED_ROUTER_ADDRESS,,}" ]]; then
  echo "generated deposit address does not match routerOfUser: api=${ROUTER_ADDRESS} chain=${EXPECTED_ROUTER_ADDRESS}" >&2
  exit 1
fi

cast send "$BASE_USDC_ADDRESS" "mint(address,uint256)" "$ROUTER_ADDRESS" "15000000000" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
cast send "$ROUTER_ADDRESS" "forward()" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null

TARGET_BALANCE="$(python3 - "$INITIAL_BALANCE" <<'PY'
from decimal import Decimal
import sys
print(Decimal(sys.argv[1]) + Decimal("15000"))
PY
)"
BALANCES="$(wait_for_balance "$ACCESS_TOKEN" "$TARGET_BALANCE")"

WITHDRAW_RESPONSE="$(curl -fsS -X POST "${API_BASE_URL}/api/v1/wallet/withdrawals" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "X-Trace-Id: smoke_$(date +%s)" \
  -H "Idempotency-Key: smoke_withdraw_${USER_ID}_$(date +%s)" \
  -d "{\"chain_id\":${LOCAL_CHAIN_ID},\"asset\":\"USDC\",\"amount\":\"10\",\"to_address\":\"${RECIPIENT_ADDRESS}\"}")"
WITHDRAW_ID="$(printf '%s' "$WITHDRAW_RESPONSE" | extract_json 'data.withdraw_id')"
AUTO_WITHDRAW="$(wait_for_withdraw_status "$ACCESS_TOKEN" "$WITHDRAW_ID" "COMPLETED")"
AUTO_WITHDRAW_TX_HASH="$(printf '%s' "$AUTO_WITHDRAW" | json_field 'tx_hash')"
ensure_explorer_has_tx "$ACCESS_TOKEN" "$AUTO_WITHDRAW_TX_HASH"

ADMIN_LOGIN_PAYLOAD="$(login "$LOCAL_ANVIL_ADMIN_ADDRESS" "$ADMIN_PRIVATE_KEY" "smoke-admin")"
ADMIN_ACCESS_TOKEN="$(printf '%s' "$ADMIN_LOGIN_PAYLOAD" | extract_json 'data.access_token')"

RISK_WITHDRAW_RESPONSE="$(curl -fsS -X POST "${API_BASE_URL}/api/v1/wallet/withdrawals" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "X-Trace-Id: smoke_$(date +%s)" \
  -H "Idempotency-Key: smoke_withdraw_risk_${USER_ID}_$(date +%s)" \
  -d "{\"chain_id\":${LOCAL_CHAIN_ID},\"asset\":\"USDC\",\"amount\":\"10001\",\"to_address\":\"${RECIPIENT_ADDRESS}\"}")"
RISK_WITHDRAW_ID="$(printf '%s' "$RISK_WITHDRAW_RESPONSE" | extract_json 'data.withdraw_id')"
wait_for_withdraw_status "$ACCESS_TOKEN" "$RISK_WITHDRAW_ID" "RISK_REVIEW" >/dev/null
APPROVE_RESPONSE="$(request_authed_json POST "/api/v1/admin/withdrawals/${RISK_WITHDRAW_ID}/approve" "$ADMIN_ACCESS_TOKEN")"
REVIEW_WITHDRAW="$(wait_for_withdraw_status "$ACCESS_TOKEN" "$RISK_WITHDRAW_ID" "COMPLETED")"
REVIEW_WITHDRAW_TX_HASH="$(printf '%s' "$REVIEW_WITHDRAW" | json_field 'tx_hash')"
ensure_explorer_has_tx "$ACCESS_TOKEN" "$REVIEW_WITHDRAW_TX_HASH"
RECIPIENT_BALANCE_AFTER="$(cast call "$BASE_USDC_ADDRESS" 'balanceOf(address)(uint256)' "$RECIPIENT_ADDRESS" --rpc-url "$BASE_RPC_URL" | awk '{print $1}')"
RECIPIENT_DELTA="$(python3 - "$RECIPIENT_BALANCE_BEFORE" "$RECIPIENT_BALANCE_AFTER" <<'PY'
import sys
print(int(sys.argv[2]) - int(sys.argv[1]))
PY
)"
if [[ "$RECIPIENT_DELTA" != "10009000000" ]]; then
  echo "unexpected recipient balance delta: ${RECIPIENT_DELTA}" >&2
  exit 1
fi

echo "login ok, user_id=${USER_ID}"
echo "router=${ROUTER_ADDRESS}"
echo "balances=${BALANCES}"
echo "auto_withdraw=${AUTO_WITHDRAW}"
echo "risk_withdraw_approved=${APPROVE_RESPONSE}"
echo "review_withdraw=${REVIEW_WITHDRAW}"
echo "recipient_balance_before=${RECIPIENT_BALANCE_BEFORE}"
echo "recipient_balance_after=${RECIPIENT_BALANCE_AFTER}"
