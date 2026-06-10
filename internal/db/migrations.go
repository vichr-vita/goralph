package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

var gooseMu sync.Mutex

// Migrate applies embedded Goose migrations to database.
func Migrate(ctx context.Context, database *sql.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetBaseFS(embeddedMigrations)
	defer goose.SetBaseFS(nil)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, database, migrationsDir); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}

	return nil
}
