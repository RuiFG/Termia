package agent

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

func buildPromptContent(req RunRequest) *schema.Message {
	return schema.UserMessage(buildPromptText(req, ""))
}

func buildStreamPromptContent(req RunRequest, chunk StreamChunk, first bool) *schema.Message {
	title := "New stream chunk"
	if first {
		title = "Initial stream chunk"
	}
	body := fmt.Sprintf("%s:\n%s", title, strings.TrimSpace(chunk.Text))
	return schema.UserMessage(buildPromptText(req, body))
}

func buildPromptText(req RunRequest, streamChunk string) string {
	var sb strings.Builder
	if strings.TrimSpace(req.Cwd) != "" {
		sb.WriteString("Current working directory:\n")
		sb.WriteString(req.Cwd)
		sb.WriteString("\n\n")
	}
	if len(req.Messages) > 0 {
		sb.WriteString("Conversation transcript:\n")
		for _, message := range req.Messages {
			rendered := renderPromptMessage(message)
			if rendered == "" {
				continue
			}
			sb.WriteString(rendered)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	query, attachments := expandFileMentions(req.Query)
	if attachments != "" {
		sb.WriteString(attachments)
		sb.WriteString("\n\n")
	}
	sb.WriteString("User request:\n")
	sb.WriteString(renderPromptMessageBody(strings.TrimSpace(query), req.SelectedCommands))
	if strings.TrimSpace(streamChunk) != "" {
		if strings.TrimSpace(query) != "" || len(req.SelectedCommands) > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(strings.TrimSpace(streamChunk))
	}
	return sb.String()
}

func renderPromptMessage(message Message) string {
	role := strings.TrimSpace(message.Role)
	if role == "" {
		role = "user"
	}
	body := renderPromptMessageBody(message.Content, message.Commands)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("- %s:\n%s", role, indentPromptBlock(body, "  "))
}

func renderPromptMessageBody(content string, commands []Command) string {
	sections := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(content); trimmed != "" {
		sections = append(sections, trimmed)
	}
	if rendered := renderPromptCommands(commands); rendered != "" {
		sections = append(sections, rendered)
	}
	return strings.Join(sections, "\n\n")
}

func renderPromptCommands(commands []Command) string {
	if len(commands) == 0 {
		return ""
	}
	lines := make([]string, 0, len(commands)+1)
	lines = append(lines, "Referenced terminal commands (metadata only; use inspect_command_output to inspect output):")
	for _, command := range commands {
		if rendered := renderPromptCommand(command); rendered != "" {
			lines = append(lines, rendered)
		}
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderPromptCommand(command Command) string {
	if strings.TrimSpace(command.ID) == "" || strings.TrimSpace(command.Command) == "" {
		return ""
	}
	fields := []string{
		fmt.Sprintf("id=%s", command.ID),
		fmt.Sprintf("command=%s", strconv.Quote(strings.TrimSpace(command.Command))),
	}
	if cwd := strings.TrimSpace(command.Cwd); cwd != "" {
		fields = append(fields, fmt.Sprintf("cwd=%s", strconv.Quote(cwd)))
	}
	if command.TsStart > 0 {
		fields = append(fields, fmt.Sprintf("started_at=%s", time.Unix(0, command.TsStart).UTC().Format(time.RFC3339)))
	}
	if command.ExitCode != nil {
		fields = append(fields, fmt.Sprintf("exit_code=%d", *command.ExitCode))
	}
	if command.DurationMs != nil {
		fields = append(fields, fmt.Sprintf("duration_ms=%d", *command.DurationMs))
	}
	if command.OutputSize != nil {
		fields = append(fields, fmt.Sprintf("output_size=%d", *command.OutputSize))
	}
	if command.TranscriptAvailable {
		fields = append(fields, "transcript_available=true")
	}
	return "- " + strings.Join(fields, " | ")
}

func indentPromptBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func expandFileMentions(query string) (string, string) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return query, ""
	}

	var attachments []string
	for _, field := range fields {
		if !strings.HasPrefix(field, "@") || len(field) < 2 {
			continue
		}
		path := strings.TrimPrefix(field, "@")
		file, err := readFile(&ReadFileReq{Path: path, MaxLines: 200, MaxBytes: 32 * 1024})
		if err != nil {
			attachments = append(attachments, fmt.Sprintf("Attachment %s: error: %v", path, err))
			continue
		}
		attachments = append(attachments, fmt.Sprintf("Attachment %s:\n%s", file.Path, file.Content))
	}
	return query, strings.Join(attachments, "\n\n")
}
