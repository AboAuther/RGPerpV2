package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type fakeExplorerReader struct {
	userID  uint64
	isAdmin bool
	limit   int
	items   []readmodel.ExplorerEvent
}

func (f *fakeExplorerReader) ListEvents(_ context.Context, userID uint64, isAdmin bool, limit int) ([]readmodel.ExplorerEvent, error) {
	f.userID = userID
	f.isAdmin = isAdmin
	f.limit = limit
	return f.items, nil
}

func TestExplorerHandler_FiltersAsUser(t *testing.T) {
	reader := &fakeExplorerReader{items: []readmodel.ExplorerEvent{{EventID: "evt_1"}}}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0x0000000000000000000000000000000000000001"}},
		nil,
		nil,
		nil,
		NewExplorerHandler(reader, []string{"0x0000000000000000000000000000000000000002"}),
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/explorer/events", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if reader.userID != 7 || reader.isAdmin {
		t.Fatalf("expected user-scoped explorer query, got userID=%d isAdmin=%v", reader.userID, reader.isAdmin)
	}
	if reader.limit != 100 {
		t.Fatalf("expected default limit 100, got %d", reader.limit)
	}
}

func TestExplorerHandler_AllowsAdminGlobalView(t *testing.T) {
	reader := &fakeExplorerReader{items: []readmodel.ExplorerEvent{{EventID: "evt_1"}}}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0x0000000000000000000000000000000000000001"}},
		nil,
		nil,
		nil,
		NewExplorerHandler(reader, []string{"0x0000000000000000000000000000000000000001"}),
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/explorer/events", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !reader.isAdmin {
		t.Fatalf("expected admin-scoped explorer query")
	}
}
