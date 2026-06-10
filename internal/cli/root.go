package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCommand creates the root goralph command.
func NewRootCommand() *cobra.Command {
	var cfgFile string

	cmd := &cobra.Command{
		Use:   "goralph",
		Short: "Run Ralph loops for Go projects",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig(cfgFile)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")

	return cmd
}

func loadConfig(cfgFile string) error {
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
		if errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("load config: %w", err)
	}

	return nil
}
