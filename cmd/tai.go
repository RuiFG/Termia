package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

var (
	taiCmd   *cobra.Command
	taiLastN int
	taiAll   bool
	taiMode  string
)

func init() {
	if os.Getenv("TERMIA_WRAPPED") != "1" {
		return
	}

	// Create tai command
	taiCmd = &cobra.Command{
		Use:   "tai [flags] \"<prompt>\"",
		Short: "AI assistant for terminal analysis",
		RunE:  taiRun,
		Args:  cobra.MinimumNArgs(1),
	}

	// Add flags to tai command
	taiCmd.Flags().IntVarP(
		&taiLastN,
		"last",
		"n",
		0,
		"number of recent commands to include",
	)
	taiCmd.Flags().BoolVar(
		&taiAll,
		"all",
		false,
		"include all recent commands",
	)
	taiCmd.Flags().StringVarP(
		&taiMode,
		"mode",
		"m",
		"all",
		"history mode: cmd|ai|all",
	)

	rootCmd.AddCommand(taiCmd)
}

// taiRun executes the tai command for lightweight analysis with tools.
func taiRun(cmd *cobra.Command, args []string) error {
	// Parse user query from arguments
	if len(args) == 0 {
		return fmt.Errorf("prompt is required")
	}
	if strings.HasPrefix(args[0], "h~") {
		args = args[1:]
	}
	userQuery := strings.Join(args, " ")
	if strings.TrimSpace(userQuery) == "" {
		return fmt.Errorf("prompt is required")
	}

	_ = os.Setenv("TERMIA_APPROVAL_MODE", "prompt")

	// Open database
	database, err := db.Open(config.DBPath(), logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Determine history selection
	selectedCommands, err := resolveTaiHistory(cmd, database)
	if err != nil {
		return err
	}

	// Create agent config (for metadata storage)
	agentCfg, err := agent.NewAgentConfigFromConfig(&cfg.LLM)
	if err != nil {
		fmt.Printf("⚠️  LLM not configured: %v. Set up with `termia config edit`\n", err)
		return nil
	}

	model, err := agent.NewModel(agentCfg)
	if err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}
	tools := agent.CreateTools(database, cfg.Agent.RequireApproval, agent.NewCLIApprovalProvider())
	reactRunner, err := agent.NewReactRunner(context.Background(), model, tools, database, logger)
	if err != nil {
		return fmt.Errorf("failed to create react runner: %w", err)
	}

	// Create context with signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Print header
	if len(selectedCommands) == 0 {
		fmt.Printf("🤖 Analyzing...\n\n")
	} else {
		fmt.Printf("🤖 Analyzing last %d command(s)...\n\n", len(selectedCommands))
	}

	// Run analysis
	stream, err := reactRunner.Run(ctx, userQuery, selectedCommands)
	if err != nil {
		return fmt.Errorf("failed to run analysis: %w", err)
	}

	// Read from stream and print chunks
	var fullResponse strings.Builder
	for chunk := range stream {
		fmt.Print(chunk)
		fullResponse.WriteString(chunk)
	}
	fmt.Println()

	// Store analysis in database
	analysis := &db.Analysis{
		ID:         generateID(),
		CommandIDs: "[]", // TODO: Populate with actual command IDs if needed
		Prompt:     userQuery,
		Response:   fullResponse.String(),
		Model:      agentCfg.Model,
		CreatedAt:  time.Now().UnixNano(),
	}

	if err := database.CreateAnalysis(analysis); err != nil {
		logger.Warn("failed to store analysis", zap.Error(err))
	}

	return nil
}

func resolveTaiHistory(cmd *cobra.Command, database *db.DB) ([]db.Command, error) {
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}

	limit := 0
	if taiAll {
		limit = 1000
	} else if taiLastN > 0 {
		limit = taiLastN
	}

	if limit == 0 {
		args := cmd.Flags().Args()
		if len(args) == 0 {
			return nil, nil
		}
		first := strings.TrimSpace(args[0])
		if !strings.HasPrefix(first, "h~") {
			return nil, nil
		}
		nStr := strings.TrimPrefix(first, "h~")
		count, err := strconv.Atoi(nStr)
		if err != nil || count <= 0 {
			return nil, fmt.Errorf("invalid history selector: %s", first)
		}
		limit = count
	}

	fetchLimit := limit + 5
	if taiAll {
		fetchLimit = limit
	}
	commands, err := database.ListRecentCommands(fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commands: %w", err)
	}

	mode := strings.ToLower(strings.TrimSpace(taiMode))
	if mode == "" {
		mode = "all"
	}
	switch mode {
	case "cmd", "ai", "all":
	default:
		return nil, fmt.Errorf("invalid history mode: %s", taiMode)
	}

	filtered := filterTaiHistory(commands, mode)
	if !taiAll && limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	reverseCommands(filtered)
	return filtered, nil
}

func filterTaiHistory(commands []db.Command, mode string) []db.Command {
	filtered := make([]db.Command, 0, len(commands))
	for _, cmd := range commands {
		trimmed := strings.TrimSpace(cmd.Command)
		if trimmed == "" {
			continue
		}
		isTai := isTaiCommand(trimmed)
		switch mode {
		case "cmd":
			if isTai {
				continue
			}
		case "ai":
			if !isTai {
				continue
			}
		}
		filtered = append(filtered, cmd)
	}
	return filtered
}

func isTaiCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	return lower == "tai" || strings.HasPrefix(lower, "tai ") || lower == "tui" || strings.HasPrefix(lower, "tui ") ||
		strings.HasPrefix(lower, "termia tai") || strings.HasPrefix(lower, "termia tui")
}

func reverseCommands(commands []db.Command) {
	for i, j := 0, len(commands)-1; i < j; i, j = i+1, j-1 {
		commands[i], commands[j] = commands[j], commands[i]
	}
}

// generateID generates a simple ID based on the current nanosecond timestamp
func generateID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
