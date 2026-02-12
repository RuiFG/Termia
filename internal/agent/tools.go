package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/termia/termia/internal/db"
)

// --- Request/Response types for function tools ---

// QueryCommandsReq is the input for the query_commands tool.
type QueryCommandsReq struct {
	Query string `json:"query,omitempty" jsonschema:"description=Search text to filter commands"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum number of commands to return (default 20)"`
}

// QueryCommandsRsp is the output for the query_commands tool.
type QueryCommandsRsp struct {
	Result string `json:"result"`
}

// GetCommandOutputReq is the input for the get_command_output tool.
type GetCommandOutputReq struct {
	CommandID string `json:"command_id" jsonschema:"description=The ID of the command to retrieve,required"`
}

// GetCommandOutputRsp is the output for the get_command_output tool.
type GetCommandOutputRsp struct {
	Result string `json:"result"`
}

// SearchHistoryReq is the input for the search_history tool.
type SearchHistoryReq struct {
	Query string `json:"query" jsonschema:"description=Full-text search query over command history,required"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (default 20)"`
}

// SearchHistoryRsp is the output for the search_history tool.
type SearchHistoryRsp struct {
	Result string `json:"result"`
}

// RunCommandReq is the input for the command tool.
type RunCommandReq struct {
	Command string `json:"command" jsonschema:"description=Shell command to execute,required"`
	Cwd     string `json:"cwd,omitempty" jsonschema:"description=Working directory (optional)"`
}

// RunCommandRsp is the output for the command tool.
type RunCommandRsp struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// CreateTools returns the toolset available to the agent as Eino tools.
func CreateTools(database *db.DB) []tool.BaseTool {
	queryCommandsTool := utils.NewTool(&schema.ToolInfo{
		Name: "query_commands",
		Desc: "Query recent command history. Filter by search text or get recent commands.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "Search text to filter commands"},
			"limit": {Type: schema.Integer, Desc: "Maximum number of commands to return (default 20)"},
		}),
	}, func(ctx context.Context, req *QueryCommandsReq) (*QueryCommandsRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if database == nil {
			return nil, fmt.Errorf("database is nil")
		}
		limit := req.Limit
		if limit <= 0 {
			limit = 20
		}
		var commands []db.Command
		var err error
		switch {
		case req.Query != "":
			commands, err = database.SearchCommands(req.Query, limit)
		default:
			commands, err = database.ListRecentCommands(limit)
		}
		if err != nil {
			return nil, fmt.Errorf("query commands: %w", err)
		}
		return &QueryCommandsRsp{Result: formatCommandsForLLM(commands)}, nil
	})

	getCommandOutputTool := utils.NewTool(&schema.ToolInfo{
		Name: "get_command_output",
		Desc: "Retrieve full output for a command by its ID.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command_id": {Type: schema.String, Required: true, Desc: "The ID of the command to retrieve"},
		}),
	}, func(ctx context.Context, req *GetCommandOutputReq) (*GetCommandOutputRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if database == nil {
			return nil, fmt.Errorf("database is nil")
		}
		commandID := strings.TrimSpace(req.CommandID)
		if commandID == "" {
			return nil, fmt.Errorf("command_id is required")
		}
		cmd, err := database.GetCommand(commandID)
		if err != nil {
			return nil, fmt.Errorf("get command: %w", err)
		}
		output, err := loadCommandOutput(database, cmd)
		if err != nil {
			return nil, fmt.Errorf("load command output: %w", err)
		}
		if output == "" {
			output = "(no output)"
		}
		return &GetCommandOutputRsp{Result: output}, nil
	})

	searchHistoryTool := utils.NewTool(&schema.ToolInfo{
		Name: "search_history",
		Desc: "Full-text search over command history.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Required: true, Desc: "Full-text search query over command history"},
			"limit": {Type: schema.Integer, Desc: "Maximum number of results to return (default 20)"},
		}),
	}, func(ctx context.Context, req *SearchHistoryReq) (*SearchHistoryRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if database == nil {
			return nil, fmt.Errorf("database is nil")
		}
		query := strings.TrimSpace(req.Query)
		if query == "" {
			return nil, fmt.Errorf("search query is required")
		}
		limit := req.Limit
		if limit <= 0 {
			limit = 20
		}
		commands, err := database.SearchCommands(query, limit)
		if err != nil {
			return nil, fmt.Errorf("search history: %w", err)
		}
		return &SearchHistoryRsp{Result: formatCommandsForLLM(commands)}, nil
	})

	runCommandTool := utils.NewTool(&schema.ToolInfo{
		Name: "command",
		Desc: "Propose and execute a shell command after user approval.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {Type: schema.String, Required: true, Desc: "Shell command to execute"},
			"cwd":     {Type: schema.String, Desc: "Working directory (optional)"},
		}),
	}, func(ctx context.Context, req *RunCommandReq) (*RunCommandRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		cmdLine := strings.TrimSpace(req.Command)
		if cmdLine == "" {
			return nil, fmt.Errorf("command is required")
		}
		approved, err := promptCommandApproval(cmdLine)
		if err != nil {
			return nil, err
		}
		if !approved {
			return &RunCommandRsp{Stdout: "", Stderr: "user rejected command", ExitCode: 130}, nil
		}
		stdout, stderr, exitCode, err := runShellCommand(ctx, cmdLine, strings.TrimSpace(req.Cwd))
		if err != nil {
			return nil, err
		}
		return &RunCommandRsp{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}, nil
	})

	return []tool.BaseTool{queryCommandsTool, getCommandOutputTool, searchHistoryTool, runCommandTool}
}

// parseToolArgs is a legacy helper kept for BuildContext usage.
func parseToolArgs(args string) (string, string, int) {
	limit := 20
	args = strings.TrimSpace(args)
	if args == "" {
		return "", "", limit
	}

	var query string
	for _, part := range strings.Fields(args) {
		if strings.HasPrefix(part, "query=") {
			query = strings.TrimPrefix(part, "query=")
			continue
		}
		if strings.HasPrefix(part, "limit=") {
			value := strings.TrimPrefix(part, "limit=")
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				limit = parsed
			}
			continue
		}
		if query == "" {
			query = part
		}
	}

	return "", query, limit
}

// formatCommandsForLLM formats command history for inclusion in model prompts.
func formatCommandsForLLM(commands []db.Command) string {
	if len(commands) == 0 {
		return "No commands found."
	}

	var builder strings.Builder
	for _, cmd := range commands {
		builder.WriteString("- ID: ")
		builder.WriteString(cmd.ID)
		builder.WriteString(" | Command: ")
		builder.WriteString(cmd.Command)
		builder.WriteString(" | Cwd: ")
		builder.WriteString(cmd.Cwd)
		builder.WriteString(" | Started: ")
		builder.WriteString(time.Unix(0, cmd.TsStart).Format(time.RFC3339))
		if cmd.TsEnd != nil {
			builder.WriteString(" | Ended: ")
			builder.WriteString(time.Unix(0, *cmd.TsEnd).Format(time.RFC3339))
		}
		if cmd.ExitCode != nil {
			builder.WriteString(" | Exit: ")
			builder.WriteString(strconv.Itoa(*cmd.ExitCode))
		}
		if cmd.DurationMs != nil {
			builder.WriteString(" | DurationMs: ")
			builder.WriteString(strconv.FormatInt(*cmd.DurationMs, 10))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func formatCommandsWithOutput(database *db.DB, commands []db.Command) string {
	if database == nil || len(commands) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, cmd := range commands {
		builder.WriteString("- ID: ")
		builder.WriteString(cmd.ID)
		builder.WriteString(" | Command: ")
		builder.WriteString(cmd.Command)
		builder.WriteString(" | Cwd: ")
		builder.WriteString(cmd.Cwd)
		if cmd.ExitCode != nil {
			builder.WriteString(" | Exit: ")
			builder.WriteString(strconv.Itoa(*cmd.ExitCode))
		}
		builder.WriteString("\n")

		output, err := loadCommandOutput(database, &cmd)
		if err != nil {
			builder.WriteString("  Output: (error loading output)\n")
			continue
		}
		if strings.TrimSpace(output) == "" {
			builder.WriteString("  Output: (no output)\n")
			continue
		}
		builder.WriteString("  Output:\n")
		for _, line := range strings.Split(output, "\n") {
			builder.WriteString("    ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func loadCommandOutput(database *db.DB, cmd *db.Command) (string, error) {
	if cmd == nil {
		return "", fmt.Errorf("command is nil")
	}
	if cmd.StartOffset == nil || cmd.EndOffset == nil {
		return "", nil
	}
	if cmd.OutputSize != nil && *cmd.OutputSize == 0 {
		return "", nil
	}

	transcriptPath := ""
	if cmd.TranscriptPath != nil {
		transcriptPath = *cmd.TranscriptPath
	}
	if transcriptPath == "" {
		return "", fmt.Errorf("missing transcript path for command %s", cmd.ID)
	}

	data, err := readTranscriptRange(transcriptPath, *cmd.StartOffset, *cmd.EndOffset)
	if err != nil {
		return "", err
	}

	return sanitizeOutput(string(data)), nil
}

func readTranscriptRange(path string, startOffset int64, endOffset int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if startOffset < 0 || endOffset < startOffset {
		return nil, fmt.Errorf("invalid transcript range")
	}

	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return nil, err
	}

	length := endOffset - startOffset
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf, nil
}

func sanitizeOutput(raw string) string {
	cleaned := stripANSICodes(raw)
	scanner := bufio.NewScanner(strings.NewReader(cleaned))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return cleaned
	}
	return strings.Join(lines, "\n")
}

var ansiSequencePattern = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
var oscPattern = regexp.MustCompile("\x1b\\][^\x1b\\x07]*(?:\x1b\\\\|\x07)")
var csiSequencePattern = regexp.MustCompile("\u009b[0-?]*[ -/]*[@-~]")

func stripANSICodes(input string) string {
	cleaned := oscPattern.ReplaceAllString(input, "")
	cleaned = ansiSequencePattern.ReplaceAllString(cleaned, "")
	cleaned = csiSequencePattern.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "\x1b", "")
	return cleaned
}

func BindTools(model model.ToolCallingChatModel, tools []tool.BaseTool) (model.ToolCallingChatModel, error) {
	if model == nil {
		return nil, fmt.Errorf("model is nil")
	}
	if len(tools) == 0 {
		return model, nil
	}
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			return nil, fmt.Errorf("tool info: %w", err)
		}
		infos = append(infos, info)
	}
	bound, err := model.WithTools(infos)
	if err != nil {
		return nil, fmt.Errorf("bind tools: %w", err)
	}
	return bound, nil
}

func promptCommandApproval(command string) (bool, error) {
	fmt.Printf("\nProposed command:\n  %s\n\nApprove? [y/N]: ", command)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read approval: %w", err)
	}
	choice := strings.TrimSpace(strings.ToLower(input))
	return choice == "y" || choice == "yes", nil
}

func runShellCommand(ctx context.Context, command string, cwd string) (string, string, int, error) {
	shell := "sh"
	shellFlag := "-lc"
	if strings.Contains(strings.ToLower(os.Getenv("SHELL")), "bash") {
		shell = "bash"
	}
	cmd := exec.CommandContext(ctx, shell, shellFlag, command)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0, nil
	}
	exitCode := 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
		return stdout.String(), stderr.String(), exitCode, nil
	}
	return stdout.String(), stderr.String(), exitCode, fmt.Errorf("run command: %w", err)
}
