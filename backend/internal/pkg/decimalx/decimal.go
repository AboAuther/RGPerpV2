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

func (d Decimal) Mul(other Decimal) Decimal {
	return Decimal{value: d.value.Mul(other.value)}
}

func (d Decimal) Div(other Decimal) Decimal {
	return Decimal{value: d.value.Div(other.value)}
}

func (d Decimal) Mod(other Decimal) Decimal {
	return Decimal{value: d.value.Mod(other.value)}
}

func (d Decimal) Neg() Decimal {
	return Decimal{value: d.value.Neg()}
}

func (d Decimal) Abs() Decimal {
	return Decimal{value: d.value.Abs()}
}

func (d Decimal) IsZero() bool {
	return d.value.IsZero()
}

func (d Decimal) LessThan(other Decimal) bool {
	return d.value.LessThan(other.value)
}

func (d Decimal) GreaterThan(other Decimal) bool {
	return d.value.GreaterThan(other.value)
}

func (d Decimal) GreaterThanOrEqual(other Decimal) bool {
	return d.value.GreaterThanOrEqual(other.value)
}

func (d Decimal) LessThanOrEqual(other Decimal) bool {
	return d.value.LessThanOrEqual(other.value)
}

func (d Decimal) Equal(other Decimal) bool {
	return d.value.Equal(other.value)
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
