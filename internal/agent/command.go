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
	"strings"
	"time"

	einoadk "github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"github.com/termia/termia/internal/db"
)

type CommandDB interface {
	CreateCommand(cmd *db.Command) error
	UpdateCommandEnd(id string, tsEnd int64, exitCode int, endOffset, outputSize int64, transcriptPath *string) error
	GetCommand(id string) (*db.Command, error)
}

type CommandToolReq struct {
	Command string `json:"command" jsonschema:"Shell command to execute"`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory"`
	CwdMode string `json:"cwd_mode,omitempty" jsonschema:"Directory handling mode: session|override"`
}

type CommandToolRsp struct {
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	CwdChanged bool   `json:"cwd_changed,omitempty"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Decision   string `json:"decision,omitempty"`
	Status     string `json:"status,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}

const (
	commandStateCwdKey     = "termia.cwd"
	commandStatePrevCwdKey = "termia.prev_cwd"
	commandCwdModeSession  = "session"
	commandCwdModeOverride = "override"
)

func NewCommandTool(database CommandDB, state *runtimeState, requireConfirmation bool) (einotool.BaseTool, error) {
	return toolutils.InferTool("command", "Execute a shell command. By default it runs in the session's current working directory. Use cwd_mode=override with cwd only for an explicit one-off directory override.", func(ctx context.Context, req CommandToolReq) (*CommandToolRsp, error) {
		cmdLine := strings.TrimSpace(req.Command)
		if cmdLine == "" {
			return nil, fmt.Errorf("command is required")
		}

		cwd := strings.TrimSpace(req.Cwd)
		cwdMode := normalizeCommandCwdMode(req.CwdMode)
		currentCwd := resolveCommandCwd(ctx, state, cwd, cwdMode)

		info := &hitlInterruptInfo{
			Kind:         HITLKindConfirm,
			Title:        "Confirmation Required",
			Prompt:       "Approval required.",
			OriginalTool: "command",
			Command:      cmdLine,
			Cwd:          currentCwd,
		}

		if requireConfirmation {
			response, ok, err := resumeHITLResponse[hitlResumeData](ctx, info)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, einotool.Interrupt(ctx, info)
			}
			if !response.Confirmed {
				return &CommandToolRsp{
					Command:  cmdLine,
					Cwd:      currentCwd,
					ExitCode: 1,
					Decision: "rejected",
					Status:   "rejected",
					Reason:   "User rejected this tool call.",
					Message:  "Do not execute the tool. Ask the user for an alternative if needed.",
					Error:    "user rejected this tool call",
				}, nil
			}
		}

		stdout, stderr, exitCode, resolvedCwd, cwdChanged, err := executeCommand(ctx, state, cmdLine, cwd, cwdMode, database)
		result := &CommandToolRsp{
			Command:    cmdLine,
			Cwd:        resolvedCwd,
			CwdChanged: cwdChanged,
			Stdout:     stdout,
			Stderr:     stderr,
			ExitCode:   exitCode,
		}
		if err != nil {
			result.Error = err.Error()
		}
		return result, nil
	})
}

func executeCommand(ctx context.Context, state *runtimeState, command, cwd, cwdMode string, database CommandDB) (string, string, int, string, bool, error) {
	shellFlag := "-lc"
	shellPath := strings.TrimSpace(os.Getenv("TERMIA_SHELL"))
	if shellPath == "" {
		shellPath = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if shellPath == "" {
		shellPath = "sh"
	}

	cmdID := uuid.New().String()
	tsStart := time.Now().UnixNano()
	resolvedCwd := resolveCommandCwd(ctx, state, cwd, cwdMode)
	var startOffset int64

	if database != nil {
		_ = database.CreateCommand(&db.Command{
			ID:          cmdID,
			TsStart:     tsStart,
			Command:     command,
			Cwd:         resolvedCwd,
			StartOffset: &startOffset,
		})
	}

	if _, err := rememberCommandCwd(ctx, state, resolvedCwd, false); err != nil {
		return "", "", 1, resolvedCwd, false, err
	}

	if cdReq, ok := parseDirectoryChangeCommand(command); ok {
		targetCwd, stdout, err := applyDirectoryChange(ctx, state, resolvedCwd, cdReq)
		tsEnd := time.Now().UnixNano()
		if database != nil {
			_ = database.UpdateCommandEnd(cmdID, tsEnd, 0, 0, 0, nil)
		}
		if err != nil {
			return "", "", 1, resolvedCwd, false, err
		}
		return stdout, "", 0, targetCwd, true, nil
	}

	cmd := exec.CommandContext(ctx, shellPath, shellFlag, command)
	if resolvedCwd != "" {
		cmd.Dir = resolvedCwd
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var transcriptPath string

	var writer *transcriptWriter
	if database != nil {
		transcriptDir := getTranscriptDir()
		if err := os.MkdirAll(transcriptDir, 0o755); err == nil {
			transcriptPath = filepath.Join(transcriptDir, fmt.Sprintf("%d.txt", time.Now().UnixNano()))
			if f, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				writer = &transcriptWriter{file: f}
				cmd.Stdout = io.MultiWriter(&stdoutBuf, writer)
				cmd.Stderr = io.MultiWriter(&stderrBuf, writer)
			}
		}
	}
	if cmd.Stdout == nil {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	runErr := cmd.Run()
	tsEnd := time.Now().UnixNano()
	stdout := sanitizeOutput(stdoutBuf.String())
	stderr := sanitizeOutput(stderrBuf.String())

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if database != nil {
		var transcriptPtr *string
		var endOffset int64
		var outputSize int64
		if writer != nil {
			_ = writer.Close()
			endOffset = writer.offset
			outputSize = endOffset - startOffset
			transcriptPtr = &transcriptPath
		}
		_ = database.UpdateCommandEnd(cmdID, tsEnd, exitCode, endOffset, outputSize, transcriptPtr)
	}

	return stdout, stderr, exitCode, resolvedCwd, false, runErr
}

type transcriptWriter struct {
	file   *os.File
	offset int64
}

func (w *transcriptWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	w.offset += int64(n)
	return n, err
}

func (w *transcriptWriter) Close() error {
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func getTranscriptDir() string {
	if dir := os.Getenv("TERMIA_TRANSCRIPTS_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".termia", "transcripts")
	}
	return filepath.Join(os.TempDir(), "termia", "transcripts")
}

type directoryChangeRequest struct {
	RawTarget string
}

func parseDirectoryChangeCommand(command string) (directoryChangeRequest, bool) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return directoryChangeRequest{}, false
	}
	if strings.Contains(trimmed, "&&") || strings.Contains(trimmed, "||") || strings.ContainsAny(trimmed, "|;><`") {
		return directoryChangeRequest{}, false
	}
	if trimmed == "cd" {
		return directoryChangeRequest{}, true
	}
	if !strings.HasPrefix(trimmed, "cd ") {
		return directoryChangeRequest{}, false
	}
	return directoryChangeRequest{RawTarget: strings.TrimSpace(strings.TrimPrefix(trimmed, "cd"))}, true
}

func applyDirectoryChange(ctx context.Context, state *runtimeState, currentCwd string, req directoryChangeRequest) (string, string, error) {
	target, err := resolveDirectoryChangeTarget(ctx, state, currentCwd, req.RawTarget)
	if err != nil {
		return currentCwd, "", err
	}
	if _, err := rememberCommandCwd(ctx, state, target, true); err != nil {
		return currentCwd, "", err
	}
	stdout := ""
	if strings.TrimSpace(req.RawTarget) == "-" {
		stdout = target
	}
	return target, stdout, nil
}

func resolveDirectoryChangeTarget(ctx context.Context, state *runtimeState, currentCwd, target string) (string, error) {
	target = strings.TrimSpace(target)
	target = strings.Trim(target, `"'`)
	if target == "" || target == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return ensureDirectory(home)
	}
	if target == "-" {
		previous := strings.TrimSpace(readCommandStateString(ctx, state, commandStatePrevCwdKey))
		if previous == "" {
			if envPrev := strings.TrimSpace(os.Getenv("OLDPWD")); envPrev != "" {
				return ensureDirectory(envPrev)
			}
			return ensureDirectory(currentCwd)
		}
		return ensureDirectory(previous)
	}
	if strings.HasPrefix(target, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			target = filepath.Join(home, strings.TrimPrefix(target, "~"+string(os.PathSeparator)))
		}
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		base := strings.TrimSpace(currentCwd)
		if base == "" {
			if wd, err := os.Getwd(); err == nil {
				base = wd
			}
		}
		resolved = filepath.Join(base, resolved)
	}
	return ensureDirectory(filepath.Clean(resolved))
}

func ensureDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("directory is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return path, nil
}

func resolveCommandCwd(ctx context.Context, state *runtimeState, fallback, mode string) string {
	if cwd := effectiveCommandCwd(readCommandStateString(ctx, state, commandStateCwdKey), fallback, mode); cwd != "" {
		return cwd
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

func effectiveCommandCwd(stateCwd, fallbackCwd, mode string) string {
	mode = normalizeCommandCwdMode(mode)
	if mode == commandCwdModeOverride {
		if cwd := strings.TrimSpace(fallbackCwd); cwd != "" {
			return cwd
		}
	}
	if cwd := strings.TrimSpace(stateCwd); cwd != "" {
		return cwd
	}
	return strings.TrimSpace(fallbackCwd)
}

func normalizeCommandCwdMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case commandCwdModeOverride:
		return commandCwdModeOverride
	default:
		return commandCwdModeSession
	}
}

func rememberCommandCwd(ctx context.Context, state *runtimeState, cwd string, changed bool) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	if !changed {
		if existing := strings.TrimSpace(readCommandStateString(ctx, state, commandStateCwdKey)); existing != "" {
			return existing, nil
		}
	}
	if changed {
		previous := strings.TrimSpace(readCommandStateString(ctx, state, commandStateCwdKey))
		if previous != "" && previous != cwd {
			einoadk.AddSessionValue(ctx, commandStatePrevCwdKey, previous)
			if state != nil {
				state.set(commandStatePrevCwdKey, previous)
			}
		}
	}
	einoadk.AddSessionValue(ctx, commandStateCwdKey, cwd)
	if state != nil {
		state.set(commandStateCwdKey, cwd)
	}
	return cwd, nil
}

func readCommandStateString(ctx context.Context, state *runtimeState, key string) string {
	value, ok := einoadk.GetSessionValue(ctx, key)
	if (!ok || value == nil) && state != nil {
		value, ok = state.get(key)
	}
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

var ansiSequencePattern = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
var oscPattern = regexp.MustCompile("\x1b\\][^\x1b\x07]*(?:\x1b\\\\|\x07)")
var csiSequencePattern = regexp.MustCompile("\u009b[0-?]*[ -/]*[@-~]")

func sanitizeOutput(raw string) string {
	cleaned := stripANSICodes(raw)
	scanner := bufio.NewScanner(strings.NewReader(cleaned))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}

func stripANSICodes(input string) string {
	cleaned := oscPattern.ReplaceAllString(input, "")
	cleaned = ansiSequencePattern.ReplaceAllString(cleaned, "")
	cleaned = csiSequencePattern.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "\x1b", "")
	return cleaned
}
