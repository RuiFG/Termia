package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

// ReactRunner executes a single ReAct agent loop with tools.
type ReactRunner struct {
	agent        *react.Agent
	db           *db.DB
	logger       *zap.Logger
	systemPrompt string
}

// NewReactRunner constructs a ReAct-based runner.
func NewReactRunner(ctx context.Context, model model.ToolCallingChatModel, tools []tool.BaseTool, database *db.DB, logger *zap.Logger) (*ReactRunner, error) {
	if model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MaxStep: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("create react agent: %w", err)
	}

	return &ReactRunner{
		agent:        reactAgent,
		db:           database,
		logger:       logger,
		systemPrompt: DefaultSystemPrompt,
	}, nil
}

// Run executes the ReAct agent and streams assistant output.
func (r *ReactRunner) Run(ctx context.Context, userQuery string, commands []db.Command) (<-chan string, error) {
	if r.agent == nil {
		return nil, fmt.Errorf("react agent is nil")
	}
	if strings.TrimSpace(userQuery) == "" {
		return nil, fmt.Errorf("user query is empty")
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

	messages := []*schema.Message{
		schema.SystemMessage(r.systemPrompt + "\nYou may call the `command` tool when needed to run terminal commands. Do not assume execution; wait for tool results."),
		schema.UserMessage(userContent),
	}

	stream, err := r.agent.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("react stream: %w", err)
	}

	output := make(chan string, 16)
	go func() {
		defer close(output)
		defer stream.Close()

		for {
			msg, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					output <- fmt.Sprintf("\nError: %v\n", err)
				}
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
