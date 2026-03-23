package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

func (r *WalletQueryRepository) GetLedgerOverview(ctx context.Context, scopeAsset string) (readmodel.LedgerOverview, error) {
	scopeAsset = normalizeAuditScopeAsset(scopeAsset)
	var rows []struct {
		AccountID   uint64
		AccountCode string
		Asset       string
		Balance     string
	}
	if err := DB(ctx, r.db).
		Table("accounts").
		Select("accounts.id AS account_id, accounts.account_code, accounts.asset, COALESCE(account_balance_snapshots.balance, '0') AS balance").
		Joins("LEFT JOIN account_balance_snapshots ON account_balance_snapshots.account_id = accounts.id AND account_balance_snapshots.asset = accounts.asset").
		Order("accounts.asset ASC, accounts.account_code ASC, accounts.id ASC").
		Scan(&rows).Error; err != nil {
		return readmodel.LedgerOverview{}, err
	}

	assets := filterOverviewByScope(aggregateLedgerOverview(rows), scopeAsset)
	return readmodel.LedgerOverview{
		ScopeAsset:  scopeAsset,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Notes: []string{
			"所有金额都以统一账本账户快照汇总，account_balance_snapshots 仅作为读优化。",
			"USER_MARGIN 展示口径为 USER_ORDER_MARGIN + USER_POSITION_MARGIN。",
			"一键审计会验证账本守恒、快照重放、业务覆盖、outbox 完整性和链上 custody mirror 对账。",
		},
		Assets: assets,
	}, nil
}

func (r *WalletQueryRepository) RunLedgerAudit(ctx context.Context, executedBy string, scopeAsset string) (readmodel.LedgerAuditReport, error) {
	scopeAsset = normalizeAuditScopeAsset(scopeAsset)
	startedAt := time.Now().UTC()

	overview, err := r.GetLedgerOverview(ctx, scopeAsset)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	filteredOverview := overview.Assets

	checks := make([]readmodel.LedgerAuditCheck, 0, 7)

	netSamples := make([]string, 0)
	for _, item := range filteredOverview {
		if !decimalx.MustFromString(item.NetBalance).IsZero() {
			netSamples = append(netSamples, fmt.Sprintf("%s=%s", item.Asset, item.NetBalance))
		}
	}
	checks = append(checks, buildAuditCheck(
		"net_balance_zero",
		"账户快照总和为零",
		len(netSamples) == 0,
		fmt.Sprintf("%d", len(filteredOverview)),
		"每个资产维度下，所有账户快照余额之和必须为 0。",
		netSamples,
	))

	unbalanced, err := r.findUnbalancedLedgerTx(ctx, scopeAsset)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	checks = append(checks, buildAuditCheck(
		"ledger_tx_balanced",
		"每笔 ledger_tx 守恒",
		len(unbalanced) == 0,
		fmt.Sprintf("%d", len(unbalanced)),
		"逐笔校验 ledger_entries 按 ledger_tx_id + asset 聚合后的金额之和必须为 0。",
		unbalanced,
	))

	snapshotMismatches, err := r.findSnapshotMismatches(ctx, scopeAsset)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	checks = append(checks, buildAuditCheck(
		"snapshot_replay_match",
		"快照可由账本重放",
		len(snapshotMismatches) == 0,
		fmt.Sprintf("%d", len(snapshotMismatches)),
		"用 ledger_entries 重放出的余额必须与 account_balance_snapshots 一致。",
		snapshotMismatches,
	))

	depositCoverage, err := r.findCreditedDepositCoverageGaps(ctx, scopeAsset)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	checks = append(checks, buildAuditCheck(
		"deposit_coverage",
		"充值入账具备账本映射",
		len(depositCoverage) == 0,
		fmt.Sprintf("%d", len(depositCoverage)),
		"CREDITED 的充值必须存在 credited_ledger_tx_id 且能关联到 ledger_tx。",
		depositCoverage,
	))

	withdrawCoverage, err := r.findWithdrawHoldCoverageGaps(ctx, scopeAsset)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	checks = append(checks, buildAuditCheck(
		"withdraw_hold_coverage",
		"提现冻结具备账本映射",
		len(withdrawCoverage) == 0,
		fmt.Sprintf("%d", len(withdrawCoverage)),
		"所有提现单必须能关联到冻结账本事务 hold_ledger_tx_id。",
		withdrawCoverage,
	))

	outboxCoverage, err := r.findLedgerOutboxCoverageGaps(ctx, scopeAsset)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	checks = append(checks, buildAuditCheck(
		"ledger_outbox_coverage",
		"ledger.committed outbox 完整",
		len(outboxCoverage) == 0,
		fmt.Sprintf("%d", len(outboxCoverage)),
		"每一笔 ledger_tx 都应存在对应的 ledger.committed outbox 事件。",
		outboxCoverage,
	))

	chainBalances, chainFailures, err := r.buildChainBalances(ctx, scopeAsset, filteredOverview)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	checks = append(checks, buildAuditCheck(
		"custody_chain_reconciliation",
		"链上余额对齐 custody mirror",
		len(chainFailures) == 0,
		fmt.Sprintf("%d", len(chainBalances)),
		"同一资产下，链上 vault 余额总和必须与账本 CUSTODY_HOT/WARM/COLD 汇总一致。",
		chainFailures,
	))

	status := "PASS"
	for _, check := range checks {
		if check.Status == "FAIL" {
			status = "FAIL"
			break
		}
	}

	finishedAt := time.Now().UTC()
	report := readmodel.LedgerAuditReport{
		AuditReportID: fmt.Sprintf("audit_%d", startedAt.UnixNano()),
		ScopeAsset:    scopeAsset,
		Status:        status,
		ExecutedBy:    executedBy,
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt.Format(time.RFC3339),
		Overview:      filteredOverview,
		ChainBalances: chainBalances,
		Checks:        checks,
	}

	overviewJSON, err := json.Marshal(report.Overview)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	var chainBalancesJSON *string
	if len(report.ChainBalances) > 0 {
		payload, marshalErr := json.Marshal(report.ChainBalances)
		if marshalErr != nil {
			return readmodel.LedgerAuditReport{}, marshalErr
		}
		value := string(payload)
		chainBalancesJSON = &value
	}
	checksJSON, err := json.Marshal(report.Checks)
	if err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	if err := DB(ctx, r.db).Create(&LedgerAuditReportModel{
		AuditReportID: report.AuditReportID,
		ScopeAsset:    report.ScopeAsset,
		Status:        report.Status,
		ExecutedBy:    report.ExecutedBy,
		OverviewJSON:  string(overviewJSON),
		ChainBalancesJSON: chainBalancesJSON,
		ChecksJSON:    string(checksJSON),
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		CreatedAt:     finishedAt,
	}).Error; err != nil {
		return readmodel.LedgerAuditReport{}, err
	}

	return report, nil
}

func (r *WalletQueryRepository) GetLatestLedgerAuditReport(ctx context.Context, scopeAsset string) (readmodel.LedgerAuditReport, error) {
	scopeAsset = normalizeAuditScopeAsset(scopeAsset)
	var model LedgerAuditReportModel
	query := DB(ctx, r.db).Order("created_at DESC")
	if scopeAsset != "ALL" {
		query = query.Where("scope_asset = ?", scopeAsset)
	}
	if err := query.First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return readmodel.LedgerAuditReport{}, errorsx.ErrNotFound
		}
		return readmodel.LedgerAuditReport{}, err
	}
	var overview []readmodel.LedgerAssetOverview
	if err := json.Unmarshal([]byte(model.OverviewJSON), &overview); err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	var chainBalances []readmodel.LedgerChainBalance
	if model.ChainBalancesJSON != nil && strings.TrimSpace(*model.ChainBalancesJSON) != "" {
		if err := json.Unmarshal([]byte(*model.ChainBalancesJSON), &chainBalances); err != nil {
			return readmodel.LedgerAuditReport{}, err
		}
	}
	var checks []readmodel.LedgerAuditCheck
	if err := json.Unmarshal([]byte(model.ChecksJSON), &checks); err != nil {
		return readmodel.LedgerAuditReport{}, err
	}
	return readmodel.LedgerAuditReport{
		AuditReportID: model.AuditReportID,
		ScopeAsset:    model.ScopeAsset,
		Status:        model.Status,
		ExecutedBy:    model.ExecutedBy,
		StartedAt:     model.StartedAt.Format(time.RFC3339),
		FinishedAt:    model.FinishedAt.Format(time.RFC3339),
		Overview:      overview,
		ChainBalances: chainBalances,
		Checks:        checks,
	}, nil
}

func aggregateLedgerOverview(rows []struct {
	AccountID   uint64
	AccountCode string
	Asset       string
	Balance     string
}) []readmodel.LedgerAssetOverview {
	byAsset := make(map[string]map[string]decimalx.Decimal)
	for _, row := range rows {
		if row.Asset == "" {
			continue
		}
		if _, ok := byAsset[row.Asset]; !ok {
			byAsset[row.Asset] = make(map[string]decimalx.Decimal)
		}
		current := byAsset[row.Asset][row.AccountCode]
		amount := decimalx.MustFromString(row.Balance)
		if current.IsZero() {
			byAsset[row.Asset][row.AccountCode] = amount
			continue
		}
		byAsset[row.Asset][row.AccountCode] = current.Add(amount)
	}

	assets := make([]string, 0, len(byAsset))
	for asset := range byAsset {
		assets = append(assets, asset)
	}
	sort.Strings(assets)

	out := make([]readmodel.LedgerAssetOverview, 0, len(assets))
	for _, asset := range assets {
		balances := byAsset[asset]
		userWallet := decimalFromMap(balances, "USER_WALLET")
		userOrderMargin := decimalFromMap(balances, "USER_ORDER_MARGIN")
		userPositionMargin := decimalFromMap(balances, "USER_POSITION_MARGIN")
		userWithdrawHold := decimalFromMap(balances, "USER_WITHDRAW_HOLD")
		systemPool := decimalFromMap(balances, "SYSTEM_POOL")
		tradingFee := decimalFromMap(balances, "TRADING_FEE_ACCOUNT")
		withdrawFee := decimalFromMap(balances, "WITHDRAW_FEE_ACCOUNT")
		penalty := decimalFromMap(balances, "PENALTY_ACCOUNT")
		fundingPool := decimalFromMap(balances, "FUNDING_POOL")
		insuranceFund := decimalFromMap(balances, "INSURANCE_FUND")
		roundingDiff := decimalFromMap(balances, "ROUNDING_DIFF_ACCOUNT")
		depositPending := decimalFromMap(balances, "DEPOSIT_PENDING_CONFIRM")
		withdrawInTransit := decimalFromMap(balances, "WITHDRAW_IN_TRANSIT")
		sweepInTransit := decimalFromMap(balances, "SWEEP_IN_TRANSIT")
		custodyHot := decimalFromMap(balances, "CUSTODY_HOT")
		custodyWarm := decimalFromMap(balances, "CUSTODY_WARM")
		custodyCold := decimalFromMap(balances, "CUSTODY_COLD")
		testFaucet := decimalFromMap(balances, "TEST_FAUCET_POOL")

		userMargin := userOrderMargin.Add(userPositionMargin)
		userLiability := userWallet.Add(userMargin).Add(userWithdrawHold)
		platformRevenue := tradingFee.Add(withdrawFee).Add(penalty)
		riskBuffer := fundingPool.Add(insuranceFund)
		inFlight := depositPending.Add(withdrawInTransit).Add(sweepInTransit)
		custodyMirror := custodyHot.Add(custodyWarm).Add(custodyCold)

		net := decimalx.MustFromString("0")
		for _, value := range balances {
			net = net.Add(value)
		}

		out = append(out, readmodel.LedgerAssetOverview{
			Asset:                 asset,
			UserWallet:            userWallet.String(),
			UserOrderMargin:       userOrderMargin.String(),
			UserPositionMargin:    userPositionMargin.String(),
			UserWithdrawHold:      userWithdrawHold.String(),
			UserMargin:            userMargin.String(),
			UserLiability:         userLiability.String(),
			SystemPool:            systemPool.String(),
			TradingFeeAccount:     tradingFee.String(),
			WithdrawFeeAccount:    withdrawFee.String(),
			PenaltyAccount:        penalty.String(),
			FundingPool:           fundingPool.String(),
			InsuranceFund:         insuranceFund.String(),
			RoundingDiffAccount:   roundingDiff.String(),
			DepositPendingConfirm: depositPending.String(),
			WithdrawInTransit:     withdrawInTransit.String(),
			SweepInTransit:        sweepInTransit.String(),
			CustodyHot:            custodyHot.String(),
			CustodyWarm:           custodyWarm.String(),
			CustodyCold:           custodyCold.String(),
			TestFaucetPool:        testFaucet.String(),
			PlatformRevenue:       platformRevenue.String(),
			RiskBuffer:            riskBuffer.String(),
			InFlight:              inFlight.String(),
			CustodyMirror:         custodyMirror.String(),
			NetBalance:            net.String(),
		})
	}
	return out
}

func decimalFromMap(values map[string]decimalx.Decimal, key string) decimalx.Decimal {
	if value, ok := values[key]; ok {
		return value
	}
	return decimalx.MustFromString("0")
}

func normalizeAuditScopeAsset(asset string) string {
	asset = strings.ToUpper(strings.TrimSpace(asset))
	if asset == "" {
		return "ALL"
	}
	return asset
}

func filterOverviewByScope(items []readmodel.LedgerAssetOverview, scopeAsset string) []readmodel.LedgerAssetOverview {
	if scopeAsset == "ALL" {
		return items
	}
	filtered := make([]readmodel.LedgerAssetOverview, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.Asset, scopeAsset) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (r *WalletQueryRepository) buildChainBalances(ctx context.Context, scopeAsset string, overview []readmodel.LedgerAssetOverview) ([]readmodel.LedgerChainBalance, []string, error) {
	if r.chainReader == nil {
		return nil, []string{"chain_reader_unavailable"}, nil
	}
	snapshots, err := r.chainReader.ListVaultBalances(ctx, scopeAsset)
	if err != nil {
		return nil, []string{fmt.Sprintf("chain_reader_error=%v", err)}, nil
	}

	custodyByAsset := make(map[string]decimalx.Decimal, len(overview))
	for _, item := range overview {
		custodyByAsset[item.Asset] = decimalx.MustFromString(item.CustodyMirror)
	}

	type assetAggregate struct {
		total decimalx.Decimal
		chains int
	}
	onchainByAsset := make(map[string]assetAggregate)
	rows := make([]readmodel.LedgerChainBalance, 0, len(snapshots)+len(overview))
	for _, snapshot := range snapshots {
		balance := decimalx.MustFromString(snapshot.Balance)
		aggregate := onchainByAsset[snapshot.Asset]
		aggregate.total = aggregate.total.Add(balance)
		aggregate.chains++
		onchainByAsset[snapshot.Asset] = aggregate
		rows = append(rows, readmodel.LedgerChainBalance{
			RowType:        "CHAIN",
			ChainID:        snapshot.ChainID,
			ChainKey:       snapshot.ChainKey,
			ChainName:      snapshot.ChainName,
			Asset:          snapshot.Asset,
			VaultAddress:   snapshot.VaultAddress,
			OnchainBalance: balance.String(),
			CustodyMirror:  "",
			Delta:          "",
			Status:         "PASS",
		})
	}

	failures := make([]string, 0)
	for _, item := range overview {
		onchainTotal := onchainByAsset[item.Asset].total
		custodyMirror := custodyByAsset[item.Asset]
		delta := onchainTotal.Add(custodyMirror)
		status := "PASS"
		if !delta.IsZero() {
			status = "FAIL"
			failures = append(failures, fmt.Sprintf("%s onchain=%s custody=%s delta=%s", item.Asset, onchainTotal.String(), custodyMirror.String(), delta.String()))
		}
		rows = append(rows, readmodel.LedgerChainBalance{
			RowType:        "TOTAL",
			ChainID:        0,
			ChainKey:       "TOTAL",
			ChainName:      "Total",
			Asset:          item.Asset,
			VaultAddress:   "",
			OnchainBalance: onchainTotal.String(),
			CustodyMirror:  custodyMirror.String(),
			Delta:          delta.String(),
			Status:         status,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Asset != rows[j].Asset {
			return rows[i].Asset < rows[j].Asset
		}
		if rows[i].RowType != rows[j].RowType {
			return rows[i].RowType < rows[j].RowType
		}
		if rows[i].ChainID != rows[j].ChainID {
			return rows[i].ChainID < rows[j].ChainID
		}
		return rows[i].ChainKey < rows[j].ChainKey
	})
	sort.Strings(failures)
	return rows, failures, nil
}

func buildAuditCheck(checkKey string, label string, pass bool, value string, summary string, sampleRefs []string) readmodel.LedgerAuditCheck {
	status := "PASS"
	if !pass {
		status = "FAIL"
	}
	if len(sampleRefs) > 5 {
		sampleRefs = sampleRefs[:5]
	}
	return readmodel.LedgerAuditCheck{
		CheckKey:   checkKey,
		Label:      label,
		Status:     status,
		Value:      value,
		Summary:    summary,
		SampleRefs: sampleRefs,
	}
}

func (r *WalletQueryRepository) findUnbalancedLedgerTx(ctx context.Context, scopeAsset string) ([]string, error) {
	var rows []struct {
		LedgerTxID string
		Asset      string
		Amount     string
	}
	query := DB(ctx, r.db).Table("ledger_entries").Select("ledger_tx_id, asset, amount")
	if scopeAsset != "ALL" {
		query = query.Where("asset = ?", scopeAsset)
	}
	if err := query.Order("ledger_tx_id ASC, id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	totals := make(map[string]decimalx.Decimal)
	for _, row := range rows {
		key := row.LedgerTxID + "|" + row.Asset
		current := totals[key]
		amount := decimalx.MustFromString(row.Amount)
		if current.IsZero() {
			totals[key] = amount
			continue
		}
		totals[key] = current.Add(amount)
	}
	out := make([]string, 0)
	for key, total := range totals {
		if !total.IsZero() {
			out = append(out, fmt.Sprintf("%s sum=%s", key, total.String()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func (r *WalletQueryRepository) findSnapshotMismatches(ctx context.Context, scopeAsset string) ([]string, error) {
	var entryRows []struct {
		AccountID uint64
		Asset     string
		Amount    string
	}
	entryQuery := DB(ctx, r.db).Table("ledger_entries").Select("account_id, asset, amount")
	if scopeAsset != "ALL" {
		entryQuery = entryQuery.Where("asset = ?", scopeAsset)
	}
	if err := entryQuery.Scan(&entryRows).Error; err != nil {
		return nil, err
	}

	replay := make(map[string]decimalx.Decimal)
	for _, row := range entryRows {
		key := fmt.Sprintf("%d|%s", row.AccountID, row.Asset)
		current := replay[key]
		amount := decimalx.MustFromString(row.Amount)
		if current.IsZero() {
			replay[key] = amount
			continue
		}
		replay[key] = current.Add(amount)
	}

	var snapshotRows []struct {
		AccountID   uint64
		AccountCode string
		Asset       string
		Balance     string
	}
	snapshotQuery := DB(ctx, r.db).
		Table("accounts").
		Select("accounts.id AS account_id, accounts.account_code, accounts.asset, COALESCE(account_balance_snapshots.balance, '0') AS balance").
		Joins("LEFT JOIN account_balance_snapshots ON account_balance_snapshots.account_id = accounts.id AND account_balance_snapshots.asset = accounts.asset")
	if scopeAsset != "ALL" {
		snapshotQuery = snapshotQuery.Where("accounts.asset = ?", scopeAsset)
	}
	if err := snapshotQuery.Scan(&snapshotRows).Error; err != nil {
		return nil, err
	}

	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, row := range snapshotRows {
		key := fmt.Sprintf("%d|%s", row.AccountID, row.Asset)
		seen[key] = struct{}{}
		snapshotBalance := decimalx.MustFromString(row.Balance)
		replayBalance := replay[key]
		if !snapshotBalance.Equal(replayBalance) {
			out = append(out, fmt.Sprintf("%s:%s snapshot=%s replay=%s", row.Asset, row.AccountCode, snapshotBalance.String(), replayBalance.String()))
		}
	}
	for key, replayBalance := range replay {
		if _, ok := seen[key]; ok || replayBalance.IsZero() {
			continue
		}
		out = append(out, fmt.Sprintf("%s missing_snapshot replay=%s", key, replayBalance.String()))
	}
	sort.Strings(out)
	return out, nil
}

func (r *WalletQueryRepository) findCreditedDepositCoverageGaps(ctx context.Context, scopeAsset string) ([]string, error) {
	if scopeAsset != "ALL" && scopeAsset != "USDC" {
		return nil, nil
	}
	var rows []struct {
		DepositID          string
		CreditedLedgerTxID string
	}
	if err := DB(ctx, r.db).
		Table("deposit_chain_txs").
		Select("deposit_chain_txs.deposit_id, deposit_chain_txs.credited_ledger_tx_id").
		Joins("LEFT JOIN ledger_tx ON ledger_tx.ledger_tx_id = deposit_chain_txs.credited_ledger_tx_id").
		Where("deposit_chain_txs.status = ? AND (deposit_chain_txs.credited_ledger_tx_id = '' OR deposit_chain_txs.credited_ledger_tx_id IS NULL OR ledger_tx.ledger_tx_id IS NULL)", "CREDITED").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, fmt.Sprintf("%s credited_ledger_tx_id=%s", row.DepositID, row.CreditedLedgerTxID))
	}
	sort.Strings(out)
	return out, nil
}

func (r *WalletQueryRepository) findWithdrawHoldCoverageGaps(ctx context.Context, scopeAsset string) ([]string, error) {
	var rows []struct {
		WithdrawID     string
		HoldLedgerTxID string
	}
	query := DB(ctx, r.db).
		Table("withdraw_requests").
		Select("withdraw_requests.withdraw_id, withdraw_requests.hold_ledger_tx_id").
		Joins("LEFT JOIN ledger_tx ON ledger_tx.ledger_tx_id = withdraw_requests.hold_ledger_tx_id")
	if scopeAsset != "ALL" {
		query = query.Where("withdraw_requests.asset = ?", scopeAsset)
	}
	if err := query.Where("withdraw_requests.hold_ledger_tx_id = '' OR ledger_tx.ledger_tx_id IS NULL").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, fmt.Sprintf("%s hold_ledger_tx_id=%s", row.WithdrawID, row.HoldLedgerTxID))
	}
	sort.Strings(out)
	return out, nil
}

func (r *WalletQueryRepository) findLedgerOutboxCoverageGaps(ctx context.Context, scopeAsset string) ([]string, error) {
	var rows []struct {
		LedgerTxID string
		BizType    string
	}
	query := DB(ctx, r.db).
		Table("ledger_tx").
		Select("ledger_tx.ledger_tx_id, ledger_tx.biz_type").
		Joins("LEFT JOIN outbox_events ON outbox_events.aggregate_type = 'ledger_tx' AND outbox_events.aggregate_id = ledger_tx.ledger_tx_id AND outbox_events.event_type = 'ledger.committed'")
	if scopeAsset != "ALL" {
		query = query.Where("ledger_tx.asset = ?", scopeAsset)
	}
	if err := query.Where("outbox_events.id IS NULL").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, fmt.Sprintf("%s biz_type=%s", row.LedgerTxID, row.BizType))
	}
	sort.Strings(out)
	return out, nil
}
