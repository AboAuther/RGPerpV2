package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/xiaobao/rgperp/backend/internal/config"
	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
	dbinfra "github.com/xiaobao/rgperp/backend/internal/infra/db"
	hedgeinfra "github.com/xiaobao/rgperp/backend/internal/infra/hedge"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

const (
	hedgeRequestedEventType = "hedge.requested"
	hedgerConsumerName      = "hedger-worker"
)

type hedgeRequestedPayload struct {
	HedgeIntentID string `json:"hedge_intent_id"`
}

type positionReader interface {
	GetNetPosition(ctx context.Context, symbol string) (string, error)
}

type hedgeVenue interface {
	hedgedomain.VenueAdapter
	positionReader
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
	if err := waitForHedgeSchema(context.Background(), gormDB, 60*time.Second); err != nil {
		log.Fatalf("wait schema: %v", err)
	}

	hedgeRepo := dbinfra.NewHedgeRepository(gormDB)
	monitorRepo := dbinfra.NewHedgeMonitorRepository(gormDB)
	adapter, venueName, err := buildHedgeVenue(cfg.Hedge, monitorRepo)
	if err != nil {
		log.Fatalf("build hedge venue: %v", err)
	}
	hedgeService, err := hedgedomain.NewService(
		hedgedomain.ServiceConfig{Venue: venueName},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		dbinfra.NewTxManager(gormDB),
		hedgeRepo,
		adapter,
		dbinfra.NewHedgeOutboxPublisher(gormDB),
	)
	if err != nil {
		log.Fatalf("create hedge service: %v", err)
	}
	log.Printf("hedger venue ready: %s", venueName)

	outboxRepo := dbinfra.NewOutboxRepository(gormDB)
	consumptionRepo := dbinfra.NewMessageConsumptionRepository(gormDB)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(runtimeCfg.Market.PollIntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runOnce := func() {
		if err := processRequestedHedges(ctx, outboxRepo, consumptionRepo, hedgeService); err != nil {
			log.Printf("requested hedges failed: %v", err)
		}
		if err := captureSystemHedgeSnapshots(ctx, monitorRepo, adapter); err != nil {
			log.Printf("capture hedge snapshots failed: %v", err)
		}
	}

	runOnce()
	for {
		select {
		case <-ctx.Done():
			log.Printf("hedger-worker stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func buildHedgeVenue(cfg config.HedgeConfig, reader hedgeinfra.ManagedPositionReader) (hedgeVenue, string, error) {
	if strings.TrimSpace(cfg.APIURL) != "" && strings.TrimSpace(cfg.PrivateKey) != "" {
		adapter, err := hedgeinfra.NewHyperliquidAdapter(cfg)
		if err != nil {
			return nil, "", err
		}
		return adapter, "hyperliquid_testnet", nil
	}
	return hedgeinfra.NewSimulatedAdapter(reader), "simulated", nil
}

func processRequestedHedges(
	ctx context.Context,
	outboxRepo *dbinfra.OutboxRepository,
	consumptionRepo *dbinfra.MessageConsumptionRepository,
	hedgeService *hedgedomain.Service,
) error {
	events, err := outboxRepo.ListPendingByEventTypeForConsumer(ctx, hedgeRequestedEventType, hedgerConsumerName, 100)
	if err != nil {
		return err
	}
	for _, event := range events {
		acquired, err := consumptionRepo.TryBegin(ctx, hedgerConsumerName, event.EventID, time.Now().UTC())
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		if err := processRequestedHedgeEvent(ctx, event, hedgeService); err != nil {
			if errors.Is(err, errorsx.ErrConflict) || errors.Is(err, errorsx.ErrNotFound) {
				log.Printf("skip stale hedge event: event_id=%s err=%v", event.EventID, err)
				continue
			}
			_ = consumptionRepo.Delete(ctx, hedgerConsumerName, event.EventID)
			log.Printf("process hedge event failed: event_id=%s err=%v", event.EventID, err)
			continue
		}
	}
	return nil
}

func processRequestedHedgeEvent(ctx context.Context, event dbinfra.OutboxEventModel, hedgeService *hedgedomain.Service) error {
	var payload hedgeRequestedPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return err
	}
	if payload.HedgeIntentID == "" {
		return nil
	}
	intent, err := hedgeService.ExecuteIntent(ctx, payload.HedgeIntentID)
	if err != nil {
		return err
	}
	log.Printf("hedge intent processed: hedge_intent_id=%s status=%s", intent.ID, intent.Status)
	return nil
}

func captureSystemHedgeSnapshots(ctx context.Context, monitorRepo *dbinfra.HedgeMonitorRepository, reader positionReader) error {
	states, err := monitorRepo.ListSnapshotStates(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, state := range states {
		internalNet := decimalx.MustFromString(state.InternalLongQty).Sub(decimalx.MustFromString(state.InternalShortQty))
		targetHedge := internalNet.Neg()
		managedNet := decimalx.MustFromString(state.ManagedLongQty).Sub(decimalx.MustFromString(state.ManagedShortQty))
		externalNet := managedNet
		if reader != nil {
			raw, err := reader.GetNetPosition(ctx, state.Symbol)
			if err != nil {
				return err
			}
			externalNet = decimalx.MustFromString(raw)
		}
		managedDrift := targetHedge.Sub(managedNet)
		externalDrift := targetHedge.Sub(externalNet)
		threshold, err := dbinfra.ComputeHedgeHealthyThreshold(state.StepSize, state.MinNotional, state.MarkPrice)
		if err != nil {
			return err
		}
		healthy := managedDrift.Abs().LessThanOrEqual(decimalx.MustFromString(threshold))
		if err := monitorRepo.CreateSystemSnapshot(ctx, dbinfra.HedgeSnapshotRecord{
			SymbolID:         state.SymbolID,
			Symbol:           state.Symbol,
			InternalNetQty:   internalNet.String(),
			TargetHedgeQty:   targetHedge.String(),
			ManagedHedgeQty:  managedNet.String(),
			ExternalHedgeQty: externalNet.String(),
			ManagedDriftQty:  managedDrift.String(),
			ExternalDriftQty: externalDrift.String(),
			HedgeHealthy:     healthy,
			CreatedAt:        now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func waitForHedgeSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"symbols",
		"positions",
		"hedge_intents",
		"hedge_orders",
		"hedge_fills",
		"hedge_positions",
		"system_hedge_snapshots",
		"mark_price_snapshots",
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
