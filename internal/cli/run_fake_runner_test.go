package cli

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"goralph/internal/runner"
)

type fakeAgentRunner struct {
	command string
	args    []string
	run     func(context.Context, runner.Request, int) (runner.Result, error)
	turns   *int32
}

func (r fakeAgentRunner) Run(ctx context.Context, req runner.Request) (runner.Result, error) {
	turn := int(atomic.AddInt32(r.turns, 1))
	startedAt := time.Now()
	metadata := runner.Metadata{
		RunnerName:    "fake",
		RunnerVersion: "test-version",
		RunnerModel:   "test-model",
		SessionID:     "fake-session-" + strconv.Itoa(turn),
		Command:       r.command,
		Args:          r.args,
		PID:           424242,
		Host:          "fake-host",
		StartedAt:     startedAt,
		ExitCode:      0,
	}
	if req.OnStart != nil {
		req.OnStart(metadata)
	}

	result, err := r.run(ctx, req, turn)
	if result.Metadata.RunnerName == "" {
		metadata.FinishedAt = time.Now()
		result.Metadata = metadata
	}
	if result.Stdout != "" && req.Stdout != nil && !req.Quiet {
		_, _ = io.WriteString(req.Stdout, result.Stdout)
	}
	if result.Stderr != "" && req.Stderr != nil && !req.Quiet {
		_, _ = io.WriteString(req.Stderr, result.Stderr)
	}
	return result, err
}

func installFakeAgentRunner(t *testing.T, run func(context.Context, runner.Request, int) (runner.Result, error)) *int32 {
	t.Helper()
	previous := newAgentRunner
	var turns int32
	newAgentRunner = func(command string, args []string) runner.Runner {
		if command == "pi" {
			t.Fatalf("fake runner test configured real Pi command")
		}
		return fakeAgentRunner{command: command, args: args, run: run, turns: &turns}
	}
	t.Cleanup(func() { newAgentRunner = previous })
	return &turns
}

func setupFakeRunnerProject(t *testing.T, prdJSON string) (string, string) {
	t.Helper()
	dbPath := isolateCommandEnv(t)
	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	prdPath := writePRDFile(t, prdJSON)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	return dbPath, repoRoot
}

func executeRunCommand(t *testing.T, dbPath string, args ...string) (string, error) {
	t.Helper()
	stdout := &bytes.Buffer{}
	cmd := NewRootCommand()
	cmd.SetArgs(append([]string{"--db", dbPath}, args...))
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	return stdout.String(), err
}

func fakePassNextPendingTask(t *testing.T, dbPath string) int64 {
	t.Helper()
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var taskID int64
	if err := database.QueryRow("SELECT id FROM task WHERE status = 'pending' ORDER BY id LIMIT 1").Scan(&taskID); err != nil {
		t.Fatalf("select pending task: %v", err)
	}
	fakePassTask(t, dbPath, taskID)
	return taskID
}

func fakePassTask(t *testing.T, dbPath string, taskID int64) {
	t.Helper()
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec("UPDATE task SET status = 'passed', updated_at = CURRENT_TIMESTAMP WHERE id = ?", taskID); err != nil {
		t.Fatalf("pass task: %v", err)
	}
	if _, err := database.Exec(`
INSERT INTO progress (project_id, task_id, run_id, summary)
SELECT project_id, id, (SELECT id FROM run WHERE status = 'running' ORDER BY id DESC LIMIT 1), 'fake runner passed task'
FROM task
WHERE id = ?`, taskID); err != nil {
		t.Fatalf("insert fake progress: %v", err)
	}
}

func setTaskStatus(t *testing.T, dbPath string, taskID int64, status string) {
	t.Helper()
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec("UPDATE task SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", status, taskID); err != nil {
		t.Fatalf("set task status: %v", err)
	}
}

func assertRunFailedWithExitError(t *testing.T, dbPath string, runID int64, wantExitError string) {
	t.Helper()
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var status string
	var exitError sql.NullString
	if err := database.QueryRow("SELECT status, exit_error FROM run WHERE id = ?", runID).Scan(&status, &exitError); err != nil {
		t.Fatalf("query run %d: %v", runID, err)
	}
	if status != "failed" || !exitError.Valid || !strings.Contains(exitError.String, wantExitError) {
		t.Fatalf("run %d status=%q exit_error=%v, want failed containing %q", runID, status, exitError, wantExitError)
	}
}

func assertTaskStatuses(t *testing.T, dbPath string, repoRoot string, want ...string) {
	t.Helper()
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != len(want) {
		t.Fatalf("task count = %d, want %d", len(tasks), len(want))
	}
	for i, task := range tasks {
		if task.status != want[i] {
			t.Fatalf("task %d status = %q, want %q; tasks=%+v", i, task.status, want[i], tasks)
		}
	}
}

func TestFakeRunnerRunOneExecutesExactlyOneTurn(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"first","steps":["one"],"passes":false},
		{"category":"runner","description":"second","steps":["two"],"passes":false}
	]`)
	turns := installFakeAgentRunner(t, func(_ context.Context, req runner.Request, _ int) (runner.Result, error) {
		if !strings.Contains(req.Prompt, "Eligible tasks, highest priority first") {
			t.Fatalf("prompt missing eligible task list:\n%s", req.Prompt)
		}
		fakePassNextPendingTask(t, dbPath)
		return runner.Result{}, nil
	})

	stdout, err := executeRunCommand(t, dbPath, "run", "--quiet", "one")
	if err != nil {
		t.Fatalf("execute run one: %v", err)
	}
	if !strings.Contains(stdout, "Status: succeeded") {
		t.Fatalf("run output = %q, want succeeded", stdout)
	}
	if got := atomic.LoadInt32(turns); got != 1 {
		t.Fatalf("fake runner turns = %d, want 1", got)
	}
	assertRunCount(t, dbPath, 1)
	assertTaskStatuses(t, dbPath, repoRoot, "passed", "pending")
}

func TestFakeRunnerRunOneTaskFlagForcesSpecifiedTask(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"default first","steps":["one"],"passes":false},
		{"category":"runner","description":"forced second","steps":["two"],"passes":false}
	]`)
	forcedTaskID := fetchImportedTasks(t, dbPath, repoRoot)[1].id
	turns := installFakeAgentRunner(t, func(_ context.Context, req runner.Request, _ int) (runner.Result, error) {
		for _, want := range []string{"Forced task from --task:", "Task ID: " + strconv.FormatInt(forcedTaskID, 10), "Description: forced second"} {
			if !strings.Contains(req.Prompt, want) {
				t.Fatalf("forced prompt missing %q:\n%s", want, req.Prompt)
			}
		}
		if strings.Contains(req.Prompt, "Description: default first") {
			t.Fatalf("forced prompt included default task:\n%s", req.Prompt)
		}
		fakePassTask(t, dbPath, forcedTaskID)
		return runner.Result{}, nil
	})

	stdout, err := executeRunCommand(t, dbPath, "run", "--quiet", "one", "--task", strconv.FormatInt(forcedTaskID, 10))
	if err != nil {
		t.Fatalf("execute run one --task: %v", err)
	}
	if !strings.Contains(stdout, "Task: "+strconv.FormatInt(forcedTaskID, 10)) {
		t.Fatalf("run output = %q, want forced task", stdout)
	}
	if got := atomic.LoadInt32(turns); got != 1 {
		t.Fatalf("fake runner turns = %d, want 1", got)
	}
	assertTaskStatuses(t, dbPath, repoRoot, "pending", "passed")
}

func TestFakeRunnerRunOneRejectsActiveRunInSameProject(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"guard same project","steps":["one"],"passes":false}
	]`)
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id
	activeRunID := seedActiveRun(t, dbPath, repoRoot, taskID)
	turns := installFakeAgentRunner(t, func(_ context.Context, _ runner.Request, _ int) (runner.Result, error) {
		t.Fatal("runner executed despite active run")
		return runner.Result{}, nil
	})

	_, err := executeRunCommand(t, dbPath, "run", "--quiet", "one")
	wantRun := "run " + strconv.FormatInt(activeRunID, 10)
	for _, want := range []string{"active run exists", wantRun, "goralph run show"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("run one active error = %v, want %q", err, want)
		}
	}
	if got := atomic.LoadInt32(turns); got != 0 {
		t.Fatalf("fake runner turns = %d, want 0", got)
	}
	assertRunCount(t, dbPath, 1)
}

func TestFakeRunnerRunOneRecoversStaleSameHostOldHeartbeatWithForce(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"old heartbeat","steps":["one"],"passes":false}
	]`)
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id
	host, _ := os.Hostname()
	oldHeartbeat := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	staleRunID := seedActiveRunWithMetadata(t, dbPath, repoRoot, taskID, 0, host, oldHeartbeat)
	turns := installFakeAgentRunner(t, func(_ context.Context, _ runner.Request, _ int) (runner.Result, error) {
		fakePassTask(t, dbPath, taskID)
		return runner.Result{}, nil
	})

	_, err := executeRunCommand(t, dbPath, "run", "--stale-after", "1h", "one")
	if err == nil || !strings.Contains(err.Error(), "stale active run requires --force") || !strings.Contains(err.Error(), "heartbeat older than 1h0m0s") {
		t.Fatalf("run one old heartbeat error = %v, want force-required heartbeat error", err)
	}
	assertRunCount(t, dbPath, 1)

	stdout, err := executeRunCommand(t, dbPath, "run", "--stale-after", "1h", "--force", "--quiet", "one")
	if err != nil {
		t.Fatalf("execute run one --force old heartbeat: %v", err)
	}
	if !strings.Contains(stdout, "Status: succeeded") {
		t.Fatalf("run output = %q, want succeeded", stdout)
	}
	if got := atomic.LoadInt32(turns); got != 1 {
		t.Fatalf("fake runner turns = %d, want 1", got)
	}
	assertRunCount(t, dbPath, 2)
	assertRunFailedWithExitError(t, dbPath, staleRunID, "stale active run recovered with --force: heartbeat older than 1h0m0s")
}

func TestFakeRunnerRunOneCrossHostStaleHeartbeatRequiresForce(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"cross host stale","steps":["one"],"passes":false}
	]`)
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id
	oldHeartbeat := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	staleRunID := seedActiveRunWithMetadata(t, dbPath, repoRoot, taskID, 0, "other-host", oldHeartbeat)
	turns := installFakeAgentRunner(t, func(_ context.Context, _ runner.Request, _ int) (runner.Result, error) {
		fakePassTask(t, dbPath, taskID)
		return runner.Result{}, nil
	})

	_, err := executeRunCommand(t, dbPath, "run", "--stale-after", "1h", "one")
	if err == nil || !strings.Contains(err.Error(), "stale active run requires --force") || !strings.Contains(err.Error(), "cross-host") {
		t.Fatalf("run one cross-host stale error = %v, want force-required cross-host error", err)
	}
	if got := atomic.LoadInt32(turns); got != 0 {
		t.Fatalf("fake runner turns = %d, want 0", got)
	}
	assertRunCount(t, dbPath, 1)

	stdout, err := executeRunCommand(t, dbPath, "run", "--stale-after", "1h", "--force", "--quiet", "one")
	if err != nil {
		t.Fatalf("execute run one --force cross-host stale: %v", err)
	}
	if !strings.Contains(stdout, "Status: succeeded") {
		t.Fatalf("run output = %q, want succeeded", stdout)
	}
	if got := atomic.LoadInt32(turns); got != 1 {
		t.Fatalf("fake runner turns = %d, want 1", got)
	}
	assertRunCount(t, dbPath, 2)
	assertRunFailedWithExitError(t, dbPath, staleRunID, "stale active run recovered with --force: cross-host heartbeat from other-host older than 1h0m0s")
}

func TestFakeRunnerRunAllStopsOnAllPassed(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"first","steps":["one"],"passes":false},
		{"category":"runner","description":"second","steps":["two"],"passes":false}
	]`)
	turns := installFakeAgentRunner(t, func(_ context.Context, _ runner.Request, _ int) (runner.Result, error) {
		fakePassNextPendingTask(t, dbPath)
		return runner.Result{}, nil
	})

	stdout, err := executeRunCommand(t, dbPath, "run", "--quiet", "all", "--max-turns", "5")
	if err != nil {
		t.Fatalf("execute run all: %v", err)
	}
	if !strings.Contains(stdout, "All tasks passed") {
		t.Fatalf("run all output = %q, want all passed", stdout)
	}
	if got := atomic.LoadInt32(turns); got != 2 {
		t.Fatalf("fake runner turns = %d, want 2", got)
	}
	assertRunCount(t, dbPath, 2)
	assertTaskStatuses(t, dbPath, repoRoot, "passed", "passed")
}

func TestFakeRunnerRunAllStopsOnCompletePromise(t *testing.T) {
	dbPath, repoRoot := setupFakeRunnerProject(t, `[
		{"category":"runner","description":"first remains","steps":["one"],"passes":false},
		{"category":"runner","description":"second remains","steps":["two"],"passes":false}
	]`)
	turns := installFakeAgentRunner(t, func(_ context.Context, _ runner.Request, _ int) (runner.Result, error) {
		return runner.Result{Stdout: "<promise>COMPLETE</promise>\n"}, nil
	})

	stdout, err := executeRunCommand(t, dbPath, "run", "--quiet", "all")
	if err != nil {
		t.Fatalf("execute run all complete promise: %v", err)
	}
	if !strings.Contains(stdout, "Agent declared completion") {
		t.Fatalf("run all output = %q, want completion", stdout)
	}
	if got := atomic.LoadInt32(turns); got != 1 {
		t.Fatalf("fake runner turns = %d, want 1", got)
	}
	assertRunCount(t, dbPath, 1)
	assertTaskStatuses(t, dbPath, repoRoot, "pending", "pending")
}

func TestFakeRunnerRunAllStopsOnBlockedOrFailedByDefault(t *testing.T) {
	for _, status := range []string{"blocked", "failed"} {
		t.Run(status, func(t *testing.T) {
			dbPath, repoRoot := setupFakeRunnerProject(t, `[
				{"category":"runner","description":"stopped","steps":["one"],"passes":false},
				{"category":"runner","description":"pending","steps":["two"],"passes":false}
			]`)
			tasks := fetchImportedTasks(t, dbPath, repoRoot)
			setTaskStatus(t, dbPath, tasks[0].id, status)
			turns := installFakeAgentRunner(t, func(_ context.Context, _ runner.Request, _ int) (runner.Result, error) {
				return runner.Result{}, fmt.Errorf("fake runner must not run")
			})

			_, err := executeRunCommand(t, dbPath, "run", "--quiet", "all")
			if err == nil || !strings.Contains(err.Error(), "blocked or failed tasks remain") {
				t.Fatalf("run all %s error = %v, want blocked/failed stop", status, err)
			}
			if got := atomic.LoadInt32(turns); got != 0 {
				t.Fatalf("fake runner turns = %d, want 0", got)
			}
			assertRunCount(t, dbPath, 0)
		})
	}
}
