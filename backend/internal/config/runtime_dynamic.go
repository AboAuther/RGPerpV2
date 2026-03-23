package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	ConfigScopeTypeGlobal  = "global"
	ConfigScopeValueGlobal = "global"
)

type DynamicConfigValue struct {
	Key         string
	ScopeType   string
	ScopeValue  string
	Version     int64
	ValueJSON   json.RawMessage
	Status      string
	CreatedBy   string
	ApprovedBy  *string
	Reason      string
	EffectiveAt time.Time
	CreatedAt   time.Time
}

type DynamicConfigLoader interface {
	LoadActiveConfigValues(ctx context.Context, scopeType string, scopeValue string) ([]DynamicConfigValue, error)
}

type RuntimeConfigStore struct {
	base    RuntimeConfigSnapshot
	loader  DynamicConfigLoader
	current atomic.Pointer[RuntimeConfigSnapshot]
}

func NewRuntimeConfigStore(base RuntimeConfigSnapshot, loader DynamicConfigLoader) (*RuntimeConfigStore, error) {
	if err := base.Validate(); err != nil {
		return nil, err
	}
	store := &RuntimeConfigStore{base: base, loader: loader}
	initial := base
	store.current.Store(&initial)
	return store, nil
}

func (s *RuntimeConfigStore) Current() RuntimeConfigSnapshot {
	current := s.current.Load()
	if current == nil {
		return s.base
	}
	return *current
}

func (s *RuntimeConfigStore) Refresh(ctx context.Context) (RuntimeConfigSnapshot, error) {
	if s.loader == nil {
		return s.Current(), nil
	}
	values, err := s.loader.LoadActiveConfigValues(ctx, ConfigScopeTypeGlobal, ConfigScopeValueGlobal)
	if err != nil {
		return RuntimeConfigSnapshot{}, err
	}
	next, err := ApplyDynamicConfigValues(s.base, values)
	if err != nil {
		return RuntimeConfigSnapshot{}, err
	}
	s.current.Store(&next)
	return next, nil
}

func (s *RuntimeConfigStore) StartPolling(ctx context.Context, interval time.Duration, onError func(error)) {
	if s.loader == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.Refresh(ctx); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
}

func ApplyDynamicConfigValues(base RuntimeConfigSnapshot, values []DynamicConfigValue) (RuntimeConfigSnapshot, error) {
	next := base
	for _, item := range values {
		if item.ScopeType != ConfigScopeTypeGlobal || item.ScopeValue != ConfigScopeValueGlobal {
			continue
		}
		if err := applyDynamicValue(&next, item.Key, item.ValueJSON); err != nil {
			return RuntimeConfigSnapshot{}, err
		}
	}
	if err := next.Validate(); err != nil {
		return RuntimeConfigSnapshot{}, err
	}
	return next, nil
}

func applyDynamicValue(snapshot *RuntimeConfigSnapshot, key string, raw json.RawMessage) error {
	switch key {
	case "system.mode":
		return decodeDynamicJSON(raw, &snapshot.Global.SystemMode)
	case "system.read_only":
		return decodeDynamicJSON(raw, &snapshot.Global.ReadOnly)
	case "system.reduce_only":
		return decodeDynamicJSON(raw, &snapshot.Global.ReduceOnly)
	case "system.trace_header_required":
		return decodeDynamicJSON(raw, &snapshot.Global.TraceHeaderRequired)
	case "risk.global_buffer_ratio":
		return decodeDynamicJSON(raw, &snapshot.Risk.GlobalBufferRatio)
	case "risk.mark_price_stale_sec":
		return decodeDynamicJSON(raw, &snapshot.Risk.MarkPriceStaleSec)
	case "risk.force_reduce_only_on_stale_price":
		return decodeDynamicJSON(raw, &snapshot.Risk.ForceReduceOnlyOnStalePrice)
	case "risk.liquidation_penalty_rate":
		return decodeDynamicJSON(raw, &snapshot.Risk.LiquidationPenaltyRate)
	case "risk.liquidation_extra_slippage_bps":
		return decodeDynamicJSON(raw, &snapshot.Risk.LiquidationExtraSlippageBps)
	case "risk.max_open_orders_per_user_per_symbol":
		return decodeDynamicJSON(raw, &snapshot.Risk.MaxOpenOrdersPerUserPerSymbol)
	case "risk.net_exposure_hard_limit":
		return decodeDynamicJSON(raw, &snapshot.Risk.NetExposureHardLimit)
	case "risk.max_exposure_slippage_bps":
		return decodeDynamicJSON(raw, &snapshot.Risk.MaxExposureSlippageBps)
	case "hedge.enabled":
		return decodeDynamicJSON(raw, &snapshot.Hedge.Enabled)
	case "hedge.soft_threshold_ratio":
		return decodeDynamicJSON(raw, &snapshot.Hedge.SoftThresholdRatio)
	case "hedge.hard_threshold_ratio":
		return decodeDynamicJSON(raw, &snapshot.Hedge.HardThresholdRatio)
	default:
		return fmt.Errorf("%w: unsupported dynamic config key %s", errorsx.ErrInvalidArgument, key)
	}
}

func decodeDynamicJSON[T any](raw json.RawMessage, target *T) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: dynamic config value is required", errorsx.ErrInvalidArgument)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: invalid dynamic config value", errorsx.ErrInvalidArgument)
	}
	return nil
}
