package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

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
}

func isolateDatabaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GO_RALPH_DB", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg-data"))
}
