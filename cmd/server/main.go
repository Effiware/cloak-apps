package main

import (
	"log"

	"github.com/effiware/cloak-apps/internal/config"
	"github.com/effiware/cloak-apps/internal/server"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	httpServer := server.HttpServer(cfg.Server.Host, cfg.Server.Port, cfg.Server.Timeout)
	log.Printf("Running server on %s", httpServer.Addr)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
