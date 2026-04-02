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
	b.WriteString(fmt.Sprintf("  Default Provider: %s\n\n", cfg.ProviderDisplayName(cfg.DefaultProvider)))

	for _, provider := range cfg.ModelProviders() {
		providerCfg := provider.Config
		model := providerCfg.Model
		if model == "" {
			model = "(not set)"
		}
		keyState := "(not set)"
		if providerCfg.ResolvedAPIKey() != "" {
			keyState = "configured"
		}
		b.WriteString(fmt.Sprintf("  %s:\n", provider.DisplayName))
		b.WriteString(fmt.Sprintf("    Model:   %s\n", model))
		b.WriteString(fmt.Sprintf("    API Key: %s\n", keyState))
		if providerCfg.BaseURL != "" {
			b.WriteString(fmt.Sprintf("    URL:     %s\n", providerCfg.BaseURL))
		}
		b.WriteString("\n")
	}

	b.WriteString("Use `termia config edit` to modify settings.\n")
	return b.String()
}
