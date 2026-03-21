package httptransport

import (
	"context"
	"fmt"
	"strconv"

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
	GenerateDepositAddress(ctx context.Context, input walletdomain.GenerateDepositAddressInput) (walletdomain.DepositAddress, error)
}

type LocalChainSupport interface {
	GrantNativeToken(ctx context.Context, address string, chainID int64) (string, error)
}

type WalletHandler struct {
	reader          WalletReader
	mutator         WalletMutator
	localChainTools LocalChainSupport
}

func NewWalletHandler(reader WalletReader, mutator WalletMutator, localChainTools LocalChainSupport) *WalletHandler {
	return &WalletHandler{reader: reader, mutator: mutator, localChainTools: localChainTools}
}

type withdrawRequest struct {
	ChainID   int64  `json:"chain_id"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	ToAddress string `json:"to_address"`
}

type localNativeFaucetRequest struct {
	ChainID int64 `json:"chain_id"`
}

func (h *WalletHandler) Register(r gin.IRoutes) {
	r.GET("/wallet/deposit-addresses", h.getDepositAddresses)
	r.POST("/wallet/deposit-addresses/:chainId/generate", h.generateDepositAddress)
	r.GET("/wallet/deposits", h.getDeposits)
	r.GET("/wallet/withdrawals", h.getWithdrawals)
	r.POST("/wallet/withdrawals", h.createWithdrawal)
	r.POST("/wallet/local-faucet/native", h.requestLocalNativeFaucet)
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

func (h *WalletHandler) generateDepositAddress(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	chainID, err := strconv.ParseInt(c.Param("chainId"), 10, 64)
	if err != nil || chainID <= 0 {
		writeError(c, fmt.Errorf("%w: invalid chain id", errorsx.ErrInvalidArgument))
		return
	}

	item, err := h.mutator.GenerateDepositAddress(c.Request.Context(), walletdomain.GenerateDepositAddressInput{
		UserID:  userIDFromContext(c),
		ChainID: chainID,
		Asset:   "USDC",
		TraceID: traceIDFromContext(c),
	})
	if err != nil {
		writeError(c, err)
		return
	}

	writeOK(c, readmodel.DepositAddressItem{
		ChainID:       item.ChainID,
		Asset:         item.Asset,
		Address:       item.Address,
		Confirmations: item.Confirmations,
	})
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

func (h *WalletHandler) requestLocalNativeFaucet(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	if h.localChainTools == nil {
		writeError(c, fmt.Errorf("%w: local native faucet disabled", errorsx.ErrForbidden))
		return
	}
	var req localNativeFaucetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}
	txHash, err := h.localChainTools.GrantNativeToken(
		c.Request.Context(),
		addressFromContext(c),
		req.ChainID,
	)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"tx_hash": txHash})
}
