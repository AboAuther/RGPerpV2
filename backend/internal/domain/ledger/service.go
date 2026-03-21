package ledger

import (
	"context"
	"fmt"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type Service struct {
	repo     Repository
	decimals DecimalFactory
}

func NewService(repo Repository, decimals DecimalFactory) *Service {
	return &Service{repo: repo, decimals: decimals}
}

func (s *Service) Post(ctx context.Context, req PostingRequest) error {
	if req.LedgerTx.ID == "" || req.LedgerTx.EventID == "" || req.LedgerTx.IdempotencyKey == "" {
		return fmt.Errorf("%w: missing ledger transaction identity", errorsx.ErrInvalidArgument)
	}
	if len(req.Entries) < 2 {
		return fmt.Errorf("%w: posting must have at least two entries", errorsx.ErrInvalidArgument)
	}

	var sum Decimal
	for idx, entry := range req.Entries {
		if entry.AccountID == 0 || entry.Asset == "" || entry.Amount == "" {
			return fmt.Errorf("%w: invalid ledger entry at index %d", errorsx.ErrInvalidArgument, idx)
		}
		amount, err := s.decimals.FromString(entry.Amount)
		if err != nil {
			return err
		}
		if sum == nil {
			sum = amount
			continue
		}
		sum = sum.Add(amount)
	}

	if sum == nil || !sum.IsZero() {
		return fmt.Errorf("%w: posting is not balanced", errorsx.ErrInvalidArgument)
	}
	return s.repo.CreatePosting(ctx, req)
}
