package market

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type Service struct {
	cfg      AggregationConfig
	catalog  CatalogRepository
	snapshot SnapshotRepository
	clients  map[string]SourceClient
}

func NewService(cfg AggregationConfig, catalog CatalogRepository, snapshot SnapshotRepository, clients []SourceClient) (*Service, error) {
	if catalog == nil || snapshot == nil {
		return nil, fmt.Errorf("%w: catalog and snapshot repositories are required", errorsx.ErrInvalidArgument)
	}
	if cfg.MaxSourceAge <= 0 || cfg.MinHealthySource <= 0 {
		return nil, fmt.Errorf("%w: invalid aggregation config", errorsx.ErrInvalidArgument)
	}
	clientMap := make(map[string]SourceClient, len(clients))
	for _, client := range clients {
		clientMap[client.Name()] = client
	}
	return &Service{
		cfg:      cfg,
		catalog:  catalog,
		snapshot: snapshot,
		clients:  clientMap,
	}, nil
}

func (s *Service) SyncOnce(ctx context.Context, now time.Time) error {
	symbols, err := s.catalog.ListActiveSymbols(ctx)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return nil
	}

	requestsBySource := make(map[string][]SourceSymbolRequest)
	for _, symbol := range symbols {
		for _, mapping := range symbol.Mappings {
			if mapping.Status != "ACTIVE" {
				continue
			}
			if _, ok := s.clients[mapping.SourceName]; !ok {
				continue
			}
			requestsBySource[mapping.SourceName] = append(requestsBySource[mapping.SourceName], SourceSymbolRequest{
				CanonicalSymbol: symbol.Symbol,
				SourceSymbol:    mapping.SourceSymbol,
			})
		}
	}

	fetchedQuotes := make(map[string]map[string]SourceQuote)
	var fetchErrs []error
	for sourceName, requests := range requestsBySource {
		if len(requests) == 0 {
			continue
		}
		quotes, err := s.clients[sourceName].Fetch(ctx, requests)
		if err != nil {
			fetchErrs = append(fetchErrs, fmt.Errorf("%s fetch failed: %w", sourceName, err))
			continue
		}
		fetchedQuotes[sourceName] = quotes
	}

	sourceSnapshots := make([]SourcePriceSnapshot, 0, len(symbols)*2)
	aggregatedSnapshots := make([]AggregatedPrice, 0, len(symbols))
	runtimeStates := make([]SymbolRuntimeState, 0, len(symbols))
	var aggregateErrs []error
	for _, symbol := range symbols {
		aggregated, rawSnapshots, err := s.aggregateSymbol(now, symbol, fetchedQuotes)
		sourceSnapshots = append(sourceSnapshots, rawSnapshots...)
		if err != nil {
			aggregateErrs = append(aggregateErrs, fmt.Errorf("%s aggregate failed: %w", symbol.Symbol, err))
			runtimeStates = append(runtimeStates, SymbolRuntimeState{
				SymbolID:       symbol.ID,
				DesiredStatus:  "REDUCE_ONLY",
				DegradedReason: err.Error(),
			})
			continue
		}
		aggregatedSnapshots = append(aggregatedSnapshots, aggregated)
		runtimeStates = append(runtimeStates, SymbolRuntimeState{
			SymbolID:      symbol.ID,
			DesiredStatus: "TRADING",
		})
	}

	if len(sourceSnapshots) > 0 {
		if err := s.snapshot.AppendSourceSnapshots(ctx, sourceSnapshots); err != nil {
			return err
		}
	}
	if len(aggregatedSnapshots) > 0 || len(runtimeStates) > 0 {
		if err := s.snapshot.ApplyAggregatedState(ctx, aggregatedSnapshots, runtimeStates); err != nil {
			return err
		}
	}
	if len(aggregatedSnapshots) == 0 {
		if len(fetchErrs) > 0 {
			return errors.Join(append(fetchErrs, aggregateErrs...)...)
		}
		if len(aggregateErrs) > 0 {
			return errors.Join(aggregateErrs...)
		}
		return fmt.Errorf("%w: no market prices aggregated", errorsx.ErrConflict)
	}
	return nil
}

func (s *Service) aggregateSymbol(now time.Time, symbol Symbol, fetchedQuotes map[string]map[string]SourceQuote) (AggregatedPrice, []SourcePriceSnapshot, error) {
	type acceptedQuote struct {
		SourceName string
		Bid        decimalx.Decimal
		Ask        decimalx.Decimal
		Mid        decimalx.Decimal
		Weight     decimalx.Decimal
		SourceTS   time.Time
		ReceivedTS time.Time
	}

	rawSnapshots := make([]SourcePriceSnapshot, 0, len(symbol.Mappings))
	candidates := make([]acceptedQuote, 0, len(symbol.Mappings))
	mids := make([]decimalx.Decimal, 0, len(symbol.Mappings))

	for _, mapping := range symbol.Mappings {
		quotesBySource, ok := fetchedQuotes[mapping.SourceName]
		if !ok {
			continue
		}
		quote, ok := quotesBySource[mapping.SourceSymbol]
		if !ok {
			continue
		}
		if now.Sub(quote.SourceTS) > s.cfg.MaxSourceAge {
			continue
		}
		scale := decimalx.MustFromString(mapping.PriceScale)
		bid := decimalx.MustFromString(quote.Bid).Mul(scale)
		ask := decimalx.MustFromString(quote.Ask).Mul(scale)
		last := decimalx.MustFromString(quote.Last).Mul(scale)
		mid := midpoint(bid, ask, last)
		if mid.IsZero() {
			continue
		}
		rawSnapshots = append(rawSnapshots, SourcePriceSnapshot{
			SymbolID:    symbol.ID,
			SourceName:  mapping.SourceName,
			Bid:         bid.String(),
			Ask:         ask.String(),
			Last:        last.String(),
			Mid:         mid.String(),
			SourceTS:    quote.SourceTS,
			ReceivedTS:  quote.ReceivedTS,
			CanonicalTS: now,
		})
		health, ok := s.cfg.SourceHealth[mapping.SourceName]
		if !ok || !health.Enabled {
			continue
		}
		weight := decimalx.MustFromString(health.Weight)
		candidates = append(candidates, acceptedQuote{
			SourceName: mapping.SourceName,
			Bid:        bid,
			Ask:        ask,
			Mid:        mid,
			Weight:     weight,
			SourceTS:   quote.SourceTS,
			ReceivedTS: quote.ReceivedTS,
		})
		mids = append(mids, mid)
	}

	if len(candidates) < s.cfg.MinHealthySource {
		return AggregatedPrice{}, rawSnapshots, fmt.Errorf("%w: insufficient healthy sources", errorsx.ErrConflict)
	}

	median := medianDecimal(mids)
	maxDeviation := decimalx.MustFromString(s.cfg.MaxDeviationBps)
	accepted := make([]acceptedQuote, 0, len(candidates))
	for _, candidate := range candidates {
		if withinDeviationBps(candidate.Mid, median, maxDeviation) {
			accepted = append(accepted, candidate)
		}
	}
	if len(accepted) < s.cfg.MinHealthySource {
		return AggregatedPrice{}, rawSnapshots, fmt.Errorf("%w: all quotes diverged", errorsx.ErrConflict)
	}

	totalWeight := decimalx.MustFromString("0")
	indexAcc := decimalx.MustFromString("0")
	bestBid := accepted[0].Bid
	bestAsk := accepted[0].Ask
	for _, item := range accepted {
		totalWeight = totalWeight.Add(item.Weight)
		indexAcc = indexAcc.Add(item.Mid.Mul(item.Weight))
		if item.Bid.GreaterThan(bestBid) {
			bestBid = item.Bid
		}
		if bestAsk.IsZero() || item.Ask.LessThan(bestAsk) {
			bestAsk = item.Ask
		}
	}
	if totalWeight.IsZero() {
		return AggregatedPrice{}, rawSnapshots, fmt.Errorf("%w: zero source weight", errorsx.ErrConflict)
	}
	indexPrice := indexAcc.Div(totalWeight)
	markPrice := clampMarkPrice(indexPrice, bestBid, bestAsk, decimalx.MustFromString(s.cfg.MarkClampBps))

	return AggregatedPrice{
		SymbolID:      symbol.ID,
		IndexPrice:    indexPrice.String(),
		MarkPrice:     markPrice.String(),
		BestBid:       bestBid.String(),
		BestAsk:       bestAsk.String(),
		CalcVersion:   now.UTC().UnixMilli(),
		CreatedAt:     now.UTC(),
		HealthyCount:  len(candidates),
		AcceptedCount: len(accepted),
	}, rawSnapshots, nil
}

func midpoint(bid decimalx.Decimal, ask decimalx.Decimal, last decimalx.Decimal) decimalx.Decimal {
	zero := decimalx.MustFromString("0")
	two := decimalx.MustFromString("2")
	if bid.GreaterThan(zero) && ask.GreaterThan(zero) {
		return bid.Add(ask).Div(two)
	}
	return last
}

func withinDeviationBps(value decimalx.Decimal, median decimalx.Decimal, limitBps decimalx.Decimal) bool {
	if median.IsZero() {
		return false
	}
	diff := value.Sub(median).Abs()
	bps := diff.Mul(decimalx.MustFromString("10000")).Div(median.Abs())
	return bps.LessThanOrEqual(limitBps)
}

func medianDecimal(values []decimalx.Decimal) decimalx.Decimal {
	if len(values) == 0 {
		return decimalx.MustFromString("0")
	}
	items := append([]decimalx.Decimal(nil), values...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].LessThan(items[j])
	})
	mid := len(items) / 2
	if len(items)%2 == 1 {
		return items[mid]
	}
	return items[mid-1].Add(items[mid]).Div(decimalx.MustFromString("2"))
}

func clampMarkPrice(indexPrice decimalx.Decimal, bestBid decimalx.Decimal, bestAsk decimalx.Decimal, clampBps decimalx.Decimal) decimalx.Decimal {
	if indexPrice.IsZero() {
		return indexPrice
	}
	band := indexPrice.Mul(clampBps).Div(decimalx.MustFromString("10000"))
	lower := indexPrice.Sub(band)
	upper := indexPrice.Add(band)
	if bestBid.GreaterThan(lower) {
		lower = bestBid
	}
	if !bestAsk.IsZero() && bestAsk.LessThan(upper) {
		upper = bestAsk
	}
	if indexPrice.LessThan(lower) {
		return lower
	}
	if indexPrice.GreaterThan(upper) {
		return upper
	}
	return indexPrice
}
