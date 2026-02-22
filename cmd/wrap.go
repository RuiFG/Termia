package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/diagnostics"
	"github.com/termia/termia/internal/shell"
	"github.com/termia/termia/internal/wrapper"
	"go.uber.org/zap"
	"golang.org/x/term"
)

var (
	wrapCmd  *cobra.Command
	noRecord bool
)

func init() {
	wrapCmd = &cobra.Command{
		Use:   "wrap",
		Short: "Start a Termia-wrapped shell",
		RunE:  wrapRun,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
	}

	wrapCmd.Flags().BoolVar(
		&noRecord,
		"no-record",
		false,
		"disable recording for privacy",
	)

	rootCmd.AddCommand(wrapCmd)

	// Set rootCmd.RunE to execute wrap command when no subcommand is given
	rootCmd.RunE = wrapRun
}

func wrapRun(cmd *cobra.Command, args []string) error {
	shellArgs, _ := stripNoRecordArg(rawShellArgs())

	return runWrapper(shellArgs, noRecord)
}

func ExecuteWrapper(rawArgs []string) error {
	if err := prepare(); err != nil {
		return err
	}

	shellArgs, parsedNoRecord := stripNoRecordArg(rawArgs)

	return runWrapper(shellArgs, parsedNoRecord)
}

func runWrapper(shellArgs []string, noRecordFlag bool) error {
	// Detect shell
	var shellInfo shell.ShellInfo
	func() {
		defer diagnostics.Track("startup.shell.detect", nil)()
		shellInfo = shell.Detect()
	}()

	if isNonInteractive(shellArgs) {
		return execShell(shellInfo.Path, shellArgs)
	}

	// Open database
	dbPath := config.DBPath()
	fields := map[string]string{
		"db_path": dbPath,
	}
	if info, statErr := os.Stat(dbPath); statErr == nil {
		fields["db_bytes"] = strconv.FormatInt(info.Size(), 10)
	}
	var database *db.DB
	if err := func() error {
		defer diagnostics.Track("startup.db.open", fields)()
		var err error
		database, err = db.Open(dbPath, logger)
		return err
	}(); err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Install shell integration
	if err := func() error {
		defer diagnostics.Track("startup.shell.install_integration", nil)()
		return shell.InstallIntegration(shellInfo.Type)
	}(); err != nil {
		logger.Warn("failed to install shell integration", zap.Error(err))
	}

	// Create wrapper
	opts := wrapper.Options{
		Shell:    shellInfo.Path,
		Args:     shellArgs,
		NoRecord: noRecordFlag,
		DB:       database,
		Logger:   logger,
	}

	var w *wrapper.Wrapper
	if err := func() error {
		defer diagnostics.Track("startup.wrapper.new", nil)()
		var err error
		w, err = wrapper.New(opts)
		return err
	}(); err != nil {
		return fmt.Errorf("failed to create wrapper: %w", err)
	}
	defer w.Close()

	// Start wrapper
	if err := func() error {
		defer diagnostics.Track("startup.wrapper.start", nil)()
		return w.Start()
	}(); err != nil {
		return fmt.Errorf("failed to start wrapper: %w", err)
	}

	// Wait for wrapper to complete and return exit code
	return w.Wait()
}

func rawShellArgs() []string {
	if len(os.Args) < 2 {
		return nil
	}

	if os.Args[1] == "wrap" {
		if len(os.Args) > 2 {
			return os.Args[2:]
		}
		return nil
	}

	return os.Args[1:]
}

func stripNoRecordArg(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}

	filtered := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == "--no-record" {
			found = true
			continue
		}
		filtered = append(filtered, arg)
	}

	return filtered, found
}

func isNonInteractive(args []string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return true
	}

	for _, arg := range args {
		if arg == "-c" {
			return true
		}
	}

	return false
}

func execShell(shellPath string, args []string) error {
	if shellPath == "" {
		return fmt.Errorf("shell path is empty")
	}

	cmd := exec.Command(shellPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	return cmd.Run()
}
