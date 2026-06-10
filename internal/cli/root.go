package cli

import (
	"context"
	"fmt"

	"goralph/internal/config"
	"goralph/internal/db"

	"github.com/spf13/cobra"
)

type settingsContextKey struct{}

// NewRootCommand creates the root goralph command.
func NewRootCommand() *cobra.Command {
	var cfgFile string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "goralph",
		Short: "Run Ralph loops for Go projects",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			settings, err := config.Load(cfgFile, dbPath)
			if err != nil {
				return err
			}
			cmd.SetContext(context.WithValue(cmd.Context(), settingsContextKey{}, settings))
			if isDBCommand(cmd) {
				return nil
			}
			return migrateDatabase(cmd.Context(), settings.DBPath)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "SQLite database path")
	cmd.AddCommand(newDBCommand())

	return cmd
}

func migrateDatabase(ctx context.Context, path string) error {
	database, err := db.Open(path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		return err
	}

	return nil
}
