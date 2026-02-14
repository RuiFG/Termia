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

const mouseMotionThrottle = 20 * time.Millisecond

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
	leftWidth     int
	rightWidth    int
	leftContentW  int
	rightContentW int
	twoColumn     bool
	modalWidth    int
	modalHeight   int
	modalXStart   int
	modalXEnd     int
	modalYStart   int
	modalYEnd     int
	modalContentX int
	modalContentY int
	modalContentW int
	modalContentH int
	ready         bool

	leftXStart    int
	leftXEnd      int
	rightXStart   int
	rightXEnd     int
	historyYStart int
	historyYEnd   int
	contentYStart int
	contentYEnd   int
	inputYStart   int
	inputYEnd     int

	// State
	focus            Focus
	middleMode       MiddleMode
	detailOpen       bool
	agentMode        AgentMode
	thinkLevel       ThinkLevel
	statusMsg        string
	contentSelection textSelection
	inputSelection   textSelection
	lastMouseMotion  time.Time

	// Team selection
	teams           []config.AgentTeamProfile
	activeTeamName  string
	activeTeamRoles []string

	// Sessions
	sessions        []db.AgentSession
	activeSessionID string
	responseBuffer  []string

	// Command palette
	paletteOpen  bool
	paletteStage paletteStage
	paletteIndex int
	paletteQuery string

	// Sub-models
	history HistoryModel
	preview PreviewModel
	detail  HistoryDetailModel
	agent   AgentModel
	modal   ModalModel
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

type commandExecutedMsg struct{}

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
		detail:          NewHistoryDetailModel(keys),
		agent:           NewAgentModel(keys),
		modal:           NewModalModel(),
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
		waitForCommandExecutedCmd(),
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

	case commandExecutedMsg:
		return a, tea.Batch(loadCommandsCmd(a.db), waitForCommandExecutedCmd())

	case commandDeletedMsg:
		a.history.RemoveCommand(msg.id)
		a.statusMsg = "Command deleted"
		return a, nil

	case outputLoadedMsg:
		if a.modal.IsOpen() && a.modal.CommandID() == msg.commandID {
			a.modal.SetContent(msg.content)
			return a, nil
		}
		if a.detailOpen && a.detail.CommandID() == msg.commandID {
			a.detail.SetContent(msg.content)
			return a, nil
		}
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
			a.responseBuffer = append(a.responseBuffer, msg.chunk)
		}
		return a, readAgentChunkCmd(msg.stream)

	case agentDoneMsg:
		if a.activeSessionID == "" {
			return a, nil
		}
		response := strings.TrimSpace(strings.Join(a.responseBuffer, ""))
		a.responseBuffer = nil
		if response == "" {
			return a, nil
		}
		return a, createMessageCmd(a.db, a.activeSessionID, "assistant", response)

	case agentErrorMsg:
		a.agent.AddMessage(fmt.Sprintf("Error: %v", msg.err))
		return a, nil

	case tea.KeyMsg:
		if a.modal.IsOpen() {
			return a.handleModalKey(msg)
		}
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
			case msg.Type == tea.KeyBackspace:
				if a.paletteQuery != "" {
					runes := []rune(a.paletteQuery)
					if len(runes) > 0 {
						a.paletteQuery = string(runes[:len(runes)-1])
					}
					a.paletteIndex = 0
				}
				return a, nil
			case msg.Type == tea.KeySpace:
				a.paletteQuery += " "
				a.paletteIndex = 0
				return a, nil
			case msg.Type == tea.KeyRunes:
				if len(msg.Runes) > 0 {
					a.paletteQuery += string(msg.Runes)
					a.paletteIndex = 0
				}
				return a, nil
			}
			return a, nil
		}

		if a.detailOpen && msg.Type == tea.KeyEsc {
			return a.closeDetail()
		}
		if a.detailOpen && msg.Type == tea.KeyCtrlC {
			text := a.detail.SelectedText()
			a.detail.ClearSelection()
			if text == "" {
				return a, nil
			}
			return a, copyToClipboardCmd(text)
		}
		if msg.Type == tea.KeyCtrlC {
			if a.contentSelection.HasSelection() {
				text := a.contentSelection.SelectedText()
				a.contentSelection.Clear()
				if text == "" {
					return a, nil
				}
				return a, copyToClipboardCmd(text)
			}
			if a.inputSelection.HasSelection() {
				text := a.inputSelection.SelectedText()
				a.inputSelection.Clear()
				if text == "" {
					return a, nil
				}
				return a, copyToClipboardCmd(text)
			}
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
		return a.handleMouse(msg)
	}

	// Forward non-key, non-mouse messages (e.g. blink, tick) to sub-models
	var cmd tea.Cmd

	// Always update input for blink (even if blurred)
	a.input, cmd = a.input.Update(msg)
	cmds = append(cmds, cmd)

	// Update active panels (non-mouse messages only — mouse is handled above)
	a.history, cmd = a.history.Update(msg)
	cmds = append(cmds, cmd)

	if a.detailOpen {
		a.detail, cmd = a.detail.Update(msg)
		cmds = append(cmds, cmd)
	} else if a.middleMode == ModePreview {
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
		return a.openDetailSelected()

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
		if a.detailOpen {
			return a.closeDetail()
		}
		a.focus = FocusHistory
		a.updateFocusState()
		return a, nil
	}

	var cmd tea.Cmd
	if a.detailOpen {
		a.detail, cmd = a.detail.Update(msg)
	} else if a.middleMode == ModePreview {
		a.preview, cmd = a.preview.Update(msg)
	} else {
		a.agent, cmd = a.agent.Update(msg)
	}
	return a, cmd
}

func (a App) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, a.keys.PageUp) {
		a.modal.PageScroll(-1)
		return a, nil
	}
	if key.Matches(msg, a.keys.PageDown) {
		a.modal.PageScroll(1)
		return a, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		a.modal.Close()
		return a, nil
	case tea.KeyCtrlC:
		text := a.modal.SelectedText()
		a.modal.ClearSelection()
		if text == "" {
			return a, nil
		}
		return a, copyToClipboardCmd(text)
	}

	a.modal.HandleKey(msg.Type)
	return a, nil
}

func (a App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	contentX := msg.X
	contentY := msg.Y
	if msg.Action == tea.MouseActionPress {
		a.lastMouseMotion = time.Time{}
	}
	if msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft {
		a.lastMouseMotion = time.Now()
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		if focus, ok := a.focusFromMouse(contentX, contentY); ok {
			a.focus = focus
			a.updateFocusState()
		}
	}
	if a.modal.IsOpen() {
		if msg.Button == tea.MouseButtonWheelUp {
			a.modal.Scroll(-3)
			return a, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			a.modal.Scroll(3)
			return a, nil
		}

		if msg.Button != tea.MouseButtonLeft {
			return a, nil
		}

		if contentX < a.modalContentX || contentX >= a.modalContentX+a.modalContentW {
			if msg.Action == tea.MouseActionRelease {
				a.modal.EndSelection()
			}
			return a, nil
		}
		if contentY < a.modalContentY || contentY >= a.modalContentY+a.modalContentH {
			if msg.Action == tea.MouseActionRelease {
				a.modal.EndSelection()
			}
			return a, nil
		}

		line := a.modal.ScrollOffset() + (contentY - a.modalContentY)
		col := contentX - a.modalContentX

		switch msg.Action {
		case tea.MouseActionPress:
			a.modal.BeginSelection(line, col)
		case tea.MouseActionMotion:
			a.modal.UpdateSelection(line, col)
		case tea.MouseActionRelease:
			a.modal.UpdateSelection(line, col)
			a.modal.EndSelection()
		}

		return a, nil
	}

	if handled, cmd := a.handleInputSelection(msg, contentX, contentY); handled {
		return a, cmd
	}

	if a.detailOpen {
		return a.handleDetailSelection(msg, contentX, contentY)
	}

	return a.handleContentSelection(msg, contentX, contentY)
}

func (a App) hasActiveDragSelection() bool {
	return a.contentSelection.dragging || a.inputSelection.dragging || a.modal.dragging || a.detail.dragging
}

func (a App) focusFromMouse(x, y int) (Focus, bool) {
	if a.rightWidth > 0 {
		if x >= a.rightXStart && x <= a.rightXEnd && y >= a.historyYStart && y <= a.historyYEnd {
			return FocusHistory, true
		}
	} else {
		if x >= a.leftXStart && x <= a.leftXEnd && y >= a.historyYStart && y <= a.historyYEnd {
			return FocusHistory, true
		}
	}

	if x >= a.leftXStart && x <= a.leftXEnd {
		if y >= a.contentYStart && y <= a.contentYEnd {
			return FocusContent, true
		}
		if y >= a.inputYStart && y <= a.inputYEnd {
			return FocusInput, true
		}
	}

	return FocusHistory, false
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
	a.responseBuffer = nil

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

func (a App) openModalSelected() (tea.Model, tea.Cmd) {
	cmd := a.history.SelectedCommand()
	if cmd == nil {
		return a, nil
	}
	header := formatCommandDetails(cmd)
	if isInteractiveCommand(cmd.Command) {
		a.modal.Open(cmd.ID)
		a.modal.SetHeader(header)
		a.modal.SetContent("(Interactive command — preview not available)")
		return a, nil
	}
	return a.openModalCommand(cmd, header)
}

func (a App) openModalCommand(cmd *db.Command, header string) (tea.Model, tea.Cmd) {
	a.modal.Open(cmd.ID)
	a.modal.SetHeader(header)
	a.modal.SetContent("")
	return a, loadOutputCmd(a.db, cmd.ID)
}

func (a App) openDetailSelected() (tea.Model, tea.Cmd) {
	cmd := a.history.SelectedCommand()
	if cmd == nil {
		return a, nil
	}
	if isInteractiveCommand(cmd.Command) {
		a.detailOpen = true
		a.detail.SetCommand(cmd)
		a.detail.SetContent("(Interactive command — preview not available)")
		return a, nil
	}
	return a.openDetailCommand(cmd)
}

func (a App) openDetailCommand(cmd *db.Command) (tea.Model, tea.Cmd) {
	a.detailOpen = true
	a.detail.SetCommand(cmd)
	a.detail.SetContent("")
	return a, loadOutputCmd(a.db, cmd.ID)
}

func (a App) closeDetail() (tea.Model, tea.Cmd) {
	a.detailOpen = false
	a.detail.ClearContent()
	a.focus = FocusHistory
	a.updateFocusState()
	return a, nil
}

func formatCommandDetails(cmd *db.Command) string {
	if cmd == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Command: ")
	b.WriteString(cmd.Command)
	b.WriteString("\nCwd: ")
	if strings.TrimSpace(cmd.Cwd) == "" {
		b.WriteString("-")
	} else {
		b.WriteString(cmd.Cwd)
	}
	b.WriteString("\nExit: ")
	if cmd.ExitCode == nil {
		b.WriteString("unknown")
	} else if *cmd.ExitCode == 0 {
		b.WriteString("success (0)")
	} else {
		b.WriteString(fmt.Sprintf("failed (%d)", *cmd.ExitCode))
	}
	b.WriteString("\nTime: ")
	if cmd.TsStart == 0 {
		b.WriteString("-")
	} else {
		b.WriteString(time.Unix(0, cmd.TsStart).Format("2006-01-02 15:04:05"))
	}
	return b.String()
}

func copyToClipboardCmd(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		_ = copyToClipboard(text)
		return nil
	}
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

	if a.modal.IsOpen() {
		return a.modal.View()
	}

	statusFW, _ := statusBarStyle.GetFrameSize()
	statusContentW := innerW - statusFW
	if statusContentW < 1 {
		statusContentW = 1
	}
	status := a.renderStatusBar(statusContentW)

	var body string
	if a.twoColumn {
		left := lipgloss.JoinVertical(
			lipgloss.Left,
			a.renderContent(a.leftContentW),
			a.renderInput(a.leftContentW),
		)
		right := a.renderHistory(a.rightContentW)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
		body = lipgloss.JoinVertical(lipgloss.Left, body, status)
	} else {
		history := a.renderHistory(a.leftContentW)
		content := a.renderContent(a.leftContentW)
		inputSection := a.renderInput(a.leftContentW)
		body = lipgloss.JoinVertical(lipgloss.Left, history, content, inputSection, status)
	}

	if a.paletteOpen {
		palette := a.renderCommandPalette(innerW)
		body = overlayContentCentered(body, palette, innerW, innerH)
	}

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
	if a.detailOpen {
		content = a.detail.View()
	} else if a.middleMode == ModePreview {
		content = a.preview.View()
	} else {
		content = a.agent.View()
	}
	if !a.detailOpen && a.contentSelection.HasSelection() {
		content = strings.Join(a.contentSelection.HighlightLines(contentWidth), "\n")
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

	return panel
}

func contentSelectionLines(content string, width, height int) []string {
	clampStyle := lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height)
	inner := clampStyle.Render(content)
	plain := stripANSICodes(inner)
	lines := strings.Split(plain, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func panelInnerOrigin(style lipgloss.Style, xStart, yStart int) (int, int) {
	return xStart + style.GetBorderLeftSize() + style.GetPaddingLeft(),
		yStart + style.GetBorderTopSize() + style.GetPaddingTop()
}

// renderInput renders the input panel with border based on focus.
// Citation badge is rendered INSIDE the panel border to prevent clipping.
func (a App) renderInput(contentWidth int) string {
	// Build inner content: input view + always-on status line
	inputView := RenderInputSection(a.input)

	// Status line: agent label + model + thinking level + citations
	citedCount := a.history.CitedCount()
	status := a.renderAgentStatusLine()
	label := "commands"
	if citedCount <= 1 {
		label = "command"
	}
	badge := citedBadgeStyle.Render(fmt.Sprintf("📎 %d %s referenced", citedCount, label))
	statusLine := buildStatusLine(status, badge, contentWidth)

	inputLines := strings.Split(inputView, "\n")
	if a.inputSelection.HasSelection() {
		inputLines = a.inputSelection.HighlightLines(contentWidth)
	}
	inputAreaLines := a.inputHeight - 1
	blankLines := inputAreaLines - len(inputLines)
	if blankLines < 1 {
		blankLines = 1
	}
	for i := 0; i < blankLines; i++ {
		inputLines = append(inputLines, "")
	}
	inputLines = append(inputLines, statusLine)
	inner := strings.Join(inputLines, "\n")

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
		parts = append(parts, a.thinkLevelStyle().Render(thinkLabel))
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

func (a App) thinkLevelStyle() lipgloss.Style {
	switch a.thinkLevel {
	case ThinkLow:
		return thinkLowStyle
	case ThinkHigh:
		return thinkHighStyle
	default:
		return thinkMediumStyle
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
	a.paletteQuery = ""
}

func (a *App) closePalette() {
	a.paletteOpen = false
	a.paletteStage = paletteStageSuggested
	a.paletteIndex = 0
	a.paletteQuery = ""
}

func (a *App) movePaletteSelection(delta int) {
	items := a.paletteVisibleItems()
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
	items := a.paletteVisibleItems()
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

func (a App) handleDetailSelection(msg tea.MouseMsg, contentX, contentY int) (tea.Model, tea.Cmd) {
	if contentX < a.leftXStart || contentX > a.leftXEnd {
		if msg.Action == tea.MouseActionRelease {
			a.detail.EndSelection()
		}
		return a, nil
	}
	if contentY < a.contentYStart || contentY > a.contentYEnd {
		if msg.Action == tea.MouseActionRelease {
			a.detail.EndSelection()
		}
		return a, nil
	}

	innerX, innerY := panelInnerOrigin(contentPaneStyle, a.leftXStart, a.contentYStart)
	innerW := a.leftContentW
	innerH := a.middleHeight

	if contentX < innerX || contentX >= innerX+innerW {
		if msg.Action == tea.MouseActionRelease {
			a.detail.EndSelection()
		}
		return a, nil
	}
	if contentY < innerY || contentY >= innerY+innerH {
		if msg.Action == tea.MouseActionRelease {
			a.detail.EndSelection()
		}
		return a, nil
	}

	if msg.Button == tea.MouseButtonWheelUp {
		a.detail.Scroll(-3)
		return a, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		a.detail.Scroll(3)
		return a, nil
	}

	if msg.Button != tea.MouseButtonLeft {
		return a, nil
	}

	headerHeight := a.detail.HeaderHeight()
	contentStartY := innerY + headerHeight
	contentEndY := contentStartY + a.detail.ContentHeight() - 1
	if contentY < contentStartY || contentY > contentEndY {
		if msg.Action == tea.MouseActionRelease {
			a.detail.EndSelection()
		}
		return a, nil
	}

	line := a.detail.ScrollOffset() + (contentY - contentStartY)
	col := contentX - innerX

	switch msg.Action {
	case tea.MouseActionPress:
		a.detail.BeginSelection(line, col)
	case tea.MouseActionMotion:
		a.detail.UpdateSelection(line, col)
	case tea.MouseActionRelease:
		a.detail.UpdateSelection(line, col)
		a.detail.EndSelection()
	}

	return a, nil
}

func (a App) handleContentSelection(msg tea.MouseMsg, contentX, contentY int) (tea.Model, tea.Cmd) {
	if contentX < a.leftXStart || contentX > a.leftXEnd {
		if msg.Action == tea.MouseActionRelease {
			a.contentSelection.EndSelection()
		}
		return a, nil
	}
	if contentY < a.contentYStart || contentY > a.contentYEnd {
		if msg.Action == tea.MouseActionRelease {
			a.contentSelection.EndSelection()
		}
		return a, nil
	}

	innerX, innerY := panelInnerOrigin(contentPaneStyle, a.leftXStart, a.contentYStart)
	innerW := a.leftContentW
	innerH := a.middleHeight

	if contentX < innerX || contentX >= innerX+innerW {
		if msg.Action == tea.MouseActionRelease {
			a.contentSelection.EndSelection()
		}
		return a, nil
	}
	if contentY < innerY || contentY >= innerY+innerH {
		if msg.Action == tea.MouseActionRelease {
			a.contentSelection.EndSelection()
		}
		return a, nil
	}

	if msg.Button != tea.MouseButtonLeft {
		return a, nil
	}

	line := contentY - innerY
	col := contentX - innerX
	var cmd tea.Cmd
	if msg.Action == tea.MouseActionPress {
		a.inputSelection.Clear()
		var content string
		if a.middleMode == ModePreview {
			content = a.preview.View()
		} else {
			content = a.agent.View()
		}
		a.contentSelection.SetLines(contentSelectionLines(content, a.leftContentW, a.middleHeight))
		a.contentSelection.BeginSelection(line, col)
		return a, nil
	}
	if len(a.contentSelection.lines) == 0 {
		var content string
		if a.middleMode == ModePreview {
			content = a.preview.View()
		} else {
			content = a.agent.View()
		}
		a.contentSelection.SetLines(contentSelectionLines(content, a.leftContentW, a.middleHeight))
	}
	if msg.Action == tea.MouseActionMotion {
		a.contentSelection.UpdateSelection(line, col)
		return a, nil
	}
	if msg.Action == tea.MouseActionRelease {
		a.contentSelection.UpdateSelection(line, col)
		a.contentSelection.EndSelection()
		text := a.contentSelection.SelectedText()
		if text != "" {
			cmd = copyToClipboardCmd(text)
		}
		return a, cmd
	}

	return a, nil
}

func (a App) handleInputSelection(msg tea.MouseMsg, contentX, contentY int) (bool, tea.Cmd) {
	if contentX < a.leftXStart || contentX > a.leftXEnd {
		if msg.Action == tea.MouseActionRelease {
			a.inputSelection.EndSelection()
		}
		return false, nil
	}
	if contentY < a.inputYStart || contentY > a.inputYEnd {
		if msg.Action == tea.MouseActionRelease {
			a.inputSelection.EndSelection()
		}
		return false, nil
	}

	innerX, innerY := panelInnerOrigin(inputBarStyle, a.leftXStart, a.inputYStart)
	innerW := a.leftContentW
	innerH := a.inputHeight

	if contentX < innerX || contentX >= innerX+innerW {
		if msg.Action == tea.MouseActionRelease {
			a.inputSelection.EndSelection()
		}
		return false, nil
	}
	if contentY < innerY || contentY >= innerY+innerH {
		if msg.Action == tea.MouseActionRelease {
			a.inputSelection.EndSelection()
		}
		return false, nil
	}

	if msg.Button != tea.MouseButtonLeft {
		return true, nil
	}

	inputAreaLines := a.inputHeight - 1
	line := contentY - innerY
	if line >= inputAreaLines {
		if msg.Action == tea.MouseActionRelease {
			a.inputSelection.EndSelection()
		}
		return true, nil
	}

	col := contentX - innerX
	if msg.Action == tea.MouseActionPress {
		a.contentSelection.Clear()
		a.inputSelection.SetLines(InputSelectionLines(a.input, inputAreaLines))
		a.inputSelection.BeginSelection(line, col)
		return true, nil
	}
	if msg.Action == tea.MouseActionMotion {
		a.inputSelection.UpdateSelection(line, col)
		return true, nil
	}
	if msg.Action == tea.MouseActionRelease {
		a.inputSelection.UpdateSelection(line, col)
		a.inputSelection.EndSelection()
		text := a.inputSelection.SelectedText()
		if text == "" {
			return true, nil
		}
		return true, copyToClipboardCmd(text)
	}

	return true, nil
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
	items := a.paletteVisibleItems()
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
	if a.paletteQuery != "" {
		lines = append(lines, metaStyle.Render("Search: "+a.paletteQuery))
	}
	selectedIndex := a.paletteIndex
	if selectedIndex < 0 || selectedIndex >= len(items) {
		selectedIndex = 0
	}
	for i, item := range items {
		line := a.formatPaletteLine(item.Label, item.Desc, contentWidth)
		style := normalRowStyle
		if i == selectedIndex {
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

func (a App) paletteVisibleItems() []paletteItem {
	items := a.paletteItems()
	query := strings.TrimSpace(a.paletteQuery)
	if query == "" {
		return items
	}
	query = strings.ToLower(query)
	filtered := make([]paletteItem, 0, len(items))
	for _, item := range items {
		label := strings.ToLower(item.Label)
		desc := strings.ToLower(item.Desc)
		if strings.Contains(label, query) || strings.Contains(desc, query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
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
	minContentW := 10
	minPanelW := panelFW + minContentW
	a.twoColumn = innerW >= minPanelW*2

	modalW := a.width
	if modalW < 1 {
		modalW = 1
	}
	modalH := a.height
	if modalH < 1 {
		modalH = 1
	}
	a.modalWidth = modalW
	a.modalHeight = modalH
	modalFW, modalFH := modalStyle.GetFrameSize()
	leftFrame := modalFW / 2
	topFrame := modalFH / 2
	a.modalContentW = modalW - modalFW
	a.modalContentH = modalH - modalFH
	if a.modalContentW < 1 {
		a.modalContentW = 1
	}
	if a.modalContentH < 1 {
		a.modalContentH = 1
	}
	a.modalXStart = 0
	a.modalYStart = 0
	a.modalXEnd = a.modalXStart + modalW - 1
	a.modalYEnd = a.modalYStart + modalH - 1
	a.modalContentX = a.modalXStart + leftFrame
	a.modalContentY = a.modalYStart + topFrame
	a.modal.SetSize(modalW, modalH)

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
	inputLines := InputLineCount(a.input)
	inputAreaLines := inputLines + 1
	if inputAreaLines < 2 {
		inputAreaLines = 2
	}
	if inputAreaLines > maxInputLines+1 {
		inputAreaLines = maxInputLines + 1
	}
	a.inputHeight = inputAreaLines + 1
	inputRendered := a.inputHeight + panelFH

	available := totalInner - a.statusHeight
	minPanelRendered := panelFH + 1
	if available < minPanelRendered {
		available = minPanelRendered
	}
	if inputRendered+minPanelRendered > available {
		inputRendered = available - minPanelRendered
		if inputRendered < minPanelRendered {
			inputRendered = minPanelRendered
		}
		a.inputHeight = inputRendered - panelFH
		if a.inputHeight < 2 {
			a.inputHeight = 2
		}
	}
	outputRendered := available - inputRendered
	if outputRendered < minPanelRendered {
		outputRendered = minPanelRendered
	}

	if a.twoColumn {
		leftW := innerW * 5 / 8
		rightW := innerW - leftW
		if leftW < minPanelW {
			leftW = minPanelW
			rightW = innerW - leftW
		}
		if rightW < minPanelW {
			rightW = minPanelW
			leftW = innerW - rightW
		}
		if leftW < minPanelW || rightW < minPanelW {
			a.twoColumn = false
		}
		a.leftWidth = leftW
		a.rightWidth = rightW
		a.leftContentW = leftW - panelFW
		a.rightContentW = rightW - panelFW
		if a.leftContentW < minContentW {
			a.leftContentW = minContentW
		}
		if a.rightContentW < minContentW {
			a.rightContentW = minContentW
		}

		a.historyHeight = available - panelFH
		a.middleHeight = outputRendered - panelFH
		if a.historyHeight < 1 {
			a.historyHeight = 1
		}
		if a.middleHeight < 1 {
			a.middleHeight = 1
		}

		a.history.SetSize(a.rightContentW, a.historyHeight)
		a.preview.SetSize(a.leftContentW, a.middleHeight)
		a.detail.SetSize(a.leftContentW, a.middleHeight)
		a.agent.SetSize(a.leftContentW, a.middleHeight)
		a.input.SetWidth(a.leftContentW)
		inputWidgetHeight := InputLineCount(a.input)
		maxWidgetHeight := a.inputHeight - 1
		if inputWidgetHeight > maxWidgetHeight {
			inputWidgetHeight = maxWidgetHeight
		}
		if inputWidgetHeight < 1 {
			inputWidgetHeight = 1
		}
		a.input.SetHeight(inputWidgetHeight)

		a.leftXStart = 1
		a.leftXEnd = a.leftXStart + a.leftWidth - 1
		a.rightXStart = a.leftXEnd + 1
		a.rightXEnd = a.rightXStart + a.rightWidth - 1
		a.contentYStart = 1
		a.contentYEnd = a.contentYStart + outputRendered - 1
		a.inputYStart = a.contentYEnd + 1
		a.inputYEnd = a.inputYStart + inputRendered - 1
		a.historyYStart = 1
		a.historyYEnd = a.historyYStart + available - 1
		return
	}

	a.leftWidth = innerW
	a.rightWidth = 0
	a.leftContentW = innerW - panelFW
	if a.leftContentW < minContentW {
		a.leftContentW = minContentW
	}
	a.rightContentW = 0

	availableVert := totalInner - inputRendered - a.statusHeight
	if availableVert < panelFH*2+2 {
		availableVert = panelFH*2 + 2
	}
	historyRendered := availableVert / 4
	if historyRendered < minPanelRendered {
		historyRendered = minPanelRendered
	}
	contentRendered := availableVert - historyRendered
	if contentRendered < minPanelRendered {
		contentRendered = minPanelRendered
	}
	a.historyHeight = historyRendered - panelFH
	a.middleHeight = contentRendered - panelFH
	if a.historyHeight < 1 {
		a.historyHeight = 1
	}
	if a.middleHeight < 1 {
		a.middleHeight = 1
	}

	a.history.SetSize(a.leftContentW, a.historyHeight)
	a.preview.SetSize(a.leftContentW, a.middleHeight)
	a.detail.SetSize(a.leftContentW, a.middleHeight)
	a.agent.SetSize(a.leftContentW, a.middleHeight)
	a.input.SetWidth(a.leftContentW)
	inputWidgetHeight := InputLineCount(a.input)
	maxWidgetHeight := a.inputHeight - 1
	if inputWidgetHeight > maxWidgetHeight {
		inputWidgetHeight = maxWidgetHeight
	}
	if inputWidgetHeight < 1 {
		inputWidgetHeight = 1
	}
	a.input.SetHeight(inputWidgetHeight)

	a.leftXStart = 1
	a.leftXEnd = innerW
	a.rightXStart = 0
	a.rightXEnd = 0
	y := 1
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

func waitForCommandExecutedCmd() tea.Cmd {
	return func() tea.Msg {
		<-agent.CommandExecutedEvents()
		return commandExecutedMsg{}
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

func mouseMotionFilter(model tea.Model, msg tea.Msg) tea.Msg {
	mouse, ok := msg.(tea.MouseMsg)
	if !ok {
		return msg
	}
	if mouse.Action != tea.MouseActionMotion {
		return msg
	}
	if mouse.Button != tea.MouseButtonLeft {
		return nil
	}
	var app App
	switch m := model.(type) {
	case App:
		app = m
	case *App:
		if m == nil {
			return msg
		}
		app = *m
	default:
		return msg
	}
	if !app.hasActiveDragSelection() {
		return nil
	}
	if !app.lastMouseMotion.IsZero() && time.Since(app.lastMouseMotion) < mouseMotionThrottle {
		return nil
	}
	return msg
}

// Run starts the TUI.
func Run(database *db.DB, cfg *config.Config, logger *zap.Logger) error {
	app := New(database, cfg, logger)
	program := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithFPS(30),
		tea.WithFilter(mouseMotionFilter),
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
