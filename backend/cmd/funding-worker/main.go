package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
	marketinfra "github.com/xiaobao/rgperp/backend/internal/infra/market"
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
	if err := waitForFundingSchema(context.Background(), gormDB, 60*time.Second); err != nil {
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

	txManager := dbinfra.NewTxManager(gormDB)
	repo := dbinfra.NewFundingRepository(gormDB)
	accountResolver := dbinfra.NewAccountResolver(gormDB)
	ledgerService := ledgerdomain.NewService(
		dbinfra.NewLedgerRepository(gormDB),
		decimalx.LedgerDecimalFactory{},
	)
	eventPublisher := dbinfra.NewFundingOutboxPublisher(gormDB)
	executor, err := fundingdomain.NewExecutor(
		fundingdomain.ExecutorConfig{Asset: "USDC"},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		txManager,
		repo,
		accountResolver,
		ledgerService,
		eventPublisher,
	)
	if err != nil {
		log.Fatalf("create funding executor: %v", err)
	}
	collector, err := fundingdomain.NewRateCollector(
		clockx.RealClock{},
		repo,
		[]fundingdomain.RateSourceClient{
			marketinfra.NewBinanceFundingRateClient(nil),
			marketinfra.NewHyperliquidFundingRateClient(nil),
		},
	)
	if err != nil {
		log.Fatalf("create funding collector: %v", err)
	}
	requestIDGen := &idgen.TimeBasedGenerator{}
	outboxRepo := dbinfra.NewOutboxRepository(gormDB)
	rateProvider := fundingdomain.NewSnapshotRateProvider(repo)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runOnce := func() {
		currentCfg := runtimeStore.Current()
		service, err := fundingdomain.NewService(fundingdomain.ServiceConfig{
			SettlementIntervalSec: currentCfg.Funding.IntervalSec,
			CapRatePerHour:        currentCfg.Funding.CapRatePerHour,
			MinValidSourceCount:   currentCfg.Funding.MinValidSourceCount,
			DefaultCryptoModel:    currentCfg.Funding.DefaultModelCrypto,
		})
		if err != nil {
			log.Printf("funding runtime config invalid: %v", err)
			return
		}
		planner, err := fundingdomain.NewPlanner(
			service,
			serviceRuntimeProvider,
			clockx.RealClock{},
			&idgen.TimeBasedGenerator{},
			txManager,
			repo,
			rateProvider,
		)
		if err != nil {
			log.Printf("create funding planner failed: %v", err)
			return
		}
		if err := collector.SyncOnce(ctx, time.Now().UTC()); err != nil {
			log.Printf("funding source sync failed: %v", err)
			return
		}
		symbols, err := repo.ListSymbolsForFunding(ctx)
		if err != nil {
			log.Printf("list funding symbols failed: %v", err)
			return
		}
		createdBatches := 0
		now := time.Now().UTC()
		for _, symbol := range symbols {
			plan, err := planner.PlanBatchForSymbol(ctx, symbol, now)
			if err != nil {
				if fundingdomain.IsInsufficientValidFundingSources(err) {
					if symbol.AssetClass != "CRYPTO" {
						if changed, restoreErr := repo.RestoreSymbolToTrading(ctx, symbol.ID); restoreErr != nil {
							log.Printf("funding non-crypto restore failed: symbol=%s symbol_id=%d err=%v", symbol.Symbol, symbol.ID, restoreErr)
						} else if changed {
							log.Printf("funding non-crypto symbol kept trading despite missing funding window: symbol=%s symbol_id=%d", symbol.Symbol, symbol.ID)
						}
						continue
					}
					changed, degradeErr := repo.DowngradeSymbolToReduceOnly(ctx, symbol.ID)
					if degradeErr != nil {
						log.Printf("funding degraded symbol update failed: symbol=%s symbol_id=%d err=%v", symbol.Symbol, symbol.ID, degradeErr)
						continue
					}
					if changed {
						if err := outboxRepo.Create(ctx, dbinfra.OutboxMessage{
							EventID:       requestIDGen.NewID("evt"),
							AggregateType: "symbol",
							AggregateID:   fmt.Sprintf("%d", symbol.ID),
							EventType:     "funding.sources.degraded",
							Payload: map[string]any{
								"symbol_id":       symbol.ID,
								"symbol":          symbol.Symbol,
								"failure_reason":  fundingdomain.FailureReasonInsufficientSources,
								"desired_status":  "REDUCE_ONLY",
								"triggered_by":    "funding-worker",
								"triggered_at":    now.Format(time.RFC3339Nano),
								"current_sources": len(symbol.Mappings),
							},
							CreatedAt: now,
						}); err != nil {
							log.Printf("queue funding degraded event failed: symbol=%s symbol_id=%d err=%v", symbol.Symbol, symbol.ID, err)
						}
						log.Printf("funding symbol degraded to reduce_only: symbol=%s symbol_id=%d reason=%s", symbol.Symbol, symbol.ID, fundingdomain.FailureReasonInsufficientSources)
					} else {
						log.Printf("funding insufficient sources persisted: symbol=%s symbol_id=%d", symbol.Symbol, symbol.ID)
					}
					continue
				}
				log.Printf("funding planning failed: symbol=%s symbol_id=%d err=%v", symbol.Symbol, symbol.ID, err)
				continue
			}
			if changed, restoreErr := repo.RestoreSymbolToTrading(ctx, symbol.ID); restoreErr != nil {
				log.Printf("funding trading restore failed: symbol=%s symbol_id=%d err=%v", symbol.Symbol, symbol.ID, restoreErr)
			} else if changed {
				log.Printf("funding symbol restored to trading: symbol=%s symbol_id=%d", symbol.Symbol, symbol.ID)
			}
			if plan != nil {
				createdBatches++
			}
		}
		if createdBatches > 0 {
			log.Printf("funding planning completed: created_batches=%d", createdBatches)
		}
		applied, err := executor.ApplyReadyBatches(ctx, 100)
		if err != nil {
			log.Printf("funding apply failed: %v", err)
			return
		}
		if len(applied) > 0 {
			log.Printf("funding apply completed: applied_batches=%d", len(applied))
		}
		impactedUsers := make(map[uint64]struct{})
		for _, batch := range applied {
			for _, userID := range batch.UserIDs {
				impactedUsers[userID] = struct{}{}
			}
		}
		for userID := range impactedUsers {
			requestID := requestIDGen.NewID("rrq")
			if err := outboxRepo.Create(ctx, dbinfra.OutboxMessage{
				EventID:       requestIDGen.NewID("evt"),
				AggregateType: "risk_recalculation",
				AggregateID:   requestID,
				EventType:     "risk.recalculate.requested",
				Payload: map[string]any{
					"request_id":   requestID,
					"user_id":      userID,
					"triggered_by": "funding",
					"trace_id":     "funding:" + requestID,
				},
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				log.Printf("queue risk recalculation after funding failed: user_id=%d err=%v", userID, err)
				continue
			}
			log.Printf("risk recalculation queued after funding: user_id=%d request_id=%s", userID, requestID)
		}
	}

	runOnce()
	for {
		pollInterval := time.Duration(runtimeStore.Current().Funding.SourcePollIntervalSec) * time.Second
		if pollInterval <= 0 {
			pollInterval = time.Minute
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Printf("funding-worker stopping: %v", ctx.Err())
			return
		case <-timer.C:
			runOnce()
		}
	}
}

func waitForFundingSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"accounts",
		"account_balance_snapshots",
		"symbols",
		"symbol_mappings",
		"positions",
		"orders",
		"mark_price_snapshots",
		"risk_snapshots",
		"liquidations",
		"liquidation_items",
		"funding_rate_snapshots",
		"funding_batches",
		"funding_batch_items",
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
