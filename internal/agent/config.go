package agent

import (
	"fmt"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/modelspec"
)

const (
	defaultAssistantName = "assistant"
	defaultUserID        = "user"
	defaultAppName       = "termia"
)

const DefaultAssistantInstruction = `You are Termia, an AI terminal assistant.

Operate with a terminal-first mindset:
- Use tools instead of inventing files, output, or shell results.
- Use request_input when you need a decision, choice, or clarification from the user.
- Use read_file, read_files, list_dir, and stream_read when file or log context matters.
- The command tool follows the session working directory by default. Only set cwd_mode=override with cwd when the user explicitly wants a one-off command in a different directory.
- When a user message references terminal commands, treat the attached command metadata as the source of truth and use inspect_command_output if you need the actual command output.
- If a tool confirmation is rejected, say the user declined the action. Do not describe it as a technical failure or an execution issue.
- Be concise, direct, and execution-aware.`

const DefaultCoordinatorInstruction = `You are the fixed coordinator for a Termia team.

Decide whether to answer directly or delegate to the most suitable team member.
When the task needs terminal or file interaction, prefer tools over speculation.
The command tool follows the session working directory by default. Only set cwd_mode=override with cwd when the user explicitly wants a one-off command in a different directory.
When a user message includes terminal command metadata, inspect outputs through inspect_command_output instead of assuming from the command text alone.
If a tool confirmation is rejected, say the user declined the action. Do not describe it as a technical failure or an execution issue.
Keep delegation focused and avoid bouncing the same task across multiple agents.`

func AssistantSpecFromConfig(cfg *config.Config) (AgentSpec, error) {
	if cfg == nil {
		return AgentSpec{}, fmt.Errorf("config is nil")
	}

	modelSpec, err := DefaultModelSpecFromConfig(&cfg.LLM)
	if err != nil {
		return AgentSpec{}, err
	}

	return AgentSpec{
		Name:        defaultAssistantName,
		Description: "General purpose terminal assistant",
		Instruction: DefaultAssistantInstruction,
		Model:       modelSpec,
		Tools:       defaultToolNames(),
	}, nil
}

func DefaultModelSpecFromConfig(llmCfg *config.LLMConfig) (ModelSpec, error) {
	spec, err := modelspec.DefaultFromConfig(llmCfg)
	if err != nil {
		return ModelSpec{}, err
	}
	return spec, nil
}

func defaultToolNames() []string {
	return []string{
		"command",
		"inspect_command_output",
		"request_input",
		"read_file",
		"read_files",
		"list_dir",
		"stream_read",
	}
}
