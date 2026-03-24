package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
	marketinfra "github.com/xiaobao/rgperp/backend/internal/infra/market"
	"github.com/xiaobao/rgperp/backend/internal/infra/marketcache"
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
	serviceRuntimeProvider := runtimeconfig.NewServiceRuntimeProvider(runtimeStore)

	bootstrap := dbinfra.NewBootstrapRepository(gormDB)
	if err := bootstrap.EnsureMarketBootstrap(context.Background()); err != nil {
		log.Fatalf("ensure market bootstrap: %v", err)
	}
	latestMarketCache := marketcache.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if latestMarketCache != nil {
		defer latestMarketCache.Close()
	}

	sourceClients := []marketdomain.SourceClient{
		marketinfra.NewBinanceTicker24hrClient(nil),
		marketinfra.NewHyperliquidMetaClient(nil),
		marketinfra.NewCoinbaseProductTickerClient(nil),
	}
	if strings.TrimSpace(cfg.MarketData.TwelveDataAPIKey) != "" {
		sourceClients = append(sourceClients, marketinfra.NewTwelveDataQuoteClient(cfg.MarketData.TwelveDataAPIKey, nil))
	} else {
		log.Printf("market-data: TWELVE_DATA_API_KEY is not set, twelvedata-backed symbols may degrade to reduce-only")
	}

	service, err := marketdomain.NewService(
		buildAggregationConfig(runtimeCfg.Market),
		dbinfra.NewMarketCatalogRepository(gormDB),
		dbinfra.NewMarketSnapshotRepository(gormDB, latestMarketCache, serviceRuntimeProvider),
		sourceClients,
	)
	if err != nil {
		log.Fatalf("create market service: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(runtimeCfg.Market.PollIntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := service.SyncOnce(ctx, time.Now().UTC()); err != nil {
		log.Printf("initial market sync failed: %v", err)
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
		}
	}
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
		"mark_price_snapshots",
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
