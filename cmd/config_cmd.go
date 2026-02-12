package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
)

var configCmd *cobra.Command
var configShowCmd *cobra.Command
var configPathCmd *cobra.Command
var configEditCmd *cobra.Command
var configResetCmd *cobra.Command
var configSetCmd *cobra.Command

func init() {
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "View or edit Termia configuration",
	}

	// config show - print current config as TOML
	configShowCmd = &cobra.Command{
		Use:   "show",
		Short: "Print current configuration as TOML",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow()
		},
	}

	// config path - print config file path
	configPathCmd = &cobra.Command{
		Use:   "path",
		Short: "Print configuration file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPath()
		},
	}

	// config edit - open config in $EDITOR
	configEditCmd = &cobra.Command{
		Use:   "edit",
		Short: "Open configuration in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigEdit()
		},
	}

	// config reset - reset to defaults
	configResetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration to defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigReset()
		},
	}

	// config set - set a config value (stub)
	configSetCmd = &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(args[0], args[1])
		},
	}

	// Add subcommands to config
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configSetCmd)

	rootCmd.AddCommand(configCmd)
}

func runConfigShow() error {
	// Load current config
	loadedCfg, err := config.LoadOrDefault()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Encode to TOML and print
	encoder := toml.NewEncoder(os.Stdout)
	if err := encoder.Encode(loadedCfg); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

func runConfigPath() error {
	fmt.Println(config.ConfigPath())
	return nil
}

func runConfigEdit() error {
	configPath := config.ConfigPath()

	// Get editor from environment, default to vi
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Open config file in editor
	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	return nil
}

func runConfigReset() error {
	configPath := config.ConfigPath()

	// Prompt for confirmation
	fmt.Printf("This will reset the config to defaults at %s. Continue? (yes/no): ", configPath)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if strings.TrimSpace(input) != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	// Get default config
	defaultCfg := config.DefaultConfig()

	// Save it
	if err := config.Save(defaultCfg, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Configuration reset to defaults at %s\n", configPath)
	return nil
}

func runConfigSet(key, value string) error {
	fmt.Printf("TODO: config set %s %s\n", key, value)
	fmt.Println("Note: TOML key path manipulation is complex and not yet implemented")
	return nil
}
