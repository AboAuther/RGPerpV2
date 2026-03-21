package clockx

import "testing"

func TestRealClock_Now(t *testing.T) {
	clock := RealClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Fatal("expected non-zero time")
	}
	if now.Location().String() != "UTC" {
		t.Fatalf("expected UTC location, got %s", now.Location())
	}
}
