package main

import (
	"context"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	withdrawexecdomain "github.com/xiaobao/rgperp/backend/internal/domain/withdrawexec"
	authinfra "github.com/xiaobao/rgperp/backend/internal/infra/auth"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/infra/db"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
	httptransport "github.com/xiaobao/rgperp/backend/internal/transport/http"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
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
	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

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
	confirmations := config.EnabledChainConfirmations(cfg)
	depositAddressRepo := db.NewDepositAddressRepository(gormDB, confirmations)
	walletQueryRepo := db.NewWalletQueryRepository(gormDB)
	var localNativeFaucet *chaininfra.LocalNativeFaucet
	if cfg.App.Env == "dev" && cfg.Review.LocalMinterPrivateKey != "" && cfg.Chains.Base.RPCURL != "" {
		faucet, err := chaininfra.NewLocalNativeFaucet(cfg.Review.LocalMinterPrivateKey, []chaininfra.LocalNativeFaucetChainConfig{{
			ChainID: config.EffectiveBaseChainID(cfg.App.Env),
			RPCURL:  cfg.Chains.Base.RPCURL,
		}}, big.NewInt(1_000_000_000_000_000_000))
		if err != nil {
			log.Fatalf("create local native faucet: %v", err)
		}
		defer faucet.Close()
		localNativeFaucet = faucet
	}
	walletService := walletdomain.NewService(
		db.NewDepositRepository(gormDB),
		db.NewWithdrawRepository(gormDB),
		userRepo,
		ledgerService,
		txManager,
		clockx.RealClock{},
		&idgen.TimeBasedGenerator{},
		db.NewAccountResolver(gormDB),
		db.NewBalanceRepository(gormDB),
		depositAddressRepo,
		allocator,
	)
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
			walletService.SetWithdrawRiskEvaluator(chaininfra.NewWithdrawRiskEvaluator(runtimeCfg.Global, runtimeCfg.Wallet, executor))
			withdrawExecService, err := withdrawexecdomain.NewService(db.NewWithdrawRepository(gormDB), walletService, executor)
			if err != nil {
				log.Fatalf("create withdraw executor service: %v", err)
			}
			go startWithdrawExecutorLoop(withdrawExecService, enabledChains)
		}
	}

	verifier := &accessVerifierAdapter{verifier: authinfra.NewJWTVerifier(cfg.Auth.AccessSecret)}
	authHandler := httptransport.NewAuthHandler(authService, cfg.Admin.Wallets)
	reviewReadRepo := db.NewReviewReadRepository()
	marketHandler := httptransport.NewMarketHandler(reviewReadRepo)
	accountHandler := httptransport.NewAccountHandler(db.NewAccountQueryRepository(gormDB), walletService, userRepo)
	walletHandler := httptransport.NewWalletHandler(
		db.NewWalletReadService(depositAddressRepo, walletQueryRepo, allocator),
		walletService,
		localNativeFaucet,
	)
	systemChains := buildSystemChains(enabledChains, cfg)
	systemHandler := httptransport.NewSystemHandler(httptransport.NewStaticSystemReader(systemChains))
	tradingHandler := httptransport.NewTradingHandler(reviewReadRepo)
	explorerHandler := httptransport.NewExplorerHandler(db.NewExplorerQueryRepository(gormDB), cfg.Admin.Wallets)
	adminHandler := httptransport.NewAdminHandler(walletService, walletQueryRepo, cfg.Admin.Wallets)

	router := httptransport.NewEngine(verifier, authHandler, marketHandler, accountHandler, walletHandler, tradingHandler, explorerHandler, adminHandler, systemHandler)

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
			LocalTestnet:      chain.Key == "local",
			LocalToolsEnabled: chain.Key == "local" && cfg.Review.LocalMinterPrivateKey != "" && chain.USDCAddress != "",
			DepositEnabled:    chain.RPCURL != "" && chain.FactoryAddress != "" && chain.USDCAddress != "",
			WithdrawEnabled:   chain.RPCURL != "" && chain.VaultAddress != "" && chain.USDCAddress != "",
			USDCAddress:       usdcAddress,
		})
	}
	return items
}

func startWithdrawExecutorLoop(service *withdrawexecdomain.Service, chains []config.EnabledChain) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
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
}

func (a *accessVerifierAdapter) VerifyAccessToken(token string) (httptransport.AccessClaims, error) {
	claims, err := a.verifier.VerifyAccessToken(token)
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
	}, nil
}
