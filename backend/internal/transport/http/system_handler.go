package httptransport

import (
	"context"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type SystemReader interface {
	ListChains(ctx context.Context) ([]readmodel.SystemChainItem, error)
}

type StaticSystemReader struct {
	chains []readmodel.SystemChainItem
}

func NewStaticSystemReader(chains []readmodel.SystemChainItem) *StaticSystemReader {
	return &StaticSystemReader{chains: chains}
}

func (r *StaticSystemReader) ListChains(_ context.Context) ([]readmodel.SystemChainItem, error) {
	out := make([]readmodel.SystemChainItem, len(r.chains))
	copy(out, r.chains)
	return out, nil
}

type SystemHandler struct {
	reader SystemReader
}

func NewSystemHandler(reader SystemReader) *SystemHandler {
	return &SystemHandler{reader: reader}
}

func (h *SystemHandler) Register(r gin.IRoutes) {
	r.GET("/system/chains", h.listChains)
}

func (h *SystemHandler) listChains(c *gin.Context) {
	items, err := h.reader.ListChains(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, items)
}
