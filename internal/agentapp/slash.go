package agentapp

import (
	"sort"
	"strings"
)

type SharedSlashCommand struct {
	Name            string
	Description     string
	Scope           MiddlewareScope
	BuildActivation func(args string) (MiddlewareActivation, error)
}

func ResolveSharedSlashCommand(input string, commands []SharedSlashCommand) (SharedSlashCommand, bool) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return SharedSlashCommand{}, false
	}

	name := normalizeSharedSlashCommandInput(trimmed)
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
			Name:        "ralph-loop",
			Description: "Start the Ralph loop middleware",
			Scope:       MiddlewareScopeRun,
			BuildActivation: func(args string) (MiddlewareActivation, error) {
				activation := MiddlewareActivation{
					Name:  "ralph-loop",
					Scope: MiddlewareScopeRun,
				}
				if trimmed := strings.TrimSpace(args); trimmed != "" {
					activation.Args = map[string]string{"args": trimmed}
				}
				return activation, nil
			},
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
