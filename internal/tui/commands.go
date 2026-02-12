package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
)

// SlashCommandResult is a message returned after executing a slash command.
type SlashCommandResult struct {
	Output          string
	SwitchFocus     *Focus
	SwitchMode      *MiddleMode
	SwitchAgentMode *AgentMode
	Quit            bool // true means exit TUI
	Clear           bool // true means clear current view
}

// executeSlashCommand processes a slash command and returns a tea.Cmd.
func executeSlashCommand(cmd *SlashCommand, database *db.DB, cfg *config.LLMConfig) tea.Cmd {
	if cmd == nil {
		return nil
	}

	switch cmd.Name {
	case "help", "h":
		return func() tea.Msg {
			return SlashCommandResult{
				Output: renderHelpText(),
			}
		}

	case "search", "s":
		if cmd.Args == "" {
			return func() tea.Msg {
				return SlashCommandResult{Output: "Usage: /search <query>"}
			}
		}
		return searchCommandsCmd(database, cmd.Args)

	case "models", "model", "m":
		return func() tea.Msg {
			focus := FocusContent
			mode := ModeAgent
			return SlashCommandResult{
				Output:      renderModelsText(cfg),
				SwitchFocus: &focus,
				SwitchMode:  &mode,
			}
		}

	case "team":
		return func() tea.Msg {
			mode := AgentModeTeam
			return SlashCommandResult{
				Output:          "Agent mode set to team.",
				SwitchAgentMode: &mode,
			}
		}

	case "copilt":
		return func() tea.Msg {
			mode := AgentModeCopilot
			return SlashCommandResult{
				Output:          "Agent mode set to copilt.",
				SwitchAgentMode: &mode,
			}
		}

	case "clear", "c":
		return func() tea.Msg {
			return SlashCommandResult{Clear: true}
		}

	case "exit":
		return tea.Quit
	case "quit", "q":
		return func() tea.Msg {
			return SlashCommandResult{Output: "Use /exit to leave the TUI."}
		}

	default:
		return func() tea.Msg {
			return SlashCommandResult{
				Output: fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd.Name),
			}
		}
	}
}

// searchCommandsCmd fires a DB search and returns results as commandsLoadedMsg.
func searchCommandsCmd(database *db.DB, query string) tea.Cmd {
	return func() tea.Msg {
		commands, err := database.SearchCommands(query, 200)
		if err != nil {
			return commandsErrorMsg{err: err}
		}
		return commandsLoadedMsg{commands: commands}
	}
}

func renderHelpText() string {
	var b strings.Builder
	b.WriteString("Available Commands:\n")
	b.WriteString("─────────────────────────────\n")
	b.WriteString("  /help, /h          Show this help\n")
	b.WriteString("  /search <q>, /s    Search command history\n")
	b.WriteString("  /models, /m        Show LLM model config\n")
	b.WriteString("  /team              Switch agent mode to team\n")
	b.WriteString("  /copilt            Switch agent mode to copilt\n")
	b.WriteString("  /clear, /c         Clear current view\n")
	b.WriteString("  /exit              Exit TUI\n")
	b.WriteString("\n")
	b.WriteString("Keybindings:\n")
	b.WriteString("─────────────────────────────\n")
	b.WriteString("  Tab / Shift+Tab    Switch panels\n")
	b.WriteString("  j/k or arrows      Navigate history\n")
	b.WriteString("  Enter              Preview command output\n")
	b.WriteString("  d                  Delete selected command\n")
	b.WriteString("  f                  Toggle favorite\n")
	b.WriteString("  g / G              Jump to top/bottom\n")
	b.WriteString("  exit               Exit TUI\n")
	return b.String()
}

func renderModelsText(cfg *config.LLMConfig) string {
	var b strings.Builder
	b.WriteString("LLM Model Configuration:\n")
	b.WriteString("─────────────────────────────\n")
	b.WriteString(fmt.Sprintf("  Default Provider: %s\n\n", cfg.DefaultProvider))

	providers := []struct {
		name string
		cfg  config.LLMProviderConfig
	}{
		{"OpenAI", cfg.OpenAI},
		{"Anthropic", cfg.Anthropic},
		{"DeepSeek", cfg.DeepSeek},
		{"Ollama", cfg.Ollama},
	}

	for _, p := range providers {
		model := p.cfg.Model
		if model == "" {
			model = "(not set)"
		}
		keyEnv := p.cfg.APIKeyEnv
		if keyEnv == "" {
			keyEnv = "(not set)"
		}
		b.WriteString(fmt.Sprintf("  %s:\n", p.name))
		b.WriteString(fmt.Sprintf("    Model:   %s\n", model))
		b.WriteString(fmt.Sprintf("    API Key: %s\n", keyEnv))
		if p.cfg.BaseURL != "" {
			b.WriteString(fmt.Sprintf("    URL:     %s\n", p.cfg.BaseURL))
		}
		b.WriteString("\n")
	}

	b.WriteString("Use `termia config edit` to modify settings.\n")
	return b.String()
}
