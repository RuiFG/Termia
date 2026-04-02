package agentapp

import (
	"fmt"
	"sort"
	"strings"
)

type SharedSlashCommand struct {
	Name        string
	Description string

	middlewareName string
	scope          MiddlewareScope
}

func (c SharedSlashCommand) BuildActivation(args string) (MiddlewareActivation, error) {
	if strings.TrimSpace(c.Name) == "" {
		return MiddlewareActivation{}, fmt.Errorf("shared slash command name is required")
	}
	if strings.TrimSpace(c.middlewareName) == "" {
		return MiddlewareActivation{}, fmt.Errorf("shared slash command %q has no middleware binding", c.Name)
	}
	activation := MiddlewareActivation{
		Name:  c.middlewareName,
		Scope: c.scope,
	}
	if trimmed := strings.TrimSpace(args); trimmed != "" {
		activation.Args = map[string]string{"args": trimmed}
	}
	return activation, nil
}

func ResolveSharedSlashCommand(input string, commands []SharedSlashCommand) (SharedSlashCommand, bool) {
	name := normalizeSharedSlashCommandInput(input)
	if name == "" {
		return SharedSlashCommand{}, false
	}
	for _, command := range commands {
		if normalizeSharedSlashCommandInput(command.Name) == name {
			return command, true
		}
	}
	return SharedSlashCommand{}, false
}

func DefaultSharedSlashCommands() []SharedSlashCommand {
	commands := []SharedSlashCommand{
		{
			Name:           "ralph-loop",
			Description:    "Start the Ralph loop middleware",
			middlewareName: "ralph-loop",
			scope:          MiddlewareScopeRun,
		},
	}
	sort.SliceStable(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	return commands
}

func normalizeSharedSlashCommandInput(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "/")
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}
