package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/shell"
	"golang.org/x/term"
)

var initShellCmd *cobra.Command

func init() {
	initShellCmd = &cobra.Command{
		Use:   "init [shell]",
		Short: "Initialize shell integration",
		Long: `Initialize shell integration by printing the eval-able snippet.

Add the output of this command to your shell configuration file (.zshrc, .bashrc, etc.)
to enable Termia integration in your shell.

Examples:
  termia init           # Auto-detect shell
  termia init zsh       # Use zsh
  termia init bash      # Use bash`,
		Args: cobra.MaximumNArgs(1),
		RunE: initShellRun,
	}

	rootCmd.AddCommand(initShellCmd)
}

func initShellRun(cmd *cobra.Command, args []string) error {
	var shellType shell.ShellType

	// Determine shell type
	if len(args) > 0 {
		// User provided shell name
		shellType = shell.DetectFromPath(args[0])
	} else {
		// Auto-detect shell
		shellInfo := shell.Detect()
		shellType = shellInfo.Type
	}

	// Install integration scripts
	if err := shell.InstallIntegration(shellType); err != nil {
		return fmt.Errorf("failed to install integration: %w", err)
	}

	// Get the init script (the source command)
	initScript, err := shell.PrintInitScript(shellType)
	if err != nil {
		return fmt.Errorf("failed to get init script: %w", err)
	}

	// Print the init script to stdout
	fmt.Println(initScript)

	// If stderr is a terminal, print helpful instructions
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprintf(os.Stderr, "\n# Add the above line to your shell configuration file:\n")
		switch shellType {
		case shell.ShellZsh:
			fmt.Fprintf(os.Stderr, "# echo '%s' >> ~/.zshrc\n", initScript)
		case shell.ShellBash:
			fmt.Fprintf(os.Stderr, "# echo '%s' >> ~/.bashrc\n", initScript)
		case shell.ShellFish:
			fmt.Fprintf(os.Stderr, "# echo '%s' >> ~/.config/fish/config.fish\n", initScript)
		}
	}

	return nil
}
