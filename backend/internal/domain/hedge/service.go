package hedge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ServiceConfig struct {
	Venue string
}

type Service struct {
	cfg    ServiceConfig
	clock  Clock
	idgen  IDGenerator
	txm    TxManager
	repo   Repository
	venue  VenueAdapter
	outbox OutboxPublisher
}

// NewService constructs a hedge executor that is intentionally separate from
// user trading state. The hedge domain manages platform risk actions without
// mutating customer orders or positions.
func NewService(cfg ServiceConfig, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, venue VenueAdapter, outbox OutboxPublisher) (*Service, error) {
	if strings.TrimSpace(cfg.Venue) == "" {
		return nil, fmt.Errorf("%w: hedge venue is required", errorsx.ErrInvalidArgument)
	}
	if clock == nil || idgen == nil || txm == nil || repo == nil || venue == nil || outbox == nil {
		return nil, fmt.Errorf("%w: missing hedge dependency", errorsx.ErrInvalidArgument)
	}
	return &Service{
		cfg:    cfg,
		clock:  clock,
		idgen:  idgen,
		txm:    txm,
		repo:   repo,
		venue:  venue,
		outbox: outbox,
	}, nil
}

// ExecuteIntent processes a single hedge intent against the configured venue.
// Only the latest open intent per symbol is allowed to execute, which prevents
// historical backlog from amplifying platform exposure.
func (s *Service) ExecuteIntent(ctx context.Context, intentID string) (Intent, error) {
	if strings.TrimSpace(intentID) == "" {
		return Intent{}, fmt.Errorf("%w: intent id is required", errorsx.ErrInvalidArgument)
	}

	var intent Intent
	var order Order
	now := s.clock.Now().UTC()
	if err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		loadedIntent, err := s.repo.GetIntentForUpdate(txCtx, intentID)
		if err != nil {
			return err
		}
		if loadedIntent.Status == IntentStatusCompleted {
			intent = loadedIntent
			return nil
		}
		if loadedIntent.Status != IntentStatusPending && loadedIntent.Status != IntentStatusExecuting {
			return fmt.Errorf("%w: hedge intent is not executable", errorsx.ErrConflict)
		}
		latestOpenIntent, err := s.repo.GetLatestOpenIntentForSymbolForUpdate(txCtx, loadedIntent.SymbolID)
		if err != nil && err != errorsx.ErrNotFound {
			return err
		}
		if err == nil && latestOpenIntent.ID != "" && latestOpenIntent.ID != loadedIntent.ID {
			loadedIntent.Status = IntentStatusSuperseded
			loadedIntent.UpdatedAt = now
			if err := s.repo.UpdateIntent(txCtx, loadedIntent); err != nil {
				return err
			}
			intent = loadedIntent
			return s.outbox.Publish(txCtx, DomainEvent{
				EventID:       s.idgen.NewID("evt"),
				AggregateType: "hedge_intent",
				AggregateID:   loadedIntent.ID,
				EventType:     "hedge.superseded",
				Payload: map[string]any{
					"hedge_intent_id":         loadedIntent.ID,
					"superseded_by_intent_id": latestOpenIntent.ID,
					"symbol":                  loadedIntent.Symbol,
					"side":                    loadedIntent.Side,
					"status":                  loadedIntent.Status,
				},
				CreatedAt: now,
			})
		}

		loadedIntent.Status = IntentStatusExecuting
		loadedIntent.UpdatedAt = now
		if err := s.repo.UpdateIntent(txCtx, loadedIntent); err != nil {
			return err
		}
		intent = loadedIntent

		loadedOrder, err := s.repo.GetLatestOrderByIntentForUpdate(txCtx, intent.ID)
		if err != nil {
			if err != errorsx.ErrNotFound {
				return err
			}
			loadedOrder = Order{
				ID:        s.idgen.NewID("hord"),
				IntentID:  intent.ID,
				Venue:     s.cfg.Venue,
				Symbol:    intent.Symbol,
				Side:      intent.Side,
				Qty:       intent.TargetQty,
				Status:    OrderStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.repo.CreateOrder(txCtx, loadedOrder); err != nil {
				return err
			}
		}
		order = loadedOrder
		return nil
	}); err != nil {
		return Intent{}, err
	}
	if intent.Status == IntentStatusCompleted || intent.Status == IntentStatusSuperseded {
		return intent, nil
	}

	result, execErr := s.venue.PlaceOrder(ctx, ExecutionRequest{
		Symbol: intent.Symbol,
		Side:   intent.Side,
		Qty:    intent.TargetQty,
		Price:  order.Price,
	})

	if err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		loadedIntent, err := s.repo.GetIntentForUpdate(txCtx, intent.ID)
		if err != nil {
			return err
		}
		loadedOrder, err := s.repo.GetLatestOrderByIntentForUpdate(txCtx, intent.ID)
		if err != nil {
			return err
		}

		if execErr != nil {
			loadedIntent.Status = IntentStatusFailed
			loadedIntent.UpdatedAt = now
			loadedOrder.Status = OrderStatusFailed
			loadedOrder.UpdatedAt = now
			errorCode := "VENUE_ERROR"
			loadedOrder.ErrorCode = &errorCode
			if err := s.repo.UpdateOrder(txCtx, loadedOrder); err != nil {
				return err
			}
			if err := s.repo.UpdateIntent(txCtx, loadedIntent); err != nil {
				return err
			}
			intent = loadedIntent
			return s.outbox.Publish(txCtx, DomainEvent{
				EventID:       s.idgen.NewID("evt"),
				AggregateType: "hedge_intent",
				AggregateID:   loadedIntent.ID,
				EventType:     "hedge.failed",
				Payload: map[string]any{
					"hedge_intent_id": loadedIntent.ID,
					"symbol":          loadedIntent.Symbol,
					"side":            loadedIntent.Side,
					"status":          loadedIntent.Status,
				},
				CreatedAt: now,
			})
		}

		if strings.TrimSpace(result.VenueOrderID) != "" {
			loadedOrder.VenueOrderID = &result.VenueOrderID
		}
		loadedOrder.Status = normalizeOrderStatus(result)
		loadedOrder.UpdatedAt = now
		loadedOrder.ErrorCode = nil
		if err := s.repo.UpdateOrder(txCtx, loadedOrder); err != nil {
			return err
		}

		totalQty := decimalx.MustFromString("0")
		weightedPrice := decimalx.MustFromString("0")
		for _, fill := range result.Fills {
			if err := s.repo.CreateFill(txCtx, Fill{
				ID:          s.idgen.NewID("hfill"),
				OrderID:     loadedOrder.ID,
				VenueFillID: fill.VenueFillID,
				Qty:         fill.Qty,
				Price:       fill.Price,
				Fee:         fill.Fee,
				CreatedAt:   now,
			}); err != nil {
				return err
			}

			if err := s.applyFill(txCtx, loadedOrder.Symbol, loadedOrder.Side, fill, now); err != nil {
				return err
			}

			fillQty := decimalx.MustFromString(fill.Qty)
			fillPrice := decimalx.MustFromString(fill.Price)
			totalQty = totalQty.Add(fillQty)
			weightedPrice = weightedPrice.Add(fillQty.Mul(fillPrice))
		}

		loadedIntent.Status = IntentStatusCompleted
		if loadedOrder.Status == OrderStatusFailed {
			loadedIntent.Status = IntentStatusFailed
		}
		loadedIntent.UpdatedAt = now
		if err := s.repo.UpdateIntent(txCtx, loadedIntent); err != nil {
			return err
		}
		intent = loadedIntent

		payload := map[string]any{
			"hedge_order_id":  loadedOrder.ID,
			"hedge_intent_id": loadedIntent.ID,
			"venue":           loadedOrder.Venue,
			"venue_order_id":  derefString(loadedOrder.VenueOrderID),
			"symbol":          loadedOrder.Symbol,
			"side":            loadedOrder.Side,
			"qty":             loadedOrder.Qty,
			"status":          loadedOrder.Status,
		}
		if !totalQty.IsZero() {
			payload["filled_qty"] = totalQty.String()
			payload["avg_fill_price"] = weightedPrice.Div(totalQty).String()
		}
		return s.outbox.Publish(txCtx, DomainEvent{
			EventID:       s.idgen.NewID("evt"),
			AggregateType: "hedge_order",
			AggregateID:   loadedOrder.ID,
			EventType:     "hedge.updated",
			Payload:       payload,
			CreatedAt:     now,
		})
	}); err != nil {
		return Intent{}, err
	}

	return intent, nil
}

func (s *Service) applyFill(ctx context.Context, symbol string, orderSide string, fill ExecutionFill, now time.Time) error {
	positionSide := PositionSideLong
	if orderSide == OrderSideSell {
		positionSide = PositionSideShort
	}

	position, err := s.repo.GetPositionForUpdate(ctx, symbol, positionSide)
	if err != nil && err != errorsx.ErrNotFound {
		return err
	}
	if err == errorsx.ErrNotFound {
		position = Position{
			Symbol:        symbol,
			Side:          positionSide,
			Qty:           "0",
			AvgEntryPrice: "0",
			RealizedPnL:   "0",
			UnrealizedPnL: "0",
		}
	}

	currentQty := decimalx.MustFromString(position.Qty)
	currentAvg := decimalx.MustFromString(position.AvgEntryPrice)
	fillQty := decimalx.MustFromString(fill.Qty)
	fillPrice := decimalx.MustFromString(fill.Price)

	// Hedge inventory is mirrored in a dedicated platform-facing position view so
	// external risk actions never leak back into the user position model.
	newQty := currentQty.Add(fillQty)
	if currentQty.IsZero() {
		currentAvg = fillPrice
	} else {
		currentAvg = currentQty.Mul(currentAvg).Add(fillQty.Mul(fillPrice)).Div(newQty)
	}

	position.Qty = newQty.String()
	position.AvgEntryPrice = currentAvg.String()
	position.UpdatedAt = now
	return s.repo.UpsertPosition(ctx, position)
}

// normalizeOrderStatus prefers explicit venue lifecycle values and otherwise
// derives a consistent internal status from the observed fills.
func normalizeOrderStatus(result ExecutionResult) string {
	switch strings.ToUpper(strings.TrimSpace(result.Status)) {
	case OrderStatusSent, OrderStatusFilled, OrderStatusFailed, OrderStatusPartial:
		return strings.ToUpper(strings.TrimSpace(result.Status))
	}
	total := decimalx.MustFromString("0")
	for _, fill := range result.Fills {
		total = total.Add(decimalx.MustFromString(fill.Qty))
	}
	if total.IsZero() {
		return OrderStatusSent
	}
	return OrderStatusFilled
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
