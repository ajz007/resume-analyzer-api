package db

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations applies embedded SQL migrations via goose. If database is nil, it's a no-op.
func RunMigrations(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return nil
	}
	goose.SetBaseFS(migrationFiles)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, database, "migrations")
}
