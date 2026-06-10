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
	databaseEnvVar      = "GO_RALPH_DB"
	viperDatabaseEnvVar = "GORALPH_DB"
	databaseDir         = "goralph"
	databaseFile        = "ralph.db"
	configDir           = "goralph"
	configFile          = "config.yaml"
	projectConfigDir    = ".ralph"
	feedbackCommandsKey = "feedback_commands"
)

// Settings contains resolved configuration values.
type Settings struct {
	DBPath           string
	Runner           string
	FeedbackCommands []string
}

// Load reads goralph configuration from an explicit path, user config, project config, and environment.
func Load(cfgFile string, dbPath string) (*Settings, error) {
	v := viper.GetViper()
	configureViper(v)

	if cfgFile != "" {
		if err := mergeConfigFile(v, cfgFile); err != nil {
			return nil, fmt.Errorf("load config %s: %w", cfgFile, err)
		}
	} else {
		if userConfigPath := xdgConfigPath(); userConfigPath != "" {
			if err := mergeOptionalConfigFile(v, userConfigPath); err != nil {
				return nil, fmt.Errorf("load user config %s: %w", userConfigPath, err)
			}
		}

		projectConfigPath, err := findProjectConfigPath()
		if err != nil {
			return nil, err
		}
		if projectConfigPath != "" {
			if err := mergeOptionalConfigFile(v, projectConfigPath); err != nil {
				return nil, fmt.Errorf("load project config %s: %w", projectConfigPath, err)
			}
		}
	}

	resolvedDBPath, err := ResolveDatabasePath(dbPath, v.GetString("db"))
	if err != nil {
		return nil, err
	}
	v.Set("db", resolvedDBPath)
	v.Set(feedbackCommandsKey, feedbackCommands(v))

	return &Settings{
		DBPath:           resolvedDBPath,
		Runner:           v.GetString("runner"),
		FeedbackCommands: v.GetStringSlice(feedbackCommandsKey),
	}, nil
}

// ResolveDatabasePath returns the SQLite database path and creates its parent directory.
func ResolveDatabasePath(explicitPath string, configPath ...string) (string, error) {
	path := explicitPath
	if path == "" {
		path = envDatabasePath()
	}
	if path == "" && len(configPath) > 0 {
		path = configPath[0]
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

func configureViper(v *viper.Viper) {
	v.SetConfigType("yaml")
	v.SetEnvPrefix("GORALPH")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()
}

func mergeConfigFile(v *viper.Viper, path string) error {
	v.SetConfigFile(path)
	return v.MergeInConfig()
}

func mergeOptionalConfigFile(v *viper.Viper, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("config path is directory")
	}
	return mergeConfigFile(v, path)
}

func xdgConfigPath() string {
	configHome, err := os.UserConfigDir()
	if err != nil || configHome == "" {
		return ""
	}
	return filepath.Join(configHome, configDir, configFile)
}

func findProjectConfigPath() (string, error) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("resolve current directory for project config: %w", err)
	}

	for {
		candidate := filepath.Join(cwd, projectConfigDir, configFile)
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("project config path is directory: %s", candidate)
			}
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat project config %s: %w", candidate, err)
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", nil
		}
		cwd = parent
	}
}

func feedbackCommands(v *viper.Viper) []string {
	for _, key := range []string{feedbackCommandsKey, "feedback.commands"} {
		if commands := v.GetStringSlice(key); len(commands) > 0 {
			return commands
		}
	}
	for _, key := range []string{"feedback_command", "feedback.command"} {
		if command := v.GetString(key); command != "" {
			return []string{command}
		}
	}
	return nil
}

func envDatabasePath() string {
	if path := os.Getenv(databaseEnvVar); path != "" {
		return path
	}
	return os.Getenv(viperDatabaseEnvVar)
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
