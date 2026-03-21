package ledger

import (
	"context"
	"errors"
	"testing"
)

type fakeDecimal struct{ value int64 }

func (d fakeDecimal) IsZero() bool { return d.value == 0 }
func (d fakeDecimal) Add(other Decimal) Decimal {
	return fakeDecimal{value: d.value + other.(fakeDecimal).value}
}
func (d fakeDecimal) String() string { return "" }

type fakeDecimalFactory struct{}

func (fakeDecimalFactory) FromString(raw string) (Decimal, error) {
	switch raw {
	case "100":
		return fakeDecimal{value: 100}, nil
	case "-100":
		return fakeDecimal{value: -100}, nil
	case "1":
		return fakeDecimal{value: 1}, nil
	case "-1":
		return fakeDecimal{value: -1}, nil
	default:
		return nil, errors.New("bad decimal")
	}
}

type stubRepo struct {
	req PostingRequest
	err error
}

func (s *stubRepo) CreatePosting(_ context.Context, req PostingRequest) error {
	s.req = req
	return s.err
}

func TestPost_BalancedPosting(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, fakeDecimalFactory{})

	err := svc.Post(context.Background(), PostingRequest{
		LedgerTx: LedgerTx{ID: "ldg_1", EventID: "evt_1", IdempotencyKey: "idem_1"},
		Entries: []LedgerEntry{
			{AccountID: 1, Asset: "USDC", Amount: "100"},
			{AccountID: 2, Asset: "USDC", Amount: "-100"},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if repo.req.LedgerTx.ID != "ldg_1" {
		t.Fatalf("expected repository call")
	}
}

func TestPost_RejectsUnbalancedPosting(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, fakeDecimalFactory{})

	err := svc.Post(context.Background(), PostingRequest{
		LedgerTx: LedgerTx{ID: "ldg_1", EventID: "evt_1", IdempotencyKey: "idem_1"},
		Entries: []LedgerEntry{
			{AccountID: 1, Asset: "USDC", Amount: "100"},
			{AccountID: 2, Asset: "USDC", Amount: "-1"},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
