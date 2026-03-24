package adminops

import (
	"context"
	"fmt"
	"strings"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	InsuranceTopUpSourceSystemPool = "SYSTEM_POOL"
	InsuranceTopUpSourceCustodyHot = "CUSTODY_HOT"

	abortReasonManualResolvedPositionClosed = "manual_resolved_position_closed"
	abortReasonManualResolvedNoOpenPosition = "manual_resolved_no_open_position"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type AccountResolver interface {
	SystemPoolAccountID(ctx context.Context, asset string) (uint64, error)
	CustodyHotAccountID(ctx context.Context, asset string) (uint64, error)
	InsuranceFundAccountID(ctx context.Context, asset string) (uint64, error)
}

type LedgerPoster interface {
	Post(ctx context.Context, req ledgerdomain.PostingRequest) error
}

type LiquidationRepository interface {
	GetLiquidationByIDForUpdate(ctx context.Context, liquidationID string) (liquidationdomain.Liquidation, error)
	UpdateLiquidation(ctx context.Context, liquidation liquidationdomain.Liquidation) error
	GetPositionForLiquidationByID(ctx context.Context, userID uint64, positionID string) (liquidationdomain.Position, error)
	ListOpenPositionsForUpdate(ctx context.Context, userID uint64) ([]liquidationdomain.Position, error)
}

type LiquidationExecutor interface {
	Execute(ctx context.Context, input liquidationdomain.ExecuteInput) (liquidationdomain.Liquidation, error)
}

type Service struct {
	clock       Clock
	idgen       IDGenerator
	txm         TxManager
	accounts    AccountResolver
	ledger      LedgerPoster
	liquidation LiquidationRepository
	executor    LiquidationExecutor
}

type InsuranceFundTopUpInput struct {
	OperatorID     string
	TraceID        string
	IdempotencyKey string
	Reason         string
	Asset          string
	Amount         string
	SourceAccount  string
}

type InsuranceFundTopUpResult struct {
	TopUpID       string `json:"topup_id"`
	Asset         string `json:"asset"`
	Amount        string `json:"amount"`
	SourceAccount string `json:"source_account"`
	Status        string `json:"status"`
}

type LiquidationActionInput struct {
	LiquidationID string
	OperatorID    string
	TraceID       string
	Reason        string
}

type LiquidationActionResult struct {
	LiquidationID string  `json:"liquidation_id"`
	Status        string  `json:"status"`
	AbortReason   *string `json:"abort_reason,omitempty"`
}

func NewService(
	clock Clock,
	idgen IDGenerator,
	txm TxManager,
	accounts AccountResolver,
	ledger LedgerPoster,
	liquidation LiquidationRepository,
	executor LiquidationExecutor,
) (*Service, error) {
	if clock == nil || idgen == nil || txm == nil || accounts == nil || ledger == nil || liquidation == nil || executor == nil {
		return nil, fmt.Errorf("%w: missing admin ops dependency", errorsx.ErrInvalidArgument)
	}
	return &Service{
		clock:       clock,
		idgen:       idgen,
		txm:         txm,
		accounts:    accounts,
		ledger:      ledger,
		liquidation: liquidation,
		executor:    executor,
	}, nil
}

func (s *Service) TopUpInsuranceFund(ctx context.Context, input InsuranceFundTopUpInput) (InsuranceFundTopUpResult, error) {
	asset := strings.ToUpper(strings.TrimSpace(input.Asset))
	if asset == "" {
		asset = "USDC"
	}
	amount, err := decimalx.NewFromString(strings.TrimSpace(input.Amount))
	if err != nil {
		return InsuranceFundTopUpResult{}, err
	}
	if !amount.GreaterThan(decimalx.MustFromString("0")) {
		return InsuranceFundTopUpResult{}, fmt.Errorf("%w: top-up amount must be positive", errorsx.ErrInvalidArgument)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return InsuranceFundTopUpResult{}, fmt.Errorf("%w: top-up reason is required", errorsx.ErrInvalidArgument)
	}
	source := strings.ToUpper(strings.TrimSpace(input.SourceAccount))
	if source == "" {
		source = InsuranceTopUpSourceSystemPool
	}
	if source != InsuranceTopUpSourceSystemPool && source != InsuranceTopUpSourceCustodyHot {
		return InsuranceFundTopUpResult{}, fmt.Errorf("%w: unsupported insurance fund source", errorsx.ErrInvalidArgument)
	}
	topUpID := s.idgen.NewID("iftop")
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("insurance_topup:%s", topUpID)
	}
	now := s.clock.Now().UTC()
	result := InsuranceFundTopUpResult{
		TopUpID:       topUpID,
		Asset:         asset,
		Amount:        amount.String(),
		SourceAccount: source,
		Status:        "COMMITTED",
	}
	err = s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		insuranceFundAccountID, err := s.accounts.InsuranceFundAccountID(txCtx, asset)
		if err != nil {
			return err
		}
		sourceAccountID, err := s.resolveTopUpSourceAccount(txCtx, asset, source)
		if err != nil {
			return err
		}
		return s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "insurance_fund.topup",
				BizRefID:       topUpID,
				Asset:          asset,
				IdempotencyKey: idempotencyKey,
				OperatorType:   "ADMIN",
				OperatorID:     strings.ToLower(strings.TrimSpace(input.OperatorID)),
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      now,
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: sourceAccountID, Asset: asset, Amount: amount.Neg().String(), EntryType: "INSURANCE_FUND_TOPUP_SOURCE"},
				{AccountID: insuranceFundAccountID, Asset: asset, Amount: amount.String(), EntryType: "INSURANCE_FUND_TOPUP"},
			},
		})
	})
	if err != nil {
		return InsuranceFundTopUpResult{}, err
	}
	return result, nil
}

func (s *Service) RetryPendingLiquidation(ctx context.Context, input LiquidationActionInput) (LiquidationActionResult, error) {
	liquidation, err := s.loadPendingManualLiquidation(ctx, input.LiquidationID)
	if err != nil {
		return LiquidationActionResult{}, err
	}
	positionID := ""
	if liquidation.Mode == liquidationdomain.ModeIsolated && len(liquidation.PrePositionsSnapshot) > 0 {
		positionID = strings.TrimSpace(liquidation.PrePositionsSnapshot[0].PositionID)
	}
	updated, err := s.executor.Execute(ctx, liquidationdomain.ExecuteInput{
		LiquidationID:         liquidation.ID,
		UserID:                liquidation.UserID,
		Mode:                  liquidation.Mode,
		PositionID:            positionID,
		TriggerRiskSnapshotID: liquidation.TriggerRiskSnapshotID,
		TraceID:               input.TraceID,
	})
	if err != nil {
		return LiquidationActionResult{}, err
	}
	return LiquidationActionResult{
		LiquidationID: updated.ID,
		Status:        updated.Status,
		AbortReason:   updated.AbortReason,
	}, nil
}

func (s *Service) ClosePendingLiquidation(ctx context.Context, input LiquidationActionInput) (LiquidationActionResult, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return LiquidationActionResult{}, fmt.Errorf("%w: close reason is required", errorsx.ErrInvalidArgument)
	}
	now := s.clock.Now().UTC()
	var updated liquidationdomain.Liquidation
	err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		liquidation, err := s.liquidation.GetLiquidationByIDForUpdate(txCtx, strings.TrimSpace(input.LiquidationID))
		if err != nil {
			return err
		}
		if liquidation.Status == liquidationdomain.StatusExecuted || liquidation.Status == liquidationdomain.StatusAborted {
			updated = liquidation
			return nil
		}
		if liquidation.Status != liquidationdomain.StatusPendingManual {
			return fmt.Errorf("%w: liquidation is not pending manual", errorsx.ErrConflict)
		}
		if err := s.ensureLiquidationCanBeClosed(txCtx, liquidation); err != nil {
			return err
		}
		abortReason := buildManualAbortReason(liquidation, reason)
		liquidation.Status = liquidationdomain.StatusAborted
		liquidation.AbortReason = &abortReason
		liquidation.UpdatedAt = now
		if err := s.liquidation.UpdateLiquidation(txCtx, liquidation); err != nil {
			return err
		}
		updated = liquidation
		return nil
	})
	if err != nil {
		return LiquidationActionResult{}, err
	}
	return LiquidationActionResult{
		LiquidationID: updated.ID,
		Status:        updated.Status,
		AbortReason:   updated.AbortReason,
	}, nil
}

func (s *Service) loadPendingManualLiquidation(ctx context.Context, liquidationID string) (liquidationdomain.Liquidation, error) {
	var liquidation liquidationdomain.Liquidation
	err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.liquidation.GetLiquidationByIDForUpdate(txCtx, strings.TrimSpace(liquidationID))
		if err != nil {
			return err
		}
		if item.Status != liquidationdomain.StatusPendingManual {
			return fmt.Errorf("%w: liquidation is not pending manual", errorsx.ErrConflict)
		}
		liquidation = item
		return nil
	})
	return liquidation, err
}

func (s *Service) ensureLiquidationCanBeClosed(ctx context.Context, liquidation liquidationdomain.Liquidation) error {
	if liquidation.Mode == liquidationdomain.ModeIsolated {
		if len(liquidation.PrePositionsSnapshot) == 0 {
			return nil
		}
		positionID := strings.TrimSpace(liquidation.PrePositionsSnapshot[0].PositionID)
		if positionID == "" {
			return nil
		}
		_, err := s.liquidation.GetPositionForLiquidationByID(ctx, liquidation.UserID, positionID)
		if err == nil {
			return fmt.Errorf("%w: liquidation position is still open; top up insurance and retry instead", errorsx.ErrConflict)
		}
		if err == errorsx.ErrNotFound {
			return nil
		}
		return err
	}
	positions, err := s.liquidation.ListOpenPositionsForUpdate(ctx, liquidation.UserID)
	if err != nil {
		return err
	}
	if len(positions) > 0 {
		return fmt.Errorf("%w: account still has open positions; retry liquidation instead", errorsx.ErrConflict)
	}
	return nil
}

func (s *Service) resolveTopUpSourceAccount(ctx context.Context, asset string, source string) (uint64, error) {
	switch source {
	case InsuranceTopUpSourceSystemPool:
		return s.accounts.SystemPoolAccountID(ctx, asset)
	case InsuranceTopUpSourceCustodyHot:
		return s.accounts.CustodyHotAccountID(ctx, asset)
	default:
		return 0, fmt.Errorf("%w: unsupported insurance fund source", errorsx.ErrInvalidArgument)
	}
}

func buildManualAbortReason(liquidation liquidationdomain.Liquidation, reason string) string {
	prefix := abortReasonManualResolvedNoOpenPosition
	if liquidation.Mode == liquidationdomain.ModeIsolated {
		prefix = abortReasonManualResolvedPositionClosed
	}
	suffix := sanitizeReasonSuffix(reason)
	if suffix == "" {
		return prefix
	}
	return prefix + ":" + suffix
}

func sanitizeReasonSuffix(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return ""
	}
	var builder strings.Builder
	for _, ch := range reason {
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
		case ch == '_' || ch == '-' || ch == ' ':
			builder.WriteByte('_')
		}
		if builder.Len() >= 96 {
			break
		}
	}
	return strings.Trim(builder.String(), "_")
}
