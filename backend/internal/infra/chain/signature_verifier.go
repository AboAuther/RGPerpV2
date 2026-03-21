package chain

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type SignatureVerifier struct{}

func NewSignatureVerifier() *SignatureVerifier {
	return &SignatureVerifier{}
}

func (v *SignatureVerifier) VerifyLogin(_ context.Context, address string, chainID int64, domain string, nonce string, signature string) error {
	message := loginMessage(domain, chainID, nonce)
	sig := common.FromHex(signature)
	if len(sig) != 65 {
		return fmt.Errorf("%w: invalid signature length", errorsx.ErrUnauthorized)
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}

	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)))
	pubKey, err := crypto.SigToPub(hash.Bytes(), sig)
	if err != nil {
		return fmt.Errorf("%w: signature recovery failed", errorsx.ErrUnauthorized)
	}
	recovered := crypto.PubkeyToAddress(*pubKey)
	if recovered.Hex() != common.HexToAddress(address).Hex() {
		return fmt.Errorf("%w: signer mismatch", errorsx.ErrUnauthorized)
	}
	return nil
}

func loginMessage(domain string, chainID int64, nonce string) string {
	return fmt.Sprintf("RGPerp Login\nDomain: %s\nChain ID: %d\nNonce: %s", domain, chainID, nonce)
}
