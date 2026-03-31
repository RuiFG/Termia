package agent

import (
	"context"
	"encoding/gob"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

type hitlInterruptInfo struct {
	Kind         HITLKind      `json:"kind"`
	Title        string        `json:"title,omitempty"`
	Prompt       string        `json:"prompt,omitempty"`
	OriginalTool string        `json:"original_tool,omitempty"`
	Questions    []AskQuestion `json:"questions,omitempty"`
	Command      string        `json:"command,omitempty"`
	Cwd          string        `json:"cwd,omitempty"`
	RiskNote     string        `json:"risk_note,omitempty"`
}

type hitlResumeData struct {
	Confirmed bool        `json:"confirmed"`
	Answers   []AskAnswer `json:"answers,omitempty"`
}

func init() {
	gob.Register(&hitlInterruptInfo{})
	gob.Register(&hitlResumeData{})
}

func resumeHITLResponse[T any](ctx context.Context, info *hitlInterruptInfo) (T, bool, error) {
	var zero T

	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if !wasInterrupted {
		return zero, false, nil
	}

	isTarget, hasData, data := tool.GetResumeContext[T](ctx)
	if !isTarget {
		return zero, false, tool.Interrupt(ctx, info)
	}
	if !hasData {
		return zero, true, nil
	}
	return data, true, nil
}

func parseHITLRequest(event *adk.AgentEvent) (HITLRequest, bool) {
	if event == nil || event.Action == nil || event.Action.Interrupted == nil {
		return HITLRequest{}, false
	}

	var (
		rootCause *adk.InterruptCtx
		info      *hitlInterruptInfo
	)
	for _, interruptCtx := range event.Action.Interrupted.InterruptContexts {
		candidate := interruptInfoFromAny(interruptCtx.Info)
		if candidate == nil {
			continue
		}
		if interruptCtx.IsRootCause {
			rootCause = interruptCtx
			info = candidate
			break
		}
		if rootCause == nil {
			rootCause = interruptCtx
			info = candidate
		}
	}
	if rootCause == nil || info == nil {
		return HITLRequest{}, false
	}

	return HITLRequest{
		ID:             strings.TrimSpace(rootCause.ID),
		FunctionCallID: strings.TrimSpace(rootCause.ID),
		Kind:           info.Kind,
		Title:          strings.TrimSpace(info.Title),
		Prompt:         strings.TrimSpace(info.Prompt),
		OriginalTool:   strings.TrimSpace(info.OriginalTool),
		Questions:      append([]AskQuestion(nil), info.Questions...),
		Command:        strings.TrimSpace(info.Command),
		Cwd:            strings.TrimSpace(info.Cwd),
		RiskNote:       strings.TrimSpace(info.RiskNote),
	}, true
}

func interruptInfoFromAny(value any) *hitlInterruptInfo {
	switch typed := value.(type) {
	case hitlInterruptInfo:
		info := typed
		return &info
	case *hitlInterruptInfo:
		if typed == nil {
			return nil
		}
		info := *typed
		return &info
	default:
		return nil
	}
}
