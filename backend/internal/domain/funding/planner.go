package funding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type Planner struct {
	service *Service
	runtime RuntimeConfigProvider
	clock   Clock
	idgen   IDGenerator
	txm     TxManager
	repo    Repository
	rates   RateProvider
}

func NewPlanner(service *Service, runtime RuntimeConfigProvider, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, rates RateProvider) (*Planner, error) {
	if service == nil || clock == nil || idgen == nil || txm == nil || repo == nil || rates == nil {
		return nil, fmt.Errorf("%w: missing funding planner dependency", errorsx.ErrInvalidArgument)
	}
	return &Planner{
		service: service,
		runtime: runtime,
		clock:   clock,
		idgen:   idgen,
		txm:     txm,
		repo:    repo,
		rates:   rates,
	}, nil
}

func (p *Planner) PlanDueBatches(ctx context.Context) ([]BatchPlan, error) {
	symbols, err := p.repo.ListSymbolsForFunding(ctx)
	if err != nil {
		return nil, err
	}
	if len(symbols) == 0 {
		return []BatchPlan{}, nil
	}

	now := p.clock.Now().UTC()
	out := make([]BatchPlan, 0, len(symbols))
	for _, symbol := range symbols {
		plan, err := p.PlanBatchForSymbol(ctx, symbol, now)
		if err != nil {
			return nil, err
		}
		if plan == nil {
			continue
		}
		out = append(out, *plan)
	}
	return out, nil
}

func (p *Planner) PlanBatchForSymbol(ctx context.Context, symbol Symbol, now time.Time) (*BatchPlan, error) {
	if symbol.ID == 0 || symbol.Symbol == "" {
		return nil, fmt.Errorf("%w: invalid funding symbol", errorsx.ErrInvalidArgument)
	}

	service, err := p.serviceForSymbol(symbol.Symbol)
	if err != nil {
		return nil, err
	}
	windowEnd := alignWindowEnd(now.UTC(), service.cfg.SettlementIntervalSec)
	windowStart := windowEnd.Add(-time.Duration(service.cfg.SettlementIntervalSec) * time.Second)
	if !windowEnd.After(windowStart) {
		return nil, fmt.Errorf("%w: invalid funding window", errorsx.ErrInvalidArgument)
	}

	var built BatchPlan
	created := false
	err = p.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := p.repo.GetBatchByWindow(txCtx, symbol.ID, windowStart, windowEnd)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errorsx.ErrNotFound) {
			return err
		}

		settlement, err := p.repo.GetSettlementPriceAtOrBefore(txCtx, symbol.ID, windowEnd)
		if err != nil {
			return err
		}
		positions, err := p.repo.ListOpenPositionsForUpdate(txCtx, symbol.ID)
		if err != nil {
			return err
		}
		sources, err := p.rates.GetRates(txCtx, symbol, windowStart, windowEnd)
		if err != nil {
			return err
		}

		built, err = service.BuildBatch(BuildBatchInput{
			FundingBatchID:  p.idgen.NewID("fb"),
			SymbolID:        symbol.ID,
			Symbol:          symbol.Symbol,
			TimeWindowStart: windowStart,
			TimeWindowEnd:   windowEnd,
			SettlementPrice: settlement.Price,
			Sources:         sources,
			Positions:       positions,
			CreatedAt:       now.UTC(),
		})
		if err != nil {
			return err
		}
		if err := p.repo.CreateBatch(txCtx, built.Batch); err != nil {
			return err
		}
		if err := p.repo.CreateBatchItems(txCtx, built.Items); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, nil
	}
	return &built, nil
}

func (p *Planner) serviceForSymbol(symbol string) (*Service, error) {
	cfg := p.service.cfg
	if p.runtime != nil {
		override := p.runtime.CurrentFundingRuntimeConfig(symbol)
		if override.SettlementIntervalSec > 0 {
			cfg.SettlementIntervalSec = override.SettlementIntervalSec
		}
		if override.CapRatePerHour != "" {
			cfg.CapRatePerHour = override.CapRatePerHour
		}
		if override.MinValidSourceCount > 0 {
			cfg.MinValidSourceCount = override.MinValidSourceCount
		}
		if override.DefaultCryptoModel != "" {
			cfg.DefaultCryptoModel = override.DefaultCryptoModel
		}
	}
	return NewService(cfg)
}

func alignWindowEnd(now time.Time, intervalSec int) time.Time {
	if intervalSec <= 0 {
		return time.Time{}
	}
	interval := int64(intervalSec)
	aligned := now.UTC().Unix() / interval * interval
	return time.Unix(aligned, 0).UTC()
}
