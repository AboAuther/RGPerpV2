package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	posttradeapp "github.com/xiaobao/rgperp/backend/internal/app/posttrade"
	runtimeconfigapp "github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
	marketinfra "github.com/xiaobao/rgperp/backend/internal/infra/market"
	marketcache "github.com/xiaobao/rgperp/backend/internal/infra/marketcache"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

func main() {
	cfg, err := config.LoadStaticConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	runtimeCfg, err := config.LoadRuntimeConfigSnapshot(cfg.App.RuntimeConfigPath)
	if err != nil {
		log.Fatalf("load runtime config: %v", err)
	}

	gormDB, err := gorm.Open(gormmysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if err := waitForMarketSchema(context.Background(), gormDB, 60*time.Second); err != nil {
		log.Fatalf("wait schema: %v", err)
	}
	runtimeConfigRepo := dbinfra.NewRuntimeConfigRepository(gormDB)
	runtimeStore, err := config.NewRuntimeConfigStore(runtimeCfg, runtimeConfigRepo)
	if err != nil {
		log.Fatalf("create runtime config store: %v", err)
	}
	if _, err := runtimeStore.Refresh(context.Background()); err != nil {
		log.Fatalf("refresh runtime config: %v", err)
	}
	runtimeStore.StartPolling(context.Background(), 2*time.Second, func(err error) {
		log.Printf("runtime config refresh failed: %v", err)
	})

	bootstrap := dbinfra.NewBootstrapRepository(gormDB)
	if err := bootstrap.EnsureMarketBootstrap(context.Background()); err != nil {
		log.Fatalf("ensure market bootstrap: %v", err)
	}
	latestMarketCache := marketcache.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if latestMarketCache != nil {
		defer latestMarketCache.Close()
	}

	service, err := marketdomain.NewService(
		buildAggregationConfig(runtimeCfg.Market),
		dbinfra.NewMarketCatalogRepository(gormDB),
		dbinfra.NewMarketSnapshotRepository(gormDB, latestMarketCache),
		[]marketdomain.SourceClient{
			marketinfra.NewBinanceTicker24hrClient(nil),
			marketinfra.NewHyperliquidMetaClient(nil),
			marketinfra.NewCoinbaseProductTickerClient(nil),
		},
	)
	if err != nil {
		log.Fatalf("create market service: %v", err)
	}
	fundingCollector, err := fundingdomain.NewRateCollector(
		clockx.RealClock{},
		dbinfra.NewFundingRepository(gormDB),
		[]fundingdomain.RateSourceClient{
			marketinfra.NewBinanceFundingRateClient(nil),
			marketinfra.NewHyperliquidFundingRateClient(nil),
		},
	)
	if err != nil {
		log.Fatalf("create funding collector: %v", err)
	}
	ledgerService := ledgerdomain.NewService(
		dbinfra.NewLedgerRepository(gormDB),
		decimalx.LedgerDecimalFactory{},
	)
	riskRepo := dbinfra.NewRiskRepository(gormDB)
	orderService, err := orderdomain.NewService(
		orderdomain.ServiceConfig{
			Asset:                  "USDC",
			TakerFeeRate:           runtimeCfg.Market.TakerFeeRate,
			MakerFeeRate:           runtimeCfg.Market.MakerFeeRate,
			DefaultMaxSlippageBps:  runtimeCfg.Market.DefaultMaxSlippageBps,
			MaxMarketDataAge:       time.Duration(runtimeCfg.Market.MaxSourceAgeSec) * time.Second,
			NetExposureHardLimit:   runtimeCfg.Risk.NetExposureHardLimit,
			MaxExposureSlippageBps: runtimeCfg.Risk.MaxExposureSlippageBps,
		},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		dbinfra.NewTxManager(gormDB),
		dbinfra.NewAccountResolver(gormDB),
		dbinfra.NewBalanceRepository(gormDB),
		ledgerService,
		dbinfra.NewOrderExecutionRepository(gormDB, latestMarketCache),
		dbinfra.NewOrderExecutionRepository(gormDB, latestMarketCache),
	)
	if err != nil {
		log.Fatalf("create order service: %v", err)
	}
	serviceRuntimeProvider := runtimeconfigapp.NewServiceRuntimeProvider(runtimeStore)
	orderService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	riskService, err := riskdomain.NewService(
		riskdomain.ServiceConfig{
			RiskBufferRatio:             runtimeCfg.Risk.GlobalBufferRatio,
			HedgeEnabled:                runtimeCfg.Hedge.Enabled,
			SoftThresholdRatio:          runtimeCfg.Hedge.SoftThresholdRatio,
			HardThresholdRatio:          runtimeCfg.Hedge.HardThresholdRatio,
			MarkPriceStaleSec:           runtimeCfg.Risk.MarkPriceStaleSec,
			ForceReduceOnlyOnStalePrice: runtimeCfg.Risk.ForceReduceOnlyOnStalePrice,
			TakerFeeRate:                runtimeCfg.Market.TakerFeeRate,
		},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		dbinfra.NewTxManager(gormDB),
		riskRepo,
		dbinfra.NewRiskOutboxPublisher(gormDB),
	)
	if err != nil {
		log.Fatalf("create risk service: %v", err)
	}
	riskService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	liquidationService, err := liquidationdomain.NewService(
		liquidationdomain.ServiceConfig{
			Asset:            "USDC",
			PenaltyRate:      runtimeCfg.Risk.LiquidationPenaltyRate,
			ExtraSlippageBps: runtimeCfg.Risk.LiquidationExtraSlippageBps,
		},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		dbinfra.NewTxManager(gormDB),
		dbinfra.NewLiquidationRepository(gormDB),
		dbinfra.NewAccountResolver(gormDB),
		ledgerService,
		riskService,
		dbinfra.NewLiquidationOutboxPublisher(gormDB),
	)
	if err != nil {
		log.Fatalf("create liquidation service: %v", err)
	}
	liquidationService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	orderService.SetPostTradeRiskProcessor(posttradeapp.NewProcessor(riskService, liquidationService))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(runtimeCfg.Market.PollIntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	fundingTicker := time.NewTicker(time.Duration(runtimeCfg.Funding.SourcePollIntervalSec) * time.Second)
	defer fundingTicker.Stop()

	if err := service.SyncOnce(ctx, time.Now().UTC()); err != nil {
		log.Printf("initial market sync failed: %v", err)
	} else if triggered, err := orderService.ExecuteTriggerOrders(ctx, runtimeCfg.Market.RestingExecutionBatch); err != nil {
		log.Printf("initial trigger order execution failed: %v", err)
	} else if triggered > 0 {
		log.Printf("initial trigger order execution completed: executed=%d", triggered)
	}
	if executed, err := orderService.ExecuteRestingOrders(ctx, runtimeCfg.Market.RestingExecutionBatch); err != nil {
		log.Printf("initial resting order execution failed: %v", err)
	} else if executed > 0 {
		log.Printf("initial resting order execution completed: executed=%d", executed)
	}
	if err := recalculateRuntimeRisk(ctx, riskRepo, riskService, liquidationService, "mark_price"); err != nil {
		log.Printf("initial runtime risk recalculation failed: %v", err)
	}
	if err := fundingCollector.SyncOnce(ctx, time.Now().UTC()); err != nil {
		log.Printf("initial funding source sync failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			log.Printf("market-data stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			if err := service.SyncOnce(ctx, time.Now().UTC()); err != nil {
				log.Printf("market-data sync failed: %v", err)
				continue
			}
			if triggered, err := orderService.ExecuteTriggerOrders(ctx, runtimeCfg.Market.RestingExecutionBatch); err != nil {
				log.Printf("trigger order execution failed: %v", err)
			} else if triggered > 0 {
				log.Printf("trigger order execution completed: executed=%d", triggered)
			}
			if executed, err := orderService.ExecuteRestingOrders(ctx, runtimeCfg.Market.RestingExecutionBatch); err != nil {
				log.Printf("resting order execution failed: %v", err)
			} else if executed > 0 {
				log.Printf("resting order execution completed: executed=%d", executed)
			}
			if err := recalculateRuntimeRisk(ctx, riskRepo, riskService, liquidationService, "mark_price"); err != nil {
				log.Printf("runtime risk recalculation failed: %v", err)
			}
		case <-fundingTicker.C:
			if err := fundingCollector.SyncOnce(ctx, time.Now().UTC()); err != nil {
				log.Printf("funding source sync failed: %v", err)
			}
		}
	}
}

func recalculateRuntimeRisk(ctx context.Context, riskRepo *dbinfra.RiskRepository, riskService *riskdomain.Service, liquidationService *liquidationdomain.Service, triggeredBy string) error {
	userIDs, err := riskRepo.ListUsersWithOpenPositions(ctx)
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		snapshot, trigger, err := riskService.RecalculateAccountRisk(ctx, userID, triggeredBy)
		if err != nil {
			return err
		}
		if trigger == nil {
			continue
		}
		liquidation, err := liquidationService.Execute(ctx, liquidationdomain.ExecuteInput{
			LiquidationID:         trigger.LiquidationID,
			UserID:                userID,
			TriggerRiskSnapshotID: snapshot.ID,
			TraceID:               triggeredBy + ":" + trigger.LiquidationID,
		})
		if err != nil {
			return err
		}
		log.Printf("runtime liquidation executed: user_id=%d liquidation_id=%s status=%s", userID, liquidation.ID, liquidation.Status)
	}
	return nil
}

func buildAggregationConfig(cfg config.MarketRuntimeConfig) marketdomain.AggregationConfig {
	health := make(map[string]marketdomain.SourceHealth, len(cfg.SourceWeights))
	for sourceName, weight := range cfg.SourceWeights {
		health[sourceName] = marketdomain.SourceHealth{
			Enabled: cfg.SourceHealthEnabled[sourceName],
			Weight:  weight,
		}
	}
	return marketdomain.AggregationConfig{
		MaxSourceAge:     time.Duration(cfg.MaxSourceAgeSec) * time.Second,
		MaxDeviationBps:  cfg.MaxDeviationBps,
		MinHealthySource: cfg.MinHealthySources,
		MarkClampBps:     cfg.MarkPriceClampBps,
		SourceHealth:     health,
	}
}

func waitForMarketSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"symbols",
		"symbol_mappings",
		"risk_tiers",
		"market_price_snapshots",
		"mark_price_snapshots",
		"funding_rate_snapshots",
	}
	for {
		ready := true
		for _, table := range requiredTables {
			if !gormDB.Migrator().HasTable(table) {
				ready = false
				break
			}
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
