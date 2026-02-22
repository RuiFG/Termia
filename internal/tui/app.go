package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agent/team"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/diagnostics"
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
	AgentModeAgent
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
	paletteStageSessions
	paletteStageTeams
)

type paletteAction int

const (
	paletteActionOpenModels paletteAction = iota
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
	Header bool
}

type paletteSection struct {
	Label string
	Items []paletteItem
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
	chordPending     bool
	thinkLevel       ThinkLevel
	statusMsg        string
	contentSelection textSelection
	inputSelection   textSelection
	lastMouseMotion  time.Time
	launchCwd        string
	cwd              string
	cwdSyncWarned    bool
	sessionCwds      map[string]string

	// Team selection
	teams           []config.AgentTeamProfile
	activeTeamName  string
	activeTeamRoles []string

	// Sessions
	sessions                 []db.AgentSession
	activeSessionID          string
	responseBuffer           []string
	pendingPromptID          string
	pendingPromptSessionID   string
	agentRunning             bool
	agentCancel              context.CancelFunc
	agentLastEsc             time.Time
	agentProgressStep        int
	approvalInput            ApprovalInput
	approvalDecision         *agent.ApprovalDecision
	approvalRequests         chan approvalRequest
	approvalResponseCh       chan agent.ApprovalDecision
	askInput                 AskInput
	askAnswers               []agent.AskAnswer
	pendingPrompts           map[string][]db.PendingPrompt
	approvalDecisionPromptID string
	askAnswersPromptID       string

	// Command palette
	paletteOpen  bool
	paletteStage paletteStage
	paletteIndex int
	paletteQuery string

	dirPromptOpen    bool
	dirPromptInput   textinput.Model
	dirPromptMatches []string
	dirPromptError   string
	dirPromptIndex   int
	dirPromptScroll  int

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

type agentStartMsg struct {
	stream <-chan string
	err    error
}

type agentDoneMsg struct{}

type agentErrorMsg struct {
	err error
}

type agentProgressTickMsg struct{}

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
	pending   []db.PendingPrompt
}

type favoriteToggledMsg struct {
	id string
}

func newDirPromptInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "> "
	input.PromptStyle = inputPromptStyle
	input.CharLimit = 500
	input.Cursor.Style = inputCursorStyle
	input.Cursor.Blink = false
	return input
}

// New creates a new App model.
func New(database *db.DB, cfg *config.Config, logger *zap.Logger) App {
	keys := DefaultKeyMap()
	teams, activeName, activeRoles := resolveTeams(cfg.Agent)
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	input := NewInputModel()
	app := App{
		db:               database,
		cfg:              cfg,
		logger:           logger,
		focus:            FocusInput, // Start with input focused (standard TUI pattern)
		middleMode:       ModeAgent,  // Default to Agent view
		agentMode:        AgentModeAgent,
		thinkLevel:       ThinkMedium,
		history:          NewHistoryModel(keys),
		preview:          NewPreviewModel(keys),
		detail:           NewHistoryDetailModel(keys),
		agent:            NewAgentModel(keys),
		modal:            NewModalModel(),
		input:            input,
		approvalInput:    NewApprovalInput(),
		approvalRequests: make(chan approvalRequest),
		askInput:         NewAskInput(),
		keys:             keys,
		teams:            teams,
		activeTeamName:   activeName,
		activeTeamRoles:  activeRoles,
		launchCwd:        cwd,
		cwd:              cwd,
		sessionCwds:      make(map[string]string),
		pendingPrompts:   make(map[string][]db.PendingPrompt),
		dirPromptInput:   newDirPromptInput(),
	}
	app.updateInputPrompt()
	return app
}

// Init loads the initial data asynchronously.
func (a App) Init() tea.Cmd {
	if a.db != nil {
		stop := diagnostics.Track("tui.init.pending_prompt_count", nil)
		err := a.updatePendingPromptCount()
		stop()
		if err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to sync pending prompt count", zap.Error(err))
			}
		}
	}
	return tea.Batch(
		loadCommandsCmd(a.db),
		loadSessionsCmd(a.db),
		waitForCommandExecutedCmd(),
		waitForApprovalRequestCmd(a.approvalRequests),
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
		a.cacheSessionCwds(a.sessions)
		if a.activeSessionID == "" {
			if len(a.sessions) == 0 {
				return a, createSessionCmd(a.db, a.launchCwd)
			}
			a.setActiveSessionID(a.sessions[0].ID)
			a.ensureSessionCwd(a.activeSessionID)
			a.applySessionCwd(a.activeSessionID)
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
		a.setActiveSessionID(msg.session.ID)
		a.cacheSessionCwd(msg.session)
		a.ensureSessionCwd(msg.session.ID)
		a.applySessionCwd(msg.session.ID)
		a.input.SetHistory(nil)
		a.agent.SetMessages(nil)
		return a, nil

	case sessionMessagesLoadedMsg:
		if msg.sessionID != a.activeSessionID {
			return a, nil
		}
		a.agent.SetMessages(formatSessionMessages(msg.messages))
		a.input.SetHistory(buildInputHistory(msg.messages))
		a.pendingPrompts[msg.sessionID] = msg.pending
		a.activatePendingPrompt()
		return a, nil

	case approvalRequestMsg:
		a.approvalResponseCh = msg.request.response
		a.approvalInput.SetPrompt(msg.request.prompt)
		a.approvalInput.SetWidth(a.leftContentW)
		a.focus = FocusInput
		a.updateFocusState()
		return a, waitForApprovalRequestCmd(a.approvalRequests)

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

	case agentStartMsg:
		if msg.err != nil {
			_ = os.Unsetenv("TERMIA_SESSION_ID")
			a.agent.AppendToLast(fmt.Sprintf("Error: %v", msg.err))
			a.agentRunning = false
			a.agentCancel = nil
			a.agentLastEsc = time.Time{}
			a.agentProgressStep = 0
			return a, nil
		}
		if msg.stream == nil {
			_ = os.Unsetenv("TERMIA_SESSION_ID")
			a.agent.AppendToLast("Error: agent stream unavailable")
			a.agentRunning = false
			a.agentCancel = nil
			a.agentLastEsc = time.Time{}
			a.agentProgressStep = 0
			return a, nil
		}
		return a, readAgentChunkCmd(msg.stream)

	case agentDoneMsg:
		_ = os.Unsetenv("TERMIA_SESSION_ID")
		a.agentRunning = false
		a.agentCancel = nil
		a.agentLastEsc = time.Time{}
		a.agentProgressStep = 0
		if a.statusMsg == "Stopping agent..." {
			a.statusMsg = ""
		}
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
		_ = os.Unsetenv("TERMIA_SESSION_ID")
		a.agent.AddMessage("error", fmt.Sprintf("Error: %v", msg.err))
		a.agentRunning = false
		a.agentCancel = nil
		a.agentLastEsc = time.Time{}
		a.agentProgressStep = 0
		if a.statusMsg == "Stopping agent..." {
			a.statusMsg = ""
		}
		return a, nil

	case agentProgressTickMsg:
		if a.agentRunning {
			a.agentProgressStep = (a.agentProgressStep + 1) % 3
			if !a.agentLastEsc.IsZero() && time.Since(a.agentLastEsc) > time.Second {
				a.agentLastEsc = time.Time{}
			}
			return a, agentProgressTickCmd()
		}
		return a, nil

	case tea.KeyMsg:
		if a.agentRunning && msg.Type != tea.KeyEsc {
			a.agentLastEsc = time.Time{}
		}
		if a.agentRunning && msg.Type == tea.KeyEsc && !a.modal.IsOpen() && !a.dirPromptOpen && !a.paletteOpen && !a.approvalInput.Active() && !a.askInput.Active() {
			return a.handleAgentEsc()
		}
		if a.modal.IsOpen() {
			return a.handleModalKey(msg)
		}
		if a.dirPromptOpen {
			return a.handleDirPromptKey(msg)
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
					a.resetPaletteIndex()
				}
				return a, nil
			case msg.Type == tea.KeySpace:
				a.paletteQuery += " "
				a.resetPaletteIndex()
				return a, nil
			case msg.Type == tea.KeyRunes:
				if len(msg.Runes) > 0 {
					a.paletteQuery += string(msg.Runes)
					a.resetPaletteIndex()
				}
				return a, nil
			}
			return a, nil
		}

		if a.chordPending {
			return a.handleChordKey(msg)
		}
		if msg.Type == tea.KeyCtrlX {
			a.chordPending = true
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

func (a App) handleChordKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		a.chordPending = false
		return a, nil
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		a.chordPending = false
		return a, nil
	}

	key := unicode.ToLower(msg.Runes[0])
	switch key {
	case 'a':
		a.agentMode = AgentModeAgent
		a.middleMode = ModeAgent
		a.statusMsg = "Mode set to Agent."
	case 't':
		a.openPaletteStage(paletteStageTeams)
	case 'c':
		a.openDirPrompt()
	}
	a.chordPending = false
	return a, nil
}

func (a *App) openDirPrompt() {
	a.dirPromptOpen = true
	a.dirPromptError = ""
	a.dirPromptMatches = nil
	a.dirPromptIndex = -1
	a.dirPromptScroll = 0
	a.dirPromptInput.SetValue("")
	a.dirPromptInput.Placeholder = tildePath(a.cwd)
	a.dirPromptInput.Focus()
	a.paletteOpen = false
	a.updateDirPromptMatches()
}

func (a *App) closeDirPrompt() {
	a.dirPromptOpen = false
	a.dirPromptError = ""
	a.dirPromptMatches = nil
	a.dirPromptIndex = -1
	a.dirPromptScroll = 0
	a.dirPromptInput.Blur()
}

func (a App) handleDirPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		a.closeDirPrompt()
		return a, nil
	case tea.KeyEnter:
		return a.submitDirPrompt()
	case tea.KeyUp:
		a.moveDirPromptSelection(-1)
		return a, nil
	case tea.KeyDown:
		a.moveDirPromptSelection(1)
		return a, nil
	case tea.KeyTab:
		if !a.applyDirPromptSelection() {
			a.completeDirPrompt()
		}
		return a, nil
	}

	var cmd tea.Cmd
	a.dirPromptInput, cmd = a.dirPromptInput.Update(msg)
	a.updateDirPromptMatches()
	return a, cmd
}

func (a *App) updateDirPromptMatches() {
	_, matches := completeDirPrompt(a.dirPromptInput.Value(), a.cwd)
	a.setDirPromptMatches(matches)
}

func (a *App) completeDirPrompt() {
	value := a.dirPromptInput.Value()
	completed, matches := completeDirPrompt(value, a.cwd)
	a.setDirPromptMatches(matches)
	if completed != value {
		a.dirPromptInput.SetValue(completed)
		a.dirPromptInput.CursorEnd()
	}
}

func (a *App) setDirPromptMatches(matches []string) {
	a.dirPromptMatches = matches
	if len(matches) == 0 {
		a.dirPromptIndex = -1
		a.dirPromptScroll = 0
		return
	}
	if a.dirPromptIndex >= len(matches) {
		a.dirPromptIndex = len(matches) - 1
	}
	if a.dirPromptIndex < -1 {
		a.dirPromptIndex = -1
	}
	a.ensureDirPromptVisible()
}

func (a *App) dirPromptVisibleMatches() []string {
	start, end := a.dirPromptWindow()
	if start >= end {
		return nil
	}
	return a.dirPromptMatches[start:end]
}

func (a *App) dirPromptMaxVisible() int {
	if len(a.dirPromptMatches) == 0 {
		return 0
	}
	return minInt(6, len(a.dirPromptMatches))
}

func (a *App) dirPromptWindow() (int, int) {
	maxItems := a.dirPromptMaxVisible()
	if maxItems == 0 {
		return 0, 0
	}
	start := a.dirPromptScroll
	if start < 0 {
		start = 0
	}
	maxStart := len(a.dirPromptMatches) - maxItems
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end := start + maxItems
	if end > len(a.dirPromptMatches) {
		end = len(a.dirPromptMatches)
	}
	return start, end
}

func (a *App) ensureDirPromptVisible() {
	maxItems := a.dirPromptMaxVisible()
	if maxItems == 0 {
		a.dirPromptScroll = 0
		return
	}
	if a.dirPromptIndex < 0 {
		a.dirPromptScroll = 0
		return
	}
	if a.dirPromptIndex < a.dirPromptScroll {
		a.dirPromptScroll = a.dirPromptIndex
	}
	if a.dirPromptIndex >= a.dirPromptScroll+maxItems {
		a.dirPromptScroll = a.dirPromptIndex - maxItems + 1
	}
	maxScroll := len(a.dirPromptMatches) - maxItems
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.dirPromptScroll > maxScroll {
		a.dirPromptScroll = maxScroll
	}
	if a.dirPromptScroll < 0 {
		a.dirPromptScroll = 0
	}
}

func (a *App) moveDirPromptSelection(delta int) {
	if len(a.dirPromptMatches) == 0 {
		a.dirPromptIndex = -1
		a.dirPromptScroll = 0
		return
	}
	if a.dirPromptIndex < 0 {
		if delta < 0 {
			a.dirPromptIndex = len(a.dirPromptMatches) - 1
		} else {
			a.dirPromptIndex = 0
		}
		a.ensureDirPromptVisible()
		return
	}
	a.dirPromptIndex += delta
	if a.dirPromptIndex < 0 {
		a.dirPromptIndex = len(a.dirPromptMatches) - 1
		a.ensureDirPromptVisible()
		return
	}
	if a.dirPromptIndex >= len(a.dirPromptMatches) {
		a.dirPromptIndex = 0
	}
	a.ensureDirPromptVisible()
}

func (a *App) applyDirPromptSelection() bool {
	if len(a.dirPromptMatches) == 0 {
		return false
	}
	if a.dirPromptIndex < 0 || a.dirPromptIndex >= len(a.dirPromptMatches) {
		return false
	}
	value := a.dirPromptInput.Value()
	dirPart, _ := splitDirInput(value)
	selection := dirPart + a.dirPromptMatches[a.dirPromptIndex]
	if selection == value {
		return false
	}
	a.dirPromptInput.SetValue(selection)
	a.dirPromptInput.CursorEnd()
	a.updateDirPromptMatches()
	return true
}

func (a App) submitDirPrompt() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(a.dirPromptInput.Value())
	if value == "" {
		a.closeDirPrompt()
		return a, nil
	}
	resolved, err := resolveDirPath(value, a.cwd)
	if err != nil {
		a.dirPromptError = err.Error()
		return a, nil
	}
	if err := os.Chdir(resolved); err != nil {
		a.dirPromptError = fmt.Sprintf("%s: %v", resolved, err)
		return a, nil
	}
	a.setCwd(resolved)
	a.closeDirPrompt()
	return a, nil
}

// handleInputKey processes key events when input is focused.
func (a App) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.approvalInput.Active() {
		decision, cmd := a.approvalInput.Update(msg)
		if decision != nil {
			return a.handleApprovalDecision(*decision)
		}
		return a, cmd
	}
	if a.askInput.Active() {
		answers, cmd := a.askInput.Update(msg)
		if answers != nil {
			return a.handleAskAnswers(*answers)
		}
		return a, cmd
	}
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

func (a App) handleApprovalDecision(decision agent.ApprovalDecision) (tea.Model, tea.Cmd) {
	promptID := a.pendingPromptID
	promptSessionID := a.pendingPromptSessionID
	if a.db != nil && promptID != "" {
		payload, err := json.Marshal(decision)
		if err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to encode approval decision", zap.Error(err))
			}
			a.statusMsg = "Error: failed to encode approval decision"
			return a, nil
		}
		if err := a.db.ResolvePendingPromptWithResponse(promptID, string(payload)); err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to resolve approval prompt", zap.Error(err))
			}
			a.statusMsg = "Error: failed to resolve approval prompt"
			return a, nil
		}
		_ = a.updatePendingPromptCount()
	}
	a.approvalDecision = &decision
	a.approvalDecisionPromptID = promptID
	a.pendingPromptID = ""
	a.pendingPromptSessionID = ""
	a.dequeuePendingPrompt(promptSessionID, promptID)
	if a.approvalResponseCh != nil {
		select {
		case a.approvalResponseCh <- decision:
		default:
		}
		a.approvalResponseCh = nil
	}
	a.approvalInput.Mode = ApprovalModeNone
	a.approvalInput.editedValue = ""
	a.approvalInput.editInput.SetValue("")
	a.approvalInput.rephrase.SetValue("")
	a.activatePendingPrompt()
	return a, nil
}

func (a App) handleAskAnswers(answers []agent.AskAnswer) (tea.Model, tea.Cmd) {
	promptID := a.pendingPromptID
	promptSessionID := a.pendingPromptSessionID
	if a.db != nil && promptID != "" {
		payload, err := json.Marshal(answers)
		if err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to encode ask answers", zap.Error(err))
			}
			a.statusMsg = "Error: failed to encode ask answers"
			return a, nil
		}
		if err := a.db.ResolvePendingPromptWithResponse(promptID, string(payload)); err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to resolve ask prompt", zap.Error(err))
			}
			a.statusMsg = "Error: failed to resolve ask prompt"
			return a, nil
		}
		_ = a.updatePendingPromptCount()
	}
	a.askAnswers = answers
	a.askAnswersPromptID = promptID
	a.pendingPromptID = ""
	a.pendingPromptSessionID = ""
	a.dequeuePendingPrompt(promptSessionID, promptID)
	a.askInput.Mode = AskModeNone
	a.askInput.Questions = nil
	a.askInput.Answers = nil
	a.askInput.Selected = nil
	a.askInput.Index = 0
	a.askInput.Cursor = 0
	a.askInput.Custom.SetValue("")
	a.activatePendingPrompt()
	return a, nil
}

func (a *App) activatePendingPrompt() {
	if a.approvalInput.Active() || a.askInput.Active() {
		return
	}
	if strings.TrimSpace(a.activeSessionID) == "" {
		return
	}
	for {
		prompts := a.pendingPrompts[a.activeSessionID]
		if len(prompts) == 0 {
			a.pendingPromptID = ""
			a.pendingPromptSessionID = ""
			return
		}
		prompt := prompts[0]
		payload := db.ParsePendingPromptPayload(prompt)
		if strings.TrimSpace(prompt.PromptType) != "" {
			payload.Type = strings.TrimSpace(prompt.PromptType)
		}
		switch payload.Type {
		case db.PendingPromptTypeAsk:
			var askPayload struct {
				Questions []agent.AskQuestion `json:"questions"`
			}
			if len(payload.Payload) == 0 {
				a.handleInvalidPendingPrompt(prompt, "Error: ask payload missing")
				continue
			}
			if err := json.Unmarshal(payload.Payload, &askPayload); err != nil {
				a.handleInvalidPendingPrompt(prompt, fmt.Sprintf("Error: invalid ask payload: %v", err))
				continue
			}
			a.pendingPromptID = prompt.PromptID
			a.pendingPromptSessionID = prompt.SessionID
			a.askInput.SetQuestions(askPayload.Questions)
			a.askInput.SetWidth(a.leftContentW)
			return
		default:
			command := strings.TrimSpace(payload.Command)
			if command == "" {
				command = strings.TrimSpace(prompt.Content)
			}
			a.pendingPromptID = prompt.PromptID
			a.pendingPromptSessionID = prompt.SessionID
			a.approvalInput.SetPrompt(agent.ApprovalPrompt{Command: command})
			a.approvalInput.SetWidth(a.leftContentW)
			return
		}
	}
}

func (a *App) handleInvalidPendingPrompt(prompt db.PendingPrompt, message string) {
	a.statusMsg = message
	if a.db != nil && prompt.PromptID != "" {
		if err := a.db.ResolvePendingPromptWithResponse(prompt.PromptID, ""); err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to resolve invalid pending prompt", zap.Error(err))
			}
		}
		if err := a.updatePendingPromptCount(); err != nil {
			if a.logger != nil {
				a.logger.Warn("failed to update pending prompt count", zap.Error(err))
			}
		}
	}
	a.dequeuePendingPrompt(prompt.SessionID, prompt.PromptID)
	if a.pendingPromptID == prompt.PromptID {
		a.pendingPromptID = ""
		a.pendingPromptSessionID = ""
	}
}

func (a *App) dequeuePendingPrompt(sessionID string, promptID string) {
	if sessionID == "" || promptID == "" {
		return
	}
	prompts := a.pendingPrompts[sessionID]
	if len(prompts) == 0 {
		return
	}
	for idx, prompt := range prompts {
		if prompt.PromptID != promptID {
			continue
		}
		prompts = append(prompts[:idx], prompts[idx+1:]...)
		break
	}
	if len(prompts) == 0 {
		delete(a.pendingPrompts, sessionID)
		return
	}
	a.pendingPrompts[sessionID] = prompts
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

	a.input.AddHistory(val)

	// Check for slash command
	if cmd := a.input.ParseSlashCommand(); cmd != nil {
		a.input.Reset()
		return a, executeSlashCommand(cmd, a.db, &a.cfg.LLM)
	}

	// Regular text input -> Send to Agent
	a.middleMode = ModeAgent
	a.agent.AddMessage("user", val)
	a.agent.AddMessage("agent", "")
	a.responseBuffer = nil

	if a.activeSessionID == "" {
		newSession, err := createSession(a.db, a.cwd)
		if err != nil {
			a.agent.AppendToLast(fmt.Sprintf("Error: %v", err))
			a.input.Reset()
			return a, nil
		}
		a.setActiveSessionID(newSession.ID)
		a.sessions = append([]db.AgentSession{newSession}, a.sessions...)
	}
	a.setActiveSessionID(a.activeSessionID)

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

	ctx, cancel := context.WithCancel(context.Background())
	a.agentCancel = cancel
	a.agentRunning = true
	a.agentLastEsc = time.Time{}
	a.agentProgressStep = 0
	startCmd := a.startAgentRunCmd(ctx, val)

	a.input.Reset()
	return a, tea.Batch(startCmd, agentProgressTickCmd())
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

func (a *App) runTeamQuery(ctx context.Context, query string) (<-chan string, error) {
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

	teamRunner, err := team.NewTeamRunner(ctx, cfg, a.db, a.logger)
	if err != nil {
		return nil, err
	}

	return teamRunner.Run(ctx, query, nil)
}

func (a *App) runAgentQuery(ctx context.Context, query string) (<-chan string, error) {
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
	tools := agent.CreateTools(a.db, a.cfg.Agent.RequireApproval, newTUIApprovalProvider(a.approvalRequests))
	reactRunner, err := agent.NewReactRunner(ctx, model, tools, a.db, a.logger)
	if err != nil {
		return nil, fmt.Errorf("create react runner: %w", err)
	}

	return reactRunner.Run(ctx, query, nil)
}

func (a App) startAgentRunCmd(ctx context.Context, query string) tea.Cmd {
	return func() tea.Msg {
		var (
			stream <-chan string
			err    error
		)
		if a.agentMode == AgentModeAgent {
			stream, err = a.runAgentQuery(ctx, query)
		} else {
			stream, err = a.runTeamQuery(ctx, query)
		}
		return agentStartMsg{stream: stream, err: err}
	}
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

func agentProgressTickCmd() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return agentProgressTickMsg{}
	})
}

// handleSlashResult processes the result of a slash command.
func (a App) handleSlashResult(result SlashCommandResult) (tea.Model, tea.Cmd) {
	if result.Quit {
		return a, tea.Quit
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
	if result.OpenPalette != nil {
		a.openPaletteStage(*result.OpenPalette)
	}

	if result.Output != "" {
		a.middleMode = ModeAgent
		a.agent.AddMessage("agent", result.Output)
	}

	a.updateFocusState()
	if result.CreateSession {
		return a, createSessionCmd(a.db, a.launchCwd)
	}
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
	if a.dirPromptOpen {
		prompt := a.renderDirPrompt(innerW)
		body = overlayContentCentered(body, prompt, innerW, innerH)
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
	if a.approvalInput.Active() {
		inputView = a.approvalInput.View(contentWidth)
	} else if a.askInput.Active() {
		inputView = a.askInput.View(contentWidth)
	}

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
	if !a.approvalInput.Active() && !a.askInput.Active() && a.inputSelection.HasSelection() {
		inputLines = a.inputSelection.HighlightLines(contentWidth)
	}
	cwdLine := a.renderInputCwdLine(contentWidth)
	if cwdLine != "" {
		inputLines = append([]string{cwdLine}, inputLines...)
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
	case AgentModeAgent:
		return agentModeStyle.Render("Agent")
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

func (a App) renderAgentRunHint() string {
	if !a.agentRunning {
		return ""
	}
	dots := a.agentProgressDots()
	if !a.agentLastEsc.IsZero() && time.Since(a.agentLastEsc) <= time.Second {
		label := "Esc again to stop"
		if dots != "" {
			label = fmt.Sprintf("%s %s", label, dots)
		}
		return metaStyle.Render(label)
	}
	return metaStyle.Render(dots)
}

func (a App) agentProgressDots() string {
	if !a.agentRunning {
		return ""
	}
	count := (a.agentProgressStep % 3) + 1
	dots := strings.Repeat(".", count)
	pad := strings.Repeat(" ", 3-count)
	return dots + pad
}

func (a App) handleAgentEsc() (tea.Model, tea.Cmd) {
	if !a.agentRunning || a.agentCancel == nil {
		return a, nil
	}
	now := time.Now()
	if !a.agentLastEsc.IsZero() && now.Sub(a.agentLastEsc) <= time.Second {
		a.agentCancel()
		a.statusMsg = "Stopping agent..."
		a.agentLastEsc = time.Time{}
		return a, nil
	}
	a.agentLastEsc = now
	return a, nil
}

func (a App) renderAgentLabel() string {
	if a.agentMode == AgentModeAgent {
		return agentModeStyle.Render("Agent")
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
	leftParts := []string{}
	if hint := a.renderAgentRunHint(); hint != "" {
		leftParts = append(leftParts, hint)
	}
	if a.statusMsg != "" {
		leftParts = append(leftParts, a.statusMsg)
	}
	left := strings.TrimSpace(strings.Join(leftParts, "  "))
	right := a.renderStatusHints()
	line := buildStatusLine(left, right, contentWidth)
	pw := statusBarStyle.GetHorizontalPadding()
	return statusBarStyle.Width(contentWidth + pw).Render(line)
}

func (a App) renderStatusHints() string {
	hints := fmt.Sprintf("Ctrl+P %s | Ctrl+T %s | Tab %s",
		metaStyle.Render("command"),
		metaStyle.Render("variants"),
		metaStyle.Render("windows"),
	)
	badge := a.pendingPromptBadge()
	if badge == "" {
		return hints
	}
	return fmt.Sprintf("%s  %s", badge, hints)
}

func (a App) pendingPromptBadge() string {
	count := a.pendingPromptCount()
	if count <= 0 {
		return ""
	}
	return metadataLabelStyle.Render(fmt.Sprintf("🔔%d", count))
}

func (a App) pendingPromptCount() int {
	path := config.PendingPromptsCountPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return count
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
	a.paletteQuery = ""
	a.resetPaletteIndex()
}

func (a *App) openPaletteStage(stage paletteStage) {
	a.paletteOpen = true
	a.paletteStage = stage
	a.paletteQuery = ""
	a.resetPaletteIndex()
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
	a.paletteIndex = nextSelectableIndex(items, a.paletteIndex, delta)
}

func (a App) handlePaletteSelect() (tea.Model, tea.Cmd) {
	items := a.paletteVisibleItems()
	if len(items) == 0 {
		return a, nil
	}
	if a.paletteIndex < 0 || a.paletteIndex >= len(items) {
		a.paletteIndex = firstSelectableIndex(items)
	}
	item := items[a.paletteIndex]
	if item.Header {
		a.paletteIndex = nextSelectableIndex(items, a.paletteIndex, 1)
		return a, nil
	}
	switch item.Action {
	case paletteActionOpenModels:
		a.paletteStage = paletteStageModels
		a.paletteIndex = 0
		return a, nil
	case paletteActionOpenSessions:
		a.paletteStage = paletteStageSessions
		a.paletteIndex = 0
		return a, nil
	case paletteActionNewSession:
		a.closePalette()
		return a, createSessionCmd(a.db, a.launchCwd)
	case paletteActionSelectModel:
		a.cfg.LLM.DefaultProvider = item.Value
		a.statusMsg = fmt.Sprintf("Model switched to %s.", item.Label)
		a.closePalette()
		return a, nil
	case paletteActionSelectAgent:
		if item.Value == "agent" {
			a.agentMode = AgentModeAgent
			a.statusMsg = "Agent mode set to Agent."
			a.closePalette()
			return a, nil
		}
		if strings.TrimSpace(item.Value) == "" {
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
		a.setActiveSessionID(item.Value)
		a.closePalette()
		a.applySessionCwd(item.Value)
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

	cwdLines := a.inputCwdLineCount()
	inputAreaLines := a.inputHeight - 1 - cwdLines
	line := contentY - innerY - cwdLines
	if line < 0 || line >= inputAreaLines {
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
	lines = append(lines, metaStyle.Render("Search: "+a.paletteQuery))
	lines = append(lines, "")
	selectedIndex := a.paletteIndex
	if selectedIndex < 0 || selectedIndex >= len(items) || items[selectedIndex].Header {
		selectedIndex = firstSelectableIndex(items)
	}
	for i, item := range items {
		if item.Header {
			if len(lines) > 0 {
				last := strings.TrimSpace(stripANSICodes(lines[len(lines)-1]))
				if last != "" {
					lines = append(lines, "")
				}
			}
			lines = append(lines, paletteSectionStyle.Render(item.Label))
			continue
		}
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

func (a App) renderDirPrompt(totalWidth int) string {
	if totalWidth <= 0 {
		return ""
	}
	promptWidth := totalWidth - 6
	if promptWidth > 70 {
		promptWidth = 70
	}
	if promptWidth < 30 {
		promptWidth = totalWidth - 4
	}
	if promptWidth < 20 {
		promptWidth = totalWidth
	}
	frameW, _ := commandPaletteStyle.GetFrameSize()
	contentWidth := promptWidth - frameW
	if contentWidth < 10 {
		contentWidth = 10
	}

	input := a.dirPromptInput
	promptCells := lipgloss.Width(input.Prompt)
	input.Width = maxInt(contentWidth-promptCells, suggestedMinWidth)
	inputLine := input.View()

	lines := []string{
		a.renderDirPromptHeader(contentWidth),
		"",
		metaStyle.Render("Current: " + formatCwdDisplay(a.cwd, contentWidth)),
		"",
		inputLine,
	}
	if strings.TrimSpace(a.dirPromptError) != "" {
		lines = append(lines, "", dirPromptErrorStyle.Render(a.dirPromptError))
	}
	start, _ := a.dirPromptWindow()
	visible := a.dirPromptVisibleMatches()
	if len(visible) > 0 {
		lines = append(lines, "", paletteSectionStyle.Render("Suggestions"))
		for i, match := range visible {
			line := match
			if lipgloss.Width(line) > contentWidth {
				line = truncateToWidth(line, contentWidth)
			}
			style := metaStyle
			if start+i == a.dirPromptIndex {
				style = selectedRowStyle
			}
			lines = append(lines, style.Render(line))
		}
	}

	content := strings.Join(lines, "\n")
	panel := commandPaletteStyle.Width(promptWidth).Render(content)
	padding := (totalWidth - promptWidth) / 2
	return padLeftLines(panel, padding)
}

func (a App) renderDirPromptHeader(width int) string {
	left := paletteHeaderTitleStyle.Render("Change Directory")
	right := metaStyle.Render("Esc")
	return buildStatusLine(left, right, width)
}

func (a App) renderPaletteHeader(width int) string {
	left := paletteHeaderTitleStyle.Render(a.paletteTitle())
	right := metaStyle.Render("Esc")
	return buildStatusLine(left, right, width)
}

func (a App) paletteTitle() string {
	switch a.paletteStage {
	case paletteStageModels:
		return "Models"
	case paletteStageSessions:
		return "Sessions"
	case paletteStageTeams:
		return "Teams"
	default:
		return "Suggested"
	}
}

func (a App) paletteItems() []paletteItem {
	switch a.paletteStage {
	case paletteStageModels:
		return a.modelPaletteItems()
	case paletteStageSessions:
		return a.sessionPaletteItems()
	case paletteStageTeams:
		return a.teamPaletteItems()
	default:
		return nil
	}
}

func (a App) paletteSections() []paletteSection {
	switch a.paletteStage {
	case paletteStageModels:
		return []paletteSection{{Label: "Models", Items: a.modelPaletteItems()}}
	case paletteStageSessions:
		return []paletteSection{{Label: "Sessions", Items: a.sessionPaletteItems()}}
	case paletteStageTeams:
		return []paletteSection{{Label: "Teams", Items: a.teamPaletteItems()}}
	default:
		suggested := []paletteItem{
			{Label: "Models", Action: paletteActionOpenModels},
			{Label: "Sessions", Action: paletteActionOpenSessions},
			{Label: "New Session", Action: paletteActionNewSession},
		}
		return []paletteSection{
			{Label: "Suggested", Items: suggested},
			{Label: "Mode", Items: a.agentPaletteItems()},
		}
	}
}

func (a App) paletteVisibleItems() []paletteItem {
	sections := a.paletteSections()
	query := strings.TrimSpace(a.paletteQuery)
	var visible []paletteItem
	for _, section := range sections {
		items := filterPaletteItems(section.Items, query)
		if len(items) == 0 {
			continue
		}
		visible = append(visible, paletteItem{Label: section.Label, Header: true})
		visible = append(visible, items...)
	}
	return visible
}

func (a *App) resetPaletteIndex() {
	items := a.paletteVisibleItems()
	a.paletteIndex = firstSelectableIndex(items)
}

func filterPaletteItems(items []paletteItem, query string) []paletteItem {
	query = strings.TrimSpace(query)
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

func firstSelectableIndex(items []paletteItem) int {
	for i, item := range items {
		if !item.Header {
			return i
		}
	}
	return 0
}

func nextSelectableIndex(items []paletteItem, start, delta int) int {
	if len(items) == 0 {
		return 0
	}
	idx := start
	for i := 0; i < len(items); i++ {
		idx += delta
		if idx < 0 {
			idx = len(items) - 1
		} else if idx >= len(items) {
			idx = 0
		}
		if !items[idx].Header {
			return idx
		}
	}
	return start
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
	items := []paletteItem{{Label: "Agent", Action: paletteActionSelectAgent, Value: "agent"}}
	for _, team := range a.teams {
		label := fmt.Sprintf("%s(Team)", team.Name)
		items = append(items, paletteItem{Label: label, Action: paletteActionSelectAgent, Value: team.Name})
	}
	return items
}

func (a App) teamPaletteItems() []paletteItem {
	items := make([]paletteItem, 0, len(a.teams))
	for _, team := range a.teams {
		label := fmt.Sprintf("%s(Team)", team.Name)
		items = append(items, paletteItem{Label: label, Action: paletteActionSelectAgent, Value: team.Name})
	}
	if len(items) == 0 {
		return []paletteItem{{Label: "No teams", Action: paletteActionSelectAgent, Value: ""}}
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
		if a.db != nil {
			count, err := a.db.CountPendingPromptsForSession(s.ID)
			if err == nil && count > 0 {
				if desc == "" {
					desc = fmt.Sprintf("🔔%d", count)
				} else {
					desc = fmt.Sprintf("%s • 🔔%d", desc, count)
				}
			}
		}
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
	if a.approvalInput.Active() {
		inputLines = maxInt(inputLines, countLines(a.approvalInput.View(innerW)))
	} else if a.askInput.Active() {
		inputLines = maxInt(inputLines, countLines(a.askInput.View(innerW)))
	}
	extraLines := 1 + a.inputCwdLineCount()
	inputAreaLines := inputLines + extraLines
	minInputArea := extraLines + 1
	if inputAreaLines < minInputArea {
		inputAreaLines = minInputArea
	}
	maxInputArea := maxInputLines + extraLines
	if inputAreaLines > maxInputArea {
		inputAreaLines = maxInputArea
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
		a.updateInputPrompt()
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
	a.updateInputPrompt()
}

func loadCommandsCmd(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		stop := diagnostics.Track("tui.db.list_commands", nil)
		commands, err := database.ListRecentCommands(200)
		stop()
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
		stop := diagnostics.Track("tui.db.list_sessions", nil)
		sessions, err := database.ListAgentSessions(200)
		stop()
		if err != nil {
			return sessionsErrorMsg{err: err}
		}
		return sessionsLoadedMsg{sessions: sessions}
	}
}

func loadSessionMessagesCmd(database *db.DB, sessionID string) tea.Cmd {
	return func() tea.Msg {
		stop := diagnostics.Track("tui.db.list_messages", nil)
		messages, err := database.ListAgentMessages(sessionID)
		stop()
		if err != nil {
			return sessionMessagesErrorMsg{err: err}
		}

		stop = diagnostics.Track("tui.db.list_pending_prompts", nil)
		pending, err := database.ListPendingPrompts(sessionID, 0)
		stop()
		if err != nil {
			return sessionMessagesErrorMsg{err: err}
		}
		return sessionMessagesLoadedMsg{sessionID: sessionID, messages: messages, pending: pending}
	}
}

func createSessionCmd(database *db.DB, cwd string) tea.Cmd {
	return func() tea.Msg {
		session, err := createSession(database, cwd)
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

func (a *App) enqueuePendingPrompt(content string, createdAt int64) error {
	if a.db == nil {
		return fmt.Errorf("database is nil")
	}
	sessionID := strings.TrimSpace(a.activeSessionID)
	if sessionID == "" {
		return fmt.Errorf("active session is empty")
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("content is empty")
	}
	promptID := generateID()
	prompt := &db.PendingPrompt{
		PromptID:  promptID,
		SessionID: sessionID,
		Content:   trimmed,
		CreatedAt: createdAt,
		Status:    db.PendingPromptStatusPending,
	}
	if err := a.db.CreatePendingPrompt(prompt); err != nil {
		return err
	}
	a.pendingPromptID = promptID
	a.pendingPromptSessionID = sessionID
	if err := a.updatePendingPromptCount(); err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to update pending prompt count", zap.Error(err))
		}
	}
	return nil
}

func (a *App) resolvePendingPrompt() {
	if a.pendingPromptID == "" {
		return
	}
	if a.db == nil {
		a.pendingPromptID = ""
		a.pendingPromptSessionID = ""
		return
	}
	if err := a.db.ResolvePendingPrompt(a.pendingPromptID); err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to resolve pending prompt", zap.Error(err))
		}
	}
	if err := a.updatePendingPromptCount(); err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to update pending prompt count", zap.Error(err))
		}
	}
	a.pendingPromptID = ""
	a.pendingPromptSessionID = ""
}

func (a *App) updatePendingPromptCount() error {
	if a.db == nil {
		return fmt.Errorf("database is nil")
	}
	return a.db.WritePendingPromptsCount(config.PendingPromptsCountPath())
}

func createSession(database *db.DB, cwd string) (db.AgentSession, error) {
	if database == nil {
		return db.AgentSession{}, fmt.Errorf("database is nil")
	}
	name := fmt.Sprintf("Session %s", time.Now().Format("2006-01-02 15:04"))
	session := db.AgentSession{
		ID:        generateID(),
		Name:      name,
		Cwd:       strings.TrimSpace(cwd),
		CreatedAt: time.Now().UnixNano(),
		UpdatedAt: time.Now().UnixNano(),
	}
	if err := database.CreateAgentSession(&session); err != nil {
		return db.AgentSession{}, err
	}
	return session, nil
}

func formatSessionMessages(messages []db.AgentMessage) []AgentMessage {
	if len(messages) == 0 {
		return nil
	}
	output := make([]AgentMessage, 0, len(messages))
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		output = append(output, AgentMessage{Role: msg.Role, Content: content})
	}
	return output
}

func buildInputHistory(messages []db.AgentMessage) []string {
	if len(messages) == 0 {
		return nil
	}
	entries := make([]string, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		value := strings.TrimSpace(msg.Content)
		if value == "" {
			continue
		}
		if len(entries) > 0 && entries[len(entries)-1] == value {
			continue
		}
		entries = append(entries, value)
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

func (a *App) updateInputPrompt() {
	a.input.SetPrompt(inputPrompt)
	promptCells := lipgloss.Width(a.dirPromptInput.Prompt)
	if a.leftContentW > 0 {
		a.dirPromptInput.Width = maxInt(a.leftContentW-promptCells, suggestedMinWidth)
		a.approvalInput.SetWidth(a.leftContentW)
		a.askInput.SetWidth(a.leftContentW)
	}
}

func (a App) inputCwdLineCount() int {
	if strings.TrimSpace(a.cwd) == "" {
		return 0
	}
	return 1
}

func (a App) renderInputCwdLine(contentWidth int) string {
	if strings.TrimSpace(a.cwd) == "" || contentWidth <= 0 {
		return ""
	}
	prefix := ""
	maxWidth := contentWidth
	if maxWidth < 1 {
		return metaStyle.Render(truncateToWidth(prefix, contentWidth))
	}
	display := formatCwdDisplay(a.cwd, maxWidth)
	line := prefix + display
	if lipgloss.Width(line) > contentWidth {
		line = truncateToWidth(line, contentWidth)
	}
	return metaStyle.Render(line)
}

func (a *App) setActiveSessionID(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	a.activeSessionID = sessionID
	if sessionID == "" {
		_ = os.Unsetenv("TERMIA_SESSION_ID")
		return
	}
	_ = os.Setenv("TERMIA_SESSION_ID", sessionID)
}

func (a *App) setCwd(cwd string) {
	if strings.TrimSpace(cwd) == "" {
		return
	}
	a.cwd = cwd
	a.recordSessionCwd(a.activeSessionID, cwd)
	a.updateInputPrompt()
	a.syncCwdToShell(cwd)
}

func (a *App) recordSessionCwd(sessionID, cwd string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	if strings.TrimSpace(cwd) == "" {
		return
	}
	if a.sessionCwds == nil {
		a.sessionCwds = make(map[string]string)
	}
	a.sessionCwds[sessionID] = cwd
	a.updateSessionCwd(sessionID, cwd)
	if a.db == nil {
		return
	}
	if err := a.db.UpdateAgentSessionCwd(sessionID, cwd, time.Now().UnixNano()); err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to update session cwd", zap.Error(err))
		}
	}
}

func (a *App) ensureSessionCwd(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	if a.sessionCwds == nil {
		a.sessionCwds = make(map[string]string)
	}
	if stored := strings.TrimSpace(a.sessionCwds[sessionID]); stored != "" {
		return
	}
	if strings.TrimSpace(a.cwd) == "" {
		return
	}
	a.recordSessionCwd(sessionID, a.cwd)
}

func (a *App) applySessionCwd(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	a.setActiveSessionID(sessionID)
	if a.sessionCwds == nil {
		a.sessionCwds = make(map[string]string)
	}
	stored, ok := a.sessionCwds[sessionID]
	if !ok || strings.TrimSpace(stored) == "" {
		stored = a.sessionCwdFromList(sessionID)
		if strings.TrimSpace(stored) == "" {
			a.recordSessionCwd(sessionID, a.cwd)
			return
		}
		a.sessionCwds[sessionID] = stored
	}
	if stored == a.cwd {
		a.syncCwdToShell(stored)
		return
	}
	if err := os.Chdir(stored); err != nil {
		a.statusMsg = fmt.Sprintf("Failed to switch directory: %v", err)
		return
	}
	a.setCwd(stored)
}

func (a *App) cacheSessionCwds(sessions []db.AgentSession) {
	if len(sessions) == 0 {
		return
	}
	if a.sessionCwds == nil {
		a.sessionCwds = make(map[string]string)
	}
	for _, session := range sessions {
		a.cacheSessionCwd(session)
	}
}

func (a *App) cacheSessionCwd(session db.AgentSession) {
	sessionID := strings.TrimSpace(session.ID)
	if sessionID == "" {
		return
	}
	stored := strings.TrimSpace(session.Cwd)
	if stored == "" {
		return
	}
	if a.sessionCwds == nil {
		a.sessionCwds = make(map[string]string)
	}
	if existing := strings.TrimSpace(a.sessionCwds[sessionID]); existing != "" {
		return
	}
	a.sessionCwds[sessionID] = stored
}

func (a *App) sessionCwdFromList(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	for _, session := range a.sessions {
		if session.ID != sessionID {
			continue
		}
		return strings.TrimSpace(session.Cwd)
	}
	return ""
}

func (a *App) updateSessionCwd(sessionID, cwd string) {
	if sessionID == "" {
		return
	}
	for i := range a.sessions {
		if a.sessions[i].ID != sessionID {
			continue
		}
		a.sessions[i].Cwd = cwd
		return
	}
}

func (a *App) syncCwdToShell(cwd string) {
	cdFile := strings.TrimSpace(os.Getenv("TERMIA_CD_FILE"))
	if cdFile == "" {
		if !a.cwdSyncWarned {
			a.statusMsg = "Shell sync inactive (run 'termia init')."
			a.cwdSyncWarned = true
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(cdFile), 0o700); err != nil {
		if !a.cwdSyncWarned {
			a.statusMsg = "Shell sync failed; using TUI-only CWD."
			a.cwdSyncWarned = true
		}
		return
	}
	if err := os.WriteFile(cdFile, []byte(cwd), 0o600); err != nil {
		if !a.cwdSyncWarned {
			a.statusMsg = "Shell sync failed; using TUI-only CWD."
			a.cwdSyncWarned = true
		}
		return
	}
}

func formatCwdPrompt(cwd string, contentWidth int) string {
	if strings.TrimSpace(cwd) == "" || contentWidth <= 0 {
		return inputPrompt
	}
	maxPromptWidth := contentWidth - suggestedMinWidth
	if maxPromptWidth < lipgloss.Width(inputPrompt) {
		return inputPrompt
	}
	suffix := " > "
	maxPathWidth := maxPromptWidth - lipgloss.Width(suffix)
	if maxPathWidth < 1 {
		return inputPrompt
	}
	display := formatCwdDisplay(cwd, maxPathWidth)
	return display + suffix
}

func formatCwdDisplay(cwd string, maxWidth int) string {
	display := tildePath(cwd)
	if maxWidth <= 0 {
		return display
	}
	if lipgloss.Width(display) <= maxWidth {
		return display
	}
	return truncateMiddle(display, maxWidth)
}

func tildePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	sep := string(os.PathSeparator)
	if strings.HasPrefix(path, home+sep) {
		return "~" + sep + strings.TrimPrefix(path, home+sep)
	}
	return path
}

func truncateMiddle(input string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(input) <= maxWidth {
		return input
	}
	ellipsis := "…"
	if maxWidth <= lipgloss.Width(ellipsis) {
		return truncate.String(input, uint(maxWidth))
	}
	leftWidth := (maxWidth - lipgloss.Width(ellipsis)) / 2
	rightWidth := maxWidth - lipgloss.Width(ellipsis) - leftWidth
	left := truncate.String(input, uint(leftWidth))
	right := tailByWidth(input, rightWidth)
	return left + ellipsis + right
}

func tailByWidth(input string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(input)
	cur := 0
	for i := len(runes) - 1; i >= 0; i-- {
		cur += lipgloss.Width(string(runes[i]))
		if cur >= width {
			return string(runes[i:])
		}
	}
	return input
}

func resolveDirPath(input, cwd string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("path is empty")
	}
	resolved := expandTilde(input)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(cwd, resolved)
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return resolved, nil
}

func completeDirPrompt(input, cwd string) (string, []string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "~" {
		return "~" + string(os.PathSeparator), []string{"~" + string(os.PathSeparator)}
	}
	dirPart, base := splitDirInput(input)
	searchDir := dirPart
	if searchDir == "" {
		searchDir = "."
	}
	searchDir = expandTilde(searchDir)
	if !filepath.IsAbs(searchDir) {
		searchDir = filepath.Join(cwd, searchDir)
	}
	searchDir = filepath.Clean(searchDir)
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return input, nil
	}
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, base) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	suggestions := make([]string, 0, len(matches))
	for _, match := range matches {
		suggestions = append(suggestions, match+string(os.PathSeparator))
	}
	if len(matches) == 0 {
		return input, suggestions
	}
	if len(matches) == 1 {
		return dirPart + matches[0] + string(os.PathSeparator), suggestions
	}
	prefix := commonPrefix(matches)
	if len(prefix) > len(base) {
		return dirPart + prefix, suggestions
	}
	return input, suggestions
}

func splitDirInput(input string) (string, string) {
	if input == "" {
		return "", ""
	}
	sep := string(os.PathSeparator)
	idx := strings.LastIndex(input, sep)
	if idx == -1 {
		return "", input
	}
	return input[:idx+1], input[idx+1:]
}

func commonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, value := range values[1:] {
		for !strings.HasPrefix(value, prefix) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func expandTilde(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			return home
		}
		return path
	}
	sep := string(os.PathSeparator)
	if strings.HasPrefix(path, "~"+sep) {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~"+sep))
		}
	}
	return path
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
