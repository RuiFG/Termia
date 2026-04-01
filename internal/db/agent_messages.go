package db

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/termia/termia/internal/textutil"
)

// AgentMessage represents a single message in a session.
type AgentMessage struct {
	ID           string
	SessionID    string
	Role         string
	Content      string
	MetadataJSON string
	CreatedAt    int64
}

type AgentMessageMetadata struct {
	CitedCommands []AgentMessageCommandMetadata  `json:"cited_commands,omitempty"`
	ToolCalls     []AgentMessageToolCallMetadata `json:"tool_calls,omitempty"`
}

type AgentMessageCommandMetadata struct {
	ID                  string `json:"id"`
	TsStart             int64  `json:"ts_start,omitempty"`
	TsEnd               *int64 `json:"ts_end,omitempty"`
	Command             string `json:"command"`
	Cwd                 string `json:"cwd,omitempty"`
	ExitCode            *int   `json:"exit_code,omitempty"`
	DurationMs          *int64 `json:"duration_ms,omitempty"`
	OutputSize          *int64 `json:"output_size,omitempty"`
	TranscriptAvailable bool   `json:"transcript_available,omitempty"`
}

type AgentMessageToolCallMetadata struct {
	CallID    string `json:"call_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	ToolName  string `json:"tool_name"`
	Summary   string `json:"summary,omitempty"`
	Result    string `json:"result,omitempty"`
	State     string `json:"state,omitempty"`
}

// CreateAgentMessage inserts a new agent message and bumps session updated_at.
func (d *DB) CreateAgentMessage(msg *AgentMessage) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}
	msg.Content = textutil.NormalizeTrimmedText(msg.Content)
	query := `
		INSERT INTO agent_messages (id, session_id, role, content, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := d.conn.Exec(query, msg.ID, msg.SessionID, msg.Role, msg.Content, msg.MetadataJSON, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create agent message: %w", err)
	}
	if err := d.UpdateAgentSessionUpdatedAt(msg.SessionID, msg.CreatedAt); err != nil {
		return err
	}
	return nil
}

// ListAgentMessages returns messages for a session ordered by created time.
func (d *DB) ListAgentMessages(sessionID string) ([]AgentMessage, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id is empty")
	}
	query := `
		SELECT id, session_id, role, content, metadata_json, created_at
		FROM agent_messages
		WHERE session_id = ?
		ORDER BY created_at ASC
	`
	rows, err := d.conn.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent messages: %w", err)
	}
	defer rows.Close()

	var messages []AgentMessage
	for rows.Next() {
		var m AgentMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.MetadataJSON, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent messages: %w", err)
	}
	return messages, nil
}

func EncodeAgentMessageMetadata(metadata AgentMessageMetadata) (string, error) {
	metadata = normalizeAgentMessageMetadata(metadata)
	if len(metadata.CitedCommands) == 0 && len(metadata.ToolCalls) == 0 {
		return "", nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("failed to encode agent message metadata: %w", err)
	}
	return string(data), nil
}

func ParseAgentMessageMetadata(msg AgentMessage) AgentMessageMetadata {
	if msg.MetadataJSON == "" {
		return AgentMessageMetadata{}
	}
	var metadata AgentMessageMetadata
	if err := json.Unmarshal([]byte(msg.MetadataJSON), &metadata); err != nil {
		return AgentMessageMetadata{}
	}
	metadata = normalizeAgentMessageMetadata(metadata)
	if len(metadata.CitedCommands) == 0 {
		metadata.CitedCommands = nil
	}
	if len(metadata.ToolCalls) == 0 {
		metadata.ToolCalls = nil
	}
	if len(metadata.CitedCommands) == 0 && len(metadata.ToolCalls) == 0 {
		return AgentMessageMetadata{}
	}
	return metadata
}

func AgentMessageCommandMetadataFromCommand(cmd Command) AgentMessageCommandMetadata {
	return AgentMessageCommandMetadata{
		ID:                  cmd.ID,
		TsStart:             cmd.TsStart,
		TsEnd:               cmd.TsEnd,
		Command:             cmd.Command,
		Cwd:                 cmd.Cwd,
		ExitCode:            cmd.ExitCode,
		DurationMs:          cmd.DurationMs,
		OutputSize:          cmd.OutputSize,
		TranscriptAvailable: cmd.TranscriptPath != nil,
	}
}

func AgentMessageCommandMetadataFromCommands(commands []Command) []AgentMessageCommandMetadata {
	if len(commands) == 0 {
		return nil
	}
	result := make([]AgentMessageCommandMetadata, 0, len(commands))
	for _, cmd := range commands {
		if cmd.ID == "" {
			continue
		}
		result = append(result, AgentMessageCommandMetadataFromCommand(cmd))
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeAgentMessageMetadata(metadata AgentMessageMetadata) AgentMessageMetadata {
	if len(metadata.CitedCommands) == 0 {
		metadata.CitedCommands = nil
	} else {
		seen := make(map[string]bool, len(metadata.CitedCommands))
		normalizedCommands := make([]AgentMessageCommandMetadata, 0, len(metadata.CitedCommands))
		for _, command := range metadata.CitedCommands {
			command.ID = normalizeMessageMetadataID(command.ID)
			command.Command = normalizeMessageMetadataText(command.Command)
			command.Cwd = normalizeMessageMetadataText(command.Cwd)
			if command.ID == "" || command.Command == "" || seen[command.ID] {
				continue
			}
			seen[command.ID] = true
			normalizedCommands = append(normalizedCommands, command)
		}
		metadata.CitedCommands = normalizedCommands
	}
	if len(metadata.ToolCalls) == 0 {
		metadata.ToolCalls = nil
		return metadata
	}
	normalizedToolCalls := make([]AgentMessageToolCallMetadata, 0, len(metadata.ToolCalls))
	seenToolCalls := make(map[string]bool, len(metadata.ToolCalls))
	for _, toolCall := range metadata.ToolCalls {
		toolCall.CallID = normalizeMessageMetadataID(toolCall.CallID)
		toolCall.AgentName = normalizeMessageMetadataText(toolCall.AgentName)
		toolCall.ToolName = normalizeMessageMetadataText(toolCall.ToolName)
		toolCall.Summary = normalizeMessageMetadataText(toolCall.Summary)
		toolCall.Result = normalizeMessageMetadataText(toolCall.Result)
		toolCall.State = normalizeMessageMetadataText(toolCall.State)
		key := toolCall.CallID
		if key == "" {
			key = toolCall.AgentName + ":" + toolCall.ToolName + ":" + toolCall.Summary
		}
		if toolCall.ToolName == "" || seenToolCalls[key] {
			continue
		}
		seenToolCalls[key] = true
		normalizedToolCalls = append(normalizedToolCalls, toolCall)
	}
	metadata.ToolCalls = normalizedToolCalls
	return metadata
}

func normalizeMessageMetadataID(id string) string {
	return normalizeMessageMetadataText(id)
}

func normalizeMessageMetadataText(value string) string {
	return strings.TrimSpace(value)
}

func (m AgentMessageMetadata) CommandIDs() []string {
	if len(m.CitedCommands) == 0 {
		return nil
	}
	ids := make([]string, 0, len(m.CitedCommands))
	for _, command := range m.CitedCommands {
		if command.ID == "" {
			continue
		}
		ids = append(ids, command.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}
