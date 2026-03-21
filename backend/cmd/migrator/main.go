package main

import (
	"context"
	"log"

	"github.com/xiaobao/rgperp/backend/internal/config"
	"github.com/xiaobao/rgperp/backend/internal/infra/db"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.LoadStaticConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	gormDB, err := gorm.Open(gormmysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	bootstrap := db.NewBootstrapRepository(gormDB)
	if err := bootstrap.EnsureSystemBootstrap(context.Background()); err != nil {
		log.Fatalf("ensure system bootstrap: %v", err)
	}

	log.Printf("migration completed")
}
