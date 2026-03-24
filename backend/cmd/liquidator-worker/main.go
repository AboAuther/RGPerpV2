package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	runtimeconfigapp "github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

const (
	liquidationTriggerEventType = "risk.liquidation.triggered"
	liquidatorConsumerName      = "liquidator-worker"
)

type liquidationTriggerPayload struct {
	LiquidationID     string `json:"liquidation_id"`
	UserID            uint64 `json:"user_id"`
	Mode              string `json:"mode"`
	PositionID        string `json:"position_id"`
	RiskSnapshotID    uint64 `json:"risk_snapshot_id"`
	TriggerPriceTS    string `json:"trigger_price_ts"`
	Status            string `json:"status"`
	MarginRatio       string `json:"margin_ratio"`
	Equity            string `json:"equity"`
	MaintenanceMargin string `json:"maintenance_margin"`
}

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
	if err := waitForLiquidationSchema(context.Background(), gormDB, 60*time.Second); err != nil {
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
	accountResolver := dbinfra.NewAccountResolver(gormDB)
	ledgerService := ledgerdomain.NewService(
		dbinfra.NewLedgerRepository(gormDB),
		decimalx.LedgerDecimalFactory{},
	)
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

	outboxRepo := dbinfra.NewOutboxRepository(gormDB)
	consumptionRepo := dbinfra.NewMessageConsumptionRepository(gormDB)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pollInterval := time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	runOnce := func() {
		events, err := outboxRepo.ListPendingByEventType(ctx, liquidationTriggerEventType, 100)
		if err != nil {
			log.Printf("list pending liquidation triggers failed: %v", err)
			return
		}
		for _, event := range events {
			if err := consumeLiquidationTrigger(ctx, event, consumptionRepo, liquidationService, riskService); err != nil {
				log.Printf("consume liquidation trigger failed: event_id=%s err=%v", event.EventID, err)
			}
		}
	}

	runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Printf("liquidator-worker stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func consumeLiquidationTrigger(
	ctx context.Context,
	event dbinfra.OutboxEventModel,
	consumptionRepo *dbinfra.MessageConsumptionRepository,
	liquidationService *liquidationdomain.Service,
	riskService *riskdomain.Service,
) error {
	acquired, err := consumptionRepo.TryBegin(ctx, liquidatorConsumerName, event.EventID, time.Now().UTC())
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}

	if err := processLiquidationTrigger(ctx, event, liquidationService, riskService); err != nil {
		_ = consumptionRepo.Delete(ctx, liquidatorConsumerName, event.EventID)
		return err
	}
	return nil
}

func processLiquidationTrigger(ctx context.Context, event dbinfra.OutboxEventModel, liquidationService *liquidationdomain.Service, riskService *riskdomain.Service) error {
	var payload liquidationTriggerPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return err
	}
	if payload.LiquidationID == "" || payload.UserID == 0 || payload.RiskSnapshotID == 0 {
		return nil
	}

	liquidation, err := liquidationService.Execute(ctx, liquidationdomain.ExecuteInput{
		LiquidationID:         payload.LiquidationID,
		UserID:                payload.UserID,
		Mode:                  payload.Mode,
		PositionID:            payload.PositionID,
		TriggerRiskSnapshotID: payload.RiskSnapshotID,
		TraceID:               event.EventID,
	})
	if err != nil {
		return err
	}

	switch liquidation.Status {
	case liquidationdomain.StatusExecuted:
		_, err = riskService.RefreshAccountRisk(ctx, payload.UserID, "liquidation_executed")
	case liquidationdomain.StatusAborted:
		_, err = riskService.RefreshAccountRisk(ctx, payload.UserID, "liquidation_aborted")
	default:
		err = nil
	}
	if err != nil {
		return err
	}
	log.Printf("liquidation processed: event_id=%s liquidation_id=%s user_id=%d status=%s", event.EventID, liquidation.ID, payload.UserID, liquidation.Status)
	return nil
}

func waitForLiquidationSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"accounts",
		"account_balance_snapshots",
		"positions",
		"orders",
		"mark_price_snapshots",
		"risk_snapshots",
		"liquidations",
		"liquidation_items",
		"ledger_tx",
		"ledger_entries",
		"outbox_events",
		"message_consumptions",
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
