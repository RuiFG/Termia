package agent

import (
	"context"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

// Runner orchestrates model interactions for Termia using Eino.
type Runner struct {
	model        einomodel.ToolCallingChatModel
	db           *db.DB
	logger       *zap.Logger
	systemPrompt string
	agentCfg     *AgentConfig
}

// NewRunner constructs a Runner backed by Eino.
func NewRunner(m einomodel.ToolCallingChatModel, database *db.DB, logger *zap.Logger, cfg *AgentConfig) *Runner {
	return &Runner{
		model:        m,
		db:           database,
		logger:       logger,
		systemPrompt: DefaultSystemPrompt,
		agentCfg:     cfg,
	}
}

// RunAnalysis runs a quick analysis for "tai <question>".
// Returns a channel of content string chunks for streaming output.
func (r *Runner) RunAnalysis(ctx context.Context, commands []db.Command, userQuery string) (<-chan string, error) {
	if r.model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	commandContext := ""
	if len(commands) > 0 {
		commandContext = formatCommandsWithOutput(r.db, commands)
		if strings.TrimSpace(commandContext) == "" {
			commandContext = formatCommandsForLLM(commands)
		}
	}
	userContent := fmt.Sprintf("User question: %s", userQuery)
	if strings.TrimSpace(commandContext) != "" {
		userContent = fmt.Sprintf("%s\n\nRecent commands:\n%s", userContent, commandContext)
	}
	messages := []*schema.Message{
		schema.SystemMessage(DefaultSystemPrompt),
		schema.UserMessage(userContent),
	}

	stream, err := r.model.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("run analysis: %w", err)
	}

	output := make(chan string, 16)
	go func() {
		defer close(output)
		defer stream.Close()

		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			if msg == nil {
				continue
			}
			if msg.Content != "" {
				output <- msg.Content
			}
		}
	}()

	return output, nil
}

// RunAgent executes a lightweight agent loop with tool calls.
func (r *Runner) RunAgent(ctx context.Context, tools []tool.BaseTool, userQuery string, commands []db.Command) (<-chan string, error) {
	if r.model == nil {
		return nil, fmt.Errorf("model is nil")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	commandContext := formatCommandsWithOutput(r.db, commands)
	if strings.TrimSpace(commandContext) == "" {
		commandContext = formatCommandsForLLM(commands)
	}

	userContent := userQuery
	if strings.TrimSpace(commandContext) != "" {
		userContent = fmt.Sprintf("%s\n\nRecent commands:\n%s", userQuery, commandContext)
	}

	history := []*schema.Message{
		schema.SystemMessage(DefaultSystemPrompt + "\nYou may call the `command` tool when needed to run terminal commands. Do not assume execution; wait for tool results."),
		schema.UserMessage(userContent),
	}

	toolsByName := map[string]tool.BaseTool{}
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("tool info: %w", err)
		}
		if info != nil && info.Name != "" {
			toolsByName[info.Name] = t
		}
	}

	output := make(chan string, 16)
	go func() {
		defer close(output)
		for steps := 0; steps < 6; steps++ {
			stream, err := r.model.Stream(ctx, history)
			if err != nil {
				output <- fmt.Sprintf("\nError: %v\n", err)
				return
			}
			chunks := make([]*schema.Message, 0, 64)
			for {
				msg, err := stream.Recv()
				if err != nil {
					break
				}
				if msg == nil {
					continue
				}
				chunks = append(chunks, msg)
				if msg.Content != "" {
					output <- msg.Content
				}
			}
			stream.Close()

			assistantMsg, err := schema.ConcatMessages(chunks)
			if err != nil {
				output <- fmt.Sprintf("\nError: %v\n", err)
				return
			}
			if assistantMsg == nil {
				return
			}
			history = append(history, assistantMsg)

			if len(assistantMsg.ToolCalls) == 0 {
				return
			}

			for _, call := range assistantMsg.ToolCalls {
				toolName := call.Function.Name
				toolImpl, ok := toolsByName[toolName]
				if !ok {
					history = append(history, schema.ToolMessage("unknown tool: "+toolName, call.ID, schema.WithToolName(toolName)))
					continue
				}
				invokable, ok := toolImpl.(tool.InvokableTool)
				if !ok {
					history = append(history, schema.ToolMessage("tool does not support invocation", call.ID, schema.WithToolName(toolName)))
					continue
				}
				result, err := invokable.InvokableRun(ctx, call.Function.Arguments)
				if err != nil {
					result = fmt.Sprintf("tool error: %v", err)
				}
				history = append(history, schema.ToolMessage(result, call.ID, schema.WithToolName(toolName)))
			}
		}
	}()

	return output, nil
}

// Close cleans up runner resources.
func (r *Runner) Close() {
	// No-op for Eino model.
}
