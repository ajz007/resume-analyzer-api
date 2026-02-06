package main

import (
	"log"

	"resume-backend/internal/bootstrap"
	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/server"
)

func main() {
	cfg := config.Load()
	app, err := bootstrap.Build(cfg)
	if err != nil {
		log.Fatalf("failed to bootstrap app: %v", err)
	}

	addr := server.Addr(cfg.Port)
	log.Printf("Starting API server on %s", addr)

	if err := app.Router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
