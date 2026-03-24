#!/usr/bin/env bash
set -euo pipefail

MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-root}"
MYSQL_DB="${MYSQL_DB:-rgperp}"
MYSQL_BIN="${MYSQL_BIN:-mysql}"

run_query() {
  local sql="$1"
  MYSQL_PWD="$MYSQL_PASSWORD" "$MYSQL_BIN" \
    --host="$MYSQL_HOST" \
    --port="$MYSQL_PORT" \
    --user="$MYSQL_USER" \
    --database="$MYSQL_DB" \
    --batch --raw -e "$sql"
}

echo "== 1. ledger_entries.asset != ledger_tx.asset =="
run_query "
SELECT
  le.ledger_tx_id,
  lt.biz_type,
  lt.asset AS ledger_tx_asset,
  le.asset AS entry_asset,
  le.account_id,
  le.amount
FROM ledger_entries le
JOIN ledger_tx lt ON lt.ledger_tx_id = le.ledger_tx_id
WHERE le.asset <> lt.asset
ORDER BY le.id ASC
LIMIT 50;
"

echo
echo "== 2. ledger_tx_id + asset non-zero groups =="
run_query "
SELECT
  ledger_tx_id,
  asset,
  CAST(SUM(CAST(amount AS DECIMAL(38,18))) AS CHAR) AS net_amount,
  COUNT(*) AS entry_count
FROM ledger_entries
GROUP BY ledger_tx_id, asset
HAVING SUM(CAST(amount AS DECIMAL(38,18))) <> 0
ORDER BY ledger_tx_id ASC, asset ASC
LIMIT 50;
"

echo
echo "== 3. cross-asset postings by ledger_tx_id =="
run_query "
SELECT
  le.ledger_tx_id,
  COUNT(DISTINCT le.asset) AS distinct_assets,
  GROUP_CONCAT(DISTINCT le.asset ORDER BY le.asset SEPARATOR ',') AS assets
FROM ledger_entries le
GROUP BY le.ledger_tx_id
HAVING COUNT(DISTINCT le.asset) > 1
ORDER BY le.ledger_tx_id ASC
LIMIT 50;
"
