package main

import (
	"log"
	"os"

	"github.com/xiaobao/rgperp/backend/internal/config"
	httptransport "github.com/xiaobao/rgperp/backend/internal/transport/http"
)

func main() {
	cfg, err := config.LoadStaticConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	_ = cfg

	router := httptransport.NewEngine(nil)

	addr := ":" + os.Getenv("APP_PORT")
	if addr == ":" {
		addr = ":8080"
	}
	if err := router.Run(addr); err != nil {
		log.Fatalf("run api-server: %v", err)
	}
}
