package hedge

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/config"
	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const defaultBridgePath = "/workspace/backend/scripts/hyperliquid_bridge.py"

type HyperliquidAdapter struct {
	apiURL         string
	accountAddress string
	privateKey     string
	bridgePath     string
}

func NewHyperliquidAdapter(cfg config.HedgeConfig) (*HyperliquidAdapter, error) {
	apiURL := strings.TrimSpace(cfg.APIURL)
	privateKey := strings.TrimSpace(cfg.PrivateKey)
	if apiURL == "" || privateKey == "" {
		return nil, fmt.Errorf("%w: hyperliquid api url and private key are required", errorsx.ErrInvalidArgument)
	}
	return &HyperliquidAdapter{
		apiURL:         apiURL,
		accountAddress: strings.TrimSpace(cfg.AccountAddress),
		privateKey:     privateKey,
		bridgePath:     defaultBridgePath,
	}, nil
}

func (a *HyperliquidAdapter) PlaceOrder(ctx context.Context, req hedgedomain.ExecutionRequest) (hedgedomain.ExecutionResult, error) {
	side, err := hyperliquidSide(req.Side)
	if err != nil {
		return hedgedomain.ExecutionResult{}, err
	}
	resp, err := a.runBridge(ctx, map[string]any{
		"action":         "order",
		"api_url":        a.apiURL,
		"wallet_address": a.accountAddress,
		"private_key":    a.privateKey,
		"symbol":         strings.ToUpper(strings.TrimSpace(req.Symbol)),
		"side":           side,
		"size":           strings.TrimSpace(req.Qty),
		"reduce_only":    false,
		"slippage_bps":   75,
		"leverage":       5,
		"is_cross":       true,
	})
	if err != nil {
		return hedgedomain.ExecutionResult{}, err
	}
	if status := strings.TrimSpace(fmt.Sprintf("%v", resp["status"])); strings.EqualFold(status, "error") {
		return hedgedomain.ExecutionResult{}, fmt.Errorf("%v", resp["error"])
	}

	orderID := strings.TrimSpace(fmt.Sprintf("%v", resp["external_order_id"]))
	filledQty := strings.TrimSpace(fmt.Sprintf("%v", resp["filled_size"]))
	filledPrice := strings.TrimSpace(fmt.Sprintf("%v", resp["filled_price"]))
	if filledQty == "" || filledQty == "<nil>" {
		filledQty = strings.TrimSpace(req.Qty)
	}
	if filledPrice == "" || filledPrice == "<nil>" {
		filledPrice = "0"
	}

	return hedgedomain.ExecutionResult{
		VenueOrderID: orderID,
		Status:       hedgedomain.OrderStatusFilled,
		Fills: []hedgedomain.ExecutionFill{{
			VenueFillID: fallbackString(orderID, "hl_fill"),
			Qty:         filledQty,
			Price:       filledPrice,
			Fee:         "0",
		}},
	}, nil
}

func (a *HyperliquidAdapter) GetNetPosition(ctx context.Context, symbol string) (string, error) {
	resp, err := a.runBridge(ctx, map[string]any{
		"action":         "position",
		"api_url":        a.apiURL,
		"wallet_address": a.accountAddress,
		"private_key":    a.privateKey,
		"symbol":         strings.ToUpper(strings.TrimSpace(symbol)),
	})
	if err != nil {
		return "", err
	}
	if status := strings.TrimSpace(fmt.Sprintf("%v", resp["status"])); strings.EqualFold(status, "error") {
		return "", fmt.Errorf("%v", resp["error"])
	}
	position := strings.TrimSpace(fmt.Sprintf("%v", resp["position"]))
	if position == "" || position == "<nil>" {
		return "0", nil
	}
	return position, nil
}

func (a *HyperliquidAdapter) runBridge(ctx context.Context, payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "python3", a.bridgePath)
	cmd.Stdin = strings.NewReader(string(raw))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("hyperliquid bridge failed: %s", strings.TrimSpace(string(out)))
	}
	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func hyperliquidSide(side string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case hedgedomain.OrderSideBuy:
		return "long", nil
	case hedgedomain.OrderSideSell:
		return "short", nil
	default:
		return "", fmt.Errorf("%w: unsupported hedge side %s", errorsx.ErrInvalidArgument, side)
	}
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
