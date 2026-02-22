package db

import (
	"encoding/json"
	"strings"
)

type PendingPromptPayload struct {
	Type    string          `json:"type"`
	Command string          `json:"command,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func MarshalPendingPromptPayload(payload PendingPromptPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ParsePendingPromptPayload(prompt PendingPrompt) PendingPromptPayload {
	raw := strings.TrimSpace(prompt.PayloadJSON)
	if raw == "" {
		return PendingPromptPayload{
			Type:    PendingPromptTypeCommand,
			Command: strings.TrimSpace(prompt.Content),
		}
	}
	var payload PendingPromptPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return PendingPromptPayload{
			Type:    PendingPromptTypeCommand,
			Command: strings.TrimSpace(prompt.Content),
		}
	}
	if strings.TrimSpace(payload.Type) == "" {
		payload.Type = PendingPromptTypeCommand
	}
	if payload.Type == PendingPromptTypeCommand && strings.TrimSpace(payload.Command) == "" {
		payload.Command = strings.TrimSpace(prompt.Content)
	}
	return payload
}
