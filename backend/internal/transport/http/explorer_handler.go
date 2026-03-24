package httptransport

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ExplorerReader interface {
	ListEvents(ctx context.Context, userID uint64, isAdmin bool, filter readmodel.ExplorerEventFilter) ([]readmodel.ExplorerEvent, error)
}

type ExplorerHandler struct {
	reader       ExplorerReader
	adminWallets map[string]struct{}
}

func NewExplorerHandler(reader ExplorerReader, adminWallets []string) *ExplorerHandler {
	allow := make(map[string]struct{}, len(adminWallets))
	for _, wallet := range adminWallets {
		allow[strings.ToLower(strings.TrimSpace(wallet))] = struct{}{}
	}
	return &ExplorerHandler{reader: reader, adminWallets: allow}
}

func (h *ExplorerHandler) Register(r gin.IRoutes) {
	r.GET("/explorer/events", h.getEvents)
}

func (h *ExplorerHandler) getEvents(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	_, isAdmin := h.adminWallets[strings.ToLower(strings.TrimSpace(addressFromContext(c)))]
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(c, errorsx.ErrInvalidArgument)
			return
		}
		if parsed > 200 {
			parsed = 200
		}
		limit = parsed
	}
	items, err := h.reader.ListEvents(c.Request.Context(), userIDFromContext(c), isAdmin, readmodel.ExplorerEventFilter{
		Query:          strings.TrimSpace(c.Query("q")),
		EventType:      strings.TrimSpace(c.Query("event_type")),
		Asset:          strings.TrimSpace(c.Query("asset")),
		LedgerTxID:     strings.TrimSpace(c.Query("ledger_tx_id")),
		ChainTxHash:    strings.TrimSpace(c.Query("chain_tx_hash")),
		OrderID:        strings.TrimSpace(c.Query("order_id")),
		FillID:         strings.TrimSpace(c.Query("fill_id")),
		PositionID:     strings.TrimSpace(c.Query("position_id")),
		Address:        strings.TrimSpace(c.Query("address")),
		FundingBatchID: strings.TrimSpace(c.Query("funding_batch_id")),
		BlockHeight:    strings.TrimSpace(c.Query("block_height")),
		Limit:          limit,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}
