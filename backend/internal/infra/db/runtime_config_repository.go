package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	runtimeconfigapp "github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	"gorm.io/gorm"
)

type RuntimeConfigRepository struct {
	db *gorm.DB
}

func NewRuntimeConfigRepository(db *gorm.DB) *RuntimeConfigRepository {
	return &RuntimeConfigRepository{db: db}
}

func (r *RuntimeConfigRepository) LoadActiveConfigValues(ctx context.Context, scopeType string, scopeValue string) ([]config.DynamicConfigValue, error) {
	var models []ConfigItemModel
	if err := DB(ctx, r.db).
		Where("scope_type = ? AND scope_value = ? AND status = ?", scopeType, scopeValue, "ACTIVE").
		Order("config_key ASC, version DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	values := make([]config.DynamicConfigValue, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		if _, ok := seen[model.ConfigKey]; ok {
			continue
		}
		seen[model.ConfigKey] = struct{}{}
		values = append(values, toDynamicConfigValue(model))
	}
	return values, nil
}

func (r *RuntimeConfigRepository) ListConfigHistory(ctx context.Context, scopeType string, scopeValue string, limit int) ([]config.DynamicConfigValue, error) {
	if limit <= 0 {
		limit = 20
	}
	var models []ConfigItemModel
	if err := DB(ctx, r.db).
		Where("scope_type = ? AND scope_value = ?", scopeType, scopeValue).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	values := make([]config.DynamicConfigValue, 0, len(models))
	for _, model := range models {
		values = append(values, toDynamicConfigValue(model))
	}
	return values, nil
}

func (r *RuntimeConfigRepository) ApplyConfigPatch(ctx context.Context, input runtimeconfigapp.ApplyPatchInput) error {
	tx := DB(ctx, r.db)
	for _, change := range input.Changes {
		var latestVersion int64
		if err := tx.Model(&ConfigItemModel{}).
			Where("config_key = ? AND scope_type = ? AND scope_value = ?", change.Key, input.ScopeType, input.ScopeValue).
			Select("COALESCE(MAX(version), 0)").
			Scan(&latestVersion).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConfigItemModel{}).
			Where("config_key = ? AND scope_type = ? AND scope_value = ? AND status = ?", change.Key, input.ScopeType, input.ScopeValue, "ACTIVE").
			Updates(map[string]any{"status": "SUPERSEDED"}).Error; err != nil {
			return err
		}
		approvedBy := change.ApprovedBy
		if err := tx.Create(&ConfigItemModel{
			ConfigKey:   change.Key,
			ScopeType:   input.ScopeType,
			ScopeValue:  input.ScopeValue,
			Version:     latestVersion + 1,
			ValueJSON:   string(change.ValueJSON),
			EffectiveAt: change.EffectiveAt,
			Status:      "ACTIVE",
			CreatedBy:   change.CreatedBy,
			ApprovedBy:  &approvedBy,
			Reason:      change.Reason,
			CreatedAt:   change.CreatedAt,
		}).Error; err != nil {
			return err
		}
	}
	return tx.Create(&AuditLogModel{
		AuditID:      input.Audit.AuditID,
		ActorType:    input.Audit.ActorType,
		ActorID:      input.Audit.ActorID,
		Action:       input.Audit.Action,
		ResourceType: input.Audit.ResourceType,
		ResourceID:   input.Audit.ResourceID,
		BeforeJSON:   input.Audit.BeforeJSON,
		AfterJSON:    input.Audit.AfterJSON,
		TraceID:      input.Audit.TraceID,
		CreatedAt:    input.Audit.CreatedAt,
	}).Error
}

func toDynamicConfigValue(model ConfigItemModel) config.DynamicConfigValue {
	return config.DynamicConfigValue{
		Key:         model.ConfigKey,
		ScopeType:   model.ScopeType,
		ScopeValue:  model.ScopeValue,
		Version:     model.Version,
		ValueJSON:   json.RawMessage(model.ValueJSON),
		Status:      model.Status,
		CreatedBy:   model.CreatedBy,
		ApprovedBy:  model.ApprovedBy,
		Reason:      model.Reason,
		EffectiveAt: model.EffectiveAt,
		CreatedAt:   model.CreatedAt,
	}
}

func (r *RuntimeConfigRepository) SeedDefaultRuntimeConfig(ctx context.Context, snapshot config.RuntimeConfigSnapshot, operatorID string, now time.Time) error {
	values, err := defaultDynamicValues(snapshot)
	if err != nil {
		return err
	}
	existing, err := r.LoadActiveConfigValues(ctx, config.ConfigScopeTypeGlobal, config.ConfigScopeValueGlobal)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	changes := make([]runtimeconfigapp.ConfigChange, 0, len(values))
	for key, raw := range values {
		changes = append(changes, runtimeconfigapp.ConfigChange{
			Key:         key,
			ValueJSON:   raw,
			CreatedBy:   operatorID,
			ApprovedBy:  operatorID,
			Reason:      "bootstrap runtime config baseline",
			EffectiveAt: now,
			CreatedAt:   now,
		})
	}
	return r.ApplyConfigPatch(ctx, runtimeconfigapp.ApplyPatchInput{
		ScopeType:  config.ConfigScopeTypeGlobal,
		ScopeValue: config.ConfigScopeValueGlobal,
		Changes:    changes,
		Audit: runtimeconfigapp.AuditLog{
			AuditID:      fmt.Sprintf("audit_bootstrap_%d", now.UnixNano()),
			ActorType:    "SYSTEM",
			ActorID:      operatorID,
			Action:       "runtime_config.bootstrap",
			ResourceType: "runtime_config",
			ResourceID:   "global",
			TraceID:      "runtime_bootstrap",
			CreatedAt:    now,
		},
	})
}

func defaultDynamicValues(snapshot config.RuntimeConfigSnapshot) (map[string]json.RawMessage, error) {
	items := map[string]any{
		"system.mode":                              snapshot.Global.SystemMode,
		"system.read_only":                         snapshot.Global.ReadOnly,
		"system.reduce_only":                       snapshot.Global.ReduceOnly,
		"system.trace_header_required":             snapshot.Global.TraceHeaderRequired,
		"risk.global_buffer_ratio":                 snapshot.Risk.GlobalBufferRatio,
		"risk.mark_price_stale_sec":                snapshot.Risk.MarkPriceStaleSec,
		"risk.force_reduce_only_on_stale_price":    snapshot.Risk.ForceReduceOnlyOnStalePrice,
		"risk.liquidation_penalty_rate":            snapshot.Risk.LiquidationPenaltyRate,
		"risk.liquidation_extra_slippage_bps":      snapshot.Risk.LiquidationExtraSlippageBps,
		"risk.max_open_orders_per_user_per_symbol": snapshot.Risk.MaxOpenOrdersPerUserPerSymbol,
		"risk.net_exposure_hard_limit":             snapshot.Risk.NetExposureHardLimit,
		"risk.max_exposure_slippage_bps":           snapshot.Risk.MaxExposureSlippageBps,
		"hedge.enabled":                            snapshot.Hedge.Enabled,
		"hedge.soft_threshold_ratio":               snapshot.Hedge.SoftThresholdRatio,
		"hedge.hard_threshold_ratio":               snapshot.Hedge.HardThresholdRatio,
	}
	values := make(map[string]json.RawMessage, len(items))
	for key, value := range items {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		values[key] = raw
	}
	return values, nil
}
