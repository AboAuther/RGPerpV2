package marketinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type BinanceBookTickerClient struct {
	baseURL string
	client  HTTPClient
}

func NewBinanceBookTickerClient(client HTTPClient) *BinanceBookTickerClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &BinanceBookTickerClient{
		baseURL: "https://fapi.binance.com",
		client:  client,
	}
}

func (c *BinanceBookTickerClient) Name() string { return "binance" }

func (c *BinanceBookTickerClient) Fetch(ctx context.Context, symbols []marketdomain.SourceSymbolRequest) (map[string]marketdomain.SourceQuote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/fapi/v1/ticker/bookTicker", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload []struct {
		Symbol   string `json:"symbol"`
		BidPrice string `json:"bidPrice"`
		AskPrice string `json:"askPrice"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		requested[symbol.SourceSymbol] = struct{}{}
	}
	now := time.Now().UTC()
	out := make(map[string]marketdomain.SourceQuote, len(symbols))
	for _, item := range payload {
		if _, ok := requested[item.Symbol]; !ok {
			continue
		}
		last := item.BidPrice
		if item.AskPrice != "" {
			last = item.AskPrice
		}
		out[item.Symbol] = marketdomain.SourceQuote{
			SourceName:   "binance",
			SourceSymbol: item.Symbol,
			Bid:          item.BidPrice,
			Ask:          item.AskPrice,
			Last:         last,
			SourceTS:     now,
			ReceivedTS:   now,
		}
	}
	return out, nil
}

type HyperliquidAllMidsClient struct {
	baseURL string
	client  HTTPClient
}

func NewHyperliquidAllMidsClient(client HTTPClient) *HyperliquidAllMidsClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HyperliquidAllMidsClient{
		baseURL: "https://api.hyperliquid.xyz",
		client:  client,
	}
}

func (c *HyperliquidAllMidsClient) Name() string { return "hyperliquid" }

func (c *HyperliquidAllMidsClient) Fetch(ctx context.Context, symbols []marketdomain.SourceSymbolRequest) (map[string]marketdomain.SourceQuote, error) {
	body, _ := json.Marshal(map[string]any{"type": "allMids"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/info", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("hyperliquid allMids status: %d", resp.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		requested[symbol.SourceSymbol] = struct{}{}
	}
	now := time.Now().UTC()
	out := make(map[string]marketdomain.SourceQuote, len(symbols))
	for symbol, mid := range payload {
		if _, ok := requested[symbol]; !ok {
			continue
		}
		out[symbol] = marketdomain.SourceQuote{
			SourceName:   "hyperliquid",
			SourceSymbol: symbol,
			Bid:          mid,
			Ask:          mid,
			Last:         mid,
			SourceTS:     now,
			ReceivedTS:   now,
		}
	}
	return out, nil
}
