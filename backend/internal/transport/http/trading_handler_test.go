package httptransport

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type fakeTradingReader struct{}

func (fakeTradingReader) ListOrders(_ context.Context, _ uint64) ([]readmodel.OrderItem, error) {
	return []readmodel.OrderItem{{OrderID: "ord_1", Symbol: "BTC-PERP", Status: "RESTING"}}, nil
}
func (fakeTradingReader) ListFills(_ context.Context, _ uint64) ([]readmodel.FillItem, error) {
	return []readmodel.FillItem{{FillID: "fill_1", OrderID: "ord_1", Symbol: "BTC-PERP"}}, nil
}
func (fakeTradingReader) ListPositions(_ context.Context, _ uint64) ([]readmodel.PositionItem, error) {
	return []readmodel.PositionItem{{PositionID: "pos_1", Symbol: "BTC-PERP", Side: "LONG"}}, nil
}

type fakeTradingMutator struct {
	createInput orderdomain.CreateOrderInput
	cancelInput orderdomain.CancelOrderInput
}

func (f *fakeTradingMutator) CreateOrder(_ context.Context, input orderdomain.CreateOrderInput) (orderdomain.Order, error) {
	f.createInput = input
	return orderdomain.Order{
		OrderID:        "ord_1",
		ClientOrderID:  input.ClientOrderID,
		Symbol:         input.Symbol,
		Side:           input.Side,
		PositionEffect: input.PositionEffect,
		Type:           input.Type,
		Qty:            input.Qty,
		FilledQty:      "0",
		AvgFillPrice:   "0",
		Status:         "RESTING",
	}, nil
}

func (f *fakeTradingMutator) CancelOrder(_ context.Context, input orderdomain.CancelOrderInput) error {
	f.cancelInput = input
	return nil
}

func TestTradingHandler_CreateOrder(t *testing.T) {
	mutator := &fakeTradingMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		NewTradingHandler(fakeTradingReader{}, mutator),
		nil,
		nil,
	)

	body := []byte(`{"client_order_id":"cli_1","symbol":"BTC-PERP","side":"BUY","position_effect":"OPEN","type":"LIMIT","qty":"1","price":"100","time_in_force":"GTC","max_slippage_bps":50}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem_1")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.createInput.UserID != 7 || mutator.createInput.IdempotencyKey != "idem_1" || mutator.createInput.Symbol != "BTC-PERP" {
		t.Fatalf("unexpected create input: %+v", mutator.createInput)
	}
}

func TestTradingHandler_CancelOrder(t *testing.T) {
	mutator := &fakeTradingMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		NewTradingHandler(fakeTradingReader{}, mutator),
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/ord_1/cancel", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Idempotency-Key", "idem_cancel")
	req.Header.Set("X-Trace-Id", "trace_cancel")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.cancelInput.UserID != 7 || mutator.cancelInput.OrderID != "ord_1" {
		t.Fatalf("unexpected cancel input: %+v", mutator.cancelInput)
	}
}
