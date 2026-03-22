package httptransport

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
)

type AuthUseCase interface {
	IssueChallenge(ctx context.Context, input authdomain.IssueChallengeInput) (authdomain.IssueChallengeOutput, error)
	Login(ctx context.Context, input authdomain.LoginInput) (authdomain.LoginResult, error)
}

type AuthHandler struct {
	authUC       AuthUseCase
	adminWallets map[string]struct{}
}

func NewAuthHandler(authUC AuthUseCase, adminWallets []string) *AuthHandler {
	allow := make(map[string]struct{}, len(adminWallets))
	for _, wallet := range adminWallets {
		allow[strings.ToLower(strings.TrimSpace(wallet))] = struct{}{}
	}
	return &AuthHandler{authUC: authUC, adminWallets: allow}
}

type issueChallengeRequest struct {
	Address string `json:"address"`
	ChainID int64  `json:"chain_id"`
}

type loginRequest struct {
	Address           string `json:"address"`
	ChainID           int64  `json:"chain_id"`
	Nonce             string `json:"nonce"`
	Signature         string `json:"signature"`
	DeviceFingerprint string `json:"device_fingerprint"`
	IP                string `json:"ip"`
	UserAgent         string `json:"user_agent"`
}

func (h *AuthHandler) Register(r gin.IRoutes) {
	r.POST("/auth/challenge", h.issueChallenge)
	r.POST("/auth/nonce", h.issueChallenge)
	r.POST("/auth/login", h.login)
}

func (h *AuthHandler) issueChallenge(c *gin.Context) {
	var req issueChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}

	resp, err := h.authUC.IssueChallenge(c.Request.Context(), authdomain.IssueChallengeInput{
		Address: req.Address,
		ChainID: req.ChainID,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	writeOK(c, gin.H{
		"nonce":      resp.Nonce,
		"message":    resp.Message,
		"domain":     resp.Domain,
		"chain_id":   resp.ChainID,
		"expires_at": resp.ExpiresAt,
	})
}

func (h *AuthHandler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, err)
		return
	}

	// Prefer request metadata when explicit fields are absent.
	if req.IP == "" {
		req.IP = c.ClientIP()
	}
	if req.UserAgent == "" {
		req.UserAgent = c.Request.UserAgent()
	}

	resp, err := h.authUC.Login(c.Request.Context(), authdomain.LoginInput{
		Address:           req.Address,
		ChainID:           req.ChainID,
		Nonce:             req.Nonce,
		Signature:         req.Signature,
		DeviceFingerprint: req.DeviceFingerprint,
		IP:                req.IP,
		UserAgent:         req.UserAgent,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	writeOK(c, gin.H{
		"access_token":  resp.AccessToken,
		"refresh_token": resp.RefreshToken,
		"expires_at":    resp.ExpiresAt,
		"user": gin.H{
			"id":          resp.User.ID,
			"evm_address": resp.User.EVMAddress,
			"status":      resp.User.Status,
			"is_admin":    h.isAdminWallet(resp.User.EVMAddress),
		},
	})
}

func (h *AuthHandler) isAdminWallet(address string) bool {
	if h == nil {
		return false
	}
	_, ok := h.adminWallets[strings.ToLower(strings.TrimSpace(address))]
	return ok
}

func NewEngine(
	verifier AccessVerifier,
	authHandler *AuthHandler,
	marketHandler *MarketHandler,
	accountHandler *AccountHandler,
	walletHandler *WalletHandler,
	tradingHandler *TradingHandler,
	explorerHandler *ExplorerHandler,
	adminHandler *AdminHandler,
	systemHandlers ...*SystemHandler,
) *gin.Engine {
	engine := gin.New()
	engine.Use(TraceMiddleware())
	engine.Use(CORSMiddleware())
	engine.Use(gin.Recovery())

	engine.GET("/healthz", func(c *gin.Context) {
		writeOK(c, gin.H{"status": http.StatusText(http.StatusOK)})
	})

	v1 := engine.Group("/api/v1")
	if len(systemHandlers) > 0 && systemHandlers[0] != nil {
		systemHandlers[0].Register(v1)
	}
	if authHandler != nil {
		authHandler.Register(v1)
	}
	if marketHandler != nil {
		marketHandler.Register(v1)
	}
	authed := v1.Group("")
	authed.Use(AuthMiddleware(verifier))
	if accountHandler != nil {
		accountHandler.Register(authed)
	}
	if walletHandler != nil {
		walletHandler.Register(authed)
	}
	if tradingHandler != nil {
		tradingHandler.Register(authed)
	}
	if explorerHandler != nil {
		explorerHandler.Register(authed)
	}
	if adminHandler != nil {
		adminHandler.Register(authed)
	}
	return engine
}
