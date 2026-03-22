package marketcache

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	snapshotKeyPrefix = "market:latest:symbol:"
	symbolSetKey      = "market:latest:symbols"
)

type Snapshot struct {
	SymbolID              uint64    `json:"symbol_id"`
	Symbol                string    `json:"symbol"`
	Status                string    `json:"status"`
	ContractMultiplier    string    `json:"contract_multiplier"`
	TickSize              string    `json:"tick_size"`
	StepSize              string    `json:"step_size"`
	MinNotional           string    `json:"min_notional"`
	InitialMarginRate     string    `json:"initial_margin_rate"`
	MaintenanceMarginRate string    `json:"maintenance_margin_rate"`
	IndexPrice            string    `json:"index_price"`
	MarkPrice             string    `json:"mark_price"`
	BestBid               string    `json:"best_bid"`
	BestAsk               string    `json:"best_ask"`
	TS                    time.Time `json:"ts"`
}

type Cache struct {
	client *redis.Client
}

func New(addr string, password string, db int) *Cache {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	return &Cache{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

func (c *Cache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Cache) StoreSnapshots(ctx context.Context, snapshots []Snapshot) error {
	if c == nil || c.client == nil || len(snapshots) == 0 {
		return nil
	}
	pipe := c.client.TxPipeline()
	for _, snapshot := range snapshots {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		key := snapshotKey(snapshot.Symbol)
		pipe.Set(ctx, key, payload, 0)
		pipe.SAdd(ctx, symbolSetKey, snapshot.Symbol)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Cache) GetTradableSymbol(ctx context.Context, symbol string) (orderdomain.TradableSymbol, error) {
	snapshot, err := c.getSnapshot(ctx, symbol)
	if err != nil {
		return orderdomain.TradableSymbol{}, err
	}
	return orderdomain.TradableSymbol{
		SymbolID:              snapshot.SymbolID,
		Symbol:                snapshot.Symbol,
		ContractMultiplier:    snapshot.ContractMultiplier,
		TickSize:              snapshot.TickSize,
		StepSize:              snapshot.StepSize,
		MinNotional:           snapshot.MinNotional,
		Status:                snapshot.Status,
		IndexPrice:            snapshot.IndexPrice,
		MarkPrice:             snapshot.MarkPrice,
		BestBid:               snapshot.BestBid,
		BestAsk:               snapshot.BestAsk,
		InitialMarginRate:     snapshot.InitialMarginRate,
		MaintenanceMarginRate: snapshot.MaintenanceMarginRate,
		SnapshotTS:            snapshot.TS,
	}, nil
}

func (c *Cache) ListTickers(ctx context.Context) ([]readmodel.TickerItem, error) {
	if c == nil || c.client == nil {
		return nil, errorsx.ErrNotFound
	}
	symbols, err := c.client.SMembers(ctx, symbolSetKey).Result()
	if err != nil {
		return nil, err
	}
	if len(symbols) == 0 {
		return nil, errorsx.ErrNotFound
	}
	sort.Strings(symbols)
	keys := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		keys = append(keys, snapshotKey(symbol))
	}
	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]readmodel.TickerItem, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		raw, ok := value.(string)
		if !ok || raw == "" {
			continue
		}
		var snapshot Snapshot
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return nil, err
		}
		out = append(out, readmodel.TickerItem{
			Symbol:     snapshot.Symbol,
			IndexPrice: snapshot.IndexPrice,
			MarkPrice:  snapshot.MarkPrice,
			BestBid:    snapshot.BestBid,
			BestAsk:    snapshot.BestAsk,
			Status:     snapshot.Status,
			Stale:      false,
			TS:         snapshot.TS.Format(time.RFC3339),
		})
	}
	if len(out) == 0 {
		return nil, errorsx.ErrNotFound
	}
	return out, nil
}

func (c *Cache) getSnapshot(ctx context.Context, symbol string) (Snapshot, error) {
	if c == nil || c.client == nil {
		return Snapshot{}, errorsx.ErrNotFound
	}
	payload, err := c.client.Get(ctx, snapshotKey(symbol)).Result()
	if err != nil {
		if err == redis.Nil {
			return Snapshot{}, errorsx.ErrNotFound
		}
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode market cache snapshot: %w", err)
	}
	return snapshot, nil
}

func snapshotKey(symbol string) string {
	return snapshotKeyPrefix + strings.ToUpper(strings.TrimSpace(symbol))
}
