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
	OpenPalette     *paletteStage
	CreateSession   bool
	Quit            bool // true means exit TUI
}

// executeSlashCommand processes a slash command and returns a tea.Cmd.
func executeSlashCommand(cmd *SlashCommand, database *db.DB, cfg *config.LLMConfig) tea.Cmd {
	if cmd == nil {
		return nil
	}
	_ = database
	_ = cfg

	switch cmd.Name {
	case "exit":
		return func() tea.Msg {
			return SlashCommandResult{Quit: true}
		}
	case "ralph-loop":
		return func() tea.Msg {
			return SlashCommandResult{Output: "Ralph loop started (placeholder)."}
		}
	default:
		return func() tea.Msg {
			return SlashCommandResult{
				Output: fmt.Sprintf("Unknown command: /%s", cmd.Name),
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
