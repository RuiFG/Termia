package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

// Plan represents a multi-step agent execution plan.
type Plan struct {
	Task      string     `json:"task"`
	Steps     []PlanStep `json:"steps"`
	Model     string     `json:"model"`
	CreatedAt time.Time  `json:"created_at"`
}

// PlanStep describes a single action the agent should perform.
type PlanStep struct {
	Index       int    `json:"index"`
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
	DependsOn   []int  `json:"depends_on"`
}

// PlanResult carries plan generation results.
type PlanResult struct {
	Plan  *Plan
	Error error
}

// StepResult captures execution output for a plan step.
type StepResult struct {
	Output   string
	ExitCode int
	Error    error
	Duration time.Duration
}

const PlanningPromptTemplate = `You are Termia Agent Planner. Return a JSON object with fields: task, steps (array of objects with index, command, explanation, depends_on).
The plan should be safe and incremental. Use numbered indexes starting at 1. Respond with JSON only.`

// GeneratePlan requests a plan from the model for tai agent.
func (r *Runner) GeneratePlan(ctx context.Context, task string, cwd string, recentCommands []db.Command) (*Plan, error) {
	if r.model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	commandContext := formatCommandsForLLM(recentCommands)
	userContent := fmt.Sprintf("Task: %s\nCwd: %s\nRecent commands:\n%s", task, cwd, commandContext)

	response, err := r.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(PlanningPromptTemplate),
		schema.UserMessage(userContent),
	})
	if err != nil {
		return nil, fmt.Errorf("generate plan content: %w", err)
	}

	// Parse the JSON response.
	var plan Plan
	responseText := strings.TrimSpace(response.Content)
	if err := json.Unmarshal([]byte(responseText), &plan); err != nil {
		return nil, fmt.Errorf("parse plan json: %w", err)
	}

	if plan.Task == "" {
		plan.Task = task
	}
	if plan.Model == "" && r.agentCfg != nil {
		plan.Model = r.agentCfg.Model
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}

	return &plan, nil
}

// ExecuteStep executes a single plan step.
func (r *Runner) ExecuteStep(ctx context.Context, step *PlanStep) (*StepResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if step == nil {
		return nil, fmt.Errorf("plan step is nil")
	}
	r.logger.Debug("execute step requested", zap.Int("index", step.Index), zap.String("command", step.Command))
	// TODO: Implement shell execution wiring for plan steps.
	return nil, fmt.Errorf("execute step not implemented")
}
