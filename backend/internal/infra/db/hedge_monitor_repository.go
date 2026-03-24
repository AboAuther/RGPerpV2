package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"gorm.io/gorm"
)

type HedgeSnapshotState struct {
	SymbolID         uint64
	Symbol           string
	StepSize         string
	MinNotional      string
	MarkPrice        string
	InternalLongQty  string
	InternalShortQty string
	ManagedLongQty   string
	ManagedShortQty  string
}

type HedgeSnapshotRecord struct {
	SymbolID         uint64
	Symbol           string
	InternalNetQty   string
	TargetHedgeQty   string
	ManagedHedgeQty  string
	ExternalHedgeQty string
	ManagedDriftQty  string
	ExternalDriftQty string
	HedgeHealthy     bool
	CreatedAt        time.Time
}

type HedgeMonitorRepository struct {
	db *gorm.DB
}

func NewHedgeMonitorRepository(db *gorm.DB) *HedgeMonitorRepository {
	return &HedgeMonitorRepository{db: db}
}

func (r *HedgeMonitorRepository) ListSnapshotStates(ctx context.Context) ([]HedgeSnapshotState, error) {
	var symbols []SymbolModel
	if err := DB(ctx, r.db).
		Where("status IN ?", []string{"TRADING", "REDUCE_ONLY"}).
		Order("id ASC").
		Find(&symbols).Error; err != nil {
		return nil, err
	}
	if len(symbols) == 0 {
		return []HedgeSnapshotState{}, nil
	}

	symbolIDs := make([]uint64, 0, len(symbols))
	symbolNames := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		symbolIDs = append(symbolIDs, symbol.ID)
		symbolNames = append(symbolNames, symbol.Symbol)
	}

	var positionRows []struct {
		SymbolID uint64 `gorm:"column:symbol_id"`
		Side     string `gorm:"column:side"`
		Qty      string `gorm:"column:qty"`
	}
	if err := DB(ctx, r.db).
		Table("positions").
		Select("symbol_id, side, qty").
		Where("symbol_id IN ? AND status = ?", symbolIDs, "OPEN").
		Scan(&positionRows).Error; err != nil {
		return nil, err
	}

	var hedgeRows []HedgePositionModel
	if err := DB(ctx, r.db).
		Where("symbol IN ?", symbolNames).
		Find(&hedgeRows).Error; err != nil {
		return nil, err
	}

	var markRows []MarkPriceSnapshotModel
	if err := DB(ctx, r.db).Raw(`
SELECT m1.*
FROM mark_price_snapshots m1
JOIN (
  SELECT symbol_id, MAX(id) AS max_id
  FROM mark_price_snapshots
  WHERE symbol_id IN ?
  GROUP BY symbol_id
) latest ON latest.max_id = m1.id
`, symbolIDs).Scan(&markRows).Error; err != nil {
		return nil, err
	}
	markBySymbol := make(map[uint64]string, len(markRows))
	for _, row := range markRows {
		markBySymbol[row.SymbolID] = row.MarkPrice
	}

	type qtyPair struct {
		longQty  string
		shortQty string
	}
	internalBySymbol := make(map[uint64]qtyPair, len(symbols))
	for _, row := range positionRows {
		current := internalBySymbol[row.SymbolID]
		if row.Side == "LONG" {
			current.longQty = decimalAdd(zeroIfEmpty(current.longQty), zeroIfEmpty(row.Qty))
		} else {
			current.shortQty = decimalAdd(zeroIfEmpty(current.shortQty), zeroIfEmpty(row.Qty))
		}
		internalBySymbol[row.SymbolID] = current
	}
	symbolIDByName := make(map[string]uint64, len(symbols))
	for _, symbol := range symbols {
		symbolIDByName[symbol.Symbol] = symbol.ID
	}
	managedBySymbol := make(map[uint64]qtyPair, len(symbols))
	for _, row := range hedgeRows {
		symbolID, ok := symbolIDByName[row.Symbol]
		if !ok {
			continue
		}
		current := managedBySymbol[symbolID]
		if row.Side == "LONG" {
			current.longQty = decimalAdd(zeroIfEmpty(current.longQty), zeroIfEmpty(row.Qty))
		} else {
			current.shortQty = decimalAdd(zeroIfEmpty(current.shortQty), zeroIfEmpty(row.Qty))
		}
		managedBySymbol[symbolID] = current
	}

	out := make([]HedgeSnapshotState, 0, len(symbols))
	for _, symbol := range symbols {
		internal := internalBySymbol[symbol.ID]
		managed := managedBySymbol[symbol.ID]
		out = append(out, HedgeSnapshotState{
			SymbolID:         symbol.ID,
			Symbol:           symbol.Symbol,
			StepSize:         symbol.StepSize,
			MinNotional:      symbol.MinNotional,
			MarkPrice:        zeroIfEmpty(markBySymbol[symbol.ID]),
			InternalLongQty:  zeroIfEmpty(internal.longQty),
			InternalShortQty: zeroIfEmpty(internal.shortQty),
			ManagedLongQty:   zeroIfEmpty(managed.longQty),
			ManagedShortQty:  zeroIfEmpty(managed.shortQty),
		})
	}
	return out, nil
}

func (r *HedgeMonitorRepository) GetManagedNetPosition(ctx context.Context, symbol string) (string, error) {
	var rows []HedgePositionModel
	if err := DB(ctx, r.db).
		Where("symbol = ?", strings.ToUpper(strings.TrimSpace(symbol))).
		Find(&rows).Error; err != nil {
		return "", err
	}
	net := decimalx.MustFromString("0")
	for _, row := range rows {
		qty := decimalx.MustFromString(zeroIfEmpty(row.Qty))
		if row.Side == "SHORT" {
			net = net.Sub(qty)
			continue
		}
		net = net.Add(qty)
	}
	return net.String(), nil
}

func (r *HedgeMonitorRepository) CreateSystemSnapshot(ctx context.Context, snapshot HedgeSnapshotRecord) error {
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	return DB(ctx, r.db).Create(&SystemHedgeSnapshotModel{
		SymbolID:         snapshot.SymbolID,
		Symbol:           snapshot.Symbol,
		InternalNetQty:   snapshot.InternalNetQty,
		TargetHedgeQty:   snapshot.TargetHedgeQty,
		ManagedHedgeQty:  snapshot.ManagedHedgeQty,
		ExternalHedgeQty: snapshot.ExternalHedgeQty,
		ManagedDriftQty:  snapshot.ManagedDriftQty,
		ExternalDriftQty: snapshot.ExternalDriftQty,
		HedgeHealthy:     snapshot.HedgeHealthy,
		CreatedAt:        snapshot.CreatedAt,
	}).Error
}

func ComputeHedgeHealthyThreshold(stepSize string, minNotional string, markPrice string) (string, error) {
	threshold := decimalx.MustFromString(zeroIfEmpty(stepSize))
	if !threshold.GreaterThan(decimalx.MustFromString("0")) {
		threshold = decimalx.MustFromString("0.0001")
	}
	notional := decimalx.MustFromString(zeroIfEmpty(minNotional))
	price := decimalx.MustFromString(zeroIfEmpty(markPrice))
	if notional.GreaterThan(decimalx.MustFromString("0")) && price.GreaterThan(decimalx.MustFromString("0")) {
		minSize := notional.Div(price)
		if minSize.GreaterThan(threshold) {
			threshold = minSize
		}
	}
	if !threshold.GreaterThan(decimalx.MustFromString("0")) {
		return "", fmt.Errorf("hedge threshold must be positive")
	}
	return threshold.String(), nil
}

func zeroIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return strings.TrimSpace(value)
}
