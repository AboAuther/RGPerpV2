#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"
if [[ ! -f "$CHAIN_ENV_FILE" && -f "${ROOT_DIR}/.local/contracts.env" ]]; then
  CHAIN_ENV_FILE="${ROOT_DIR}/.local/contracts.env"
fi
API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
ADMIN_PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
USER_PRIVATE_KEY="${USER_PRIVATE_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"

if [[ ! -f "$CHAIN_ENV_FILE" ]]; then
  echo "missing chain env file: ${CHAIN_ENV_FILE}" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "$CHAIN_ENV_FILE"
set +a

LOGIN_CHAIN_ID="${LOGIN_CHAIN_ID:-${ETH_CHAIN_ID:-31337}}"
USER_ADDRESS="$(cast wallet address --private-key "$USER_PRIVATE_KEY")"

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
  shift 3 || true
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "${API_BASE_URL}${path}" -H 'Content-Type: application/json' -H "X-Trace-Id: multichain_$(date +%s)" "$@" -d "$body"
  else
    curl -fsS -X "$method" "${API_BASE_URL}${path}" -H "X-Trace-Id: multichain_$(date +%s)" "$@"
  fi
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

login() {
  local address="$1"
  local private_key="$2"
  local fingerprint="$3"
  local challenge nonce message signature
  challenge="$(request_json POST '/api/v1/auth/challenge' "{\"address\":\"${address}\",\"chain_id\":${LOGIN_CHAIN_ID}}")"
  nonce="$(printf '%s' "$challenge" | json_field 'data.nonce')"
  message="$(printf '%s' "$challenge" | json_field 'data.message')"
  signature="$(cast wallet sign --private-key "$private_key" "$message")"
  request_json POST '/api/v1/auth/login' "{\"address\":\"${address}\",\"chain_id\":${LOGIN_CHAIN_ID},\"nonce\":\"${nonce}\",\"signature\":\"${signature}\",\"device_fingerprint\":\"${fingerprint}\"}"
}

current_user_balance() {
  local token="$1"
  request_json GET '/api/v1/account/balances' '' -H "Authorization: Bearer ${token}" | python3 -c '
import json, sys
from decimal import Decimal
data = json.load(sys.stdin)["data"]
wallet = next((item for item in data if item["account_code"] == "USER_WALLET" and item["asset"] == "USDC"), None)
print(Decimal(wallet["balance"]) if wallet else Decimal("0"))
'
}

wait_for_balance() {
  local token="$1"
  local minimum="$2"
  for _ in $(seq 1 90); do
    local balances
    balances="$(request_json GET '/api/v1/account/balances' '' -H "Authorization: Bearer ${token}")"
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

wait_for_credited_deposit() {
  local token="$1"
  local chain_id="$2"
  local address="$3"
  local amount="$4"
  local tx_hash="$5"
  for _ in $(seq 1 120); do
    local payload matched
    payload="$(request_json GET '/api/v1/wallet/deposits' '' -H "Authorization: Bearer ${token}")"
    matched="$(printf '%s' "$payload" | python3 -c '
import json, sys
from decimal import Decimal

chain_id = int(sys.argv[1])
address = sys.argv[2].lower()
amount = Decimal(sys.argv[3])
tx_hash = sys.argv[4].lower()
items = json.load(sys.stdin)["data"]
item = next((
    x for x in items
    if x["chain_id"] == chain_id
    and x["address"].lower() == address
    and Decimal(x["amount"]) == amount
    and x["tx_hash"].lower() == tx_hash
    and x["status"] == "CREDITED"
), None)
if not item:
    sys.exit(1)
print(json.dumps(item))
' "$chain_id" "${address,,}" "$amount" "${tx_hash,,}" 2>/dev/null || true)"
    if [[ -n "$matched" ]]; then
      printf '%s\n' "$matched"
      return 0
    fi
    sleep 1
  done
  return 1
}

deposit_on_chain() {
  local token="$1"
  local user_id="$2"
  local prefix="$3"
  local amount="$4"
  local units
  units="$(python3 - "$amount" <<'PY'
from decimal import Decimal
import sys
print(int(Decimal(sys.argv[1]) * Decimal("1000000")))
PY
)"

  local chain_id rpc_url factory_address usdc_address generate_payload router_address expected_router deposit_record
  chain_id="$(eval "printf '%s' \"\${${prefix}_CHAIN_ID}\"")"
  rpc_url="$(eval "printf '%s' \"\${${prefix}_RPC_URL_HOST:-\${${prefix}_RPC_URL}}\"")"
  factory_address="$(eval "printf '%s' \"\${${prefix}_FACTORY_ADDRESS}\"")"
  usdc_address="$(eval "printf '%s' \"\${${prefix}_USDC_ADDRESS}\"")"

  generate_payload="$(request_json POST "/api/v1/wallet/deposit-addresses/${chain_id}/generate" '' -H "Authorization: Bearer ${token}")"
  router_address="$(printf '%s' "$generate_payload" | json_field 'data.address')"
  expected_router="$(cast call "$factory_address" "routerOfUser(uint256)(address)" "$user_id" --rpc-url "$rpc_url")"
  if [[ "${router_address,,}" != "${expected_router,,}" ]]; then
    echo "router mismatch for ${prefix}: api=${router_address} chain=${expected_router}" >&2
    exit 1
  fi

  cast send "$usdc_address" "mint(address,uint256)" "$router_address" "$units" --rpc-url "$rpc_url" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
  local forward_output forward_tx_hash
  forward_output="$(cast send "$router_address" "forward()" --rpc-url "$rpc_url" --private-key "$ADMIN_PRIVATE_KEY")"
  forward_tx_hash="$(printf '%s\n' "$forward_output" | awk '/transactionHash/ {print $2; exit}')"
  if [[ -z "$forward_tx_hash" ]]; then
    echo "failed to parse forward tx hash for ${prefix}" >&2
    exit 1
  fi

  deposit_record="$(wait_for_credited_deposit "$token" "$chain_id" "${router_address,,}" "$amount" "$forward_tx_hash")"
  echo "${prefix} deposit ok: ${deposit_record}"
}

wait_for_api

LOGIN_PAYLOAD="$(login "$USER_ADDRESS" "$USER_PRIVATE_KEY" "multichain-smoke")"
ACCESS_TOKEN="$(printf '%s' "$LOGIN_PAYLOAD" | json_field 'data.access_token')"
USER_ID="$(printf '%s' "$LOGIN_PAYLOAD" | json_field 'data.user.id')"
INITIAL_BALANCE="$(current_user_balance "$ACCESS_TOKEN")"

echo "login ok, user_id=${USER_ID}, initial_balance=${INITIAL_BALANCE}"
echo "depositing on ETH"
deposit_on_chain "$ACCESS_TOKEN" "$USER_ID" ETH "1001"
echo "depositing on ARB"
deposit_on_chain "$ACCESS_TOKEN" "$USER_ID" ARB "2002"
echo "depositing on BASE"
deposit_on_chain "$ACCESS_TOKEN" "$USER_ID" BASE "3003"

TARGET_BALANCE="$(python3 - "$INITIAL_BALANCE" <<'PY'
from decimal import Decimal
import sys
print(Decimal(sys.argv[1]) + Decimal("6006"))
PY
)"
BALANCES="$(wait_for_balance "$ACCESS_TOKEN" "$TARGET_BALANCE")"

echo "target_balance=${TARGET_BALANCE}"
echo "balances=${BALANCES}"
