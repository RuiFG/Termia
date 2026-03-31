package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
	"golang.org/x/term"
)

var (
	taiCmd         *cobra.Command
	taiLastN       int
	taiAll         bool
	taiHistoryMode string
	taiMode        string
	taiTeam        string
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
		&taiHistoryMode,
		"history-mode",
		"H",
		"all",
		"history mode: cmd|ai|all",
	)
	taiCmd.Flags().StringVar(
		&taiMode,
		"mode",
		"assistant",
		"agent mode: assistant|team",
	)
	taiCmd.Flags().StringVar(
		&taiTeam,
		"team",
		"",
		"team definition name when using team mode",
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

	// Create context with signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	agentHistory := make([]agent.Command, len(selectedCommands))
	for i, cmd := range selectedCommands {
		agentHistory[i] = agent.Command{
			ID:                  cmd.ID,
			TsStart:             cmd.TsStart,
			TsEnd:               cmd.TsEnd,
			Command:             cmd.Command,
			Cwd:                 cmd.Cwd,
			ExitCode:            cmd.ExitCode,
			DurationMs:          cmd.DurationMs,
			OutputSize:          cmd.OutputSize,
			TranscriptAvailable: cmd.TranscriptPath != nil,
		}
	}

	var streamReader *agent.StreamReader
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		streamReader = agent.NewStreamReader(os.Stdin)
	}

	runtime := agent.NewRuntime(cfg, database, agent.NewCLIResponder())
	stream, err := runtime.Run(ctx, agent.RunRequest{
		Mode:             agent.Mode(strings.ToLower(strings.TrimSpace(taiMode))),
		TeamName:         strings.TrimSpace(taiTeam),
		Query:            userQuery,
		Cwd:              workingDirectory(),
		SelectedCommands: agentHistory,
		StreamReader:     streamReader,
	})
	if err != nil {
		return fmt.Errorf("failed to run analysis: %w", err)
	}

	// Read from stream and print chunks
	var fullResponse strings.Builder
	renderer := newTaiRenderer()
	for chunk := range stream {
		switch chunk.Kind {
		case agent.RuntimeEventText:
			renderer.WriteAssistant(chunk.Text, &fullResponse)
		case agent.RuntimeEventToolCall:
			if chunk.ToolCall == nil {
				continue
			}
			renderer.WriteTool(*chunk.ToolCall)
		case agent.RuntimeEventToolResult:
			if chunk.ToolCall == nil {
				continue
			}
			renderer.WriteTool(*chunk.ToolCall)
		case agent.RuntimeEventError:
			renderer.WriteError(chunk.Text)
		}
	}
	renderer.Finish()

	// Store analysis in database
	analysis := &db.Analysis{
		ID:         generateID(),
		CommandIDs: "[]",
		Prompt:     userQuery,
		Response:   fullResponse.String(),
		Model:      describeTaiModel(strings.ToLower(strings.TrimSpace(taiMode)), strings.TrimSpace(taiTeam)),
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

	mode := strings.ToLower(strings.TrimSpace(taiHistoryMode))
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

func describeTaiModel(mode, team string) string {
	if mode != "team" {
		return "assistant"
	}
	team = strings.TrimSpace(team)
	if team == "" {
		return "team"
	}
	return "team:" + team
}

func workingDirectory() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

var (
	taiAssistantPrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6EDF3"}).Bold(true)
	taiToolPendingStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"})
	taiToolSuccessStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"})
	taiToolErrorStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"})
	taiErrorPrefixStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}).Bold(true)
	taiErrorBodyStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"})
)

type taiRenderer struct {
	assistantOpen      bool
	assistantLineStart bool
	assistantFirstLine bool
	toolCalls          map[string]agent.ToolCallEvent
	toolStateKeys      map[string]string
	bufferedTools      map[string]agent.ToolCallEvent
	bufferedToolOrder  []string
}

func newTaiRenderer() *taiRenderer {
	return &taiRenderer{
		assistantLineStart: true,
		assistantFirstLine: true,
		toolCalls:          make(map[string]agent.ToolCallEvent),
		toolStateKeys:      make(map[string]string),
		bufferedTools:      make(map[string]agent.ToolCallEvent),
	}
}

func (r *taiRenderer) WriteAssistant(text string, fullResponse *strings.Builder) {
	if text == "" {
		return
	}
	if fullResponse != nil {
		fullResponse.WriteString(text)
	}
	if !r.assistantOpen {
		r.assistantOpen = true
		r.assistantLineStart = true
		r.assistantFirstLine = true
	}
	agent.LockConsoleOutput()
	defer agent.UnlockConsoleOutput()
	if r.flushBufferedToolsLocked() {
		r.assistantOpen = true
		r.assistantLineStart = true
		r.assistantFirstLine = true
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for len(text) > 0 {
		if r.assistantLineStart {
			if r.assistantFirstLine {
				fmt.Print(taiAssistantPrefixStyle.Render("• "))
				r.assistantFirstLine = false
			} else {
				fmt.Print("  ")
			}
			r.assistantLineStart = false
		}
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			fmt.Print(text)
			return
		}
		fmt.Print(text[:idx+1])
		text = text[idx+1:]
		r.assistantLineStart = true
	}
}

func (r *taiRenderer) WriteTool(toolCall agent.ToolCallEvent) {
	toolCall, key, ok := r.prepareToolCallForRender(toolCall)
	if !ok {
		return
	}
	if existing, exists := r.bufferedTools[key]; exists {
		if !shouldReplaceBufferedTool(existing, toolCall) {
			return
		}
	} else {
		r.bufferedToolOrder = append(r.bufferedToolOrder, key)
	}
	r.bufferedTools[key] = toolCall
}

func (r *taiRenderer) WriteError(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	agent.LockConsoleOutput()
	defer agent.UnlockConsoleOutput()
	r.flushBufferedToolsLocked()
	r.closeAssistantLineLocked()
	fmt.Println(taiErrorPrefixStyle.Render("• ") + taiErrorBodyStyle.Render(text))
}

func (r *taiRenderer) Finish() {
	agent.LockConsoleOutput()
	defer agent.UnlockConsoleOutput()
	r.flushBufferedToolsLocked()
	r.closeAssistantLineLocked()
}

func (r *taiRenderer) closeAssistantLine() {
	agent.LockConsoleOutput()
	defer agent.UnlockConsoleOutput()
	r.closeAssistantLineLocked()
}

func (r *taiRenderer) closeAssistantLineLocked() {
	if r.assistantOpen && !r.assistantLineStart {
		fmt.Println()
	}
	r.assistantOpen = false
	r.assistantLineStart = true
	r.assistantFirstLine = true
}

func (r *taiRenderer) normalizeToolCall(toolCall agent.ToolCallEvent) (agent.ToolCallEvent, string) {
	key := strings.TrimSpace(toolCall.CallID)
	if key == "" {
		key = strings.TrimSpace(toolCall.ToolName) + "|" + strings.TrimSpace(toolCall.Summary)
	}
	if previous, ok := r.toolCalls[key]; ok {
		if strings.TrimSpace(toolCall.CallID) == "" {
			toolCall.CallID = previous.CallID
		}
		if strings.TrimSpace(toolCall.AgentName) == "" {
			toolCall.AgentName = previous.AgentName
		}
		if strings.TrimSpace(toolCall.ToolName) == "" {
			toolCall.ToolName = previous.ToolName
		}
		if strings.TrimSpace(toolCall.Summary) == "" {
			toolCall.Summary = previous.Summary
		}
	}
	r.toolCalls[key] = toolCall
	return toolCall, key
}

func (r *taiRenderer) prepareToolCallForRender(toolCall agent.ToolCallEvent) (agent.ToolCallEvent, string, bool) {
	toolCall, rawKey := r.normalizeToolCall(toolCall)
	if toolCall.ToolName == "" {
		return agent.ToolCallEvent{}, "", false
	}
	if strings.EqualFold(strings.TrimSpace(toolCall.ToolName), "request_input") {
		return agent.ToolCallEvent{}, "", false
	}
	if toolCall.State == agent.ToolCallStatePending {
		return agent.ToolCallEvent{}, "", false
	}
	key := taiToolDisplayKey(toolCall)
	if key == "" {
		key = rawKey
	}
	stateKey := taiToolStateKey(toolCall)
	if previous, ok := r.toolStateKeys[key]; ok && previous == stateKey {
		return agent.ToolCallEvent{}, "", false
	}
	r.toolStateKeys[key] = stateKey
	return toolCall, key, true
}

func taiToolStateKey(toolCall agent.ToolCallEvent) string {
	return strings.Join([]string{
		strings.TrimSpace(toolCall.ToolName),
		strings.TrimSpace(toolCall.Summary),
		strings.TrimSpace(toolCall.Result),
		string(toolCall.State),
	}, "|")
}

func taiToolDisplayKey(toolCall agent.ToolCallEvent) string {
	parts := []string{}
	if agentName := strings.TrimSpace(toolCall.AgentName); agentName != "" && !strings.EqualFold(agentName, "assistant") {
		parts = append(parts, agentName)
	}
	if toolName := strings.TrimSpace(toolCall.ToolName); toolName != "" {
		parts = append(parts, toolName)
	}
	if summary := strings.TrimSpace(toolCall.Summary); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(parts, "|")
}

func shouldReplaceBufferedTool(current, next agent.ToolCallEvent) bool {
	if current.State == agent.ToolCallStateSuccess && next.State == agent.ToolCallStateError {
		return false
	}
	if current.State == agent.ToolCallStateError && next.State == agent.ToolCallStateSuccess {
		return true
	}
	if strings.TrimSpace(current.Result) == "" && strings.TrimSpace(next.Result) != "" {
		return true
	}
	return true
}

func (r *taiRenderer) flushBufferedToolsLocked() bool {
	if len(r.bufferedToolOrder) == 0 {
		return false
	}
	r.closeAssistantLineLocked()
	for _, key := range r.bufferedToolOrder {
		toolCall, ok := r.bufferedTools[key]
		if !ok {
			continue
		}
		fmt.Println(renderTaiToolCall(toolCall))
	}
	clear(r.bufferedTools)
	r.bufferedToolOrder = r.bufferedToolOrder[:0]
	return true
}

func renderTaiToolCall(toolCall agent.ToolCallEvent) string {
	parts := []string{}
	if agentName := strings.TrimSpace(toolCall.AgentName); agentName != "" && !strings.EqualFold(agentName, "user") {
		if !strings.EqualFold(agentName, "assistant") {
			parts = append(parts, agentName)
		}
	}
	head := strings.TrimSpace(toolCall.ToolName)
	if summary := strings.TrimSpace(toolCall.Summary); summary != "" {
		head = strings.TrimSpace(head + " " + summary)
	}
	if head != "" {
		parts = append(parts, head)
	}
	if result := strings.TrimSpace(toolCall.Result); result != "" && !strings.EqualFold(result, "ok") && !strings.EqualFold(result, "done") {
		parts = append(parts, "· "+result)
	}
	style := taiToolPendingStyle
	switch toolCall.State {
	case agent.ToolCallStateSuccess:
		style = taiToolSuccessStyle
	case agent.ToolCallStateError:
		style = taiToolErrorStyle
	}
	return style.Render("• " + strings.Join(parts, " "))
}
