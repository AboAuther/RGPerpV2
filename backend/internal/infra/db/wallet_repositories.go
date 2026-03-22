package db

import (
	"context"
	"errors"
	"time"

	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

type DepositRepository struct{ db *gorm.DB }

func NewDepositRepository(db *gorm.DB) *DepositRepository { return &DepositRepository{db: db} }

func (r *DepositRepository) Create(ctx context.Context, deposit walletdomain.DepositChainTx) error {
	return DB(ctx, r.db).Create(&DepositChainTxModel{
		DepositID:          deposit.DepositID,
		UserID:             deposit.UserID,
		ChainID:            deposit.ChainID,
		TxHash:             deposit.TxHash,
		LogIndex:           deposit.LogIndex,
		FromAddress:        deposit.FromAddress,
		ToAddress:          deposit.ToAddress,
		TokenAddress:       deposit.TokenAddress,
		Amount:             deposit.Amount,
		BlockNumber:        deposit.BlockNumber,
		Confirmations:      deposit.Confirmations,
		Status:             deposit.Status,
		CreditedLedgerTxID: deposit.CreditedLedgerTxID,
		CreatedAt:          deposit.CreatedAt,
		UpdatedAt:          deposit.UpdatedAt,
	}).Error
}

func (r *DepositRepository) GetByID(ctx context.Context, depositID string) (walletdomain.DepositChainTx, error) {
	var model DepositChainTxModel
	err := DB(ctx, r.db).Where("deposit_id = ?", depositID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return walletdomain.DepositChainTx{}, errorsx.ErrNotFound
		}
		return walletdomain.DepositChainTx{}, err
	}
	return walletdomain.DepositChainTx{
		DepositID:          model.DepositID,
		UserID:             model.UserID,
		ChainID:            model.ChainID,
		TxHash:             model.TxHash,
		LogIndex:           model.LogIndex,
		FromAddress:        model.FromAddress,
		ToAddress:          model.ToAddress,
		TokenAddress:       model.TokenAddress,
		Amount:             model.Amount,
		BlockNumber:        model.BlockNumber,
		Confirmations:      model.Confirmations,
		RequiredConfs:      requiredConfirmations(model.ChainID),
		Status:             model.Status,
		Asset:              "USDC",
		CreditedLedgerTxID: model.CreditedLedgerTxID,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}, nil
}

func (r *DepositRepository) GetByTxLog(ctx context.Context, chainID int64, txHash string, logIndex int64) (walletdomain.DepositChainTx, error) {
	var model DepositChainTxModel
	err := DB(ctx, r.db).Where("chain_id = ? AND tx_hash = ? AND log_index = ?", chainID, txHash, logIndex).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return walletdomain.DepositChainTx{}, errorsx.ErrNotFound
		}
		return walletdomain.DepositChainTx{}, err
	}
	return r.GetByID(ctx, model.DepositID)
}

func (r *DepositRepository) ListPendingByChain(ctx context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.DepositChainTx, error) {
	query := DB(ctx, r.db).Where("chain_id = ?", chainID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	var models []DepositChainTxModel
	if err := query.Order("block_number ASC, log_index ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]walletdomain.DepositChainTx, 0, len(models))
	for _, model := range models {
		out = append(out, walletdomain.DepositChainTx{
			DepositID:          model.DepositID,
			UserID:             model.UserID,
			ChainID:            model.ChainID,
			TxHash:             model.TxHash,
			LogIndex:           model.LogIndex,
			FromAddress:        model.FromAddress,
			ToAddress:          model.ToAddress,
			TokenAddress:       model.TokenAddress,
			Amount:             model.Amount,
			Asset:              "USDC",
			BlockNumber:        model.BlockNumber,
			Confirmations:      model.Confirmations,
			RequiredConfs:      requiredConfirmations(model.ChainID),
			Status:             model.Status,
			CreditedLedgerTxID: model.CreditedLedgerTxID,
			CreatedAt:          model.CreatedAt,
			UpdatedAt:          model.UpdatedAt,
		})
	}
	return out, nil
}

func (r *DepositRepository) UpdateConfirmations(ctx context.Context, depositID string, confirmations int, status string) error {
	result := DB(ctx, r.db).Model(&DepositChainTxModel{}).
		Where("deposit_id = ?", depositID).
		Updates(map[string]any{
			"confirmations": confirmations,
			"status":        status,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrNotFound
	}
	return nil
}

func (r *DepositRepository) MarkCredited(ctx context.Context, depositID string, ledgerTxID string) error {
	result := DB(ctx, r.db).
		Model(&DepositChainTxModel{}).
		Where("deposit_id = ? AND status = ?", depositID, walletdomain.StatusCreditReady).
		Updates(map[string]any{
			"status":                walletdomain.StatusCredited,
			"credited_ledger_tx_id": ledgerTxID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrConflict
	}
	return nil
}

func (r *DepositRepository) MarkReorgReversed(ctx context.Context, depositID string) error {
	result := DB(ctx, r.db).
		Model(&DepositChainTxModel{}).
		Where("deposit_id = ? AND status IN ?", depositID, []string{
			walletdomain.StatusDetected,
			walletdomain.StatusConfirming,
			walletdomain.StatusCreditReady,
		}).
		Update("status", walletdomain.StatusReorgReversed)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrConflict
	}
	return nil
}

func (r *DepositRepository) ListByUser(ctx context.Context, userID uint64) ([]walletdomain.DepositChainTx, error) {
	var models []DepositChainTxModel
	if err := DB(ctx, r.db).Where("user_id = ?", userID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]walletdomain.DepositChainTx, 0, len(models))
	for _, model := range models {
		out = append(out, walletdomain.DepositChainTx{
			DepositID:          model.DepositID,
			UserID:             model.UserID,
			ChainID:            model.ChainID,
			TxHash:             model.TxHash,
			LogIndex:           model.LogIndex,
			FromAddress:        model.FromAddress,
			ToAddress:          model.ToAddress,
			TokenAddress:       model.TokenAddress,
			Amount:             model.Amount,
			Asset:              "USDC",
			BlockNumber:        model.BlockNumber,
			Confirmations:      model.Confirmations,
			RequiredConfs:      requiredConfirmations(model.ChainID),
			Status:             model.Status,
			CreditedLedgerTxID: model.CreditedLedgerTxID,
			CreatedAt:          model.CreatedAt,
			UpdatedAt:          model.UpdatedAt,
		})
	}
	return out, nil
}

type WithdrawRepository struct{ db *gorm.DB }

func NewWithdrawRepository(db *gorm.DB) *WithdrawRepository { return &WithdrawRepository{db: db} }

func (r *WithdrawRepository) Create(ctx context.Context, withdraw walletdomain.WithdrawRequest) error {
	return DB(ctx, r.db).Create(&WithdrawRequestModel{
		WithdrawID:     withdraw.WithdrawID,
		UserID:         withdraw.UserID,
		ChainID:        withdraw.ChainID,
		Asset:          withdraw.Asset,
		Amount:         withdraw.Amount,
		FeeAmount:      withdraw.FeeAmount,
		ToAddress:      withdraw.ToAddress,
		Status:         withdraw.Status,
		RiskFlag:       nullableString(withdraw.RiskFlag),
		HoldLedgerTxID: withdraw.HoldLedgerTxID,
		CreatedAt:      withdraw.CreatedAt,
		UpdatedAt:      withdraw.UpdatedAt,
	}).Error
}

func (r *WithdrawRepository) GetByID(ctx context.Context, withdrawID string) (walletdomain.WithdrawRequest, error) {
	var model WithdrawRequestModel
	err := DB(ctx, r.db).Where("withdraw_id = ?", withdrawID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return walletdomain.WithdrawRequest{}, errorsx.ErrNotFound
		}
		return walletdomain.WithdrawRequest{}, err
	}
	return walletdomain.WithdrawRequest{
		WithdrawID:      model.WithdrawID,
		UserID:          model.UserID,
		ChainID:         model.ChainID,
		Asset:           model.Asset,
		Amount:          model.Amount,
		FeeAmount:       model.FeeAmount,
		ToAddress:       model.ToAddress,
		Status:          model.Status,
		RiskFlag:        derefString(model.RiskFlag),
		HoldLedgerTxID:  model.HoldLedgerTxID,
		BroadcastTxHash: model.BroadcastTxHash,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}, nil
}

func (r *WithdrawRepository) ListByChainStatuses(ctx context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.WithdrawRequest, error) {
	query := DB(ctx, r.db).Where("chain_id = ?", chainID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var models []WithdrawRequestModel
	if err := query.Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]walletdomain.WithdrawRequest, 0, len(models))
	for _, model := range models {
		out = append(out, walletdomain.WithdrawRequest{
			WithdrawID:      model.WithdrawID,
			UserID:          model.UserID,
			ChainID:         model.ChainID,
			Asset:           model.Asset,
			Amount:          model.Amount,
			FeeAmount:       model.FeeAmount,
			ToAddress:       model.ToAddress,
			Status:          model.Status,
			RiskFlag:        derefString(model.RiskFlag),
			HoldLedgerTxID:  model.HoldLedgerTxID,
			BroadcastTxHash: model.BroadcastTxHash,
			CreatedAt:       model.CreatedAt,
			UpdatedAt:       model.UpdatedAt,
		})
	}
	return out, nil
}

func (r *WithdrawRepository) UpdateStatus(ctx context.Context, withdrawID string, from []string, to string) error {
	query := DB(ctx, r.db).Model(&WithdrawRequestModel{}).Where("withdraw_id = ?", withdrawID)
	if len(from) > 0 {
		query = query.Where("status IN ?", from)
	}
	result := query.Update("status", to)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrConflict
	}
	return nil
}

func (r *WithdrawRepository) MarkBroadcasted(ctx context.Context, withdrawID string, txHash string) error {
	result := DB(ctx, r.db).Model(&WithdrawRequestModel{}).
		Where("withdraw_id = ?", withdrawID).
		Updates(map[string]any{
			"broadcast_tx_hash": txHash,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrConflict
	}
	return nil
}

func (r *WithdrawRepository) MarkCompleted(ctx context.Context, withdrawID string) error {
	now := time.Now().UTC()
	result := DB(ctx, r.db).Model(&WithdrawRequestModel{}).
		Where("withdraw_id = ?", withdrawID).
		Update("completed_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrConflict
	}
	return nil
}

func (r *WithdrawRepository) MarkRefunded(ctx context.Context, withdrawID string) error {
	result := DB(ctx, r.db).Model(&WithdrawRequestModel{}).
		Where("withdraw_id = ?", withdrawID).
		Update("updated_at", time.Now().UTC())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrConflict
	}
	return nil
}

func (r *WithdrawRepository) ListByUser(ctx context.Context, userID uint64) ([]walletdomain.WithdrawRequest, error) {
	var models []WithdrawRequestModel
	if err := DB(ctx, r.db).Where("user_id = ?", userID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]walletdomain.WithdrawRequest, 0, len(models))
	for _, model := range models {
		out = append(out, walletdomain.WithdrawRequest{
			WithdrawID:      model.WithdrawID,
			UserID:          model.UserID,
			ChainID:         model.ChainID,
			Asset:           model.Asset,
			Amount:          model.Amount,
			FeeAmount:       model.FeeAmount,
			ToAddress:       model.ToAddress,
			Status:          model.Status,
			RiskFlag:        derefString(model.RiskFlag),
			HoldLedgerTxID:  model.HoldLedgerTxID,
			BroadcastTxHash: model.BroadcastTxHash,
			CreatedAt:       model.CreatedAt,
			UpdatedAt:       model.UpdatedAt,
		})
	}
	return out, nil
}

type AccountResolver struct{ db *gorm.DB }

func NewAccountResolver(db *gorm.DB) *AccountResolver { return &AccountResolver{db: db} }

func (r *AccountResolver) ResolveTradeAccounts(ctx context.Context, userID uint64, asset string) (orderdomain.TradeAccounts, error) {
	requiredUserCodes := map[string]func(*orderdomain.TradeAccounts, uint64){
		"USER_WALLET":          func(a *orderdomain.TradeAccounts, id uint64) { a.UserWalletAccountID = id },
		"USER_ORDER_MARGIN":    func(a *orderdomain.TradeAccounts, id uint64) { a.UserOrderMarginAccountID = id },
		"USER_POSITION_MARGIN": func(a *orderdomain.TradeAccounts, id uint64) { a.UserPositionMarginAccountID = id },
	}
	requiredSystemCodes := map[string]func(*orderdomain.TradeAccounts, uint64){
		"SYSTEM_POOL":         func(a *orderdomain.TradeAccounts, id uint64) { a.SystemPoolAccountID = id },
		"TRADING_FEE_ACCOUNT": func(a *orderdomain.TradeAccounts, id uint64) { a.TradingFeeAccountID = id },
	}

	var models []AccountModel
	if err := DB(ctx, r.db).
		Where("asset = ? AND ((user_id = ? AND account_code IN ?) OR (user_id IS NULL AND account_code IN ?))",
			asset,
			userID,
			keys(requiredUserCodes),
			keys(requiredSystemCodes),
		).
		Find(&models).Error; err != nil {
		return orderdomain.TradeAccounts{}, err
	}

	var accounts orderdomain.TradeAccounts
	for _, model := range models {
		if model.UserID != nil && *model.UserID == userID {
			if assign, ok := requiredUserCodes[model.AccountCode]; ok {
				assign(&accounts, model.ID)
			}
			continue
		}
		if model.UserID == nil {
			if assign, ok := requiredSystemCodes[model.AccountCode]; ok {
				assign(&accounts, model.ID)
			}
		}
	}

	if accounts.UserWalletAccountID == 0 || accounts.UserOrderMarginAccountID == 0 || accounts.UserPositionMarginAccountID == 0 || accounts.SystemPoolAccountID == 0 || accounts.TradingFeeAccountID == 0 {
		return orderdomain.TradeAccounts{}, errorsx.ErrNotFound
	}
	return accounts, nil
}

func (r *AccountResolver) UserWalletAccountID(ctx context.Context, userID uint64, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, &userID, "USER_WALLET", asset)
}
func (r *AccountResolver) UserOrderMarginAccountID(ctx context.Context, userID uint64, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, &userID, "USER_ORDER_MARGIN", asset)
}
func (r *AccountResolver) UserPositionMarginAccountID(ctx context.Context, userID uint64, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, &userID, "USER_POSITION_MARGIN", asset)
}
func (r *AccountResolver) UserWithdrawHoldAccountID(ctx context.Context, userID uint64, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, &userID, "USER_WITHDRAW_HOLD", asset)
}
func (r *AccountResolver) SystemPoolAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "SYSTEM_POOL", asset)
}
func (r *AccountResolver) TradingFeeAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "TRADING_FEE_ACCOUNT", asset)
}
func (r *AccountResolver) DepositPendingAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "DEPOSIT_PENDING_CONFIRM", asset)
}
func (r *AccountResolver) WithdrawInTransitAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "WITHDRAW_IN_TRANSIT", asset)
}
func (r *AccountResolver) WithdrawFeeAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "WITHDRAW_FEE_ACCOUNT", asset)
}
func (r *AccountResolver) CustodyHotAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "CUSTODY_HOT", asset)
}
func (r *AccountResolver) TestFaucetPoolAccountID(ctx context.Context, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, nil, "TEST_FAUCET_POOL", asset)
}

func (r *AccountResolver) lookupAccountID(ctx context.Context, userID *uint64, accountCode string, asset string) (uint64, error) {
	var model AccountModel
	query := DB(ctx, r.db).Where("account_code = ? AND asset = ?", accountCode, asset)
	if userID == nil {
		query = query.Where("user_id IS NULL")
	} else {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errorsx.ErrNotFound
		}
		return 0, err
	}
	return model.ID, nil
}

func keys[T any](items map[string]T) []string {
	out := make([]string, 0, len(items))
	for key := range items {
		out = append(out, key)
	}
	return out
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	out := value
	return &out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (r *DepositAddressRepository) GetByChainAddress(ctx context.Context, chainID int64, address string) (walletdomain.DepositAddress, error) {
	var model DepositAddressModel
	err := DB(ctx, r.db).
		Where("chain_id = ? AND lower(address) = lower(?)", chainID, address).
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

func (r *DepositAddressRepository) AssignToUser(ctx context.Context, userID uint64, chainID int64, asset string, address string) error {
	result := DB(ctx, r.db).
		Model(&DepositAddressModel{}).
		Where("user_id = ? AND chain_id = ? AND asset = ?", userID, chainID, asset).
		Updates(map[string]any{
			"address": address,
			"status":  "ACTIVE",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrNotFound
	}
	return nil
}
