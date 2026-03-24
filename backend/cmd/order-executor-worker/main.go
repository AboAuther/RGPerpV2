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
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
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
	if err := dbinfra.ConfigureMySQLConnectionPool(gormDB, cfg.MySQL.MaxOpenConns, cfg.MySQL.MaxIdleConns, time.Duration(cfg.MySQL.ConnMaxLifetimeSec)*time.Second); err != nil {
		log.Fatalf("configure mysql pool: %v", err)
	}
	if err := waitForOrderSchema(context.Background(), gormDB, 60*time.Second); err != nil {
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

	latestMarketCache := marketcache.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if latestMarketCache != nil {
		defer latestMarketCache.Close()
	}

	txManager := dbinfra.NewTxManager(gormDB)
	ledgerService := ledgerdomain.NewService(
		dbinfra.NewLedgerRepository(gormDB),
		decimalx.LedgerDecimalFactory{},
	)
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
		txManager,
		dbinfra.NewAccountResolver(gormDB),
		dbinfra.NewBalanceRepository(gormDB),
		ledgerService,
		dbinfra.NewOrderExecutionRepository(gormDB, latestMarketCache),
		dbinfra.NewOrderExecutionRepository(gormDB, latestMarketCache),
	)
	if err != nil {
		log.Fatalf("create order service: %v", err)
	}

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
		txManager,
		dbinfra.NewRiskRepository(gormDB),
		dbinfra.NewRiskOutboxPublisher(gormDB),
	)
	if err != nil {
		log.Fatalf("create risk service: %v", err)
	}

	serviceRuntimeProvider := runtimeconfigapp.NewServiceRuntimeProvider(runtimeStore)
	orderService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	riskService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	orderService.SetPostTradeRiskProcessor(posttradeapp.NewProcessor(clockx.RealClock{}, &idgen.TimeBasedGenerator{}, dbinfra.NewRiskOutboxPublisher(gormDB)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(runtimeCfg.Market.PollIntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runOnce := func() {
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
	}

	runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Printf("order-executor-worker stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func waitForOrderSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"accounts",
		"account_balance_snapshots",
		"symbols",
		"orders",
		"fills",
		"positions",
		"mark_price_snapshots",
		"risk_snapshots",
		"ledger_tx",
		"ledger_entries",
		"outbox_events",
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
