package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/vichr-vita/goralph/internal/db/sqlc"
)

func openMigratedTempDB(t *testing.T) (*sql.DB, *sqlc.Queries) {
	t.Helper()

	database, err := Open(filepath.Join(t.TempDir(), "ralph.db"))
	if err != nil {
		t.Fatalf("open temp database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate temp database: %v", err)
	}

	return database, sqlc.New(database)
}

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
	database, _ := openMigratedTempDB(t)

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

func TestMigrateFreshDatabaseEnforcesConstraintsAndForeignKeys(t *testing.T) {
	database, queries := openMigratedTempDB(t)
	ctx := context.Background()

	project, err := queries.CreateProject(ctx, sqlc.CreateProjectParams{Name: "test", RootPath: filepath.Join(t.TempDir(), "repo")})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := queries.CreateProject(ctx, sqlc.CreateProjectParams{Name: "dupe", RootPath: project.RootPath}); err == nil {
		t.Fatalf("create project with duplicate root_path succeeded, want unique constraint error")
	}

	task, err := queries.CreateTask(ctx, sqlc.CreateTaskParams{ProjectID: project.ID, Category: "testing", Description: "db tests", Status: "pending"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := database.Exec("INSERT INTO task (project_id, category, description, status) VALUES (?, 'testing', 'orphan', 'pending')", project.ID+1000); err == nil {
		t.Fatalf("insert task with missing project succeeded, want foreign key error")
	}

	if _, err := queries.CreateTaskStep(ctx, sqlc.CreateTaskStepParams{TaskID: task.ID, Position: 1, Description: "step one"}); err != nil {
		t.Fatalf("create task step: %v", err)
	}
	if _, err := queries.CreateTaskStep(ctx, sqlc.CreateTaskStepParams{TaskID: task.ID, Position: 1, Description: "duplicate"}); err == nil {
		t.Fatalf("create duplicate task step position succeeded, want unique constraint error")
	}
	if _, err := database.Exec("INSERT INTO task_step (task_id, position, description) VALUES (?, 1, 'orphan')", task.ID+1000); err == nil {
		t.Fatalf("insert task step with missing task succeeded, want foreign key error")
	}

	run, err := queries.CreateRun(ctx, sqlc.CreateRunParams{ProjectID: project.ID, TaskID: sql.NullInt64{Int64: task.ID, Valid: true}, RunnerName: "test", Host: "host"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := database.Exec("INSERT INTO run (project_id, runner_name, status) VALUES (?, 'test', 'running')", project.ID+1000); err == nil {
		t.Fatalf("insert run with missing project succeeded, want foreign key error")
	}
	if _, err := database.Exec("INSERT INTO run (project_id, task_id, runner_name, status) VALUES (?, ?, 'test', 'running')", project.ID, task.ID+1000); err == nil {
		t.Fatalf("insert run with missing task succeeded, want foreign key error")
	}

	progress, err := queries.CreateProgress(ctx, sqlc.CreateProgressParams{
		ProjectID: project.ID,
		TaskID:    sql.NullInt64{Int64: task.ID, Valid: true},
		RunID:     sql.NullInt64{Int64: run.ID, Valid: true},
		Summary:   "progress",
	})
	if err != nil {
		t.Fatalf("create progress: %v", err)
	}
	if _, err := database.Exec("INSERT INTO progress (project_id, task_id, run_id, summary) VALUES (?, ?, ?, 'orphan')", project.ID+1000, task.ID, run.ID); err == nil {
		t.Fatalf("insert progress with missing project succeeded, want foreign key error")
	}

	if _, err := queries.UpsertFeedbackCommand(ctx, sqlc.UpsertFeedbackCommandParams{ProjectID: project.ID, Name: "status", Command: "go test ./..."}); err != nil {
		t.Fatalf("upsert feedback command: %v", err)
	}
	if _, err := database.Exec("INSERT INTO feedback_command (project_id, name, command) VALUES (?, 'bad', 'noop')", project.ID+1000); err == nil {
		t.Fatalf("insert feedback command with missing project succeeded, want foreign key error")
	}

	if _, err := database.Exec("DELETE FROM task WHERE id = ?", task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var stepCount int64
	if err := database.QueryRow("SELECT COUNT(*) FROM task_step WHERE task_id = ?", task.ID).Scan(&stepCount); err != nil {
		t.Fatalf("count task steps after cascade: %v", err)
	}
	if stepCount != 0 {
		t.Fatalf("task steps after deleting task = %d, want 0", stepCount)
	}
	var progressTaskID sql.NullInt64
	if err := database.QueryRow("SELECT task_id FROM progress WHERE id = ?", progress.ID).Scan(&progressTaskID); err != nil {
		t.Fatalf("query progress task after task delete: %v", err)
	}
	if progressTaskID.Valid {
		t.Fatalf("progress task_id after deleting task valid = true, want false")
	}
}

func TestSQLCQueriesCoverDatabaseWorkflow(t *testing.T) {
	_, queries := openMigratedTempDB(t)
	ctx := context.Background()

	project, err := queries.CreateProject(ctx, sqlc.CreateProjectParams{Name: "alpha", RootPath: filepath.Join(t.TempDir(), "alpha")})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	gotProject, err := queries.GetProjectByRootPath(ctx, project.RootPath)
	if err != nil {
		t.Fatalf("get project by root path: %v", err)
	}
	if gotProject.ID != project.ID {
		t.Fatalf("project id = %d, want %d", gotProject.ID, project.ID)
	}
	updatedProject, err := queries.UpdateProject(ctx, sqlc.UpdateProjectParams{Name: "alpha-renamed", Description: "desc", ID: project.ID})
	if err != nil {
		t.Fatalf("update project: %v", err)
	}
	if updatedProject.Name != "alpha-renamed" || updatedProject.Description != "desc" {
		t.Fatalf("updated project = (%q, %q), want renamed desc", updatedProject.Name, updatedProject.Description)
	}

	taskOne, err := queries.CreateTask(ctx, sqlc.CreateTaskParams{ProjectID: project.ID, Category: "testing", Description: "one", Status: "pending"})
	if err != nil {
		t.Fatalf("create task one: %v", err)
	}
	taskTwo, err := queries.CreateTask(ctx, sqlc.CreateTaskParams{ProjectID: project.ID, Category: "testing", Description: "two", Status: "failed"})
	if err != nil {
		t.Fatalf("create task two: %v", err)
	}
	count, err := queries.CountTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 2 {
		t.Fatalf("task count = %d, want 2", count)
	}
	tasks, err := queries.ListTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 || tasks[0].ID != taskOne.ID || tasks[1].ID != taskTwo.ID {
		t.Fatalf("listed task ids = %+v, want [%d %d]", tasks, taskOne.ID, taskTwo.ID)
	}
	failedTasks, err := queries.ListTasksByProjectAndStatus(ctx, sqlc.ListTasksByProjectAndStatusParams{ProjectID: project.ID, Status: "failed"})
	if err != nil {
		t.Fatalf("list failed tasks: %v", err)
	}
	if len(failedTasks) != 1 || failedTasks[0].ID != taskTwo.ID {
		t.Fatalf("failed tasks = %+v, want task %d", failedTasks, taskTwo.ID)
	}
	gotTask, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{ProjectID: project.ID, ID: taskOne.ID})
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if gotTask.Description != "one" {
		t.Fatalf("task description = %q, want one", gotTask.Description)
	}
	updatedTask, err := queries.UpdateTask(ctx, sqlc.UpdateTaskParams{
		Category:       "testing",
		Description:    "one updated",
		Status:         "in_progress",
		ProgressReport: "half",
		ProjectID:      project.ID,
		ID:             taskOne.ID,
	})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if updatedTask.Status != "in_progress" || updatedTask.ProgressReport != "half" {
		t.Fatalf("updated task = (%q, %q), want in_progress half", updatedTask.Status, updatedTask.ProgressReport)
	}
	nextTask, err := queries.GetNextEligibleTaskByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("get next eligible task: %v", err)
	}
	if nextTask.ID != taskTwo.ID {
		t.Fatalf("next eligible task id = %d, want %d", nextTask.ID, taskTwo.ID)
	}

	stepOne, err := queries.CreateTaskStep(ctx, sqlc.CreateTaskStepParams{TaskID: taskOne.ID, Position: 2, Description: "second"})
	if err != nil {
		t.Fatalf("create step one: %v", err)
	}
	stepTwo, err := queries.CreateTaskStep(ctx, sqlc.CreateTaskStepParams{TaskID: taskOne.ID, Position: 1, Description: "first"})
	if err != nil {
		t.Fatalf("create step two: %v", err)
	}
	steps, err := queries.ListTaskStepsByTask(ctx, taskOne.ID)
	if err != nil {
		t.Fatalf("list task steps: %v", err)
	}
	if len(steps) != 2 || steps[0].ID != stepTwo.ID || steps[1].ID != stepOne.ID {
		t.Fatalf("steps = %+v, want position order", steps)
	}
	prdRows, err := queries.ListTaskPRDRowsByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list task PRD rows: %v", err)
	}
	if len(prdRows) != 3 {
		t.Fatalf("PRD row count = %d, want 3", len(prdRows))
	}

	runOne, err := queries.CreateRun(ctx, sqlc.CreateRunParams{ProjectID: project.ID, TaskID: sql.NullInt64{Int64: taskOne.ID, Valid: true}, RunnerName: "pi", Host: "host-a"})
	if err != nil {
		t.Fatalf("create run one: %v", err)
	}
	processedRun, err := queries.UpdateRunProcess(ctx, sqlc.UpdateRunProcessParams{Pid: sql.NullInt64{Int64: 123, Valid: true}, Host: "host-b", ProjectID: project.ID, ID: runOne.ID})
	if err != nil {
		t.Fatalf("update run process: %v", err)
	}
	if !processedRun.Pid.Valid || processedRun.Pid.Int64 != 123 || processedRun.Host != "host-b" {
		t.Fatalf("processed run pid/host = (%v, %q), want 123 host-b", processedRun.Pid, processedRun.Host)
	}
	if err := queries.UpdateRunHeartbeat(ctx, sqlc.UpdateRunHeartbeatParams{ProjectID: project.ID, ID: runOne.ID}); err != nil {
		t.Fatalf("update run heartbeat: %v", err)
	}
	activeRun, err := queries.GetActiveRunByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("get active run: %v", err)
	}
	if activeRun.ID != runOne.ID {
		t.Fatalf("active run id = %d, want %d", activeRun.ID, runOne.ID)
	}
	retargetedRun, err := queries.SetRunTaskID(ctx, sqlc.SetRunTaskIDParams{TaskID: sql.NullInt64{Int64: taskTwo.ID, Valid: true}, ProjectID: project.ID, ID: runOne.ID})
	if err != nil {
		t.Fatalf("set run task id: %v", err)
	}
	if !retargetedRun.TaskID.Valid || retargetedRun.TaskID.Int64 != taskTwo.ID {
		t.Fatalf("run task id = %v, want %d", retargetedRun.TaskID, taskTwo.ID)
	}
	failedRun, err := queries.MarkRunFailed(ctx, sqlc.MarkRunFailedParams{ExitError: sql.NullString{String: "boom", Valid: true}, ProjectID: project.ID, ID: runOne.ID})
	if err != nil {
		t.Fatalf("mark run failed: %v", err)
	}
	if failedRun.Status != "failed" || !failedRun.ExitError.Valid || failedRun.ExitError.String != "boom" {
		t.Fatalf("failed run status/error = (%q, %v), want failed boom", failedRun.Status, failedRun.ExitError)
	}
	runTwo, err := queries.CreateRun(ctx, sqlc.CreateRunParams{ProjectID: project.ID, TaskID: sql.NullInt64{Int64: taskTwo.ID, Valid: true}, RunnerName: "pi", Host: "host-c"})
	if err != nil {
		t.Fatalf("create run two: %v", err)
	}
	gotRun, err := queries.GetRunByProjectAndID(ctx, sqlc.GetRunByProjectAndIDParams{ProjectID: project.ID, ID: runTwo.ID})
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.ID != runTwo.ID {
		t.Fatalf("got run id = %d, want %d", gotRun.ID, runTwo.ID)
	}
	finishedRun, err := queries.FinishRun(ctx, sqlc.FinishRunParams{
		RunnerName:    "pi",
		RunnerVersion: "v1",
		RunnerModel:   "test-model",
		SessionID:     "session",
		SessionPath:   "/tmp/session.jsonl",
		Status:        "succeeded",
		ExitCode:      sql.NullInt64{Int64: 0, Valid: true},
		Host:          "host-c",
		ProjectID:     project.ID,
		ID:            runTwo.ID,
	})
	if err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if finishedRun.Status != "succeeded" || finishedRun.RunnerModel != "test-model" {
		t.Fatalf("finished run = (%q, %q), want succeeded test-model", finishedRun.Status, finishedRun.RunnerModel)
	}
	runs, err := queries.ListRunsByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("run count = %d, want 2", len(runs))
	}

	progressOne, err := queries.CreateProgress(ctx, sqlc.CreateProgressParams{
		ProjectID: project.ID,
		TaskID:    sql.NullInt64{Int64: taskOne.ID, Valid: true},
		RunID:     sql.NullInt64{Int64: runOne.ID, Valid: true},
		Summary:   "first progress",
	})
	if err != nil {
		t.Fatalf("create progress one: %v", err)
	}
	progressTwo, err := queries.CreateProgress(ctx, sqlc.CreateProgressParams{
		ProjectID: project.ID,
		TaskID:    sql.NullInt64{Int64: taskTwo.ID, Valid: true},
		RunID:     sql.NullInt64{Int64: runTwo.ID, Valid: true},
		Summary:   "second progress",
	})
	if err != nil {
		t.Fatalf("create progress two: %v", err)
	}
	projectProgress, err := queries.ListProgressByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list project progress: %v", err)
	}
	if len(projectProgress) != 2 || projectProgress[0].ID != progressTwo.ID || projectProgress[1].ID != progressOne.ID {
		t.Fatalf("project progress = %+v, want newest first", projectProgress)
	}
	taskProgress, err := queries.ListProgressByProjectAndTask(ctx, sqlc.ListProgressByProjectAndTaskParams{ProjectID: project.ID, TaskID: sql.NullInt64{Int64: taskOne.ID, Valid: true}})
	if err != nil {
		t.Fatalf("list task progress: %v", err)
	}
	if len(taskProgress) != 1 || taskProgress[0].ID != progressOne.ID {
		t.Fatalf("task progress = %+v, want progress %d", taskProgress, progressOne.ID)
	}
	runProgress, err := queries.ListProgressByRun(ctx, sqlc.ListProgressByRunParams{ProjectID: project.ID, RunID: sql.NullInt64{Int64: runTwo.ID, Valid: true}})
	if err != nil {
		t.Fatalf("list run progress: %v", err)
	}
	if len(runProgress) != 1 || runProgress[0].ID != progressTwo.ID {
		t.Fatalf("run progress = %+v, want progress %d", runProgress, progressTwo.ID)
	}
	latestProgress, err := queries.ListLatestProgressByTask(ctx, sqlc.ListLatestProgressByTaskParams{TaskID: sql.NullInt64{Int64: taskTwo.ID, Valid: true}, Limit: 1})
	if err != nil {
		t.Fatalf("list latest progress: %v", err)
	}
	if len(latestProgress) != 1 || latestProgress[0].ID != progressTwo.ID {
		t.Fatalf("latest progress = %+v, want progress %d", latestProgress, progressTwo.ID)
	}

	feedback, err := queries.UpsertFeedbackCommand(ctx, sqlc.UpsertFeedbackCommandParams{ProjectID: project.ID, Name: "retry", Command: "go test ./..."})
	if err != nil {
		t.Fatalf("upsert feedback: %v", err)
	}
	feedback, err = queries.UpsertFeedbackCommand(ctx, sqlc.UpsertFeedbackCommandParams{ProjectID: project.ID, Name: "retry", Command: "gofmt ./..."})
	if err != nil {
		t.Fatalf("update feedback: %v", err)
	}
	if feedback.Command != "gofmt ./..." {
		t.Fatalf("feedback command = %q, want gofmt ./...", feedback.Command)
	}
	gotFeedback, err := queries.GetFeedbackCommandByProjectAndName(ctx, sqlc.GetFeedbackCommandByProjectAndNameParams{ProjectID: project.ID, Name: "retry"})
	if err != nil {
		t.Fatalf("get feedback: %v", err)
	}
	if gotFeedback.ID != feedback.ID {
		t.Fatalf("feedback id = %d, want %d", gotFeedback.ID, feedback.ID)
	}
	feedbackCommands, err := queries.ListFeedbackCommandsByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("list feedback commands: %v", err)
	}
	if len(feedbackCommands) != 1 || feedbackCommands[0].Command != "gofmt ./..." {
		t.Fatalf("feedback commands = %+v, want updated retry", feedbackCommands)
	}

	if err := queries.DeleteTaskStepsByTask(ctx, taskOne.ID); err != nil {
		t.Fatalf("delete task steps by task: %v", err)
	}
	steps, err = queries.ListTaskStepsByTask(ctx, taskOne.ID)
	if err != nil {
		t.Fatalf("list task steps after delete: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("steps after delete = %d, want 0", len(steps))
	}
	if err := queries.DeleteTasksByProject(ctx, project.ID); err != nil {
		t.Fatalf("delete tasks by project: %v", err)
	}
	count, err = queries.CountTasksByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("count tasks after delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("task count after delete = %d, want 0", count)
	}
	if _, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{ProjectID: project.ID, ID: taskOne.ID}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get deleted task error = %v, want sql.ErrNoRows", err)
	}
}
