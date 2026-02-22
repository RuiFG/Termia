package team

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

type TeamRunner struct {
	teamAgent   adk.Agent
	runner      *adk.Runner
	logger      *zap.Logger
	db          *db.DB
	models      map[string]string
	executionID string
	sequence    uint64
}

func NewTeamRunner(ctx context.Context, cfg *config.Config, database *db.DB, logger *zap.Logger) (*TeamRunner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	roles, err := LoadRoles(cfg.Agent)
	if err != nil {
		return nil, err
	}

	tools := agent.CreateTools(database, cfg.Agent.RequireApproval, nil)
	toolMap, err := indexTools(ctx, tools)
	if err != nil {
		return nil, err
	}

	agents := make(map[string]adk.Agent)
	models := make(map[string]string)
	for name, role := range roles {
		modelCfg, err := resolveRoleModelConfig(cfg, role)
		if err != nil {
			return nil, fmt.Errorf("role %s model: %w", name, err)
		}
		model, err := agent.NewModel(modelCfg)
		if err != nil {
			return nil, fmt.Errorf("role %s model init: %w", name, err)
		}
		roleTools, err := filterTools(role.ToolsAllowlist, toolMap)
		if err != nil {
			return nil, fmt.Errorf("role %s tools: %w", name, err)
		}

		roleInstruction := buildRoleInstruction(role)
		if role.Name == "coordinator" {
			roleInstruction = buildSupervisorInstruction(role, roles)
		}
		roleAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
			Name:          role.Name,
			Description:   role.Description,
			Instruction:   roleInstruction,
			Model:         model,
			ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: roleTools}},
			MaxIterations: role.MaxSteps,
			Exit:          &adk.ExitTool{},
		})
		if err != nil {
			return nil, fmt.Errorf("create agent %s: %w", name, err)
		}
		agents[name] = roleAgent
		models[name] = modelCfg.Model
	}

	supervisorAgent, subAgents, err := buildSupervisor(ctx, roles, agents)
	if err != nil {
		return nil, err
	}

	team, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: supervisorAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		return nil, err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: team, EnableStreaming: true})

	return &TeamRunner{
		teamAgent:   team,
		runner:      runner,
		logger:      logger,
		db:          database,
		models:      models,
		executionID: generateID(),
	}, nil
}

func (r *TeamRunner) Run(ctx context.Context, userQuery string, commands []db.Command) (<-chan string, error) {
	if r.runner == nil {
		return nil, fmt.Errorf("team runner is nil")
	}
	if strings.TrimSpace(userQuery) == "" {
		return nil, fmt.Errorf("user query is empty")
	}

	userContent := buildUserContent(userQuery, commands)
	iter := r.runner.Run(ctx, []adk.Message{schema.UserMessage(userContent)})
	output := make(chan string, 16)
	r.logEvent("run_start", "coordinator", "", userQuery)

	go func() {
		defer close(output)
		for {
			event, ok := iter.Next()
			if !ok {
				r.logEvent("run_end", "coordinator", "", "")
				return
			}
			if event == nil {
				continue
			}
			if event.Err != nil {
				r.logEvent("error", event.AgentName, "", event.Err.Error())
				output <- fmt.Sprintf("\nError: %v\n", event.Err)
				return
			}
			if event.Action != nil && event.Action.TransferToAgent != nil {
				payload := fmt.Sprintf("transfer_to:%s", event.Action.TransferToAgent.DestAgentName)
				r.logEvent("transfer", event.AgentName, "", payload)
			}
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}
			mv := event.Output.MessageOutput
			if mv.IsStreaming {
				r.handleStream(output, event.AgentName, mv)
				continue
			}
			msg := mv.Message
			if msg == nil {
				continue
			}
			content := msg.Content
			r.logMessageEvent(event.AgentName, mv, content)
			if mv.Role == schema.Assistant && strings.TrimSpace(content) != "" {
				output <- content
			}
		}
	}()

	return output, nil
}

func (r *TeamRunner) handleStream(output chan<- string, agentName string, mv *adk.MessageVariant) {
	for {
		msg, err := mv.MessageStream.Recv()
		if err != nil {
			if err == io.EOF {
				return
			}
			r.logEvent("error", agentName, "", err.Error())
			return
		}
		if msg == nil {
			continue
		}
		content := msg.Content
		r.logMessageEvent(agentName, mv, content)
		if mv.Role == schema.Assistant && strings.TrimSpace(content) != "" {
			output <- content
		}
	}
}

func (r *TeamRunner) logMessageEvent(agentName string, mv *adk.MessageVariant, content string) {
	if mv.Role == schema.Tool {
		r.logEvent("tool", agentName, mv.ToolName, content)
		return
	}
	r.logEvent("assistant", agentName, "", content)
}

func (r *TeamRunner) logEvent(eventType, agentName, toolName, payload string) {
	if r.db == nil {
		return
	}
	var toolNamePtr *string
	if strings.TrimSpace(toolName) != "" {
		toolNamePtr = &toolName
	}
	model := r.models[agentName]
	if model == "" {
		model = "unknown"
	}
	payloadJSON := ""
	if strings.TrimSpace(payload) != "" {
		data, err := json.Marshal(payload)
		if err == nil {
			payloadJSON = string(data)
		}
	}

	seq := atomic.AddUint64(&r.sequence, 1)
	traceID := fmt.Sprintf("%s:%d", r.executionID, seq)
	traceIDPtr := &traceID

	latency := int64(0)
	latencyPtr := &latency

	_ = r.db.CreateAgentEvent(&db.AgentEvent{
		ID:          generateID(),
		ExecutionID: r.executionID,
		AgentName:   agentName,
		EventType:   eventType,
		PayloadJSON: payloadJSON,
		Model:       model,
		ToolName:    toolNamePtr,
		LatencyMs:   latencyPtr,
		Ts:          time.Now().UnixNano(),
		TraceID:     traceIDPtr,
	})
}

func buildSupervisor(ctx context.Context, roles map[string]Role, agents map[string]adk.Agent) (adk.Agent, []adk.Agent, error) {
	supervisorRole, ok := roles["coordinator"]
	if !ok {
		for _, role := range roles {
			supervisorRole = role
			break
		}
	}
	if supervisorRole.Name == "" {
		return nil, nil, fmt.Errorf("no supervisor role available")
	}

	supervisorAgent := agents[supervisorRole.Name]
	if supervisorAgent == nil {
		return nil, nil, fmt.Errorf("supervisor agent not found")
	}

	var subAgents []adk.Agent
	for name, agent := range agents {
		if name == supervisorRole.Name {
			continue
		}
		subAgents = append(subAgents, agent)
	}

	if len(subAgents) == 0 {
		return nil, nil, fmt.Errorf("no sub-agents configured")
	}

	return supervisorAgent, subAgents, nil
}

func resolveRoleModelConfig(cfg *config.Config, role Role) (*agent.AgentConfig, error) {
	if strings.TrimSpace(role.ModelOverride) == "" {
		return agent.NewAgentConfigFromConfig(&cfg.LLM)
	}
	provider, model := parseModelOverride(role.ModelOverride)
	if provider == "" {
		base, err := agent.NewAgentConfigFromConfig(&cfg.LLM)
		if err != nil {
			return nil, err
		}
		base.Model = model
		return base, nil
	}
	conf, err := agent.NewAgentConfigFromProvider(&cfg.LLM, provider)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(model) != "" {
		conf.Model = model
	}
	return conf, nil
}

func parseModelOverride(raw string) (provider string, model string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if strings.EqualFold(raw, "default") {
		return "", ""
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.ToLower(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
}

func buildRoleInstruction(role Role) string {
	var builder strings.Builder
	builder.WriteString(role.SystemPrompt)
	if len(role.RoutingHints) > 0 {
		builder.WriteString("\nRouting hints: ")
		builder.WriteString(strings.Join(role.RoutingHints, ", "))
	}
	return builder.String()
}

func buildSupervisorInstruction(role Role, roles map[string]Role) string {
	var builder strings.Builder
	builder.WriteString(buildRoleInstruction(role))
	builder.WriteString("\n\nAvailable roles:\n")
	for _, subRole := range roles {
		if subRole.Name == role.Name {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(subRole.Name)
		builder.WriteString(": ")
		builder.WriteString(subRole.Description)
		if len(subRole.RoutingHints) > 0 {
			builder.WriteString(" (hints: ")
			builder.WriteString(strings.Join(subRole.RoutingHints, ", "))
			builder.WriteString(")")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildUserContent(userQuery string, commands []db.Command) string {
	commandContext := formatCommandsForLLM(commands)
	if strings.TrimSpace(commandContext) == "" {
		return userQuery
	}
	return fmt.Sprintf("%s\n\nRecent commands:\n%s", userQuery, commandContext)
}

func formatCommandsForLLM(commands []db.Command) string {
	if len(commands) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, cmd := range commands {
		builder.WriteString("- ID: ")
		builder.WriteString(cmd.ID)
		builder.WriteString(" | Command: ")
		builder.WriteString(cmd.Command)
		builder.WriteString(" | Cwd: ")
		builder.WriteString(cmd.Cwd)
		if cmd.ExitCode != nil {
			builder.WriteString(" | Exit: ")
			builder.WriteString(fmt.Sprintf("%d", *cmd.ExitCode))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func indexTools(ctx context.Context, tools []tool.BaseTool) (map[string]tool.BaseTool, error) {
	toolMap := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, err
		}
		if info != nil && info.Name != "" {
			toolMap[info.Name] = t
		}
	}
	return toolMap, nil
}

func filterTools(allowlist []string, toolMap map[string]tool.BaseTool) ([]tool.BaseTool, error) {
	if len(allowlist) == 0 {
		return nil, fmt.Errorf("tools_allowlist is empty")
	}
	var tools []tool.BaseTool
	for _, name := range allowlist {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		tool, ok := toolMap[key]
		if !ok {
			return nil, fmt.Errorf("tool not found: %s", key)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
