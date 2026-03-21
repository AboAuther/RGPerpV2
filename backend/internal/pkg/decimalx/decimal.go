package decimalx

import (
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type Decimal struct {
	value decimal.Decimal
}

func NewFromString(raw string) (Decimal, error) {
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return Decimal{}, fmt.Errorf("%w: invalid decimal", errorsx.ErrInvalidArgument)
	}
	return Decimal{value: d}, nil
}

func MustFromString(raw string) Decimal {
	d, err := NewFromString(raw)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Decimal) String() string {
	return d.value.String()
}

func (d Decimal) Add(other Decimal) Decimal {
	return Decimal{value: d.value.Add(other.value)}
}

func (d Decimal) Sub(other Decimal) Decimal {
	return Decimal{value: d.value.Sub(other.value)}
}

func (d Decimal) Neg() Decimal {
	return Decimal{value: d.value.Neg()}
}

func (d Decimal) IsZero() bool {
	return d.value.IsZero()
}

type LedgerDecimalFactory struct{}

func (LedgerDecimalFactory) FromString(raw string) (ledger.Decimal, error) {
	d, err := NewFromString(raw)
	if err != nil {
		return nil, err
	}
	return ledgerDecimalAdapter{decimal: d}, nil
}

type ledgerDecimalAdapter struct {
	decimal Decimal
}

func (d ledgerDecimalAdapter) IsZero() bool {
	return d.decimal.IsZero()
}

func (d ledgerDecimalAdapter) Add(other ledger.Decimal) ledger.Decimal {
	return ledgerDecimalAdapter{decimal: d.decimal.Add(other.(ledgerDecimalAdapter).decimal)}
}

func (d ledgerDecimalAdapter) String() string {
	return d.decimal.String()
}
