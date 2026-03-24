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
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

const (
	recalculateRequestedEventType = "risk.recalculate.requested"
	riskEngineConsumerName        = "risk-engine-worker"
)

type recalculateRequestPayload struct {
	RequestID   string `json:"request_id"`
	UserID      uint64 `json:"user_id"`
	TriggeredBy string `json:"triggered_by"`
	TraceID     string `json:"trace_id"`
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
	if err := waitForRiskSchema(context.Background(), gormDB, 60*time.Second); err != nil {
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

	riskRepo := dbinfra.NewRiskRepository(gormDB)
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
	riskService.SetRuntimeConfigProvider(runtimeconfigapp.NewServiceRuntimeProvider(runtimeStore))

	outboxRepo := dbinfra.NewOutboxRepository(gormDB)
	consumptionRepo := dbinfra.NewMessageConsumptionRepository(gormDB)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(runtimeCfg.Market.PollIntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runOnce := func() {
		if err := recalculateByMarkPrice(ctx, riskRepo, riskService); err != nil {
			log.Printf("price-driven risk recalculation failed: %v", err)
		}
		if err := processRequestedRecalculations(ctx, outboxRepo, consumptionRepo, riskService); err != nil {
			log.Printf("requested risk recalculation failed: %v", err)
		}
	}

	runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Printf("risk-engine-worker stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func recalculateByMarkPrice(ctx context.Context, riskRepo *dbinfra.RiskRepository, riskService *riskdomain.Service) error {
	userIDs, err := riskRepo.ListUsersWithOpenPositions(ctx)
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		snapshot, trigger, err := riskService.RecalculateAccountRisk(ctx, userID, "mark_price")
		if err != nil {
			return err
		}
		if trigger != nil {
			log.Printf("price-driven liquidation triggered: user_id=%d liquidation_id=%s risk_snapshot_id=%d", userID, trigger.LiquidationID, snapshot.ID)
		}
	}
	return nil
}

func processRequestedRecalculations(
	ctx context.Context,
	outboxRepo *dbinfra.OutboxRepository,
	consumptionRepo *dbinfra.MessageConsumptionRepository,
	riskService *riskdomain.Service,
) error {
	events, err := outboxRepo.ListPendingByEventType(ctx, recalculateRequestedEventType, 100)
	if err != nil {
		return err
	}
	for _, event := range events {
		acquired, err := consumptionRepo.TryBegin(ctx, riskEngineConsumerName, event.EventID, time.Now().UTC())
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		if err := processRequestedEvent(ctx, event, riskService); err != nil {
			_ = consumptionRepo.Delete(ctx, riskEngineConsumerName, event.EventID)
			return err
		}
	}
	return nil
}

func processRequestedEvent(ctx context.Context, event dbinfra.OutboxEventModel, riskService *riskdomain.Service) error {
	var payload recalculateRequestPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return err
	}
	if payload.UserID == 0 {
		return nil
	}
	triggeredBy := payload.TriggeredBy
	if triggeredBy == "" {
		triggeredBy = "manual"
	}
	snapshot, trigger, err := riskService.RecalculateAccountRisk(ctx, payload.UserID, triggeredBy)
	if err != nil {
		return err
	}
	if trigger != nil {
		log.Printf("requested liquidation triggered: user_id=%d liquidation_id=%s risk_snapshot_id=%d trace_id=%s", payload.UserID, trigger.LiquidationID, snapshot.ID, payload.TraceID)
	}
	return nil
}

func waitForRiskSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"accounts",
		"account_balance_snapshots",
		"positions",
		"mark_price_snapshots",
		"risk_snapshots",
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
