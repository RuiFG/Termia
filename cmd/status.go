package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/shell"
)

var statusCmd *cobra.Command

func init() {
	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show Termia status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}

	rootCmd.AddCommand(statusCmd)
}

func runStatus() error {
	// Open database
	database, err := db.Open(config.DBPath(), logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		return err
	}
	defer database.Close()

	// Print Termia version
	fmt.Printf("Termia Version: %s\n", Version)

	// Print paths
	fmt.Printf("Config Path: %s\n", config.ConfigPath())
	fmt.Printf("DB Path: %s\n", config.DBPath())
	fmt.Printf("Storage Dir: %s\n", config.TermiaDir())

	// Get DB file size
	dbPath := config.DBPath()
	if info, err := os.Stat(dbPath); err == nil {
		fmt.Printf("DB Size: %s\n", formatBytes(info.Size()))
	}

	// Show shell info
	shellInfo := shell.Detect()
	fmt.Printf("Shell: %s (%s)\n", shellInfo.Type, shellInfo.Path)
	if shellInfo.Version != "" {
		fmt.Printf("Shell Version: %s\n", shellInfo.Version)
	}

	// Check if wrapped
	wrappedEnv := os.Getenv("TERMIA_WRAPPED")
	if wrappedEnv != "" {
		fmt.Printf("Currently Wrapped: yes\n")
	} else {
		fmt.Printf("Currently Wrapped: no\n")
	}

	// List recent commands
	commands, err := database.ListRecentCommands(10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list recent commands: %v\n", err)
		return err
	}

	if len(commands) > 0 {
		fmt.Println("\nRecent Commands:")
		for i, c := range commands {
			cmd := c.Command
			if len(cmd) > 50 {
				cmd = cmd[:47] + "..."
			}
			fmt.Printf("  [%d] %s\n", i+1, cmd)
		}
	}

	return nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
