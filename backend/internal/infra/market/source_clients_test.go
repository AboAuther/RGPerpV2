package marketinfra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
)

func TestBinanceTicker24hrClientFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fapi/v1/ticker/24hr":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDC","lastPrice":"100.5","quoteVolume":"123456","closeTime":1710000000000}]`))
		case "/fapi/v1/ticker/bookTicker":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDC","bidPrice":"100","askPrice":"101"}]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewBinanceTicker24hrClient(server.Client())
	client.baseURL = server.URL

	quotes, err := client.Fetch(context.Background(), []marketdomain.SourceSymbolRequest{{SourceSymbol: "BTCUSDC"}})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if quotes["BTCUSDC"].QuoteVolume != "123456" {
		t.Fatalf("unexpected quote volume: %s", quotes["BTCUSDC"].QuoteVolume)
	}
	if quotes["BTCUSDC"].Bid != "100" || quotes["BTCUSDC"].Ask != "101" {
		t.Fatalf("unexpected bid/ask: %+v", quotes["BTCUSDC"])
	}
}

func TestHyperliquidMetaClientFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"universe":[{"name":"BTC"},{"name":"ETH"}]},
			[
				{"midPx":"100","markPx":"100","oraclePx":"100","dayNtlVlm":"2000"},
				{"midPx":"200","markPx":"200","oraclePx":"200","dayNtlVlm":"3000"}
			]
		]`))
	}))
	defer server.Close()

	client := NewHyperliquidMetaClient(server.Client())
	client.baseURL = server.URL

	quotes, err := client.Fetch(context.Background(), []marketdomain.SourceSymbolRequest{{SourceSymbol: "BTC"}})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if quotes["BTC"].Last != "100" {
		t.Fatalf("unexpected hyperliquid last: %s", quotes["BTC"].Last)
	}
	if quotes["BTC"].QuoteVolume != "2000" {
		t.Fatalf("unexpected hyperliquid volume: %s", quotes["BTC"].QuoteVolume)
	}
}

func TestCoinbaseProductTickerClientFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/products/BTC-USD/ticker":
			_, _ = w.Write([]byte(`{"ask":"101","bid":"100","price":"100.5","volume":"10","time":"2026-03-22T13:32:43.053507699Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewCoinbaseProductTickerClient(server.Client())
	client.baseURL = server.URL
	client.concurrency = 1

	quotes, err := client.Fetch(context.Background(), []marketdomain.SourceSymbolRequest{{SourceSymbol: "BTC-USD"}})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	quote, ok := quotes["BTC-USD"]
	if !ok {
		t.Fatal("expected BTC-USD quote")
	}
	if quote.QuoteVolume != "1005" {
		t.Fatalf("unexpected coinbase quote volume: %s", quote.QuoteVolume)
	}
	if quote.SourceTS.IsZero() || quote.SourceTS.Before(time.Date(2026, 3, 22, 13, 32, 43, 0, time.UTC)) {
		t.Fatalf("unexpected source ts: %s", quote.SourceTS)
	}
}

func TestTwelveDataQuoteClientFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quote" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "AAPL" {
			t.Fatalf("unexpected symbol: %s", got)
		}
		if got := r.URL.Query().Get("apikey"); got != "test-key" {
			t.Fatalf("unexpected api key: %s", got)
		}
		_, _ = w.Write([]byte(`{"symbol":"AAPL","close":"187.52","volume":"123456","datetime":"2026-03-24 09:30:00"}`))
	}))
	defer server.Close()

	client := NewTwelveDataQuoteClient("test-key", server.Client())
	client.baseURL = server.URL
	client.concurrency = 1

	quotes, err := client.Fetch(context.Background(), []marketdomain.SourceSymbolRequest{{SourceSymbol: "AAPL"}})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	quote, ok := quotes["AAPL"]
	if !ok {
		t.Fatal("expected AAPL quote")
	}
	if quote.Last != "187.52" || quote.Bid != "187.52" || quote.Ask != "187.52" {
		t.Fatalf("unexpected twelvedata prices: %+v", quote)
	}
	if quote.SourceTS.IsZero() {
		t.Fatal("expected non-zero source ts")
	}
}
