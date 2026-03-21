package httptransport

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type WalletReader interface {
	ListDepositAddresses(ctx context.Context, userID uint64) ([]walletdomain.DepositAddress, error)
	ListDeposits(ctx context.Context, userID uint64) ([]readmodel.DepositItem, error)
	ListWithdrawals(ctx context.Context, userID uint64) ([]readmodel.WithdrawItem, error)
}

type WalletMutator interface {
	RequestWithdraw(ctx context.Context, input walletdomain.RequestWithdrawInput) (walletdomain.WithdrawRequest, error)
	GrantReviewFaucet(ctx context.Context, input walletdomain.GrantReviewFaucetInput) (walletdomain.DepositChainTx, error)
}

type WalletHandler struct {
	reader              WalletReader
	mutator             WalletMutator
	reviewFaucetEnabled bool
}

func NewWalletHandler(reader WalletReader, mutator WalletMutator, reviewFaucetEnabled bool) *WalletHandler {
	return &WalletHandler{reader: reader, mutator: mutator, reviewFaucetEnabled: reviewFaucetEnabled}
}

type withdrawRequest struct {
	ChainID   int64  `json:"chain_id"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	ToAddress string `json:"to_address"`
}

type faucetRequest struct {
	Address string `json:"address"`
	ChainID int64  `json:"chain_id"`
}

func (h *WalletHandler) Register(r gin.IRoutes) {
	r.GET("/wallet/deposit-addresses", h.getDepositAddresses)
	r.GET("/wallet/deposits", h.getDeposits)
	r.GET("/wallet/withdrawals", h.getWithdrawals)
	r.POST("/wallet/withdrawals", h.createWithdrawal)
	r.POST("/review/faucet", h.requestFaucet)
}

func (h *WalletHandler) getDepositAddresses(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	items, err := h.reader.ListDepositAddresses(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	resp := make([]readmodel.DepositAddressItem, 0, len(items))
	for _, item := range items {
		resp = append(resp, readmodel.DepositAddressItem{
			ChainID:       item.ChainID,
			Asset:         item.Asset,
			Address:       item.Address,
			Confirmations: item.Confirmations,
		})
	}
	writeOK(c, resp)
}

func (h *WalletHandler) getDeposits(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	items, err := h.reader.ListDeposits(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}

func (h *WalletHandler) getWithdrawals(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	items, err := h.reader.ListWithdrawals(c.Request.Context(), userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}

func (h *WalletHandler) createWithdrawal(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	var req withdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}
	withdraw, err := h.mutator.RequestWithdraw(c.Request.Context(), walletdomain.RequestWithdrawInput{
		UserID:         userIDFromContext(c),
		ChainID:        req.ChainID,
		Asset:          req.Asset,
		Amount:         req.Amount,
		FeeAmount:      "1",
		ToAddress:      req.ToAddress,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		TraceID:        traceIDFromContext(c),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{
		"withdraw_id": withdraw.WithdrawID,
		"status":      withdraw.Status,
	})
}

func (h *WalletHandler) requestFaucet(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	if !h.reviewFaucetEnabled {
		writeError(c, fmt.Errorf("%w: review faucet disabled", errorsx.ErrForbidden))
		return
	}
	var req faucetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Address), strings.TrimSpace(addressFromContext(c))) {
		writeError(c, fmt.Errorf("%w: faucet address mismatch", errorsx.ErrForbidden))
		return
	}
	if _, err := h.mutator.GrantReviewFaucet(c.Request.Context(), walletdomain.GrantReviewFaucetInput{
		UserID:         userIDFromContext(c),
		ChainID:        req.ChainID,
		Asset:          "USDC",
		Amount:         "10000",
		ToAddress:      req.Address,
		IdempotencyKey: "review_faucet:" + traceIDFromContext(c),
		TraceID:        traceIDFromContext(c),
	}); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}
