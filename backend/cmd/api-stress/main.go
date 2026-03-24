package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	defaultOrderP95SLA  = 20 * time.Millisecond
	defaultOrderP99SLA  = 50 * time.Millisecond
	defaultCancelP99SLA = 30 * time.Millisecond
	defaultTargetTPS    = 5000.0
)

type config struct {
	baseURL            string
	scenario           string
	stageConcurrency   []int
	stageDuration      time.Duration
	warmupDuration     time.Duration
	requestTimeout     time.Duration
	pauseBetweenStages time.Duration
	privateKeyHex      string
	token              string
	chainID            int64
	deviceFingerprint  string
	tracePrefix        string
	symbol             string
	marketQty          string
	limitQty           string
	limitPrice         string
	successThreshold   float64
	orderP95SLA        time.Duration
	orderP99SLA        time.Duration
	cancelP99SLA       time.Duration
	targetTPS          float64
	jsonOutput         bool
}

type apiClient struct {
	baseURL     string
	httpClient  *http.Client
	token       string
	tracePrefix string
}

type apiEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

type healthResponse struct {
	Status string `json:"status"`
}

type authChallengeData struct {
	Nonce   string `json:"nonce"`
	Message string `json:"message"`
}

type loginData struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID         uint64 `json:"id"`
		EVMAddress string `json:"evm_address"`
	} `json:"user"`
}

type symbolItem struct {
	Symbol      string `json:"symbol"`
	TickSize    string `json:"tick_size"`
	MinNotional string `json:"min_notional"`
	Status      string `json:"status"`
}

type balanceItem struct {
	AccountCode string `json:"account_code"`
	Asset       string `json:"asset"`
	Balance     string `json:"balance"`
}

type orderResponseData struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type sample struct {
	Operation string
	OK        bool
	Duration  time.Duration
	Error     string
}

type workerResult struct {
	Cycles  int
	Samples []sample
}

type operationSummary struct {
	Name        string         `json:"name"`
	Count       int            `json:"count"`
	Success     int            `json:"success"`
	SuccessRate float64        `json:"success_rate"`
	RPS         float64        `json:"rps"`
	P50Ms       float64        `json:"p50_ms"`
	P95Ms       float64        `json:"p95_ms"`
	P99Ms       float64        `json:"p99_ms"`
	MaxMs       float64        `json:"max_ms"`
	Errors      map[string]int `json:"errors,omitempty"`
}

type stageVerdict struct {
	SLAPass         bool     `json:"sla_pass"`
	SLAFailReasons  []string `json:"sla_fail_reasons,omitempty"`
	TargetTPSPass   bool     `json:"target_tps_pass"`
	ObservedTargetR float64  `json:"observed_target_rps"`
}

type stageSummary struct {
	Scenario      string                      `json:"scenario"`
	Concurrency   int                         `json:"concurrency"`
	Warmup        string                      `json:"warmup"`
	Duration      string                      `json:"duration"`
	Cycles        int                         `json:"cycles"`
	TotalRequests int                         `json:"total_requests"`
	TotalRPS      float64                     `json:"total_rps"`
	Operations    map[string]operationSummary `json:"operations"`
	Verdict       stageVerdict                `json:"verdict"`
}

type runSummary struct {
	Scenario                string         `json:"scenario"`
	BaseURL                 string         `json:"base_url"`
	UserAddress             string         `json:"user_address"`
	UserID                  uint64         `json:"user_id"`
	Symbol                  string         `json:"symbol"`
	StageDuration           string         `json:"stage_duration"`
	WarmupDuration          string         `json:"warmup_duration"`
	SuccessThreshold        float64        `json:"success_threshold"`
	OrderP95SLA             string         `json:"order_p95_sla"`
	OrderP99SLA             string         `json:"order_p99_sla"`
	CancelP99SLA            string         `json:"cancel_p99_sla"`
	TargetTPS               float64        `json:"target_tps"`
	Stages                  []stageSummary `json:"stages"`
	HighestSLAPassingStage  *int           `json:"highest_sla_passing_stage,omitempty"`
	HighestObservedTotalRPS float64        `json:"highest_observed_total_rps"`
	RequirementNotes        []string       `json:"requirement_notes"`
}

type scenario interface {
	Name() string
	Preflight(ctx context.Context, client *apiClient, cfg config) error
	RunCycle(ctx context.Context, client *apiClient, cfg config, sequence uint64) ([]sample, error)
}

type marketRoundTripScenario struct{}

type limitCycleScenario struct{}
type limitPlaceOnlyScenario struct{}

type orderRequest struct {
	ClientOrderID  string  `json:"client_order_id"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	PositionEffect string  `json:"position_effect"`
	Type           string  `json:"type"`
	Qty            string  `json:"qty"`
	Price          *string `json:"price,omitempty"`
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		exitErr(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	summary, err := run(ctx, cfg)
	if err != nil {
		exitErr(err)
	}

	if cfg.jsonOutput {
		payload, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			exitErr(err)
		}
		fmt.Println(string(payload))
		return
	}
	printSummary(summary)
}

func parseConfig() (config, error) {
	var cfg config
	var stageValues string

	flag.StringVar(&cfg.baseURL, "base-url", envOrDefault("API_BASE_URL", "http://127.0.0.1:8080"), "API base URL")
	flag.StringVar(&cfg.scenario, "scenario", envOrDefault("STRESS_SCENARIO", "market_round_trip"), "Scenario: market_round_trip, limit_cycle, or limit_place_only")
	flag.StringVar(&stageValues, "stages", envOrDefault("STRESS_STAGES", "1,5,10,20,40"), "Comma separated concurrency stages")
	flag.DurationVar(&cfg.stageDuration, "stage-duration", envDuration("STRESS_STAGE_DURATION", 10*time.Second), "Measured duration per stage")
	flag.DurationVar(&cfg.warmupDuration, "warmup", envDuration("STRESS_WARMUP", 2*time.Second), "Warmup duration before each stage")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", envDuration("STRESS_REQUEST_TIMEOUT", 5*time.Second), "Per-request timeout")
	flag.DurationVar(&cfg.pauseBetweenStages, "pause-between-stages", envDuration("STRESS_STAGE_PAUSE", time.Second), "Pause between stages")
	flag.StringVar(&cfg.privateKeyHex, "private-key", envOrDefault("STRESS_PRIVATE_KEY", envOrDefault("USER_PRIVATE_KEY", "")), "EVM private key used to log in when token is omitted")
	flag.StringVar(&cfg.token, "token", envOrDefault("STRESS_TOKEN", ""), "Bearer token; overrides private key login")
	flag.Int64Var(&cfg.chainID, "chain-id", envInt64("STRESS_CHAIN_ID", envInt64("LOCAL_CHAIN_ID", 31337)), "Login chain ID")
	flag.StringVar(&cfg.deviceFingerprint, "device-fingerprint", envOrDefault("STRESS_DEVICE_FINGERPRINT", "codex-api-stress"), "Device fingerprint used during login")
	flag.StringVar(&cfg.tracePrefix, "trace-prefix", envOrDefault("STRESS_TRACE_PREFIX", "api-stress"), "Trace ID prefix")
	flag.StringVar(&cfg.symbol, "symbol", envOrDefault("STRESS_SYMBOL", "ETH-USDC"), "Trading symbol")
	flag.StringVar(&cfg.marketQty, "market-qty", envOrDefault("STRESS_MARKET_QTY", "0.1"), "Quantity used in market_round_trip")
	flag.StringVar(&cfg.limitQty, "limit-qty", envOrDefault("STRESS_LIMIT_QTY", "1"), "Quantity used in limit_cycle")
	flag.StringVar(&cfg.limitPrice, "limit-price", envOrDefault("STRESS_LIMIT_PRICE", "10"), "Limit price used in limit_cycle")
	flag.Float64Var(&cfg.successThreshold, "success-threshold", envFloat64("STRESS_SUCCESS_THRESHOLD", 0.99), "Minimum operation success rate")
	flag.DurationVar(&cfg.orderP95SLA, "order-p95-sla", envDuration("STRESS_ORDER_P95_SLA", defaultOrderP95SLA), "Order endpoint p95 SLA")
	flag.DurationVar(&cfg.orderP99SLA, "order-p99-sla", envDuration("STRESS_ORDER_P99_SLA", defaultOrderP99SLA), "Order endpoint p99 SLA")
	flag.DurationVar(&cfg.cancelP99SLA, "cancel-p99-sla", envDuration("STRESS_CANCEL_P99_SLA", defaultCancelP99SLA), "Cancel endpoint p99 SLA")
	flag.Float64Var(&cfg.targetTPS, "target-tps", envFloat64("STRESS_TARGET_TPS", defaultTargetTPS), "Reference throughput target from design docs")
	flag.BoolVar(&cfg.jsonOutput, "json", false, "Print JSON summary")
	flag.Parse()

	stages, err := parseStages(stageValues)
	if err != nil {
		return config{}, err
	}
	cfg.stageConcurrency = stages
	cfg.scenario = strings.TrimSpace(cfg.scenario)
	cfg.baseURL = strings.TrimRight(strings.TrimSpace(cfg.baseURL), "/")
	cfg.tracePrefix = strings.TrimSpace(cfg.tracePrefix)
	if cfg.tracePrefix == "" {
		return config{}, errors.New("trace-prefix is required")
	}
	if cfg.baseURL == "" {
		return config{}, errors.New("base-url is required")
	}
	if cfg.stageDuration <= 0 {
		return config{}, errors.New("stage-duration must be positive")
	}
	if cfg.warmupDuration < 0 {
		return config{}, errors.New("warmup must be zero or positive")
	}
	if cfg.requestTimeout <= 0 {
		return config{}, errors.New("request-timeout must be positive")
	}
	if cfg.chainID <= 0 {
		return config{}, errors.New("chain-id must be positive")
	}
	if cfg.token == "" && strings.TrimSpace(cfg.privateKeyHex) == "" {
		return config{}, errors.New("either token or private-key is required")
	}
	if cfg.successThreshold <= 0 || cfg.successThreshold > 1 {
		return config{}, errors.New("success-threshold must be in (0,1]")
	}
	if cfg.symbol == "" {
		return config{}, errors.New("symbol is required")
	}
	if cfg.marketQty == "" || cfg.limitQty == "" || cfg.limitPrice == "" {
		return config{}, errors.New("market-qty, limit-qty and limit-price are required")
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config) (runSummary, error) {
	client := &apiClient{
		baseURL: cfg.baseURL,
		httpClient: &http.Client{
			Timeout: cfg.requestTimeout,
		},
		tracePrefix: cfg.tracePrefix,
	}

	if err := client.healthcheck(ctx); err != nil {
		return runSummary{}, err
	}

	var userAddress string
	var userID uint64
	if cfg.token != "" {
		client.token = cfg.token
	} else {
		privKey, err := parsePrivateKey(cfg.privateKeyHex)
		if err != nil {
			return runSummary{}, err
		}
		userAddress = crypto.PubkeyToAddress(privKey.PublicKey).Hex()
		login, err := client.login(ctx, privKey, userAddress, cfg.chainID, cfg.deviceFingerprint)
		if err != nil {
			return runSummary{}, err
		}
		client.token = login.AccessToken
		userAddress = login.User.EVMAddress
		userID = login.User.ID
	}

	if err := client.ensureSymbol(ctx, cfg.symbol); err != nil {
		return runSummary{}, err
	}

	scn, err := buildScenario(cfg.scenario)
	if err != nil {
		return runSummary{}, err
	}
	if err := scn.Preflight(ctx, client, cfg); err != nil {
		return runSummary{}, err
	}

	balances, _ := client.balances(ctx)
	summary := runSummary{
		Scenario:         scn.Name(),
		BaseURL:          cfg.baseURL,
		UserAddress:      userAddress,
		UserID:           userID,
		Symbol:           cfg.symbol,
		StageDuration:    cfg.stageDuration.String(),
		WarmupDuration:   cfg.warmupDuration.String(),
		SuccessThreshold: cfg.successThreshold,
		OrderP95SLA:      cfg.orderP95SLA.String(),
		OrderP99SLA:      cfg.orderP99SLA.String(),
		CancelP99SLA:     cfg.cancelP99SLA.String(),
		TargetTPS:        cfg.targetTPS,
		RequirementNotes: buildRequirementNotes(balances),
	}

	highestRPS := 0.0
	var highestPassing *int
	for idx, concurrency := range cfg.stageConcurrency {
		stage, err := runStage(ctx, client, cfg, scn, concurrency)
		if err != nil {
			return runSummary{}, fmt.Errorf("stage %d concurrency=%d: %w", idx+1, concurrency, err)
		}
		summary.Stages = append(summary.Stages, stage)
		if stage.TotalRPS > highestRPS {
			highestRPS = stage.TotalRPS
		}
		if stage.Verdict.SLAPass {
			value := concurrency
			highestPassing = &value
		}
		if cfg.pauseBetweenStages > 0 && idx < len(cfg.stageConcurrency)-1 {
			select {
			case <-ctx.Done():
				return runSummary{}, ctx.Err()
			case <-time.After(cfg.pauseBetweenStages):
			}
		}
	}
	summary.HighestObservedTotalRPS = highestRPS
	summary.HighestSLAPassingStage = highestPassing
	return summary, nil
}

func buildScenario(name string) (scenario, error) {
	switch strings.TrimSpace(name) {
	case "market_round_trip":
		return marketRoundTripScenario{}, nil
	case "limit_cycle":
		return limitCycleScenario{}, nil
	case "limit_place_only":
		return limitPlaceOnlyScenario{}, nil
	default:
		return nil, fmt.Errorf("unsupported scenario %q", name)
	}
}

func (marketRoundTripScenario) Name() string { return "market_round_trip" }

func (marketRoundTripScenario) Preflight(ctx context.Context, client *apiClient, cfg config) error {
	seq := uint64(time.Now().UnixNano())
	samples, err := marketRoundTripScenario{}.RunCycle(ctx, client, cfg, seq)
	if err != nil {
		return err
	}
	for _, item := range samples {
		if !item.OK {
			return fmt.Errorf("market_round_trip preflight failed on %s: %s", item.Operation, item.Error)
		}
	}
	return nil
}

func (marketRoundTripScenario) RunCycle(ctx context.Context, client *apiClient, cfg config, sequence uint64) ([]sample, error) {
	openID := fmt.Sprintf("%s-open-%d", cfg.tracePrefix, sequence)
	closeID := fmt.Sprintf("%s-close-%d", cfg.tracePrefix, sequence)

	openSample, openOrder, err := client.createOrder(ctx, orderRequest{
		ClientOrderID:  openID,
		Symbol:         cfg.symbol,
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            cfg.marketQty,
	})
	openSample.Operation = "market_open"
	if err != nil {
		return []sample{openSample}, nil
	}
	if !openSample.OK {
		return []sample{openSample}, nil
	}
	if openOrder.Status != "FILLED" {
		openSample.OK = false
		openSample.Error = "unexpected_status:" + openOrder.Status
		return []sample{openSample}, nil
	}

	closeSample, closeOrder, err := client.createOrder(ctx, orderRequest{
		ClientOrderID:  closeID,
		Symbol:         cfg.symbol,
		Side:           "SELL",
		PositionEffect: "CLOSE",
		Type:           "MARKET",
		Qty:            cfg.marketQty,
	})
	closeSample.Operation = "market_close"
	if err != nil {
		return []sample{openSample, closeSample}, nil
	}
	if !closeSample.OK {
		return []sample{openSample, closeSample}, nil
	}
	if closeOrder.Status != "FILLED" {
		closeSample.OK = false
		closeSample.Error = "unexpected_status:" + closeOrder.Status
	}
	return []sample{openSample, closeSample}, nil
}

func (limitCycleScenario) Name() string { return "limit_cycle" }

func (limitCycleScenario) Preflight(ctx context.Context, client *apiClient, cfg config) error {
	seq := uint64(time.Now().UnixNano())
	samples, err := limitCycleScenario{}.RunCycle(ctx, client, cfg, seq)
	if err != nil {
		return err
	}
	for _, item := range samples {
		if !item.OK {
			return fmt.Errorf("limit_cycle preflight failed on %s: %s", item.Operation, item.Error)
		}
	}
	return nil
}

func (limitCycleScenario) RunCycle(ctx context.Context, client *apiClient, cfg config, sequence uint64) ([]sample, error) {
	clientOrderID := fmt.Sprintf("%s-limit-%d", cfg.tracePrefix, sequence)
	limitPrice := cfg.limitPrice
	createSample, order, err := client.createOrder(ctx, orderRequest{
		ClientOrderID:  clientOrderID,
		Symbol:         cfg.symbol,
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		Qty:            cfg.limitQty,
		Price:          &limitPrice,
	})
	createSample.Operation = "limit_create"
	if err != nil {
		return []sample{createSample}, nil
	}
	if !createSample.OK {
		return []sample{createSample}, nil
	}
	if order.Status != "RESTING" {
		createSample.OK = false
		createSample.Error = "unexpected_status:" + order.Status
		return []sample{createSample}, nil
	}

	cancelSample := client.cancelOrder(ctx, order.OrderID, fmt.Sprintf("%s-cancel-%d", cfg.tracePrefix, sequence))
	cancelSample.Operation = "limit_cancel"
	return []sample{createSample, cancelSample}, nil
}

func (limitPlaceOnlyScenario) Name() string { return "limit_place_only" }

func (limitPlaceOnlyScenario) Preflight(ctx context.Context, client *apiClient, cfg config) error {
	seq := uint64(time.Now().UnixNano())
	samples, err := limitPlaceOnlyScenario{}.RunCycle(ctx, client, cfg, seq)
	if err != nil {
		return err
	}
	for _, item := range samples {
		if !item.OK {
			return fmt.Errorf("limit_place_only preflight failed on %s: %s", item.Operation, item.Error)
		}
	}
	return nil
}

func (limitPlaceOnlyScenario) RunCycle(ctx context.Context, client *apiClient, cfg config, sequence uint64) ([]sample, error) {
	clientOrderID := fmt.Sprintf("%s-place-%d", cfg.tracePrefix, sequence)
	limitPrice := cfg.limitPrice
	createSample, order, err := client.createOrder(ctx, orderRequest{
		ClientOrderID:  clientOrderID,
		Symbol:         cfg.symbol,
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		Qty:            cfg.limitQty,
		Price:          &limitPrice,
	})
	createSample.Operation = "limit_place"
	if err != nil {
		return []sample{createSample}, nil
	}
	if !createSample.OK {
		return []sample{createSample}, nil
	}
	if order.Status != "RESTING" {
		createSample.OK = false
		createSample.Error = "unexpected_status:" + order.Status
		return []sample{createSample}, nil
	}

	cancelSample := client.cancelOrder(ctx, order.OrderID, fmt.Sprintf("%s-place-clean-%d", cfg.tracePrefix, sequence))
	if !cancelSample.OK {
		createSample.OK = false
		createSample.Error = "cleanup_" + cancelSample.Error
	}
	return []sample{createSample}, nil
}

func runStage(ctx context.Context, client *apiClient, cfg config, scn scenario, concurrency int) (stageSummary, error) {
	totalDuration := cfg.warmupDuration + cfg.stageDuration
	stageCtx, cancel := context.WithTimeout(ctx, totalDuration)
	defer cancel()

	measureAt := time.Now().Add(cfg.warmupDuration)
	deadline := measureAt.Add(cfg.stageDuration)
	results := make(chan workerResult, concurrency)
	var sequence atomic.Uint64

	for workerID := 0; workerID < concurrency; workerID++ {
		go func() {
			local := workerResult{}
			for {
				if stageCtx.Err() != nil {
					results <- local
					return
				}
				now := time.Now()
				if !deadline.IsZero() && !now.Before(deadline) {
					results <- local
					return
				}
				seq := sequence.Add(1)
				samples, err := scn.RunCycle(stageCtx, client, cfg, seq)
				if err != nil {
					results <- local
					return
				}
				if time.Now().Before(measureAt) {
					continue
				}
				local.Cycles++
				local.Samples = append(local.Samples, samples...)
			}
		}()
	}

	var merged workerResult
	for i := 0; i < concurrency; i++ {
		select {
		case <-ctx.Done():
			return stageSummary{}, ctx.Err()
		case result := <-results:
			merged.Cycles += result.Cycles
			merged.Samples = append(merged.Samples, result.Samples...)
		}
	}

	summary := summarizeStage(cfg, scn.Name(), concurrency, merged, cfg.stageDuration)
	return summary, nil
}

func summarizeStage(cfg config, name string, concurrency int, result workerResult, duration time.Duration) stageSummary {
	type opCollector struct {
		successes []time.Duration
		errors    map[string]int
		count     int
		success   int
	}
	ops := map[string]*opCollector{}
	totalReqs := 0

	for _, item := range result.Samples {
		totalReqs++
		col, ok := ops[item.Operation]
		if !ok {
			col = &opCollector{errors: map[string]int{}}
			ops[item.Operation] = col
		}
		col.count++
		if item.OK {
			col.success++
			col.successes = append(col.successes, item.Duration)
		} else {
			col.errors[item.Error]++
		}
	}

	stage := stageSummary{
		Scenario:      name,
		Concurrency:   concurrency,
		Warmup:        cfg.warmupDuration.String(),
		Duration:      duration.String(),
		Cycles:        result.Cycles,
		TotalRequests: totalReqs,
		TotalRPS:      float64(totalReqs) / duration.Seconds(),
		Operations:    map[string]operationSummary{},
		Verdict: stageVerdict{
			SLAPass:         true,
			TargetTPSPass:   float64(totalReqs)/duration.Seconds() >= cfg.targetTPS,
			ObservedTargetR: float64(totalReqs) / duration.Seconds(),
		},
	}

	for name, col := range ops {
		p50, p95, p99, max := percentiles(col.successes)
		summary := operationSummary{
			Name:        name,
			Count:       col.count,
			Success:     col.success,
			SuccessRate: safeRatio(col.success, col.count),
			RPS:         float64(col.count) / duration.Seconds(),
			P50Ms:       durationMs(p50),
			P95Ms:       durationMs(p95),
			P99Ms:       durationMs(p99),
			MaxMs:       durationMs(max),
		}
		if len(col.errors) > 0 {
			summary.Errors = col.errors
		}
		stage.Operations[name] = summary

		if summary.SuccessRate < cfg.successThreshold {
			stage.Verdict.SLAPass = false
			stage.Verdict.SLAFailReasons = append(stage.Verdict.SLAFailReasons, fmt.Sprintf("%s success rate %.2f < %.2f", name, summary.SuccessRate, cfg.successThreshold))
		}
		switch name {
		case "market_open", "market_close", "limit_create", "limit_place":
			if p95 > cfg.orderP95SLA {
				stage.Verdict.SLAPass = false
				stage.Verdict.SLAFailReasons = append(stage.Verdict.SLAFailReasons, fmt.Sprintf("%s p95 %s > %s", name, p95, cfg.orderP95SLA))
			}
			if p99 > cfg.orderP99SLA {
				stage.Verdict.SLAPass = false
				stage.Verdict.SLAFailReasons = append(stage.Verdict.SLAFailReasons, fmt.Sprintf("%s p99 %s > %s", name, p99, cfg.orderP99SLA))
			}
		case "limit_cancel":
			if p99 > cfg.cancelP99SLA {
				stage.Verdict.SLAPass = false
				stage.Verdict.SLAFailReasons = append(stage.Verdict.SLAFailReasons, fmt.Sprintf("%s p99 %s > %s", name, p99, cfg.cancelP99SLA))
			}
		}
	}

	sort.Strings(stage.Verdict.SLAFailReasons)
	return stage
}

func (c *apiClient) healthcheck(ctx context.Context) error {
	var data healthResponse
	if err := c.request(ctx, http.MethodGet, "/healthz", "", nil, &data, false); err != nil {
		return fmt.Errorf("healthcheck failed: %w", err)
	}
	return nil
}

func (c *apiClient) login(ctx context.Context, privateKey *ecdsa.PrivateKey, address string, chainID int64, fingerprint string) (loginData, error) {
	var challenge authChallengeData
	if err := c.request(ctx, http.MethodPost, "/api/v1/auth/challenge", c.trace("challenge"), map[string]any{
		"address":  address,
		"chain_id": chainID,
	}, &challenge, false); err != nil {
		return loginData{}, err
	}

	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(challenge.Message), challenge.Message)))
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return loginData{}, err
	}
	var login loginData
	if err := c.request(ctx, http.MethodPost, "/api/v1/auth/login", c.trace("login"), map[string]any{
		"address":            address,
		"chain_id":           chainID,
		"nonce":              challenge.Nonce,
		"signature":          "0x" + hex.EncodeToString(signature),
		"device_fingerprint": fingerprint,
	}, &login, false); err != nil {
		return loginData{}, err
	}
	return login, nil
}

func (c *apiClient) ensureSymbol(ctx context.Context, symbol string) error {
	var symbols []symbolItem
	if err := c.request(ctx, http.MethodGet, "/api/v1/markets/symbols", c.trace("symbols"), nil, &symbols, false); err != nil {
		return err
	}
	for _, item := range symbols {
		if item.Symbol == symbol {
			return nil
		}
	}
	return fmt.Errorf("symbol %s not found in /api/v1/markets/symbols", symbol)
}

func (c *apiClient) balances(ctx context.Context) ([]balanceItem, error) {
	var balances []balanceItem
	if err := c.request(ctx, http.MethodGet, "/api/v1/account/balances", c.trace("balances"), nil, &balances, true); err != nil {
		return nil, err
	}
	return balances, nil
}

func (c *apiClient) createOrder(ctx context.Context, payload orderRequest) (sample, orderResponseData, error) {
	start := time.Now()
	var raw map[string]any
	err := c.request(ctx, http.MethodPost, "/api/v1/orders", c.trace(payload.ClientOrderID), payload, &raw, true, header{
		key:   "Idempotency-Key",
		value: payload.ClientOrderID,
	})
	item := sample{
		OK:       err == nil,
		Duration: time.Since(start),
	}
	if err != nil {
		item.Error = classifyError(err)
		return item, orderResponseData{}, nil
	}
	data := orderResponseData{
		OrderID: stringValue(raw["order_id"]),
		Status:  stringValue(raw["status"]),
	}
	if data.OrderID == "" || data.Status == "" {
		payload, _ := json.Marshal(raw)
		item.OK = false
		item.Error = "empty_order_response:" + string(payload)
		return item, orderResponseData{}, nil
	}
	return item, data, nil
}

func (c *apiClient) cancelOrder(ctx context.Context, orderID string, cancelID string) sample {
	start := time.Now()
	err := c.request(ctx, http.MethodPost, "/api/v1/orders/"+orderID+"/cancel", c.trace(cancelID), nil, &map[string]any{}, true, header{
		key:   "Idempotency-Key",
		value: cancelID,
	})
	item := sample{
		OK:       err == nil,
		Duration: time.Since(start),
	}
	if err != nil {
		item.Error = classifyError(err)
	}
	return item
}

type header struct {
	key   string
	value string
}

func (c *apiClient) request(ctx context.Context, method string, path string, traceID string, payload any, out any, auth bool, extraHeaders ...header) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if traceID != "" {
		req.Header.Set("X-Trace-Id", traceID)
	}
	if auth {
		if c.token == "" {
			return errors.New("missing bearer token")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for _, item := range extraHeaders {
		req.Header.Set(item.key, item.value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || envelope.Code != "OK" {
		return apiError{
			StatusCode: resp.StatusCode,
			Code:       envelope.Code,
			Message:    envelope.Message,
		}
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

func (c *apiClient) trace(suffix string) string {
	const maxSuffix = 12
	suffix = strings.TrimSpace(suffix)
	if len(suffix) > maxSuffix {
		suffix = suffix[:maxSuffix]
	}
	return fmt.Sprintf("%s-%x-%s", c.tracePrefix, time.Now().UnixNano(), suffix)
}

type apiError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e apiError) Error() string {
	return fmt.Sprintf("api status=%d code=%s message=%s", e.StatusCode, e.Code, e.Message)
}

func classifyError(err error) string {
	var apiErr apiError
	if errors.As(err, &apiErr) {
		if apiErr.Code != "" {
			return fmt.Sprintf("api:%s", apiErr.Code)
		}
		return fmt.Sprintf("http:%d", apiErr.StatusCode)
	}
	return "transport_error"
}

func parseStages(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	stages := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid stage concurrency %q", part)
		}
		stages = append(stages, value)
	}
	if len(stages) == 0 {
		return nil, errors.New("at least one stage is required")
	}
	return stages, nil
}

func percentiles(values []time.Duration) (time.Duration, time.Duration, time.Duration, time.Duration) {
	if len(values) == 0 {
		return 0, 0, 0, 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return quantile(sorted, 0.50), quantile(sorted, 0.95), quantile(sorted, 0.99), sorted[len(sorted)-1]
}

func quantile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(float64(len(sorted)-1) * q)
	return sorted[index]
}

func durationMs(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}

func safeRatio(numerator int, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func buildRequirementNotes(balances []balanceItem) []string {
	notes := []string{
		"Design doc target: order endpoint p95 < 20ms, p99 < 50ms; cancel endpoint p99 < 30ms.",
		"Chief architect target: matching and order processing sustain at least 5000 TPS under real trading load.",
		"Current dev runtime uses market.poll_interval_ms = 2000ms; asynchronous order execution cannot satisfy 100ms-class completion SLA without runtime or architecture changes.",
		"Current liquidator worker polls once per second; liquidation 1s target has little headroom in the current implementation.",
	}
	if len(balances) == 0 {
		return notes
	}
	for _, item := range balances {
		if item.AccountCode == "USER_WALLET" && item.Asset == "USDC" {
			notes = append(notes, "Current USER_WALLET USDC balance: "+item.Balance)
			break
		}
	}
	return notes
}

func parsePrivateKey(raw string) (*ecdsa.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "0x")
	if raw == "" {
		return nil, errors.New("private key is empty")
	}
	privateKey, err := crypto.HexToECDSA(raw)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat64(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func printSummary(summary runSummary) {
	fmt.Printf("Scenario: %s\n", summary.Scenario)
	fmt.Printf("Base URL: %s\n", summary.BaseURL)
	fmt.Printf("User: %s (id=%d)\n", summary.UserAddress, summary.UserID)
	fmt.Printf("Symbol: %s\n", summary.Symbol)
	fmt.Printf("Stage duration: %s, warmup: %s\n", summary.StageDuration, summary.WarmupDuration)
	fmt.Printf("SLA: order p95<%s, order p99<%s, cancel p99<%s, success>=%.2f\n", summary.OrderP95SLA, summary.OrderP99SLA, summary.CancelP99SLA, summary.SuccessThreshold)
	fmt.Printf("Target TPS: %.0f\n", summary.TargetTPS)
	fmt.Println()

	for _, stage := range summary.Stages {
		fmt.Printf("Stage concurrency=%d total_rps=%.2f cycles=%d verdict=%s\n", stage.Concurrency, stage.TotalRPS, stage.Cycles, passFail(stage.Verdict.SLAPass))
		names := make([]string, 0, len(stage.Operations))
		for name := range stage.Operations {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			op := stage.Operations[name]
			fmt.Printf("  %-14s count=%-6d ok=%-6d success=%.2f p95=%.2fms p99=%.2fms max=%.2fms rps=%.2f\n", op.Name, op.Count, op.Success, op.SuccessRate, op.P95Ms, op.P99Ms, op.MaxMs, op.RPS)
			if len(op.Errors) > 0 {
				keys := make([]string, 0, len(op.Errors))
				for key := range op.Errors {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					fmt.Printf("    error %-28s %d\n", key, op.Errors[key])
				}
			}
		}
		if len(stage.Verdict.SLAFailReasons) > 0 {
			for _, reason := range stage.Verdict.SLAFailReasons {
				fmt.Printf("  fail: %s\n", reason)
			}
		}
		if !stage.Verdict.TargetTPSPass {
			fmt.Printf("  note: observed total rps %.2f < target %.0f\n", stage.Verdict.ObservedTargetR, summary.TargetTPS)
		}
		fmt.Println()
	}

	if summary.HighestSLAPassingStage != nil {
		fmt.Printf("Highest SLA-passing stage: concurrency=%d\n", *summary.HighestSLAPassingStage)
	} else {
		fmt.Println("Highest SLA-passing stage: none")
	}
	fmt.Printf("Highest observed total RPS: %.2f\n", summary.HighestObservedTotalRPS)
	fmt.Println()
	fmt.Println("Notes:")
	for _, note := range summary.RequirementNotes {
		fmt.Printf("- %s\n", note)
	}
}

func passFail(value bool) string {
	if value {
		return "PASS"
	}
	return "FAIL"
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
