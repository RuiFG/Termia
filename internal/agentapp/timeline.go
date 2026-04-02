package agentapp

import (
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/textutil"
)

func NormalizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "user":
		return "user"
	case "assistant", "agent":
		return "assistant"
	case "tool":
		return "tool"
	case "system":
		return "system"
	case "reasoning":
		return "reasoning"
	case "error":
		return "error"
	default:
		if role == "" {
			return "assistant"
		}
		return role
	}
}

func AppendTimelineText(entries []TimelineEntry, role, content string, appendToLast bool) []TimelineEntry {
	content = textutil.NormalizeLineEndings(content)
	if content == "" {
		return entries
	}
	role = NormalizeRole(role)
	if appendToLast && len(entries) > 0 {
		last := &entries[len(entries)-1]
		if last.ToolCall == nil && NormalizeRole(last.Role) == role {
			last.Content += content
			return entries
		}
	}
	return append(entries, TimelineEntry{Role: role, Content: content})
}

func UpsertTimelineToolCall(entries []TimelineEntry, call runtimeagent.ToolCallEvent) []TimelineEntry {
	call.CallID = strings.TrimSpace(call.CallID)
	call.ToolName = textutil.NormalizeInlineText(call.ToolName)
	call.Summary = textutil.NormalizeInlineText(call.Summary)
	call.Result = textutil.NormalizeInlineText(call.Result)
	call.AgentName = textutil.NormalizeInlineText(call.AgentName)
	if call.ToolName == "" {
		return entries
	}

	for i := len(entries) - 1; i >= 0; i-- {
		current := entries[i].ToolCall
		if current == nil {
			continue
		}
		if call.CallID != "" && current.CallID == call.CallID {
			merged := *current
			if merged.CallID == "" {
				merged.CallID = call.CallID
			}
			if merged.AgentName == "" {
				merged.AgentName = call.AgentName
			}
			if merged.ToolName == "" {
				merged.ToolName = call.ToolName
			}
			if merged.Summary == "" {
				merged.Summary = call.Summary
			}
			if merged.Result == "" {
				merged.Result = call.Result
			}
			if call.State != "" && (merged.State == "" || merged.State == runtimeagent.ToolCallStatePending) {
				merged.State = call.State
			}
			entries[i].ToolCall = &merged
			return entries
		}
	}

	return append(entries, TimelineEntry{Role: "tool", ToolCall: &call})
}

func MarkLatestPendingToolFailed(entries []TimelineEntry, reason string) []TimelineEntry {
	reason = textutil.NormalizeInlineText(reason)
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ToolCall == nil {
			continue
		}
		if entries[i].ToolCall.State != runtimeagent.ToolCallStatePending {
			continue
		}
		call := *entries[i].ToolCall
		call.State = runtimeagent.ToolCallStateError
		if call.Result == "" {
			call.Result = reason
		}
		entries[i].ToolCall = &call
		return entries
	}
	return entries
}
