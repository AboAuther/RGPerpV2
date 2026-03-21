package authx

import "testing"

func TestNormalizeEVMAddress_Success(t *testing.T) {
	got, err := NormalizeEVMAddress(" 0xABCDEFabcdefABCDEFabcdefABCDEFabcdefabcd ")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got != "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("unexpected normalized address: %s", got)
	}
}

func TestNormalizeEVMAddress_Invalid(t *testing.T) {
	cases := []string{
		"",
		"0x1234",
		"0xzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		"1234567890123456789012345678901234567890",
	}
	for _, tc := range cases {
		if _, err := NormalizeEVMAddress(tc); err == nil {
			t.Fatalf("expected error for %q", tc)
		}
	}
}
