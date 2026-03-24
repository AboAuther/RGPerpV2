package main

import (
	"context"
	"log"
	"math/big"
	"strconv"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	adminopsapp "github.com/xiaobao/rgperp/backend/internal/app/adminops"
	posttradeapp "github.com/xiaobao/rgperp/backend/internal/app/posttrade"
	runtimeconfigapp "github.com/xiaobao/rgperp/backend/internal/app/runtimeconfig"
	"github.com/xiaobao/rgperp/backend/internal/config"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	"github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	withdrawexecdomain "github.com/xiaobao/rgperp/backend/internal/domain/withdrawexec"
	authinfra "github.com/xiaobao/rgperp/backend/internal/infra/auth"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/infra/db"
	"github.com/xiaobao/rgperp/backend/internal/infra/marketcache"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
	httptransport "github.com/xiaobao/rgperp/backend/internal/transport/http"
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
	if err := db.ConfigureMySQLConnectionPool(gormDB, cfg.MySQL.MaxOpenConns, cfg.MySQL.MaxIdleConns, time.Duration(cfg.MySQL.ConnMaxLifetimeSec)*time.Second); err != nil {
		log.Fatalf("configure mysql pool: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("migrate db: %v", err)
	}
	runtimeConfigRepo := db.NewRuntimeConfigRepository(gormDB)
	if err := runtimeConfigRepo.SeedDefaultRuntimeConfig(context.Background(), runtimeCfg, "bootstrap", time.Now().UTC()); err != nil {
		log.Fatalf("seed runtime config: %v", err)
	}
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

	enabledChains := config.EnabledChains(cfg)
	routerChains := make([]chaininfra.RouterAllocatorChainConfig, 0, len(enabledChains))
	for _, chain := range enabledChains {
		routerChains = append(routerChains, chaininfra.RouterAllocatorChainConfig{
			ChainID:        chain.ChainID,
			RPCURL:         chain.RPCURL,
			FactoryAddress: chain.FactoryAddress,
		})
	}
	allocator, err := chaininfra.NewRouterDepositAddressAllocator(cfg.Review.LocalMinterPrivateKey, routerChains)
	if err != nil {
		log.Fatalf("create deposit allocator: %v", err)
	}
	defer allocator.Close()
	bootstrap := db.NewBootstrapRepository(gormDB)
	if err := bootstrap.EnsureSystemBootstrap(context.Background()); err != nil {
		log.Fatalf("ensure system bootstrap: %v", err)
	}
	if err := bootstrap.EnsureMarketBootstrap(context.Background()); err != nil {
		log.Fatalf("ensure market bootstrap: %v", err)
	}
	latestMarketCache := marketcache.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if latestMarketCache != nil {
		defer latestMarketCache.Close()
	}

	// The API server keeps the core synchronous business path in one process so
	// monetary writes, policy checks, and durable outbox publication stay easy to
	// reason about and audit.
	txManager := db.NewTxManager(gormDB)
	userRepo := db.NewUserRepository(gormDB)
	authService := authdomain.NewService(
		authdomain.ServiceConfig{
			Domain:           cfg.Auth.Domain,
			NonceTTL:         time.Duration(runtimeCfg.Auth.NonceTTLSec) * time.Second,
			AccessTTL:        time.Duration(runtimeCfg.Auth.AccessTTLSec) * time.Second,
			RefreshTTL:       time.Duration(runtimeCfg.Auth.RefreshTTLSec) * time.Second,
			DefaultUserState: "ACTIVE",
		},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		db.NewNonceRepository(gormDB),
		userRepo,
		db.NewSessionRepository(gormDB),
		chaininfra.NewSignatureVerifier(),
		authinfra.NewJWTIssuer(cfg.Auth.AccessSecret, cfg.Auth.RefreshSecret),
		txManager,
		bootstrap,
	)

	ledgerService := ledgerdomain.NewService(
		db.NewLedgerRepository(gormDB),
		decimalx.LedgerDecimalFactory{},
	)
	accountResolver := db.NewAccountResolver(gormDB)
	fundingEventPublisher := db.NewFundingOutboxPublisher(gormDB)
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
		accountResolver,
		db.NewBalanceRepository(gormDB),
		ledgerService,
		db.NewOrderExecutionRepository(gormDB, latestMarketCache),
		db.NewOrderExecutionRepository(gormDB, latestMarketCache),
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
		txManager,
		db.NewRiskRepository(gormDB),
		db.NewRiskOutboxPublisher(gormDB),
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
		db.NewLiquidationRepository(gormDB),
		accountResolver,
		ledgerService,
		riskService,
		db.NewLiquidationOutboxPublisher(gormDB),
	)
	if err != nil {
		log.Fatalf("create liquidation service: %v", err)
	}
	liquidationService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	riskRequestProcessor := posttradeapp.NewProcessor(clockx.RealClock{}, &idgen.TimeBasedGenerator{}, db.NewRiskOutboxPublisher(gormDB))
	orderService.SetPostTradeRiskProcessor(riskRequestProcessor)
	runtimeConfigService, err := runtimeconfigapp.NewService(runtimeCfg, clockx.RealClock{}, &idgen.TimeBasedGenerator{}, txManager, runtimeConfigRepo, runtimeStore)
	if err != nil {
		log.Fatalf("create runtime config service: %v", err)
	}
	confirmations := config.EnabledChainConfirmations(cfg)
	// Deposit confirmations are resolved once from static chain config so every
	// wallet-facing read model exposes a consistent settlement policy.
	depositAddressRepo := db.NewDepositAddressRepository(gormDB, confirmations)
	vaultBalanceChains := make([]chaininfra.VaultBalanceReaderChainConfig, 0, len(enabledChains))
	for _, chain := range enabledChains {
		if chain.RPCURL == "" || chain.VaultAddress == "" || chain.USDCAddress == "" {
			continue
		}
		vaultBalanceChains = append(vaultBalanceChains, chaininfra.VaultBalanceReaderChainConfig{
			ChainID:      chain.ChainID,
			ChainKey:     chain.Key,
			ChainName:    config.ChainDisplayName(chain),
			Asset:        chain.Asset,
			RPCURL:       chain.RPCURL,
			VaultAddress: chain.VaultAddress,
			TokenAddress: chain.USDCAddress,
		})
	}
	var vaultBalanceReader *chaininfra.VaultBalanceReader
	if len(vaultBalanceChains) > 0 {
		reader, err := chaininfra.NewVaultBalanceReader(vaultBalanceChains)
		if err != nil {
			log.Fatalf("create vault balance reader: %v", err)
		}
		defer reader.Close()
		vaultBalanceReader = reader
	}
	walletQueryRepo := db.NewWalletQueryRepositoryWithRiskConfig(
		gormDB,
		confirmations,
		db.RiskMonitorConfig{
			HardLimitNotional:      runtimeCfg.Risk.NetExposureHardLimit,
			MaxExposureSlippageBps: runtimeCfg.Risk.MaxExposureSlippageBps,
		},
		vaultBalanceReader,
	)
	var localNativeFaucet *chaininfra.LocalNativeFaucet
	if cfg.App.Env == "dev" && cfg.Review.LocalMinterPrivateKey != "" {
		faucetChains := make([]chaininfra.LocalNativeFaucetChainConfig, 0, len(enabledChains))
		for _, chain := range enabledChains {
			if !chain.LocalTestnet || chain.RPCURL == "" {
				continue
			}
			faucetChains = append(faucetChains, chaininfra.LocalNativeFaucetChainConfig{
				ChainID: chain.ChainID,
				RPCURL:  chain.RPCURL,
			})
		}
		if len(faucetChains) > 0 {
			faucet, err := chaininfra.NewLocalNativeFaucet(cfg.Review.LocalMinterPrivateKey, faucetChains, big.NewInt(1_000_000_000_000_000_000))
			if err != nil {
				log.Fatalf("create local native faucet: %v", err)
			}
			defer faucet.Close()
			localNativeFaucet = faucet
		}
	}
	withdrawRepo := db.NewWithdrawRepository(gormDB)
	walletService := walletdomain.NewService(
		db.NewDepositRepository(gormDB, confirmations),
		withdrawRepo,
		userRepo,
		ledgerService,
		txManager,
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		accountResolver,
		db.NewBalanceRepository(gormDB),
		depositAddressRepo,
		allocator,
	)
	walletService.SetRuntimeConfigProvider(serviceRuntimeProvider)
	if cfg.Review.LocalMinterPrivateKey != "" {
		withdrawChains := make([]chaininfra.VaultWithdrawChainConfig, 0, len(enabledChains))
		for _, chain := range enabledChains {
			if chain.RPCURL == "" || chain.VaultAddress == "" || chain.USDCAddress == "" {
				continue
			}
			withdrawChains = append(withdrawChains, chaininfra.VaultWithdrawChainConfig{
				ChainID:      chain.ChainID,
				RPCURL:       chain.RPCURL,
				VaultAddress: chain.VaultAddress,
				TokenAddress: chain.USDCAddress,
			})
		}
		if len(withdrawChains) > 0 {
			executor, err := chaininfra.NewVaultWithdrawExecutor(cfg.Review.LocalMinterPrivateKey, withdrawChains)
			if err != nil {
				log.Fatalf("create vault withdraw executor: %v", err)
			}
			defer executor.Close()
			walletService.SetWithdrawRiskEvaluator(chaininfra.NewWithdrawRiskEvaluator(runtimeCfg.Global, runtimeCfg.Wallet, executor, withdrawRepo))
			if cfg.API.WithdrawExecutorEnabled {
				withdrawExecService, err := withdrawexecdomain.NewService(withdrawRepo, walletService, executor, txManager)
				if err != nil {
					log.Fatalf("create withdraw executor service: %v", err)
				}
				log.Printf("withdraw executor loop enabled in api-server; keep enabled on only one api-server instance when scaling horizontally")
				go startWithdrawExecutorLoop(withdrawExecService, enabledChains)
			} else {
				log.Printf("withdraw executor loop disabled for this api-server instance")
			}
		}
	}

	sessionRepo := db.NewSessionRepository(gormDB)
	verifier := &accessVerifierAdapter{
		verifier: authinfra.NewJWTVerifier(cfg.Auth.AccessSecret, cfg.Auth.RefreshSecret),
		sessions: sessionRepo,
	}
	authHandler := httptransport.NewAuthHandler(authService, verifier, cfg.Admin.Wallets)
	marketReadRepo := db.NewMarketReadRepository(gormDB, latestMarketCache, time.Duration(runtimeCfg.Market.MaxSourceAgeSec)*time.Second, serviceRuntimeProvider, serviceRuntimeProvider)
	tradingReadRepo := db.NewTradingReadRepository(gormDB)
	marketHandler := httptransport.NewMarketHandler(marketReadRepo)
	accountHandler := httptransport.NewAccountHandler(db.NewAccountQueryRepositoryWithRuntime(gormDB, serviceRuntimeProvider), walletService, userRepo)
	walletHandler := httptransport.NewWalletHandler(
		db.NewWalletReadService(depositAddressRepo, walletQueryRepo, allocator),
		walletService,
		localNativeFaucet,
	)
	systemChains := buildSystemChains(enabledChains, cfg)
	systemHandler := httptransport.NewSystemHandler(httptransport.NewStaticSystemReader(systemChains))
	tradingHandler := httptransport.NewTradingHandler(tradingReadRepo, orderService)
	explorerHandler := httptransport.NewExplorerHandler(db.NewExplorerQueryRepository(gormDB), cfg.Admin.Wallets)
	adminHandler := httptransport.NewAdminHandler(walletService, walletQueryRepo, cfg.Admin.Wallets)
	adminHandler.SetRiskMutator(riskRequestProcessor)
	adminHandler.SetConfigManager(adminRuntimeConfigManager{service: runtimeConfigService})
	adminOpsService, err := adminopsapp.NewService(
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		txManager,
		accountResolver,
		ledgerService,
		db.NewLiquidationRepository(gormDB),
		liquidationService,
	)
	if err != nil {
		log.Fatalf("create admin ops service: %v", err)
	}
	adminHandler.SetLedgerMutator(adminLedgerManager{service: adminOpsService})
	adminHandler.SetLiquidationMutator(adminLiquidationManager{service: adminOpsService})
	fundingReverser, err := fundingdomain.NewReverser(
		fundingdomain.ReverserConfig{Asset: "USDC"},
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		txManager,
		db.NewFundingRepository(gormDB),
		accountResolver,
		ledgerService,
		fundingEventPublisher,
	)
	if err != nil {
		log.Fatalf("create funding reverser: %v", err)
	}
	adminHandler.SetFundingMutator(adminFundingManager{
		reverser: fundingReverser,
		reader:   walletQueryRepo,
		outbox:   db.NewOutboxRepository(gormDB),
		idgen:    &idgen.TimeBasedGenerator{},
		db:       gormDB,
	})

	router := httptransport.NewEngine(verifier, httpRuntimeConfigProvider{store: runtimeStore}, authHandler, marketHandler, accountHandler, walletHandler, tradingHandler, explorerHandler, adminHandler, systemHandler)

	addr := ":" + strconv.Itoa(cfg.App.Port)
	if err := router.Run(addr); err != nil {
		log.Fatalf("run api-server: %v", err)
	}
}

func buildSystemChains(chains []config.EnabledChain, cfg config.StaticConfig) []readmodel.SystemChainItem {
	items := make([]readmodel.SystemChainItem, 0, len(chains))
	for _, chain := range chains {
		var usdcAddress *string
		if chain.USDCAddress != "" {
			value := chain.USDCAddress
			usdcAddress = &value
		}
		items = append(items, readmodel.SystemChainItem{
			ChainID:           chain.ChainID,
			Key:               chain.Key,
			Name:              config.ChainDisplayName(chain),
			Asset:             chain.Asset,
			Confirmations:     chain.Confirmations,
			LocalTestnet:      chain.LocalTestnet,
			LocalToolsEnabled: chain.LocalTestnet && cfg.Review.LocalMinterPrivateKey != "" && chain.USDCAddress != "",
			DepositEnabled:    chain.RPCURL != "" && chain.FactoryAddress != "" && chain.USDCAddress != "",
			WithdrawEnabled:   chain.RPCURL != "" && chain.VaultAddress != "" && chain.USDCAddress != "",
			USDCAddress:       usdcAddress,
		})
	}
	return items
}

type adminRuntimeConfigManager struct {
	service runtimeConfigService
}

type runtimeConfigService interface {
	GetRuntimeConfigView(ctx context.Context, limit int) (readmodel.RuntimeConfigView, error)
	UpdateRuntimeConfig(ctx context.Context, input runtimeconfigapp.UpdateInput) (readmodel.RuntimeConfigView, error)
}

type adminFundingManager struct {
	reverser *fundingdomain.Reverser
	reader   *db.WalletQueryRepository
	outbox   *db.OutboxRepository
	idgen    *idgen.TimeBasedGenerator
	db       *gorm.DB
}

func (m adminRuntimeConfigManager) GetRuntimeConfigView(ctx context.Context, limit int) (readmodel.RuntimeConfigView, error) {
	return m.service.GetRuntimeConfigView(ctx, limit)
}

func (m adminRuntimeConfigManager) UpdateRuntimeConfig(ctx context.Context, input httptransport.AdminRuntimeConfigUpdateInput) (readmodel.RuntimeConfigView, error) {
	return m.service.UpdateRuntimeConfig(ctx, runtimeconfigapp.UpdateInput{
		OperatorID: input.OperatorID,
		TraceID:    input.TraceID,
		Reason:     input.Reason,
		Values:     input.Values,
		PairValues: input.PairValues,
	})
}

func (m adminFundingManager) ReverseFundingBatch(ctx context.Context, fundingBatchID string, operatorID string, traceID string, reason string) (readmodel.AdminFundingBatchItem, error) {
	if _, err := m.reverser.ReverseBatch(ctx, fundingdomain.ReverseBatchInput{
		FundingBatchID: fundingBatchID,
		OperatorID:     operatorID,
		TraceID:        traceID,
		Reason:         reason,
	}); err != nil {
		return readmodel.AdminFundingBatchItem{}, err
	}
	items, err := m.reader.ListFundingBatches(ctx, 200)
	if err != nil {
		return readmodel.AdminFundingBatchItem{}, err
	}
	var target *readmodel.AdminFundingBatchItem
	for idx := range items {
		if items[idx].FundingBatchID == fundingBatchID {
			target = &items[idx]
			break
		}
	}
	if target == nil {
		return readmodel.AdminFundingBatchItem{}, errorsx.ErrNotFound
	}
	return *target, m.enqueueRiskRecalculations(ctx, fundingBatchID)
}

func (m adminFundingManager) enqueueRiskRecalculations(ctx context.Context, fundingBatchID string) error {
	var userIDs []uint64
	if err := m.db.WithContext(ctx).Raw(`
SELECT DISTINCT user_id
FROM funding_batch_items
WHERE funding_batch_id = ? AND user_id <> 0
`, fundingBatchID).Scan(&userIDs).Error; err != nil {
		return err
	}
	for _, userID := range userIDs {
		requestID := m.idgen.NewID("rrq")
		if err := m.outbox.Create(ctx, db.OutboxMessage{
			EventID:       m.idgen.NewID("evt"),
			AggregateType: "risk_recalculation",
			AggregateID:   requestID,
			EventType:     "risk.recalculate.requested",
			Payload: map[string]any{
				"request_id":   requestID,
				"user_id":      userID,
				"triggered_by": "funding_reversal",
				"trace_id":     traceIDForFundingReverse(fundingBatchID),
			},
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func traceIDForFundingReverse(fundingBatchID string) string {
	return "funding:reverse:" + fundingBatchID
}

func startWithdrawExecutorLoop(service *withdrawexecdomain.Service, chains []config.EnabledChain) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		// This loop must stay single-active across api-server replicas because
		// nonce reservation and on-chain broadcast are serialized per signer.
		for _, chain := range chains {
			if err := service.ProcessChain(context.Background(), chain.ChainID, 50); err != nil {
				log.Printf("withdraw executor sync failed: chain_id=%d err=%v", chain.ChainID, err)
			}
		}
		<-ticker.C
	}
}

type accessVerifierAdapter struct {
	verifier *authinfra.JWTVerifier
	sessions interface {
		GetActiveByAccessJTI(ctx context.Context, accessJTI string) (authdomain.Session, error)
	}
}

type httpRuntimeConfigProvider struct {
	store *config.RuntimeConfigStore
}

func (p httpRuntimeConfigProvider) CurrentHTTPRuntimeConfig() httptransport.HTTPRuntimeConfig {
	if p.store == nil {
		return httptransport.HTTPRuntimeConfig{}
	}
	current := p.store.Current()
	return httptransport.HTTPRuntimeConfig{
		TraceHeaderRequired: current.Global.TraceHeaderRequired,
	}
}

func (a *accessVerifierAdapter) VerifyAccessToken(token string) (httptransport.AccessClaims, error) {
	claims, err := a.verifier.VerifyAccessToken(token)
	if err != nil {
		return httptransport.AccessClaims{}, err
	}
	if claims.UserID == "" {
		return httptransport.AccessClaims{}, errorsx.ErrUnauthorized
	}
	if a.sessions == nil {
		return httptransport.AccessClaims{}, errorsx.ErrUnauthorized
	}
	session, err := a.sessions.GetActiveByAccessJTI(context.Background(), claims.JTI)
	if err != nil {
		return httptransport.AccessClaims{}, errorsx.ErrUnauthorized
	}
	if session.UserID == 0 || session.UserID != parseUint64OrZero(claims.UserID) {
		return httptransport.AccessClaims{}, errorsx.ErrUnauthorized
	}
	return httptransport.AccessClaims{
		UserID:    claims.UserID,
		Address:   claims.Address,
		SessionID: claims.SessionID,
		JTI:       claims.JTI,
	}, nil
}

func parseUint64OrZero(raw string) uint64 {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func (a *accessVerifierAdapter) VerifyRefreshToken(token string) (httptransport.AccessClaims, error) {
	claims, err := a.verifier.VerifyRefreshToken(token)
	if err != nil {
		return httptransport.AccessClaims{}, err
	}
	if claims.UserID == "" {
		return httptransport.AccessClaims{}, errorsx.ErrUnauthorized
	}
	return httptransport.AccessClaims{
		UserID:    claims.UserID,
		Address:   claims.Address,
		SessionID: claims.SessionID,
		JTI:       claims.JTI,
	}, nil
}
