package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadMergesUserAndProjectConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	userDBPath := filepath.Join(t.TempDir(), "user", databaseFile)
	writeFile(t, filepath.Join(configHome, configDir, configFile), "runner: user-runner\nfeedback_commands:\n  - user feedback\ndb: "+userDBPath+"\n")

	repoRoot := t.TempDir()
	workDir := filepath.Join(repoRoot, "nested", "pkg")
	projectDBPath := filepath.Join(t.TempDir(), "project", databaseFile)
	writeFile(t, filepath.Join(repoRoot, projectConfigDir, configFile), "runner: project-runner\nfeedback_commands:\n  - project feedback\ndb: "+projectDBPath+"\n")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	chdir(t, workDir)

	settings, err := Load("", "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.Runner != "project-runner" {
		t.Fatalf("runner = %q, want project-runner", settings.Runner)
	}
	if !reflect.DeepEqual(settings.FeedbackCommands, []string{"project feedback"}) {
		t.Fatalf("feedback commands = %#v, want project feedback", settings.FeedbackCommands)
	}
	if settings.DBPath != projectDBPath {
		t.Fatalf("db path = %q, want %q", settings.DBPath, projectDBPath)
	}
	if got := viper.GetString("runner"); got != "project-runner" {
		t.Fatalf("viper runner = %q, want project-runner", got)
	}
}

func TestLoadDefaultsToPiRunner(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	settings, err := Load("", "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.Runner != defaultRunnerCommand {
		t.Fatalf("runner = %q, want %q", settings.Runner, defaultRunnerCommand)
	}
	if settings.RunnerCommand != defaultRunnerCommand {
		t.Fatalf("runner command = %q, want %q", settings.RunnerCommand, defaultRunnerCommand)
	}
	if !reflect.DeepEqual(settings.RunnerArgs, []string{defaultRunnerPromptFlag}) {
		t.Fatalf("runner args = %#v, want prompt flag", settings.RunnerArgs)
	}
}

func TestLoadReadsRunnerCommandAndArgs(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, configPath, "runner:\n  command: custom-runner\n  args:\n    - --one\n    - two\n")

	settings, err := Load(configPath, "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.RunnerCommand != "custom-runner" {
		t.Fatalf("runner command = %q, want custom-runner", settings.RunnerCommand)
	}
	if !reflect.DeepEqual(settings.RunnerArgs, []string{"--one", "two"}) {
		t.Fatalf("runner args = %#v, want configured args", settings.RunnerArgs)
	}
}

func TestLoadReadsFlatRunnerCommandAndArgs(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, configPath, "runner_command: flat-runner\nrunner_args:\n  - --flat\n")

	settings, err := Load(configPath, "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.RunnerCommand != "flat-runner" {
		t.Fatalf("runner command = %q, want flat-runner", settings.RunnerCommand)
	}
	if !reflect.DeepEqual(settings.RunnerArgs, []string{"--flat"}) {
		t.Fatalf("runner args = %#v, want flat args", settings.RunnerArgs)
	}
}

func TestLoadReadsPromptTemplatePath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	want := filepath.Join(t.TempDir(), "prompt.tmpl")
	writeFile(t, configPath, "prompt:\n  template: "+want+"\n")

	settings, err := Load(configPath, "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.PromptTemplatePath != want {
		t.Fatalf("prompt template path = %q, want %q", settings.PromptTemplatePath, want)
	}
	if got := viper.GetString(promptTemplateKey); got != want {
		t.Fatalf("viper prompt template = %q, want %q", got, want)
	}
}

func TestLoadReadsFlatPromptTemplatePath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	want := filepath.Join(t.TempDir(), "prompt.tmpl")
	writeFile(t, configPath, "prompt_template: "+want+"\n")

	settings, err := Load(configPath, "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.PromptTemplatePath != want {
		t.Fatalf("prompt template path = %q, want %q", settings.PromptTemplatePath, want)
	}
}

func TestLoadUsesUserConfigDatabasePath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	want := filepath.Join(t.TempDir(), "user", databaseFile)
	writeFile(t, filepath.Join(configHome, configDir, configFile), "db: "+want+"\n")

	settings, err := Load("", "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.DBPath != want {
		t.Fatalf("db path = %q, want %q", settings.DBPath, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestLoadDatabasePathPrecedence(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	isolateConfigEnv(t)

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configDBPath := filepath.Join(t.TempDir(), "config", databaseFile)
	writeFile(t, filepath.Join(configHome, configDir, configFile), "db: "+configDBPath+"\n")

	envDBPath := filepath.Join(t.TempDir(), "env", databaseFile)
	t.Setenv(databaseEnvVar, envDBPath)
	flagDBPath := filepath.Join(t.TempDir(), "flag", databaseFile)

	settings, err := Load("", flagDBPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if settings.DBPath != flagDBPath {
		t.Fatalf("db path = %q, want flag path %q", settings.DBPath, flagDBPath)
	}

	settings, err = Load("", "")
	if err != nil {
		t.Fatalf("load config without flag: %v", err)
	}
	if settings.DBPath != envDBPath {
		t.Fatalf("db path = %q, want env path %q", settings.DBPath, envDBPath)
	}
}

func TestResolveDatabasePathUsesExplicitPath(t *testing.T) {
	t.Setenv(databaseEnvVar, filepath.Join(t.TempDir(), "env", databaseFile))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))

	want := filepath.Join(t.TempDir(), "explicit", "custom.db")
	got, err := ResolveDatabasePath(want)
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestResolveDatabasePathUsesEnvWhenExplicitAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))

	want := filepath.Join(t.TempDir(), "env", databaseFile)
	t.Setenv(databaseEnvVar, want)

	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestResolveDatabasePathUsesXDGDataHome(t *testing.T) {
	t.Setenv(databaseEnvVar, "")

	xdgDataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	want := filepath.Join(xdgDataHome, databaseDir, databaseFile)
	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func TestResolveDatabasePathFallsBackToLocalShare(t *testing.T) {
	t.Setenv(databaseEnvVar, "")
	t.Setenv("XDG_DATA_HOME", "")

	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".local", "share", databaseDir, databaseFile)
	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("resolve database path: %v", err)
	}
	if got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
	assertDirExists(t, filepath.Dir(want))
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
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

func isolateConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv(databaseEnvVar, "")
	t.Setenv(viperDatabaseEnvVar, "")
	t.Setenv("GORALPH_RUNNER", "")
	t.Setenv("GORALPH_RUNNER_COMMAND", "")
	t.Setenv("GORALPH_RUNNER_ARGS", "")
	t.Setenv("GORALPH_FEEDBACK_COMMANDS", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("parent path %q is not a directory", path)
	}
}
