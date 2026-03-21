package idgen

import "testing"

func TestTimeBasedGenerator_NewID(t *testing.T) {
	var gen TimeBasedGenerator
	id1 := gen.NewID("test")
	id2 := gen.NewID("test")
	if id1 == id2 {
		t.Fatal("expected unique ids")
	}
	if len(id1) == 0 || id1[:5] != "test_" {
		t.Fatalf("unexpected id format: %s", id1)
	}
}
