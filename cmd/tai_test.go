package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agentapp"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

func TestTaiRendererNormalizesToolResultSummaryFromPendingCall(t *testing.T) {
	renderer := newTaiRenderer()
	if _, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "pwd",
		State:    agent.ToolCallStatePending,
	}); ok {
		t.Fatalf("expected pending tool call to stay buffered")
	}
	toolCall, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected final tool result to render")
	}
	if toolCall.Summary != "pwd" {
		t.Fatalf("expected summary to be restored from pending call, got %#v", toolCall)
	}
}

func TestTaiRendererDeduplicatesRepeatedFinalToolState(t *testing.T) {
	renderer := newTaiRenderer()
	first, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "ls",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected first final tool state to render")
	}
	if first.Summary != "ls" {
		t.Fatalf("expected first rendered call to keep summary, got %#v", first)
	}
	if _, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "ls",
		State:    agent.ToolCallStateSuccess,
	}); ok {
		t.Fatalf("expected repeated final tool state to be suppressed")
	}
}

func TestTaiRendererSkipsRequestInputToolLines(t *testing.T) {
	renderer := newTaiRenderer()
	if _, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-2",
		ToolName: "request_input",
		Summary:  "requested user input",
		Result:   "answered 1 question",
		State:    agent.ToolCallStateSuccess,
	}); ok {
		t.Fatalf("expected request_input tool lines to be suppressed in tai")
	}
}

func TestTaiRendererLatestToolStateWinsBeforeFlush(t *testing.T) {
	renderer := newTaiRenderer()
	errorCall, key, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-3",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "exit 1",
		State:    agent.ToolCallStateError,
	})
	if !ok {
		t.Fatalf("expected error state to buffer")
	}
	renderer.bufferedToolOrder = append(renderer.bufferedToolOrder, key)
	renderer.bufferedTools[key] = errorCall

	successCall, key, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-3",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "ok",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected success state to replace buffered error")
	}
	renderer.bufferedTools[key] = successCall

	got := renderer.bufferedTools[key]
	if got.State != agent.ToolCallStateSuccess {
		t.Fatalf("expected latest buffered state to win, got %#v", got)
	}
}

func TestTaiRendererUsesDisplayKeyToCollapseDuplicateCallIDs(t *testing.T) {
	renderer := newTaiRenderer()
	first, key, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-a",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "exit 1",
		State:    agent.ToolCallStateError,
	})
	if !ok {
		t.Fatalf("expected first tool event to render")
	}
	renderer.bufferedToolOrder = append(renderer.bufferedToolOrder, key)
	renderer.bufferedTools[key] = first

	second, secondKey, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-b",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "ok",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected second tool event to render")
	}
	if secondKey != key {
		t.Fatalf("expected duplicate display key, got %q vs %q", secondKey, key)
	}
	if !shouldReplaceBufferedTool(renderer.bufferedTools[key], second) {
		t.Fatalf("expected successful replacement to win over earlier error")
	}
	renderer.bufferedTools[key] = second

	if len(renderer.bufferedToolOrder) != 1 {
		t.Fatalf("expected one buffered line, got %#v", renderer.bufferedToolOrder)
	}
	if got := renderer.bufferedTools[key]; got.State != agent.ToolCallStateSuccess {
		t.Fatalf("expected latest displayed state to be success, got %#v", got)
	}
}

func TestRenderTaiToolCallKeepsCommandLineCompact(t *testing.T) {
	line := renderTaiToolCall(agent.ToolCallEvent{
		ToolName: "command",
		Summary:  "ls",
		Result:   "ok",
		State:    agent.ToolCallStateSuccess,
	})
	if !strings.Contains(line, "command ls") {
		t.Fatalf("expected command summary in rendered line, got %q", line)
	}
	if strings.Contains(line, "· ok") {
		t.Fatalf("expected success marker to stay compact, got %q", line)
	}
}

func TestNormalizeTaiHistoryModeDefaultsToCmd(t *testing.T) {
	mode, err := normalizeTaiHistoryMode("")
	if err != nil {
		t.Fatalf("expected default history mode, got error: %v", err)
	}
	if mode != "cmd" {
		t.Fatalf("expected default history mode cmd, got %q", mode)
	}
}

func TestFilterTaiHistoryCmdExcludesTaiCommands(t *testing.T) {
	commands := []db.Command{
		{Command: "tai 问我一个多选问题"},
		{Command: "ls"},
		{Command: "termia tui"},
		{Command: "pwd"},
	}
	filtered := filterTaiHistory(commands, "cmd")
	if len(filtered) != 2 {
		t.Fatalf("expected only non-AI commands, got %#v", filtered)
	}
	if strings.TrimSpace(filtered[0].Command) != "ls" || strings.TrimSpace(filtered[1].Command) != "pwd" {
		t.Fatalf("expected non-AI command order preserved, got %#v", filtered)
	}
}

func TestFilterTaiHistoryCmdExcludesOnlyCurrentTermiaBinaryPath(t *testing.T) {
	cwd := t.TempDir()
	t.Setenv("TERMIA_BIN", filepath.Join(cwd, "bin", "custom-launcher"))

	commands := []db.Command{
		{Command: "./bin/custom-launcher", Cwd: cwd},
		{Command: "./other/custom-launcher", Cwd: cwd},
		{Command: "pwd", Cwd: cwd},
	}

	filtered := filterTaiHistory(commands, "cmd")
	if len(filtered) != 2 {
		t.Fatalf("expected only the active Termia launcher command to be hidden, got %#v", filtered)
	}
	if strings.TrimSpace(filtered[0].Command) != "./other/custom-launcher" || strings.TrimSpace(filtered[1].Command) != "pwd" {
		t.Fatalf("expected same-name command in a different path to remain visible, got %#v", filtered)
	}
}

func TestResolveTaiHistoryRecentUsesWrapperStartTimeAndCapsAtTwentyCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "termia.db")
	wrapperStartedAt := time.Now().UnixNano()
	seedTaiTestDB(t, dbPath, func(database *db.DB) {
		createTaiTestCommand(t, database, "cmd-old", "pwd", wrapperStartedAt-1, false)
		createTaiTestCommand(t, database, "cmd-ai", "tai explain", wrapperStartedAt+1, false)
		for i := 1; i <= 21; i++ {
			createTaiTestCommand(
				t,
				database,
				fmt.Sprintf("cmd-%02d", i),
				fmt.Sprintf("echo %02d", i),
				wrapperStartedAt+int64(i+1),
				false,
			)
		}
	})

	prevCmdCount, prevRecent, prevHistoryMode, prevNewSession := taiCommandCount, taiRecent, taiHistoryMode, taiNewSession
	taiCommandCount = 0
	taiRecent = true
	taiHistoryMode = "cmd"
	taiNewSession = false
	t.Cleanup(func() {
		taiCommandCount = prevCmdCount
		taiRecent = prevRecent
		taiHistoryMode = prevHistoryMode
		taiNewSession = prevNewSession
	})
	t.Setenv("TERMIA_WRAPPER_STARTED_AT", strconv.FormatInt(wrapperStartedAt, 10))

	withTaiTestDB(t, dbPath, func(database *db.DB) {
		commands, err := resolveTaiHistory(newTaiRunTestCommand(), database)
		if err != nil {
			t.Fatalf("resolveTaiHistory returned error: %v", err)
		}
		if len(commands) != 20 {
			t.Fatalf("expected recent history to be capped at 20 commands, got %d (%#v)", len(commands), commands)
		}
		if commands[0].ID != "cmd-02" || commands[len(commands)-1].ID != "cmd-21" {
			t.Fatalf("expected newest 20 startup commands in chronological order, got first=%q last=%q", commands[0].ID, commands[len(commands)-1].ID)
		}
		for _, command := range commands {
			if command.ID == "cmd-old" {
				t.Fatalf("expected commands before wrapper startup to be excluded, got %#v", commands)
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(command.Command)), "tai ") {
				t.Fatalf("expected ai commands to be filtered by default history mode, got %#v", commands)
			}
		}
	})
}

func TestTaiRunUsesSharedAgentAppServiceAndStoresAnalysis(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "termia.db")
	seedTaiTestDB(t, dbPath, func(database *db.DB) {
		now := time.Now().UnixNano()
		createTaiTestCommand(t, database, "cmd-ai", "tai explain", now+2, false)
		createTaiTestCommand(t, database, "cmd-shell", "git status", now+1, true)
	})

	prevOpen := openTaiDB
	openTaiDB = func() (*db.DB, error) {
		return db.Open(dbPath, zap.NewNop())
	}
	t.Cleanup(func() {
		openTaiDB = prevOpen
	})

	fakeSvc := &fakeTaiAgentAppService{
		events: []agent.RuntimeEvent{
			{Kind: agent.RuntimeEventText, Text: "analysis result"},
		},
	}
	prevFactory := newTaiAgentAppService
	newTaiAgentAppService = func(_ *config.Config, _ *db.DB) taiAgentAppService {
		return fakeSvc
	}
	t.Cleanup(func() {
		newTaiAgentAppService = prevFactory
	})

	prevCmdCount, prevRecent, prevHistoryMode, prevNewSession := taiCommandCount, taiRecent, taiHistoryMode, taiNewSession
	taiCommandCount = 1
	taiRecent = false
	taiHistoryMode = "cmd"
	taiNewSession = false
	t.Cleanup(func() {
		taiCommandCount = prevCmdCount
		taiRecent = prevRecent
		taiHistoryMode = prevHistoryMode
		taiNewSession = prevNewSession
	})

	t.Setenv("TERMIA_SESSION_ID", "session-current")

	cmd := newTaiRunTestCommand()
	if err := taiRun(cmd, []string{"summarize"}); err != nil {
		t.Fatalf("taiRun returned error: %v", err)
	}
	if len(fakeSvc.requests) != 1 {
		t.Fatalf("expected one service run request, got %d", len(fakeSvc.requests))
	}
	req := fakeSvc.requests[0]
	if req.Query != "summarize" {
		t.Fatalf("expected query to pass through unchanged, got %q", req.Query)
	}
	if req.SessionID != "session-current" {
		t.Fatalf("expected current session ID to be forwarded, got %q", req.SessionID)
	}
	if len(req.SelectedCommands) != 1 || req.SelectedCommands[0].ID != "cmd-shell" {
		t.Fatalf("expected selected shell history to be forwarded, got %#v", req.SelectedCommands)
	}
	if !req.SelectedCommands[0].TranscriptAvailable {
		t.Fatalf("expected transcript availability to be forwarded from db command")
	}

	withTaiTestDB(t, dbPath, func(database *db.DB) {
		analyses, err := database.ListAnalyses(10)
		if err != nil {
			t.Fatalf("ListAnalyses returned error: %v", err)
		}
		if len(analyses) != 1 {
			t.Fatalf("expected one analysis row, got %#v", analyses)
		}
		if analyses[0].Prompt != "summarize" || analyses[0].Response != "analysis result" {
			t.Fatalf("expected analysis row to store prompt/response, got %#v", analyses[0])
		}
		if analyses[0].CommandIDs != `["cmd-shell"]` {
			t.Fatalf("expected selected command IDs to persist, got %s", analyses[0].CommandIDs)
		}
	})
}

func TestTaiRunSupportsHSelectorPrefixWithSharedService(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "termia.db")
	seedTaiTestDB(t, dbPath, func(database *db.DB) {
		now := time.Now().UnixNano()
		createTaiTestCommand(t, database, "cmd-1", "pwd", now+1, false)
		createTaiTestCommand(t, database, "cmd-2", "ls", now+2, false)
	})

	prevOpen := openTaiDB
	openTaiDB = func() (*db.DB, error) {
		return db.Open(dbPath, zap.NewNop())
	}
	t.Cleanup(func() {
		openTaiDB = prevOpen
	})

	fakeSvc := &fakeTaiAgentAppService{}
	prevFactory := newTaiAgentAppService
	newTaiAgentAppService = func(_ *config.Config, _ *db.DB) taiAgentAppService {
		return fakeSvc
	}
	t.Cleanup(func() {
		newTaiAgentAppService = prevFactory
	})

	prevCmdCount, prevRecent, prevHistoryMode, prevNewSession := taiCommandCount, taiRecent, taiHistoryMode, taiNewSession
	taiCommandCount = 0
	taiRecent = false
	taiHistoryMode = "cmd"
	taiNewSession = false
	t.Cleanup(func() {
		taiCommandCount = prevCmdCount
		taiRecent = prevRecent
		taiHistoryMode = prevHistoryMode
		taiNewSession = prevNewSession
	})

	runCmd := &cobra.Command{
		Use:  "tai",
		Args: cobra.MinimumNArgs(1),
		RunE: taiRun,
	}
	runCmd.SetArgs([]string{"h~2", "why"})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("command execution returned error: %v", err)
	}
	if len(fakeSvc.requests) != 1 {
		t.Fatalf("expected one service run request, got %d", len(fakeSvc.requests))
	}
	req := fakeSvc.requests[0]
	if req.Query != "why" {
		t.Fatalf("expected h-selector to be stripped from query, got %q", req.Query)
	}
	if len(req.SelectedCommands) != 2 {
		t.Fatalf("expected two selected commands from h~2, got %#v", req.SelectedCommands)
	}
	if req.SelectedCommands[0].ID != "cmd-1" || req.SelectedCommands[1].ID != "cmd-2" {
		t.Fatalf("expected commands in chronological order, got %#v", req.SelectedCommands)
	}
}

func TestTaiRunRequestsNewSessionFromSharedService(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "termia.db")

	prevOpen := openTaiDB
	openTaiDB = func() (*db.DB, error) {
		return db.Open(dbPath, zap.NewNop())
	}
	t.Cleanup(func() {
		openTaiDB = prevOpen
	})

	fakeSvc := &fakeTaiAgentAppService{
		events: []agent.RuntimeEvent{
			{Kind: agent.RuntimeEventText, Text: "fresh session"},
		},
	}
	prevFactory := newTaiAgentAppService
	newTaiAgentAppService = func(_ *config.Config, _ *db.DB) taiAgentAppService {
		return fakeSvc
	}
	t.Cleanup(func() {
		newTaiAgentAppService = prevFactory
	})

	prevCmdCount, prevRecent, prevHistoryMode, prevNewSession := taiCommandCount, taiRecent, taiHistoryMode, taiNewSession
	taiCommandCount = 0
	taiRecent = false
	taiHistoryMode = "cmd"
	taiNewSession = true
	t.Cleanup(func() {
		taiCommandCount = prevCmdCount
		taiRecent = prevRecent
		taiHistoryMode = prevHistoryMode
		taiNewSession = prevNewSession
	})

	t.Setenv("TERMIA_SESSION_ID", "session-current")

	cmd := newTaiRunTestCommand()
	if err := taiRun(cmd, []string{"start fresh"}); err != nil {
		t.Fatalf("taiRun returned error: %v", err)
	}
	if len(fakeSvc.requests) != 1 {
		t.Fatalf("expected one service run request, got %d", len(fakeSvc.requests))
	}
	if !fakeSvc.requests[0].NewSession {
		t.Fatalf("expected tai -n to request a new session, got %#v", fakeSvc.requests[0])
	}
}

func TestTaiRunReturnsErrorOnStreamErrorAndSkipsAnalysis(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "termia.db")

	prevOpen := openTaiDB
	openTaiDB = func() (*db.DB, error) {
		return db.Open(dbPath, zap.NewNop())
	}
	t.Cleanup(func() {
		openTaiDB = prevOpen
	})

	fakeSvc := &fakeTaiAgentAppService{
		events: []agent.RuntimeEvent{
			{Kind: agent.RuntimeEventError, Text: "runtime failed"},
		},
	}
	prevFactory := newTaiAgentAppService
	newTaiAgentAppService = func(_ *config.Config, _ *db.DB) taiAgentAppService {
		return fakeSvc
	}
	t.Cleanup(func() {
		newTaiAgentAppService = prevFactory
	})

	prevCmdCount, prevRecent, prevHistoryMode, prevNewSession := taiCommandCount, taiRecent, taiHistoryMode, taiNewSession
	taiCommandCount = 0
	taiRecent = false
	taiHistoryMode = "cmd"
	taiNewSession = false
	t.Cleanup(func() {
		taiCommandCount = prevCmdCount
		taiRecent = prevRecent
		taiHistoryMode = prevHistoryMode
		taiNewSession = prevNewSession
	})

	cmd := newTaiRunTestCommand()
	err := taiRun(cmd, []string{"inspect failure"})
	if err == nil {
		t.Fatalf("expected taiRun to return stream error")
	}
	if !strings.Contains(err.Error(), "runtime failed") {
		t.Fatalf("expected returned error to include runtime failure text, got %v", err)
	}

	withTaiTestDB(t, dbPath, func(database *db.DB) {
		analyses, listErr := database.ListAnalyses(10)
		if listErr != nil {
			t.Fatalf("ListAnalyses returned error: %v", listErr)
		}
		if len(analyses) != 0 {
			t.Fatalf("expected no analysis rows on stream error, got %#v", analyses)
		}
	})
}

func TestTaiNormalizeToolCallNormalizesInlineFields(t *testing.T) {
	toolCall := taiNormalizeToolCall(agent.ToolCallEvent{
		CallID:    " call-1 ",
		AgentName: " assistant\r ",
		ToolName:  " command\r ",
		Summary:   " netstat\r\n-tuln ",
		Result:    " open\r ports ",
	})
	if toolCall.CallID != "call-1" {
		t.Fatalf("expected trimmed call id, got %q", toolCall.CallID)
	}
	if toolCall.AgentName != "assistant" {
		t.Fatalf("expected normalized agent name, got %q", toolCall.AgentName)
	}
	if toolCall.ToolName != "command" {
		t.Fatalf("expected normalized tool name, got %q", toolCall.ToolName)
	}
	if toolCall.Summary != "netstat -tuln" {
		t.Fatalf("expected normalized summary, got %q", toolCall.Summary)
	}
	if toolCall.Result != "open ports" {
		t.Fatalf("expected normalized result, got %q", toolCall.Result)
	}
}

type fakeTaiAgentAppService struct {
	requests []agentapp.RunRequest
	events   []agent.RuntimeEvent
	err      error
}

func (f *fakeTaiAgentAppService) Run(ctx context.Context, req agentapp.RunRequest) (<-chan agent.RuntimeEvent, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	stream := make(chan agent.RuntimeEvent, len(f.events))
	for _, event := range f.events {
		select {
		case <-ctx.Done():
			close(stream)
			return stream, nil
		case stream <- event:
		}
	}
	close(stream)
	return stream, nil
}

func newTaiRunTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "tai"}
	cmd.Flags().Int("cmd", 0, "")
	cmd.Flags().BoolP("recent", "r", false, "")
	cmd.Flags().String("history-mode", "cmd", "")
	cmd.Flags().BoolP("new-session", "n", false, "")
	return cmd
}

func seedTaiTestDB(t *testing.T, dbPath string, seed func(*db.DB)) {
	t.Helper()
	withTaiTestDB(t, dbPath, seed)
}

func withTaiTestDB(t *testing.T, dbPath string, fn func(*db.DB)) {
	t.Helper()
	database, err := db.Open(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer func() {
		_ = database.Close()
	}()
	fn(database)
}

func createTaiTestCommand(t *testing.T, database *db.DB, id, command string, tsEnd int64, withTranscript bool) {
	t.Helper()
	var transcriptPath *string
	if withTranscript {
		path := "/tmp/transcript.log"
		transcriptPath = &path
	}
	if err := database.CreateCommand(&db.Command{
		ID:             id,
		TsStart:        tsEnd - int64(time.Second),
		TsEnd:          ptrInt64(tsEnd),
		DurationMs:     ptrInt64(100),
		Command:        command,
		Cwd:            "/tmp/project",
		TranscriptPath: transcriptPath,
	}); err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
