package httptransport

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type apiResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	TraceID string      `json:"trace_id"`
	Data    interface{} `json:"data,omitempty"`
}

func writeOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, apiResponse{
		Code:    "OK",
		Message: "ok",
		TraceID: traceIDFromContext(c),
		Data:    data,
	})
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := err.Error()

	switch {
	case errors.Is(err, errorsx.ErrInvalidArgument):
		status = http.StatusBadRequest
		code = "INVALID_ARGUMENT"
	case errors.Is(err, errorsx.ErrUnauthorized), errors.Is(err, errorsx.ErrExpired):
		status = http.StatusUnauthorized
		code = "UNAUTHORIZED"
	case errors.Is(err, errorsx.ErrForbidden):
		status = http.StatusForbidden
		code = "FORBIDDEN"
	case errors.Is(err, errorsx.ErrNotFound):
		status = http.StatusNotFound
		code = "NOT_FOUND"
	case errors.Is(err, errorsx.ErrConflict):
		status = http.StatusConflict
		code = "CONFLICT"
	}

	c.JSON(status, apiResponse{
		Code:    code,
		Message: message,
		TraceID: traceIDFromContext(c),
	})
}

func traceIDFromContext(c *gin.Context) string {
	traceID := c.GetHeader("X-Trace-Id")
	if traceID == "" {
		traceID = c.GetString("trace_id")
	}
	return traceID
}
