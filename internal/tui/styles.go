package tui

import "github.com/charmbracelet/lipgloss"

// Color palette — adaptive for light/dark terminals.
var (
	// Primary accent
	colorPrimary = lipgloss.AdaptiveColor{Light: "#5B44E8", Dark: "#7C6BFF"}
	// Secondary accent
	colorSecondary = lipgloss.AdaptiveColor{Light: "#1A8CFF", Dark: "#58A6FF"}
	// Success (green)
	colorSuccess = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	// Error (red)
	colorError = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	// Warning (yellow)
	colorWarning = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	// Muted text
	colorMuted = lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"}
	// Subtle (borders, separators)
	colorSubtle = lipgloss.AdaptiveColor{Light: "#D0D7DE", Dark: "#30363D"}
	// Surface (panel backgrounds)
	colorSurface = lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#0D1117"}
	// On surface (text on panels)
	colorOnSurface = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6EDF3"}
	// Strong emphasis text
	colorStrong = lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#F0F6FC"}
	// Highlight (selected row bg)
	colorHighlight = lipgloss.AdaptiveColor{Light: "#DDF4FF", Dark: "#161B22"}
)

// Layout styles.
var (
	// Main container — intentionally borderless so the panes can sit flush
	// against the terminal edges.
	containerStyle = lipgloss.NewStyle()

	// Panel styles (replacing tabs).
	activePaneBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
	}

	inactivePaneBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
	}

	// Top panel (History) - 1/4 height
	historyPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSubtle).
				Padding(0, 2)

	focusedHistoryPaneStyle = historyPaneStyle.Copy().
				BorderForeground(colorPrimary)

	// Middle panel (Content/Agent) - 5/8 height
	contentPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSubtle).
				Padding(0, 2)

	focusedContentPaneStyle = contentPaneStyle.Copy().
				BorderForeground(colorPrimary)

	// Input bar at bottom.
	inputBarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSubtle).
			Padding(0, 2)

	focusedInputBarStyle = inputBarStyle.Copy().
				BorderForeground(colorPrimary)

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	// Status bar at very bottom (inside container or outside?).
	// If we use 3 blocks, status might be part of bottom or separate.
	statusBarStyle = lipgloss.NewStyle().
			Padding(0, 1)
)

// Content element styles.
var (
	// Command list items.
	selectedRowStyle = lipgloss.NewStyle().
				Background(colorHighlight).
				Foreground(colorOnSurface).
				Bold(true)

	selectedSlashRowStyle = lipgloss.NewStyle().
				Background(colorHighlight).
				Bold(true)

	normalRowStyle = lipgloss.NewStyle().
			Foreground(colorOnSurface)

	// Exit code badges.
	exitOKStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	exitErrStyle = lipgloss.NewStyle().
			Foreground(colorError)

	// Favorite star.
	favoriteStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	// Timestamps, metadata.
	metaStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	metadataLabelStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Bold(true)

	thinkLowStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	thinkMediumStyle = lipgloss.NewStyle().
				Foreground(colorWarning)

	thinkHighStyle = lipgloss.NewStyle().
			Foreground(colorError)

	// CWD path.
	cwdStyle = lipgloss.NewStyle().
			Foreground(colorSecondary)

	// Header/title within panels.
	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorOnSurface).
			Padding(1, 2)

	paletteHeaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorOnSurface)

	paletteSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorMuted)

	// Preview header with command info.
	previewHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary).
				Padding(1, 2).
				Border(lipgloss.Border{Bottom: "─"}, false, false, true, false).
				BorderForeground(colorSubtle)

	// Empty state text.
	emptyStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Padding(1, 2)

	// Slash command menu.
	slashMenuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSubtle).
			BorderBottom(false).
			Padding(0, 1)

	commandPaletteStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSubtle).
				Padding(0, 1)

	// Loading indicator.
	loadingStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	// Cited command badge in input bar.
	citedBadgeStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	// Agent mode badges in input bar.
	teamModeStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	agentModeStyle = lipgloss.NewStyle().
			Foreground(colorWarning).
			Bold(true)

	// Cited command marker in history rows.
	citedMarkerStyle = lipgloss.NewStyle().
				Foreground(colorSecondary)

	roleHeaderStyle = lipgloss.NewStyle().
			Foreground(colorOnSurface).
			Bold(true)
	roleSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorSubtle)

	conversationDividerStyle = lipgloss.NewStyle().
					Foreground(colorSubtle)
	userPromptPrefixStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)
	assistantBulletPrefixStyle = lipgloss.NewStyle().
					Foreground(colorOnSurface).
					Bold(true)
	systemBulletPrefixStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)
	errorBulletPrefixStyle = lipgloss.NewStyle().
				Foreground(colorError).
				Bold(true)
	userBodyStyle = lipgloss.NewStyle().
			Foreground(colorOnSurface)
	assistantBodyStyle = lipgloss.NewStyle().
				Foreground(colorOnSurface)
	systemBodyStyle = lipgloss.NewStyle().
			Foreground(colorWarning)
	errorBodyStyle = lipgloss.NewStyle().
			Foreground(colorError)
	toolPendingStyle = lipgloss.NewStyle().
				Foreground(colorMuted)
	toolSuccessStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(colorError)
	strongTextStyle = lipgloss.NewStyle().
			Foreground(colorStrong)
	inlineCodeStyle = lipgloss.NewStyle().
			Foreground(colorSecondary)
	codeBlockStyle = lipgloss.NewStyle().
			Foreground(colorSecondary)
	quotePrefixStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)
	quoteTextStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)
	listBulletStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)
	listNumberStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)
	markdownHeading1Style = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)
	markdownHeading2Style = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)
	markdownHeading3Style = lipgloss.NewStyle().
				Foreground(colorOnSurface).
				Bold(true)
	hitlTitleStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)
	hitlSubtitleStyle = lipgloss.NewStyle().
				Foreground(colorMuted)
	hitlCommandStyle = lipgloss.NewStyle().
				Foreground(colorSecondary)
	hitlHintStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
	hitlChoiceTitleStyle = lipgloss.NewStyle().
				Foreground(colorOnSurface).
				Bold(true)
	hitlChoiceDescStyle = lipgloss.NewStyle().
				Foreground(colorMuted)
	hitlChoiceFocusStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)
	hitlSelectedStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)
	hitlChoiceMarkerStyle = lipgloss.NewStyle().
				Foreground(colorMuted)
	hitlChoiceFocusMarkerStyle = lipgloss.NewStyle().
					Foreground(colorSecondary).
					Bold(true)
	hitlChoiceSelectedMarkerStyle = lipgloss.NewStyle().
					Foreground(colorPrimary).
					Bold(true)
	hitlActionStyle = lipgloss.NewStyle().
			Foreground(colorOnSurface).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSubtle).
			Padding(0, 1)
	hitlActionFocusedStyle = hitlActionStyle.Copy().
				Foreground(colorPrimary).
				BorderForeground(colorPrimary).
				Background(colorHighlight).
				Bold(true)
	hitlActionDangerStyle = hitlActionStyle.Copy().
				Foreground(colorError)
	hitlActionDangerFocusedStyle = hitlActionFocusedStyle.Copy().
					Foreground(colorError)
	hitlActionKeyStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Bold(true)

	dirPromptErrorStyle = lipgloss.NewStyle().
				Foreground(colorError)

	pastePlaceholderStyle = lipgloss.NewStyle().
				Foreground(colorWarning)

	inputCursorStyle = lipgloss.NewStyle().
				Reverse(true)
)
