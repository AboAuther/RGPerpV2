package httptransport

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type AccountReader interface {
	GetSummary(ctx context.Context, userID uint64) (readmodel.AccountSummary, error)
	ListBalances(ctx context.Context, userID uint64) ([]readmodel.BalanceItem, error)
	GetRisk(ctx context.Context, userID uint64) (readmodel.RiskSnapshot, error)
	ListFunding(ctx context.Context, userID uint64) ([]readmodel.FundingItem, error)
	ListTransfers(ctx context.Context, userID uint64) ([]readmodel.TransferItem, error)
}

type TransferUseCase interface {
	Transfer(ctx context.Context, req walletdomain.TransferRequest) error
}

type AccountHandler struct {
	reader    AccountReader
	transfers TransferUseCase
	resolver  walletdomain.TransferResolver
}

func NewAccountHandler(reader AccountReader, transfers TransferUseCase, resolver walletdomain.TransferResolver) *AccountHandler {
	return &AccountHandler{reader: reader, transfers: transfers, resolver: resolver}
}

type transferRequest struct {
	ToAddress string `json:"to_address"`
	Amount    string `json:"amount"`
	Asset     string `json:"asset"`
}

func (h *AccountHandler) Register(r gin.IRoutes) {
	r.GET("/account/summary", h.getSummary)
	r.GET("/account/balances", h.getBalances)
	r.GET("/account/risk", h.getRisk)
	r.GET("/account/funding", h.getFunding)
	r.GET("/account/transfers", h.getTransfers)
	r.POST("/account/transfer", h.transfer)
}

func (h *AccountHandler) getSummary(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.GetSummary(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *AccountHandler) getBalances(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.ListBalances(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *AccountHandler) getRisk(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.GetRisk(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *AccountHandler) getFunding(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.ListFunding(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *AccountHandler) getTransfers(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	data, err := h.reader.ListTransfers(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *AccountHandler) transfer(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	var req transferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}
	if h.resolver == nil {
		writeError(c, fmt.Errorf("%w: transfer resolver unavailable", errorsx.ErrForbidden))
		return
	}
	toUserID, err := h.resolver.ResolveUserIDByAddress(c.Request.Context(), req.ToAddress)
	if err != nil {
		writeError(c, err)
		return
	}
	userID := userIDFromContext(c)
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("transfer:%d:%s", userID, traceIDFromContext(c))
	}
	if err := h.transfers.Transfer(c.Request.Context(), walletdomain.TransferRequest{
		FromUserID:     userID,
		ToUserID:       toUserID,
		Asset:          req.Asset,
		Amount:         req.Amount,
		IdempotencyKey: idempotencyKey,
		TraceID:        traceIDFromContext(c),
	}); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}
