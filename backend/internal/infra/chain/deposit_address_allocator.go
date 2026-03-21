package chain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
)

type DeterministicDepositAddressAllocator struct{}

func NewDeterministicDepositAddressAllocator() *DeterministicDepositAddressAllocator {
	return &DeterministicDepositAddressAllocator{}
}

func (a *DeterministicDepositAddressAllocator) Allocate(user authdomain.User, chainID int64, asset string) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("rgperp:%d:%s:%d", user.ID, asset, chainID)))
	return "0x" + hex.EncodeToString(sum[len(sum)-20:]), nil
}
