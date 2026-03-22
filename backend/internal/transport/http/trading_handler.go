package httptransport

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type TradingReader interface {
	ListOrders(ctx context.Context, userID uint64) ([]readmodel.OrderItem, error)
	ListFills(ctx context.Context, userID uint64) ([]readmodel.FillItem, error)
	ListPositions(ctx context.Context, userID uint64) ([]readmodel.PositionItem, error)
}

type TradingMutator interface {
	CreateOrder(ctx context.Context, input orderdomain.CreateOrderInput) (orderdomain.Order, error)
	CancelOrder(ctx context.Context, input orderdomain.CancelOrderInput) error
}

type TradingHandler struct {
	reader  TradingReader
	mutator TradingMutator
}

func NewTradingHandler(reader TradingReader, mutator TradingMutator) *TradingHandler {
	return &TradingHandler{reader: reader, mutator: mutator}
}

func (h *TradingHandler) Register(r gin.IRoutes) {
	r.POST("/orders", h.createOrder)
	r.GET("/orders", h.getOrders)
	r.POST("/orders/:orderId/cancel", h.cancelOrder)
	r.GET("/fills", h.getFills)
	r.GET("/positions", h.getPositions)
}

type createOrderRequest struct {
	ClientOrderID  string  `json:"client_order_id"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	PositionEffect string  `json:"position_effect"`
	Type           string  `json:"type"`
	Qty            string  `json:"qty"`
	Price          *string `json:"price"`
	TriggerPrice   *string `json:"trigger_price"`
	ReduceOnly     bool    `json:"reduce_only"`
	TimeInForce    string  `json:"time_in_force"`
	MaxSlippageBps int     `json:"max_slippage_bps"`
}

func (h *TradingHandler) createOrder(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	if h.mutator == nil {
		writeError(c, fmt.Errorf("%w: trading disabled", errorsx.ErrForbidden))
		return
	}
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}
	order, err := h.mutator.CreateOrder(c.Request.Context(), orderdomain.CreateOrderInput{
		UserID:         userIDFromContext(c),
		ClientOrderID:  req.ClientOrderID,
		Symbol:         req.Symbol,
		Side:           req.Side,
		PositionEffect: req.PositionEffect,
		Type:           req.Type,
		TimeInForce:    req.TimeInForce,
		Price:          req.Price,
		TriggerPrice:   req.TriggerPrice,
		Qty:            req.Qty,
		ReduceOnly:     req.ReduceOnly,
		MaxSlippageBps: req.MaxSlippageBps,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		TraceID:        traceIDFromContext(c),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, readmodel.OrderItem{
		OrderID:        order.OrderID,
		ClientOrderID:  order.ClientOrderID,
		Symbol:         order.Symbol,
		Side:           order.Side,
		PositionEffect: order.PositionEffect,
		Type:           order.Type,
		Qty:            order.Qty,
		FilledQty:      order.FilledQty,
		AvgFillPrice:   order.AvgFillPrice,
		Price:          order.Price,
		TriggerPrice:   order.TriggerPrice,
		ReduceOnly:     order.ReduceOnly,
		Status:         order.Status,
		RejectReason:   order.RejectReason,
	})
}

func (h *TradingHandler) cancelOrder(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	if h.mutator == nil {
		writeError(c, fmt.Errorf("%w: trading disabled", errorsx.ErrForbidden))
		return
	}
	if err := h.mutator.CancelOrder(c.Request.Context(), orderdomain.CancelOrderInput{
		UserID:         userIDFromContext(c),
		OrderID:        c.Param("orderId"),
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		TraceID:        traceIDFromContext(c),
	}); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"status": "CANCELED"})
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
