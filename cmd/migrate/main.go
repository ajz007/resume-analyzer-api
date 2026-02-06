package main

// Run database migrations:
//   go run ./cmd/migrate

import (
	"context"
	"log"
	"os"

	"resume-backend/internal/shared/config"
	"resume-backend/internal/shared/storage/db"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	opts := db.OptionsFromEnv(db.DefaultMigrateOptions())
	sqlDB, err := db.Connect(ctx, cfg.DatabaseURL, opts)
	if err != nil {
		log.Printf("failed to connect database: %v", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	if err := db.RunMigrations(ctx, sqlDB); err != nil {
		log.Printf("failed to run migrations: %v", err)
		os.Exit(1)
	}
}
