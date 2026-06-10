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

func TestMigrateFreshDatabaseEnforcesTaskStatusEnum(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	result, err := database.Exec("INSERT INTO project (name, root_path) VALUES (?, ?)", "test", t.TempDir())
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projectID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("project id: %v", err)
	}

	for _, status := range []string{"pending", "in_progress", "blocked", "passed", "failed"} {
		t.Run(status, func(t *testing.T) {
			_, err := database.Exec(
				"INSERT INTO task (project_id, category, description, status) VALUES (?, ?, ?, ?)",
				projectID,
				"database",
				"task "+status,
				status,
			)
			if err != nil {
				t.Fatalf("insert task with status %q: %v", status, err)
			}
		})
	}

	for _, status := range []string{"unknown", "completed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			_, err := database.Exec(
				"INSERT INTO task (project_id, category, description, status) VALUES (?, ?, ?, ?)",
				projectID,
				"database",
				"task "+status,
				status,
			)
			if err == nil {
				t.Fatalf("insert task with status %q succeeded, want constraint error", status)
			}
		})
	}
}

func TestMigrateFreshDatabaseAllowsOnlyOneActiveRunPerProject(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	result, err := database.Exec("INSERT INTO project (name, root_path) VALUES (?, ?)", "test", t.TempDir())
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projectID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("project id: %v", err)
	}

	if _, err := database.Exec("INSERT INTO run (project_id, runner_name, status) VALUES (?, 'pi', 'running')", projectID); err != nil {
		t.Fatalf("insert first active run: %v", err)
	}
	if _, err := database.Exec("INSERT INTO run (project_id, runner_name, status) VALUES (?, 'pi', 'running')", projectID); err == nil {
		t.Fatalf("insert second active run succeeded, want unique constraint error")
	}
	if _, err := database.Exec("INSERT INTO run (project_id, runner_name, status) VALUES (?, 'pi', 'failed')", projectID); err != nil {
		t.Fatalf("insert finished run beside active run: %v", err)
	}
}

func TestMigrateFreshDatabaseCreatesNormalizedTables(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	wantTables := []string{
		"project",
		"task",
		"task_step",
		"progress",
		"run",
		"feedback_command",
	}
	for _, table := range wantTables {
		t.Run(table, func(t *testing.T) {
			var name string
			if err := database.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
				t.Fatalf("query table %s: %v", table, err)
			}
			if name != table {
				t.Fatalf("table name = %q, want %q", name, table)
			}
		})
	}
}
