package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/diagnostics"
	"go.uber.org/zap"
)

// Version is set by ldflags at build time
var Version = "dev"

var (
	cfg     *config.Config
	logger  *zap.Logger
	verbose bool
)

// rootCmd is initialized at package level so all init() functions across
// cmd/*.go files can safely call rootCmd.AddCommand() regardless of init order.
var rootCmd = &cobra.Command{
	Use:   "termia",
	Short: "Your terminal, elevated with AI",
	Long: ` _____ _____ ____  __  __ ___    _    
|_   _| ____|  _ \|  \/  |_ _|  / \   
  | | |  _| | |_) | |\/| || |  / _ \  
  | | | |___|  _ <| |  | || | / ___ \ 
  |_| |_____|_| \_\_|  |_|___/_/   \_\

Your terminal, elevated with AI`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       Version,
}

func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return prepare()
	}

	// Add persistent flags
	rootCmd.PersistentFlags().BoolVarP(
		&verbose,
		"verbose",
		"v",
		false,
		"enable debug logging",
	)
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func prepare() error {
	// Initialize logger — silent by default, verbose (-v) enables debug output
	if verbose {
		var logErr error
		logger, logErr = zap.NewDevelopment()
		if logErr != nil {
			return logErr
		}
	} else {
		logger = zap.NewNop()
	}

	// Load or create default config
	var err error
	if err := func() error {
		defer diagnostics.Track("startup.prepare.config_load", nil)()
		cfg, err = config.LoadOrDefault()
		return err
	}(); err != nil {
		return err
	}

	// Ensure all required directories exist
	if err := func() error {
		defer diagnostics.Track("startup.prepare.ensure_dirs", nil)()
		return config.EnsureDirs()
	}(); err != nil {
		return err
	}

	return nil
}

func ShouldRunWrapper(args []string) bool {
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}

		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == arg || cmd.HasAlias(arg) {
				return false
			}
		}

		return true
	}

	return true
}

type exitCoder interface {
	ExitCode() int
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr exitCoder
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return 1
}
