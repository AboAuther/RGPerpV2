package posttrade

import (
	"context"
	"testing"
	"time"

	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
)

type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time {
	return f.now
}

type fakeIDGenerator struct {
	values []string
	index  int
}

func (f *fakeIDGenerator) NewID(prefix string) string {
	value := prefix + "_default"
	if f.index < len(f.values) {
		value = f.values[f.index]
	}
	f.index++
	return value
}

type publishedEvent struct {
	eventType string
	payload   map[string]any
}

type fakeOutboxPublisher struct {
	published []publishedEvent
	err       error
}

func (f *fakeOutboxPublisher) Publish(_ context.Context, event riskdomain.DomainEvent) error {
	if f.err != nil {
		return f.err
	}
	payload, _ := event.Payload.(map[string]any)
	f.published = append(f.published, publishedEvent{
		eventType: event.EventType,
		payload:   payload,
	})
	return nil
}

func TestProcessorRecalculateAfterTradeQueuesRiskRecalculation(t *testing.T) {
	publisher := &fakeOutboxPublisher{}
	processor := NewProcessor(fakeClock{now: time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)}, &fakeIDGenerator{values: []string{"rrq_001", "evt_001"}}, publisher)

	err := processor.RecalculateAfterTrade(context.Background(), 7, "trace_trade")
	if err != nil {
		t.Fatalf("RecalculateAfterTrade error = %v", err)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("expected one published event, got %d", len(publisher.published))
	}
	event := publisher.published[0]
	if event.eventType != recalculateRequestedEventType {
		t.Fatalf("expected event type %s, got %s", recalculateRequestedEventType, event.eventType)
	}
	if event.payload["user_id"] != uint64(7) {
		t.Fatalf("expected user_id=7, got %+v", event.payload["user_id"])
	}
	if event.payload["triggered_by"] != "trade_fill" {
		t.Fatalf("expected triggered_by trade_fill, got %+v", event.payload["triggered_by"])
	}
	if event.payload["trace_id"] != "trace_trade" {
		t.Fatalf("expected trace_id trace_trade, got %+v", event.payload["trace_id"])
	}
}

func TestProcessorRecalculateAccountQueuesAdminRequest(t *testing.T) {
	publisher := &fakeOutboxPublisher{}
	processor := NewProcessor(fakeClock{now: time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)}, &fakeIDGenerator{values: []string{"rrq_002", "evt_002"}}, publisher)

	result, err := processor.RecalculateAccount(context.Background(), 9, "0xadmin")
	if err != nil {
		t.Fatalf("RecalculateAccount error = %v", err)
	}
	if result.UserID != 9 || result.TriggeredBy != "admin" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.RecalculationRequestID == nil || *result.RecalculationRequestID != "rrq_002" {
		t.Fatalf("expected request id rrq_002, got %+v", result.RecalculationRequestID)
	}
	if result.RecalculationStatus == nil || *result.RecalculationStatus != "QUEUED" {
		t.Fatalf("expected status QUEUED, got %+v", result.RecalculationStatus)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("expected one published event, got %d", len(publisher.published))
	}
}
