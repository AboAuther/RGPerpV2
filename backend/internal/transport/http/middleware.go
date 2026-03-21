package httptransport

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	contextKeyUserID    = "user_id"
	contextKeyTraceID   = "trace_id"
	contextKeyAddress   = "evm_address"
	defaultTraceIDLabel = "trace_generated"
)

type AccessVerifier interface {
	VerifyAccessToken(token string) (AccessClaims, error)
}

type AccessClaims struct {
	UserID    string
	Address   string
	SessionID string
}

func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := strings.TrimSpace(c.GetHeader("X-Trace-Id"))
		if traceID == "" {
			traceID = defaultTraceIDLabel
		}
		c.Set(contextKeyTraceID, traceID)
		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Trace-Id")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Max-Age", "600")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func AuthMiddleware(verifier AccessVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil {
			writeError(c, errorsx.ErrUnauthorized)
			c.Abort()
			return
		}
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
		claims, err := verifier.VerifyAccessToken(token)
		if err != nil {
			writeError(c, err)
			c.Abort()
			return
		}
		userID, err := strconv.ParseUint(claims.UserID, 10, 64)
		if err != nil {
			writeError(c, errorsx.ErrUnauthorized)
			c.Abort()
			return
		}
		c.Set(contextKeyUserID, userID)
		c.Set(contextKeyAddress, claims.Address)
		c.Next()
	}
}

func userIDFromContext(c *gin.Context) uint64 {
	value, ok := c.Get(contextKeyUserID)
	if !ok {
		return 0
	}
	userID, _ := value.(uint64)
	return userID
}

func addressFromContext(c *gin.Context) string {
	value, ok := c.Get(contextKeyAddress)
	if !ok {
		return ""
	}
	address, _ := value.(string)
	return address
}

func requireUser(c *gin.Context) bool {
	if userIDFromContext(c) == 0 {
		c.JSON(http.StatusUnauthorized, apiResponse{
			Code:    "UNAUTHORIZED",
			Message: "unauthorized",
			TraceID: traceIDFromContext(c),
		})
		return false
	}
	return true
}
