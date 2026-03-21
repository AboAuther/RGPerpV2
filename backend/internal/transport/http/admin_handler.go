package httptransport

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type WithdrawApprover interface {
	ApproveWithdraw(ctx context.Context, input walletdomain.ApproveWithdrawInput) error
}

type AdminHandler struct {
	approver     WithdrawApprover
	adminWallets map[string]struct{}
}

func NewAdminHandler(approver WithdrawApprover, adminWallets []string) *AdminHandler {
	allow := make(map[string]struct{}, len(adminWallets))
	for _, wallet := range adminWallets {
		allow[strings.ToLower(strings.TrimSpace(wallet))] = struct{}{}
	}
	return &AdminHandler{
		approver:     approver,
		adminWallets: allow,
	}
}

func (h *AdminHandler) Register(r gin.IRoutes) {
	r.POST("/admin/withdrawals/:withdrawId/approve", h.approveWithdrawal)
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
