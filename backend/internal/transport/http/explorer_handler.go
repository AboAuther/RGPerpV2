package httptransport

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type ExplorerReader interface {
	ListEvents(ctx context.Context, userID uint64, isAdmin bool, limit int) ([]readmodel.ExplorerEvent, error)
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
	items, err := h.reader.ListEvents(c.Request.Context(), userIDFromContext(c), isAdmin, 100)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}
