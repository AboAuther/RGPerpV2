package httptransport

import (
	"context"

	"github.com/gin-gonic/gin"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type MarketReader interface {
	ListSymbols(ctx context.Context) ([]readmodel.SymbolItem, error)
	ListTickers(ctx context.Context) ([]readmodel.TickerItem, error)
	ListFundingQuotes(ctx context.Context) ([]readmodel.FundingQuoteItem, error)
}

type MarketHandler struct {
	reader MarketReader
}

func NewMarketHandler(reader MarketReader) *MarketHandler {
	return &MarketHandler{reader: reader}
}

func (h *MarketHandler) Register(r gin.IRoutes) {
	r.GET("/markets/symbols", h.getSymbols)
	r.GET("/markets/tickers", h.getTickers)
	r.GET("/markets/funding", h.getFundingQuotes)
}

func (h *MarketHandler) getSymbols(c *gin.Context) {
	data, err := h.reader.ListSymbols(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *MarketHandler) getTickers(c *gin.Context) {
	data, err := h.reader.ListTickers(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}

func (h *MarketHandler) getFundingQuotes(c *gin.Context) {
	data, err := h.reader.ListFundingQuotes(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, data)
}
