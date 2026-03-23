package funding

import (
	"fmt"
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ServiceConfig struct {
	SettlementIntervalSec int
	CapRatePerHour        string
	MinValidSourceCount   int
	DefaultCryptoModel    string
}

type Service struct {
	cfg ServiceConfig
}

func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.SettlementIntervalSec <= 0 || cfg.CapRatePerHour == "" || cfg.MinValidSourceCount <= 0 {
		return nil, fmt.Errorf("%w: invalid funding config", errorsx.ErrInvalidArgument)
	}
	if strings.TrimSpace(cfg.DefaultCryptoModel) == "" {
		cfg.DefaultCryptoModel = ModelExternalAvg
	}
	return &Service{cfg: cfg}, nil
}

func (s *Service) NormalizeRate(rate string, sourceIntervalSeconds int) (string, error) {
	if strings.TrimSpace(rate) == "" || sourceIntervalSeconds <= 0 {
		return "", fmt.Errorf("%w: invalid funding source rate", errorsx.ErrInvalidArgument)
	}
	sourceRate, err := decimalx.NewFromString(rate)
	if err != nil {
		return "", err
	}
	settlementInterval := decimalx.MustFromString(fmt.Sprintf("%d", s.cfg.SettlementIntervalSec))
	sourceInterval := decimalx.MustFromString(fmt.Sprintf("%d", sourceIntervalSeconds))
	return sourceRate.Mul(settlementInterval).Div(sourceInterval).String(), nil
}

func (s *Service) AggregateRate(sources []SourceRate) (string, []string, error) {
	validRates := make([]decimalx.Decimal, 0, len(sources))
	sourceNames := make([]string, 0, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.SourceName) == "" || strings.TrimSpace(source.Rate) == "" || source.IntervalSeconds <= 0 {
			continue
		}
		normalized, err := s.NormalizeRate(source.Rate, source.IntervalSeconds)
		if err != nil {
			return "", nil, err
		}
		validRates = append(validRates, decimalx.MustFromString(normalized))
		sourceNames = append(sourceNames, source.SourceName)
	}
	if len(validRates) < s.cfg.MinValidSourceCount {
		return "", nil, fmt.Errorf("%w: insufficient valid funding sources", errorsx.ErrConflict)
	}

	sum := decimalx.MustFromString("0")
	for _, rate := range validRates {
		sum = sum.Add(rate)
	}
	avg := sum.Div(decimalx.MustFromString(fmt.Sprintf("%d", len(validRates))))
	cap := settlementCap(s.cfg.CapRatePerHour, s.cfg.SettlementIntervalSec)
	if avg.GreaterThan(cap) {
		avg = cap
	} else if avg.LessThan(cap.Neg()) {
		avg = cap.Neg()
	}
	return avg.String(), sourceNames, nil
}

func (s *Service) BuildBatch(input BuildBatchInput) (BatchPlan, error) {
	if strings.TrimSpace(input.FundingBatchID) == "" || input.SymbolID == 0 || strings.TrimSpace(input.Symbol) == "" {
		return BatchPlan{}, fmt.Errorf("%w: invalid funding batch identity", errorsx.ErrInvalidArgument)
	}
	if input.TimeWindowStart.IsZero() || input.TimeWindowEnd.IsZero() || !input.TimeWindowEnd.After(input.TimeWindowStart) {
		return BatchPlan{}, fmt.Errorf("%w: invalid funding time window", errorsx.ErrInvalidArgument)
	}
	settlementPrice, err := decimalx.NewFromString(input.SettlementPrice)
	if err != nil {
		return BatchPlan{}, err
	}
	if !settlementPrice.GreaterThan(decimalx.MustFromString("0")) {
		return BatchPlan{}, fmt.Errorf("%w: settlement price must be positive", errorsx.ErrInvalidArgument)
	}

	aggregatedRate, _, err := s.AggregateRate(input.Sources)
	if err != nil {
		return BatchPlan{}, err
	}
	rate := decimalx.MustFromString(aggregatedRate)
	items := make([]BatchItem, 0, len(input.Positions))
	createdAt := input.CreatedAt.UTC()
	for _, position := range input.Positions {
		if strings.TrimSpace(position.PositionID) == "" || position.UserID == 0 || strings.TrimSpace(position.Side) == "" || strings.TrimSpace(position.Qty) == "" {
			return BatchPlan{}, fmt.Errorf("%w: invalid funding position snapshot", errorsx.ErrInvalidArgument)
		}

		qty := decimalx.MustFromString(position.Qty).Abs()
		if qty.IsZero() {
			continue
		}
		multiplier := decimalx.MustFromString(position.ContractMultiplier)
		notional := qty.Mul(settlementPrice).Mul(multiplier)
		feeAbs := notional.Mul(rate.Abs())
		signedFee, err := signedFundingFee(position.Side, rate, feeAbs)
		if err != nil {
			return BatchPlan{}, err
		}
		if signedFee.IsZero() {
			continue
		}
		items = append(items, BatchItem{
			FundingBatchID: input.FundingBatchID,
			PositionID:     position.PositionID,
			UserID:         position.UserID,
			FundingFee:     signedFee.String(),
			Status:         ItemStatusPending,
			CreatedAt:      createdAt,
		})
	}

	return BatchPlan{
		Batch: Batch{
			ID:              input.FundingBatchID,
			SymbolID:        input.SymbolID,
			Symbol:          input.Symbol,
			TimeWindowStart: input.TimeWindowStart.UTC(),
			TimeWindowEnd:   input.TimeWindowEnd.UTC(),
			NormalizedRate:  rate.String(),
			SettlementPrice: settlementPrice.String(),
			Status:          BatchStatusReady,
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
		},
		Items: items,
	}, nil
}

func settlementCap(capPerHour string, settlementIntervalSec int) decimalx.Decimal {
	hourlyCap := decimalx.MustFromString(capPerHour)
	interval := decimalx.MustFromString(fmt.Sprintf("%d", settlementIntervalSec))
	return hourlyCap.Mul(interval).Div(decimalx.MustFromString("3600"))
}

func signedFundingFee(side string, normalizedRate decimalx.Decimal, feeAbs decimalx.Decimal) (decimalx.Decimal, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case PositionSideLong:
		if normalizedRate.GreaterThan(decimalx.MustFromString("0")) {
			return feeAbs.Neg(), nil
		}
		return feeAbs, nil
	case PositionSideShort:
		if normalizedRate.GreaterThan(decimalx.MustFromString("0")) {
			return feeAbs, nil
		}
		return feeAbs.Neg(), nil
	default:
		return decimalx.Decimal{}, fmt.Errorf("%w: unsupported position side", errorsx.ErrInvalidArgument)
	}
}
