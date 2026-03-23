package httptransport

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	GetLedgerOverview(ctx context.Context, scopeAsset string) (readmodel.LedgerOverview, error)
	GetLatestLedgerAuditReport(ctx context.Context, scopeAsset string) (readmodel.LedgerAuditReport, error)
	RunLedgerAudit(ctx context.Context, executedBy string, scopeAsset string) (readmodel.LedgerAuditReport, error)
	GetRiskMonitorDashboard(ctx context.Context) (readmodel.RiskMonitorDashboard, error)
}

type AdminRiskMutator interface {
	RecalculateAccount(ctx context.Context, userID uint64, operatorID string) (readmodel.AdminRiskRecalculationResult, error)
}

type AdminConfigManager interface {
	GetRuntimeConfigView(ctx context.Context, limit int) (readmodel.RuntimeConfigView, error)
	UpdateRuntimeConfig(ctx context.Context, input AdminRuntimeConfigUpdateInput) (readmodel.RuntimeConfigView, error)
}

type AdminRuntimeConfigUpdateInput struct {
	OperatorID string
	TraceID    string
	Reason     string
	Values     map[string]json.RawMessage
}

type AdminHandler struct {
	approver     WithdrawApprover
	reader       AdminWithdrawalReader
	riskMutator  AdminRiskMutator
	configs      AdminConfigManager
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

func (h *AdminHandler) SetRiskMutator(mutator AdminRiskMutator) {
	h.riskMutator = mutator
}

func (h *AdminHandler) SetConfigManager(manager AdminConfigManager) {
	h.configs = manager
}

func (h *AdminHandler) Register(r gin.IRoutes) {
	r.GET("/admin/withdrawals", h.listWithdrawals)
	r.POST("/admin/withdrawals/:withdrawId/approve", h.approveWithdrawal)
	r.GET("/admin/risk/net-exposures", h.getRiskMonitorDashboard)
	r.POST("/admin/risk/accounts/:userId/recalculate", h.recalculateAccountRisk)
	r.GET("/admin/configs/runtime", h.getRuntimeConfig)
	r.POST("/admin/configs/runtime", h.updateRuntimeConfig)
	r.GET("/admin/ledger/overview", h.getLedgerOverview)
	r.GET("/admin/ledger/audits/latest", h.getLatestLedgerAudit)
	r.GET("/admin/ledger/audits/latest/export", h.exportLatestLedgerAudit)
	r.POST("/admin/ledger/audits/run", h.runLedgerAudit)
}

func (h *AdminHandler) getRiskMonitorDashboard(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	item, err := h.reader.GetRiskMonitorDashboard(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) recalculateAccountRisk(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.riskMutator == nil {
		writeError(c, fmt.Errorf("%w: admin risk mutation disabled", errorsx.ErrForbidden))
		return
	}
	userID, err := strconv.ParseUint(strings.TrimSpace(c.Param("userId")), 10, 64)
	if err != nil || userID == 0 {
		writeError(c, fmt.Errorf("%w: invalid user id", errorsx.ErrInvalidArgument))
		return
	}
	result, err := h.riskMutator.RecalculateAccount(c.Request.Context(), userID, strings.ToLower(addressFromContext(c)))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, result)
}

type runtimeConfigPatchRequest struct {
	Reason string `json:"reason"`
	Global struct {
		ReadOnly            *bool `json:"read_only"`
		ReduceOnly          *bool `json:"reduce_only"`
		TraceHeaderRequired *bool `json:"trace_header_required"`
	} `json:"global"`
	Risk struct {
		GlobalBufferRatio             *string `json:"global_buffer_ratio"`
		MarkPriceStaleSec             *int    `json:"mark_price_stale_sec"`
		ForceReduceOnlyOnStalePrice   *bool   `json:"force_reduce_only_on_stale_price"`
		LiquidationPenaltyRate        *string `json:"liquidation_penalty_rate"`
		LiquidationExtraSlippageBps   *int    `json:"liquidation_extra_slippage_bps"`
		MaxOpenOrdersPerUserPerSymbol *int    `json:"max_open_orders_per_user_per_symbol"`
		NetExposureHardLimit          *string `json:"net_exposure_hard_limit"`
		MaxExposureSlippageBps        *int    `json:"max_exposure_slippage_bps"`
	} `json:"risk"`
	Hedge struct {
		Enabled            *bool   `json:"enabled"`
		SoftThresholdRatio *string `json:"soft_threshold_ratio"`
		HardThresholdRatio *string `json:"hard_threshold_ratio"`
	} `json:"hedge"`
}

func (h *AdminHandler) getRuntimeConfig(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.configs == nil {
		writeError(c, fmt.Errorf("%w: admin config manager disabled", errorsx.ErrForbidden))
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "20")))
	item, err := h.configs.GetRuntimeConfigView(c.Request.Context(), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) updateRuntimeConfig(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.configs == nil {
		writeError(c, fmt.Errorf("%w: admin config manager disabled", errorsx.ErrForbidden))
		return
	}
	var req runtimeConfigPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, fmt.Errorf("%w: invalid runtime config patch", errorsx.ErrInvalidArgument))
		return
	}
	values, err := flattenRuntimeConfigPatch(req)
	if err != nil {
		writeError(c, err)
		return
	}
	item, err := h.configs.UpdateRuntimeConfig(c.Request.Context(), AdminRuntimeConfigUpdateInput{
		OperatorID: strings.ToLower(addressFromContext(c)),
		TraceID:    traceIDFromContext(c),
		Reason:     strings.TrimSpace(req.Reason),
		Values:     values,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
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

func (h *AdminHandler) getLedgerOverview(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	item, err := h.reader.GetLedgerOverview(c.Request.Context(), c.Query("asset"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) getLatestLedgerAudit(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	scopeAsset := c.Query("asset")
	item, err := h.reader.GetLatestLedgerAuditReport(c.Request.Context(), scopeAsset)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) exportLatestLedgerAudit(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "json")))
	scopeAsset := c.Query("asset")
	item, err := h.reader.GetLatestLedgerAuditReport(c.Request.Context(), scopeAsset)
	if err != nil {
		writeError(c, err)
		return
	}
	filename := fmt.Sprintf("ledger-audit-%s-%s.%s", strings.ToLower(item.ScopeAsset), item.AuditReportID, format)
	switch format {
	case "json":
		payload, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			writeError(c, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	case "csv":
		payload, err := marshalLedgerAuditCSV(item)
		if err != nil {
			writeError(c, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		c.Data(http.StatusOK, "text/csv; charset=utf-8", payload)
	default:
		writeError(c, fmt.Errorf("%w: unsupported format", errorsx.ErrInvalidArgument))
	}
}

func (h *AdminHandler) runLedgerAudit(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	item, err := h.reader.RunLedgerAudit(c.Request.Context(), strings.ToLower(addressFromContext(c)), c.Query("asset"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) requireAdmin(c *gin.Context) bool {
	if !requireUser(c) {
		return false
	}
	address := strings.ToLower(addressFromContext(c))
	if _, ok := h.adminWallets[address]; !ok {
		writeError(c, fmt.Errorf("%w: admin wallet required", errorsx.ErrForbidden))
		return false
	}
	return true
}

func marshalLedgerAuditCSV(report readmodel.LedgerAuditReport) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"section", "asset", "chain_id", "chain_name", "field", "value", "status", "summary"}); err != nil {
		return nil, err
	}
	for _, item := range report.Overview {
		rows := [][2]string{
			{"user_liability", item.UserLiability},
			{"user_wallet", item.UserWallet},
			{"user_margin", item.UserMargin},
			{"user_withdraw_hold", item.UserWithdrawHold},
			{"platform_revenue", item.PlatformRevenue},
			{"risk_buffer", item.RiskBuffer},
			{"in_flight", item.InFlight},
			{"custody_mirror", item.CustodyMirror},
			{"net_balance", item.NetBalance},
		}
		for _, row := range rows {
			if err := writer.Write([]string{"overview", item.Asset, "", "", row[0], row[1], "", ""}); err != nil {
				return nil, err
			}
		}
	}
	for _, item := range report.ChainBalances {
		if err := writer.Write([]string{"chain_balance", item.Asset, fmt.Sprintf("%d", item.ChainID), item.ChainName, item.RowType, item.OnchainBalance, item.Status, item.Delta}); err != nil {
			return nil, err
		}
	}
	for _, item := range report.Checks {
		if err := writer.Write([]string{"check", "", "", "", item.CheckKey, item.Value, item.Status, item.Summary}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func flattenRuntimeConfigPatch(req runtimeConfigPatchRequest) (map[string]json.RawMessage, error) {
	values := make(map[string]json.RawMessage)
	appendValue := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		values[key] = raw
		return nil
	}
	if req.Global.ReadOnly != nil {
		if err := appendValue("system.read_only", *req.Global.ReadOnly); err != nil {
			return nil, err
		}
	}
	if req.Global.ReduceOnly != nil {
		if err := appendValue("system.reduce_only", *req.Global.ReduceOnly); err != nil {
			return nil, err
		}
	}
	if req.Global.TraceHeaderRequired != nil {
		if err := appendValue("system.trace_header_required", *req.Global.TraceHeaderRequired); err != nil {
			return nil, err
		}
	}
	if req.Risk.GlobalBufferRatio != nil {
		if err := appendValue("risk.global_buffer_ratio", strings.TrimSpace(*req.Risk.GlobalBufferRatio)); err != nil {
			return nil, err
		}
	}
	if req.Risk.MarkPriceStaleSec != nil {
		if err := appendValue("risk.mark_price_stale_sec", *req.Risk.MarkPriceStaleSec); err != nil {
			return nil, err
		}
	}
	if req.Risk.ForceReduceOnlyOnStalePrice != nil {
		if err := appendValue("risk.force_reduce_only_on_stale_price", *req.Risk.ForceReduceOnlyOnStalePrice); err != nil {
			return nil, err
		}
	}
	if req.Risk.LiquidationPenaltyRate != nil {
		if err := appendValue("risk.liquidation_penalty_rate", strings.TrimSpace(*req.Risk.LiquidationPenaltyRate)); err != nil {
			return nil, err
		}
	}
	if req.Risk.LiquidationExtraSlippageBps != nil {
		if err := appendValue("risk.liquidation_extra_slippage_bps", *req.Risk.LiquidationExtraSlippageBps); err != nil {
			return nil, err
		}
	}
	if req.Risk.MaxOpenOrdersPerUserPerSymbol != nil {
		if err := appendValue("risk.max_open_orders_per_user_per_symbol", *req.Risk.MaxOpenOrdersPerUserPerSymbol); err != nil {
			return nil, err
		}
	}
	if req.Risk.NetExposureHardLimit != nil {
		if err := appendValue("risk.net_exposure_hard_limit", strings.TrimSpace(*req.Risk.NetExposureHardLimit)); err != nil {
			return nil, err
		}
	}
	if req.Risk.MaxExposureSlippageBps != nil {
		if err := appendValue("risk.max_exposure_slippage_bps", *req.Risk.MaxExposureSlippageBps); err != nil {
			return nil, err
		}
	}
	if req.Hedge.Enabled != nil {
		if err := appendValue("hedge.enabled", *req.Hedge.Enabled); err != nil {
			return nil, err
		}
	}
	if req.Hedge.SoftThresholdRatio != nil {
		if err := appendValue("hedge.soft_threshold_ratio", strings.TrimSpace(*req.Hedge.SoftThresholdRatio)); err != nil {
			return nil, err
		}
	}
	if req.Hedge.HardThresholdRatio != nil {
		if err := appendValue("hedge.hard_threshold_ratio", strings.TrimSpace(*req.Hedge.HardThresholdRatio)); err != nil {
			return nil, err
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: no runtime config change detected", errorsx.ErrInvalidArgument)
	}
	return values, nil
}
