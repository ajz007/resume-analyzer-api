package main

import (
	"log"

	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/server"
)

func main() {
	cfg := config.Load()
	r := server.NewRouter(cfg)

	addr := server.Addr(cfg.Port)
	log.Printf("Starting API server on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
