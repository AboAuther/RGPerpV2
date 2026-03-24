package hedge

import (
	"context"
	"fmt"
	"strings"
	"time"

	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
)

type ManagedPositionReader interface {
	GetManagedNetPosition(ctx context.Context, symbol string) (string, error)
}

type SimulatedAdapter struct {
	reader ManagedPositionReader
}

func NewSimulatedAdapter(reader ManagedPositionReader) *SimulatedAdapter {
	return &SimulatedAdapter{reader: reader}
}

func (a *SimulatedAdapter) PlaceOrder(_ context.Context, req hedgedomain.ExecutionRequest) (hedgedomain.ExecutionResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	side := strings.ToUpper(strings.TrimSpace(req.Side))
	if symbol == "" || side == "" || strings.TrimSpace(req.Qty) == "" {
		return hedgedomain.ExecutionResult{}, fmt.Errorf("invalid simulated hedge order")
	}
	fillPrice := "0"
	if req.Price != nil && strings.TrimSpace(*req.Price) != "" {
		fillPrice = strings.TrimSpace(*req.Price)
	}
	now := time.Now().UTC().UnixNano()
	return hedgedomain.ExecutionResult{
		VenueOrderID: fmt.Sprintf("sim_%s_%d", strings.ToLower(symbol), now),
		Status:       hedgedomain.OrderStatusFilled,
		Fills: []hedgedomain.ExecutionFill{{
			VenueFillID: fmt.Sprintf("sim_fill_%d", now),
			Qty:         strings.TrimSpace(req.Qty),
			Price:       fillPrice,
			Fee:         "0",
		}},
	}, nil
}

func (a *SimulatedAdapter) GetNetPosition(ctx context.Context, symbol string) (string, error) {
	if a == nil || a.reader == nil {
		return "0", nil
	}
	return a.reader.GetManagedNetPosition(ctx, symbol)
}
