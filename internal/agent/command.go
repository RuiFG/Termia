package agent

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/termia/termia/internal/db"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
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
}

const (
	commandStateCwdKey     = "termia.cwd"
	commandStatePrevCwdKey = "termia.prev_cwd"
	commandCwdModeSession  = "session"
	commandCwdModeOverride = "override"
)

func NewCommandTool(database CommandDB) (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:                "command",
			Description:         "Execute a shell command. By default it runs in the session's current working directory. Use cwd_mode=override with cwd only for an explicit one-off directory override.",
			RequireConfirmation: true,
		},
		func(tc adktool.Context, req *CommandToolReq) (*CommandToolRsp, error) {
			cmdLine := strings.TrimSpace(req.Command)
			if cmdLine == "" {
				return nil, fmt.Errorf("command is required")
			}
			cwd := strings.TrimSpace(req.Cwd)
			cwdMode := normalizeCommandCwdMode(req.CwdMode)
			stdout, stderr, exitCode, currentCwd, cwdChanged, err := executeCommand(tc, cmdLine, cwd, cwdMode, database)
			if err != nil {
				return &CommandToolRsp{
					Command:    cmdLine,
					Cwd:        currentCwd,
					CwdChanged: cwdChanged,
					Stdout:     stdout,
					Stderr:     stderr,
					ExitCode:   exitCode,
				}, nil
			}
			return &CommandToolRsp{
				Command:    cmdLine,
				Cwd:        currentCwd,
				CwdChanged: cwdChanged,
				Stdout:     stdout,
				Stderr:     stderr,
				ExitCode:   exitCode,
			}, nil
		},
	)
}

func executeCommand(ctx adktool.Context, command, cwd, cwdMode string, database CommandDB) (string, string, int, string, bool, error) {
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
	resolvedCwd := resolveCommandCwd(ctx, cwd, cwdMode)
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

	if _, err := rememberCommandCwd(ctx, resolvedCwd, false); err != nil {
		return "", "", 1, resolvedCwd, false, err
	}

	if cdReq, ok := parseDirectoryChangeCommand(command); ok {
		targetCwd, stdout, err := applyDirectoryChange(ctx, resolvedCwd, cdReq)
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

func applyDirectoryChange(ctx adktool.Context, currentCwd string, req directoryChangeRequest) (string, string, error) {
	target, err := resolveDirectoryChangeTarget(ctx, currentCwd, req.RawTarget)
	if err != nil {
		return currentCwd, "", err
	}
	if _, err := rememberCommandCwd(ctx, target, true); err != nil {
		return currentCwd, "", err
	}
	stdout := ""
	if strings.TrimSpace(req.RawTarget) == "-" {
		stdout = target
	}
	return target, stdout, nil
}

func resolveDirectoryChangeTarget(ctx adktool.Context, currentCwd, target string) (string, error) {
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
		previous := strings.TrimSpace(readCommandStateString(ctx, commandStatePrevCwdKey))
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

func resolveCommandCwd(ctx adktool.Context, fallback, mode string) string {
	if cwd := effectiveCommandCwd(readCommandStateString(ctx, commandStateCwdKey), fallback, mode); cwd != "" {
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

func rememberCommandCwd(ctx adktool.Context, cwd string, changed bool) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	if !changed {
		if existing := strings.TrimSpace(readCommandStateString(ctx, commandStateCwdKey)); existing != "" {
			return existing, nil
		}
	}
	if changed {
		previous := strings.TrimSpace(readCommandStateString(ctx, commandStateCwdKey))
		if previous != "" && previous != cwd {
			if err := ctx.State().Set(commandStatePrevCwdKey, previous); err != nil {
				return "", err
			}
		}
	}
	if err := ctx.State().Set(commandStateCwdKey, cwd); err != nil {
		return "", err
	}
	return cwd, nil
}

func readCommandStateString(ctx adktool.Context, key string) string {
	if ctx == nil {
		return ""
	}
	value, err := ctx.State().Get(key)
	if err != nil || value == nil {
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
