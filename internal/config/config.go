package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load reads goralph configuration from an explicit path, default config paths, and environment.
func Load(cfgFile string) error {
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
