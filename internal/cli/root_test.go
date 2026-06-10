package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
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
	id          int64
	description string
	status      string
	steps       string
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
		SELECT t.id, t.description, t.status, COALESCE((
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
		if err := rows.Scan(&task.id, &task.description, &task.status, &task.steps); err != nil {
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
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("create git root: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
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
