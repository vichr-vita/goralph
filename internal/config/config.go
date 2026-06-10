package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	databaseEnvVar = "GO_RALPH_DB"
	databaseDir    = "goralph"
	databaseFile   = "ralph.db"
)

// Settings contains resolved configuration values.
type Settings struct {
	DBPath string
}

// Load reads goralph configuration from an explicit path, default config paths, and environment.
func Load(cfgFile string, dbPath string) (*Settings, error) {
	v := viper.GetViper()
	v.SetEnvPrefix("GORALPH")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".ralph")
		v.AddConfigPath("$HOME/.config/goralph")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	resolvedDBPath, err := ResolveDatabasePath(dbPath)
	if err != nil {
		return nil, err
	}
	v.Set("db", resolvedDBPath)

	return &Settings{DBPath: resolvedDBPath}, nil
}

// ResolveDatabasePath returns the SQLite database path and creates its parent directory.
func ResolveDatabasePath(explicitPath string) (string, error) {
	path := explicitPath
	if path == "" {
		path = os.Getenv(databaseEnvVar)
	}
	if path == "" {
		path = xdgDatabasePath()
	}
	if path == "" {
		fallback, err := fallbackDatabasePath()
		if err != nil {
			return "", err
		}
		path = fallback
	}

	if err := ensureParentDir(path); err != nil {
		return "", err
	}

	return path, nil
}

func xdgDatabasePath() string {
	xdgDataHome := os.Getenv("XDG_DATA_HOME")
	if xdgDataHome == "" {
		return ""
	}
	return filepath.Join(xdgDataHome, databaseDir, databaseFile)
}

func fallbackDatabasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for database path: %w", err)
	}
	if home == "" {
		return "", errors.New("resolve home directory for database path: empty home directory")
	}
	return filepath.Join(home, ".local", "share", databaseDir, databaseFile), nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database parent directory: %w", err)
	}
	return nil
}
