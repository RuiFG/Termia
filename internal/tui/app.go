package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agent/team"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

// Focus state for the 3 panels.
type Focus int

const (
	FocusHistory Focus = iota
	FocusContent
	FocusInput
)

// MiddleMode determines what the middle panel displays.
type MiddleMode int

const (
	ModeAgent MiddleMode = iota
	ModePreview
)

// AgentMode determines which agent backend is used.
type AgentMode int

const (
	AgentModeTeam AgentMode = iota
	AgentModeCopilot
)

// ThinkLevel controls the UI thinking level indicator.
type ThinkLevel int

const (
	ThinkLow ThinkLevel = iota
	ThinkMedium
	ThinkHigh
)

type paletteStage int

const (
	paletteStageSuggested paletteStage = iota
	paletteStageModels
	paletteStageAgents
	paletteStageSessions
)

type paletteAction int

const (
	paletteActionOpenModels paletteAction = iota
	paletteActionOpenAgents
	paletteActionOpenSessions
	paletteActionNewSession
	paletteActionSelectModel
	paletteActionSelectAgent
	paletteActionSelectSession
)

type paletteItem struct {
	Label  string
	Desc   string
	Action paletteAction
	Value  string
}

// App is the main TUI model that orchestrates the 3-panel layout.
type App struct {
	// Dependencies
	db     *db.DB
	logger *zap.Logger
	cfg    *config.Config

	// Layout
	width         int
	height        int
	historyHeight int
	middleHeight  int
	menuHeight    int
	inputHeight   int
	statusHeight  int
	ready         bool

	// Panel Y positions (rendered, relative to container content area)
	historyYStart int
	historyYEnd   int
	contentYStart int
	contentYEnd   int
	inputYStart   int
	inputYEnd     int

	// State
	focus      Focus
	middleMode MiddleMode
	agentMode  AgentMode
	thinkLevel ThinkLevel
	statusMsg  string

	// Team selection
	teams           []config.AgentTeamProfile
	activeTeamName  string
	activeTeamRoles []string

	// Sessions
	sessions        []db.AgentSession
	activeSessionID string
	responseBuffer  strings.Builder

	// Command palette
	paletteOpen  bool
	paletteStage paletteStage
	paletteIndex int

	// Sub-models
	history HistoryModel
	preview PreviewModel
	agent   AgentModel
	input   InputModel
	keys    KeyMap
}

// Messages.
type commandsLoadedMsg struct {
	commands []db.Command
}

type commandsErrorMsg struct {
	err error
}

type commandDeletedMsg struct {
	id string
}

type outputLoadedMsg struct {
	commandID string
	content   string
}

type agentChunkMsg struct {
	chunk  string
	stream <-chan string
}

type agentDoneMsg struct{}

type agentErrorMsg struct {
	err error
}

type sessionsLoadedMsg struct {
	sessions []db.AgentSession
}

type sessionsErrorMsg struct {
	err error
}

type sessionMessagesErrorMsg struct {
	err error
}

type sessionCreatedMsg struct {
	session db.AgentSession
}

type sessionMessagesLoadedMsg struct {
	sessionID string
	messages  []db.AgentMessage
}

type favoriteToggledMsg struct {
	id string
}

// New creates a new App model.
func New(database *db.DB, cfg *config.Config, logger *zap.Logger) App {
	keys := DefaultKeyMap()
	teams, activeName, activeRoles := resolveTeams(cfg.Agent)
	return App{
		db:              database,
		cfg:             cfg,
		logger:          logger,
		focus:           FocusInput, // Start with input focused (standard TUI pattern)
		middleMode:      ModeAgent,  // Default to Agent view
		agentMode:       AgentModeTeam,
		thinkLevel:      ThinkMedium,
		history:         NewHistoryModel(keys),
		preview:         NewPreviewModel(keys),
		agent:           NewAgentModel(keys),
		input:           NewInputModel(),
		keys:            keys,
		teams:           teams,
		activeTeamName:  activeName,
		activeTeamRoles: activeRoles,
	}
}

// Init loads the initial data asynchronously.
func (a App) Init() tea.Cmd {
	return tea.Batch(
		loadCommandsCmd(a.db),
		loadSessionsCmd(a.db),
		a.input.Focus(),
	)
}

// Update handles all messages.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		a.layoutPanels()
		return a, nil

	case commandsLoadedMsg:
		a.history.SetCommands(msg.commands)
		return a, nil

	case sessionsLoadedMsg:
		a.sessions = msg.sessions
		if a.activeSessionID == "" {
			if len(a.sessions) == 0 {
				return a, createSessionCmd(a.db)
			}
			a.activeSessionID = a.sessions[0].ID
			return a, loadSessionMessagesCmd(a.db, a.activeSessionID)
		}
		return a, nil

	case sessionsErrorMsg:
		if a.logger != nil {
			a.logger.Warn("failed to load sessions", zap.Error(msg.err))
		}
		a.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		return a, nil

	case sessionCreatedMsg:
		a.sessions = append([]db.AgentSession{msg.session}, a.sessions...)
		a.activeSessionID = msg.session.ID
		a.agent.SetMessages(nil)
		return a, nil

	case sessionMessagesLoadedMsg:
		if msg.sessionID != a.activeSessionID {
			return a, nil
		}
		a.agent.SetMessages(formatSessionMessages(msg.messages))
		return a, nil

	case sessionMessagesErrorMsg:
		if a.logger != nil {
			a.logger.Warn("failed to load session messages", zap.Error(msg.err))
		}
		a.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		return a, nil

	case commandsErrorMsg:
		if a.logger != nil {
			a.logger.Warn("failed to load commands", zap.Error(msg.err))
		}
		a.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		return a, nil

	case commandDeletedMsg:
		a.history.RemoveCommand(msg.id)
		a.statusMsg = "Command deleted"
		return a, nil

	case outputLoadedMsg:
		// Only apply output if it's for the currently previewed command (prevents stale data)
		if a.preview.CommandID() == msg.commandID {
			a.preview.SetContent(msg.content)
		}
		return a, nil

	case favoriteToggledMsg:
		// Reload commands to reflect the toggle
		return a, loadCommandsCmd(a.db)

	case SlashCommandResult:
		return a.handleSlashResult(msg)

	case agentChunkMsg:
		if msg.chunk != "" {
			a.agent.AppendToLast(msg.chunk)
			a.responseBuffer.WriteString(msg.chunk)
		}
		return a, readAgentChunkCmd(msg.stream)

	case agentDoneMsg:
		if a.activeSessionID == "" {
			return a, nil
		}
		response := strings.TrimSpace(a.responseBuffer.String())
		a.responseBuffer.Reset()
		if response == "" {
			return a, nil
		}
		return a, createMessageCmd(a.db, a.activeSessionID, "assistant", response)

	case agentErrorMsg:
		a.agent.AddMessage(fmt.Sprintf("Error: %v", msg.err))
		return a, nil

	case tea.KeyMsg:
		if a.paletteOpen {
			switch {
			case msg.Type == tea.KeyEsc:
				a.closePalette()
				return a, nil
			case key.Matches(msg, a.keys.Palette):
				a.closePalette()
				return a, nil
			case msg.Type == tea.KeyUp:
				a.movePaletteSelection(-1)
				return a, nil
			case msg.Type == tea.KeyDown:
				a.movePaletteSelection(1)
				return a, nil
			case msg.Type == tea.KeyEnter:
				return a.handlePaletteSelect()
			}
			return a, nil
		}

		// Global Keybindings
		if key.Matches(msg, a.keys.ForceQuit) {
			return a, nil
		}
		if key.Matches(msg, a.keys.Palette) {
			a.openPalette()
			return a, nil
		}
		if key.Matches(msg, a.keys.Variants) {
			a.cycleThinkLevel()
			return a, nil
		}

		// Tab switching (Focus Cycle)
		// Input -> History -> Content -> Input
		if key.Matches(msg, a.keys.NextTab) {
			a.cycleFocus(true)
			return a, nil
		}
		if key.Matches(msg, a.keys.PrevTab) {
			a.cycleFocus(false)
			return a, nil
		}

		// Handle keys based on focus
		switch a.focus {
		case FocusInput:
			model, cmd := a.handleInputKey(msg)
			app := model.(App)
			app.layoutPanels()
			return app, cmd
		case FocusHistory:
			return a.handleHistoryKey(msg)
		case FocusContent:
			return a.handleContentKey(msg)
		}

	case tea.MouseMsg:
		return a, nil
	}

	// Forward non-key, non-mouse messages (e.g. blink, tick) to sub-models
	var cmd tea.Cmd

	// Always update input for blink (even if blurred)
	a.input, cmd = a.input.Update(msg)
	cmds = append(cmds, cmd)

	// Update active panels (non-mouse messages only — mouse is handled above)
	a.history, cmd = a.history.Update(msg)
	cmds = append(cmds, cmd)

	if a.middleMode == ModePreview {
		a.preview, cmd = a.preview.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		a.agent, cmd = a.agent.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

func (a *App) cycleFocus(forward bool) {
	if forward {
		a.focus++
		if a.focus > FocusInput {
			a.focus = FocusHistory
		}
	} else {
		a.focus--
		if a.focus < FocusHistory {
			a.focus = FocusInput
		}
	}
	a.updateFocusState()
}

func (a *App) updateFocusState() {
	if a.focus == FocusInput {
		a.input.Focus()
	} else {
		a.input.Blur()
	}
}

// handleInputKey processes key events when input is focused.
func (a App) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if a.input.SelectSlashSuggestion() {
			a.layoutPanels()
			return a, nil
		}
		return a.submitInput()
	case tea.KeyEsc:
		// Esc in input -> Go to History
		a.focus = FocusHistory
		a.updateFocusState()
		return a, nil
	case tea.KeyUp, tea.KeyDown:
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}

	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// handleHistoryKey processes key events when history is focused.
func (a App) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Quit):
		return a, nil

	case key.Matches(msg, a.keys.Enter):
		// Enter on history -> Load output into Middle Panel and Switch to Preview Mode
		return a.previewSelected()

	case key.Matches(msg, a.keys.Delete):
		return a.deleteSelected()

	case key.Matches(msg, a.keys.Favorite):
		return a.toggleFavorite()

	case key.Matches(msg, a.keys.Cite):
		a.history.ToggleCited()
		a.layoutPanels() // recalculate: citation badge changes input height
		return a, nil
	}

	var cmd tea.Cmd
	a.history, cmd = a.history.Update(msg)
	return a, cmd
}

// handleContentKey processes key events when content (middle) is focused.
func (a App) handleContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Quit):
		return a, nil
	case key.Matches(msg, a.keys.Back):
		// Back -> Focus History
		a.focus = FocusHistory
		a.updateFocusState()
		return a, nil
	}

	var cmd tea.Cmd
	if a.middleMode == ModePreview {
		a.preview, cmd = a.preview.Update(msg)
	} else {
		a.agent, cmd = a.agent.Update(msg)
	}
	return a, cmd
}

// submitInput handles Enter press on the input bar.
func (a App) submitInput() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(a.input.Value())
	if val == "" {
		return a, nil
	}

	// Handle exit command (case-insensitive)
	if strings.EqualFold(val, "exit") {
		return a, tea.Quit
	}

	// Check for slash command
	if cmd := a.input.ParseSlashCommand(); cmd != nil {
		a.input.Reset()
		return a, executeSlashCommand(cmd, a.db, &a.cfg.LLM)
	}

	// Regular text input -> Send to Agent
	a.middleMode = ModeAgent
	a.agent.AddMessage(fmt.Sprintf("> %s", val))
	a.agent.AddMessage("")
	a.responseBuffer.Reset()

	if a.activeSessionID == "" {
		newSession, err := createSession(a.db)
		if err != nil {
			a.agent.AppendToLast(fmt.Sprintf("Error: %v", err))
			a.input.Reset()
			return a, nil
		}
		a.activeSessionID = newSession.ID
		a.sessions = append([]db.AgentSession{newSession}, a.sessions...)
	}

	if err := a.db.CreateAgentMessage(&db.AgentMessage{
		ID:        generateID(),
		SessionID: a.activeSessionID,
		Role:      "user",
		Content:   val,
		CreatedAt: time.Now().UnixNano(),
	}); err != nil {
		a.agent.AppendToLast(fmt.Sprintf("Error: %v", err))
		a.input.Reset()
		return a, nil
	}

	var (
		stream <-chan string
		err    error
	)
	if a.agentMode == AgentModeCopilot {
		stream, err = a.runCopilotQuery(val)
	} else {
		stream, err = a.runTeamQuery(val)
	}
	if err != nil {
		a.agent.AppendToLast(fmt.Sprintf("Error: %v", err))
		a.input.Reset()
		return a, nil
	}

	a.input.Reset()
	return a, readAgentChunkCmd(stream)
}

func (a *App) runTeamQuery(query string) (<-chan string, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	cfg := a.teamConfig()
	if err := team.EnsureDefaultRoles(cfg.Agent); err != nil {
		return nil, err
	}
	agentCfg, err := agent.NewAgentConfigFromConfig(&a.cfg.LLM)
	if err != nil {
		return nil, fmt.Errorf("LLM not configured: %w", err)
	}
	_ = agentCfg

	teamRunner, err := team.NewTeamRunner(context.Background(), cfg, a.db, a.logger)
	if err != nil {
		return nil, err
	}

	return teamRunner.Run(context.Background(), query, nil)
}

func (a *App) runCopilotQuery(query string) (<-chan string, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentCfg, err := agent.NewAgentConfigFromConfig(&a.cfg.LLM)
	if err != nil {
		return nil, fmt.Errorf("LLM not configured: %w", err)
	}
	model, err := agent.NewModel(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}
	tools := agent.CreateTools(a.db, a.cfg.Agent.RequireApproval)
	reactRunner, err := agent.NewReactRunner(context.Background(), model, tools, a.db, a.logger)
	if err != nil {
		return nil, fmt.Errorf("create react runner: %w", err)
	}

	return reactRunner.Run(context.Background(), query, nil)
}

func (a App) teamConfig() *config.Config {
	if len(a.activeTeamRoles) == 0 {
		return a.cfg
	}
	clone := *a.cfg
	clone.Agent = a.cfg.Agent
	clone.Agent.Roles = append([]string{}, a.activeTeamRoles...)
	return &clone
}

func readAgentChunkCmd(stream <-chan string) tea.Cmd {
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		chunk, ok := <-stream
		if !ok {
			return agentDoneMsg{}
		}
		return agentChunkMsg{chunk: chunk, stream: stream}
	}
}

// handleSlashResult processes the result of a slash command.
func (a App) handleSlashResult(result SlashCommandResult) (tea.Model, tea.Cmd) {
	if result.Quit {
		return a, tea.Quit
	}

	if result.Clear {
		a.statusMsg = ""
		a.agent.messages = nil
		a.agent.refreshContent()
		return a, nil
	}

	if result.SwitchFocus != nil {
		a.focus = *result.SwitchFocus
	}
	if result.SwitchMode != nil {
		a.middleMode = *result.SwitchMode
	}
	if result.SwitchAgentMode != nil {
		a.agentMode = *result.SwitchAgentMode
	}

	if result.Output != "" {
		a.middleMode = ModeAgent
		a.agent.AddMessage(result.Output)
	}

	a.updateFocusState()
	return a, nil
}

// previewSelected loads command output into the preview model (Middle Panel).
func (a App) previewSelected() (tea.Model, tea.Cmd) {
	cmd := a.history.SelectedCommand()
	if cmd == nil {
		return a, nil
	}

	a.middleMode = ModePreview
	a.preview.SetCommand(cmd)

	// Check if this is an interactive/TUI command whose output would corrupt display
	if isInteractiveCommand(cmd.Command) {
		a.preview.SetContent("(Interactive command — preview not available)")
		return a, nil
	}

	return a, loadOutputCmd(a.db, cmd.ID)
}

// isInteractiveCommand checks if a command is an interactive/TUI program
// whose output contains terminal control sequences that would corrupt the display.
func isInteractiveCommand(cmdStr string) bool {
	// Extract base command name (first word, strip path)
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return false
	}

	parts := strings.Fields(cmdStr)
	base := parts[0]

	// Strip path prefix (e.g., /usr/bin/vim -> vim)
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	if idx := strings.LastIndex(base, "\\"); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.ToLower(base)

	interactiveCommands := map[string]bool{
		"vim": true, "nvim": true, "vi": true, "nano": true, "emacs": true,
		"htop": true, "top": true, "btop": true, "gtop": true, "atop": true,
		"less": true, "more": true, "man": true, "most": true,
		"tmux": true, "screen": true,
		"tui": true, "mc": true, "ranger": true, "nnn": true, "lf": true,
		"fzf": true, "sk": true,
		"watch": true, "dialog": true, "whiptail": true,
		"mutt": true, "neomutt": true, "alpine": true,
		"weechat": true, "irssi": true,
		"ncdu": true, "tig": true, "lazygit": true, "lazydocker": true,
		"termia": true,
	}

	return interactiveCommands[base]
}

// deleteSelected deletes the currently selected command.
func (a App) deleteSelected() (tea.Model, tea.Cmd) {
	cmd := a.history.SelectedCommand()
	if cmd == nil {
		return a, nil
	}

	id := cmd.ID
	return a, func() tea.Msg {
		err := a.db.DeleteCommand(id)
		if err != nil {
			return commandsErrorMsg{err: err}
		}
		return commandDeletedMsg{id: id}
	}
}

// toggleFavorite toggles favorite on the selected command.
func (a App) toggleFavorite() (tea.Model, tea.Cmd) {
	cmd := a.history.SelectedCommand()
	if cmd == nil {
		return a, nil
	}

	id := cmd.ID
	return a, func() tea.Msg {
		err := a.db.ToggleFavorite(id)
		if err != nil {
			return commandsErrorMsg{err: err}
		}
		return favoriteToggledMsg{id: id}
	}
}

// View renders the complete TUI.
func (a App) View() string {
	if !a.ready {
		return loadingStyle.Render("  Starting Termia...")
	}

	// Container frame dimensions (border + padding)
	containerFW, containerFH := containerStyle.GetFrameSize()
	innerW := a.width - containerFW
	innerH := a.height - containerFH
	if innerW < 20 {
		innerW = 20
	}
	if innerH < 6 {
		innerH = 6
	}

	// Panel frame dimensions (border + padding)
	panelFW, _ := historyPaneStyle.GetFrameSize()
	panelContentWidth := innerW - panelFW
	if panelContentWidth < 10 {
		panelContentWidth = 10
	}

	// Render components
	history := a.renderHistory(panelContentWidth)
	content := a.renderContent(panelContentWidth)
	inputSection := a.renderInput(panelContentWidth)
	status := a.renderStatusBar(panelContentWidth)

	// Compose layout: history + content + input
	body := lipgloss.JoinVertical(lipgloss.Left, history, content, inputSection, status)

	// Container uses Height/Width to ensure minimum size (pads if body is shorter).
	// Do NOT use MaxHeight/MaxWidth — they truncate AFTER borders, clipping the bottom border.
	return containerStyle.
		Width(innerW).
		Height(innerH).
		Render(body)
}

// renderHistory renders the history panel with border based on focus.
func (a App) renderHistory(contentWidth int) string {
	// Clamp inner content to exact height BEFORE border is applied.
	// Never use MaxHeight on bordered styles — it clips the border itself.
	clamp := lipgloss.NewStyle().Width(contentWidth).Height(a.historyHeight).MaxHeight(a.historyHeight)
	inner := clamp.Render(a.history.View())

	// Width() on a bordered+padded style sets the area INCLUDING padding,
	// so text area = Width - padding. To get text area = contentWidth,
	// pass contentWidth + horizontal padding.
	pw := historyPaneStyle.GetHorizontalPadding()
	style := historyPaneStyle.Width(contentWidth + pw)
	if a.focus == FocusHistory {
		style = focusedHistoryPaneStyle.Width(contentWidth + pw)
	}
	return style.Render(inner)
}

// renderContent renders the content panel (agent or preview) with border based on focus.
// If the slash menu is active, it is overlaid onto the bottom of the content area
// BEFORE the panel border is applied, avoiding ANSI complexity.
func (a App) renderContent(contentWidth int) string {
	var content string
	if a.middleMode == ModePreview {
		content = a.preview.View()
	} else {
		content = a.agent.View()
	}

	// Clamp inner content to exact height BEFORE border is applied.
	// Never use MaxHeight on bordered styles — it clips the border itself.
	clamp := lipgloss.NewStyle().Width(contentWidth).Height(a.middleHeight).MaxHeight(a.middleHeight)
	inner := clamp.Render(content)

	// Width() on a bordered+padded style sets the area INCLUDING padding,
	// so text area = Width - padding. To get text area = contentWidth,
	// pass contentWidth + horizontal padding.
	pw := contentPaneStyle.GetHorizontalPadding()
	style := contentPaneStyle.Width(contentWidth + pw)
	if a.focus == FocusContent {
		style = focusedContentPaneStyle.Width(contentWidth + pw)
	}
	panel := style.Render(inner)

	// Overlay the slash menu onto the full panel so it aligns with input borders.
	if a.menuHeight > 0 && !a.paletteOpen {
		totalWidth := lipgloss.Width(panel)
		totalHeight := lipgloss.Height(panel)
		menuFW, _ := slashMenuStyle.GetFrameSize()
		menuPW := slashMenuStyle.GetHorizontalPadding()
		borderW := menuFW - menuPW
		menuWidth := totalWidth - borderW
		if menuWidth < 10 {
			menuWidth = 10
		}
		menuContentW := menuWidth - menuPW
		if menuContentW < 10 {
			menuContentW = 10
		}
		menuContent := RenderSlashMenu(a.input, menuContentW)
		if menuContent != "" {
			menuStr := slashMenuStyle.Width(menuWidth).Render(menuContent)
			menuStr = trimTrailingBlankLines(menuStr)
			panel = overlayContent(panel, menuStr, totalWidth, totalHeight)
		}
	}

	if a.paletteOpen {
		totalWidth := lipgloss.Width(panel)
		totalHeight := lipgloss.Height(panel)
		palette := a.renderCommandPalette(totalWidth)
		panel = overlayContentCentered(panel, palette, totalWidth, totalHeight)
	}

	return panel
}

// renderInput renders the input panel with border based on focus.
// Citation badge is rendered INSIDE the panel border to prevent clipping.
func (a App) renderInput(contentWidth int) string {
	// Build inner content: input view + always-on status line
	inputView := RenderInputSection(a.input)

	// Status line: agent label + model + thinking level + citations
	citedCount := a.history.CitedCount()
	status := a.renderAgentStatusLine()
	badge := citedBadgeStyle.Render(fmt.Sprintf("📎 %d commands referenced", citedCount))
	inner := inputView + "\n" + strings.TrimSpace(status+" "+badge)

	// Clamp inner content to exact height BEFORE border is applied.
	clamp := lipgloss.NewStyle().Width(contentWidth).Height(a.inputHeight).MaxHeight(a.inputHeight)
	inner = clamp.Render(inner)

	// Width() on a bordered+padded style sets the area INCLUDING padding,
	// so text area = Width - padding. To get text area = contentWidth,
	// pass contentWidth + horizontal padding.
	pw := inputBarStyle.GetHorizontalPadding()
	style := inputBarStyle.Width(contentWidth + pw)
	if a.focus == FocusInput {
		style = focusedInputBarStyle.Width(contentWidth + pw)
	}
	return style.Render(inner)
}

func (a App) renderAgentModeBadge() string {
	switch a.agentMode {
	case AgentModeCopilot:
		return copilotModeStyle.Render("Copilt")
	default:
		return teamModeStyle.Render("Team")
	}
}

func (a App) renderAgentStatusLine() string {
	agentLabel := a.renderAgentLabel()
	modelLabel := a.currentModelLabel()
	thinkLabel := a.thinkLevelLabel()
	parts := []string{agentLabel}
	if modelLabel != "" {
		parts = append(parts, metaStyle.Render("Model: "+modelLabel))
	}
	if thinkLabel != "" {
		parts = append(parts, metaStyle.Render("Think: "+thinkLabel))
	}
	return strings.Join(parts, "  ")
}

func (a App) renderAgentLabel() string {
	if a.agentMode == AgentModeCopilot {
		return copilotModeStyle.Render("Copilt")
	}
	name := strings.TrimSpace(a.activeTeamName)
	if name == "" {
		return teamModeStyle.Render("Team")
	}
	return teamModeStyle.Render(fmt.Sprintf("%s(Team)", name))
}

func (a App) currentModelLabel() string {
	provider := strings.ToLower(strings.TrimSpace(a.cfg.LLM.DefaultProvider))
	switch provider {
	case "openai":
		return a.cfg.LLM.OpenAI.Model
	case "anthropic":
		return a.cfg.LLM.Anthropic.Model
	case "deepseek":
		return a.cfg.LLM.DeepSeek.Model
	case "ollama":
		return a.cfg.LLM.Ollama.Model
	default:
		return ""
	}
}

func (a App) thinkLevelLabel() string {
	switch a.thinkLevel {
	case ThinkLow:
		return "Low"
	case ThinkHigh:
		return "High"
	default:
		return "Medium"
	}
}

// renderStatusBar renders the bottom help/status bar.
func trimTrailingBlankLines(content string) string {
	lines := strings.Split(content, "\n")
	last := len(lines) - 1
	for last >= 0 {
		if strings.TrimSpace(lines[last]) != "" {
			break
		}
		last--
	}
	if last < 0 {
		return ""
	}
	return strings.Join(lines[:last+1], "\n")
}

func (a App) renderStatusBar(contentWidth int) string {
	left := ""
	if a.statusMsg != "" {
		left = a.statusMsg
	}
	right := a.renderStatusHints()
	line := buildStatusLine(left, right, contentWidth)
	pw := statusBarStyle.GetHorizontalPadding()
	return statusBarStyle.Width(contentWidth + pw).Render(line)
}

func (a App) renderStatusHints() string {
	return fmt.Sprintf("Ctrl+P %s | Ctrl+T %s | Tab %s",
		metaStyle.Render("command"),
		metaStyle.Render("variants"),
		metaStyle.Render("windows"),
	)
}

func buildStatusLine(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	left = strings.TrimSpace(left)
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)

	if rightW >= width {
		return truncate.String(right, uint(width))
	}

	if leftW > 0 && leftW+1+rightW > width {
		maxLeft := width - rightW - 1
		if maxLeft < 0 {
			maxLeft = 0
		}
		left = truncate.String(left, uint(maxLeft))
		leftW = lipgloss.Width(left)
	}

	if leftW == 0 {
		spaces := width - rightW
		if spaces < 0 {
			spaces = 0
		}
		return strings.Repeat(" ", spaces) + right
	}

	spaces := width - leftW - rightW
	if spaces < 1 {
		spaces = 1
	}
	return left + strings.Repeat(" ", spaces) + right
}

func overlayStatusLine(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	left = padToWidth(left, width)
	right = padToWidth(right, width)
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	if len(rightRunes) > len(leftRunes) {
		return right
	}
	for i := 0; i < len(rightRunes); i++ {
		idx := len(leftRunes) - len(rightRunes) + i
		if rightRunes[i] != ' ' {
			leftRunes[idx] = rightRunes[i]
		}
	}
	return string(leftRunes)
}

func (a *App) openPalette() {
	a.paletteOpen = true
	a.paletteStage = paletteStageSuggested
	a.paletteIndex = 0
}

func (a *App) closePalette() {
	a.paletteOpen = false
	a.paletteStage = paletteStageSuggested
	a.paletteIndex = 0
}

func (a *App) movePaletteSelection(delta int) {
	items := a.paletteItems()
	if len(items) == 0 {
		a.paletteIndex = 0
		return
	}
	a.paletteIndex += delta
	if a.paletteIndex < 0 {
		a.paletteIndex = len(items) - 1
	} else if a.paletteIndex >= len(items) {
		a.paletteIndex = 0
	}
}

func (a App) handlePaletteSelect() (tea.Model, tea.Cmd) {
	items := a.paletteItems()
	if len(items) == 0 {
		return a, nil
	}
	if a.paletteIndex < 0 || a.paletteIndex >= len(items) {
		a.paletteIndex = 0
	}
	item := items[a.paletteIndex]
	switch item.Action {
	case paletteActionOpenModels:
		a.paletteStage = paletteStageModels
		a.paletteIndex = 0
		return a, nil
	case paletteActionOpenAgents:
		a.paletteStage = paletteStageAgents
		a.paletteIndex = 0
		return a, nil
	case paletteActionOpenSessions:
		a.paletteStage = paletteStageSessions
		a.paletteIndex = 0
		return a, nil
	case paletteActionNewSession:
		a.closePalette()
		return a, createSessionCmd(a.db)
	case paletteActionSelectModel:
		a.cfg.LLM.DefaultProvider = item.Value
		a.statusMsg = fmt.Sprintf("Model switched to %s.", item.Label)
		a.closePalette()
		return a, nil
	case paletteActionSelectAgent:
		if item.Value == "copilt" {
			a.agentMode = AgentModeCopilot
			a.statusMsg = "Agent mode set to copilt."
			a.closePalette()
			return a, nil
		}
		team, ok := a.teamByName(item.Value)
		if ok {
			a.agentMode = AgentModeTeam
			a.activeTeamName = team.Name
			a.activeTeamRoles = append([]string{}, team.Roles...)
			a.statusMsg = fmt.Sprintf("Agent mode set to %s.", team.Name)
		}
		a.closePalette()
		return a, nil
	case paletteActionSelectSession:
		a.activeSessionID = item.Value
		a.closePalette()
		return a, loadSessionMessagesCmd(a.db, item.Value)
	default:
		return a, nil
	}
}

func (a *App) cycleThinkLevel() {
	switch a.thinkLevel {
	case ThinkLow:
		a.thinkLevel = ThinkMedium
	case ThinkMedium:
		a.thinkLevel = ThinkHigh
	default:
		a.thinkLevel = ThinkLow
	}
}

func (a App) renderCommandPalette(totalWidth int) string {
	items := a.paletteItems()
	if totalWidth <= 0 {
		return ""
	}
	paletteWidth := totalWidth - 8
	if paletteWidth > 60 {
		paletteWidth = 60
	}
	if paletteWidth < 30 {
		paletteWidth = totalWidth - 4
	}
	if paletteWidth < 20 {
		paletteWidth = totalWidth
	}
	frameW, _ := commandPaletteStyle.GetFrameSize()
	contentWidth := paletteWidth - frameW
	if contentWidth < 10 {
		contentWidth = 10
	}
	var lines []string
	lines = append(lines, a.renderPaletteHeader(contentWidth))
	lines = append(lines, "")
	lines = append(lines, panelTitleStyle.Render(a.paletteTitle()))
	for i, item := range items {
		line := a.formatPaletteLine(item.Label, item.Desc, contentWidth)
		style := normalRowStyle
		if i == a.paletteIndex {
			style = selectedSlashRowStyle
		}
		lines = append(lines, style.Width(contentWidth).Inline(true).Render(line))
	}
	if len(items) == 0 {
		lines = append(lines, emptyStyle.Render("No items"))
	}
	content := strings.Join(lines, "\n")
	panel := commandPaletteStyle.Width(paletteWidth).Render(content)
	padding := (totalWidth - paletteWidth) / 2
	return padLeftLines(panel, padding)
}

func (a App) renderPaletteHeader(width int) string {
	left := "Commands"
	right := metaStyle.Render("Esc")
	leftLine := lipgloss.PlaceHorizontal(width, lipgloss.Left, left)
	rightLine := lipgloss.PlaceHorizontal(width, lipgloss.Right, right)
	return overlayStatusLine(leftLine, rightLine, width)
}

func (a App) paletteTitle() string {
	switch a.paletteStage {
	case paletteStageModels:
		return "Switch Model"
	case paletteStageAgents:
		return "Switch Agent"
	case paletteStageSessions:
		return "Switch Session"
	default:
		return "Suggested"
	}
}

func (a App) paletteItems() []paletteItem {
	switch a.paletteStage {
	case paletteStageModels:
		return a.modelPaletteItems()
	case paletteStageAgents:
		return a.agentPaletteItems()
	case paletteStageSessions:
		return a.sessionPaletteItems()
	default:
		return []paletteItem{
			{Label: "Switch Model", Action: paletteActionOpenModels},
			{Label: "Switch Agent", Action: paletteActionOpenAgents},
			{Label: "Switch Session", Action: paletteActionOpenSessions},
			{Label: "New Session", Action: paletteActionNewSession},
		}
	}
}

func (a App) modelPaletteItems() []paletteItem {
	providers := []struct {
		label string
		key   string
		model string
	}{
		{"OpenAI", "openai", a.cfg.LLM.OpenAI.Model},
		{"Anthropic", "anthropic", a.cfg.LLM.Anthropic.Model},
		{"DeepSeek", "deepseek", a.cfg.LLM.DeepSeek.Model},
		{"Ollama", "ollama", a.cfg.LLM.Ollama.Model},
	}
	items := make([]paletteItem, 0, len(providers))
	for _, p := range providers {
		model := p.model
		if strings.TrimSpace(model) == "" {
			model = "(not set)"
		}
		items = append(items, paletteItem{Label: p.label, Desc: model, Action: paletteActionSelectModel, Value: p.key})
	}
	return items
}

func (a App) agentPaletteItems() []paletteItem {
	items := []paletteItem{{Label: "Copilt", Action: paletteActionSelectAgent, Value: "copilt"}}
	for _, team := range a.teams {
		label := fmt.Sprintf("%s(Team)", team.Name)
		items = append(items, paletteItem{Label: label, Action: paletteActionSelectAgent, Value: team.Name})
	}
	return items
}

func (a App) sessionPaletteItems() []paletteItem {
	if len(a.sessions) == 0 {
		return []paletteItem{{Label: "No sessions", Action: paletteActionOpenSessions}}
	}
	items := make([]paletteItem, 0, len(a.sessions))
	for _, s := range a.sessions {
		desc := formatRelativeTime(s.UpdatedAt)
		items = append(items, paletteItem{Label: s.Name, Desc: desc, Action: paletteActionSelectSession, Value: s.ID})
	}
	return items
}

func (a App) teamByName(name string) (config.AgentTeamProfile, bool) {
	for _, team := range a.teams {
		if strings.EqualFold(team.Name, name) {
			return team, true
		}
	}
	return config.AgentTeamProfile{}, false
}

func (a App) formatPaletteLine(label, desc string, width int) string {
	line := label
	if strings.TrimSpace(desc) == "" {
		return line
	}
	leftWidth := lipgloss.Width(label)
	maxDesc := width - leftWidth - 1
	if maxDesc < 0 {
		maxDesc = 0
	}
	if lipgloss.Width(desc) > maxDesc {
		desc = truncateToWidth(desc, maxDesc)
	}
	spaces := width - leftWidth - lipgloss.Width(desc)
	if spaces < 1 {
		spaces = 1
	}
	return label + strings.Repeat(" ", spaces) + metaStyle.Render(desc)
}

func padLeftLines(content string, padding int) string {
	if padding <= 0 {
		return content
	}
	pad := strings.Repeat(" ", padding)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

// layoutPanels recalculates panel dimensions on resize.
//
// Layout strategy:
//   - Container has a RoundedBorder -> frame adds to rendered size.
//   - Each panel (history, content, input) has RoundedBorder + Padding(0,1).
//   - Slash menu floats as overlay on top of the content panel (does NOT affect height budget).
//   - Citation badge renders as 1 extra line below input border when citations exist.
//
// All frame sizes are computed via GetFrameSize() instead of hardcoded values.
func (a *App) layoutPanels() {
	if !a.ready {
		return
	}

	// Container frame: border adds to rendered size
	containerFW, containerFH := containerStyle.GetFrameSize()
	innerW := a.width - containerFW
	if innerW < 20 {
		innerW = 20
	}
	totalInner := a.height - containerFH // available lines inside container

	// Panel frame: border + padding
	panelFW, panelFH := historyPaneStyle.GetFrameSize()
	panelContentWidth := innerW - panelFW
	if panelContentWidth < 10 {
		panelContentWidth = 10
	}

	// Track menu height for overlay rendering (does NOT affect layout budget)
	suggestions := a.input.SlashSuggestions()
	if len(suggestions) > 0 {
		a.menuHeight = len(suggestions)
	} else {
		a.menuHeight = 0
	}

	// Status bar occupies one line outside input panel
	a.statusHeight = 1

	// Fixed input rendered height: content + frame
	// inputHeight is dynamic based on wrapped input lines (plus a reserved status line)
	a.inputHeight = InputLineCount(a.input)
	if a.inputHeight < 1 {
		a.inputHeight = 1
	}
	// Reserve an extra status line (used for cited count + future hints)
	a.inputHeight += 1
	if a.inputHeight > maxInputLines+1 {
		a.inputHeight = maxInputLines + 1
	}
	inputRendered := a.inputHeight + panelFH

	// Available height for history + content
	available := totalInner - inputRendered - a.statusHeight
	if available < panelFH*2+2 { // minimum for 2 panels with 1 line each
		available = panelFH*2 + 2
	}

	// Split: history ~25%, content gets the rest
	historyRendered := available / 4
	if historyRendered < panelFH+1 { // minimum: 1 content line + frame
		historyRendered = panelFH + 1
	}
	contentRendered := available - historyRendered
	if contentRendered < panelFH+1 {
		contentRendered = panelFH + 1
	}

	// Content heights for .Height() calls (subtract frame)
	a.historyHeight = historyRendered - panelFH
	a.middleHeight = contentRendered - panelFH
	// a.inputHeight is already set above (1 or 2 depending on citations)

	// Clamp minimums
	if a.historyHeight < 1 {
		a.historyHeight = 1
	}
	if a.middleHeight < 1 {
		a.middleHeight = 1
	}
	if a.inputHeight < 1 {
		a.inputHeight = 1
	}

	// Update sub-model sizes (viewport content dimensions)
	a.history.SetSize(panelContentWidth, a.historyHeight)
	a.preview.SetSize(panelContentWidth, a.middleHeight)
	a.agent.SetSize(panelContentWidth, a.middleHeight)
	a.input.SetWidth(panelContentWidth)
	a.input.SetHeight(InputLineCount(a.input))

	// Compute panel Y boundaries (relative to terminal, including container border)
	// Container border top = containerFH/2 (split evenly top/bottom for rounded border = 1)
	y := 1 // start after container top border
	a.historyYStart = y
	a.historyYEnd = y + historyRendered - 1
	y += historyRendered

	a.contentYStart = y
	a.contentYEnd = y + contentRendered - 1
	y += contentRendered

	a.inputYStart = y
	a.inputYEnd = y + inputRendered - 1
}

func loadCommandsCmd(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		commands, err := database.ListRecentCommands(200)
		if err != nil {
			return commandsErrorMsg{err: err}
		}
		return commandsLoadedMsg{commands: commands}
	}
}

func loadSessionsCmd(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		sessions, err := database.ListAgentSessions(200)
		if err != nil {
			return sessionsErrorMsg{err: err}
		}
		return sessionsLoadedMsg{sessions: sessions}
	}
}

func loadSessionMessagesCmd(database *db.DB, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := database.ListAgentMessages(sessionID)
		if err != nil {
			return sessionMessagesErrorMsg{err: err}
		}
		return sessionMessagesLoadedMsg{sessionID: sessionID, messages: messages}
	}
}

func createSessionCmd(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		session, err := createSession(database)
		if err != nil {
			return sessionsErrorMsg{err: err}
		}
		return sessionCreatedMsg{session: session}
	}
}

func createMessageCmd(database *db.DB, sessionID, role, content string) tea.Cmd {
	return func() tea.Msg {
		msg := &db.AgentMessage{
			ID:        generateID(),
			SessionID: sessionID,
			Role:      role,
			Content:   content,
			CreatedAt: time.Now().UnixNano(),
		}
		if err := database.CreateAgentMessage(msg); err != nil {
			return agentErrorMsg{err: err}
		}
		return nil
	}
}

func createSession(database *db.DB) (db.AgentSession, error) {
	if database == nil {
		return db.AgentSession{}, fmt.Errorf("database is nil")
	}
	name := fmt.Sprintf("Session %s", time.Now().Format("2006-01-02 15:04"))
	session := db.AgentSession{
		ID:        generateID(),
		Name:      name,
		CreatedAt: time.Now().UnixNano(),
		UpdatedAt: time.Now().UnixNano(),
	}
	if err := database.CreateAgentSession(&session); err != nil {
		return db.AgentSession{}, err
	}
	return session, nil
}

func formatSessionMessages(messages []db.AgentMessage) []string {
	if len(messages) == 0 {
		return nil
	}
	output := make([]string, 0, len(messages))
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if msg.Role == "user" {
			output = append(output, fmt.Sprintf("> %s", content))
			continue
		}
		output = append(output, content)
	}
	return output
}

func generateID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// Run starts the TUI.
func Run(database *db.DB, cfg *config.Config, logger *zap.Logger) error {
	app := New(database, cfg, logger)
	program := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

func formatRelativeTime(ts int64) string {
	if ts == 0 {
		return ""
	}
	now := time.Now()
	timestamp := time.Unix(0, ts)
	delta := now.Sub(timestamp)
	if delta < 0 {
		delta = -delta
	}
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(delta.Hours()/24))
	default:
		return timestamp.Format("2006-01-02")
	}
}

func formatDuration(ms *int64) string {
	if ms == nil || *ms == 0 {
		return ""
	}
	dur := time.Duration(*ms) * time.Millisecond
	if dur < time.Second {
		return fmt.Sprintf("%dms", dur.Milliseconds())
	}
	if dur < time.Minute {
		return fmt.Sprintf("%0.1fs", dur.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(dur.Minutes()), int(dur.Seconds())%60)
}
