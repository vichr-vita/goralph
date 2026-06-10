package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestValidateTaskStatusRejectsUnknown(t *testing.T) {
	for _, status := range []string{"pending", "in_progress", "blocked", "passed", "failed"} {
		t.Run(status, func(t *testing.T) {
			if err := validateTaskStatus(status); err != nil {
				t.Fatalf("validateTaskStatus(%q): %v", status, err)
			}
		})
	}

	for _, status := range []string{"", "unknown", "completed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			if err := validateTaskStatus(status); err == nil {
				t.Fatalf("validateTaskStatus(%q) succeeded, want error", status)
			}
		})
	}
}

func TestRootCommandExecutesWithoutConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}
}

func TestRootCommandLoadsExplicitConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner: pi\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}
	if got := viper.GetString("runner"); got != "pi" {
		t.Fatalf("runner = %q, want pi", got)
	}
}

func TestRootCommandUsesDatabaseFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}
	if got := viper.GetString("db"); got != dbPath {
		t.Fatalf("db = %q, want %q", got, dbPath)
	}
	if info, err := os.Stat(filepath.Dir(dbPath)); err != nil {
		t.Fatalf("stat db parent: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("db parent is not directory")
	}
	assertGooseVersionRecorded(t, dbPath)
}

func TestRootCommandAutoCreatesCurrentProject(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot := filepath.Join(t.TempDir(), "sample-repo")
	workDir := filepath.Join(repoRoot, "nested", "pkg")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("create git root: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}
	assertProject(t, dbPath, repoRoot, "sample-repo")
}

func TestProjectInfoPrintsCurrentProject(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	stdout := &bytes.Buffer{}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "project", "info"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute project info: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"Project", "Name: sample-repo", "Root: " + repoRoot} {
		if !strings.Contains(output, want) {
			t.Fatalf("project info output = %q, want %q", output, want)
		}
	}
}

func TestProjectInfoPrintsJSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	stdout := &bytes.Buffer{}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "project", "info"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute project info --json: %v", err)
	}
	var got projectOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode project JSON %q: %v", stdout.String(), err)
	}
	if got.Name != "sample-repo" || got.RootPath != repoRoot || got.Description != "" {
		t.Fatalf("project JSON = %+v, want sample-repo at %s", got, repoRoot)
	}
}

func TestProjectInitUpdatesAutoCreatedProject(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	stdout := &bytes.Buffer{}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "project", "init", "--name", "Custom", "--description", "Demo project"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute project init: %v", err)
	}
	var got projectOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode project JSON %q: %v", stdout.String(), err)
	}
	if got.Name != "Custom" || got.Description != "Demo project" || got.RootPath != repoRoot {
		t.Fatalf("project init JSON = %+v, want updated project", got)
	}
	assertProjectDetails(t, dbPath, repoRoot, "Custom", "Demo project", 1)
}

func TestRootCommandErrorsWithoutGitRoot(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	workDir := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	chdir(t, workDir)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "ralph.db")})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("execute root command succeeded, want no git root error")
	}
	if !strings.Contains(err.Error(), "no git root found") {
		t.Fatalf("error = %q, want no git root found", err.Error())
	}
}

func TestPRDValidateCommandValidatesFileWithoutGitRoot(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	workDir := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	chdir(t, workDir)

	prdPath := filepath.Join(t.TempDir(), "prd.json")
	if err := os.WriteFile(prdPath, []byte(`[{"category":"cli","description":"validate PRD","steps":["step"],"passes":false}]`), 0o600); err != nil {
		t.Fatalf("write PRD: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "ralph.db"), "prd", "validate", prdPath})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd validate: %v", err)
	}
	if !strings.Contains(stdout.String(), "validated "+prdPath) {
		t.Fatalf("prd validate output = %q, want validated path", stdout.String())
	}
}

func TestPRDImportInsertsTasksAndSteps(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"cli","description":"pending task","steps":["one","two"],"passes":false},
		{"category":"db","description":"passed task","steps":["done"],"passes":true}
	]`)

	stdout := &bytes.Buffer{}
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	if !strings.Contains(stdout.String(), "imported 2 tasks (replace)") {
		t.Fatalf("prd import output = %q, want import count", stdout.String())
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if tasks[0].description != "pending task" || tasks[0].status != "pending" || tasks[0].steps != "one,two" {
		t.Fatalf("first task = %+v, want pending task with steps", tasks[0])
	}
	if tasks[1].description != "passed task" || tasks[1].status != "passed" || tasks[1].steps != "done" {
		t.Fatalf("second task = %+v, want passed task", tasks[1])
	}
}

func TestPRDImportExistingTasksNeedExplicitNonInteractiveMode(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	firstPath := writePRDFile(t, `[{"category":"cli","description":"first","steps":["one"],"passes":false}]`)
	secondPath := writePRDFile(t, `[{"category":"cli","description":"second","steps":["two"],"passes":false}]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", firstPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute first prd import: %v", err)
	}

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", secondPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("execute second prd import succeeded, want explicit mode error")
	}
	if !strings.Contains(err.Error(), "--replace or --append") {
		t.Fatalf("second prd import error = %q, want explicit mode hint", err.Error())
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 || tasks[0].description != "first" {
		t.Fatalf("tasks after refused import = %+v, want first task only", tasks)
	}
}

func TestPRDImportReplaceAndAppendModes(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	firstPath := writePRDFile(t, `[{"category":"cli","description":"first","steps":["one"],"passes":false}]`)
	replacePath := writePRDFile(t, `[{"category":"cli","description":"replacement","steps":["two"],"passes":false}]`)
	appendPath := writePRDFile(t, `[{"category":"cli","description":"appended","steps":["three"],"passes":false}]`)

	for _, args := range [][]string{
		{"--db", dbPath, "prd", "import", firstPath},
		{"--db", dbPath, "prd", "import", "--replace", replacePath},
		{"--db", dbPath, "prd", "import", "--append", appendPath},
	} {
		cmd := NewRootCommand()
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	if tasks[0].description != "replacement" || tasks[1].description != "appended" {
		t.Fatalf("tasks = %+v, want replacement then appended", tasks)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", "--replace", "--append", appendPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("replace+append error = %v, want conflict", err)
	}
}

func TestPRDExportWritesStdoutAndFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	_, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"cli","description":"pending task","steps":["second","first"],"passes":false},
		{"category":"db","description":"passed task","steps":[],"passes":true}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}

	reorderTaskSteps(t, dbPath)

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "export"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd export stdout: %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "[\n  {") {
		t.Fatalf("prd export stdout = %q, want pretty JSON array", stdout.String())
	}
	assertExportedPRD(t, []byte(stdout.String()))

	exportPath := filepath.Join(t.TempDir(), "exported.json")
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "export", exportPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd export file: %v", err)
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read exported PRD: %v", err)
	}
	if !strings.HasPrefix(string(data), "[\n  {") {
		t.Fatalf("prd export file = %q, want pretty JSON array", string(data))
	}
	assertExportedPRD(t, data)
}

func TestTaskListSupportsStatusFilterAndJSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	_, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"cli","description":"pending task","steps":["one","two"],"passes":false},
		{"category":"db","description":"passed task","steps":["done"],"passes":true}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "task", "list", "--status", "passed"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute task list: %v", err)
	}

	var got []taskOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode task list JSON %q: %v", stdout.String(), err)
	}
	if len(got) != 1 {
		t.Fatalf("task list count = %d, want 1", len(got))
	}
	if got[0].Description != "passed task" || got[0].Status != "passed" || len(got[0].Steps) != 1 || got[0].Steps[0].Description != "done" {
		t.Fatalf("task list JSON = %+v, want passed task with step", got[0])
	}
}

func TestTaskAddCreatesPendingTaskWithOrderedSteps(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	stdout := &bytes.Buffer{}
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "task", "add", "--category", "cli", "--description", "new task", "--step", "first", "--step", "second"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute task add: %v", err)
	}

	var got taskOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode task add JSON %q: %v", stdout.String(), err)
	}
	if got.Category != "cli" || got.Description != "new task" || got.Status != "pending" {
		t.Fatalf("task add JSON = %+v, want pending cli task", got)
	}
	if len(got.Steps) != 2 || got.Steps[0].Position != 1 || got.Steps[0].Description != "first" || got.Steps[1].Position != 2 || got.Steps[1].Description != "second" {
		t.Fatalf("task add steps = %+v, want ordered steps", got.Steps)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 || tasks[0].category != "cli" || tasks[0].description != "new task" || tasks[0].status != "pending" || tasks[0].steps != "first,second" {
		t.Fatalf("stored tasks = %+v, want added task", tasks)
	}
}

func TestTaskUpdateEditsFieldsAndReplacesOrderedSteps(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[{"category":"cli","description":"old","steps":["one","two"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "task", "update", strconv.FormatInt(taskID, 10), "--category", "db", "--description", "new", "--status", "blocked", "--progress_report", "blocked by review", "--step", "replacement", "--step", "next"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute task update: %v", err)
	}

	var got taskOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode task update JSON %q: %v", stdout.String(), err)
	}
	if got.Category != "db" || got.Description != "new" || got.Status != "blocked" || got.ProgressReport != "blocked by review" {
		t.Fatalf("task update JSON = %+v, want updated fields", got)
	}
	if len(got.Steps) != 2 || got.Steps[0].Description != "replacement" || got.Steps[1].Description != "next" {
		t.Fatalf("task update steps = %+v, want replacement order", got.Steps)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 || tasks[0].category != "db" || tasks[0].description != "new" || tasks[0].status != "blocked" || tasks[0].progressReport != "blocked by review" || tasks[0].steps != "replacement,next" {
		t.Fatalf("stored tasks = %+v, want updated task", tasks)
	}
}

func TestTaskUpdateRejectsInvalidStatusBeforeWriting(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[{"category":"cli","description":"unchanged","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "task", "update", strconv.FormatInt(taskID, 10), "--description", "changed", "--status", "completed"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown task status") {
		t.Fatalf("task update error = %v, want unknown status", err)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 || tasks[0].description != "unchanged" || tasks[0].status != "pending" {
		t.Fatalf("stored tasks after invalid update = %+v, want unchanged", tasks)
	}
}

func TestTaskLifecycleShortcutCommands(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[{"category":"cli","description":"shortcut me","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id
	taskIDArg := strconv.FormatInt(taskID, 10)

	for _, tc := range []struct {
		args       []string
		wantStatus string
	}{
		{[]string{"task", "start", taskIDArg}, "in_progress"},
		{[]string{"task", "pass", taskIDArg}, "passed"},
		{[]string{"task", "block", taskIDArg, "--reason", "waiting on review"}, "blocked"},
		{[]string{"task", "unblock", taskIDArg}, "pending"},
		{[]string{"task", "fail", taskIDArg, "--reason", "tests failed"}, "failed"},
		{[]string{"task", "update", taskIDArg, "--status", "pending"}, "pending"},
	} {
		cmd := NewRootCommand()
		cmd.SetArgs(append([]string{"--db", dbPath}, tc.args...))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		tasks := fetchImportedTasks(t, dbPath, repoRoot)
		if len(tasks) != 1 || tasks[0].status != tc.wantStatus {
			t.Fatalf("tasks after %v = %+v, want status %s", tc.args, tasks, tc.wantStatus)
		}
	}

	progress := fetchTaskProgressSummaries(t, dbPath, taskID)
	if strings.Join(progress, ",") != "tests failed,waiting on review" {
		t.Fatalf("progress = %+v, want fail and block reasons newest first", progress)
	}
}

func TestTaskLifecycleShortcutRequiresReason(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[{"category":"cli","description":"shortcut me","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	taskID := fetchImportedTasks(t, dbPath, repoRoot)[0].id

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "task", "fail", strconv.FormatInt(taskID, 10), "--reason", "   "})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "reason cannot be empty") {
		t.Fatalf("task fail error = %v, want reason cannot be empty", err)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 || tasks[0].status != "pending" {
		t.Fatalf("stored tasks after invalid fail = %+v, want pending", tasks)
	}
}

func TestTaskShowPrintsDetailsAndLatestProgress(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[{"category":"cli","description":"inspect me","steps":["one","two"],"passes":false}]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	seedTaskProgress(t, dbPath, tasks[0].id, "agent says 60%", []string{"first progress", "latest progress"})

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "task", "show", strconv.FormatInt(tasks[0].id, 10)})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute task show: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"Task ", "Status: pending", "Progress report: agent says 60%", "1. one", "2. two", "latest progress"} {
		if !strings.Contains(output, want) {
			t.Fatalf("task show output = %q, want %q", output, want)
		}
	}
}

func TestProgressAddAndListSupportsTaskFilterJSONAndActiveRun(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"cli","description":"first task","steps":["one"],"passes":false},
		{"category":"db","description":"second task","steps":[],"passes":false}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	runID := seedActiveRun(t, dbPath, repoRoot, tasks[0].id)

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "progress", "add", "--task", strconv.FormatInt(tasks[0].id, 10), "--summary", "worked on task"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute progress add task: %v", err)
	}
	var taskProgress progressOutput
	if err := json.Unmarshal(stdout.Bytes(), &taskProgress); err != nil {
		t.Fatalf("decode task progress JSON %q: %v", stdout.String(), err)
	}
	if taskProgress.TaskID == nil || *taskProgress.TaskID != tasks[0].id || taskProgress.RunID == nil || *taskProgress.RunID != runID || taskProgress.Summary != "worked on task" {
		t.Fatalf("task progress JSON = %+v, want task %d run %d", taskProgress, tasks[0].id, runID)
	}

	stdout.Reset()
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "progress", "add", "--summary", "project note"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute progress add project: %v", err)
	}
	var projectProgress progressOutput
	if err := json.Unmarshal(stdout.Bytes(), &projectProgress); err != nil {
		t.Fatalf("decode project progress JSON %q: %v", stdout.String(), err)
	}
	if projectProgress.TaskID != nil || projectProgress.RunID == nil || *projectProgress.RunID != runID || projectProgress.Summary != "project note" {
		t.Fatalf("project progress JSON = %+v, want no task and run %d", projectProgress, runID)
	}

	stdout.Reset()
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "progress", "list"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute progress list: %v", err)
	}
	var all []progressOutput
	if err := json.Unmarshal(stdout.Bytes(), &all); err != nil {
		t.Fatalf("decode progress list JSON %q: %v", stdout.String(), err)
	}
	if len(all) != 2 || all[0].Summary != "project note" || all[1].Summary != "worked on task" {
		t.Fatalf("progress list = %+v, want newest project note then task progress", all)
	}

	stdout.Reset()
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "progress", "list", "--task", strconv.FormatInt(tasks[0].id, 10)})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute progress list --task: %v", err)
	}
	var filtered []progressOutput
	if err := json.Unmarshal(stdout.Bytes(), &filtered); err != nil {
		t.Fatalf("decode progress filter JSON %q: %v", stdout.String(), err)
	}
	if len(filtered) != 1 || filtered[0].Summary != "worked on task" || filtered[0].TaskID == nil || *filtered[0].TaskID != tasks[0].id {
		t.Fatalf("filtered progress = %+v, want only task progress", filtered)
	}
}

func TestRunOneCreatesAndFinishesRunRecord(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	promptPath := filepath.Join(tempDir, "prompt.txt")
	countPath := filepath.Join(tempDir, "count.txt")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\nfeedback_commands:\n  - go test ./...\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"track me","steps":["one"],"passes":false},
		{"category":"runner","description":"leave me pending","steps":["two"],"passes":false}
	]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	taskID := tasks[0].id
	seedTaskProgress(t, dbPath, taskID, "half done", []string{"latest note"})
	runnerScript := "#!/bin/sh\nfor last do :; done\nprintf x >> " + strconv.Quote(countPath) + "\nprintf '%s' \"$last\" > " + strconv.Quote(promptPath) + "\nGO_RALPH_TEST_HELPER=run_one_update GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " GO_RALPH_TEST_TASK=" + strconv.Quote(strconv.FormatInt(taskID, 10)) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunOneHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "one"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run one: %v", err)
	}
	if !strings.Contains(stdout.String(), "Status: succeeded") {
		t.Fatalf("run output = %q, want succeeded", stdout.String())
	}

	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	prompt := string(promptBytes)
	for _, want := range []string{"Ralph loop agent prompt contract", "Name: sample-repo", "Root path: " + repoRoot, "Eligible tasks, highest priority first. Choose exactly one highest-priority task:", "Description: track me", "1. one", "Description: leave me pending", "Progress report: half done", "latest note", "go test ./...", "goralph task start <task-id>", "goralph progress add --task <task-id>", "goralph task pass <task-id>", "goralph task fail <task-id>", "goralph task block <task-id>", "Work on only one feature.", "If multiple eligible tasks appear, choose the first task unless recent progress shows it is blocked.", "Commit the feature.", "<promise>COMPLETE</promise>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var gotTaskID sql.NullInt64
	var status string
	var pid sql.NullInt64
	var host string
	var heartbeatAt sql.NullString
	var startedAt sql.NullString
	var finishedAt sql.NullString
	if err := database.QueryRow("SELECT task_id, status, pid, host, heartbeat_at, started_at, finished_at FROM run").Scan(&gotTaskID, &status, &pid, &host, &heartbeatAt, &startedAt, &finishedAt); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if !gotTaskID.Valid || gotTaskID.Int64 != taskID || status != "succeeded" || !pid.Valid || host == "" || !heartbeatAt.Valid || !startedAt.Valid || !finishedAt.Valid {
		t.Fatalf("run row task=%v status=%q pid=%v host=%q heartbeat=%v started=%v finished=%v", gotTaskID, status, pid, host, heartbeatAt, startedAt, finishedAt)
	}

	countBytes, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("read runner count: %v", err)
	}
	if got := string(countBytes); got != "x" {
		t.Fatalf("runner count marker = %q, want one turn", got)
	}
	var firstStatus, secondStatus string
	if err := database.QueryRow("SELECT status FROM task WHERE id = ?", taskID).Scan(&firstStatus); err != nil {
		t.Fatalf("query first task status: %v", err)
	}
	if err := database.QueryRow("SELECT status FROM task WHERE id = ?", tasks[1].id).Scan(&secondStatus); err != nil {
		t.Fatalf("query second task status: %v", err)
	}
	if firstStatus != "passed" || secondStatus != "pending" {
		t.Fatalf("task statuses = %q, %q; want passed, pending", firstStatus, secondStatus)
	}
}

func TestRunOneTaskFlagForcesSpecifiedTask(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	promptPath := filepath.Join(tempDir, "prompt.txt")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"default first","steps":["one"],"passes":false},
		{"category":"runner","description":"forced second","steps":["two"],"passes":false}
	]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	forcedTaskID := tasks[1].id
	runnerScript := "#!/bin/sh\nfor last do :; done\nprintf '%s' \"$last\" > " + strconv.Quote(promptPath) + "\nGO_RALPH_TEST_HELPER=run_one_update GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " GO_RALPH_TEST_TASK=" + strconv.Quote(strconv.FormatInt(forcedTaskID, 10)) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunOneHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "one", "--task", strconv.FormatInt(forcedTaskID, 10)})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run one --task: %v", err)
	}
	if !strings.Contains(stdout.String(), "Task: "+strconv.FormatInt(forcedTaskID, 10)) || !strings.Contains(stdout.String(), "Status: succeeded") {
		t.Fatalf("run output = %q, want forced task succeeded", stdout.String())
	}

	prompt := readTestFile(t, promptPath)
	for _, want := range []string{"Forced task from --task:", "Task ID: " + strconv.FormatInt(forcedTaskID, 10), "Description: forced second", "1. two"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("forced prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, unwanted := range []string{"Eligible tasks, highest priority first", "Description: default first"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("forced prompt contains %q:\n%s", unwanted, prompt)
		}
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var runTaskID sql.NullInt64
	if err := database.QueryRow("SELECT task_id FROM run").Scan(&runTaskID); err != nil {
		t.Fatalf("query run task: %v", err)
	}
	if !runTaskID.Valid || runTaskID.Int64 != forcedTaskID {
		t.Fatalf("run task id = %v, want %d", runTaskID, forcedTaskID)
	}
	var firstStatus, secondStatus string
	if err := database.QueryRow("SELECT status FROM task WHERE id = ?", tasks[0].id).Scan(&firstStatus); err != nil {
		t.Fatalf("query first task status: %v", err)
	}
	if err := database.QueryRow("SELECT status FROM task WHERE id = ?", forcedTaskID).Scan(&secondStatus); err != nil {
		t.Fatalf("query forced task status: %v", err)
	}
	if firstStatus != "pending" || secondStatus != "passed" {
		t.Fatalf("task statuses = %q, %q; want pending, passed", firstStatus, secondStatus)
	}
}

func TestRunOneTaskFlagRejectsUnknownOrOtherProjectTask(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "source-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	prdPath := writePRDFile(t, `[{"category":"runner","description":"source task","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}

	runnerPath := filepath.Join(tempDir, "should-not-run.sh")
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, otherWorkDir := createTestGitWorkDir(t, "other-repo")
	chdir(t, otherWorkDir)
	viper.Reset()

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "one", "--task", strconv.FormatInt(tasks[0].id, 10)})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "task "+strconv.FormatInt(tasks[0].id, 10)+" not found") {
		t.Fatalf("run one --task other project error = %v, want task not found", err)
	}

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "one", "--task", "999999"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "task 999999 not found") {
		t.Fatalf("run one --task unknown error = %v, want task not found", err)
	}

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "one", "--task", "0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid task id") {
		t.Fatalf("run one --task zero error = %v, want invalid task id", err)
	}
}

func TestRunOneReportsNoEligibleTasks(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	_, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	runnerPath := filepath.Join(tempDir, "missing-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"done already","steps":["one"],"passes":true}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	viper.Reset()

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "one"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run one no eligible: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "No eligible task" {
		t.Fatalf("run one output = %q, want no eligible outcome", got)
	}
}

func TestRunOneRequiresCleanWorktreeUnlessAllowed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[{"category":"runner","description":"dirty guarded","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write dirty marker: %v", err)
	}
	runnerScript := "#!/bin/sh\nGO_RALPH_TEST_HELPER=run_one_update GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " GO_RALPH_TEST_TASK=" + strconv.Quote(strconv.FormatInt(tasks[0].id, 10)) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunOneHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "one"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "dirty git worktree") || !strings.Contains(err.Error(), "dirty.txt") {
		t.Fatalf("run one dirty error = %v, want dirty worktree with path", err)
	}
	assertRunCount(t, dbPath, 0)

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "--allow-dirty", "one"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run one --allow-dirty: %v", err)
	}
	if !strings.Contains(stdout.String(), "Status: succeeded") {
		t.Fatalf("run output = %q, want succeeded", stdout.String())
	}
	assertRunCount(t, dbPath, 1)
}

func TestRunAllRequiresCleanWorktree(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	runnerPath := filepath.Join(tempDir, "should-not-run.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	prdPath := writePRDFile(t, `[{"category":"runner","description":"dirty guarded","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write dirty marker: %v", err)
	}
	viper.Reset()

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "dirty git worktree") || !strings.Contains(err.Error(), "dirty.txt") {
		t.Fatalf("run all dirty error = %v, want dirty worktree with path", err)
	}
	assertRunCount(t, dbPath, 0)
}

func TestRunAllRechecksCleanWorktreeBeforeEachTurn(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	countPath := filepath.Join(tempDir, "count.txt")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[{"category":"runner","description":"still pending","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	runnerScript := "#!/bin/sh\nprintf x >> " + strconv.Quote(countPath) + "\nprintf dirty > " + strconv.Quote(filepath.Join(repoRoot, "dirty-after-first.txt")) + "\nGO_RALPH_TEST_HELPER=run_all_progress_pending GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunAllHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all", "--max-turns", "3"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "dirty git worktree") || !strings.Contains(err.Error(), "dirty-after-first.txt") {
		t.Fatalf("run all second-turn dirty error = %v, want dirty worktree with path", err)
	}
	if got := readTestFile(t, countPath); got != "x" {
		t.Fatalf("runner count = %q, want one turn", got)
	}
	assertRunCount(t, dbPath, 1)
}

func TestRunAllPassesPendingTasksUntilAllPassed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	countPath := filepath.Join(tempDir, "count.txt")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"first","steps":["one"],"passes":false},
		{"category":"runner","description":"second","steps":["two"],"passes":false}
	]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	if got := len(fetchImportedTasks(t, dbPath, repoRoot)); got != 2 {
		t.Fatalf("task count = %d, want 2", got)
	}

	runnerScript := "#!/bin/sh\nprintf x >> " + strconv.Quote(countPath) + "\nGO_RALPH_TEST_HELPER=run_all_pass_pending GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunAllHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all", "--max-turns", "5"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run all: %v", err)
	}
	if !strings.Contains(stdout.String(), "All tasks passed") {
		t.Fatalf("run all output = %q, want all passed", stdout.String())
	}
	if got := readTestFile(t, countPath); got != "xx" {
		t.Fatalf("runner count = %q, want two turns", got)
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	var pending int
	if err := database.QueryRow("SELECT COUNT(*) FROM task WHERE status != 'passed'").Scan(&pending); err != nil {
		t.Fatalf("count pending tasks: %v", err)
	}
	if pending != 0 {
		t.Fatalf("unfinished tasks = %d, want 0", pending)
	}
	var runs int
	if err := database.QueryRow("SELECT COUNT(*) FROM run WHERE status = 'succeeded'").Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runs != 2 {
		t.Fatalf("succeeded runs = %d, want 2", runs)
	}
}

func TestRunAllStopsOnCompletePromise(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	_, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[{"category":"runner","description":"pending","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nprintf '<promise>COMPLETE</promise>\\n'\n"), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run all complete promise: %v", err)
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	var runs int
	if err := database.QueryRow("SELECT COUNT(*) FROM run WHERE status = 'succeeded'").Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestRunAllBlockedDefaultAndContinueOnBlocked(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	countPath := filepath.Join(tempDir, "count.txt")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"blocked","steps":["one"],"passes":false},
		{"category":"runner","description":"pending","steps":["two"],"passes":false}
	]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := database.Exec("UPDATE task SET status = 'blocked' WHERE id = ?", tasks[0].id); err != nil {
		t.Fatalf("block task: %v", err)
	}
	database.Close()
	runnerScript := "#!/bin/sh\nprintf x >> " + strconv.Quote(countPath) + "\nGO_RALPH_TEST_HELPER=run_all_pass_pending GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunAllHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "blocked or failed tasks remain") {
		t.Fatalf("run all blocked error = %v, want blocked stop", err)
	}
	if _, err := os.Stat(countPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runner count file err = %v, want not exist", err)
	}

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all", "--continue-on-blocked", "--max-turns", "3"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run all continue blocked: %v", err)
	}
	if got := readTestFile(t, countPath); got != "x" {
		t.Fatalf("runner count = %q, want one turn", got)
	}
}

func TestRunAllStopsAtMaxTurns(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	_, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	countPath := filepath.Join(tempDir, "count.txt")
	runnerPath := filepath.Join(tempDir, "fake-runner.sh")
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prdPath := writePRDFile(t, `[{"category":"runner","description":"pending","steps":["one"],"passes":false}]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	runnerScript := "#!/bin/sh\nprintf x >> " + strconv.Quote(countPath) + "\nGO_RALPH_TEST_HELPER=run_all_progress_pending GO_RALPH_TEST_DB=" + strconv.Quote(dbPath) + " " + strconv.Quote(os.Args[0]) + " -test.run '^TestRunAllHelper$' >/dev/null\n"
	if err := os.WriteFile(runnerPath, []byte(runnerScript), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	viper.Reset()

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "--quiet", "all", "--max-turns", "2"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run all max turns: %v", err)
	}
	if !strings.Contains(stdout.String(), "Max turns reached (2)") {
		t.Fatalf("run all output = %q, want max turns", stdout.String())
	}
	if got := readTestFile(t, countPath); got != "xx" {
		t.Fatalf("runner count = %q, want two turns", got)
	}
}

func TestRunOneHelper(t *testing.T) {
	if os.Getenv("GO_RALPH_TEST_HELPER") != "run_one_update" {
		t.Skip("helper subprocess only")
	}
	taskID, err := strconv.ParseInt(os.Getenv("GO_RALPH_TEST_TASK"), 10, 64)
	if err != nil || taskID <= 0 {
		t.Fatalf("parse helper task id: %v", err)
	}
	database, err := sql.Open("sqlite", os.Getenv("GO_RALPH_TEST_DB"))
	if err != nil {
		t.Fatalf("open helper database: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec("UPDATE task SET status = 'passed', updated_at = CURRENT_TIMESTAMP WHERE id = ?", taskID); err != nil {
		t.Fatalf("helper update task: %v", err)
	}
	if _, err := database.Exec(`
INSERT INTO progress (project_id, task_id, run_id, summary)
SELECT project_id, id, (SELECT id FROM run WHERE status = 'running' ORDER BY id DESC LIMIT 1), 'helper completed task'
FROM task
WHERE id = ?`, taskID); err != nil {
		t.Fatalf("helper insert progress: %v", err)
	}
}

func TestRunAllHelper(t *testing.T) {
	helper := os.Getenv("GO_RALPH_TEST_HELPER")
	if helper != "run_all_pass_pending" && helper != "run_all_progress_pending" {
		t.Skip("helper subprocess only")
	}
	database, err := sql.Open("sqlite", os.Getenv("GO_RALPH_TEST_DB"))
	if err != nil {
		t.Fatalf("open helper database: %v", err)
	}
	defer database.Close()

	var taskID int64
	if err := database.QueryRow("SELECT id FROM task WHERE status = 'pending' ORDER BY id LIMIT 1").Scan(&taskID); err != nil {
		t.Fatalf("select pending task: %v", err)
	}
	if helper == "run_all_pass_pending" {
		if _, err := database.Exec("UPDATE task SET status = 'passed', updated_at = CURRENT_TIMESTAMP WHERE id = ?", taskID); err != nil {
			t.Fatalf("helper pass task: %v", err)
		}
	}
	if _, err := database.Exec(`
INSERT INTO progress (project_id, task_id, run_id, summary)
SELECT project_id, id, (SELECT id FROM run WHERE status = 'running' ORDER BY id DESC LIMIT 1), 'helper run all progress'
FROM task
WHERE id = ?`, taskID); err != nil {
		t.Fatalf("helper insert progress: %v", err)
	}
}

func TestRunListPrintsRuns(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"list me","steps":["one"],"passes":false}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	runID := seedRun(t, dbPath, repoRoot, tasks[0].id)

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "run", "list"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run list: %v", err)
	}
	for _, want := range []string{"ID\tTASK\tSTATUS", strconv.FormatInt(runID, 10), "succeeded", "session-123"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("run list output missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "run", "list"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run list JSON: %v", err)
	}
	var out []runOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode run list JSON %q: %v", stdout.String(), err)
	}
	if len(out) != 1 || out[0].ID != runID || out[0].SessionID != "session-123" {
		t.Fatalf("run list JSON = %+v", out)
	}
}

func TestRunShowPrintsMetadataProgressAndSessionPath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"stream output","steps":["one"],"passes":false}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}

	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	runID := seedRun(t, dbPath, repoRoot, tasks[0].id)
	seedRunProgress(t, dbPath, repoRoot, tasks[0].id, runID, "agent made progress")

	stdout := &bytes.Buffer{}
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "run", "show", strconv.FormatInt(runID, 10)})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run show: %v", err)
	}
	for _, want := range []string{"Run ", "Runner: pi", "Model: test-model", "Status: succeeded", "Exit code: 0", "Session ID: session-123", "Session file: /tmp/session.jsonl", "agent made progress"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("run show output missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "--json", "run", "show", strconv.FormatInt(runID, 10)})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run show JSON: %v", err)
	}
	var out runOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode run show JSON %q: %v", stdout.String(), err)
	}
	if out.SessionID != "session-123" || out.SessionPath != "/tmp/session.jsonl" || len(out.Progress) != 1 || out.Progress[0].Summary != "agent made progress" {
		t.Fatalf("run show JSON = %+v", out)
	}
}

func TestRunOpenAndExportUsePiSessionMetadata(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"open me","steps":["one"],"passes":false}
	]`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	runID := seedRun(t, dbPath, repoRoot, tasks[0].id)

	argsPath := filepath.Join(tempDir, "args.txt")
	runnerPath := filepath.Join(tempDir, "fake-pi.sh")
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+argsPath+"\"\n"), 0o700); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("runner_command: "+runnerPath+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "open", strconv.FormatInt(runID, 10)})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run open: %v", err)
	}
	if got := strings.TrimSpace(readTestFile(t, argsPath)); got != "--session\n/tmp/session.jsonl" {
		t.Fatalf("open args = %q", got)
	}

	outPath := filepath.Join(tempDir, "session.html")
	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--config", configPath, "--db", dbPath, "run", "export", strconv.FormatInt(runID, 10), outPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run export: %v", err)
	}
	if got := strings.TrimSpace(readTestFile(t, argsPath)); got != "--export\n/tmp/session.jsonl\n"+outPath {
		t.Fatalf("export args = %q", got)
	}
}

func TestRunOpenErrorsWithoutSessionMetadata(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	repoRoot, workDir := createTestGitWorkDir(t, "sample-repo")
	chdir(t, workDir)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	prdPath := writePRDFile(t, `[
		{"category":"runner","description":"missing session","steps":["one"],"passes":false}
	]`)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "prd", "import", prdPath})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute prd import: %v", err)
	}
	tasks := fetchImportedTasks(t, dbPath, repoRoot)
	runID := seedRunWithoutSession(t, dbPath, repoRoot, tasks[0].id)

	cmd = NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "run", "open", strconv.FormatInt(runID, 10)})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("execute run open without session succeeded, want error")
	}
	if !strings.Contains(err.Error(), "no stored session metadata") {
		t.Fatalf("run open error = %q", err.Error())
	}
}

func TestDBPathPrintsResolvedDatabasePath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	stdout := &bytes.Buffer{}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "db", "path"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute db path: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != dbPath {
		t.Fatalf("db path output = %q, want %q", got, dbPath)
	}
}

func TestDBMigrateRunsMigrationsOnExplicitDatabase(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	stdout := &bytes.Buffer{}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "db", "migrate"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute db migrate: %v", err)
	}
	if !strings.Contains(stdout.String(), dbPath) {
		t.Fatalf("db migrate output = %q, want path %q", stdout.String(), dbPath)
	}
	assertGooseVersionRecorded(t, dbPath)
}

func TestDBResetRequiresForce(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "db", "reset"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("execute db reset without force succeeded, want error")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("db reset error = %q, want --force", err.Error())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("db file exists after refused reset: %v", err)
	}
}

func TestDBResetForceRecreatesMigratedDatabase(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateDatabaseEnv(t)

	dbPath := filepath.Join(t.TempDir(), "db", "ralph.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("create seed database parent: %v", err)
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	if _, err := database.Exec("CREATE TABLE stale (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create stale table: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close seed database: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "db", "reset", "--force"})
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute db reset --force: %v", err)
	}
	if !strings.Contains(stdout.String(), dbPath) {
		t.Fatalf("db reset output = %q, want path %q", stdout.String(), dbPath)
	}
	assertGooseVersionRecorded(t, dbPath)
	assertTableMissing(t, dbPath, "stale")
}

type importedTaskRow struct {
	id             int64
	category       string
	description    string
	status         string
	progressReport string
	steps          string
}

func seedTaskProgress(t *testing.T, dbPath string, taskID int64, progressReport string, summaries []string) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec("UPDATE task SET progress_report = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", progressReport, taskID); err != nil {
		t.Fatalf("update task progress report: %v", err)
	}

	var projectID int64
	if err := database.QueryRow("SELECT project_id FROM task WHERE id = ?", taskID).Scan(&projectID); err != nil {
		t.Fatalf("query task project: %v", err)
	}
	for _, summary := range summaries {
		if _, err := database.Exec("INSERT INTO progress (project_id, task_id, summary) VALUES (?, ?, ?)", projectID, taskID, summary); err != nil {
			t.Fatalf("insert task progress: %v", err)
		}
	}
}

func seedRun(t *testing.T, dbPath string, rootPath string, taskID int64) int64 {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var projectID int64
	if err := database.QueryRow("SELECT id FROM project WHERE root_path = ?", rootPath).Scan(&projectID); err != nil {
		t.Fatalf("query project id: %v", err)
	}
	result, err := database.Exec("INSERT INTO run (project_id, task_id, runner_name, runner_model, session_id, session_path, status, exit_code, pid, host, started_at, finished_at) VALUES (?, ?, 'pi', 'test-model', 'session-123', '/tmp/session.jsonl', 'succeeded', 0, 42, 'test-host', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", projectID, taskID)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read run id: %v", err)
	}
	return runID
}

func seedRunWithoutSession(t *testing.T, dbPath string, rootPath string, taskID int64) int64 {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var projectID int64
	if err := database.QueryRow("SELECT id FROM project WHERE root_path = ?", rootPath).Scan(&projectID); err != nil {
		t.Fatalf("query project id: %v", err)
	}
	result, err := database.Exec("INSERT INTO run (project_id, task_id, runner_name, status, host, started_at, finished_at) VALUES (?, ?, 'pi', 'succeeded', 'test-host', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", projectID, taskID)
	if err != nil {
		t.Fatalf("insert run without session: %v", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read run id: %v", err)
	}
	return runID
}

func seedRunProgress(t *testing.T, dbPath string, rootPath string, taskID int64, runID int64, summary string) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var projectID int64
	if err := database.QueryRow("SELECT id FROM project WHERE root_path = ?", rootPath).Scan(&projectID); err != nil {
		t.Fatalf("query project id: %v", err)
	}
	if _, err := database.Exec("INSERT INTO progress (project_id, task_id, run_id, summary) VALUES (?, ?, ?, ?)", projectID, taskID, runID, summary); err != nil {
		t.Fatalf("insert run progress: %v", err)
	}
}

func seedActiveRun(t *testing.T, dbPath string, rootPath string, taskID int64) int64 {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var projectID int64
	if err := database.QueryRow("SELECT id FROM project WHERE root_path = ?", rootPath).Scan(&projectID); err != nil {
		t.Fatalf("query project id: %v", err)
	}
	result, err := database.Exec("INSERT INTO run (project_id, task_id, runner_name, status, started_at) VALUES (?, ?, 'test-runner', 'running', CURRENT_TIMESTAMP)", projectID, taskID)
	if err != nil {
		t.Fatalf("insert active run: %v", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read active run id: %v", err)
	}
	return runID
}

func fetchTaskProgressSummaries(t *testing.T, dbPath string, taskID int64) []string {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	rows, err := database.Query("SELECT summary FROM progress WHERE task_id = ? ORDER BY created_at DESC, id DESC", taskID)
	if err != nil {
		t.Fatalf("query task progress: %v", err)
	}
	defer rows.Close()

	var summaries []string
	for rows.Next() {
		var summary string
		if err := rows.Scan(&summary); err != nil {
			t.Fatalf("scan task progress: %v", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate task progress: %v", err)
	}
	return summaries
}

func reorderTaskSteps(t *testing.T, dbPath string) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	for _, query := range []string{
		"UPDATE task_step SET position = 10 WHERE description = 'second'",
		"UPDATE task_step SET position = 1 WHERE description = 'first'",
		"UPDATE task_step SET position = 2 WHERE description = 'second'",
	} {
		if _, err := database.Exec(query); err != nil {
			t.Fatalf("reorder task steps: %v", err)
		}
	}
}

func assertExportedPRD(t *testing.T, data []byte) {
	t.Helper()

	var items []struct {
		Category    string   `json:"category"`
		Description string   `json:"description"`
		Steps       []string `json:"steps"`
		Passes      bool     `json:"passes"`
	}
	if bytes.Contains(data, []byte(`"steps": null`)) {
		t.Fatalf("exported PRD has null steps: %s", data)
	}
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("decode exported PRD: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("exported item count = %d, want 2", len(items))
	}
	if items[0].Category != "cli" || items[0].Description != "pending task" || items[0].Passes {
		t.Fatalf("first exported item = %+v, want pending cli task", items[0])
	}
	if len(items[0].Steps) != 2 || items[0].Steps[0] != "first" || items[0].Steps[1] != "second" {
		t.Fatalf("first exported steps = %+v, want position order", items[0].Steps)
	}
	if items[1].Category != "db" || items[1].Description != "passed task" || !items[1].Passes || len(items[1].Steps) != 0 {
		t.Fatalf("second exported item = %+v, want passed db task with no steps", items[1])
	}
}

func writePRDFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "prd.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write PRD: %v", err)
	}
	return path
}

func fetchImportedTasks(t *testing.T, dbPath string, rootPath string) []importedTaskRow {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	rows, err := database.Query(`
		SELECT t.id, t.category, t.description, t.status, t.progress_report, COALESCE((
			SELECT group_concat(description, ',')
			FROM (
				SELECT description
				FROM task_step
				WHERE task_id = t.id
				ORDER BY position
			)
		), '') AS steps
		FROM task t
		JOIN project p ON p.id = t.project_id
		WHERE p.root_path = ?
		ORDER BY t.id`, rootPath)
	if err != nil {
		t.Fatalf("query tasks: %v", err)
	}
	defer rows.Close()

	var tasks []importedTaskRow
	for rows.Next() {
		var task importedTaskRow
		if err := rows.Scan(&task.id, &task.category, &task.description, &task.status, &task.progressReport, &task.steps); err != nil {
			t.Fatalf("scan task: %v", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tasks: %v", err)
	}
	return tasks
}

func createTestGitWorkDir(t *testing.T, repoName string) (string, string) {
	t.Helper()

	repoRoot := filepath.Join(t.TempDir(), repoName)
	workDir := filepath.Join(repoRoot, "nested", "pkg")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	cmd := exec.Command("git", "init", "--quiet", repoRoot)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init git repo: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return repoRoot, workDir
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func assertRunCount(t *testing.T, dbPath string, want int) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var got int
	if err := database.QueryRow("SELECT COUNT(*) FROM run").Scan(&got); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if got != want {
		t.Fatalf("run count = %d, want %d", got, want)
	}
}

func assertProject(t *testing.T, dbPath string, rootPath string, name string) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var gotName string
	if err := database.QueryRow("SELECT name FROM project WHERE root_path = ?", rootPath).Scan(&gotName); err != nil {
		t.Fatalf("query project: %v", err)
	}
	if gotName != name {
		t.Fatalf("project name = %q, want %q", gotName, name)
	}
}

func assertProjectDetails(t *testing.T, dbPath string, rootPath string, name string, description string, wantCount int) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var gotName string
	var gotDescription string
	if err := database.QueryRow("SELECT name, description FROM project WHERE root_path = ?", rootPath).Scan(&gotName, &gotDescription); err != nil {
		t.Fatalf("query project: %v", err)
	}
	if gotName != name || gotDescription != description {
		t.Fatalf("project = (%q, %q), want (%q, %q)", gotName, gotDescription, name, description)
	}

	var gotCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM project WHERE root_path = ?", rootPath).Scan(&gotCount); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if gotCount != wantCount {
		t.Fatalf("project count = %d, want %d", gotCount, wantCount)
	}
}

func assertGooseVersionRecorded(t *testing.T, dbPath string) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM goose_db_version WHERE version_id = 1 AND is_applied = 1").Scan(&count); err != nil {
		t.Fatalf("query goose version: %v", err)
	}
	if count != 1 {
		t.Fatalf("goose version rows = %d, want 1", count)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func assertTableMissing(t *testing.T, dbPath string, table string) {
	t.Helper()

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("table %s exists after reset", table)
	}
}

func isolateDatabaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GO_RALPH_DB", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))
}
