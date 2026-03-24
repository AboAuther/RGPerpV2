#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"
if [[ ! -f "$CHAIN_ENV_FILE" && -f "${ROOT_DIR}/.local/contracts.env" ]]; then
  CHAIN_ENV_FILE="${ROOT_DIR}/.local/contracts.env"
fi
API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"
MYSQL_SERVICE_NAME="${MYSQL_SERVICE_NAME:-mysql}"
MYSQL_DB_NAME="${MYSQL_DB_NAME:-rgperp}"
MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-root}"
REDIS_SERVICE_NAME="${REDIS_SERVICE_NAME:-redis}"
ADMIN_PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
USER_PRIVATE_KEY="${USER_PRIVATE_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"
LOCAL_CHAIN_ID="${LOCAL_CHAIN_ID:-31337}"
FUND_AMOUNT_UNITS="${FUND_AMOUNT_UNITS:-5000}"
RUN_ID="${RUN_ID:-$(date +%s)}"

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
: "${LOCAL_ANVIL_ADMIN_ADDRESS:?LOCAL_ANVIL_ADMIN_ADDRESS is required}"

USER_ADDRESS="$(cast wallet address --private-key "$USER_PRIVATE_KEY")"

request_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  shift 3 || true

  local args=(
    -fsS
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

wait_for_api() {
  for _ in $(seq 1 120); do
    if curl -fsS "${API_BASE_URL}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "api-server not healthy" >&2
  exit 1
}

login_payload() {
  local address="$1"
  local private_key="$2"
  local fingerprint="$3"

  local challenge nonce message signature payload
  challenge="$(request_json POST '/api/v1/auth/challenge' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID}}" -H "X-Trace-Id: trade_nonce_$(date +%s)")"
  assert_ok '/api/v1/auth/challenge' "$challenge" >/dev/null
  nonce="$(printf '%s' "$challenge" | jq -r '.data.nonce')"
  message="$(printf '%s' "$challenge" | jq -r '.data.message')"
  signature="$(cast wallet sign --private-key "$private_key" "$message")"
  payload="$(request_json POST '/api/v1/auth/login' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID},\"nonce\":\"${nonce}\",\"signature\":\"${signature}\",\"device_fingerprint\":\"${fingerprint}\"}" -H "X-Trace-Id: trade_login_$(date +%s)")"
  assert_ok '/api/v1/auth/login' "$payload" >/dev/null
  printf '%s' "$payload"
}

ensure_wallet_funded() {
  local token="$1"
  local user_id="$2"
  local minimum="$3"

  local balances wallet
  balances="$(request_json GET '/api/v1/account/balances' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_balance_$(date +%s)")"
  assert_ok '/api/v1/account/balances' "$balances" >/dev/null
  wallet="$(printf '%s' "$balances" | jq -r '.data[] | select(.account_code=="USER_WALLET" and .asset=="USDC") | .balance' | head -n1)"
  wallet="${wallet:-0}"
  if python3 - "$wallet" "$minimum" <<'PY'
from decimal import Decimal
import sys
sys.exit(0 if Decimal(sys.argv[1]) >= Decimal(sys.argv[2]) else 1)
PY
  then
    return 0
  fi

  local generate deposit_address expected_router topup_units
  generate="$(request_json POST "/api/v1/wallet/deposit-addresses/${LOCAL_CHAIN_ID}/generate" '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_deposit_addr_$(date +%s)")"
  assert_ok 'generate deposit address' "$generate" >/dev/null
  deposit_address="$(printf '%s' "$generate" | jq -r '.data.address')"
  expected_router="$(cast call "$BASE_FACTORY_ADDRESS" "routerOfUser(uint256)(address)" "$user_id" --rpc-url "$BASE_RPC_URL")"
  if [[ "${expected_router,,}" == "0x0000000000000000000000000000000000000000" ]]; then
    cast send "$BASE_FACTORY_ADDRESS" "createRouter(uint256,bytes32)" "$user_id" "$(cast keccak "rgperp:user:${user_id}")" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
    expected_router="$(cast call "$BASE_FACTORY_ADDRESS" "routerOfUser(uint256)(address)" "$user_id" --rpc-url "$BASE_RPC_URL")"
  fi
  if [[ "${deposit_address,,}" != "${expected_router,,}" ]]; then
    echo "deposit address mismatch: api=${deposit_address} chain=${expected_router}" >&2
    exit 1
  fi

  topup_units="$(python3 - "$minimum" "$wallet" <<'PY'
from decimal import Decimal
import sys
needed = Decimal(sys.argv[1]) - Decimal(sys.argv[2])
if needed < Decimal("0"):
    needed = Decimal("0")
buffered = needed + Decimal("100")
print(int(buffered * Decimal(1000000)))
PY
)"
  cast send "$BASE_USDC_ADDRESS" "mint(address,uint256)" "$deposit_address" "$topup_units" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null
  cast send "$deposit_address" "forward()" --rpc-url "$BASE_RPC_URL" --private-key "$ADMIN_PRIVATE_KEY" >/dev/null

  for _ in $(seq 1 90); do
    balances="$(request_json GET '/api/v1/account/balances' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_refund_$(date +%s)")"
    wallet="$(printf '%s' "$balances" | jq -r '.data[] | select(.account_code=="USER_WALLET" and .asset=="USDC") | .balance' | head -n1)"
    wallet="${wallet:-0}"
    if python3 - "$wallet" "$minimum" <<'PY'
from decimal import Decimal
import sys
sys.exit(0 if Decimal(sys.argv[1]) >= Decimal(sys.argv[2]) else 1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "wallet funding timeout" >&2
  exit 1
}

mysql_exec() {
  local sql="$1"
  docker compose exec -T "$MYSQL_SERVICE_NAME" mysql -uroot "-p${MYSQL_ROOT_PASSWORD}" "$MYSQL_DB_NAME" -N -e "$sql"
}

redis_exec() {
  docker compose exec -T "$REDIS_SERVICE_NAME" redis-cli "$@"
}

set_symbol_status() {
  local symbol="$1"
  local status="$2"
  mysql_exec "UPDATE symbols SET status='${status}' WHERE symbol='${symbol}';"
  local key="market:latest:symbol:${symbol}"
  local payload
  payload="$(redis_exec GET "$key")"
  if [[ -n "$payload" ]]; then
    payload="$(printf '%s' "$payload" | python3 -c '
import json, sys
obj = json.load(sys.stdin)
obj["status"] = sys.argv[1]
print(json.dumps(obj, separators=(",", ":")))
' "$status")"
    redis_exec SET "$key" "$payload" >/dev/null
  fi
}

wait_for_status() {
  local token="$1"
  local path="$2"
  local jq_expr="$3"
  local expected="$4"
  for _ in $(seq 1 60); do
    local payload current
    payload="$(request_json GET "$path" '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_poll_$(date +%s)")"
    current="$(printf '%s' "$payload" | jq -r "$jq_expr")"
    if [[ "$current" == "$expected" ]]; then
      printf '%s' "$payload"
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for ${path} -> ${expected}" >&2
  exit 1
}

wait_for_position_qty() {
  local token="$1"
  local symbol="$2"
  local side="$3"
  local expected="$4"
  for _ in $(seq 1 60); do
    local payload qty
    payload="$(request_json GET '/api/v1/positions' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_pos_$(date +%s)")"
    qty="$(printf '%s' "$payload" | jq -r --arg symbol "$symbol" --arg side "$side" '.data[] | select(.symbol==$symbol and .side==$side) | .qty' | head -n1)"
    qty="${qty:-0}"
    if python3 - "$qty" "$expected" <<'PY'
from decimal import Decimal
import sys
sys.exit(0 if Decimal(sys.argv[1]) == Decimal(sys.argv[2]) else 1)
PY
    then
      printf '%s' "$payload"
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for position ${symbol}/${side} qty=${expected}" >&2
  exit 1
}

current_position_qty() {
 local token="$1"
 local symbol="$2"
 local side="$3"
  local payload qty
  payload="$(request_json GET '/api/v1/positions' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_pos_now_$(date +%s)")"
  qty="$(printf '%s' "$payload" | jq -r --arg symbol "$symbol" --arg side "$side" '.data[] | select(.symbol==$symbol and .side==$side) | .qty' | head -n1)"
  printf '%s' "${qty:-0}"
}

current_mark_price() {
  local token="$1"
  local symbol="$2"
  local payload mark
  payload="$(request_json GET '/api/v1/markets/tickers' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_mark_$(date +%s)")"
  assert_ok '/api/v1/markets/tickers' "$payload" >/dev/null
  mark="$(printf '%s' "$payload" | jq -r --arg symbol "$symbol" '.data[] | select(.symbol==$symbol) | .mark_price' | head -n1)"
  if [[ -z "$mark" || "$mark" == "null" ]]; then
    echo "missing mark price for ${symbol}" >&2
    exit 1
  fi
  printf '%s' "$mark"
}

current_tick_size() {
  local token="$1"
  local symbol="$2"
  local payload tick
  payload="$(request_json GET '/api/v1/markets/symbols' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_tick_$(date +%s)")"
  assert_ok '/api/v1/markets/symbols' "$payload" >/dev/null
  tick="$(printf '%s' "$payload" | jq -r --arg symbol "$symbol" '.data[] | select(.symbol==$symbol) | .tick_size' | head -n1)"
  if [[ -z "$tick" || "$tick" == "null" ]]; then
    echo "missing tick size for ${symbol}" >&2
    exit 1
  fi
  printf '%s' "$tick"
}

decimal_mul() {
  python3 - "$1" "$2" <<'PY'
from decimal import Decimal, ROUND_HALF_EVEN
import sys
value = (Decimal(sys.argv[1]) * Decimal(sys.argv[2])).quantize(Decimal("0.000000000000000001"), rounding=ROUND_HALF_EVEN)
print(format(value, "f"))
PY
}

decimal_align_step() {
  python3 - "$1" "$2" <<'PY'
from decimal import Decimal, ROUND_FLOOR
import sys
value = Decimal(sys.argv[1])
step = Decimal(sys.argv[2])
aligned = ((value / step).quantize(Decimal("1"), rounding=ROUND_FLOOR)) * step
print(format(aligned, "f"))
PY
}

wait_for_position_delta() {
  local token="$1"
  local symbol="$2"
  local side="$3"
  local before="$4"
  local expected_delta="$5"
  for _ in $(seq 1 60); do
    local current
    current="$(current_position_qty "$token" "$symbol" "$side")"
    if python3 - "$current" "$before" "$expected_delta" <<'PY'
from decimal import Decimal
import sys
current = Decimal(sys.argv[1])
before = Decimal(sys.argv[2])
expected = Decimal(sys.argv[3])
sys.exit(0 if current == before + expected else 1)
PY
    then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for position delta ${symbol}/${side} += ${expected_delta}" >&2
  exit 1
}

wait_for_order_status() {
  local token="$1"
  local client_order_id="$2"
  local expected="$3"
  for _ in $(seq 1 60); do
    local payload current
    payload="$(request_json GET '/api/v1/orders' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_order_$(date +%s)")"
    current="$(printf '%s' "$payload" | jq -r --arg id "$client_order_id" '.data[] | select(.client_order_id==$id) | .status' | head -n1)"
    if [[ "$current" == "$expected" ]]; then
      printf '%s' "$payload"
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for order ${client_order_id} -> ${expected}" >&2
  exit 1
}

wait_for_fill() {
  local token="$1"
  local order_id="$2"
  for _ in $(seq 1 60); do
    local payload matched
    payload="$(request_json GET '/api/v1/fills' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_fill_$(date +%s)")"
    matched="$(printf '%s' "$payload" | jq -r --arg oid "$order_id" '.data[] | select(.order_id==$oid) | .fill_id' | head -n1)"
    if [[ -n "$matched" ]]; then
      printf '%s' "$payload"
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for fill ${order_id}" >&2
  exit 1
}

count_fills_for_order() {
  local token="$1"
  local order_id="$2"
  local payload count
  payload="$(request_json GET '/api/v1/fills' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_fill_count_$(date +%s)")"
  assert_ok '/api/v1/fills' "$payload" >/dev/null
  count="$(printf '%s' "$payload" | jq -r --arg oid "$order_id" '[.data[] | select(.order_id==$oid)] | length')"
  printf '%s' "$count"
}

assert_explorer_trade_event() {
  local token="$1"
  local event_type="$2"
  local field="$3"
  local expected="$4"
  local payload matched
  payload="$(request_json GET '/api/v1/explorer/events' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: trade_explorer_$(date +%s)")"
  assert_ok '/api/v1/explorer/events' "$payload" >/dev/null
  matched="$(printf '%s' "$payload" | jq -r --arg event_type "$event_type" --arg field "$field" --arg expected "$expected" '
    .data[]
    | select(.event_type == $event_type)
    | select(((.[$field] // "") == $expected) or ((.payload[$field] // "") == $expected))
    | .event_id
  ' | head -n1)"
  if [[ -z "$matched" ]]; then
    echo "missing explorer event ${event_type} ${field}=${expected}: ${payload}" >&2
    exit 1
  fi
}

wait_for_rejected_open() {
  local token="$1"
  local body="$2"
  local payload code message
  payload="$(curl -sS -X POST "${API_BASE_URL}/api/v1/orders" \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -H "Idempotency-Key: trade-reg-open-reject-${RUN_ID}" \
    -H "X-Trace-Id: trade_open_reject_$(date +%s)" \
    --data "$body")"
  code="$(printf '%s' "$payload" | jq -r '.code // empty')"
  message="$(printf '%s' "$payload" | jq -r '.message // empty')"
  if [[ "$code" == "FORBIDDEN" || "$message" == *"not tradable for OPEN"* ]]; then
    printf '%s' "$payload"
    return 0
  fi
  echo "expected OPEN rejection, got: ${payload}" >&2
  exit 1
}

wait_for_api
trap 'set_symbol_status "BTC-USDC" "TRADING" >/dev/null 2>&1 || true' EXIT
USER_LOGIN="$(login_payload "$USER_ADDRESS" "$USER_PRIVATE_KEY" 'trade-reg-user')"
USER_TOKEN="$(printf '%s' "$USER_LOGIN" | jq -r '.data.access_token')"
USER_ID="$(printf '%s' "$USER_LOGIN" | jq -r '.data.user.id')"
ensure_wallet_funded "$USER_TOKEN" "$USER_ID" "$FUND_AMOUNT_UNITS"

SYMBOLS="$(request_json GET '/api/v1/markets/symbols' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: trade_symbols_$(date +%s)")"
assert_ok '/api/v1/markets/symbols' "$SYMBOLS"
TICKERS="$(request_json GET '/api/v1/markets/tickers' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: trade_tickers_$(date +%s)")"
assert_ok '/api/v1/markets/tickers' "$TICKERS"

OPEN_BODY='{"client_order_id":"trade-reg-open-market","symbol":"BTC-USDC","side":"BUY","position_effect":"OPEN","type":"MARKET","qty":"0.001"}'
OPEN_CLIENT_ORDER_ID="trade-reg-open-market-${RUN_ID}"
OPEN_BODY="{\"client_order_id\":\"${OPEN_CLIENT_ORDER_ID}\",\"symbol\":\"BTC-USDC\",\"side\":\"BUY\",\"position_effect\":\"OPEN\",\"type\":\"MARKET\",\"qty\":\"0.001\"}"
LONG_QTY_BEFORE="$(current_position_qty "$USER_TOKEN" 'BTC-USDC' 'LONG')"
OPEN_ORDER="$(request_json POST '/api/v1/orders' "$OPEN_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${OPEN_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_open_$(date +%s)")"
assert_ok 'market open order' "$OPEN_ORDER"
OPEN_ORDER_ID="$(printf '%s' "$OPEN_ORDER" | jq -r '.data.order_id')"
OPEN_STATUS="$(printf '%s' "$OPEN_ORDER" | jq -r '.data.status')"
if [[ "$OPEN_STATUS" != "FILLED" ]]; then
  echo "expected FILLED market open, got ${OPEN_STATUS}" >&2
  exit 1
fi
wait_for_fill "$USER_TOKEN" "$OPEN_ORDER_ID" >/dev/null
wait_for_position_delta "$USER_TOKEN" 'BTC-USDC' 'LONG' "$LONG_QTY_BEFORE" '0.001000000000000000'
OPEN_FILL_COUNT_BEFORE_DUPLICATE="$(count_fills_for_order "$USER_TOKEN" "$OPEN_ORDER_ID")"
DUPLICATE_OPEN_ORDER="$(request_json POST '/api/v1/orders' "$OPEN_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${OPEN_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_open_duplicate_$(date +%s)")"
assert_ok 'duplicate market open order returns idempotent response' "$DUPLICATE_OPEN_ORDER"
DUPLICATE_ORDER_ID="$(printf '%s' "$DUPLICATE_OPEN_ORDER" | jq -r '.data.order_id')"
if [[ "$DUPLICATE_ORDER_ID" != "$OPEN_ORDER_ID" ]]; then
  echo "expected duplicate order to return ${OPEN_ORDER_ID}, got ${DUPLICATE_ORDER_ID}" >&2
  exit 1
fi
OPEN_FILL_COUNT_AFTER_DUPLICATE="$(count_fills_for_order "$USER_TOKEN" "$OPEN_ORDER_ID")"
if [[ "$OPEN_FILL_COUNT_AFTER_DUPLICATE" != "$OPEN_FILL_COUNT_BEFORE_DUPLICATE" ]]; then
  echo "duplicate order created extra fill: before=${OPEN_FILL_COUNT_BEFORE_DUPLICATE} after=${OPEN_FILL_COUNT_AFTER_DUPLICATE}" >&2
  exit 1
fi

LIMIT_CLIENT_ORDER_ID="trade-reg-limit-open-${RUN_ID}"
LIMIT_BODY="{\"client_order_id\":\"${LIMIT_CLIENT_ORDER_ID}\",\"symbol\":\"ETH-USDC\",\"side\":\"BUY\",\"position_effect\":\"OPEN\",\"type\":\"LIMIT\",\"qty\":\"1\",\"price\":\"10\"}"
LIMIT_ORDER="$(request_json POST '/api/v1/orders' "$LIMIT_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${LIMIT_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_limit_$(date +%s)")"
assert_ok 'limit open order' "$LIMIT_ORDER"
LIMIT_ORDER_ID="$(printf '%s' "$LIMIT_ORDER" | jq -r '.data.order_id')"
LIMIT_STATUS="$(printf '%s' "$LIMIT_ORDER" | jq -r '.data.status')"
if [[ "$LIMIT_STATUS" != "RESTING" ]]; then
  echo "expected RESTING limit open, got ${LIMIT_STATUS}" >&2
  exit 1
fi
wait_for_order_status "$USER_TOKEN" "${LIMIT_CLIENT_ORDER_ID}" 'RESTING' >/dev/null

CANCEL_PAYLOAD="$(request_json POST "/api/v1/orders/${LIMIT_ORDER_ID}/cancel" '' -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: trade-reg-limit-cancel-${RUN_ID}" -H "X-Trace-Id: trade_cancel_$(date +%s)")"
assert_ok 'cancel limit order' "$CANCEL_PAYLOAD"
wait_for_order_status "$USER_TOKEN" "${LIMIT_CLIENT_ORDER_ID}" 'CANCELED' >/dev/null

AUTO_LIMIT_CLIENT_ORDER_ID="trade-reg-limit-auto-${RUN_ID}"
ETH_LONG_BEFORE="$(current_position_qty "$USER_TOKEN" 'ETH-USDC' 'LONG')"
AUTO_LIMIT_BODY="{\"client_order_id\":\"${AUTO_LIMIT_CLIENT_ORDER_ID}\",\"symbol\":\"ETH-USDC\",\"side\":\"BUY\",\"position_effect\":\"OPEN\",\"type\":\"LIMIT\",\"qty\":\"0.1\",\"price\":\"100000\"}"
AUTO_LIMIT_ORDER="$(request_json POST '/api/v1/orders' "$AUTO_LIMIT_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${AUTO_LIMIT_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_limit_auto_$(date +%s)")"
assert_ok 'auto executable limit open order' "$AUTO_LIMIT_ORDER"
AUTO_LIMIT_ORDER_ID="$(printf '%s' "$AUTO_LIMIT_ORDER" | jq -r '.data.order_id')"
wait_for_order_status "$USER_TOKEN" "${AUTO_LIMIT_CLIENT_ORDER_ID}" 'FILLED' >/dev/null
wait_for_fill "$USER_TOKEN" "$AUTO_LIMIT_ORDER_ID" >/dev/null
wait_for_position_delta "$USER_TOKEN" 'ETH-USDC' 'LONG' "$ETH_LONG_BEFORE" '0.100000000000000000' >/dev/null

# Use an already satisfied trigger to make local TP smoke deterministic.
ETH_MARK_PRICE="$(current_mark_price "$USER_TOKEN" 'ETH-USDC')"
ETH_TICK_SIZE="$(current_tick_size "$USER_TOKEN" 'ETH-USDC')"
TP_TRIGGER_PRICE="$(decimal_align_step "$(decimal_mul "$ETH_MARK_PRICE" '0.5')" "$ETH_TICK_SIZE")"
TP_CLIENT_ORDER_ID="trade-reg-tp-close-${RUN_ID}"
TP_BODY="{\"client_order_id\":\"${TP_CLIENT_ORDER_ID}\",\"symbol\":\"ETH-USDC\",\"side\":\"SELL\",\"position_effect\":\"CLOSE\",\"type\":\"TAKE_PROFIT_MARKET\",\"qty\":\"0.1\",\"trigger_price\":\"${TP_TRIGGER_PRICE}\"}"
TP_ORDER="$(request_json POST '/api/v1/orders' "$TP_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${TP_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_tp_$(date +%s)")"
assert_ok 'take profit trigger order' "$TP_ORDER"
TP_ORDER_ID="$(printf '%s' "$TP_ORDER" | jq -r '.data.order_id')"
TP_STATUS="$(printf '%s' "$TP_ORDER" | jq -r '.data.status')"
if [[ "$TP_STATUS" != "TRIGGER_WAIT" ]]; then
  echo "expected TRIGGER_WAIT take profit order, got ${TP_STATUS}" >&2
  exit 1
fi
wait_for_order_status "$USER_TOKEN" "${TP_CLIENT_ORDER_ID}" 'FILLED' >/dev/null
wait_for_fill "$USER_TOKEN" "$TP_ORDER_ID" >/dev/null
wait_for_position_qty "$USER_TOKEN" 'ETH-USDC' 'LONG' "$ETH_LONG_BEFORE" >/dev/null

ETH_SHORT_BEFORE="$(current_position_qty "$USER_TOKEN" 'ETH-USDC' 'SHORT')"
SHORT_OPEN_CLIENT_ORDER_ID="trade-reg-open-short-${RUN_ID}"
SHORT_OPEN_BODY="{\"client_order_id\":\"${SHORT_OPEN_CLIENT_ORDER_ID}\",\"symbol\":\"ETH-USDC\",\"side\":\"SELL\",\"position_effect\":\"OPEN\",\"type\":\"MARKET\",\"qty\":\"0.1\"}"
SHORT_OPEN_ORDER="$(request_json POST '/api/v1/orders' "$SHORT_OPEN_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${SHORT_OPEN_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_short_open_$(date +%s)")"
assert_ok 'market short open order' "$SHORT_OPEN_ORDER"
SHORT_OPEN_ORDER_ID="$(printf '%s' "$SHORT_OPEN_ORDER" | jq -r '.data.order_id')"
SHORT_OPEN_STATUS="$(printf '%s' "$SHORT_OPEN_ORDER" | jq -r '.data.status')"
if [[ "$SHORT_OPEN_STATUS" != "FILLED" ]]; then
  echo "expected FILLED market short open, got ${SHORT_OPEN_STATUS}" >&2
  exit 1
fi
wait_for_fill "$USER_TOKEN" "$SHORT_OPEN_ORDER_ID" >/dev/null
wait_for_position_delta "$USER_TOKEN" 'ETH-USDC' 'SHORT' "$ETH_SHORT_BEFORE" '0.100000000000000000' >/dev/null

# Use an already satisfied trigger to make local SL smoke deterministic.
ETH_MARK_PRICE="$(current_mark_price "$USER_TOKEN" 'ETH-USDC')"
STOP_TRIGGER_PRICE="$(decimal_align_step "$(decimal_mul "$ETH_MARK_PRICE" '0.5')" "$ETH_TICK_SIZE")"
STOP_CLIENT_ORDER_ID="trade-reg-stop-close-${RUN_ID}"
STOP_BODY="{\"client_order_id\":\"${STOP_CLIENT_ORDER_ID}\",\"symbol\":\"ETH-USDC\",\"side\":\"BUY\",\"position_effect\":\"CLOSE\",\"type\":\"STOP_MARKET\",\"qty\":\"0.1\",\"trigger_price\":\"${STOP_TRIGGER_PRICE}\"}"
STOP_ORDER="$(request_json POST '/api/v1/orders' "$STOP_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${STOP_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_stop_$(date +%s)")"
assert_ok 'stop market trigger order' "$STOP_ORDER"
STOP_ORDER_ID="$(printf '%s' "$STOP_ORDER" | jq -r '.data.order_id')"
STOP_STATUS="$(printf '%s' "$STOP_ORDER" | jq -r '.data.status')"
if [[ "$STOP_STATUS" != "TRIGGER_WAIT" ]]; then
  echo "expected TRIGGER_WAIT stop order, got ${STOP_STATUS}" >&2
  exit 1
fi
wait_for_order_status "$USER_TOKEN" "${STOP_CLIENT_ORDER_ID}" 'FILLED' >/dev/null
wait_for_fill "$USER_TOKEN" "$STOP_ORDER_ID" >/dev/null
wait_for_position_qty "$USER_TOKEN" 'ETH-USDC' 'SHORT' "$ETH_SHORT_BEFORE" >/dev/null

set_symbol_status "BTC-USDC" "PAUSED"
wait_for_rejected_open "$USER_TOKEN" "{\"client_order_id\":\"trade-reg-open-paused-${RUN_ID}\",\"symbol\":\"BTC-USDC\",\"side\":\"BUY\",\"position_effect\":\"OPEN\",\"type\":\"MARKET\",\"qty\":\"0.001\"}" >/dev/null

CLOSE_CLIENT_ORDER_ID="trade-reg-close-market-${RUN_ID}"
CLOSE_BODY="{\"client_order_id\":\"${CLOSE_CLIENT_ORDER_ID}\",\"symbol\":\"BTC-USDC\",\"side\":\"SELL\",\"position_effect\":\"CLOSE\",\"type\":\"MARKET\",\"qty\":\"0.001\"}"
set_symbol_status "BTC-USDC" "TRADING"
CLOSE_ORDER="$(request_json POST '/api/v1/orders' "$CLOSE_BODY" -H "Authorization: Bearer ${USER_TOKEN}" -H "Idempotency-Key: ${CLOSE_CLIENT_ORDER_ID}" -H "X-Trace-Id: trade_close_$(date +%s)")"
assert_ok 'market close order' "$CLOSE_ORDER"
CLOSE_STATUS="$(printf '%s' "$CLOSE_ORDER" | jq -r '.data.status')"
if [[ "$CLOSE_STATUS" != "FILLED" ]]; then
  echo "expected FILLED market close, got ${CLOSE_STATUS}" >&2
  exit 1
fi
wait_for_position_qty "$USER_TOKEN" 'BTC-USDC' 'LONG' "$LONG_QTY_BEFORE" >/dev/null

ORDERS="$(request_json GET '/api/v1/orders' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: trade_orders_$(date +%s)")"
assert_ok '/api/v1/orders' "$ORDERS"
FILLS="$(request_json GET '/api/v1/fills' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: trade_fills_$(date +%s)")"
assert_ok '/api/v1/fills' "$FILLS"
POSITIONS="$(request_json GET '/api/v1/positions' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: trade_positions_$(date +%s)")"
assert_ok '/api/v1/positions' "$POSITIONS"
SUMMARY="$(request_json GET '/api/v1/account/summary' '' -H "Authorization: Bearer ${USER_TOKEN}" -H "X-Trace-Id: trade_summary_$(date +%s)")"
assert_ok '/api/v1/account/summary' "$SUMMARY"

assert_explorer_trade_event "$USER_TOKEN" 'trade.order.accepted' 'order_id' "$OPEN_ORDER_ID"
assert_explorer_trade_event "$USER_TOKEN" 'trade.order.canceled' 'order_id' "$LIMIT_ORDER_ID"
assert_explorer_trade_event "$USER_TOKEN" 'trade.fill.created' 'order_id' "$TP_ORDER_ID"
assert_explorer_trade_event "$USER_TOKEN" 'trade.position.updated' 'order_id' "$STOP_ORDER_ID"

echo
echo "trading regression passed"
echo "user_address=${USER_ADDRESS}"
echo "open_order_id=${OPEN_ORDER_ID}"
echo "limit_order_id=${LIMIT_ORDER_ID}"
echo "tp_order_id=${TP_ORDER_ID}"
echo "stop_order_id=${STOP_ORDER_ID}"
