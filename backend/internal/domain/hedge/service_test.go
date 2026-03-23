package hedge

import (
	"context"
	"testing"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type hedgeFakeClock struct{ now time.Time }

func (f hedgeFakeClock) Now() time.Time { return f.now }

type hedgeFakeIDGen struct {
	values []string
	idx    int
}

func (f *hedgeFakeIDGen) NewID(_ string) string {
	value := f.values[f.idx]
	f.idx++
	return value
}

type hedgeStubTxManager struct{}

func (hedgeStubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type hedgeStubRepo struct {
	intent      Intent
	order       Order
	orderErr    error
	fills       []Fill
	position    Position
	positionErr error
}

func (s *hedgeStubRepo) GetIntentForUpdate(context.Context, string) (Intent, error) {
	return s.intent, nil
}

func (s *hedgeStubRepo) GetLatestOrderByIntentForUpdate(context.Context, string) (Order, error) {
	if s.orderErr != nil {
		return Order{}, s.orderErr
	}
	return s.order, nil
}

func (s *hedgeStubRepo) CreateOrder(_ context.Context, order Order) error {
	s.order = order
	s.orderErr = nil
	return nil
}

func (s *hedgeStubRepo) UpdateOrder(_ context.Context, order Order) error {
	s.order = order
	return nil
}

func (s *hedgeStubRepo) UpdateIntent(_ context.Context, intent Intent) error {
	s.intent = intent
	return nil
}

func (s *hedgeStubRepo) CreateFill(_ context.Context, fill Fill) error {
	s.fills = append(s.fills, fill)
	return nil
}

func (s *hedgeStubRepo) GetPositionForUpdate(context.Context, string, string) (Position, error) {
	if s.positionErr != nil {
		return Position{}, s.positionErr
	}
	return s.position, nil
}

func (s *hedgeStubRepo) UpsertPosition(_ context.Context, position Position) error {
	s.position = position
	s.positionErr = nil
	return nil
}

type hedgeStubVenue struct {
	result ExecutionResult
	err    error
}

func (s hedgeStubVenue) PlaceOrder(context.Context, ExecutionRequest) (ExecutionResult, error) {
	return s.result, s.err
}

type hedgeStubOutbox struct {
	events []DomainEvent
}

func (s *hedgeStubOutbox) Publish(_ context.Context, event DomainEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestExecuteIntentUpdatesMirrorPosition(t *testing.T) {
	repo := &hedgeStubRepo{
		intent: Intent{
			ID:        "hint_1",
			SymbolID:  1,
			Symbol:    "BTC-PERP",
			Side:      OrderSideSell,
			TargetQty: "2",
			Status:    IntentStatusPending,
		},
		orderErr:    errorsx.ErrNotFound,
		positionErr: errorsx.ErrNotFound,
	}
	outbox := &hedgeStubOutbox{}
	service, err := NewService(ServiceConfig{Venue: "hyperliquid_testnet"}, hedgeFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &hedgeFakeIDGen{values: []string{"hord_1", "hfill_1", "evt_1"}}, hedgeStubTxManager{}, repo, hedgeStubVenue{
		result: ExecutionResult{
			VenueOrderID: "venue_1",
			Status:       "FILLED",
			Fills: []ExecutionFill{{
				VenueFillID: "venue_fill_1",
				Qty:         "2",
				Price:       "100",
				Fee:         "0.1",
			}},
		},
	}, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	intent, err := service.ExecuteIntent(context.Background(), "hint_1")
	if err != nil {
		t.Fatalf("execute intent: %v", err)
	}
	if intent.Status != IntentStatusCompleted {
		t.Fatalf("expected completed intent, got %s", intent.Status)
	}
	if repo.order.Status != OrderStatusFilled || repo.order.VenueOrderID == nil || *repo.order.VenueOrderID != "venue_1" {
		t.Fatalf("unexpected order: %+v", repo.order)
	}
	if len(repo.fills) != 1 {
		t.Fatalf("expected 1 hedge fill, got %d", len(repo.fills))
	}
	if repo.position.Side != PositionSideShort || repo.position.Qty != "2" || repo.position.AvgEntryPrice != "100" {
		t.Fatalf("unexpected hedge position: %+v", repo.position)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "hedge.updated" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
}
