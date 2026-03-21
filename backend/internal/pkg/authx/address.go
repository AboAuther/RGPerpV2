package authx

import (
	"fmt"
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

// NormalizeEVMAddress normalizes and validates a user-provided EVM address.
func NormalizeEVMAddress(address string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(address))
	if len(normalized) != 42 || !strings.HasPrefix(normalized, "0x") {
		return "", fmt.Errorf("%w: invalid evm address", errorsx.ErrInvalidArgument)
	}
	for _, c := range normalized[2:] {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return "", fmt.Errorf("%w: invalid evm address", errorsx.ErrInvalidArgument)
		}
	}
	return normalized, nil
}
