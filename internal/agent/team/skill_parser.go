package team

import (
	"fmt"
	"strconv"
	"strings"
)

type skillDoc struct {
	fields map[string]string
}

func parseSkillMarkdown(content string) (Role, error) {
	front, err := extractFrontMatter(content)
	if err != nil {
		return Role{}, err
	}
	fields := front.fields

	required := []string{"name", "description", "system_prompt", "tools_allowlist", "routing_hints", "max_steps", "model_override"}
	for _, key := range required {
		if strings.TrimSpace(fields[key]) == "" {
			return Role{}, fmt.Errorf("missing required field: %s", key)
		}
	}

	maxSteps, err := strconv.Atoi(strings.TrimSpace(fields["max_steps"]))
	if err != nil || maxSteps <= 0 {
		return Role{}, fmt.Errorf("invalid max_steps: %s", fields["max_steps"])
	}

	role := Role{
		Name:           strings.ToLower(strings.TrimSpace(fields["name"])),
		Description:    strings.TrimSpace(fields["description"]),
		SystemPrompt:   strings.TrimSpace(fields["system_prompt"]),
		ToolsAllowlist: parseStringList(fields["tools_allowlist"]),
		RoutingHints:   parseStringList(fields["routing_hints"]),
		MaxSteps:       maxSteps,
		ModelOverride:  strings.TrimSpace(fields["model_override"]),
	}

	if role.Name == "" {
		return Role{}, fmt.Errorf("role name is empty")
	}

	if len(role.ToolsAllowlist) == 0 {
		return Role{}, fmt.Errorf("tools_allowlist must not be empty")
	}

	if len(role.RoutingHints) == 0 {
		return Role{}, fmt.Errorf("routing_hints must not be empty")
	}

	return role, nil
}

func extractFrontMatter(content string) (*skillDoc, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing front matter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, fmt.Errorf("unterminated front matter")
	}

	fields := map[string]string{}
	for _, line := range lines[1:end] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid front matter line: %s", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		fields[key] = value
	}

	return &skillDoc{fields: fields}, nil
}

func parseStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
		if inner == "" {
			return nil
		}
		parts := strings.Split(inner, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			item := strings.TrimSpace(part)
			item = strings.Trim(item, "\"")
			if item == "" {
				continue
			}
			items = append(items, item)
		}
		return items
	}
	return []string{strings.Trim(raw, "\"")}
}
