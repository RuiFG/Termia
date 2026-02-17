package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/recorder"
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

// ReadFileReq is the input for the read_file tool.
type ReadFileReq struct {
	Path string `json:"path" jsonschema:"description=File path to read,required"`
}

// ReadFileRsp is the output for the read_file tool.
type ReadFileRsp struct {
	Content string `json:"content"`
}

// GrepReq is the input for the grep tool.
type GrepReq struct {
	Pattern string `json:"pattern" jsonschema:"description=Regex pattern,required"`
	Path    string `json:"path" jsonschema:"description=Path to file or directory,required"`
}

// GrepRsp is the output for the grep tool.
type GrepRsp struct {
	Matches string `json:"matches"`
}

// EditFileReq is the input for the edit_file tool.
type EditFileReq struct {
	Path    string `json:"path" jsonschema:"description=File path to edit,required"`
	OldText string `json:"old_text" jsonschema:"description=Exact text to replace,required"`
	NewText string `json:"new_text" jsonschema:"description=Replacement text,required"`
}

// EditFileRsp is the output for the edit_file tool.
type EditFileRsp struct {
	Result string `json:"result"`
}

// WriteFileReq is the input for the write_file tool.
type WriteFileReq struct {
	Path    string `json:"path" jsonschema:"description=File path to write,required"`
	Content string `json:"content" jsonschema:"description=File content,required"`
}

// WriteFileRsp is the output for the write_file tool.
type WriteFileRsp struct {
	Result string `json:"result"`
}

// CreateTools returns the toolset available to the agent as Eino tools.
func CreateTools(database *db.DB, requireApproval bool) []tool.BaseTool {
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

	readFileTool := utils.NewTool(&schema.ToolInfo{
		Name: "read_file",
		Desc: "Read a text file from disk.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Required: true, Desc: "File path to read"},
		}),
	}, func(ctx context.Context, req *ReadFileReq) (*ReadFileRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		path, err := safePath(req.Path)
		if err != nil {
			return nil, err
		}
		if requireApproval {
			approved, err := requestApprovalOrEnqueue(database, fmt.Sprintf("read file: %s", path))
			if err != nil {
				return nil, err
			}
			if !approved {
				return &ReadFileRsp{Content: ""}, nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		return &ReadFileRsp{Content: string(data)}, nil
	})

	grepTool := utils.NewTool(&schema.ToolInfo{
		Name: "grep",
		Desc: "Search for a regex pattern in a file or directory.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {Type: schema.String, Required: true, Desc: "Regex pattern"},
			"path":    {Type: schema.String, Required: true, Desc: "File or directory path"},
		}),
	}, func(ctx context.Context, req *GrepReq) (*GrepRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pattern := strings.TrimSpace(req.Pattern)
		if pattern == "" {
			return nil, fmt.Errorf("pattern is required")
		}
		path, err := safePath(req.Path)
		if err != nil {
			return nil, err
		}
		if requireApproval {
			approved, err := requestApprovalOrEnqueue(database, fmt.Sprintf("grep path: %s", path))
			if err != nil {
				return nil, err
			}
			if !approved {
				return &GrepRsp{Matches: ""}, nil
			}
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		matches, err := grepPath(re, path)
		if err != nil {
			return nil, err
		}
		return &GrepRsp{Matches: matches}, nil
	})

	editFileTool := utils.NewTool(&schema.ToolInfo{
		Name: "edit_file",
		Desc: "Replace exact text in a file.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":     {Type: schema.String, Required: true, Desc: "File path"},
			"old_text": {Type: schema.String, Required: true, Desc: "Exact text to replace"},
			"new_text": {Type: schema.String, Required: true, Desc: "Replacement text"},
		}),
	}, func(ctx context.Context, req *EditFileReq) (*EditFileRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		path, err := safePath(req.Path)
		if err != nil {
			return nil, err
		}
		if requireApproval {
			approved, err := requestApprovalOrEnqueue(database, fmt.Sprintf("edit file: %s", path))
			if err != nil {
				return nil, err
			}
			if !approved {
				return &EditFileRsp{Result: "user rejected edit"}, nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		oldText := req.OldText
		if oldText == "" {
			return nil, fmt.Errorf("old_text is required")
		}
		content := string(data)
		if !strings.Contains(content, oldText) {
			return nil, fmt.Errorf("old_text not found in file")
		}
		updated := strings.Replace(content, oldText, req.NewText, 1)
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}
		return &EditFileRsp{Result: "updated"}, nil
	})

	writeFileTool := utils.NewTool(&schema.ToolInfo{
		Name: "write_file",
		Desc: "Write content to a file (overwrite).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Required: true, Desc: "File path"},
			"content": {Type: schema.String, Required: true, Desc: "File content"},
		}),
	}, func(ctx context.Context, req *WriteFileReq) (*WriteFileRsp, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		path, err := safePath(req.Path)
		if err != nil {
			return nil, err
		}
		if requireApproval {
			approved, err := requestApprovalOrEnqueue(database, fmt.Sprintf("write file: %s", path))
			if err != nil {
				return nil, err
			}
			if !approved {
				return &WriteFileRsp{Result: "user rejected write"}, nil
			}
		}
		if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}
		return &WriteFileRsp{Result: "written"}, nil
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
		if requireApproval {
			approved, err := requestApprovalOrEnqueue(database, cmdLine)
			if err != nil {
				return nil, err
			}
			if !approved {
				return &RunCommandRsp{Stdout: "", Stderr: "user rejected command", ExitCode: 130}, nil
			}
		}

		cwd := strings.TrimSpace(req.Cwd)
		var (
			cmdID            string
			startOffset      int64
			transcriptWriter *recorder.TranscriptWriter
		)
		if database != nil {
			writer, err := recorder.NewTranscriptWriter(config.TranscriptsDir())
			if err != nil {
				return nil, fmt.Errorf("create transcript: %w", err)
			}
			transcriptWriter = writer
			startOffset = writer.Offset()
			cmdID = uuid.New().String()
			resolvedCwd := cwd
			if resolvedCwd == "" {
				if wd, err := os.Getwd(); err == nil {
					resolvedCwd = wd
				}
			}
			cmd := &db.Command{
				ID:          cmdID,
				TsStart:     time.Now().UnixNano(),
				Command:     cmdLine,
				Cwd:         resolvedCwd,
				StartOffset: &startOffset,
			}
			if err := database.CreateCommand(cmd); err != nil {
				_ = transcriptWriter.Close()
				return nil, fmt.Errorf("create command: %w", err)
			}
		}

		stdout, stderr, exitCode, err := runShellCommand(ctx, cmdLine, cwd, transcriptWriter)
		sessionEnd := time.Now().UnixNano()

		if transcriptWriter != nil && database != nil {
			endOffset := transcriptWriter.Offset()
			outputSize := endOffset - startOffset
			transcriptPath := transcriptWriter.Path()
			if updateErr := database.UpdateCommandEnd(cmdID, sessionEnd, exitCode, endOffset, outputSize, &transcriptPath); updateErr != nil {
				_ = transcriptWriter.Close()
				return nil, fmt.Errorf("update command: %w", updateErr)
			}
			if closeErr := transcriptWriter.Close(); closeErr != nil {
				return nil, fmt.Errorf("close transcript: %w", closeErr)
			}
			notifyCommandExecuted()
		}

		if queueErr := enqueueHistoryCommand(cmdLine); queueErr != nil {
			return nil, queueErr
		}
		if err != nil {
			return nil, err
		}
		return &RunCommandRsp{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}, nil
	})

	return []tool.BaseTool{
		queryCommandsTool,
		getCommandOutputTool,
		searchHistoryTool,
		runCommandTool,
		readFileTool,
		grepTool,
		editFileTool,
		writeFileTool,
	}
}

func safePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	return abs, nil
}

func grepPath(re *regexp.Regexp, path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return grepFile(re, path)
	}

	var builder strings.Builder
	walkErr := filepath.WalkDir(path, func(entryPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		matches, err := grepFile(re, entryPath)
		if err != nil {
			return err
		}
		if strings.TrimSpace(matches) != "" {
			builder.WriteString(matches)
			if !strings.HasSuffix(matches, "\n") {
				builder.WriteString("\n")
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("grep path: %w", walkErr)
	}

	return builder.String(), nil
}

func grepFile(re *regexp.Regexp, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	var builder strings.Builder
	for i, line := range lines {
		if re.MatchString(line) {
			builder.WriteString(fmt.Sprintf("%s:%d:%s\n", path, i+1, line))
		}
	}
	return builder.String(), nil
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

func requestApprovalOrEnqueue(database *db.DB, message string) (bool, error) {
	approvalMode := strings.ToLower(strings.TrimSpace(os.Getenv("TERMIA_APPROVAL_MODE")))
	if approvalMode == "prompt" {
		return promptCommandApproval(message)
	}
	if os.Getenv("TERMIA_WRAPPED") == "1" && os.Getenv("TERMIA_TUI_ACTIVE") != "1" {
		if _, err := enqueuePendingPrompt(database, message); err != nil {
			return false, err
		}
		return false, nil
	}
	return promptCommandApproval(message)
}

func enqueuePendingPrompt(database *db.DB, content string) (bool, error) {
	if database == nil {
		return false, nil
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false, nil
	}

	sessionID := strings.TrimSpace(os.Getenv("TERMIA_SESSION_ID"))
	if sessionID == "" {
		sessions, err := database.ListAgentSessions(1)
		if err != nil {
			return false, err
		}
		if len(sessions) == 0 {
			return false, nil
		}
		sessionID = sessions[0].ID
	}
	if sessionID == "" {
		return false, nil
	}

	prompt := &db.PendingPrompt{
		PromptID:  uuid.New().String(),
		SessionID: sessionID,
		Content:   trimmed,
		CreatedAt: time.Now().UnixNano(),
		Status:    db.PendingPromptStatusPending,
	}
	if err := database.CreatePendingPrompt(prompt); err != nil {
		return false, err
	}
	if err := database.WritePendingPromptsCount(config.PendingPromptsCountPath()); err != nil {
		return true, err
	}
	return true, nil
}

func runShellCommand(ctx context.Context, command string, cwd string, outputWriter io.Writer) (string, string, int, error) {
	shellFlag := "-lc"
	shellPath := strings.TrimSpace(os.Getenv("TERMIA_SHELL"))
	if shellPath == "" {
		shellPath = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if shellPath == "" {
		shellPath = "sh"
	}
	cmd := exec.CommandContext(ctx, shellPath, shellFlag, command)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if outputWriter != nil {
		cmd.Stdout = io.MultiWriter(&stdout, outputWriter)
		cmd.Stderr = io.MultiWriter(&stderr, outputWriter)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
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

func enqueueHistoryCommand(command string) error {
	queue := strings.TrimSpace(os.Getenv("TERMIA_HISTORY_QUEUE"))
	if queue == "" {
		queue = config.HistoryQueuePath()
	}
	if queue == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(queue), 0755); err != nil {
		return fmt.Errorf("create history queue dir: %w", err)
	}
	file, err := os.OpenFile(queue, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open history queue: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(command + "\n"); err != nil {
		return fmt.Errorf("write history queue: %w", err)
	}
	return nil
}
