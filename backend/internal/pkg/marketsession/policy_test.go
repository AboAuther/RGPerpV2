package marketsession

import (
	"testing"
	"time"
)

func TestAllowsOpenAt_USEquityRegular(t *testing.T) {
	timestamp := mustParseRFC3339(t, "2026-03-24T14:00:00Z")
	open, err := AllowsOpenAt(PolicyUSEquityRegular, timestamp)
	if err != nil {
		t.Fatalf("AllowsOpenAt: %v", err)
	}
	if !open {
		t.Fatal("expected US equity session to be open")
	}

	closedAt := mustParseRFC3339(t, "2026-03-24T21:00:00Z")
	open, err = AllowsOpenAt(PolicyUSEquityRegular, closedAt)
	if err != nil {
		t.Fatalf("AllowsOpenAt closed: %v", err)
	}
	if open {
		t.Fatal("expected US equity session to be closed")
	}
}

func TestAllowsOpenAt_XAUUSD24x5(t *testing.T) {
	openAt := mustParseRFC3339(t, "2026-03-24T15:00:00Z")
	open, err := AllowsOpenAt(PolicyXAUUSD24x5, openAt)
	if err != nil {
		t.Fatalf("AllowsOpenAt open: %v", err)
	}
	if !open {
		t.Fatal("expected gold session to be open")
	}

	dailyBreak := mustParseRFC3339(t, "2026-03-24T21:30:00Z")
	open, err = AllowsOpenAt(PolicyXAUUSD24x5, dailyBreak)
	if err != nil {
		t.Fatalf("AllowsOpenAt break: %v", err)
	}
	if open {
		t.Fatal("expected gold session daily maintenance break")
	}

	weekend := mustParseRFC3339(t, "2026-03-28T15:00:00Z")
	open, err = AllowsOpenAt(PolicyXAUUSD24x5, weekend)
	if err != nil {
		t.Fatalf("AllowsOpenAt weekend: %v", err)
	}
	if open {
		t.Fatal("expected gold weekend session to be closed")
	}
}

func mustParseRFC3339(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return parsed
}
