package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
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

	chains := []db.ChainSpec{
		{ChainID: 1, Asset: "USDC", Confirmations: cfg.Chains.Ethereum.Confirmations},
		{ChainID: 42161, Asset: "USDC", Confirmations: cfg.Chains.Arbitrum.Confirmations},
		{ChainID: 8453, Asset: "USDC", Confirmations: cfg.Chains.Base.Confirmations},
	}
	bootstrap := db.NewBootstrapRepository(gormDB, chains, chaininfra.NewDeterministicDepositAddressAllocator())
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
	confirmations := map[int64]int{
		1:     cfg.Chains.Ethereum.Confirmations,
		42161: cfg.Chains.Arbitrum.Confirmations,
		8453:  cfg.Chains.Base.Confirmations,
	}
	depositAddressRepo := db.NewDepositAddressRepository(gormDB, confirmations)
	walletQueryRepo := db.NewWalletQueryRepository(gormDB)
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
	)

	verifier := &accessVerifierAdapter{verifier: authinfra.NewJWTVerifier(cfg.Auth.AccessSecret)}
	authHandler := httptransport.NewAuthHandler(authService)
	accountHandler := httptransport.NewAccountHandler(db.NewAccountQueryRepository(gormDB), walletService, userRepo)
	walletHandler := httptransport.NewWalletHandler(
		db.NewWalletReadService(depositAddressRepo, walletQueryRepo),
		walletService,
		cfg.App.Env == "review" && cfg.Review.FaucetEnabled && runtimeCfg.Review.Faucet.Enabled,
	)
	explorerHandler := httptransport.NewExplorerHandler(db.NewExplorerQueryRepository(gormDB), cfg.Admin.Wallets)
	adminHandler := httptransport.NewAdminHandler(walletService, cfg.Admin.Wallets)

	router := httptransport.NewEngine(verifier, authHandler, accountHandler, walletHandler, explorerHandler, adminHandler)

	addr := ":" + strconv.Itoa(cfg.App.Port)
	if err := router.Run(addr); err != nil {
		log.Fatalf("run api-server: %v", err)
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
