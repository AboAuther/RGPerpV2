package decimalx

import "testing"

func TestDecimalOperations(t *testing.T) {
	a := MustFromString("100.5")
	b := MustFromString("0.5")

	if got := a.Sub(b).String(); got != "100" {
		t.Fatalf("unexpected sub result: %s", got)
	}
	if got := b.Neg().String(); got != "-0.5" {
		t.Fatalf("unexpected neg result: %s", got)
	}
	if got := a.Add(b).String(); got != "101" {
		t.Fatalf("unexpected add result: %s", got)
	}
}

func TestNewFromString_Invalid(t *testing.T) {
	if _, err := NewFromString("bad-number"); err == nil {
		t.Fatal("expected invalid decimal error")
	}
}
