package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
)

func TestSignatureVerifier_VerifyLogin(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	message := authdomain.BuildLoginMessage("localhost", 8453, "challenge_1")
	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)))
	sig, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	verifier := NewSignatureVerifier()
	if err := verifier.VerifyLogin(context.Background(), address.Hex(), 8453, "localhost", "challenge_1", "0x"+hex.EncodeToString(sig)); err != nil {
		t.Fatalf("verify: %v", err)
	}
}
