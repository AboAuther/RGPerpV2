package marketinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type BinanceTicker24hrClient struct {
	baseURL string
	client  HTTPClient
}

type binance24hrTicker struct {
	LastPrice   string
	QuoteVolume string
	CloseTime   int64
}

type binanceBookTicker struct {
	BidPrice string
	AskPrice string
}

func NewBinanceTicker24hrClient(client HTTPClient) *BinanceTicker24hrClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &BinanceTicker24hrClient{
		baseURL: "https://fapi.binance.com",
		client:  client,
	}
}

func NewBinanceBookTickerClient(client HTTPClient) *BinanceTicker24hrClient {
	return NewBinanceTicker24hrClient(client)
}

func (c *BinanceTicker24hrClient) Name() string { return "binance" }

func (c *BinanceTicker24hrClient) Fetch(ctx context.Context, symbols []marketdomain.SourceSymbolRequest) (map[string]marketdomain.SourceQuote, error) {
	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		requested[symbol.SourceSymbol] = struct{}{}
	}
	if len(requested) == 0 {
		return map[string]marketdomain.SourceQuote{}, nil
	}

	ticker24h, err := c.fetch24hrQuotes(ctx)
	if err != nil {
		return nil, err
	}
	bookTickers, err := c.fetchBookTickers(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	out := make(map[string]marketdomain.SourceQuote, len(symbols))
	for symbol, ticker := range ticker24h {
		if _, ok := requested[symbol]; !ok {
			continue
		}
		bookTicker, ok := bookTickers[symbol]
		if !ok || strings.TrimSpace(bookTicker.BidPrice) == "" || strings.TrimSpace(bookTicker.AskPrice) == "" {
			continue
		}
		sourceTS := now
		if ticker.CloseTime > 0 {
			sourceTS = time.UnixMilli(ticker.CloseTime).UTC()
		}
		out[symbol] = marketdomain.SourceQuote{
			SourceName:   "binance",
			SourceSymbol: symbol,
			Bid:          bookTicker.BidPrice,
			Ask:          bookTicker.AskPrice,
			Last:         ticker.LastPrice,
			QuoteVolume:  ticker.QuoteVolume,
			SourceTS:     sourceTS,
			ReceivedTS:   now,
		}
	}
	return out, nil
}

func (c *BinanceTicker24hrClient) fetch24hrQuotes(ctx context.Context) (map[string]binance24hrTicker, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/fapi/v1/ticker/24hr", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance 24hr ticker status: %d", resp.StatusCode)
	}

	var payload []struct {
		Symbol      string `json:"symbol"`
		LastPrice   string `json:"lastPrice"`
		QuoteVolume string `json:"quoteVolume"`
		CloseTime   int64  `json:"closeTime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make(map[string]binance24hrTicker, len(payload))
	for _, item := range payload {
		out[item.Symbol] = binance24hrTicker{
			LastPrice:   item.LastPrice,
			QuoteVolume: item.QuoteVolume,
			CloseTime:   item.CloseTime,
		}
	}
	return out, nil
}

func (c *BinanceTicker24hrClient) fetchBookTickers(ctx context.Context) (map[string]binanceBookTicker, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/fapi/v1/ticker/bookTicker", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance bookTicker status: %d", resp.StatusCode)
	}

	var payload []struct {
		Symbol   string `json:"symbol"`
		BidPrice string `json:"bidPrice"`
		AskPrice string `json:"askPrice"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make(map[string]binanceBookTicker, len(payload))
	for _, item := range payload {
		out[item.Symbol] = binanceBookTicker{
			BidPrice: item.BidPrice,
			AskPrice: item.AskPrice,
		}
	}
	return out, nil
}

type HyperliquidMetaClient struct {
	baseURL string
	client  HTTPClient
}

func NewHyperliquidMetaClient(client HTTPClient) *HyperliquidMetaClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HyperliquidMetaClient{
		baseURL: "https://api.hyperliquid.xyz",
		client:  client,
	}
}

func NewHyperliquidAllMidsClient(client HTTPClient) *HyperliquidMetaClient {
	return NewHyperliquidMetaClient(client)
}

func (c *HyperliquidMetaClient) Name() string { return "hyperliquid" }

func (c *HyperliquidMetaClient) Fetch(ctx context.Context, symbols []marketdomain.SourceSymbolRequest) (map[string]marketdomain.SourceQuote, error) {
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
		return nil, fmt.Errorf("hyperliquid metaAndAssetCtxs payload length: %d", len(payload))
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
		MidPx        *string `json:"midPx"`
		MarkPx       string  `json:"markPx"`
		OraclePx     string  `json:"oraclePx"`
		DayNtlVlm    string  `json:"dayNtlVlm"`
		Funding      string  `json:"funding"`
		OpenInterest string  `json:"openInterest"`
	}
	if err := json.Unmarshal(payload[1], &assetCtxs); err != nil {
		return nil, err
	}

	indexBySymbol := make(map[string]int, len(meta.Universe))
	for idx, item := range meta.Universe {
		indexBySymbol[item.Name] = idx
	}

	now := time.Now().UTC()
	out := make(map[string]marketdomain.SourceQuote, len(symbols))
	for _, symbol := range symbols {
		idx, ok := indexBySymbol[symbol.SourceSymbol]
		if !ok || idx >= len(assetCtxs) {
			continue
		}
		ctxItem := assetCtxs[idx]
		mid := firstNonEmpty(ptrString(ctxItem.MidPx), ctxItem.MarkPx, ctxItem.OraclePx)
		if mid == "" {
			continue
		}
		out[symbol.SourceSymbol] = marketdomain.SourceQuote{
			SourceName:   "hyperliquid",
			SourceSymbol: symbol.SourceSymbol,
			Bid:          mid,
			Ask:          mid,
			Last:         mid,
			QuoteVolume:  ctxItem.DayNtlVlm,
			SourceTS:     now,
			ReceivedTS:   now,
		}
	}
	return out, nil
}

type CoinbaseProductTickerClient struct {
	baseURL     string
	client      HTTPClient
	concurrency int
}

func NewCoinbaseProductTickerClient(client HTTPClient) *CoinbaseProductTickerClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &CoinbaseProductTickerClient{
		baseURL:     "https://api.exchange.coinbase.com",
		client:      client,
		concurrency: 8,
	}
}

func (c *CoinbaseProductTickerClient) Name() string { return "coinbase" }

func (c *CoinbaseProductTickerClient) Fetch(ctx context.Context, symbols []marketdomain.SourceSymbolRequest) (map[string]marketdomain.SourceQuote, error) {
	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		if symbol.SourceSymbol == "" {
			continue
		}
		requested[symbol.SourceSymbol] = struct{}{}
	}
	if len(requested) == 0 {
		return map[string]marketdomain.SourceQuote{}, nil
	}

	type result struct {
		symbol string
		quote  marketdomain.SourceQuote
		err    error
	}

	out := make(map[string]marketdomain.SourceQuote, len(requested))
	results := make(chan result, len(requested))
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup

	for symbol := range requested {
		wg.Add(1)
		go func(sourceSymbol string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			quote, err := c.fetchProductTicker(ctx, sourceSymbol)
			results <- result{symbol: sourceSymbol, quote: quote, err: err}
		}(symbol)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var firstErr error
	for item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		if item.quote.Bid == "" && item.quote.Ask == "" && item.quote.Last == "" {
			continue
		}
		out[item.symbol] = item.quote
	}

	if len(out) > 0 {
		return out, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (c *CoinbaseProductTickerClient) fetchProductTicker(ctx context.Context, sourceSymbol string) (marketdomain.SourceQuote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/products/"+url.PathEscape(sourceSymbol)+"/ticker", nil)
	if err != nil {
		return marketdomain.SourceQuote{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return marketdomain.SourceQuote{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return marketdomain.SourceQuote{}, nil
	}
	if resp.StatusCode >= 400 {
		return marketdomain.SourceQuote{}, fmt.Errorf("coinbase product ticker %s status: %d", sourceSymbol, resp.StatusCode)
	}

	var payload struct {
		Ask    string `json:"ask"`
		Bid    string `json:"bid"`
		Price  string `json:"price"`
		Volume string `json:"volume"`
		Time   string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return marketdomain.SourceQuote{}, err
	}
	if payload.Price == "" && payload.Bid == "" && payload.Ask == "" {
		return marketdomain.SourceQuote{}, nil
	}
	now := time.Now().UTC()
	sourceTS := now
	if payload.Time != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, payload.Time); err == nil {
			sourceTS = parsed.UTC()
		}
	}
	return marketdomain.SourceQuote{
		SourceName:   "coinbase",
		SourceSymbol: sourceSymbol,
		Bid:          payload.Bid,
		Ask:          payload.Ask,
		Last:         payload.Price,
		QuoteVolume:  toQuoteVolume(payload.Price, payload.Volume),
		SourceTS:     sourceTS,
		ReceivedTS:   now,
	}, nil
}

type TwelveDataQuoteClient struct {
	baseURL     string
	client      HTTPClient
	apiKey      string
	concurrency int
}

func NewTwelveDataQuoteClient(apiKey string, client HTTPClient) *TwelveDataQuoteClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &TwelveDataQuoteClient{
		baseURL:     "https://api.twelvedata.com",
		client:      client,
		apiKey:      strings.TrimSpace(apiKey),
		concurrency: 8,
	}
}

func (c *TwelveDataQuoteClient) Name() string { return "twelvedata" }

func (c *TwelveDataQuoteClient) Fetch(ctx context.Context, symbols []marketdomain.SourceSymbolRequest) (map[string]marketdomain.SourceQuote, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("twelvedata api key is required")
	}
	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		if strings.TrimSpace(symbol.SourceSymbol) == "" {
			continue
		}
		requested[symbol.SourceSymbol] = struct{}{}
	}
	if len(requested) == 0 {
		return map[string]marketdomain.SourceQuote{}, nil
	}

	type result struct {
		symbol string
		quote  marketdomain.SourceQuote
		err    error
	}

	out := make(map[string]marketdomain.SourceQuote, len(requested))
	results := make(chan result, len(requested))
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup

	for symbol := range requested {
		wg.Add(1)
		go func(sourceSymbol string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			quote, err := c.fetchQuote(ctx, sourceSymbol)
			results <- result{symbol: sourceSymbol, quote: quote, err: err}
		}(symbol)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var firstErr error
	for item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		out[item.symbol] = item.quote
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (c *TwelveDataQuoteClient) fetchQuote(ctx context.Context, sourceSymbol string) (marketdomain.SourceQuote, error) {
	query := url.Values{}
	query.Set("symbol", sourceSymbol)
	query.Set("apikey", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/quote?"+query.Encode(), nil)
	if err != nil {
		return marketdomain.SourceQuote{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return marketdomain.SourceQuote{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return marketdomain.SourceQuote{}, fmt.Errorf("twelvedata quote %s status: %d", sourceSymbol, resp.StatusCode)
	}

	var payload struct {
		Code          int    `json:"code"`
		Status        string `json:"status"`
		Message       string `json:"message"`
		Close         string `json:"close"`
		Price         string `json:"price"`
		Volume        string `json:"volume"`
		Datetime      string `json:"datetime"`
		Timestamp     int64  `json:"timestamp"`
		PreviousClose string `json:"previous_close"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return marketdomain.SourceQuote{}, err
	}
	if payload.Code != 0 || strings.EqualFold(payload.Status, "error") {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = "quote request failed"
		}
		return marketdomain.SourceQuote{}, fmt.Errorf("twelvedata %s: %s", sourceSymbol, message)
	}
	last := firstNonEmpty(payload.Close, payload.Price, payload.PreviousClose)
	if last == "" {
		return marketdomain.SourceQuote{}, fmt.Errorf("twelvedata %s: missing last price", sourceSymbol)
	}
	now := time.Now().UTC()
	sourceTS := now
	if payload.Timestamp > 0 {
		sourceTS = time.Unix(payload.Timestamp, 0).UTC()
	} else if parsed, ok := parseTwelveDataTime(payload.Datetime); ok {
		sourceTS = parsed
	}
	return marketdomain.SourceQuote{
		SourceName:   "twelvedata",
		SourceSymbol: sourceSymbol,
		Bid:          last,
		Ask:          last,
		Last:         last,
		QuoteVolume:  payload.Volume,
		SourceTS:     sourceTS,
		ReceivedTS:   now,
	}, nil
}

func parseTwelveDataTime(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func toQuoteVolume(price string, baseVolume string) string {
	if price == "" || baseVolume == "" {
		return ""
	}
	px, err := decimalx.NewFromString(price)
	if err != nil {
		return ""
	}
	volume, err := decimalx.NewFromString(baseVolume)
	if err != nil {
		return ""
	}
	if !px.GreaterThan(decimalx.MustFromString("0")) || !volume.GreaterThan(decimalx.MustFromString("0")) {
		return ""
	}
	return px.Mul(volume).String()
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
