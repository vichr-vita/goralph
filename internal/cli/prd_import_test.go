package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vichr-vita/goralph/internal/db"
	"github.com/vichr-vita/goralph/internal/db/sqlc"
	"github.com/vichr-vita/goralph/internal/prd"
)

func TestImportPRDItemsReplaceAndAppendAgainstTempDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ralph.db")
	rootPath := filepath.Join(t.TempDir(), "repo")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	project, err := sqlc.New(database).CreateProject(ctx, sqlc.CreateProjectParams{Name: "repo", RootPath: rootPath})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close setup database: %v", err)
	}

	result, err := importPRDItems(ctx, dbPath, project.ID, []prd.Item{
		{Category: "testing", Description: "seed", Steps: []string{"old step"}, Passes: false},
	}, importModeDefault, nil)
	if err != nil {
		t.Fatalf("seed import: %v", err)
	}
	if result.Imported != 1 || result.Mode != importModeReplace {
		t.Fatalf("seed result = %+v, want one replace import", result)
	}

	result, err = importPRDItems(ctx, dbPath, project.ID, []prd.Item{
		{Category: "testing", Description: "replacement", Steps: []string{"new step"}, Passes: true},
	}, importModeReplace, nil)
	if err != nil {
		t.Fatalf("replace import: %v", err)
	}
	if result.Imported != 1 || result.Mode != importModeReplace {
		t.Fatalf("replace result = %+v, want one replace import", result)
	}

	result, err = importPRDItems(ctx, dbPath, project.ID, []prd.Item{
		{Category: "testing", Description: "appended", Steps: []string{"append step"}, Passes: false},
	}, importModeAppend, nil)
	if err != nil {
		t.Fatalf("append import: %v", err)
	}
	if result.Imported != 1 || result.Mode != importModeAppend {
		t.Fatalf("append result = %+v, want one append import", result)
	}

	tasks := fetchImportedTasks(t, dbPath, rootPath)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if tasks[0].description != "replacement" || tasks[0].status != "passed" || tasks[0].steps != "new step" {
		t.Fatalf("first task = %+v, want passed replacement with new step", tasks[0])
	}
	if tasks[1].description != "appended" || tasks[1].status != "pending" || tasks[1].steps != "append step" {
		t.Fatalf("second task = %+v, want pending appended with append step", tasks[1])
	}
}
