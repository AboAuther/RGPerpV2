package marketinfra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
)

func TestBinanceFundingRateClientFetchRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/premiumIndex" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"symbol":"BTCUSDC","lastFundingRate":"-0.00002","time":1710000000000}]`))
	}))
	defer server.Close()

	client := NewBinanceFundingRateClient(server.Client())
	client.baseURL = server.URL

	rates, err := client.FetchRates(context.Background(), []fundingdomain.SourceRateRequest{{SourceSymbol: "BTCUSDC"}})
	if err != nil {
		t.Fatalf("fetch rates: %v", err)
	}
	if rates["BTCUSDC"].Rate != "-0.00002" {
		t.Fatalf("unexpected binance funding rate: %s", rates["BTCUSDC"].Rate)
	}
	if rates["BTCUSDC"].IntervalSeconds != 28800 {
		t.Fatalf("unexpected interval: %d", rates["BTCUSDC"].IntervalSeconds)
	}
}

func TestHyperliquidFundingRateClientFetchRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"universe":[{"name":"BTC"},{"name":"ETH"}]},
			[
				{"funding":"0.0001"},
				{"funding":"0.0002"}
			]
		]`))
	}))
	defer server.Close()

	client := NewHyperliquidFundingRateClient(server.Client())
	client.baseURL = server.URL

	rates, err := client.FetchRates(context.Background(), []fundingdomain.SourceRateRequest{{SourceSymbol: "BTC"}})
	if err != nil {
		t.Fatalf("fetch rates: %v", err)
	}
	if rates["BTC"].Rate != "0.0001" {
		t.Fatalf("unexpected hyperliquid funding rate: %s", rates["BTC"].Rate)
	}
	if rates["BTC"].IntervalSeconds != 3600 {
		t.Fatalf("unexpected interval: %d", rates["BTC"].IntervalSeconds)
	}
}
