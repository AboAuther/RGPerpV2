#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHAIN_ENV_FILE="${CHAIN_ENV_FILE:-${ROOT_DIR}/deploy/env/local-chains.env}"
if [[ ! -f "$CHAIN_ENV_FILE" && -f "${ROOT_DIR}/.local/contracts.env" ]]; then
  CHAIN_ENV_FILE="${ROOT_DIR}/.local/contracts.env"
fi
if [[ ! -f "$CHAIN_ENV_FILE" ]]; then
  TMP_CHAIN_ENV="$(mktemp)"
  if docker compose exec -T api-server cat /shared/local-chains.env >"$TMP_CHAIN_ENV" 2>/dev/null; then
    CHAIN_ENV_FILE="$TMP_CHAIN_ENV"
  else
    rm -f "$TMP_CHAIN_ENV"
  fi
fi

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_PRIVATE_KEY="${ADMIN_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
USER_PRIVATE_KEY="${USER_PRIVATE_KEY:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d}"
LOCAL_CHAIN_ID="${LOCAL_CHAIN_ID:-31337}"
STRESS_SYMBOL="${STRESS_SYMBOL:-ETH-USDC}"
BENCH_MIN_USDC="${BENCH_MIN_USDC:-2000}"
BENCH_SCENARIO="${BENCH_SCENARIO:-limit_cycle}"
OUTPUT_DIR="${OUTPUT_DIR:-${ROOT_DIR}/.local/benchmarks}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d-%H%M%S)}"
JSON_OUTPUT="${OUTPUT_DIR}/order-api-benchmark-${RUN_ID}.json"

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

mkdir -p "$OUTPUT_DIR"

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
  challenge="$(request_json POST '/api/v1/auth/challenge' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID}}" -H "X-Trace-Id: bench_nonce_${RUN_ID}")"
  assert_ok '/api/v1/auth/challenge' "$challenge"
  nonce="$(printf '%s' "$challenge" | jq -r '.data.nonce')"
  message="$(printf '%s' "$challenge" | jq -r '.data.message')"
  signature="$(cast wallet sign --private-key "$private_key" "$message")"
  payload="$(request_json POST '/api/v1/auth/login' "{\"address\":\"${address}\",\"chain_id\":${LOCAL_CHAIN_ID},\"nonce\":\"${nonce}\",\"signature\":\"${signature}\",\"device_fingerprint\":\"${fingerprint}\"}" -H "X-Trace-Id: bench_login_${RUN_ID}")"
  assert_ok '/api/v1/auth/login' "$payload"
  printf '%s' "$payload"
}

ensure_wallet_funded() {
  local token="$1"
  local user_id="$2"
  local minimum="$3"

  local balances wallet
  balances="$(request_json GET '/api/v1/account/balances' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: bench_balance_${RUN_ID}")"
  assert_ok '/api/v1/account/balances' "$balances"
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
  generate="$(request_json POST "/api/v1/wallet/deposit-addresses/${LOCAL_CHAIN_ID}/generate" '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: bench_deposit_addr_${RUN_ID}")"
  assert_ok 'generate deposit address' "$generate"
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
    balances="$(request_json GET '/api/v1/account/balances' '' -H "Authorization: Bearer ${token}" -H "X-Trace-Id: bench_refund_${RUN_ID}")"
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

print_summary() {
  local json_file="$1"
  local primary_operation
  local secondary_operation=""

  case "$BENCH_SCENARIO" in
    limit_place_only)
      primary_operation="limit_place"
      ;;
    *)
      primary_operation="limit_create"
      secondary_operation="limit_cancel"
      ;;
  esac

  jq -r '
    .stages[]
    | [
        "stage=" + (.concurrency|tostring),
        "tps=" + (.total_rps|tostring),
        "primary_p95_ms=" + ((.operations[$primary].p95_ms // 0)|tostring),
        "primary_p99_ms=" + ((.operations[$primary].p99_ms // 0)|tostring),
        "secondary_p95_ms=" + ((.operations[$secondary].p95_ms // 0)|tostring),
        "secondary_p99_ms=" + ((.operations[$secondary].p99_ms // 0)|tostring),
        "success=" + ((.operations[$primary].success_rate // 0)|tostring)
      ]
      | @tsv
  ' --arg primary "$primary_operation" --arg secondary "$secondary_operation" "$json_file" | while IFS=$'\t' read -r stage tps cp95 cp99 xp95 xp99 success; do
    printf '%s %s %s %s %s %s %s\n' "$stage" "$tps" "$cp95" "$cp99" "$xp95" "$xp99" "$success"
  done

  echo
  echo "best_stage:"
  jq -r '
    (.stages | max_by(.total_rps)) as $best
    | [
        "stage=" + ($best.concurrency|tostring),
        "tps=" + ($best.total_rps|tostring),
        "primary_p95_ms=" + (($best.operations[$primary].p95_ms // 0)|tostring),
        "primary_p99_ms=" + (($best.operations[$primary].p99_ms // 0)|tostring),
        "secondary_p99_ms=" + (($best.operations[$secondary].p99_ms // 0)|tostring)
      ]
      | join(" ")
  ' --arg primary "$primary_operation" --arg secondary "$secondary_operation" "$json_file"
}

USER_ADDRESS="$(cast wallet address --private-key "$USER_PRIVATE_KEY")"

wait_for_api

LOGIN_PAYLOAD="$(login_payload "$USER_ADDRESS" "$USER_PRIVATE_KEY" "order-api-benchmark")"
ACCESS_TOKEN="$(printf '%s' "$LOGIN_PAYLOAD" | jq -r '.data.access_token')"
USER_ID="$(printf '%s' "$LOGIN_PAYLOAD" | jq -r '.data.user.id')"
ensure_wallet_funded "$ACCESS_TOKEN" "$USER_ID" "$BENCH_MIN_USDC"

echo "running order-path benchmark..."
(
  cd "${ROOT_DIR}/backend"
  API_BASE_URL="$API_BASE_URL" \
  STRESS_PRIVATE_KEY="$USER_PRIVATE_KEY" \
  STRESS_CHAIN_ID="$LOCAL_CHAIN_ID" \
  STRESS_SYMBOL="$STRESS_SYMBOL" \
  STRESS_SCENARIO="$BENCH_SCENARIO" \
  go run ./cmd/api-stress \
    --scenario "$BENCH_SCENARIO" \
    --json \
    "$@"
) >"$JSON_OUTPUT"

echo "saved_json=${JSON_OUTPUT}"
print_summary "$JSON_OUTPUT"
