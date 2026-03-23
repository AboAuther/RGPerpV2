package runtimeconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	active, err := s.repo.LoadActiveConfigValues(ctx, config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	snapshot, err := config.ApplyDynamicConfigValues(s.base, active)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	history, err := s.repo.ListConfigHistory(ctx, config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal, limit)
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
	if operatorID == "" || strings.TrimSpace(input.Reason) == "" || len(input.Values) == 0 {
		return readmodel.RuntimeConfigView{}, fmt.Errorf("%w: invalid runtime config patch", errorsx.ErrInvalidArgument)
	}

	active, err := s.repo.LoadActiveConfigValues(ctx, config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}
	beforeSnapshot, err := config.ApplyDynamicConfigValues(s.base, active)
	if err != nil {
		return readmodel.RuntimeConfigView{}, err
	}

	activeMap := make(map[string]json.RawMessage, len(active))
	for _, item := range active {
		activeMap[item.Key] = append(json.RawMessage(nil), item.ValueJSON...)
	}

	changes := make([]ConfigChange, 0, len(input.Values))
	for key, raw := range input.Values {
		candidate := append(json.RawMessage(nil), raw...)
		if current, ok := activeMap[key]; ok && bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(candidate)) {
			continue
		}
		activeMap[key] = candidate
		now := s.clock.Now().UTC()
		changes = append(changes, ConfigChange{
			Key:         key,
			ValueJSON:   candidate,
			CreatedBy:   operatorID,
			ApprovedBy:  operatorID,
			Reason:      strings.TrimSpace(input.Reason),
			EffectiveAt: now,
			CreatedAt:   now,
		})
	}
	if len(changes) == 0 {
		return readmodel.RuntimeConfigView{}, fmt.Errorf("%w: no runtime config change detected", errorsx.ErrInvalidArgument)
	}

	nextValues := make([]config.DynamicConfigValue, 0, len(activeMap))
	for key, raw := range activeMap {
		nextValues = append(nextValues, config.DynamicConfigValue{
			Key:        key,
			ScopeType:  config.ConfigScopeTypeGlobal,
			ScopeValue: config.ConfigScopeValueGlobal,
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
		return s.repo.ApplyConfigPatch(txCtx, ApplyPatchInput{
			ScopeType:  config.ConfigScopeTypeGlobal,
			ScopeValue: config.ConfigScopeValueGlobal,
			Changes:    changes,
			Audit: AuditLog{
				AuditID:      s.idgen.NewID("audit"),
				ActorType:    "ADMIN",
				ActorID:      operatorID,
				Action:       "runtime_config.update",
				ResourceType: resourceTypeRuntimeConfig,
				ResourceID:   resourceIDGlobal,
				BeforeJSON:   beforeJSON,
				AfterJSON:    afterJSON,
				TraceID:      strings.TrimSpace(input.TraceID),
				CreatedAt:    s.clock.Now().UTC(),
			},
		})
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
		RiskGlobalBufferRatio:             snapshot.Risk.GlobalBufferRatio,
		RiskMarkPriceStaleSec:             snapshot.Risk.MarkPriceStaleSec,
		RiskForceReduceOnlyOnStalePrice:   snapshot.Risk.ForceReduceOnlyOnStalePrice,
		RiskLiquidationPenaltyRate:        snapshot.Risk.LiquidationPenaltyRate,
		RiskLiquidationExtraSlippageBps:   snapshot.Risk.LiquidationExtraSlippageBps,
		RiskMaxOpenOrdersPerUserPerSymbol: snapshot.Risk.MaxOpenOrdersPerUserPerSymbol,
		RiskNetExposureHardLimit:          snapshot.Risk.NetExposureHardLimit,
		RiskMaxExposureSlippageBps:        snapshot.Risk.MaxExposureSlippageBps,
		HedgeEnabled:                      snapshot.Hedge.Enabled,
		HedgeSoftThresholdRatio:           snapshot.Hedge.SoftThresholdRatio,
		HedgeHardThresholdRatio:           snapshot.Hedge.HardThresholdRatio,
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
