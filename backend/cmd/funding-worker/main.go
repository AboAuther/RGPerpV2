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

	runtimeconfigapp "github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
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

	txManager := dbinfra.NewTxManager(gormDB)
	repo := dbinfra.NewFundingRepository(gormDB)
	accountResolver := dbinfra.NewAccountResolver(gormDB)
	ledgerService := ledgerdomain.NewService(
		dbinfra.NewLedgerRepository(gormDB),
		decimalx.LedgerDecimalFactory{},
	)
	service, err := fundingdomain.NewService(fundingdomain.ServiceConfig{
		SettlementIntervalSec: runtimeCfg.Funding.IntervalSec,
		CapRatePerHour:        runtimeCfg.Funding.CapRatePerHour,
		MinValidSourceCount:   runtimeCfg.Funding.MinValidSourceCount,
		DefaultCryptoModel:    runtimeCfg.Funding.DefaultModelCrypto,
	})
	if err != nil {
		log.Fatalf("create funding service: %v", err)
	}
	planner, err := fundingdomain.NewPlanner(
		service,
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		txManager,
		repo,
		fundingdomain.NewSnapshotRateProvider(repo),
	)
	if err != nil {
		log.Fatalf("create funding planner: %v", err)
	}
	executor, err := fundingdomain.NewExecutor(
		fundingdomain.ExecutorConfig{Asset: "USDC"},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		txManager,
		repo,
		accountResolver,
		ledgerService,
	)
	if err != nil {
		log.Fatalf("create funding executor: %v", err)
	}
	serviceRuntimeProvider := runtimeconfigapp.NewServiceRuntimeProvider(runtimeStore)
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
	riskService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	liquidationService, err := liquidationdomain.NewService(
		liquidationdomain.ServiceConfig{
			Asset:            "USDC",
			PenaltyRate:      runtimeCfg.Risk.LiquidationPenaltyRate,
			ExtraSlippageBps: runtimeCfg.Risk.LiquidationExtraSlippageBps,
		},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		txManager,
		dbinfra.NewLiquidationRepository(gormDB),
		accountResolver,
		ledgerService,
		riskService,
		dbinfra.NewLiquidationOutboxPublisher(gormDB),
	)
	if err != nil {
		log.Fatalf("create liquidation service: %v", err)
	}
	liquidationService.SetRuntimeConfigProvider(serviceRuntimeProvider)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pollInterval := time.Duration(runtimeCfg.Funding.SourcePollIntervalSec) * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	runOnce := func() {
		plans, err := planner.PlanDueBatches(ctx)
		if err != nil {
			log.Printf("funding planning failed: %v", err)
			return
		}
		if len(plans) > 0 {
			log.Printf("funding planning completed: created_batches=%d", len(plans))
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
			snapshot, trigger, err := riskService.RecalculateAccountRisk(ctx, userID, "funding")
			if err != nil {
				log.Printf("risk recalculation after funding failed: user_id=%d err=%v", userID, err)
				continue
			}
			if trigger == nil {
				continue
			}
			liquidation, err := liquidationService.Execute(ctx, liquidationdomain.ExecuteInput{
				LiquidationID:         trigger.LiquidationID,
				UserID:                userID,
				TriggerRiskSnapshotID: snapshot.ID,
				TraceID:               "funding:" + trigger.LiquidationID,
			})
			if err != nil {
				log.Printf("liquidation execution after funding failed: user_id=%d liquidation_id=%s err=%v", userID, trigger.LiquidationID, err)
				continue
			}
			log.Printf("liquidation executed after funding: user_id=%d liquidation_id=%s status=%s", userID, liquidation.ID, liquidation.Status)
		}
	}

	runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Printf("funding-worker stopping: %v", ctx.Err())
			return
		case <-ticker.C:
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
