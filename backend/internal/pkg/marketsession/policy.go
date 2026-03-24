package marketsession

import (
	"fmt"
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	PolicyAlwaysOpen      = "ALWAYS_OPEN"
	PolicyUSEquityRegular = "US_EQUITY_REGULAR"
	PolicyXAUUSD24x5      = "XAUUSD_24_5"
)

func NormalizePolicy(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return PolicyAlwaysOpen
	}
	return normalized
}

func ValidatePolicy(value string) error {
	switch NormalizePolicy(value) {
	case PolicyAlwaysOpen, PolicyUSEquityRegular, PolicyXAUUSD24x5:
		return nil
	default:
		return fmt.Errorf("%w: unsupported session policy %q", errorsx.ErrInvalidArgument, value)
	}
}

func AllowsOpenAt(policy string, ts time.Time) (bool, error) {
	switch NormalizePolicy(policy) {
	case PolicyAlwaysOpen:
		return true, nil
	case PolicyUSEquityRegular:
		return allowsUSEquityRegular(ts)
	case PolicyXAUUSD24x5:
		return allowsXAUUSD24x5(ts)
	default:
		return false, fmt.Errorf("%w: unsupported session policy %q", errorsx.ErrInvalidArgument, policy)
	}
}

func allowsUSEquityRegular(ts time.Time) (bool, error) {
	local, err := inNewYork(ts)
	if err != nil {
		return false, err
	}
	if local.Weekday() < time.Monday || local.Weekday() > time.Friday {
		return false, nil
	}
	minuteOfDay := local.Hour()*60 + local.Minute()
	return minuteOfDay >= 9*60+30 && minuteOfDay < 16*60, nil
}

func allowsXAUUSD24x5(ts time.Time) (bool, error) {
	local, err := inNewYork(ts)
	if err != nil {
		return false, err
	}
	minuteOfDay := local.Hour()*60 + local.Minute()
	switch local.Weekday() {
	case time.Saturday:
		return false, nil
	case time.Sunday:
		return minuteOfDay >= 18*60, nil
	case time.Friday:
		return minuteOfDay < 17*60, nil
	case time.Monday, time.Tuesday, time.Wednesday, time.Thursday:
		return minuteOfDay < 17*60 || minuteOfDay >= 18*60, nil
	default:
		return false, nil
	}
}

func inNewYork(ts time.Time) (time.Time, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, fmt.Errorf("load America/New_York: %w", err)
	}
	return ts.In(loc), nil
}
