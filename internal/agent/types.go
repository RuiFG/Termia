package agent

import (
	"context"
	"time"

	"github.com/termia/termia/internal/modelspec"
)

type Mode string

const (
	ModeAssistant Mode = "assistant"
	ModeTeam      Mode = "team"
)

const (
	AskOptionTitleMaxLen   = 24
	AskOptionDescMaxLen    = 80
	AskTypeYourAnswerTitle = "Type Your Answer"
	AskTypeYourAnswerDesc  = "Provide your own answer"
)

type Message struct {
	Role     string
	Content  string
	Commands []Command
}

type RuntimeEventKind string

const (
	RuntimeEventText       RuntimeEventKind = "text"
	RuntimeEventReasoning  RuntimeEventKind = "reasoning"
	RuntimeEventToolCall   RuntimeEventKind = "tool_call"
	RuntimeEventToolResult RuntimeEventKind = "tool_result"
	RuntimeEventCwd        RuntimeEventKind = "cwd"
	RuntimeEventError      RuntimeEventKind = "error"
)

type ToolCallState string

const (
	ToolCallStatePending ToolCallState = "pending"
	ToolCallStateSuccess ToolCallState = "success"
	ToolCallStateError   ToolCallState = "error"
)

type ToolCallEvent struct {
	CallID    string
	AgentName string
	ToolName  string
	Summary   string
	Result    string
	State     ToolCallState
}

type RuntimeEvent struct {
	Kind     RuntimeEventKind
	Text     string
	Cwd      string
	ToolCall *ToolCallEvent
}

type Command struct {
	ID                  string
	TsStart             int64
	TsEnd               *int64
	Command             string
	Cwd                 string
	ExitCode            *int
	DurationMs          *int64
	OutputSize          *int64
	TranscriptAvailable bool
}

type ModelSpec = modelspec.Spec

type AgentSpec struct {
	Name        string    `toml:"name" json:"name"`
	Description string    `toml:"description" json:"description"`
	Instruction string    `toml:"instruction" json:"instruction"`
	Model       ModelSpec `toml:"model" json:"model"`
	Tools       []string  `toml:"tools" json:"tools"`
}

type TeamSpec struct {
	Name        string      `toml:"name" json:"name"`
	Description string      `toml:"description" json:"description"`
	Coordinator AgentSpec   `toml:"coordinator" json:"coordinator"`
	Agents      []AgentSpec `toml:"agents" json:"agents"`
}

type TeamSummary struct {
	Name        string
	Description string
	Path        string
}

type AskOption struct {
	Title       string `json:"title" toml:"title"`
	Description string `json:"description,omitempty" toml:"description"`
}

type AskQuestion struct {
	ID          string      `json:"id,omitempty" toml:"id"`
	Header      string      `json:"header" toml:"header"`
	Question    string      `json:"question" toml:"question"`
	Options     []AskOption `json:"options" toml:"options"`
	Multiple    bool        `json:"multiple,omitempty" toml:"multiple"`
	AllowCustom bool        `json:"allow_custom,omitempty" toml:"allow_custom"`
}

type AskAnswer struct {
	ID              string   `json:"id,omitempty"`
	Question        string   `json:"question"`
	SelectedOptions []string `json:"selected_options,omitempty"`
	CustomTexts     []string `json:"custom_texts,omitempty"`
	Cancelled       bool     `json:"cancelled,omitempty"`
}

type HITLKind string

const (
	HITLKindConfirm   HITLKind = "confirm"
	HITLKindInputForm HITLKind = "input_form"
)

type HITLRequest struct {
	ID             string
	Kind           HITLKind
	Title          string
	Prompt         string
	OriginalTool   string
	FunctionCallID string
	Questions      []AskQuestion
	Command        string
	Cwd            string
	RiskNote       string
}

type HITLResponse struct {
	Confirmed bool        `json:"confirmed"`
	Answers   []AskAnswer `json:"answers,omitempty"`
	Payload   any         `json:"payload,omitempty"`
}

type HITLResponder interface {
	Handle(ctx context.Context, request HITLRequest) (HITLResponse, error)
}

type StreamChunk struct {
	Text       string
	LinesRead  int
	EndOffset  int64
	EOF        bool
	TimedOut   bool
	ReceivedAt time.Time
}

type RunRequest struct {
	Mode             Mode
	TeamName         string
	SessionID        string
	Query            string
	Cwd              string
	SelectedCommands []Command
	Messages         []Message
	StreamReader     *StreamReader
	StreamChunkLines int
	StreamChunkWait  time.Duration
}
