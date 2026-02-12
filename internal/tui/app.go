package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	statusMsg  string

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

type favoriteToggledMsg struct {
	id string
}

// New creates a new App model.
func New(database *db.DB, cfg *config.Config, logger *zap.Logger) App {
	keys := DefaultKeyMap()
	return App{
		db:         database,
		cfg:        cfg,
		logger:     logger,
		focus:      FocusInput, // Start with input focused (standard TUI pattern)
		middleMode: ModeAgent,  // Default to Agent view
		history:    NewHistoryModel(keys),
		preview:    NewPreviewModel(keys),
		agent:      NewAgentModel(keys),
		input:      NewInputModel(),
		keys:       keys,
	}
}

// Init loads the initial data asynchronously.
func (a App) Init() tea.Cmd {
	return tea.Batch(
		loadCommandsCmd(a.db),
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

	case tea.KeyMsg:
		// Global Keybindings
		if key.Matches(msg, a.keys.ForceQuit) {
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
	a.agent.AddMessage("(Agent integration pending — use `termia tai` for now)")

	a.input.Reset()
	return a, nil
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

	// Compose layout: history + content + input
	body := lipgloss.JoinVertical(lipgloss.Left, history, content, inputSection)

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
	if a.menuHeight > 0 {
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

// renderInput renders the input panel with border based on focus.
// Citation badge is rendered INSIDE the panel border to prevent clipping.
func (a App) renderInput(contentWidth int) string {
	// Build inner content: input view + always-on status line
	inputView := RenderInputSection(a.input)

	// Status line: cited count always present (reserve space for future right hints)
	citedCount := a.history.CitedCount()
	badge := citedBadgeStyle.Render(fmt.Sprintf(" 📎 %d commands referenced", citedCount))
	inner := inputView + "\n" + badge

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
	if a.statusMsg != "" {
		return statusBarStyle.Width(contentWidth).Render(a.statusMsg)
	}

	return statusBarStyle.Width(contentWidth).Render(
		"Tab: Focus | Enter: Select/Submit | Esc: Back | /: Commands",
	)
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
	available := totalInner - inputRendered
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
