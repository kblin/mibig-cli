// Package cmd implements the commands of the mibig tool.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "mibig",
	Short: "Work with MIBiG entries",
	Long: `mibig works with MIBiG JSON entries.

Settings are read from flags, MIBIG_* environment variables (e.g.
MIBIG_EDITOR) and the config file, in that order of precedence. The
config file defaults to $XDG_CONFIG_HOME/mibig/config.yaml
(~/.config/mibig/config.yaml).`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initConfig()
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $XDG_CONFIG_HOME/mibig/config.yaml)")
}

func initConfig() error {
	viper.SetEnvPrefix("mibig")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		dir, err := configDir()
		if err != nil {
			return nil // no config directory, use defaults
		}
		viper.AddConfigPath(filepath.Join(dir, "mibig"))
		viper.SetConfigName("config")
	}
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if cfgFile == "" && errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("reading config: %w", err)
	}
	return nil
}

// configDir returns $XDG_CONFIG_HOME, or ~/.config. Unlike
// os.UserConfigDir, it does not use ~/Library/Application Support on macOS,
// which is unusual for command line tools.
func configDir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}
