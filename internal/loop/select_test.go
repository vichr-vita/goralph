package loop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vichr-vita/goralph/internal/db"
	"github.com/vichr-vita/goralph/internal/db/sqlc"
)

func TestSelectEligibleTaskChoosesPendingBeforeFailed(t *testing.T) {
	ctx := context.Background()
	queries := newTestQueries(t, ctx)
	projectID := createTestProject(t, ctx, queries)

	createTestTask(t, ctx, queries, projectID, "passed first", db.TaskStatusPassed)
	pending := createTestTask(t, ctx, queries, projectID, "pending task", db.TaskStatusPending)
	createTestTask(t, ctx, queries, projectID, "failed retry", db.TaskStatusFailed)

	selection, err := SelectEligibleTask(ctx, queries, projectID)
	if err != nil {
		t.Fatalf("select eligible task: %v", err)
	}
	if !selection.HasTask {
		t.Fatalf("selection has no task, want pending task")
	}
	if selection.Task.ID != pending.ID || selection.Task.Status != string(db.TaskStatusPending) {
		t.Fatalf("selected task = %+v, want pending task %+v", selection.Task, pending)
	}
}

func TestSelectEligibleTaskTreatsFailedAsEligibleRetry(t *testing.T) {
	ctx := context.Background()
	queries := newTestQueries(t, ctx)
	projectID := createTestProject(t, ctx, queries)

	createTestTask(t, ctx, queries, projectID, "passed", db.TaskStatusPassed)
	failed := createTestTask(t, ctx, queries, projectID, "failed retry", db.TaskStatusFailed)

	selection, err := SelectEligibleTask(ctx, queries, projectID)
	if err != nil {
		t.Fatalf("select eligible task: %v", err)
	}
	if !selection.HasTask || selection.Task.ID != failed.ID || selection.Task.Status != string(db.TaskStatusFailed) {
		t.Fatalf("selection = %+v, want failed task %+v", selection, failed)
	}
}

func TestSelectNonCompleteTasksExcludesPassedTasks(t *testing.T) {
	ctx := context.Background()
	queries := newTestQueries(t, ctx)
	projectID := createTestProject(t, ctx, queries)

	createTestTask(t, ctx, queries, projectID, "passed", db.TaskStatusPassed)
	pending := createTestTask(t, ctx, queries, projectID, "pending", db.TaskStatusPending)
	blocked := createTestTask(t, ctx, queries, projectID, "blocked", db.TaskStatusBlocked)
	inProgress := createTestTask(t, ctx, queries, projectID, "in progress", db.TaskStatusInProgress)
	failed := createTestTask(t, ctx, queries, projectID, "failed", db.TaskStatusFailed)

	tasks, err := SelectNonCompleteTasks(ctx, queries, projectID)
	if err != nil {
		t.Fatalf("select non-complete tasks: %v", err)
	}
	wantIDs := []int64{pending.ID, blocked.ID, inProgress.ID, failed.ID}
	if len(tasks) != len(wantIDs) {
		t.Fatalf("non-complete task count = %d, want %d; tasks=%+v", len(tasks), len(wantIDs), tasks)
	}
	for index, wantID := range wantIDs {
		if tasks[index].ID != wantID {
			t.Fatalf("non-complete task %d ID = %d, want %d; tasks=%+v", index, tasks[index].ID, wantID, tasks)
		}
	}
}

func TestSelectEligibleTaskReportsNoEligibleTasks(t *testing.T) {
	ctx := context.Background()
	queries := newTestQueries(t, ctx)
	projectID := createTestProject(t, ctx, queries)

	for _, tc := range []struct {
		description string
		status      db.TaskStatus
	}{
		{"passed", db.TaskStatusPassed},
		{"blocked", db.TaskStatusBlocked},
		{"in progress", db.TaskStatusInProgress},
	} {
		createTestTask(t, ctx, queries, projectID, tc.description, tc.status)
	}

	selection, err := SelectEligibleTask(ctx, queries, projectID)
	if err != nil {
		t.Fatalf("select eligible task: %v", err)
	}
	if selection.HasTask {
		t.Fatalf("selection = %+v, want no eligible tasks", selection)
	}
}

func newTestQueries(t *testing.T, ctx context.Context) *sqlc.Queries {
	t.Helper()

	database, err := db.Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}
	})

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return sqlc.New(database)
}

func createTestProject(t *testing.T, ctx context.Context, queries *sqlc.Queries) int64 {
	t.Helper()

	project, err := queries.CreateProject(ctx, sqlc.CreateProjectParams{
		Name:     "test-project",
		RootPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return project.ID
}

func createTestTask(t *testing.T, ctx context.Context, queries *sqlc.Queries, projectID int64, description string, status db.TaskStatus) sqlc.Task {
	t.Helper()

	task, err := queries.CreateTask(ctx, sqlc.CreateTaskParams{
		ProjectID:   projectID,
		Category:    "test",
		Description: description,
		Status:      string(status),
	})
	if err != nil {
		t.Fatalf("create task %q: %v", description, err)
	}
	return task
}
