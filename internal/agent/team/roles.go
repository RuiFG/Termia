package team

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/termia/termia/internal/config"
)

const skillFileName = "SKILL.md"

type Role struct {
	Name           string
	Description    string
	SystemPrompt   string
	ToolsAllowlist []string
	RoutingHints   []string
	MaxSteps       int
	ModelOverride  string
	Path           string
}

type RoleSpec struct {
	Name           string
	Description    string
	SystemPrompt   string
	ToolsAllowlist []string
	RoutingHints   []string
	MaxSteps       int
	ModelOverride  string
}

func EnsureDefaultRoles(agentCfg config.AgentTeamConfig) error {
	rolesDir := config.AgentsDir()
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		return fmt.Errorf("ensure roles dir: %w", err)
	}

	defaults := []RoleSpec{
		{
			Name:           "coordinator",
			Description:    "Coordinates ops tasks, delegates to specialists when needed.",
			SystemPrompt:   "You are the Coordinator for Termia ops. Decide whether to answer directly or delegate to a specialist. Use routing hints to choose the right role. Be concise, safe, and execution-aware.",
			ToolsAllowlist: []string{"query_commands", "get_command_output", "search_history", "command", "read_file", "grep", "edit_file", "write_file"},
			RoutingHints:   []string{"delegate", "route", "assign", "specialist"},
			MaxSteps:       20,
			ModelOverride:  "default",
		},
		{
			Name:           "researcher",
			Description:    "Investigates ops issues by gathering info from logs, history, and files.",
			SystemPrompt:   "You are the Researcher for Termia ops. Focus on information gathering, diagnostics, and summarizing evidence. Avoid making changes unless explicitly approved.",
			ToolsAllowlist: []string{"query_commands", "get_command_output", "search_history", "command", "read_file", "grep"},
			RoutingHints:   []string{"investigate", "diagnose", "logs", "metrics", "why", "error"},
			MaxSteps:       20,
			ModelOverride:  "default",
		},
	}

	for _, role := range defaults {
		skillPath := filepath.Join(rolesDir, role.Name, skillFileName)
		if _, err := os.Stat(skillPath); err == nil {
			continue
		}
		if err := writeRoleSkill(role); err != nil {
			return err
		}
	}

	return nil
}

func LoadRoles(agentCfg config.AgentTeamConfig) (map[string]Role, error) {
	if err := EnsureDefaultRoles(agentCfg); err != nil {
		return nil, err
	}

	roles := make(map[string]Role)
	for _, roleName := range agentCfg.Roles {
		roleName = strings.ToLower(strings.TrimSpace(roleName))
		if roleName == "" {
			continue
		}
		role, err := loadRole(roleName)
		if err != nil {
			return nil, err
		}
		roles[roleName] = role
	}

	if len(roles) == 0 {
		return nil, errors.New("no roles loaded from allowlist")
	}

	return roles, nil
}

func loadRole(name string) (Role, error) {
	rolesDir := config.AgentsDir()
	skillPath := filepath.Join(rolesDir, name, skillFileName)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return Role{}, fmt.Errorf("read skill file for %s: %w", name, err)
	}
	role, err := parseSkillMarkdown(string(data))
	if err != nil {
		return Role{}, fmt.Errorf("parse skill file for %s: %w", name, err)
	}
	role.Path = skillPath
	return role, nil
}

func writeRoleSkill(role RoleSpec) error {
	rolesDir := config.AgentsDir()
	roleDir := filepath.Join(rolesDir, role.Name)
	if err := os.MkdirAll(roleDir, 0755); err != nil {
		return fmt.Errorf("create role dir: %w", err)
	}
	content := renderRoleSkill(role)
	path := filepath.Join(roleDir, skillFileName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	return nil
}

func renderRoleSkill(role RoleSpec) string {
	return fmt.Sprintf(`---
name: %s
description: %s
system_prompt: %s
tools_allowlist: %s
routing_hints: %s
max_steps: %d
model_override: %s
---
`,
		role.Name,
		sanitizeInline(role.Description),
		sanitizeInline(role.SystemPrompt),
		formatList(role.ToolsAllowlist),
		formatList(role.RoutingHints),
		role.MaxSteps,
		sanitizeInline(role.ModelOverride),
	)
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("\"%s\"", strings.ReplaceAll(item, "\"", "\\\"")))
	}
	return fmt.Sprintf("[%s]", strings.Join(quoted, ", "))
}

func sanitizeInline(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.TrimSpace(value)
	return value
}
