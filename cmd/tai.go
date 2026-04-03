package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agentapp"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/sessionstate"
	"github.com/termia/termia/internal/textutil"
	"go.uber.org/zap"
	"golang.org/x/term"
)

var (
	taiCmd          *cobra.Command
	taiCommandCount int
	taiRecent       bool
	taiHistoryMode  string
	taiNewSession   bool
)

const (
	taiRecentCommandLimit     = 20
	taiWrapperStartedAtEnvKey = "TERMIA_WRAPPER_STARTED_AT"
)

type taiAgentAppService interface {
	Run(context.Context, agentapp.RunRequest) (<-chan agent.RuntimeEvent, error)
}

var openTaiDB = func() (*db.DB, error) {
	runLogger := logger
	if runLogger == nil {
		runLogger = zap.NewNop()
	}
	return db.Open(config.DBPath(), runLogger)
}

var newTaiAgentAppService = func(cfg *config.Config, database *db.DB) taiAgentAppService {
	return agentapp.NewService(cfg, database, func(cfg *config.Config, database *db.DB, responder agent.HITLResponder) agentapp.Runtime {
		return agent.NewRuntime(cfg, database, responder)
	})
}

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
		&taiCommandCount,
		"cmd",
		"c",
		0,
		"number of recent commands to include",
	)
	taiCmd.Flags().BoolVar(
		&taiRecent,
		"recent",
		false,
		"include up to 20 commands from the current shell session",
	)
	taiCmd.Flags().Lookup("recent").Shorthand = "r"
	taiCmd.Flags().BoolVarP(
		&taiNewSession,
		"new-session",
		"n",
		false,
		"start a new conversation session",
	)
	taiCmd.Flags().StringVarP(
		&taiHistoryMode,
		"history-mode",
		"H",
		"cmd",
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
	database, err := openTaiDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Determine history selection
	selectedCommands, err := resolveTaiHistory(cmd, database)
	if err != nil {
		return err
	}
	cwd := workingDirectory()
	sessionID := strings.TrimSpace(sessionstate.CurrentID())

	// Create context with signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var streamReader *agent.StreamReader
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		streamReader = agent.NewStreamReader(os.Stdin)
	}

	appService := newTaiAgentAppService(cfg, database)
	stream, err := appService.Run(ctx, agentapp.RunRequest{
		SessionID:        sessionID,
		Query:            userQuery,
		Cwd:              cwd,
		NewSession:       taiNewSession,
		SelectedCommands: taiAgentCommandsFromDBCommands(selectedCommands),
		StreamReader:     streamReader,
		Responder:        agent.NewCLIResponder(),
	})
	if err != nil {
		return fmt.Errorf("failed to run analysis: %w", err)
	}

	// Read from stream and print chunks
	var fullResponse strings.Builder
	var streamErrorText string
	renderer := newTaiRenderer()
	for chunk := range stream {
		switch chunk.Kind {
		case agent.RuntimeEventText:
			renderer.WriteAssistant(chunk.Text, &fullResponse)
		case agent.RuntimeEventReasoning:
			renderer.WriteReasoning(chunk.Text)
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
			if streamErrorText == "" {
				if text := strings.TrimSpace(chunk.Text); text != "" {
					streamErrorText = text
				}
			}
			renderer.WriteError(chunk.Text)
		}
	}
	renderer.Finish()
	if streamErrorText != "" {
		return fmt.Errorf("analysis failed: %s", streamErrorText)
	}

	// Store analysis in database
	analysis := &db.Analysis{
		ID:         generateID(),
		CommandIDs: taiEncodeSelectedCommandIDs(selectedCommands),
		Prompt:     userQuery,
		Response:   fullResponse.String(),
		Model:      taiAnalysisModel(database, sessionID),
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
	if taiRecent {
		limit = taiRecentCommandLimit
	} else if taiCommandCount > 0 {
		limit = taiCommandCount
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

	commands, err := loadTaiHistoryCommands(database, limit)
	if err != nil {
		return nil, err
	}

	mode, err := normalizeTaiHistoryMode(taiHistoryMode)
	if err != nil {
		return nil, err
	}

	filtered := filterTaiHistory(commands, mode)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	reverseCommands(filtered)
	return filtered, nil
}

func loadTaiHistoryCommands(database *db.DB, limit int) ([]db.Command, error) {
	if taiRecent {
		startedAt, err := taiWrapperStartedAt()
		if err != nil {
			return nil, err
		}
		commands, err := database.ListRecentCommandsSince(startedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch recent commands: %w", err)
		}
		return commands, nil
	}
	commands, err := database.ListRecentCommands(limit + 5)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commands: %w", err)
	}
	return commands, nil
}

func taiWrapperStartedAt() (int64, error) {
	raw := strings.TrimSpace(os.Getenv(taiWrapperStartedAtEnvKey))
	if raw == "" {
		return 0, fmt.Errorf("wrapper start timestamp is unavailable; restart the Termia shell to use --recent")
	}
	startedAt, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || startedAt <= 0 {
		return 0, fmt.Errorf("invalid wrapper start timestamp: %s", raw)
	}
	return startedAt, nil
}

func normalizeTaiHistoryMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "cmd", nil
	}
	switch mode {
	case "cmd", "ai", "all":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid history mode: %s", mode)
	}
}

func filterTaiHistory(commands []db.Command, mode string) []db.Command {
	filtered := make([]db.Command, 0, len(commands))
	for _, cmd := range commands {
		trimmed := strings.TrimSpace(cmd.Command)
		if trimmed == "" {
			continue
		}
		isTai := isTaiCommand(cmd)
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

func isTaiCommand(command db.Command) bool {
	lower := strings.ToLower(strings.TrimSpace(command.Command))
	return lower == "tai" || strings.HasPrefix(lower, "tai ") || lower == "tui" || strings.HasPrefix(lower, "tui ") ||
		strings.HasPrefix(lower, "termia tai") || strings.HasPrefix(lower, "termia tui") ||
		isTermiaLaunchCommand(command)
}

func isTermiaLaunchCommand(command db.Command) bool {
	termiaBin := strings.TrimSpace(os.Getenv("TERMIA_BIN"))
	if termiaBin == "" {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(command.Command))
	if len(fields) == 0 {
		return false
	}
	commandPath := strings.Trim(fields[0], `"'`)
	if commandPath == "" {
		return false
	}
	if !filepath.IsAbs(commandPath) && strings.ContainsAny(commandPath, `/\`) {
		cwd := strings.TrimSpace(command.Cwd)
		if cwd == "" {
			return false
		}
		commandPath = filepath.Join(cwd, commandPath)
	}
	return filepath.Clean(commandPath) == filepath.Clean(termiaBin)
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

func taiAnalysisModel(database *db.DB, preferredSessionID string) string {
	if database == nil {
		return "assistant"
	}
	sessionID := strings.TrimSpace(preferredSessionID)
	if sessionID != "" {
		session, ok, err := database.GetAgentSession(sessionID)
		if err == nil && ok {
			return describeTaiModel(session.Mode, session.TeamName)
		}
	}
	session, ok, err := database.LatestAgentSession()
	if err == nil && ok {
		return describeTaiModel(session.Mode, session.TeamName)
	}
	return "assistant"
}

func workingDirectory() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func taiAgentCommandsFromDBCommands(commands []db.Command) []agent.Command {
	if len(commands) == 0 {
		return nil
	}
	result := make([]agent.Command, 0, len(commands))
	for _, cmd := range commands {
		if strings.TrimSpace(cmd.ID) == "" || strings.TrimSpace(cmd.Command) == "" {
			continue
		}
		result = append(result, agent.Command{
			ID:                  cmd.ID,
			TsStart:             cmd.TsStart,
			TsEnd:               cmd.TsEnd,
			Command:             cmd.Command,
			Cwd:                 cmd.Cwd,
			ExitCode:            cmd.ExitCode,
			DurationMs:          cmd.DurationMs,
			OutputSize:          cmd.OutputSize,
			TranscriptAvailable: cmd.TranscriptPath != nil,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func taiEncodeSelectedCommandIDs(commands []db.Command) string {
	if len(commands) == 0 {
		return "[]"
	}
	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		if command.ID == "" {
			continue
		}
		ids = append(ids, command.ID)
	}
	if len(ids) == 0 {
		return "[]"
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func taiNormalizeToolCall(toolCall agent.ToolCallEvent) agent.ToolCallEvent {
	return agent.ToolCallEvent{
		CallID:    strings.TrimSpace(toolCall.CallID),
		AgentName: textutil.NormalizeInlineText(toolCall.AgentName),
		ToolName:  textutil.NormalizeInlineText(toolCall.ToolName),
		Summary:   textutil.NormalizeInlineText(toolCall.Summary),
		Result:    textutil.NormalizeInlineText(toolCall.Result),
		State:     toolCall.State,
	}
}

var (
	taiAssistantPrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6EDF3"}).Bold(true)
	taiReasoningPrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"}).Bold(true)
	taiReasoningBodyStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"}).Italic(true)
	taiToolPendingStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"})
	taiToolSuccessStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"})
	taiToolErrorStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"})
	taiErrorPrefixStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}).Bold(true)
	taiErrorBodyStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"})
)

type taiStreamKind int

const (
	taiStreamNone taiStreamKind = iota
	taiStreamAssistant
	taiStreamReasoning
)

type taiStreamStyle struct {
	PrefixStyle  lipgloss.Style
	BodyStyle    lipgloss.Style
	Prefix       string
	Continuation string
}

type taiRenderer struct {
	streamKind        taiStreamKind
	streamOpen        bool
	streamLineStart   bool
	streamFirstLine   bool
	toolCalls         map[string]agent.ToolCallEvent
	toolStateKeys     map[string]string
	bufferedTools     map[string]agent.ToolCallEvent
	bufferedToolOrder []string
}

func newTaiRenderer() *taiRenderer {
	return &taiRenderer{
		streamLineStart: true,
		streamFirstLine: true,
		toolCalls:       make(map[string]agent.ToolCallEvent),
		toolStateKeys:   make(map[string]string),
		bufferedTools:   make(map[string]agent.ToolCallEvent),
	}
}

func (r *taiRenderer) WriteAssistant(text string, fullResponse *strings.Builder) {
	text = textutil.NormalizeLineEndings(text)
	if text == "" {
		return
	}
	if fullResponse != nil {
		fullResponse.WriteString(text)
	}
	r.writeStreamText(taiStreamAssistant, text)
}

func (r *taiRenderer) WriteReasoning(text string) {
	r.writeStreamText(taiStreamReasoning, text)
}

func (r *taiRenderer) writeStreamText(kind taiStreamKind, text string) {
	text = textutil.NormalizeLineEndings(text)
	if text == "" {
		return
	}
	agent.LockConsoleOutput()
	defer agent.UnlockConsoleOutput()
	if r.flushBufferedToolsLocked() {
		r.resetStreamLocked()
	}
	if r.streamKind != kind {
		r.closeStreamLocked()
		r.streamKind = kind
	}
	if !r.streamOpen {
		r.streamOpen = true
		r.streamLineStart = true
		r.streamFirstLine = true
	}
	style := taiStreamStyleFor(kind)
	for len(text) > 0 {
		if r.streamLineStart {
			if r.streamFirstLine {
				fmt.Print(style.PrefixStyle.Render(style.Prefix))
				r.streamFirstLine = false
			} else {
				fmt.Print(style.Continuation)
			}
			r.streamLineStart = false
		}
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			fmt.Print(style.BodyStyle.Render(text))
			return
		}
		fmt.Print(style.BodyStyle.Render(text[:idx+1]))
		text = text[idx+1:]
		r.streamLineStart = true
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
	r.closeStreamLocked()
	fmt.Println(taiErrorPrefixStyle.Render("• ") + taiErrorBodyStyle.Render(text))
}

func (r *taiRenderer) Finish() {
	agent.LockConsoleOutput()
	defer agent.UnlockConsoleOutput()
	r.flushBufferedToolsLocked()
	r.closeStreamLocked()
}

func (r *taiRenderer) closeStreamLocked() {
	if r.streamOpen && !r.streamLineStart {
		fmt.Println()
	}
	r.resetStreamLocked()
}

func (r *taiRenderer) resetStreamLocked() {
	r.streamKind = taiStreamNone
	r.streamOpen = false
	r.streamLineStart = true
	r.streamFirstLine = true
}

func taiStreamStyleFor(kind taiStreamKind) taiStreamStyle {
	switch kind {
	case taiStreamReasoning:
		return taiStreamStyle{
			PrefixStyle:  taiReasoningPrefixStyle,
			BodyStyle:    taiReasoningBodyStyle,
			Prefix:       "… ",
			Continuation: "  ",
		}
	default:
		return taiStreamStyle{
			PrefixStyle:  taiAssistantPrefixStyle,
			BodyStyle:    lipgloss.NewStyle(),
			Prefix:       "• ",
			Continuation: "  ",
		}
	}
}

func (r *taiRenderer) normalizeToolCall(toolCall agent.ToolCallEvent) (agent.ToolCallEvent, string) {
	toolCall = taiNormalizeToolCall(toolCall)
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
	r.closeStreamLocked()
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
	toolCall = taiNormalizeToolCall(toolCall)
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
