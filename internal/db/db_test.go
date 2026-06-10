package db

import (
	"context"
	"path/filepath"
	"testing"

	"goralph/internal/db/sqlc"
)

func TestOpenUsesModerncSQLiteAndGeneratedQueries(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	got, err := sqlc.New(database).Ping(context.Background())
	if err != nil {
		t.Fatalf("run generated ping query: %v", err)
	}
	if got != 1 {
		t.Fatalf("ping = %d, want 1", got)
	}
}

func TestMigrateFreshDatabaseRecordsGooseVersion(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	var versionID int64
	var isApplied bool
	if err := database.QueryRow("SELECT version_id, is_applied FROM goose_db_version WHERE version_id = 1").Scan(&versionID, &isApplied); err != nil {
		t.Fatalf("query goose version: %v", err)
	}
	if versionID != 1 {
		t.Fatalf("version_id = %d, want 1", versionID)
	}
	if !isApplied {
		t.Fatalf("is_applied = false, want true")
	}
}
