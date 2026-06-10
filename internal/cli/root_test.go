package cli

import (
	"bytes"
	"database/sql"
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
