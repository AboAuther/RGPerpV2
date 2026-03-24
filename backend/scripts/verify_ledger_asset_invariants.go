package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:root@tcp(127.0.0.1:3306)/rgperp?charset=utf8mb4&parseTime=True&loc=UTC"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}

	runAndPrint(
		db,
		"1. ledger_entries.asset != ledger_tx.asset",
		`
SELECT
  le.ledger_tx_id,
  lt.biz_type,
  lt.asset AS ledger_tx_asset,
  le.asset AS entry_asset,
  le.account_id,
  CAST(le.amount AS CHAR) AS amount
FROM ledger_entries le
JOIN ledger_tx lt ON lt.ledger_tx_id = le.ledger_tx_id
WHERE le.asset <> lt.asset
ORDER BY le.id ASC
LIMIT 50
`,
	)

	runAndPrint(
		db,
		"2. ledger_tx_id + asset non-zero groups",
		`
SELECT
  ledger_tx_id,
  asset,
  CAST(SUM(CAST(amount AS DECIMAL(38,18))) AS CHAR) AS net_amount,
  COUNT(*) AS entry_count
FROM ledger_entries
GROUP BY ledger_tx_id, asset
HAVING SUM(CAST(amount AS DECIMAL(38,18))) <> 0
ORDER BY ledger_tx_id ASC, asset ASC
LIMIT 50
`,
	)

	runAndPrint(
		db,
		"3. cross-asset postings by ledger_tx_id",
		`
SELECT
  ledger_tx_id,
  COUNT(DISTINCT asset) AS distinct_assets,
  GROUP_CONCAT(DISTINCT asset ORDER BY asset SEPARATOR ',') AS assets
FROM ledger_entries
GROUP BY ledger_tx_id
HAVING COUNT(DISTINCT asset) > 1
ORDER BY ledger_tx_id ASC
LIMIT 50
`,
	)
}

func runAndPrint(db *sql.DB, title string, query string) {
	fmt.Printf("== %s ==\n", title)

	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("query %q: %v", title, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		log.Fatalf("columns %q: %v", title, err)
	}

	found := false
	for rows.Next() {
		found = true
		values := make([]sql.NullString, len(cols))
		scanArgs := make([]any, len(cols))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			log.Fatalf("scan %q: %v", title, err)
		}
		for i, col := range cols {
			value := "NULL"
			if values[i].Valid {
				value = values[i].String
			}
			if i > 0 {
				fmt.Print("\t")
			}
			fmt.Printf("%s=%s", col, value)
		}
		fmt.Println()
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows %q: %v", title, err)
	}
	if !found {
		fmt.Println("(no rows)")
	}
	fmt.Println()
}
