package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/sessionstate"
	"github.com/termia/termia/internal/textutil"
)

type taiTimelineMessage struct {
	Role     string
	Content  string
	ToolCall *agent.ToolCallEvent
}

func resolveTaiSession(cmd *cobra.Command, database *db.DB, cwd string) (db.AgentSession, agent.Mode, string, error) {
	if database == nil {
		return db.AgentSession{}, "", "", fmt.Errorf("database is nil")
	}
	base, hasBase, err := resolveExistingTaiSession(database)
	if err != nil {
		return db.AgentSession{}, "", "", err
	}
	mode, teamName, err := resolveTaiRuntime(cmd, base, hasBase)
	if err != nil {
		return db.AgentSession{}, "", "", err
	}
	cwd = strings.TrimSpace(cwd)
	if taiNewSession || !hasBase {
		session, err := createTaiSession(database, cwd, mode, teamName)
		if err != nil {
			return db.AgentSession{}, "", "", err
		}
		return session, mode, teamName, nil
	}
	now := time.Now().UnixNano()
	if cwd != "" && cwd != strings.TrimSpace(base.Cwd) {
		if err := database.UpdateAgentSessionCwd(base.ID, cwd, now); err != nil {
			return db.AgentSession{}, "", "", err
		}
		base.Cwd = cwd
		base.UpdatedAt = now
	}
	specSnapshot := buildTaiSessionSpecSnapshot(mode, teamName)
	if mode != normalizedSessionMode(base.Mode) || teamName != strings.TrimSpace(base.TeamName) || specSnapshot != strings.TrimSpace(base.SpecSnapshotJSON) {
		if err := database.UpdateAgentSessionRuntime(base.ID, string(mode), teamName, specSnapshot, time.Now().UnixNano()); err != nil {
			return db.AgentSession{}, "", "", err
		}
		base.Mode = string(mode)
		base.TeamName = teamName
		base.SpecSnapshotJSON = specSnapshot
	}
	return base, mode, teamName, nil
}

func resolveExistingTaiSession(database *db.DB) (db.AgentSession, bool, error) {
	if database == nil {
		return db.AgentSession{}, false, fmt.Errorf("database is nil")
	}
	if currentID := sessionstate.CurrentID(); currentID != "" {
		session, ok, err := database.GetAgentSession(currentID)
		if err != nil {
			return db.AgentSession{}, false, err
		}
		if ok {
			return session, true, nil
		}
	}
	return database.LatestAgentSession()
}

func resolveTaiRuntime(cmd *cobra.Command, base db.AgentSession, hasBase bool) (agent.Mode, string, error) {
	modeValue := normalizedSessionMode(base.Mode)
	if modeValue == "" && cfg != nil {
		modeValue = normalizedSessionMode(cfg.Agent.DefaultMode)
	}
	teamName := strings.TrimSpace(base.TeamName)
	if teamName == "" && modeValue == agent.ModeTeam && cfg != nil {
		teamName = strings.TrimSpace(cfg.Agent.DefaultTeam)
	}
	if cmd != nil && cmd.Flags().Changed("mode") {
		var err error
		modeValue, teamName, err = parseTaiRuntimeModeValue(taiMode)
		if err != nil {
			return "", "", err
		}
	}
	if modeValue == "" {
		modeValue = agent.ModeAssistant
	}
	if !hasBase && modeValue == agent.ModeTeam && teamName == "" && cfg != nil {
		teamName = strings.TrimSpace(cfg.Agent.DefaultTeam)
	}
	if modeValue != agent.ModeTeam {
		teamName = ""
	}
	return modeValue, teamName, nil
}

func normalizedSessionMode(mode string) agent.Mode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case string(agent.ModeTeam):
		return agent.ModeTeam
	case string(agent.ModeAssistant):
		return agent.ModeAssistant
	default:
		return ""
	}
}

func parseTaiRuntimeModeValue(value string) (agent.Mode, string, error) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "":
		return "", "", fmt.Errorf("mode is empty")
	case string(agent.ModeAssistant):
		return agent.ModeAssistant, "", nil
	case string(agent.ModeTeam):
		return "", "", fmt.Errorf("invalid mode %q: use assistant or a team name", value)
	default:
		return agent.ModeTeam, value, nil
	}
}

func createTaiSession(database *db.DB, cwd string, mode agent.Mode, teamName string) (db.AgentSession, error) {
	if database == nil {
		return db.AgentSession{}, fmt.Errorf("database is nil")
	}
	if mode != agent.ModeTeam {
		teamName = ""
	}
	now := time.Now().UnixNano()
	session := db.AgentSession{
		ID:               generateID(),
		Name:             fmt.Sprintf("Session %s", time.Now().Format("2006-01-02 15:04")),
		Mode:             string(mode),
		TeamName:         strings.TrimSpace(teamName),
		SpecSnapshotJSON: buildTaiSessionSpecSnapshot(mode, teamName),
		Cwd:              strings.TrimSpace(cwd),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := database.CreateAgentSession(&session); err != nil {
		return db.AgentSession{}, err
	}
	return session, nil
}

func buildTaiSessionSpecSnapshot(mode agent.Mode, teamName string) string {
	if mode != agent.ModeTeam {
		teamName = ""
	}
	payload := map[string]any{
		"mode":      string(mode),
		"team_name": strings.TrimSpace(teamName),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func taiConversationMessages(database *db.DB, sessionID string) ([]agent.Message, error) {
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	messages, err := database.ListAgentMessages(sessionID)
	if err != nil {
		return nil, err
	}
	return taiConversationMessagesFromDB(messages), nil
}

func taiConversationMessagesFromDB(messages []db.AgentMessage) []agent.Message {
	if len(messages) == 0 {
		return nil
	}
	output := make([]agent.Message, 0, len(messages))
	for _, message := range messages {
		role := normalizeTaiConversationRole(message.Role)
		if role == "tool" || role == "error" {
			continue
		}
		content := textutil.NormalizeTrimmedText(message.Content)
		if content == "" {
			continue
		}
		output = append(output, agent.Message{
			Role:     role,
			Content:  content,
			Commands: taiCommandsFromMessageMetadata(db.ParseAgentMessageMetadata(message)),
		})
	}
	if len(output) == 0 {
		return nil
	}
	return output
}

func normalizeTaiConversationRole(role string) string {
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
	case "error":
		return "error"
	default:
		if role == "" {
			return "assistant"
		}
		return role
	}
}

func taiCommandsFromMessageMetadata(metadata db.AgentMessageMetadata) []agent.Command {
	if len(metadata.CitedCommands) == 0 {
		return nil
	}
	commands := make([]agent.Command, 0, len(metadata.CitedCommands))
	for _, cited := range metadata.CitedCommands {
		if cited.ID == "" || strings.TrimSpace(cited.Command) == "" {
			continue
		}
		commands = append(commands, agent.Command{
			ID:                  cited.ID,
			TsStart:             cited.TsStart,
			TsEnd:               cited.TsEnd,
			Command:             cited.Command,
			Cwd:                 cited.Cwd,
			ExitCode:            cited.ExitCode,
			DurationMs:          cited.DurationMs,
			OutputSize:          cited.OutputSize,
			TranscriptAvailable: cited.TranscriptAvailable,
		})
	}
	if len(commands) == 0 {
		return nil
	}
	return commands
}

func taiAgentCommandsFromDBCommands(commands []db.Command) []agent.Command {
	if len(commands) == 0 {
		return nil
	}
	result := make([]agent.Command, 0, len(commands))
	for _, cmd := range commands {
		if strings.TrimSpace(cmd.ID) == "" || strings.TrimSpace(cmd.Command) == "" {
			continue
		}
		result = append(result, agent.Command{
			ID:                  cmd.ID,
			TsStart:             cmd.TsStart,
			TsEnd:               cmd.TsEnd,
			Command:             cmd.Command,
			Cwd:                 cmd.Cwd,
			ExitCode:            cmd.ExitCode,
			DurationMs:          cmd.DurationMs,
			OutputSize:          cmd.OutputSize,
			TranscriptAvailable: cmd.TranscriptPath != nil,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func taiEncodeSelectedCommandMetadata(commands []db.Command) (string, error) {
	return db.EncodeAgentMessageMetadata(db.AgentMessageMetadata{
		CitedCommands: db.AgentMessageCommandMetadataFromCommands(commands),
	})
}

func taiEncodeSelectedCommandIDs(commands []db.Command) string {
	if len(commands) == 0 {
		return "[]"
	}
	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		if command.ID == "" {
			continue
		}
		ids = append(ids, command.ID)
	}
	if len(ids) == 0 {
		return "[]"
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func taiPersistTimelineMessages(database *db.DB, sessionID string, messages []taiTimelineMessage) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}
	persisted := taiNormalizeTimelineMessages(messages)
	if len(persisted) == 0 {
		return nil
	}
	createdAt := time.Now().UnixNano()
	for idx, message := range persisted {
		metadataJSON, err := taiEncodeTimelineMessageMetadata(message)
		if err != nil {
			return err
		}
		record := &db.AgentMessage{
			ID:           generateID(),
			SessionID:    sessionID,
			Role:         normalizeTaiConversationRole(message.Role),
			Content:      textutil.NormalizeTrimmedText(message.Content),
			MetadataJSON: metadataJSON,
			CreatedAt:    createdAt + int64(idx),
		}
		if err := database.CreateAgentMessage(record); err != nil {
			return err
		}
	}
	return nil
}

func taiEncodeTimelineMessageMetadata(message taiTimelineMessage) (string, error) {
	if message.ToolCall == nil {
		return "", nil
	}
	toolCall := taiNormalizeToolCall(*message.ToolCall)
	metadata := db.AgentMessageMetadata{
		ToolCalls: []db.AgentMessageToolCallMetadata{
			{
				CallID:    toolCall.CallID,
				AgentName: toolCall.AgentName,
				ToolName:  toolCall.ToolName,
				Summary:   toolCall.Summary,
				Result:    toolCall.Result,
				State:     string(toolCall.State),
			},
		},
	}
	return db.EncodeAgentMessageMetadata(metadata)
}

func taiNormalizeTimelineMessages(messages []taiTimelineMessage) []taiTimelineMessage {
	if len(messages) == 0 {
		return nil
	}
	result := make([]taiTimelineMessage, 0, len(messages))
	for _, message := range messages {
		if !taiRenderableTimelineMessage(message) {
			continue
		}
		result = append(result, message)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func taiRenderableTimelineMessage(message taiTimelineMessage) bool {
	if message.ToolCall != nil {
		return strings.TrimSpace(message.ToolCall.ToolName) != ""
	}
	return textutil.NormalizeTrimmedText(message.Content) != ""
}

func taiAppendTimelineText(messages []taiTimelineMessage, role, content string, appendToLast bool) []taiTimelineMessage {
	content = textutil.NormalizeLineEndings(content)
	if content == "" {
		return messages
	}
	role = normalizeTaiConversationRole(role)
	if appendToLast && len(messages) > 0 {
		last := &messages[len(messages)-1]
		if normalizeTaiConversationRole(last.Role) == role && last.ToolCall == nil {
			last.Content += content
			return messages
		}
	}
	return append(messages, taiTimelineMessage{Role: role, Content: content})
}

func taiUpsertTimelineToolCall(messages []taiTimelineMessage, toolCall agent.ToolCallEvent) []taiTimelineMessage {
	normalized := taiNormalizeToolCall(toolCall)
	if normalized.ToolName == "" {
		return messages
	}
	if normalized.CallID != "" {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].ToolCall == nil {
				continue
			}
			if strings.TrimSpace(messages[i].ToolCall.CallID) != normalized.CallID {
				continue
			}
			merged := taiMergeToolCall(*messages[i].ToolCall, normalized)
			messages[i].ToolCall = &merged
			return messages
		}
	}
	if normalized.State != "" && normalized.State != agent.ToolCallStatePending {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].ToolCall == nil {
				continue
			}
			current := messages[i].ToolCall
			if current.State != agent.ToolCallStatePending {
				continue
			}
			if current.ToolName != normalized.ToolName || current.Summary != normalized.Summary {
				continue
			}
			merged := taiMergeToolCall(*current, normalized)
			messages[i].ToolCall = &merged
			return messages
		}
	}
	return append(messages, taiTimelineMessage{Role: "tool", ToolCall: &normalized})
}

func taiMarkLatestPendingToolFailed(messages []taiTimelineMessage, reason string) []taiTimelineMessage {
	reason = textutil.NormalizeInlineText(reason)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ToolCall == nil {
			continue
		}
		if messages[i].ToolCall.State != agent.ToolCallStatePending {
			continue
		}
		call := *messages[i].ToolCall
		call.State = agent.ToolCallStateError
		if strings.TrimSpace(call.Result) == "" {
			call.Result = strings.TrimSpace(reason)
		}
		messages[i].ToolCall = &call
		return messages
	}
	return messages
}

func taiNormalizeToolCall(toolCall agent.ToolCallEvent) agent.ToolCallEvent {
	return agent.ToolCallEvent{
		CallID:    strings.TrimSpace(toolCall.CallID),
		AgentName: textutil.NormalizeInlineText(toolCall.AgentName),
		ToolName:  textutil.NormalizeInlineText(toolCall.ToolName),
		Summary:   textutil.NormalizeInlineText(toolCall.Summary),
		Result:    textutil.NormalizeInlineText(toolCall.Result),
		State:     toolCall.State,
	}
}

func taiMergeToolCall(existing, incoming agent.ToolCallEvent) agent.ToolCallEvent {
	merged := existing
	if merged.CallID == "" {
		merged.CallID = incoming.CallID
	}
	if merged.AgentName == "" {
		merged.AgentName = incoming.AgentName
	}
	if merged.ToolName == "" {
		merged.ToolName = incoming.ToolName
	}
	if strings.TrimSpace(incoming.Summary) != "" {
		merged.Summary = incoming.Summary
	}
	if strings.TrimSpace(incoming.Result) != "" {
		merged.Result = incoming.Result
	}
	if incoming.State != "" {
		merged.State = incoming.State
	}
	return merged
}
