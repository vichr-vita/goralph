package cli

import (
	"goralph/internal/config"

	"github.com/spf13/cobra"
)

// NewRootCommand creates the root goralph command.
func NewRootCommand() *cobra.Command {
	var cfgFile string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "goralph",
		Short: "Run Ralph loops for Go projects",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := config.Load(cfgFile, dbPath)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "SQLite database path")

	return cmd
}
