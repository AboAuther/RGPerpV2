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
	ReturnWithdrawToReview(ctx context.Context, input walletdomain.ReturnWithdrawToReviewInput) error
	RefundWithdraw(ctx context.Context, input walletdomain.RefundWithdrawInput) error
}

type AdminWithdrawalReader interface {
	ListAdminWithdrawals(ctx context.Context, limit int) ([]readmodel.AdminWithdrawReviewItem, error)
	ListAdminLiquidations(ctx context.Context, limit int) ([]readmodel.AdminLiquidationItem, error)
	ListFundingBatches(ctx context.Context, limit int) ([]readmodel.AdminFundingBatchItem, error)
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

type AdminFundingMutator interface {
	ReverseFundingBatch(ctx context.Context, fundingBatchID string, operatorID string, traceID string, reason string) (readmodel.AdminFundingBatchItem, error)
}

type AdminLedgerMutator interface {
	TopUpInsuranceFund(ctx context.Context, input AdminInsuranceFundTopUpInput) (map[string]any, error)
}

type AdminLiquidationMutator interface {
	RetryPendingLiquidation(ctx context.Context, input AdminLiquidationActionInput) (map[string]any, error)
	ClosePendingLiquidation(ctx context.Context, input AdminLiquidationCloseInput) (map[string]any, error)
}

type AdminRuntimeConfigUpdateInput struct {
	OperatorID string
	TraceID    string
	Reason     string
	Values     map[string]json.RawMessage
	PairValues map[string]map[string]json.RawMessage
}

type AdminInsuranceFundTopUpInput struct {
	OperatorID     string
	TraceID        string
	IdempotencyKey string
	Reason         string
	Asset          string
	Amount         string
	SourceAccount  string
}

type AdminLiquidationActionInput struct {
	LiquidationID string
	OperatorID    string
	TraceID       string
}

type AdminLiquidationCloseInput struct {
	LiquidationID string
	OperatorID    string
	TraceID       string
	Reason        string
}

type AdminHandler struct {
	approver     WithdrawApprover
	reader       AdminWithdrawalReader
	riskMutator  AdminRiskMutator
	configs      AdminConfigManager
	funding      AdminFundingMutator
	ledger       AdminLedgerMutator
	liquidations AdminLiquidationMutator
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

func (h *AdminHandler) SetFundingMutator(mutator AdminFundingMutator) {
	h.funding = mutator
}

func (h *AdminHandler) SetLedgerMutator(mutator AdminLedgerMutator) {
	h.ledger = mutator
}

func (h *AdminHandler) SetLiquidationMutator(mutator AdminLiquidationMutator) {
	h.liquidations = mutator
}

func (h *AdminHandler) Register(r gin.IRoutes) {
	r.GET("/admin/withdrawals", h.listWithdrawals)
	r.GET("/admin/liquidations", h.listLiquidations)
	r.POST("/admin/liquidations/:liquidationId/retry", h.retryLiquidation)
	r.POST("/admin/liquidations/:liquidationId/close", h.closeLiquidation)
	r.POST("/admin/withdrawals/:withdrawId/approve", h.approveWithdrawal)
	r.POST("/admin/withdrawals/:withdrawId/review", h.returnWithdrawalToReview)
	r.POST("/admin/withdrawals/:withdrawId/refund", h.refundWithdrawal)
	r.GET("/admin/risk/net-exposures", h.getRiskMonitorDashboard)
	r.POST("/admin/risk/accounts/:userId/recalculate", h.recalculateAccountRisk)
	r.GET("/admin/funding-batches", h.listFundingBatches)
	r.POST("/admin/funding-batches/:fundingBatchId/reverse", h.reverseFundingBatch)
	r.GET("/admin/configs/runtime", h.getRuntimeConfig)
	r.POST("/admin/configs/runtime", h.updateRuntimeConfig)
	r.GET("/admin/ledger/overview", h.getLedgerOverview)
	r.POST("/admin/ledger/insurance-fund/topups", h.topUpInsuranceFund)
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
	Market struct {
		TakerFeeRate          *string `json:"taker_fee_rate"`
		MakerFeeRate          *string `json:"maker_fee_rate"`
		DefaultMaxSlippageBps *int    `json:"default_max_slippage_bps"`
	} `json:"market"`
	Risk struct {
		GlobalBufferRatio             *string `json:"global_buffer_ratio"`
		MarkPriceStaleSec             *int    `json:"mark_price_stale_sec"`
		ForceReduceOnlyOnStalePrice   *bool   `json:"force_reduce_only_on_stale_price"`
		LiquidationPenaltyRate        *string `json:"liquidation_penalty_rate"`
		MaintenanceMarginUpliftRatio  *string `json:"maintenance_margin_uplift_ratio"`
		LiquidationExtraSlippageBps   *int    `json:"liquidation_extra_slippage_bps"`
		MaxOpenOrdersPerUserPerSymbol *int    `json:"max_open_orders_per_user_per_symbol"`
		NetExposureHardLimit          *string `json:"net_exposure_hard_limit"`
		MaxExposureSlippageBps        *int    `json:"max_exposure_slippage_bps"`
	} `json:"risk"`
	Funding struct {
		IntervalSec           *int    `json:"interval_sec"`
		SourcePollIntervalSec *int    `json:"source_poll_interval_sec"`
		CapRatePerHour        *string `json:"cap_rate_per_hour"`
		MinValidSourceCount   *int    `json:"min_valid_source_count"`
		DefaultModelCrypto    *string `json:"default_model_crypto"`
	} `json:"funding"`
	Hedge struct {
		Enabled            *bool   `json:"enabled"`
		SoftThresholdRatio *string `json:"soft_threshold_ratio"`
		HardThresholdRatio *string `json:"hard_threshold_ratio"`
	} `json:"hedge"`
	Pairs map[string]runtimeConfigPairPatchRequest `json:"pairs"`
}

type runtimeConfigPairPatchRequest struct {
	Market struct {
		MaxLeverage           *string `json:"max_leverage"`
		SessionPolicy         *string `json:"session_policy"`
		TakerFeeRate          *string `json:"taker_fee_rate"`
		MakerFeeRate          *string `json:"maker_fee_rate"`
		DefaultMaxSlippageBps *int    `json:"default_max_slippage_bps"`
	} `json:"market"`
	Risk struct {
		LiquidationPenaltyRate       *string `json:"liquidation_penalty_rate"`
		MaintenanceMarginUpliftRatio *string `json:"maintenance_margin_uplift_ratio"`
	} `json:"risk"`
	Funding struct {
		IntervalSec *int `json:"interval_sec"`
	} `json:"funding"`
}

type reverseFundingBatchRequest struct {
	Reason string `json:"reason"`
}

type insuranceFundTopUpRequest struct {
	Asset         string `json:"asset"`
	Amount        string `json:"amount"`
	SourceAccount string `json:"source_account"`
	Reason        string `json:"reason"`
}

type closeLiquidationRequest struct {
	Reason string `json:"reason"`
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
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeError(c, fmt.Errorf("%w: invalid runtime config patch: %v", errorsx.ErrInvalidArgument, err))
		return
	}
	values, pairValues, err := flattenRuntimeConfigPatch(req)
	if err != nil {
		writeError(c, err)
		return
	}
	item, err := h.configs.UpdateRuntimeConfig(c.Request.Context(), AdminRuntimeConfigUpdateInput{
		OperatorID: strings.ToLower(addressFromContext(c)),
		TraceID:    traceIDFromContext(c),
		Reason:     strings.TrimSpace(req.Reason),
		Values:     values,
		PairValues: pairValues,
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

func (h *AdminHandler) listLiquidations(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	items, err := h.reader.ListAdminLiquidations(c.Request.Context(), 200)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}

func (h *AdminHandler) retryLiquidation(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.liquidations == nil {
		writeError(c, fmt.Errorf("%w: liquidation admin operations disabled", errorsx.ErrForbidden))
		return
	}
	item, err := h.liquidations.RetryPendingLiquidation(c.Request.Context(), AdminLiquidationActionInput{
		LiquidationID: strings.TrimSpace(c.Param("liquidationId")),
		OperatorID:    strings.ToLower(addressFromContext(c)),
		TraceID:       traceIDFromContext(c),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) closeLiquidation(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.liquidations == nil {
		writeError(c, fmt.Errorf("%w: liquidation admin operations disabled", errorsx.ErrForbidden))
		return
	}
	var req closeLiquidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, fmt.Errorf("%w: invalid liquidation close request", errorsx.ErrInvalidArgument))
		return
	}
	item, err := h.liquidations.ClosePendingLiquidation(c.Request.Context(), AdminLiquidationCloseInput{
		LiquidationID: strings.TrimSpace(c.Param("liquidationId")),
		OperatorID:    strings.ToLower(addressFromContext(c)),
		TraceID:       traceIDFromContext(c),
		Reason:        strings.TrimSpace(req.Reason),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, item)
}

func (h *AdminHandler) listFundingBatches(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "200")))
	items, err := h.reader.ListFundingBatches(c.Request.Context(), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}

func (h *AdminHandler) reverseFundingBatch(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.funding == nil {
		writeError(c, fmt.Errorf("%w: funding reversal disabled", errorsx.ErrForbidden))
		return
	}
	var req reverseFundingBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, fmt.Errorf("%w: invalid funding reversal request", errorsx.ErrInvalidArgument))
		return
	}
	result, err := h.funding.ReverseFundingBatch(
		c.Request.Context(),
		strings.TrimSpace(c.Param("fundingBatchId")),
		strings.ToLower(addressFromContext(c)),
		traceIDFromContext(c),
		strings.TrimSpace(req.Reason),
	)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, result)
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

func (h *AdminHandler) returnWithdrawalToReview(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	address := strings.ToLower(addressFromContext(c))
	if _, ok := h.adminWallets[address]; !ok {
		writeError(c, fmt.Errorf("%w: admin wallet required", errorsx.ErrForbidden))
		return
	}
	withdrawID := c.Param("withdrawId")
	if err := h.approver.ReturnWithdrawToReview(c.Request.Context(), walletdomain.ReturnWithdrawToReviewInput{
		WithdrawID: withdrawID,
		OperatorID: address,
		TraceID:    traceIDFromContext(c),
	}); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"withdraw_id": withdrawID, "status": walletdomain.StatusRiskReview})
}

func (h *AdminHandler) refundWithdrawal(c *gin.Context) {
	if !requireUser(c) {
		return
	}
	address := strings.ToLower(addressFromContext(c))
	if _, ok := h.adminWallets[address]; !ok {
		writeError(c, fmt.Errorf("%w: admin wallet required", errorsx.ErrForbidden))
		return
	}
	withdrawID := c.Param("withdrawId")
	if err := h.approver.RefundWithdraw(c.Request.Context(), walletdomain.RefundWithdrawInput{
		WithdrawID:     withdrawID,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		TraceID:        traceIDFromContext(c),
	}); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"withdraw_id": withdrawID, "status": walletdomain.StatusRefunded})
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

func (h *AdminHandler) topUpInsuranceFund(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.ledger == nil {
		writeError(c, fmt.Errorf("%w: insurance fund top-up disabled", errorsx.ErrForbidden))
		return
	}
	var req insuranceFundTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, fmt.Errorf("%w: invalid insurance fund top-up request", errorsx.ErrInvalidArgument))
		return
	}
	item, err := h.ledger.TopUpInsuranceFund(c.Request.Context(), AdminInsuranceFundTopUpInput{
		OperatorID:     strings.ToLower(addressFromContext(c)),
		TraceID:        traceIDFromContext(c),
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		Reason:         strings.TrimSpace(req.Reason),
		Asset:          strings.TrimSpace(req.Asset),
		Amount:         strings.TrimSpace(req.Amount),
		SourceAccount:  strings.TrimSpace(req.SourceAccount),
	})
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

func flattenRuntimeConfigPatch(req runtimeConfigPatchRequest) (map[string]json.RawMessage, map[string]map[string]json.RawMessage, error) {
	values := make(map[string]json.RawMessage)
	pairValues := make(map[string]map[string]json.RawMessage)
	appendValue := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		values[key] = raw
		return nil
	}
	appendPairValue := func(pair string, key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		pair = strings.ToUpper(strings.TrimSpace(pair))
		if pair == "" {
			return fmt.Errorf("%w: pair is required", errorsx.ErrInvalidArgument)
		}
		if _, ok := pairValues[pair]; !ok {
			pairValues[pair] = make(map[string]json.RawMessage)
		}
		pairValues[pair][key] = raw
		return nil
	}
	if req.Global.ReadOnly != nil {
		if err := appendValue("system.read_only", *req.Global.ReadOnly); err != nil {
			return nil, nil, err
		}
	}
	if req.Global.ReduceOnly != nil {
		if err := appendValue("system.reduce_only", *req.Global.ReduceOnly); err != nil {
			return nil, nil, err
		}
	}
	if req.Global.TraceHeaderRequired != nil {
		if err := appendValue("system.trace_header_required", *req.Global.TraceHeaderRequired); err != nil {
			return nil, nil, err
		}
	}
	if req.Market.TakerFeeRate != nil {
		if err := appendValue("market.taker_fee_rate", strings.TrimSpace(*req.Market.TakerFeeRate)); err != nil {
			return nil, nil, err
		}
	}
	if req.Market.MakerFeeRate != nil {
		if err := appendValue("market.maker_fee_rate", strings.TrimSpace(*req.Market.MakerFeeRate)); err != nil {
			return nil, nil, err
		}
	}
	if req.Market.DefaultMaxSlippageBps != nil {
		if err := appendValue("market.default_max_slippage_bps", *req.Market.DefaultMaxSlippageBps); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.GlobalBufferRatio != nil {
		if err := appendValue("risk.global_buffer_ratio", strings.TrimSpace(*req.Risk.GlobalBufferRatio)); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.MarkPriceStaleSec != nil {
		if err := appendValue("risk.mark_price_stale_sec", *req.Risk.MarkPriceStaleSec); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.ForceReduceOnlyOnStalePrice != nil {
		if err := appendValue("risk.force_reduce_only_on_stale_price", *req.Risk.ForceReduceOnlyOnStalePrice); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.LiquidationPenaltyRate != nil {
		if err := appendValue("risk.liquidation_penalty_rate", strings.TrimSpace(*req.Risk.LiquidationPenaltyRate)); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.MaintenanceMarginUpliftRatio != nil {
		if err := appendValue("risk.maintenance_margin_uplift_ratio", strings.TrimSpace(*req.Risk.MaintenanceMarginUpliftRatio)); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.LiquidationExtraSlippageBps != nil {
		if err := appendValue("risk.liquidation_extra_slippage_bps", *req.Risk.LiquidationExtraSlippageBps); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.MaxOpenOrdersPerUserPerSymbol != nil {
		if err := appendValue("risk.max_open_orders_per_user_per_symbol", *req.Risk.MaxOpenOrdersPerUserPerSymbol); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.NetExposureHardLimit != nil {
		if err := appendValue("risk.net_exposure_hard_limit", strings.TrimSpace(*req.Risk.NetExposureHardLimit)); err != nil {
			return nil, nil, err
		}
	}
	if req.Risk.MaxExposureSlippageBps != nil {
		if err := appendValue("risk.max_exposure_slippage_bps", *req.Risk.MaxExposureSlippageBps); err != nil {
			return nil, nil, err
		}
	}
	if req.Funding.IntervalSec != nil {
		if err := appendValue("funding.interval_sec", *req.Funding.IntervalSec); err != nil {
			return nil, nil, err
		}
	}
	if req.Funding.SourcePollIntervalSec != nil {
		if err := appendValue("funding.source_poll_interval_sec", *req.Funding.SourcePollIntervalSec); err != nil {
			return nil, nil, err
		}
	}
	if req.Funding.CapRatePerHour != nil {
		if err := appendValue("funding.cap_rate_per_hour", strings.TrimSpace(*req.Funding.CapRatePerHour)); err != nil {
			return nil, nil, err
		}
	}
	if req.Funding.MinValidSourceCount != nil {
		if err := appendValue("funding.min_valid_source_count", *req.Funding.MinValidSourceCount); err != nil {
			return nil, nil, err
		}
	}
	if req.Funding.DefaultModelCrypto != nil {
		if err := appendValue("funding.default_model_crypto", strings.TrimSpace(*req.Funding.DefaultModelCrypto)); err != nil {
			return nil, nil, err
		}
	}
	if req.Hedge.Enabled != nil {
		if err := appendValue("hedge.enabled", *req.Hedge.Enabled); err != nil {
			return nil, nil, err
		}
	}
	if req.Hedge.SoftThresholdRatio != nil {
		if err := appendValue("hedge.soft_threshold_ratio", strings.TrimSpace(*req.Hedge.SoftThresholdRatio)); err != nil {
			return nil, nil, err
		}
	}
	if req.Hedge.HardThresholdRatio != nil {
		if err := appendValue("hedge.hard_threshold_ratio", strings.TrimSpace(*req.Hedge.HardThresholdRatio)); err != nil {
			return nil, nil, err
		}
	}
	for pair, patch := range req.Pairs {
		if patch.Market.MaxLeverage != nil {
			if err := appendPairValue(pair, "market.max_leverage", strings.TrimSpace(*patch.Market.MaxLeverage)); err != nil {
				return nil, nil, err
			}
		}
		if patch.Market.SessionPolicy != nil {
			if err := appendPairValue(pair, "market.session_policy", strings.TrimSpace(*patch.Market.SessionPolicy)); err != nil {
				return nil, nil, err
			}
		}
		if patch.Market.TakerFeeRate != nil {
			if err := appendPairValue(pair, "market.taker_fee_rate", strings.TrimSpace(*patch.Market.TakerFeeRate)); err != nil {
				return nil, nil, err
			}
		}
		if patch.Market.MakerFeeRate != nil {
			if err := appendPairValue(pair, "market.maker_fee_rate", strings.TrimSpace(*patch.Market.MakerFeeRate)); err != nil {
				return nil, nil, err
			}
		}
		if patch.Market.DefaultMaxSlippageBps != nil {
			if err := appendPairValue(pair, "market.default_max_slippage_bps", *patch.Market.DefaultMaxSlippageBps); err != nil {
				return nil, nil, err
			}
		}
		if patch.Risk.LiquidationPenaltyRate != nil {
			if err := appendPairValue(pair, "risk.liquidation_penalty_rate", strings.TrimSpace(*patch.Risk.LiquidationPenaltyRate)); err != nil {
				return nil, nil, err
			}
		}
		if patch.Risk.MaintenanceMarginUpliftRatio != nil {
			if err := appendPairValue(pair, "risk.maintenance_margin_uplift_ratio", strings.TrimSpace(*patch.Risk.MaintenanceMarginUpliftRatio)); err != nil {
				return nil, nil, err
			}
		}
		if patch.Funding.IntervalSec != nil {
			if err := appendPairValue(pair, "funding.interval_sec", *patch.Funding.IntervalSec); err != nil {
				return nil, nil, err
			}
		}
	}
	if len(values) == 0 && len(pairValues) == 0 {
		return nil, nil, fmt.Errorf("%w: no runtime config change detected", errorsx.ErrInvalidArgument)
	}
	return values, pairValues, nil
}
