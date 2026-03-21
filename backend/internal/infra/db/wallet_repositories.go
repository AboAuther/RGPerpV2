package db

import (
	"context"
	"errors"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

type DepositRepository struct{ db *gorm.DB }

func NewDepositRepository(db *gorm.DB) *DepositRepository { return &DepositRepository{db: db} }

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
		Amount:             model.Amount,
		Status:             model.Status,
		Asset:              "USDC",
		CreditedLedgerTxID: model.CreditedLedgerTxID,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}, nil
}

func (r *DepositRepository) MarkCredited(ctx context.Context, depositID string, ledgerTxID string) error {
	return DB(ctx, r.db).
		Model(&DepositChainTxModel{}).
		Where("deposit_id = ?", depositID).
		Updates(map[string]any{
			"status":                walletdomain.StatusCredited,
			"credited_ledger_tx_id": ledgerTxID,
		}).Error
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
		HoldLedgerTxID:  model.HoldLedgerTxID,
		BroadcastTxHash: model.BroadcastTxHash,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}, nil
}

func (r *WithdrawRepository) MarkBroadcasted(ctx context.Context, withdrawID string, txHash string) error {
	return DB(ctx, r.db).Model(&WithdrawRequestModel{}).
		Where("withdraw_id = ?", withdrawID).
		Updates(map[string]any{
			"status":            walletdomain.StatusBroadcasted,
			"broadcast_tx_hash": txHash,
		}).Error
}

func (r *WithdrawRepository) MarkRefunded(ctx context.Context, withdrawID string) error {
	return DB(ctx, r.db).Model(&WithdrawRequestModel{}).
		Where("withdraw_id = ?", withdrawID).
		Update("status", walletdomain.StatusRefunded).Error
}

type AccountResolver struct{ db *gorm.DB }

func NewAccountResolver(db *gorm.DB) *AccountResolver { return &AccountResolver{db: db} }

func (r *AccountResolver) UserWalletAccountID(ctx context.Context, userID uint64, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, &userID, "USER_WALLET", asset)
}
func (r *AccountResolver) UserWithdrawHoldAccountID(ctx context.Context, userID uint64, asset string) (uint64, error) {
	return r.lookupAccountID(ctx, &userID, "USER_WITHDRAW_HOLD", asset)
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
