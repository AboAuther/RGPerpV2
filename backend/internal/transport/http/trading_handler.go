package httptransport

import (
	"context"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type TradingReader interface {
	ListOrders(ctx context.Context, userID uint64) ([]readmodel.OrderItem, error)
	ListFills(ctx context.Context, userID uint64) ([]readmodel.FillItem, error)
	ListPositions(ctx context.Context, userID uint64) ([]readmodel.PositionItem, error)
}

type TradingHandler struct {
	reader TradingReader
}

func NewTradingHandler(reader TradingReader) *TradingHandler {
	return &TradingHandler{reader: reader}
}

func (h *TradingHandler) Register(r gin.IRoutes) {
	r.GET("/orders", h.getOrders)
	r.GET("/fills", h.getFills)
	r.GET("/positions", h.getPositions)
}

func (h *TradingHandler) getOrders(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.ListOrders(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *TradingHandler) getFills(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.ListFills(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *TradingHandler) getPositions(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.ListPositions(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}
