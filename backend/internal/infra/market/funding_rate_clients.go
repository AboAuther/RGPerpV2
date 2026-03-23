package marketinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
)

type BinanceFundingRateClient struct {
	baseURL string
	client  HTTPClient
}

func NewBinanceFundingRateClient(client HTTPClient) *BinanceFundingRateClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &BinanceFundingRateClient{
		baseURL: "https://fapi.binance.com",
		client:  client,
	}
}

func (c *BinanceFundingRateClient) Name() string { return "binance" }

func (c *BinanceFundingRateClient) FetchRates(ctx context.Context, symbols []fundingdomain.SourceRateRequest) (map[string]fundingdomain.SourceRateQuote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/fapi/v1/premiumIndex", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance premiumIndex status: %d", resp.StatusCode)
	}

	var payload []struct {
		Symbol          string `json:"symbol"`
		LastFundingRate string `json:"lastFundingRate"`
		Time            int64  `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		requested[symbol.SourceSymbol] = struct{}{}
	}
	now := time.Now().UTC()
	out := make(map[string]fundingdomain.SourceRateQuote, len(symbols))
	for _, item := range payload {
		if _, ok := requested[item.Symbol]; !ok || item.LastFundingRate == "" {
			continue
		}
		sourceTS := now
		if item.Time > 0 {
			sourceTS = time.UnixMilli(item.Time).UTC()
		}
		out[item.Symbol] = fundingdomain.SourceRateQuote{
			SourceName:      "binance",
			SourceSymbol:    item.Symbol,
			Rate:            item.LastFundingRate,
			IntervalSeconds: 8 * 3600,
			SourceTS:        sourceTS,
			ReceivedTS:      now,
		}
	}
	return out, nil
}

type HyperliquidFundingRateClient struct {
	baseURL string
	client  HTTPClient
}

func NewHyperliquidFundingRateClient(client HTTPClient) *HyperliquidFundingRateClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HyperliquidFundingRateClient{
		baseURL: "https://api.hyperliquid.xyz",
		client:  client,
	}
}

func (c *HyperliquidFundingRateClient) Name() string { return "hyperliquid" }

func (c *HyperliquidFundingRateClient) FetchRates(ctx context.Context, symbols []fundingdomain.SourceRateRequest) (map[string]fundingdomain.SourceRateQuote, error) {
	body, _ := json.Marshal(map[string]any{"type": "metaAndAssetCtxs"})
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
		return nil, fmt.Errorf("hyperliquid metaAndAssetCtxs status: %d", resp.StatusCode)
	}

	var payload []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload) != 2 {
		return nil, fmt.Errorf("hyperliquid funding payload length: %d", len(payload))
	}

	var meta struct {
		Universe []struct {
			Name string `json:"name"`
		} `json:"universe"`
	}
	if err := json.Unmarshal(payload[0], &meta); err != nil {
		return nil, err
	}
	var assetCtxs []struct {
		Funding string `json:"funding"`
	}
	if err := json.Unmarshal(payload[1], &assetCtxs); err != nil {
		return nil, err
	}

	indexBySymbol := make(map[string]int, len(meta.Universe))
	for idx, item := range meta.Universe {
		indexBySymbol[item.Name] = idx
	}
	now := time.Now().UTC()
	out := make(map[string]fundingdomain.SourceRateQuote, len(symbols))
	for _, symbol := range symbols {
		idx, ok := indexBySymbol[symbol.SourceSymbol]
		if !ok || idx >= len(assetCtxs) || assetCtxs[idx].Funding == "" {
			continue
		}
		out[symbol.SourceSymbol] = fundingdomain.SourceRateQuote{
			SourceName:      "hyperliquid",
			SourceSymbol:    symbol.SourceSymbol,
			Rate:            assetCtxs[idx].Funding,
			IntervalSeconds: 3600,
			SourceTS:        now,
			ReceivedTS:      now,
		}
	}
	return out, nil
}
