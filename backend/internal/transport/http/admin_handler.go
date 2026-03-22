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

type WithdrawApprover interface {
	ApproveWithdraw(ctx context.Context, input walletdomain.ApproveWithdrawInput) error
}

type AdminWithdrawalReader interface {
	ListAdminWithdrawals(ctx context.Context, limit int) ([]readmodel.AdminWithdrawReviewItem, error)
}

type AdminHandler struct {
	approver     WithdrawApprover
	reader       AdminWithdrawalReader
	adminWallets map[string]struct{}
}

func NewAdminHandler(approver WithdrawApprover, reader AdminWithdrawalReader, adminWallets []string) *AdminHandler {
	allow := make(map[string]struct{}, len(adminWallets))
	for _, wallet := range adminWallets {
		allow[strings.ToLower(strings.TrimSpace(wallet))] = struct{}{}
	}
	return &AdminHandler{
		approver:     approver,
		reader:       reader,
		adminWallets: allow,
	}
}

func (h *AdminHandler) Register(r gin.IRoutes) {
	r.GET("/admin/withdrawals", h.listWithdrawals)
	r.POST("/admin/withdrawals/:withdrawId/approve", h.approveWithdrawal)
}

func (h *AdminHandler) listWithdrawals(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	address := strings.ToLower(addressFromContext(c))
	if _, ok := h.adminWallets[address]; !ok {
		writeError(c, fmt.Errorf("%w: admin wallet required", errorsx.ErrForbidden))
		return
	}
	items, err := h.reader.ListAdminWithdrawals(c.Request.Context(), 200)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}

func (h *AdminHandler) approveWithdrawal(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	address := strings.ToLower(addressFromContext(c))
	if _, ok := h.adminWallets[address]; !ok {
		writeError(c, fmt.Errorf("%w: admin wallet required", errorsx.ErrForbidden))
		return
	}
	withdrawID := c.Param("withdrawId")
	if err := h.approver.ApproveWithdraw(c.Request.Context(), walletdomain.ApproveWithdrawInput{
		WithdrawID:     withdrawID,
		OperatorID:     address,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		TraceID:        traceIDFromContext(c),
	}); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"withdraw_id": withdrawID, "status": walletdomain.StatusApproved})
}
