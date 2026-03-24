package main

import (
	"context"
	"encoding/json"
	"testing"

	runtimeconfigapp "github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	httptransport "github.com/xiaobao/rgperp/backend/internal/transport/http"
)

type fakeRuntimeConfigService struct {
	updateSeen runtimeconfigapp.UpdateInput
}

func (f *fakeRuntimeConfigService) GetRuntimeConfigView(_ context.Context, _ int) (readmodel.RuntimeConfigView, error) {
	return readmodel.RuntimeConfigView{}, nil
}

func (f *fakeRuntimeConfigService) UpdateRuntimeConfig(_ context.Context, input runtimeconfigapp.UpdateInput) (readmodel.RuntimeConfigView, error) {
	f.updateSeen = input
	return readmodel.RuntimeConfigView{}, nil
}

func TestAdminRuntimeConfigManager_PassesPairValues(t *testing.T) {
	service := &fakeRuntimeConfigService{}
	manager := adminRuntimeConfigManager{service: service}

	pairValues := map[string]map[string]json.RawMessage{
		"BTC-USDC": {
			"market.max_leverage":   json.RawMessage(`"1000"`),
			"market.session_policy": json.RawMessage(`"ALWAYS_OPEN"`),
		},
	}
	_, err := manager.UpdateRuntimeConfig(context.Background(), httptransport.AdminRuntimeConfigUpdateInput{
		OperatorID: "0xabc",
		TraceID:    "trace_runtimecfg",
		Reason:     "test",
		PairValues: pairValues,
	})
	if err != nil {
		t.Fatalf("UpdateRuntimeConfig returned error: %v", err)
	}
	if service.updateSeen.OperatorID != "0xabc" {
		t.Fatalf("expected operator id to be forwarded, got %q", service.updateSeen.OperatorID)
	}
	if service.updateSeen.PairValues == nil {
		t.Fatalf("expected pair values to be forwarded")
	}
	if _, ok := service.updateSeen.PairValues["BTC-USDC"]; !ok {
		t.Fatalf("expected BTC-USDC pair patch to be forwarded")
	}
	if _, ok := service.updateSeen.PairValues["BTC-USDC"]["market.max_leverage"]; !ok {
		t.Fatalf("expected market.max_leverage to be forwarded")
	}
}
