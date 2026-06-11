package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/vichr-vita/goralph/internal/config"

	"github.com/spf13/cobra"
)

func newDBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Manage the goralph database",
	}

	cmd.AddCommand(newDBPathCommand())
	cmd.AddCommand(newDBMigrateCommand())
	cmd.AddCommand(newDBResetCommand())

	return cmd
}

func newDBPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the database path",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			if jsonOutputFromContext(cmd.Context()) {
				return writeJSON(cmd, pathOutput{Path: settings.DBPath})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), settings.DBPath)
			return err
		},
	}
}

func newDBMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			if err := migrateDatabase(cmd.Context(), settings.DBPath); err != nil {
				return err
			}
			if jsonOutputFromContext(cmd.Context()) {
				return writeJSON(cmd, databaseActionOutput{OK: true, Action: "migrate", Path: settings.DBPath})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "migrated %s\n", settings.DBPath)
			return err
		},
	}
}

func newDBResetCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return errors.New("refusing to reset database without --force")
			}

			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			if err := resetDatabase(cmd.Context(), settings.DBPath); err != nil {
				return err
			}
			if jsonOutputFromContext(cmd.Context()) {
				return writeJSON(cmd, databaseActionOutput{OK: true, Action: "reset", Path: settings.DBPath})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "reset %s\n", settings.DBPath)
			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm destructive database reset")

	return cmd
}

func resetDatabase(ctx context.Context, path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove database file %s: %w", path+suffix, err)
		}
	}
	return migrateDatabase(ctx, path)
}

func isProjectlessCommand(cmd *cobra.Command) bool {
	if isPRDValidateCommand(cmd) {
		return true
	}
	for current := cmd; current != nil; current = current.Parent() {
		switch current.Name() {
		case "completion", "db":
			return true
		}
	}
	return false
}

func settingsFromContext(ctx context.Context) (*config.Settings, error) {
	settings, ok := ctx.Value(settingsContextKey{}).(*config.Settings)
	if !ok || settings == nil {
		return nil, errors.New("command settings not loaded")
	}
	return settings, nil
}
