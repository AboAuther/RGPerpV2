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
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/exposurex"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BalanceRepository struct {
	db *gorm.DB
}

func NewBalanceRepository(db *gorm.DB) *BalanceRepository {
	return &BalanceRepository{db: db}
}

func (r *BalanceRepository) GetAccountBalance(ctx context.Context, accountID uint64, asset string) (string, error) {
	return r.GetAccountBalanceForUpdate(ctx, accountID, asset)
}

func (r *BalanceRepository) GetAccountBalanceForUpdate(ctx context.Context, accountID uint64, asset string) (string, error) {
	balances, err := r.GetAccountBalancesForUpdate(ctx, []uint64{accountID}, asset)
	if err != nil {
		return "", err
	}
	return balances[accountID], nil
}

func (r *BalanceRepository) GetAccountBalancesForUpdate(ctx context.Context, accountIDs []uint64, asset string) (map[uint64]string, error) {
	tx := DB(ctx, r.db)
	out := make(map[uint64]string, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}

	var accounts []AccountModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", accountIDs).
		Order("id ASC").
		Find(&accounts).Error; err != nil {
		return nil, err
	}
	if len(accounts) != len(accountIDs) {
		seen := make(map[uint64]struct{}, len(accounts))
		for _, account := range accounts {
			seen[account.ID] = struct{}{}
		}
		for _, accountID := range accountIDs {
			if _, ok := seen[accountID]; !ok {
				return nil, errorsx.ErrNotFound
			}
		}
	}
	for _, accountID := range accountIDs {
		out[accountID] = "0"
	}

	var snapshots []AccountBalanceSnapshotModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id IN ? AND asset = ?", accountIDs, asset).
		Order("account_id ASC").
		Find(&snapshots).Error; err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		out[snapshot.AccountID] = snapshot.Balance
	}
	return out, nil
}

type DepositAddressRepository struct {
	db            *gorm.DB
	confirmations map[int64]int
}

func NewDepositAddressRepository(db *gorm.DB, confirmations map[int64]int) *DepositAddressRepository {
	return &DepositAddressRepository{db: db, confirmations: cloneConfirmations(confirmations)}
}

func (r *DepositAddressRepository) ListByUser(ctx context.Context, userID uint64) ([]walletdomain.DepositAddress, error) {
	var models []DepositAddressModel
	query := DB(ctx, r.db).Where("user_id = ?", userID)
	if len(r.confirmations) > 0 {
		chainIDs := make([]int64, 0, len(r.confirmations))
		for chainID, confirmations := range r.confirmations {
			if confirmations > 0 {
				chainIDs = append(chainIDs, chainID)
			}
		}
		if len(chainIDs) > 0 {
			query = query.Where("chain_id IN ?", chainIDs)
		}
	}
	if err := query.Order("chain_id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]walletdomain.DepositAddress, 0, len(models))
	for _, model := range models {
		items = append(items, walletdomain.DepositAddress{
			UserID:        model.UserID,
			ChainID:       model.ChainID,
			Asset:         model.Asset,
			Address:       model.Address,
			Status:        model.Status,
			Confirmations: r.confirmations[model.ChainID],
			CreatedAt:     model.CreatedAt,
		})
	}
	return items, nil
}

func (r *DepositAddressRepository) GetByUserChainAsset(ctx context.Context, userID uint64, chainID int64, asset string) (walletdomain.DepositAddress, error) {
	var model DepositAddressModel
	err := DB(ctx, r.db).
		Where("user_id = ? AND chain_id = ? AND asset = ?", userID, chainID, asset).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return walletdomain.DepositAddress{}, errorsx.ErrNotFound
		}
		return walletdomain.DepositAddress{}, err
	}
	return walletdomain.DepositAddress{
		UserID:        model.UserID,
		ChainID:       model.ChainID,
		Asset:         model.Asset,
		Address:       model.Address,
		Status:        model.Status,
		Confirmations: r.confirmations[model.ChainID],
		CreatedAt:     model.CreatedAt,
	}, nil
}

func (r *DepositAddressRepository) Upsert(ctx context.Context, address walletdomain.DepositAddress) error {
	now := time.Now().UTC()
	model := DepositAddressModel{
		UserID:    address.UserID,
		ChainID:   address.ChainID,
		Address:   address.Address,
		Asset:     address.Asset,
		Status:    address.Status,
		CreatedAt: address.CreatedAt,
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = now
	}
	return DB(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "chain_id"},
			{Name: "asset"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"address": address.Address,
			"status":  address.Status,
		}),
	}).Create(&model).Error
}

type AccountQueryRepository struct {
	db      *gorm.DB
	runtime RiskRuntimeConfigProvider
}

func NewAccountQueryRepository(db *gorm.DB) *AccountQueryRepository {
	return &AccountQueryRepository{db: db}
}

type RiskRuntimeConfigProvider interface {
	CurrentRiskRuntimeConfig() riskdomain.ServiceConfig
}

func NewAccountQueryRepositoryWithRuntime(db *gorm.DB, runtime RiskRuntimeConfigProvider) *AccountQueryRepository {
	return &AccountQueryRepository{db: db, runtime: runtime}
}

func (r *AccountQueryRepository) ListBalances(ctx context.Context, userID uint64) ([]readmodel.BalanceItem, error) {
	var rows []struct {
		AccountCode string
		Asset       string
		Balance     string
	}
	if err := DB(ctx, r.db).
		Table("accounts").
		Select("accounts.account_code, accounts.asset, COALESCE(account_balance_snapshots.balance, '0') AS balance").
		Joins("LEFT JOIN account_balance_snapshots ON account_balance_snapshots.account_id = accounts.id AND account_balance_snapshots.asset = accounts.asset").
		Where("accounts.user_id = ?", userID).
		Order("accounts.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]readmodel.BalanceItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, readmodel.BalanceItem{
			AccountCode: row.AccountCode,
			Asset:       row.Asset,
			Balance:     row.Balance,
		})
	}
	return out, nil
}

func (r *AccountQueryRepository) GetSummary(ctx context.Context, userID uint64) (readmodel.AccountSummary, error) {
	state, err := loadRiskAccountState(ctx, DB(ctx, r.db), userID, false)
	if err != nil {
		return readmodel.AccountSummary{}, err
	}
	metrics := riskdomain.ComputeAccountMetrics(state, r.currentRiskConfig())
	return readmodel.AccountSummary{
		Equity:                 metrics.Equity,
		AvailableBalance:       metrics.AvailableBalance,
		TotalInitialMargin:     metrics.InitialMargin,
		TotalMaintenanceMargin: metrics.MaintenanceMargin,
		UnrealizedPnL:          metrics.UnrealizedPnL,
		MarginRatio:            metrics.MarginRatio,
	}, nil
}

func (r *AccountQueryRepository) GetRisk(ctx context.Context, userID uint64) (readmodel.RiskSnapshot, error) {
	state, stateErr := loadRiskAccountState(ctx, DB(ctx, r.db), userID, false)
	cfg := r.currentRiskConfig()
	markIssue, markIssueNote := currentMarkPriceIssue(state.Positions, cfg, time.Now().UTC())
	var latest RiskSnapshotModel
	err := DB(ctx, r.db).
		Order("id DESC").
		Where("user_id = ?", userID).
		First(&latest).Error
	if err == nil {
		riskState := "SAFE"
		canOpenRisk := true
		notes := []string{
			fmt.Sprintf("最新风险快照权益=%s，维持保证金=%s，风险率=%s。", latest.Equity, latest.MaintenanceMargin, latest.MarginRatio),
		}
		switch latest.RiskLevel {
		case "NO_NEW_RISK":
			riskState = "NO_NEW_RISK"
			canOpenRisk = false
			notes = append(notes, "账户已进入 NO_NEW_RISK，禁止新增风险，仅允许减仓或补充保证金。")
		case "LIQUIDATING":
			riskState = "LIQUIDATING"
			canOpenRisk = false
			notes = append(notes, "账户已进入强平流程，禁止新增风险。")
		case "SAFE":
			notes = append(notes, "账户风险处于安全区间。")
		default:
			riskState = latest.RiskLevel
		}
		if markIssue {
			canOpenRisk = false
			notes = append(notes, markIssueNote)
		}
		return readmodel.RiskSnapshot{
			AccountStatus:  "ACTIVE",
			RiskState:      riskState,
			MarkPriceStale: markIssue,
			CanOpenRisk:    canOpenRisk,
			Notes:          notes,
		}, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return readmodel.RiskSnapshot{}, err
	}

	if stateErr != nil {
		if err != nil {
			return readmodel.RiskSnapshot{}, err
		}
		return readmodel.RiskSnapshot{}, stateErr
	}
	metrics := riskdomain.ComputeAccountMetrics(state, cfg)
	risk := readmodel.RiskSnapshot{
		AccountStatus:  "ACTIVE",
		RiskState:      metrics.RiskLevel,
		MarkPriceStale: markIssue,
		CanOpenRisk:    !markIssue && metrics.RiskLevel == riskdomain.RiskLevelSafe,
		Notes: []string{
			fmt.Sprintf("当前风险视图权益=%s，维持保证金=%s，风险率=%s。", metrics.Equity, metrics.MaintenanceMargin, metrics.MarginRatio),
		},
	}
	if metrics.RiskLevel == riskdomain.RiskLevelNoNewRisk {
		risk.Notes = append(risk.Notes, "账户已进入 NO_NEW_RISK，禁止新增风险，仅允许减仓或补充保证金。")
	}
	if metrics.RiskLevel == riskdomain.RiskLevelLiquidating {
		risk.CanOpenRisk = false
		risk.Notes = append(risk.Notes, "账户已进入强平流程，禁止新增风险。")
	}
	if metrics.RiskLevel == riskdomain.RiskLevelSafe {
		risk.Notes = append(risk.Notes, "账户风险处于安全区间。")
	}
	if markIssue {
		risk.CanOpenRisk = false
		risk.Notes = append(risk.Notes, markIssueNote)
	}
	return risk, nil
}

func (r *AccountQueryRepository) currentRiskConfig() riskdomain.ServiceConfig {
	if r == nil || r.runtime == nil {
		return riskdomain.ServiceConfig{
			RiskBufferRatio:             "0",
			SoftThresholdRatio:          "0.2",
			HardThresholdRatio:          "0.4",
			TakerFeeRate:                "0",
			ForceReduceOnlyOnStalePrice: true,
		}
	}
	cfg := r.runtime.CurrentRiskRuntimeConfig()
	if cfg.RiskBufferRatio == "" {
		cfg.RiskBufferRatio = "0"
	}
	if cfg.SoftThresholdRatio == "" {
		cfg.SoftThresholdRatio = "0.2"
	}
	if cfg.HardThresholdRatio == "" {
		cfg.HardThresholdRatio = "0.4"
	}
	if cfg.TakerFeeRate == "" {
		cfg.TakerFeeRate = "0"
	}
	return cfg
}

func currentMarkPriceIssue(positions []riskdomain.PositionExposure, cfg riskdomain.ServiceConfig, now time.Time) (bool, string) {
	if len(positions) == 0 {
		return false, ""
	}
	staleThreshold := time.Duration(cfg.MarkPriceStaleSec) * time.Second
	for _, position := range positions {
		if !decimalx.MustFromString(position.MarkPrice).GreaterThan(decimalx.MustFromString("0")) {
			return true, fmt.Sprintf("当前 %s 的 MARK PRICE（标记价格） 无效，系统已禁止新增风险，仅允许 REDUCE_ONLY（只减仓）。", position.Symbol)
		}
		if staleThreshold > 0 && (position.MarkPriceUpdatedAt.IsZero() || now.Sub(position.MarkPriceUpdatedAt) > staleThreshold) && cfg.ForceReduceOnlyOnStalePrice {
			return true, fmt.Sprintf("当前 %s 的 MARK PRICE（标记价格） 已过期，系统已禁止新增风险，仅允许 REDUCE_ONLY（只减仓）。", position.Symbol)
		}
	}
	return false, ""
}

func (r *AccountQueryRepository) ListFunding(ctx context.Context, userID uint64) ([]readmodel.FundingItem, error) {
	return NewTradingReadRepository(r.db).ListFunding(ctx, userID)
}

func (r *AccountQueryRepository) ListTransfers(ctx context.Context, userID uint64) ([]readmodel.TransferItem, error) {
	var txs []LedgerTxModel
	if err := DB(ctx, r.db).
		Where(`
			biz_type = ?
			AND (
				operator_id = ?
				OR EXISTS (
					SELECT 1
					FROM ledger_entries
					JOIN accounts ON accounts.id = ledger_entries.account_id
					WHERE ledger_entries.ledger_tx_id = ledger_tx.ledger_tx_id
						AND (
							ledger_entries.user_id = ?
							OR accounts.user_id = ?
						)
				)
			)
		`, "TRANSFER", fmt.Sprintf("%d", userID), userID, userID).
		Order("created_at DESC").
		Find(&txs).Error; err != nil {
		return nil, err
	}

	out := make([]readmodel.TransferItem, 0, len(txs))
	for _, tx := range txs {
		direction, counterpartyAddress, fromAccount, toAccount := describeTransferForUser(ctx, r.db, tx.LedgerTxID, userID)
		out = append(out, readmodel.TransferItem{
			TransferID:          tx.BizRefID,
			Asset:               tx.Asset,
			Amount:              extractPositiveTransferAmount(ctx, r.db, tx.LedgerTxID),
			Direction:           direction,
			CounterpartyAddress: counterpartyAddress,
			FromAccount:         fromAccount,
			ToAccount:           toAccount,
			Status:              tx.Status,
			CreatedAt:           tx.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

type WalletQueryRepository struct {
	db            *gorm.DB
	confirmations map[int64]int
	riskMonitor   RiskMonitorConfig
	chainReader   interface {
		ListVaultBalances(ctx context.Context, scopeAsset string) ([]chaininfra.VaultBalanceSnapshot, error)
	}
}

type RiskMonitorConfig struct {
	HardLimitNotional      string
	MaxExposureSlippageBps int
}

func NewWalletQueryRepository(db *gorm.DB, confirmations map[int64]int, chainReaders ...interface {
	ListVaultBalances(ctx context.Context, scopeAsset string) ([]chaininfra.VaultBalanceSnapshot, error)
}) *WalletQueryRepository {
	var chainReader interface {
		ListVaultBalances(ctx context.Context, scopeAsset string) ([]chaininfra.VaultBalanceSnapshot, error)
	}
	if len(chainReaders) > 0 {
		chainReader = chainReaders[0]
	}
	return &WalletQueryRepository{db: db, confirmations: cloneConfirmations(confirmations), chainReader: chainReader}
}

func NewWalletQueryRepositoryWithRiskConfig(db *gorm.DB, confirmations map[int64]int, riskCfg RiskMonitorConfig, chainReaders ...interface {
	ListVaultBalances(ctx context.Context, scopeAsset string) ([]chaininfra.VaultBalanceSnapshot, error)
}) *WalletQueryRepository {
	repo := NewWalletQueryRepository(db, confirmations, chainReaders...)
	repo.riskMonitor = riskCfg
	return repo
}

func (r *WalletQueryRepository) GetRiskMonitorDashboard(ctx context.Context) (readmodel.RiskMonitorDashboard, error) {
	var symbolRows []struct {
		ID                 uint64 `gorm:"column:id"`
		Symbol             string `gorm:"column:symbol"`
		Status             string `gorm:"column:status"`
		ContractMultiplier string `gorm:"column:contract_multiplier"`
	}
	if err := DB(ctx, r.db).
		Table("symbols").
		Select("id, symbol, status, contract_multiplier").
		Where("status IN ?", []string{"TRADING", "REDUCE_ONLY", "PAUSED"}).
		Order("symbol ASC").
		Scan(&symbolRows).Error; err != nil {
		return readmodel.RiskMonitorDashboard{}, err
	}

	var markRows []struct {
		SymbolID  uint64 `gorm:"column:symbol_id"`
		MarkPrice string `gorm:"column:mark_price"`
	}
	if err := DB(ctx, r.db).Raw(`
SELECT m1.symbol_id, m1.mark_price
FROM mark_price_snapshots m1
JOIN (
  SELECT symbol_id, MAX(id) AS max_id
  FROM mark_price_snapshots
  GROUP BY symbol_id
) latest ON latest.max_id = m1.id
`).Scan(&markRows).Error; err != nil {
		return readmodel.RiskMonitorDashboard{}, err
	}
	marksBySymbol := make(map[uint64]string, len(markRows))
	for _, row := range markRows {
		marksBySymbol[row.SymbolID] = row.MarkPrice
	}

	var positionRows []struct {
		SymbolID uint64 `gorm:"column:symbol_id"`
		Side     string `gorm:"column:side"`
		Qty      string `gorm:"column:qty"`
	}
	if err := DB(ctx, r.db).
		Table("positions").
		Select("symbol_id, side, qty").
		Where("status = ?", "OPEN").
		Scan(&positionRows).Error; err != nil {
		return readmodel.RiskMonitorDashboard{}, err
	}
	exposuresBySymbol := make(map[uint64]struct {
		LongQty  string
		ShortQty string
	}, len(symbolRows))
	for _, row := range positionRows {
		current := exposuresBySymbol[row.SymbolID]
		if current.LongQty == "" {
			current.LongQty = "0"
		}
		if current.ShortQty == "" {
			current.ShortQty = "0"
		}
		if row.Side == "LONG" {
			current.LongQty = decimalAdd(current.LongQty, row.Qty)
		} else {
			current.ShortQty = decimalAdd(current.ShortQty, row.Qty)
		}
		exposuresBySymbol[row.SymbolID] = current
	}

	limit := r.riskMonitor.HardLimitNotional
	if strings.TrimSpace(limit) == "" {
		limit = "0"
	}
	items := make([]readmodel.SymbolNetExposureItem, 0, len(symbolRows))
	for _, symbol := range symbolRows {
		exposure := exposuresBySymbol[symbol.ID]
		longQty := exposure.LongQty
		shortQty := exposure.ShortQty
		if longQty == "" {
			longQty = "0"
		}
		if shortQty == "" {
			shortQty = "0"
		}
		markPrice := marksBySymbol[symbol.ID]
		if markPrice == "" {
			markPrice = "0"
		}
		netQty := exposurex.SignedNetQty(longQty, shortQty)
		netNotional := exposurex.SignedNetNotional(longQty, shortQty, markPrice, symbol.ContractMultiplier)
		utilizationRatio := "0"
		limitDecimal := decimalx.MustFromString(limit)
		if limitDecimal.GreaterThan(decimalx.MustFromString("0")) {
			utilizationRatio = netNotional.Abs().Div(limitDecimal).String()
		}
		var blockedOpenSide *string
		if limitDecimal.GreaterThan(decimalx.MustFromString("0")) && netNotional.Abs().GreaterThanOrEqual(limitDecimal) {
			side := "BUY"
			if netQty.LessThan(decimalx.MustFromString("0")) {
				side = "SELL"
			}
			blockedOpenSide = &side
		}
		items = append(items, readmodel.SymbolNetExposureItem{
			Symbol:            symbol.Symbol,
			Status:            symbol.Status,
			MarkPrice:         markPrice,
			LongQty:           longQty,
			ShortQty:          shortQty,
			NetQty:            netQty.String(),
			NetNotional:       netNotional.String(),
			HardLimitNotional: limit,
			UtilizationRatio:  utilizationRatio,
			BlockedOpenSide:   blockedOpenSide,
			BuyAdjustmentBps:  exposurex.DirectionAdjustmentBps(netNotional, "BUY", limit, r.riskMonitor.MaxExposureSlippageBps),
			SellAdjustmentBps: exposurex.DirectionAdjustmentBps(netNotional, "SELL", limit, r.riskMonitor.MaxExposureSlippageBps),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := decimalx.MustFromString(items[i].UtilizationRatio).Abs()
		right := decimalx.MustFromString(items[j].UtilizationRatio).Abs()
		if left.Equal(right) {
			return items[i].Symbol < items[j].Symbol
		}
		return left.GreaterThan(right)
	})

	return readmodel.RiskMonitorDashboard{
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		HardLimitNotional:     limit,
		MaxDynamicSlippageBps: r.riskMonitor.MaxExposureSlippageBps,
		Items:                 items,
	}, nil
}

type WalletReadService struct {
	addresses *DepositAddressRepository
	wallets   *WalletQueryRepository
	allocator walletdomain.DepositAddressAllocator
}

func NewWalletReadService(addresses *DepositAddressRepository, wallets *WalletQueryRepository, allocator ...walletdomain.DepositAddressAllocator) *WalletReadService {
	var optionalAllocator walletdomain.DepositAddressAllocator
	if len(allocator) > 0 {
		optionalAllocator = allocator[0]
	}
	return &WalletReadService{
		addresses: addresses,
		wallets:   wallets,
		allocator: optionalAllocator,
	}
}

func (s *WalletReadService) ListDepositAddresses(ctx context.Context, userID uint64) ([]walletdomain.DepositAddress, error) {
	items, err := s.addresses.ListByUser(ctx, userID)
	if err != nil || s.allocator == nil {
		return items, err
	}

	filtered := make([]walletdomain.DepositAddress, 0, len(items))
	for _, item := range items {
		canonical, valid, validateErr := s.allocator.Validate(ctx, item.UserID, item.ChainID, item.Asset, item.Address)
		if validateErr != nil || !valid {
			continue
		}
		if canonical != "" && canonical != item.Address {
			item.Address = canonical
			if err := s.addresses.Upsert(ctx, item); err != nil {
				return nil, err
			}
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *WalletReadService) ListDeposits(ctx context.Context, userID uint64) ([]readmodel.DepositItem, error) {
	return s.wallets.ListDeposits(ctx, userID)
}

func (s *WalletReadService) ListWithdrawals(ctx context.Context, userID uint64) ([]readmodel.WithdrawItem, error) {
	return s.wallets.ListWithdrawals(ctx, userID)
}

func (r *WalletQueryRepository) ListDeposits(ctx context.Context, userID uint64) ([]readmodel.DepositItem, error) {
	var models []DepositChainTxModel
	if err := DB(ctx, r.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.DepositItem, 0, len(models))
	for _, model := range models {
		items = append(items, readmodel.DepositItem{
			DepositID:             model.DepositID,
			ChainID:               model.ChainID,
			Asset:                 "USDC",
			Amount:                model.Amount,
			TxHash:                model.TxHash,
			Confirmations:         model.Confirmations,
			RequiredConfirmations: r.requiredConfirmations(model.ChainID),
			Status:                model.Status,
			Address:               model.ToAddress,
			DetectedAt:            model.CreatedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func (r *WalletQueryRepository) ListWithdrawals(ctx context.Context, userID uint64) ([]readmodel.WithdrawItem, error) {
	var models []WithdrawRequestModel
	if err := DB(ctx, r.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.WithdrawItem, 0, len(models))
	for _, model := range models {
		var txHash *string
		if model.BroadcastTxHash != "" {
			hash := model.BroadcastTxHash
			txHash = &hash
		}
		items = append(items, readmodel.WithdrawItem{
			WithdrawID: model.WithdrawID,
			ChainID:    model.ChainID,
			Asset:      model.Asset,
			Amount:     model.Amount,
			FeeAmount:  model.FeeAmount,
			ToAddress:  model.ToAddress,
			Status:     model.Status,
			TxHash:     txHash,
			CreatedAt:  model.CreatedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func (r *WalletQueryRepository) ListAdminWithdrawals(ctx context.Context, limit int) ([]readmodel.AdminWithdrawReviewItem, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []struct {
		WithdrawRequestModel
		UserAddress string
	}
	if err := DB(ctx, r.db).
		Table("withdraw_requests").
		Select("withdraw_requests.*, users.evm_address AS user_address").
		Joins("JOIN users ON users.id = withdraw_requests.user_id").
		Order("CASE withdraw_requests.status WHEN 'RISK_REVIEW' THEN 0 WHEN 'FAILED' THEN 1 WHEN 'SIGNING' THEN 2 ELSE 3 END ASC, withdraw_requests.created_at DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.AdminWithdrawReviewItem, 0, len(rows))
	for _, row := range rows {
		var txHash *string
		if row.BroadcastTxHash != "" {
			value := row.BroadcastTxHash
			txHash = &value
		}
		items = append(items, readmodel.AdminWithdrawReviewItem{
			WithdrawID:  row.WithdrawID,
			UserID:      row.UserID,
			UserAddress: row.UserAddress,
			ChainID:     row.ChainID,
			Asset:       row.Asset,
			Amount:      row.Amount,
			FeeAmount:   row.FeeAmount,
			ToAddress:   row.ToAddress,
			Status:      row.Status,
			RiskFlag:    row.RiskFlag,
			TxHash:      txHash,
			CreatedAt:   row.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   row.UpdatedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func (r *WalletQueryRepository) ListAdminLiquidations(ctx context.Context, limit int) ([]readmodel.AdminLiquidationItem, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []struct {
		LiquidationModel
		UserAddress   string  `gorm:"column:user_address"`
		Symbol        *string `gorm:"column:symbol"`
		PositionCount int     `gorm:"column:position_count"`
	}
	if err := DB(ctx, r.db).
		Table("liquidations").
		Select(`
			liquidations.*,
			users.evm_address AS user_address,
			symbols.symbol AS symbol,
			(
				SELECT COUNT(*)
				FROM liquidation_items
				WHERE liquidation_items.liquidation_id = liquidations.liquidation_id
			) AS position_count
		`).
		Joins("JOIN users ON users.id = liquidations.user_id").
		Joins("LEFT JOIN symbols ON symbols.id = liquidations.symbol_id").
		Order("liquidations.created_at DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.AdminLiquidationItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, readmodel.AdminLiquidationItem{
			LiquidationID:         row.LiquidationID,
			UserID:                row.UserID,
			UserAddress:           row.UserAddress,
			Symbol:                row.Symbol,
			Mode:                  row.Mode,
			Status:                row.Status,
			TriggerRiskSnapshotID: row.TriggerRiskSnapshotID,
			PositionCount:         row.PositionCount,
			PenaltyAmount:         row.PenaltyAmount,
			InsuranceFundUsed:     row.InsuranceFundUsed,
			BankruptAmount:        row.BankruptAmount,
			AbortReason:           row.AbortReason,
			CreatedAt:             row.CreatedAt.Format(time.RFC3339),
			UpdatedAt:             row.UpdatedAt.Format(time.RFC3339),
		})
	}
	return items, nil
}

func (r *WalletQueryRepository) ListFundingBatches(ctx context.Context, limit int) ([]readmodel.AdminFundingBatchItem, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []struct {
		FundingBatchModel
		Symbol        string `gorm:"column:symbol"`
		AppliedCount  int    `gorm:"column:applied_count"`
		FailedCount   int    `gorm:"column:failed_count"`
		ReversedCount int    `gorm:"column:reversed_count"`
	}
	if err := DB(ctx, r.db).
		Table("funding_batches").
		Select(`
			funding_batches.*,
			symbols.symbol,
			SUM(CASE WHEN funding_batch_items.status = 'APPLIED' THEN 1 ELSE 0 END) AS applied_count,
			SUM(CASE WHEN funding_batch_items.status = 'FAILED' THEN 1 ELSE 0 END) AS failed_count,
			SUM(CASE WHEN funding_batch_items.status = 'REVERSED' THEN 1 ELSE 0 END) AS reversed_count
		`).
		Joins("JOIN symbols ON symbols.id = funding_batches.symbol_id").
		Joins("LEFT JOIN funding_batch_items ON funding_batch_items.funding_batch_id = funding_batches.funding_batch_id").
		Group("funding_batches.id, symbols.symbol").
		Order("funding_batches.time_window_end DESC, funding_batches.id DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.AdminFundingBatchItem, 0, len(rows))
	for _, row := range rows {
		var reversedAt *string
		if row.ReversedAt != nil {
			value := row.ReversedAt.UTC().Format(time.RFC3339)
			reversedAt = &value
		}
		items = append(items, readmodel.AdminFundingBatchItem{
			FundingBatchID:  row.FundingBatchID,
			Symbol:          row.Symbol,
			TimeWindowStart: row.TimeWindowStart.UTC().Format(time.RFC3339),
			TimeWindowEnd:   row.TimeWindowEnd.UTC().Format(time.RFC3339),
			NormalizedRate:  row.NormalizedRate,
			SettlementPrice: row.SettlementPrice,
			Status:          row.Status,
			AppliedCount:    row.AppliedCount,
			FailedCount:     row.FailedCount,
			ReversedCount:   row.ReversedCount,
			ReversedAt:      reversedAt,
			ReversedBy:      row.ReversedBy,
			ReversalReason:  row.ReversalReason,
			CreatedAt:       row.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:       row.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return items, nil
}

type ExplorerQueryRepository struct {
	db *gorm.DB
}

func NewExplorerQueryRepository(db *gorm.DB) *ExplorerQueryRepository {
	return &ExplorerQueryRepository{db: db}
}

func (r *ExplorerQueryRepository) ListEvents(ctx context.Context, userID uint64, isAdmin bool, filter readmodel.ExplorerEventFilter) ([]readmodel.ExplorerEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	var rows []struct {
		OutboxEventModel
		Asset          string
		BizType        string
		BizRefID       string
		OrderUserID    *uint64
		FillUserID     *uint64
		PositionUserID *uint64
	}
	query := DB(ctx, r.db).Table("outbox_events").
		Select(`
			outbox_events.*,
			ledger_tx.asset,
			ledger_tx.biz_type,
			ledger_tx.biz_ref_id,
			orders.user_id AS order_user_id,
			fills.user_id AS fill_user_id,
			positions.user_id AS position_user_id
		`).
		Joins("LEFT JOIN ledger_tx ON outbox_events.aggregate_type = 'ledger_tx' AND ledger_tx.ledger_tx_id = outbox_events.aggregate_id").
		Joins("LEFT JOIN orders ON outbox_events.aggregate_type = 'order' AND orders.order_id = outbox_events.aggregate_id").
		Joins("LEFT JOIN fills ON outbox_events.aggregate_type = 'fill' AND fills.fill_id = outbox_events.aggregate_id").
		Joins("LEFT JOIN positions ON outbox_events.aggregate_type = 'position' AND positions.position_id = outbox_events.aggregate_id")
	if filter.EventType != "" {
		query = query.Where("outbox_events.event_type = ?", filter.EventType)
	}
	if filter.Asset != "" {
		query = query.Where("(ledger_tx.asset = ? OR LOWER(outbox_events.payload_json) LIKE ?)", filter.Asset, "%\"asset\":\""+strings.ToLower(filter.Asset)+"\"%")
	}
	if filter.Query != "" {
		like := "%" + strings.ToLower(filter.Query) + "%"
		query = query.Where(`
			LOWER(outbox_events.event_id) LIKE ?
			OR LOWER(outbox_events.event_type) LIKE ?
			OR LOWER(outbox_events.aggregate_id) LIKE ?
			OR LOWER(COALESCE(ledger_tx.ledger_tx_id, '')) LIKE ?
			OR LOWER(COALESCE(ledger_tx.biz_ref_id, '')) LIKE ?
			OR LOWER(COALESCE(ledger_tx.asset, '')) LIKE ?
			OR LOWER(outbox_events.payload_json) LIKE ?
		`, like, like, like, like, like, like, like)
	}
	if !isAdmin {
		query = query.Where(`
			(
				ledger_tx.biz_type IN ('DEPOSIT_DETECTED','DEPOSIT','REVIEW_FAUCET')
				AND EXISTS (
					SELECT 1 FROM deposit_chain_txs d
					WHERE d.deposit_id = ledger_tx.biz_ref_id AND d.user_id = ?
				)
			)
			OR
			(
				ledger_tx.biz_type IN ('WITHDRAW_HOLD','WITHDRAW_BROADCAST','WITHDRAW_COMPLETE','WITHDRAW_REFUND','WITHDRAW_REFUND_REVERSAL')
				AND EXISTS (
					SELECT 1 FROM withdraw_requests w
					WHERE w.withdraw_id = ledger_tx.biz_ref_id AND w.user_id = ?
				)
			)
			OR
				(
					ledger_tx.biz_type = 'TRANSFER'
					AND (
						ledger_tx.operator_id = ?
					OR EXISTS (
						SELECT 1
						FROM ledger_entries le
						WHERE le.ledger_tx_id = ledger_tx.ledger_tx_id
							AND le.user_id = ?
						)
					)
				)
				OR
				(
					ledger_tx.biz_type IN ('funding.settlement','funding.reversal')
					AND EXISTS (
						SELECT 1
						FROM ledger_entries le
						WHERE le.ledger_tx_id = ledger_tx.ledger_tx_id
							AND le.user_id = ?
					)
				)
				OR
				(outbox_events.aggregate_type = 'order' AND orders.user_id = ?)
				OR
				(outbox_events.aggregate_type = 'fill' AND fills.user_id = ?)
				OR
				(outbox_events.aggregate_type = 'position' AND positions.user_id = ?)
			`, userID, userID, fmt.Sprintf("%d", userID), userID, userID, userID, userID, userID)
	}
	if err := query.Order("outbox_events.created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.ExplorerEvent, 0, len(rows))
	for _, row := range rows {
		payload := map[string]any{}
		_ = json.Unmarshal([]byte(row.PayloadJSON), &payload)
		ledgerTxID := ""
		if row.AggregateType == "ledger_tx" {
			ledgerTxID = row.AggregateID
		} else {
			ledgerTxID = payloadString(payload, "ledger_tx_id")
		}
		orderID := payloadString(payload, "order_id")
		if row.AggregateType == "order" {
			orderID = row.AggregateID
		}
		fillID := payloadString(payload, "fill_id")
		if row.AggregateType == "fill" {
			fillID = row.AggregateID
		}
		positionID := payloadString(payload, "position_id")
		if row.AggregateType == "position" {
			positionID = row.AggregateID
		}
		chainTxHash := payloadString(payload, "tx_hash")
		address := payloadString(payload, "router_address", "to_address", "address")
		amount := payloadString(payload, "amount", "fee_amount")
		if chainTxHash == "" || address == "" {
			fallbackTxHash, fallbackAddress := r.lookupExplorerChainRefs(ctx, row.BizType, row.BizRefID)
			if chainTxHash == "" {
				chainTxHash = fallbackTxHash
			}
			if address == "" {
				address = fallbackAddress
			}
		}
		if amount == "" {
			amount = r.lookupExplorerAmount(ctx, row.BizType, row.BizRefID, ledgerTxID)
		}
		asset := row.Asset
		if asset == "" {
			asset = payloadString(payload, "asset")
		}
		items = append(items, readmodel.ExplorerEvent{
			EventID:     row.EventID,
			EventType:   row.EventType,
			Asset:       optionalStringPtr(asset),
			Amount:      optionalStringPtr(amount),
			CreatedAt:   row.CreatedAt.Format(time.RFC3339),
			LedgerTxID:  optionalStringPtr(ledgerTxID),
			ChainTxHash: optionalStringPtr(chainTxHash),
			OrderID:     optionalStringPtr(orderID),
			FillID:      optionalStringPtr(fillID),
			PositionID:  optionalStringPtr(positionID),
			Address:     optionalStringPtr(address),
			Payload:     payload,
		})
	}
	return items, nil
}

func (r *ExplorerQueryRepository) lookupExplorerChainRefs(ctx context.Context, bizType string, bizRefID string) (string, string) {
	tx := DB(ctx, r.db)
	switch bizType {
	case "DEPOSIT_DETECTED", "DEPOSIT":
		var deposit DepositChainTxModel
		if err := tx.Where("deposit_id = ?", bizRefID).Order("id DESC").First(&deposit).Error; err == nil {
			return deposit.TxHash, deposit.ToAddress
		}
	case "WITHDRAW_HOLD", "WITHDRAW_BROADCAST", "WITHDRAW_COMPLETE", "WITHDRAW_REFUND", "WITHDRAW_REFUND_REVERSAL":
		var withdraw WithdrawRequestModel
		if err := tx.Where("withdraw_id = ?", bizRefID).First(&withdraw).Error; err == nil {
			return withdraw.BroadcastTxHash, withdraw.ToAddress
		}
	}
	return "", ""
}

func (r *ExplorerQueryRepository) lookupExplorerAmount(ctx context.Context, bizType string, bizRefID string, ledgerTxID string) string {
	tx := DB(ctx, r.db)
	switch bizType {
	case "DEPOSIT_DETECTED", "DEPOSIT":
		var deposit DepositChainTxModel
		if err := tx.Where("deposit_id = ?", bizRefID).Order("id DESC").First(&deposit).Error; err == nil {
			return deposit.Amount
		}
	case "WITHDRAW_HOLD", "WITHDRAW_BROADCAST", "WITHDRAW_COMPLETE", "WITHDRAW_REFUND", "WITHDRAW_REFUND_REVERSAL":
		var withdraw WithdrawRequestModel
		if err := tx.Where("withdraw_id = ?", bizRefID).First(&withdraw).Error; err == nil {
			return withdraw.Amount
		}
	case "TRANSFER":
		return extractPositiveTransferAmount(ctx, r.db, ledgerTxID)
	case "funding.settlement", "funding.reversal":
		return extractUserLedgerAmount(ctx, r.db, ledgerTxID)
	}
	return ""
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && text != "" {
			return text
		}
	}
	return ""
}

func optionalStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func extractPositiveTransferAmount(ctx context.Context, db *gorm.DB, ledgerTxID string) string {
	var entries []LedgerEntryModel
	if err := DB(ctx, db).Where("ledger_tx_id = ?", ledgerTxID).Find(&entries).Error; err != nil {
		return "0"
	}
	for _, entry := range entries {
		amount := decimalx.MustFromString(entry.Amount)
		if amount.GreaterThan(decimalx.MustFromString("0")) {
			return entry.Amount
		}
	}
	return "0"
}

func extractUserLedgerAmount(ctx context.Context, db *gorm.DB, ledgerTxID string) string {
	var entries []LedgerEntryModel
	if err := DB(ctx, db).Where("ledger_tx_id = ? AND user_id IS NOT NULL", ledgerTxID).Order("id ASC").Find(&entries).Error; err != nil {
		return "0"
	}
	for _, entry := range entries {
		if entry.Amount != "" {
			return entry.Amount
		}
	}
	return "0"
}

type transferEntryDescriptor struct {
	UserID        *uint64
	AccountUserID *uint64
	AccountCode   string
	Amount        string
	EVMAddress    *string
}

func describeTransferForUser(ctx context.Context, db *gorm.DB, ledgerTxID string, userID uint64) (direction string, counterpartyAddress string, fromAccount string, toAccount string) {
	var rows []transferEntryDescriptor
	if err := DB(ctx, db).
		Table("ledger_entries").
		Select(`
			ledger_entries.user_id,
			accounts.user_id AS account_user_id,
			accounts.account_code,
			ledger_entries.amount,
			users.evm_address
		`).
		Joins("JOIN accounts ON accounts.id = ledger_entries.account_id").
		Joins("LEFT JOIN users ON users.id = COALESCE(ledger_entries.user_id, accounts.user_id)").
		Where("ledger_entries.ledger_tx_id = ?", ledgerTxID).
		Order("ledger_entries.id ASC").
		Scan(&rows).Error; err != nil {
		return "UNKNOWN", "", "USER_WALLET", "USER_WALLET"
	}

	var sender transferEntryDescriptor
	var receiver transferEntryDescriptor
	for _, row := range rows {
		amount := decimalx.MustFromString(row.Amount)
		if amount.LessThan(decimalx.MustFromString("0")) {
			sender = row
			fromAccount = row.AccountCode
		}
		if amount.GreaterThan(decimalx.MustFromString("0")) {
			receiver = row
			toAccount = row.AccountCode
		}
	}
	if fromAccount == "" {
		fromAccount = "USER_WALLET"
	}
	if toAccount == "" {
		toAccount = "USER_WALLET"
	}

	senderID := effectiveTransferUserID(sender)
	receiverID := effectiveTransferUserID(receiver)

	switch {
	case senderID == userID && receiverID == userID:
		if sender.EVMAddress != nil {
			counterpartyAddress = *sender.EVMAddress
		}
		if counterpartyAddress == "" && senderID != 0 {
			counterpartyAddress = lookupUserEVMAddress(ctx, db, senderID)
		}
		return "SELF", counterpartyAddress, fromAccount, toAccount
	case senderID == userID:
		if receiver.EVMAddress != nil {
			counterpartyAddress = *receiver.EVMAddress
		}
		if counterpartyAddress == "" && receiverID != 0 {
			counterpartyAddress = lookupUserEVMAddress(ctx, db, receiverID)
		}
		return "OUT", counterpartyAddress, fromAccount, toAccount
	case receiverID == userID:
		if sender.EVMAddress != nil {
			counterpartyAddress = *sender.EVMAddress
		}
		if counterpartyAddress == "" && senderID != 0 {
			counterpartyAddress = lookupUserEVMAddress(ctx, db, senderID)
		}
		return "IN", counterpartyAddress, fromAccount, toAccount
	default:
		return "UNKNOWN", "", fromAccount, toAccount
	}
}

func effectiveTransferUserID(entry transferEntryDescriptor) uint64 {
	if entry.UserID != nil && *entry.UserID != 0 {
		return *entry.UserID
	}
	if entry.AccountUserID != nil {
		return *entry.AccountUserID
	}
	return 0
}

func lookupUserEVMAddress(ctx context.Context, db *gorm.DB, userID uint64) string {
	var user UserModel
	if err := DB(ctx, db).Where("id = ?", userID).First(&user).Error; err != nil {
		return ""
	}
	return user.EVMAddress
}

func cloneConfirmations(values map[int64]int) map[int64]int {
	if len(values) == 0 {
		return map[int64]int{}
	}
	out := make(map[int64]int, len(values))
	for chainID, confirmations := range values {
		out[chainID] = confirmations
	}
	return out
}

func (r *WalletQueryRepository) requiredConfirmations(chainID int64) int {
	if r == nil {
		return 0
	}
	return r.confirmations[chainID]
}
