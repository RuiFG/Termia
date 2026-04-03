package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func (r *Runtime) Run(ctx context.Context, req RunRequest) (<-chan RuntimeEvent, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		req.SessionID = newSessionID()
	}

	state := newRuntimeState()
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		state.set(commandStateCwdKey, cwd)
	}

	root, err := r.buildRootAgent(ctx, req, state)
	if err != nil {
		return nil, err
	}

	runr := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           root,
		EnableStreaming: true,
		CheckPointStore: newMemoryCheckPointStore(),
	})

	output := make(chan RuntimeEvent, 32)
	go func() {
		defer close(output)
		_ = r.runConversation(ctx, runr, req, state, output)
	}()
	return output, nil
}

func (r *Runtime) runConversation(ctx context.Context, runr *adk.Runner, req RunRequest, state *runtimeState, output chan<- RuntimeEvent) error {
	history := make([]adk.Message, 0, 16)
	next := buildPromptContent(req)
	maxLines, wait := streamDefaults(req)

	if req.StreamReader != nil {
		chunk, err := req.StreamReader.NextChunk(ctx, maxLines, wait)
		if err != nil && err != io.EOF {
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}
		if chunk.Text != "" {
			next = buildStreamPromptContent(req, chunk, true)
		}
	}

	turnIndex := 0
	for next != nil {
		checkPointID := fmt.Sprintf("%s-turn-%d", req.SessionID, turnIndex)
		turnMessages, err := r.runTurn(ctx, runr, checkPointID, state.snapshot(), history, next, output)
		if err != nil {
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}

		history = append(history, next)
		history = append(history, turnMessages...)
		turnIndex++

		if req.StreamReader == nil {
			next = nil
			continue
		}

		chunk, err := req.StreamReader.NextChunk(ctx, maxLines, wait)
		if err != nil {
			if err == io.EOF {
				if !req.StreamReader.CloseMessage() {
					next = nil
					continue
				}
				next = schema.UserMessage("The stream source has reached EOF. Provide a concise final assessment if needed.")
				req.StreamReader = nil
				continue
			}
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}
		if chunk.Text == "" {
			next = nil
			continue
		}
		next = buildStreamPromptContent(req, chunk, false)
	}

	return nil
}

func (r *Runtime) runTurn(
	ctx context.Context,
	runr *adk.Runner,
	checkPointID string,
	sessionValues map[string]any,
	history []adk.Message,
	input *schema.Message,
	output chan<- RuntimeEvent,
) ([]adk.Message, error) {
	seenToolCalls := make(map[string]bool)
	seenToolResults := make(map[string]bool)
	turnMessages := make([]adk.Message, 0, 16)

	messages := make([]adk.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, input)

	iter := runr.Run(
		ctx,
		messages,
		adk.WithCheckPointID(checkPointID),
		adk.WithSessionValues(sessionValues),
	)

	for {
		request, err := consumeRuntimeIterator(iter, output, seenToolCalls, seenToolResults, &turnMessages)
		if err != nil {
			return nil, err
		}
		if request == nil {
			return turnMessages, nil
		}
		if r.responder == nil {
			return nil, fmt.Errorf("hitl required for tool %s but no responder is configured", request.OriginalTool)
		}
		if strings.TrimSpace(request.ID) == "" {
			return nil, fmt.Errorf("hitl request is missing resume target id")
		}

		response, err := r.responder.Handle(ctx, *request)
		if err != nil {
			return nil, err
		}

		iter, err = runr.ResumeWithParams(ctx, checkPointID, &adk.ResumeParams{
			Targets: map[string]any{
				request.ID: hitlResumeData{
					Confirmed: response.Confirmed,
					Answers:   response.Answers,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("resume interrupted run: %w", err)
		}
	}
}

func (r *Runtime) buildRootAgent(ctx context.Context, req RunRequest, state *runtimeState) (adk.Agent, error) {
	registry, err := NewToolRegistry(r.db, state, r.requireCommandConfirmation())
	if err != nil {
		return nil, err
	}

	mode := req.Mode
	if mode == "" && r.cfg != nil {
		mode = Mode(strings.ToLower(strings.TrimSpace(r.cfg.Agent.DefaultMode)))
	}
	if mode == "" {
		mode = ModeAssistant
	}

	switch mode {
	case ModeTeam:
		return r.buildTeamAgent(ctx, req.TeamName, registry)
	default:
		return r.buildAssistantAgent(ctx, registry)
	}
}

func (r *Runtime) requireCommandConfirmation() bool {
	if r.cfg == nil {
		return true
	}
	return r.cfg.Agent.RequireCommandConfirmation
}

func (r *Runtime) buildAssistantAgent(ctx context.Context, registry *ToolRegistry) (adk.Agent, error) {
	spec, err := AssistantSpecFromConfig(r.cfg)
	if err != nil {
		return nil, err
	}
	return r.buildChatModelAgent(ctx, spec, registry)
}

func (r *Runtime) buildTeamAgent(ctx context.Context, teamName string, registry *ToolRegistry) (adk.Agent, error) {
	if strings.TrimSpace(teamName) == "" && r.cfg != nil {
		teamName = r.cfg.Agent.DefaultTeam
	}
	if strings.TrimSpace(teamName) == "" {
		return nil, fmt.Errorf("team mode requires a team name or agent.default_team")
	}

	spec, err := LoadTeamByName(r.cfg, teamName)
	if err != nil {
		return nil, err
	}

	subAgents := make([]adk.Agent, 0, len(spec.Agents))
	for _, member := range spec.Agents {
		subAgent, err := r.buildChatModelAgent(ctx, member, registry)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member.Name, err)
		}
		subAgents = append(subAgents, subAgent)
	}

	model, err := NewModel(ctx, spec.Coordinator.Model)
	if err != nil {
		return nil, fmt.Errorf("coordinator: %w", err)
	}

	instruction := strings.TrimSpace(spec.Coordinator.Instruction)
	if instruction == "" {
		instruction = DefaultCoordinatorInstruction
	}
	description := strings.TrimSpace(spec.Coordinator.Description)
	if description == "" {
		description = spec.Coordinator.Name
	}

	return deep.New(ctx, &deep.Config{
		Name:                   spec.Coordinator.Name,
		Description:            description,
		ChatModel:              model,
		Instruction:            instruction,
		SubAgents:              subAgents,
		ToolsConfig:            buildToolsConfig(registry.Filter(spec.Coordinator.Tools)),
		WithoutGeneralSubAgent: len(subAgents) > 0,
	})
}

func (r *Runtime) buildChatModelAgent(ctx context.Context, spec AgentSpec, registry *ToolRegistry) (adk.Agent, error) {
	model, err := NewModel(ctx, spec.Model)
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(spec.Description)
	if description == "" {
		description = spec.Name
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        spec.Name,
		Description: description,
		Instruction: spec.Instruction,
		Model:       model,
		ToolsConfig: buildToolsConfig(registry.Filter(spec.Tools)),
	})
}

func buildToolsConfig(tools []tool.BaseTool) adk.ToolsConfig {
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools:               tools,
			ExecuteSequentially: true,
		},
		EmitInternalEvents: true,
	}
}

func streamDefaults(req RunRequest) (int, time.Duration) {
	maxLines := req.StreamChunkLines
	if maxLines <= 0 {
		maxLines = 120
	}
	wait := req.StreamChunkWait
	if wait <= 0 {
		wait = 3 * time.Second
	}
	return maxLines, wait
}

func newSessionID() string {
	n := atomic.AddUint64(&sessionCounter, 1)
	return fmt.Sprintf("session-%d-%d", time.Now().UnixNano(), n)
}

var sessionCounter uint64
