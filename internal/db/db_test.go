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
