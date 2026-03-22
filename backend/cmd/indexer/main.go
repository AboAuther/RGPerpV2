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

	"github.com/xiaobao/rgperp/backend/internal/config"
	indexerdomain "github.com/xiaobao/rgperp/backend/internal/domain/indexer"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/infra/db"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

const pollInterval = 2 * time.Second

func main() {
	cfg, err := config.LoadStaticConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	gormDB, err := gorm.Open(gormmysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if err := waitForSchema(context.Background(), gormDB, 60*time.Second); err != nil {
		log.Fatalf("wait schema: %v", err)
	}

	bootstrap := db.NewBootstrapRepository(gormDB)
	if err := bootstrap.EnsureSystemBootstrap(context.Background()); err != nil {
		log.Fatalf("ensure system bootstrap: %v", err)
	}

	txManager := db.NewTxManager(gormDB)
	ledgerService := ledgerdomain.NewService(db.NewLedgerRepository(gormDB), decimalx.LedgerDecimalFactory{})
	confirmations := config.EnabledChainConfirmations(cfg)
	depositAddressRepo := db.NewDepositAddressRepository(gormDB, confirmations)
	walletService := walletdomain.NewService(
		db.NewDepositRepository(gormDB),
		db.NewWithdrawRepository(gormDB),
		db.NewUserRepository(gormDB),
		ledgerService,
		txManager,
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		db.NewAccountResolver(gormDB),
		db.NewBalanceRepository(gormDB),
		depositAddressRepo,
		nil,
	)

	chainRules := buildChainRules(cfg)
	source, err := chaininfra.NewEVMEventSource(buildEVMConfigs(cfg))
	if err != nil {
		log.Fatalf("create evm event source: %v", err)
	}
	defer source.Close()

	indexerService, err := indexerdomain.NewService(
		walletService,
		db.NewDepositRepository(gormDB),
		db.NewWithdrawRepository(gormDB),
		depositAddressRepo,
		db.NewIndexerEventPublisher(gormDB),
		txManager,
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		chainRules,
	)
	if err != nil {
		log.Fatalf("create indexer service: %v", err)
	}

	runner, err := indexerdomain.NewRunner(
		source,
		indexerService,
		db.NewIndexerCursorRepository(gormDB),
		clockx.RealClock{},
		chainRules,
		500,
	)
	if err != nil {
		log.Fatalf("create indexer runner: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Printf("indexer started with %d enabled chains", len(chainRules))
	if err := syncAllChains(ctx, runner, chainRules); err != nil {
		log.Printf("initial sync failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("indexer stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			if err := syncAllChains(ctx, runner, chainRules); err != nil {
				log.Printf("sync failed: %v", err)
			}
		}
	}
}

func waitForSchema(ctx context.Context, gormDB *gorm.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	requiredTables := []string{
		"users",
		"accounts",
		"ledger_tx",
		"ledger_entries",
		"deposit_chain_txs",
		"withdraw_requests",
		"chain_cursors",
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

func syncAllChains(ctx context.Context, runner *indexerdomain.Runner, chainRules []indexerdomain.ChainRule) error {
	for _, rule := range chainRules {
		if err := runner.SyncChain(ctx, rule.ChainID); err != nil {
			return err
		}
	}
	return nil
}

func buildChainRules(cfg config.StaticConfig) []indexerdomain.ChainRule {
	enabledChains := config.EnabledChains(cfg)
	rules := make([]indexerdomain.ChainRule, 0, len(enabledChains))
	for _, chain := range enabledChains {
		if chain.RPCURL == "" || chain.VaultAddress == "" || chain.USDCAddress == "" {
			continue
		}
		rules = append(rules, indexerdomain.ChainRule{
			ChainID:               chain.ChainID,
			Asset:                 chain.Asset,
			RequiredConfirmations: chain.Confirmations,
			VaultAddress:          chain.VaultAddress,
			TokenAddress:          chain.USDCAddress,
			FactoryAddress:        chain.FactoryAddress,
		})
	}
	return rules
}

func buildEVMConfigs(cfg config.StaticConfig) []chaininfra.EVMChainConfig {
	enabledChains := config.EnabledChains(cfg)
	out := make([]chaininfra.EVMChainConfig, 0, len(enabledChains))
	for _, chain := range enabledChains {
		if chain.RPCURL == "" {
			continue
		}
		out = append(out, chaininfra.EVMChainConfig{
			ChainID:        chain.ChainID,
			RPCURL:         chain.RPCURL,
			VaultAddress:   chain.VaultAddress,
			TokenAddress:   chain.USDCAddress,
			FactoryAddress: chain.FactoryAddress,
		})
	}
	return out
}
