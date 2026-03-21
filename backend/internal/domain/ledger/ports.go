package ledger

import "context"

type Decimal interface {
	IsZero() bool
	Add(other Decimal) Decimal
	String() string
}

type DecimalFactory interface {
	FromString(raw string) (Decimal, error)
}

type Repository interface {
	CreatePosting(ctx context.Context, posting PostingRequest) error
}
