package runtimeconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	resourceTypeRuntimeConfig = "runtime_config"
	resourceIDGlobal          = "global"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Refresher interface {
	Current() config.RuntimeConfigSnapshot
	Refresh(ctx context.Context) (config.RuntimeConfigSnapshot, error)
}

type Repository interface {
	LoadActiveConfigValues(ctx context.Context, scopeType string, scopeValue string) ([]config.DynamicConfigValue, error)
	ListConfigHistory(ctx context.Context, scopeType string, scopeValue string, limit int) ([]config.DynamicConfigValue, error)
	ApplyConfigPatch(ctx context.Context, input ApplyPatchInput) error
}

type ApplyPatchInput struct {
	ScopeType  string
	ScopeValue string
	Changes    []ConfigChange
	Audit      AuditLog
}

type ConfigChange struct {
	Key         string
	ValueJSON   json.RawMessage
	CreatedBy   string
	ApprovedBy  string
	Reason      string
	EffectiveAt time.Time
	CreatedAt   time.Time
}

type AuditLog struct {
	AuditID      string
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	BeforeJSON   *string
	AfterJSON    *string
	TraceID      string
	CreatedAt    time.Time
}

type UpdateInput struct {
	OperatorID string
	TraceID    string
	Reason     string
	Values     map[string]json.RawMessage
	PairValues map[string]map[string]json.RawMessage
}

type Service struct {
	base      config.RuntimeConfigSnapshot
	clock     Clock
	idgen     IDGenerator
	txm       TxManager
	repo      Repository
	refresher Refresher
}

func NewService(base config.RuntimeConfigSnapshot, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, refresher Refresher) (*Service, error) {
	if err := base.Validate(); err != nil {
		return nil, err
	}
	if clock == nil || idgen == nil || txm == nil || repo == nil {
		return nil, fmt.Errorf("%w: missing runtime config dependency", errorsx.ErrInvalidArgument)
	}
	return &Service{
		base:      base,
		clock:     clock,
		idgen:     idgen,
		txm:       txm,
		repo:      repo,
		refresher: refresher,
	}, nil
}

func (s *Service) GetRuntimeConfigView(ctx context.Context, limit int) (readmodel.RuntimeConfigView, error) {
	if limit <= 0 {
		limit = 20
	}
	active, err := s.loadAllActiveValues(ctx)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	snapshot, err := config.ApplyDynamicConfigValues(s.base, active)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	history, err := s.repo.ListConfigHistory(ctx, "", "", limit)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	return readmodel.RuntimeConfigView{
		Snapshot:    buildSnapshotView(snapshot),
		GeneratedAt: s.clock.Now().UTC().Format(time.RFC3339Nano),
		History:     buildHistoryItems(history),
	}, nil
}

func (s *Service) UpdateRuntimeConfig(ctx context.Context, input UpdateInput) (readmodel.RuntimeConfigView, error) {
	operatorID := strings.ToLower(strings.TrimSpace(input.OperatorID))
	if operatorID == "" || strings.TrimSpace(input.Reason) == "" || (len(input.Values) == 0 && len(input.PairValues) == 0) {
		return readmodel.RuntimeConfigView{}, fmt.Errorf("%w: invalid runtime config patch", errorsx.ErrInvalidArgument)
	}

	active, err := s.loadAllActiveValues(ctx)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	beforeSnapshot, err := config.ApplyDynamicConfigValues(s.base, active)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}

	activeMap := make(map[string]json.RawMessage, len(active))
	for _, item := range active {
		activeMap[scopeConfigMapKey(item.ScopeType, item.ScopeValue, item.Key)] = append(json.RawMessage(nil), item.ValueJSON...)
	}

	groupedChanges := make(map[string][]ConfigChange)
	scopeOrder := make([]string, 0)
	seenScopes := make(map[string]struct{})
	appendScopeChange := func(scopeType string, scopeValue string, key string, raw json.RawMessage) {
		scopeKey := scopeConfigMapKey(scopeType, scopeValue, key)
		activeMap[scopeKey] = raw
		groupKey := scopeGroupKey(scopeType, scopeValue)
		if _, ok := seenScopes[groupKey]; !ok {
			scopeOrder = append(scopeOrder, groupKey)
			seenScopes[groupKey] = struct{}{}
		}
		now := s.clock.Now().UTC()
		groupedChanges[groupKey] = append(groupedChanges[groupKey], ConfigChange{
			Key:         key,
			ValueJSON:   raw,
			CreatedBy:   operatorID,
			ApprovedBy:  operatorID,
			Reason:      strings.TrimSpace(input.Reason),
			EffectiveAt: now,
			CreatedAt:   now,
		})
	}

	for key, raw := range input.Values {
		candidate := append(json.RawMessage(nil), raw...)
		scopeKey := scopeConfigMapKey(config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal, key)
		if current, ok := activeMap[scopeKey]; ok && bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(candidate)) {
			continue
		}
		appendScopeChange(config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal, key, candidate)
	}
	for pair, values := range input.PairValues {
		scopeValue := normalizePairKey(pair)
		for key, raw := range values {
			candidate := append(json.RawMessage(nil), raw...)
			scopeKey := scopeConfigMapKey(config.ConfigScopeTypePair, scopeValue, key)
			if current, ok := activeMap[scopeKey]; ok && bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(candidate)) {
				continue
			}
			appendScopeChange(config.ConfigScopeTypePair, scopeValue, key, candidate)
		}
	}
	if len(groupedChanges) == 0 {
		return readmodel.RuntimeConfigView{}, fmt.Errorf("%w: no runtime config change detected", errorsx.ErrInvalidArgument)
	}

	nextValues := make([]config.DynamicConfigValue, 0, len(activeMap))
	for scopedKey, raw := range activeMap {
		scopeType, scopeValue, key := splitScopeConfigMapKey(scopedKey)
		nextValues = append(nextValues, config.DynamicConfigValue{
			Key:        key,
			ScopeType:  scopeType,
			ScopeValue: scopeValue,
			ValueJSON:  raw,
		})
	}
	afterSnapshot, err := config.ApplyDynamicConfigValues(s.base, nextValues)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	beforeJSON, err := marshalString(buildSnapshotView(beforeSnapshot))
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	afterJSON, err := marshalString(buildSnapshotView(afterSnapshot))
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}

	err = s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		sort.Strings(scopeOrder)
		for _, groupKey := range scopeOrder {
			scopeType, scopeValue := splitScopeGroupKey(groupKey)
			resourceID := scopeValue
			if scopeType == config.ConfigScopeTypeGlobal {
				resourceID = resourceIDGlobal
			}
			if err := s.repo.ApplyConfigPatch(txCtx, ApplyPatchInput{
				ScopeType:  scopeType,
				ScopeValue: scopeValue,
				Changes:    groupedChanges[groupKey],
				Audit: AuditLog{
					AuditID:      s.idgen.NewID("audit"),
					ActorType:    "ADMIN",
					ActorID:      operatorID,
					Action:       "runtime_config.update",
					ResourceType: resourceTypeRuntimeConfig,
					ResourceID:   resourceID,
					BeforeJSON:   beforeJSON,
					AfterJSON:    afterJSON,
					TraceID:      strings.TrimSpace(input.TraceID),
					CreatedAt:    s.clock.Now().UTC(),
				},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}

	if s.refresher != nil {
		if _, err := s.refresher.Refresh(ctx); err != nil {
			return readmodel.RuntimeConfigView{}, err
		}
	}
	return s.GetRuntimeConfigView(ctx, 20)
}

func buildSnapshotView(snapshot config.RuntimeConfigSnapshot) readmodel.RuntimeConfigSnapshotView {
	return readmodel.RuntimeConfigSnapshotView{
		SystemMode:                        snapshot.Global.SystemMode,
		ReadOnly:                          snapshot.Global.ReadOnly,
		ReduceOnly:                        snapshot.Global.ReduceOnly,
		TraceHeaderRequired:               snapshot.Global.TraceHeaderRequired,
		MarketTakerFeeRate:                snapshot.Market.TakerFeeRate,
		MarketMakerFeeRate:                snapshot.Market.MakerFeeRate,
		MarketDefaultMaxSlippageBps:       snapshot.Market.DefaultMaxSlippageBps,
		RiskGlobalBufferRatio:             snapshot.Risk.GlobalBufferRatio,
		RiskMarkPriceStaleSec:             snapshot.Risk.MarkPriceStaleSec,
		RiskForceReduceOnlyOnStalePrice:   snapshot.Risk.ForceReduceOnlyOnStalePrice,
		RiskLiquidationPenaltyRate:        snapshot.Risk.LiquidationPenaltyRate,
		RiskMaintenanceMarginUpliftRatio:  snapshot.Risk.MaintenanceMarginUpliftRatio,
		RiskLiquidationExtraSlippageBps:   snapshot.Risk.LiquidationExtraSlippageBps,
		RiskMaxOpenOrdersPerUserPerSymbol: snapshot.Risk.MaxOpenOrdersPerUserPerSymbol,
		RiskNetExposureHardLimit:          snapshot.Risk.NetExposureHardLimit,
		RiskMaxExposureSlippageBps:        snapshot.Risk.MaxExposureSlippageBps,
		FundingIntervalSec:                snapshot.Funding.IntervalSec,
		FundingSourcePollIntervalSec:      snapshot.Funding.SourcePollIntervalSec,
		FundingCapRatePerHour:             snapshot.Funding.CapRatePerHour,
		FundingMinValidSourceCount:        snapshot.Funding.MinValidSourceCount,
		FundingDefaultModelCrypto:         snapshot.Funding.DefaultModelCrypto,
		HedgeEnabled:                      snapshot.Hedge.Enabled,
		HedgeSoftThresholdRatio:           snapshot.Hedge.SoftThresholdRatio,
		HedgeHardThresholdRatio:           snapshot.Hedge.HardThresholdRatio,
		PairOverrides:                     buildPairOverrideView(snapshot.Pairs),
	}
}

func buildHistoryItems(values []config.DynamicConfigValue) []readmodel.RuntimeConfigHistoryItem {
	items := make([]readmodel.RuntimeConfigHistoryItem, 0, len(values))
	for _, item := range values {
		var parsed any
		if err := json.Unmarshal(item.ValueJSON, &parsed); err != nil {
			parsed = string(item.ValueJSON)
		}
		items = append(items, readmodel.RuntimeConfigHistoryItem{
			ConfigKey:   item.Key,
			ScopeType:   item.ScopeType,
			ScopeValue:  item.ScopeValue,
			Version:     item.Version,
			Value:       parsed,
			Status:      item.Status,
			CreatedBy:   item.CreatedBy,
			ApprovedBy:  item.ApprovedBy,
			Reason:      item.Reason,
			EffectiveAt: item.EffectiveAt.UTC().Format(time.RFC3339Nano),
			CreatedAt:   item.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return items
}

func marshalString(value any) (*string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	return &text, nil
}

func (s *Service) loadAllActiveValues(ctx context.Context) ([]config.DynamicConfigValue, error) {
	globalValues, err := s.repo.LoadActiveConfigValues(ctx, config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal)
	if err != nil {
		return nil, err
	}
	pairValues, err := s.repo.LoadActiveConfigValues(ctx, config.ConfigScopeTypePair, "")
	if err != nil {
		return nil, err
	}
	return append(globalValues, pairValues...), nil
}

func buildPairOverrideView(pairs map[string]config.PairRuntimeConfig) map[string]readmodel.RuntimeConfigPairOverrideView {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]readmodel.RuntimeConfigPairOverrideView, len(pairs))
	for pair, cfg := range pairs {
		out[pair] = readmodel.RuntimeConfigPairOverrideView{
			MaxLeverage:                  cfg.MaxLeverage,
			SessionPolicy:                cfg.SessionPolicy,
			TakerFeeRate:                 cfg.TakerFeeRate,
			MakerFeeRate:                 cfg.MakerFeeRate,
			DefaultMaxSlippageBps:        cfg.DefaultMaxSlippageBps,
			LiquidationPenaltyRate:       cfg.LiquidationPenaltyRate,
			FundingIntervalSec:           cfg.FundingIntervalSec,
			MaintenanceMarginUpliftRatio: cfg.MaintenanceMarginUpliftRatio,
		}
	}
	return out
}

func scopeGroupKey(scopeType string, scopeValue string) string {
	return scopeType + "|" + scopeValue
}

func splitScopeGroupKey(value string) (string, string) {
	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func scopeConfigMapKey(scopeType string, scopeValue string, key string) string {
	return scopeType + "|" + scopeValue + "|" + key
}

func splitScopeConfigMapKey(value string) (string, string, string) {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 {
		return "", "", value
	}
	return parts[0], parts[1], parts[2]
}

func normalizePairKey(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
