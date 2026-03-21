package chain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type DeterministicDepositAddressAllocator struct{}

func NewDeterministicDepositAddressAllocator() *DeterministicDepositAddressAllocator {
	return &DeterministicDepositAddressAllocator{}
}

func (a *DeterministicDepositAddressAllocator) Allocate(_ context.Context, userID uint64, chainID int64, asset string) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("rgperp:%d:%s:%d", userID, asset, chainID)))
	return "0x" + hex.EncodeToString(sum[len(sum)-20:]), nil
}
