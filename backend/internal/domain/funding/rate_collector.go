package funding

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type RateCollector struct {
	clock   Clock
	repo    Repository
	clients map[string]RateSourceClient
}

func NewRateCollector(clock Clock, repo Repository, clients []RateSourceClient) (*RateCollector, error) {
	if clock == nil || repo == nil {
		return nil, fmt.Errorf("%w: missing funding rate collector dependency", errorsx.ErrInvalidArgument)
	}
	clientMap := make(map[string]RateSourceClient, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		clientMap[client.Name()] = client
	}
	if len(clientMap) == 0 {
		return nil, fmt.Errorf("%w: funding rate clients are required", errorsx.ErrInvalidArgument)
	}
	return &RateCollector{
		clock:   clock,
		repo:    repo,
		clients: clientMap,
	}, nil
}

func (c *RateCollector) SyncOnce(ctx context.Context, now time.Time) error {
	symbols, err := c.repo.ListSymbolsForFunding(ctx)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return nil
	}

	requestsBySource := make(map[string][]SourceRateRequest)
	for _, symbol := range symbols {
		for _, mapping := range symbol.Mappings {
			if mapping.Status != "ACTIVE" {
				continue
			}
			if _, ok := c.clients[mapping.SourceName]; !ok {
				continue
			}
			requestsBySource[mapping.SourceName] = append(requestsBySource[mapping.SourceName], SourceRateRequest{
				CanonicalSymbol: symbol.Symbol,
				SourceSymbol:    mapping.SourceSymbol,
			})
		}
	}

	collected := make(map[string]map[string]SourceRateQuote)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error
	for sourceName, requests := range requestsBySource {
		if len(requests) == 0 {
			continue
		}
		client := c.clients[sourceName]
		wg.Add(1)
		go func(sourceName string, requests []SourceRateRequest) {
			defer wg.Done()
			rates, err := client.FetchRates(ctx, requests)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s funding fetch failed: %w", sourceName, err))
				return
			}
			collected[sourceName] = rates
		}(sourceName, requests)
	}
	wg.Wait()

	snapshots := make([]RateSnapshot, 0, len(symbols)*2)
	collectedAt := now.UTC()
	for _, symbol := range symbols {
		for _, mapping := range symbol.Mappings {
			bySource, ok := collected[mapping.SourceName]
			if !ok {
				continue
			}
			quote, ok := bySource[mapping.SourceSymbol]
			if !ok || quote.Rate == "" || quote.IntervalSeconds <= 0 {
				continue
			}
			snapshots = append(snapshots, RateSnapshot{
				SymbolID:        symbol.ID,
				Symbol:          symbol.Symbol,
				SourceName:      quote.SourceName,
				SourceSymbol:    quote.SourceSymbol,
				Rate:            quote.Rate,
				IntervalSeconds: quote.IntervalSeconds,
				SourceTS:        quote.SourceTS,
				ReceivedTS:      quote.ReceivedTS,
				CollectedAt:     collectedAt,
			})
		}
	}
	if len(snapshots) > 0 {
		if err := c.repo.AppendRateSnapshots(ctx, snapshots); err != nil {
			return err
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", errorsx.ErrConflict, errs)
	}
	return nil
}
