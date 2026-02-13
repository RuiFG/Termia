package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings for the TUI.
type KeyMap struct {
	// Navigation
	Up   key.Binding
	Down key.Binding

	// Tab switching
	NextTab key.Binding
	PrevTab key.Binding

	// Actions
	Enter    key.Binding
	Delete   key.Binding
	Favorite key.Binding
	Copy     key.Binding
	Cite     key.Binding // Space to cite/reference a command in history

	// Modes
	Search   key.Binding
	Slash    key.Binding
	Palette  key.Binding
	Variants key.Binding

	// Quit
	Quit      key.Binding
	ForceQuit key.Binding
	Back      key.Binding

	// Scrolling (preview/agent)
	PageUp   key.Binding
	PageDown key.Binding
	HalfUp   key.Binding
	HalfDown key.Binding
	GotoTop  key.Binding
	GotoEnd  key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Favorite: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "favorite"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy"),
		),
		Cite: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "cite"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Slash: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "command"),
		),
		Palette: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "commands"),
		),
		Variants: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "variants"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", ""),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", ""),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdown", "page down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "half page up"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "half page down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "top"),
		),
		GotoEnd: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
	}
}

// HistoryHelp returns short help text for history mode.
func HistoryHelp() string {
	return "↑↓: navigate | enter: detail | space: cite | d: delete | f: fav | tab: switch"
}

// PreviewHelp returns short help text for preview mode.
func PreviewHelp() string {
	return "↑↓: scroll | pgup/pgdn: page | esc: back | tab: switch"
}

// AgentHelp returns short help text for agent mode.
func AgentHelp() string {
	return "type a message | /help: commands | tab: switch | esc: back"
}
