package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type fakeSystemReader struct {
	items []readmodel.SystemChainItem
}

func (f fakeSystemReader) ListChains(_ context.Context) ([]readmodel.SystemChainItem, error) {
	return f.items, nil
}

func TestSystemHandler_ListChains(t *testing.T) {
	engine := NewEngine(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewSystemHandler(fakeSystemReader{items: []readmodel.SystemChainItem{{
			ChainID:       31337,
			Key:           "local",
			Name:          "Local Anvil",
			Asset:         "USDC",
			Confirmations: 1,
			LocalTestnet:  true,
		}}}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/chains", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
