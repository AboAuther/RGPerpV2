package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
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
	tx := DB(ctx, r.db)
	var account AccountModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", accountID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errorsx.ErrNotFound
		}
		return "", err
	}
	var snapshot AccountBalanceSnapshotModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id = ? AND asset = ?", accountID, asset).First(&snapshot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "0", nil
		}
		return "", err
	}
	return snapshot.Balance, nil
}

type DepositAddressRepository struct {
	db            *gorm.DB
	confirmations map[int64]int
}

func NewDepositAddressRepository(db *gorm.DB, confirmations map[int64]int) *DepositAddressRepository {
	setRequiredConfirmations(confirmations)
	return &DepositAddressRepository{db: db, confirmations: confirmations}
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
	db *gorm.DB
}

func NewAccountQueryRepository(db *gorm.DB) *AccountQueryRepository {
	return &AccountQueryRepository{db: db}
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
	balances, err := r.ListBalances(ctx, userID)
	if err != nil {
		return readmodel.AccountSummary{}, err
	}
	var positions []PositionModel
	if err := DB(ctx, r.db).Where("user_id = ? AND status = ?", userID, "OPEN").Find(&positions).Error; err != nil {
		return readmodel.AccountSummary{}, err
	}

	equity := decimalx.MustFromString("0")
	available := decimalx.MustFromString("0")
	initialMargin := decimalx.MustFromString("0")
	maintenanceMargin := decimalx.MustFromString("0")
	unrealizedPnL := decimalx.MustFromString("0")
	orderHold := decimalx.MustFromString("0")
	positionMarginBalance := decimalx.MustFromString("0")

	for _, item := range balances {
		value := decimalx.MustFromString(item.Balance)
		equity = equity.Add(value)
		switch item.AccountCode {
		case "USER_WALLET":
			available = value
		case "USER_ORDER_MARGIN":
			orderHold = orderHold.Add(value)
		case "USER_POSITION_MARGIN":
			positionMarginBalance = positionMarginBalance.Add(value)
		}
	}

	for _, position := range positions {
		initialMargin = initialMargin.Add(decimalx.MustFromString(position.InitialMargin))
		maintenanceMargin = maintenanceMargin.Add(decimalx.MustFromString(position.MaintenanceMargin))
		unrealizedPnL = unrealizedPnL.Add(decimalx.MustFromString(position.UnrealizedPnL))
	}

	equity = equity.Add(unrealizedPnL)
	initialMargin = initialMargin.Add(orderHold)

	marginRatio := "0"
	if maintenanceMargin.GreaterThan(decimalx.MustFromString("0")) {
		marginRatio = equity.Div(maintenanceMargin).String()
	}
	if !positionMarginBalance.IsZero() && initialMargin.IsZero() {
		initialMargin = positionMarginBalance
	}
	if maintenanceMargin.IsZero() && !positionMarginBalance.IsZero() {
		maintenanceMargin = positionMarginBalance
		if equity.GreaterThan(decimalx.MustFromString("0")) {
			marginRatio = equity.Div(maintenanceMargin).String()
		}
	}
	return readmodel.AccountSummary{
		Equity:                 equity.String(),
		AvailableBalance:       available.String(),
		TotalInitialMargin:     initialMargin.String(),
		TotalMaintenanceMargin: maintenanceMargin.String(),
		UnrealizedPnL:          unrealizedPnL.String(),
		MarginRatio:            marginRatio,
	}, nil
}

func (r *AccountQueryRepository) GetRisk(ctx context.Context, userID uint64) (readmodel.RiskSnapshot, error) {
	summary, err := r.GetSummary(ctx, userID)
	if err != nil {
		return readmodel.RiskSnapshot{}, err
	}
	risk := readmodel.RiskSnapshot{
		AccountStatus:  "ACTIVE",
		RiskState:      "SAFE",
		MarkPriceStale: false,
		CanOpenRisk:    true,
		Notes:          []string{"当前风险视图基于账户子账户余额与仓位快照，订单与仓位资金均遵循统一账本模型。"},
	}
	if summary.Equity == "0" {
		risk.RiskState = "WATCH"
		risk.Notes = append(risk.Notes, "账户权益为 0，新增风险应保持保守策略。")
	}
	return risk, nil
}

func (r *AccountQueryRepository) ListFunding(ctx context.Context, userID uint64) ([]readmodel.FundingItem, error) {
	return NewTradingReadRepository(r.db).ListFunding(ctx, userID)
}

func (r *AccountQueryRepository) ListTransfers(ctx context.Context, userID uint64) ([]readmodel.TransferItem, error) {
	var txs []LedgerTxModel
	if err := DB(ctx, r.db).
		Where("biz_type = ? AND operator_id = ?", "TRANSFER", fmt.Sprintf("%d", userID)).
		Order("created_at DESC").
		Find(&txs).Error; err != nil {
		return nil, err
	}

	out := make([]readmodel.TransferItem, 0, len(txs))
	for _, tx := range txs {
		out = append(out, readmodel.TransferItem{
			TransferID:  tx.BizRefID,
			Asset:       tx.Asset,
			Amount:      extractPositiveTransferAmount(ctx, r.db, tx.LedgerTxID),
			FromAccount: "USER_WALLET",
			ToAccount:   "USER_WALLET",
			Status:      tx.Status,
			CreatedAt:   tx.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

type WalletQueryRepository struct {
	db *gorm.DB
}

func NewWalletQueryRepository(db *gorm.DB) *WalletQueryRepository {
	return &WalletQueryRepository{db: db}
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
			RequiredConfirmations: requiredConfirmations(model.ChainID),
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
		Order("CASE withdraw_requests.status WHEN 'RISK_REVIEW' THEN 0 ELSE 1 END ASC, withdraw_requests.created_at DESC").
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

func (r *ExplorerQueryRepository) ListEvents(ctx context.Context, userID uint64, isAdmin bool, limit int) ([]readmodel.ExplorerEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct {
		OutboxEventModel
		Asset    string
		BizType  string
		BizRefID string
	}
	query := DB(ctx, r.db).Table("outbox_events").
		Select("outbox_events.*, ledger_tx.asset, ledger_tx.biz_type, ledger_tx.biz_ref_id").
		Joins("JOIN ledger_tx ON ledger_tx.ledger_tx_id = outbox_events.aggregate_id")
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
				ledger_tx.biz_type = 'TRANSFER' AND ledger_tx.operator_id = ?
			)
		`, userID, userID, fmt.Sprintf("%d", userID))
	}
	if err := query.Order("outbox_events.created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]readmodel.ExplorerEvent, 0, len(rows))
	for _, row := range rows {
		payload := map[string]any{}
		_ = json.Unmarshal([]byte(row.PayloadJSON), &payload)
		ledgerTxID := row.AggregateID
		chainTxHash := payloadString(payload, "tx_hash")
		address := payloadString(payload, "router_address", "to_address", "address")
		amount := payloadString(payload, "amount")
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
		items = append(items, readmodel.ExplorerEvent{
			EventID:     row.EventID,
			EventType:   row.EventType,
			Asset:       optionalStringPtr(asset),
			Amount:      optionalStringPtr(amount),
			CreatedAt:   row.CreatedAt.Format(time.RFC3339),
			LedgerTxID:  &ledgerTxID,
			ChainTxHash: optionalStringPtr(chainTxHash),
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

func requiredConfirmations(chainID int64) int {
	requiredConfirmationMu.RLock()
	if value, ok := requiredConfirmationByChain[chainID]; ok && value > 0 {
		requiredConfirmationMu.RUnlock()
		return value
	}
	requiredConfirmationMu.RUnlock()
	switch chainID {
	case 1:
		return 12
	case 42161, 8453:
		return 20
	case 31337:
		return 1
	default:
		return 0
	}
}

var (
	requiredConfirmationMu      sync.RWMutex
	requiredConfirmationByChain = map[int64]int{}
)

func setRequiredConfirmations(values map[int64]int) {
	requiredConfirmationMu.Lock()
	defer requiredConfirmationMu.Unlock()
	requiredConfirmationByChain = make(map[int64]int, len(values))
	for chainID, confirmations := range values {
		requiredConfirmationByChain[chainID] = confirmations
	}
}
