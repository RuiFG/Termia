# Termia Implementation Progress

**Started**: 2026-02-09
**Last Updated**: 2026-02-10
**Status**: ALL WAVES COMPLETE + AGENT REFACTOR COMPLETE + TUI REWRITE COMPLETE + TUI SINGLE-PANE REFACTOR COMPLETE

---

## Wave 1: Foundation

| ID | Task | Status | Files | Completed |
|----|------|--------|-------|-----------|
| T1 | Project scaffold | done | go.mod, main.go, PROGRESS.md | 2026-02-09 |
| T2 | DB schema | done | scripts/schema.sql | 2026-02-09 |
| T3 | Shell integration | done | scripts/termia.zsh, scripts/termia.bash | 2026-02-09 |
| T4 | Config template | done | scripts/config.toml | 2026-02-09 |

## Wave 2: Core Infrastructure

| ID | Task | Status | Files | Completed |
|----|------|--------|-------|-----------|
| T5 | Config package | done | internal/config/config.go, defaults.go, paths.go | 2026-02-09 |
| T6 | DB package | done | internal/db/db.go, commands.go, analyses.go | 2026-02-09 |
| T7 | Recorder types | done | internal/recorder/types.go | 2026-02-09 |
| T8 | Embedded assets | done | embedded/embed.go | 2026-02-09 |

## Wave 3: Core Components

| ID | Task | Status | Files | Completed |
|----|------|--------|-------|-----------|
| T9 | PTY Wrapper | done | internal/wrapper/wrapper.go, bridge.go, marker.go, signal.go | 2026-02-09 |
| T10 | Recorder engine | done | internal/recorder/recorder.go, transcript.go | 2026-02-09 |
| T11 | Shell package | done | internal/shell/detect.go, integration.go | 2026-02-09 |
| T12 | Cobra root cmd | done | cmd/root.go | 2026-02-09 |

## Wave 4: Features

| ID | Task | Status | Files | Completed |
|----|------|--------|-------|-----------|
| T13 | TUI package | done | internal/tui/app.go, styles.go, history.go, preview.go, search.go, agent.go | 2026-02-09 |
| T14 | Agent package | done | internal/agent/config.go, factory.go, tools.go, runner.go, planner.go, streaming.go (eino) | 2026-02-09 |
| T15 | Main commands | done | cmd/wrap.go, cmd/init_shell.go | 2026-02-09 |
| T16 | Utility commands | done | cmd/status.go, cmd/doctor.go, cmd/clean.go, cmd/config_cmd.go | 2026-02-09 |

## Wave 5: Integration

| ID | Task | Status | Files | Completed |
|----|------|--------|-------|-----------|
| T17 | TUI + AI commands | done | cmd/tui.go, cmd/tai.go (now internal-only) | 2026-02-09 |
| T18 | Final verification | done | PROGRESS.md update, cross-package API fixes | 2026-02-09 |
| T19 | History output + shell shim fixes | done | internal/wrapper/wrapper.go, internal/recorder/recorder.go, internal/tui/output.go, internal/db/db.go, internal/tui/history.go, embedded/termia.bash, embedded/termia.zsh, embedded/termia.bashrc, embedded/termia.zshrc, scripts/termia.bash, scripts/termia.zsh, scripts/termia.bashrc, scripts/termia.zshrc, internal/shell/integration.go | 2026-02-09 |
| T20 | UX polish: TERMIA_BIN, silent logs, no filtering | done | internal/wrapper/wrapper.go, cmd/root.go, cmd/tui.go, embedded/termia.bash, embedded/termia.zsh, scripts/termia.bash, scripts/termia.zsh | 2026-02-10 |
| T21 | Fix phantom commands from PROMPT_COMMAND chain (conda, oh-my-zsh) | done | embedded/termia.bash, scripts/termia.bash | 2026-02-10 |

---

## Summary

- **Total Tasks**: 47 (21 original + 8 agent refactor + 11 TUI rewrite + 3 internal-only + 2 eino + 7 tai fixes + 6 TUI refactor - 11 shared)
- **Completed**: 47
- **In Progress**: 0
- **Pending**: 0
- **Total Files**: 53 (42 Go + 11 non-Go)

## Session 4: Internal-only tui/tai Commands

**Date**: 2026-02-10
**Goal**: Remove external `termia tui`/`termia tai` commands so they only run inside Termia.

| ID | Task | Status | Files Changed |
|----|------|--------|---------------|
| I1 | Gate tui/tai command registration | done | cmd/tui.go, cmd/tai.go |
| I2 | Keep shell functions invoking internal commands | done | embedded/termia.bash, embedded/termia.zsh, scripts/termia.bash, scripts/termia.zsh |
| I3 | Update progress inventory for internal-only commands | done | PROGRESS.md |

## Session 5: Migrate LLM framework to Eino

**Date**: 2026-02-10
**Goal**: Replace trpc-agent-go with cloudwego/eino and implement tai analysis + planning.

| ID | Task | Status | Files Changed |
|----|------|--------|---------------|
| E1 | Swap agent framework to Eino | done | go.mod, go.sum, internal/agent/factory.go, internal/agent/runner.go, internal/agent/planner.go, internal/agent/tools.go |
| E2 | Bind tools + output context for tai | done | cmd/tai.go, internal/agent/tools.go |

## Session 6: tai command tool + history behavior fixes

**Date**: 2026-02-10
**Goal**: Add human-in-the-loop command execution for tai and correct history ordering/defaults.

| ID | Task | Status | Files Changed |
|----|------|--------|---------------|
| S1 | Add command tool with approval prompt | done | internal/agent/tools.go |
| S2 | Add lightweight agent loop for tool calls | done | internal/agent/runner.go |
| S3 | Update tai history parsing (h~N, default none) | done | cmd/tai.go |
| S4 | Update tai prompt + history order behavior | done | internal/agent/config.go, cmd/tai.go |
| S5 | Update PRD/PROGRESS docs | done | PRD.md, PROGRESS.md |
| S6 | Include tai/tui commands in tai history selection | done | cmd/tai.go, PRD.md, PROGRESS.md |
| S7 | Add tai history mode flag (-m cmd|ai|all) | done | cmd/tai.go, PRD.md, PROGRESS.md |

## Session 7: TUI Refactor - Single Pane Layout

**Date**: 2026-02-10
**Goal**: Refactor TUI to a single interface with 3 vertical blocks and focus cycling.

| ID | Task | Status | Files Changed |
|----|------|--------|---------------|
| F1 | Refactor TUI to 3-block vertical layout | done | internal/tui/app.go, styles.go |
| F2 | Remove tab bar UI | done | internal/tui/app.go, tabs.go (removed) |
| F3 | Implement Focus cycling (History -> Content -> Input) | done | internal/tui/app.go, keys.go |
| F4 | Implement explicit 'exit' command | done | internal/tui/commands.go |
| F5 | Maintain slash command functionality | done | internal/tui/input.go, commands.go |
| F6 | Middle pane dynamic switching (Agent/Preview) | done | internal/tui/app.go |

## File Inventory

### Scripts and Config (9 files)
- go.mod - Module definition, Go 1.23, all dependencies
- scripts/schema.sql - SQLite schema (P0+P1 tables, indexes, FTS, triggers)
- scripts/termia.zsh - Zsh integration (preexec/precmd hooks)
- scripts/termia.bash - Bash integration (DEBUG trap, PROMPT_COMMAND)
- scripts/termia.zshrc - Zsh shim rc (sources user rc then Termia)
- scripts/termia.bashrc - Bash shim rc (sources user rc then Termia)
- scripts/config.toml - Default TOML config template
- embedded/termia.zsh - Embedded zsh integration
- embedded/termia.bash - Embedded bash integration
- embedded/termia.zshrc - Embedded zsh shim rc
- embedded/termia.bashrc - Embedded bash shim rc

### Go Source (42 files)
- main.go - Entry point
- embedded/embed.go - go:embed for SQL/scripts/config
- internal/config/config.go - Config structs + Load/Save
- internal/config/defaults.go - DefaultConfig()
- internal/config/paths.go - XDG path helpers
- internal/db/db.go - DB wrapper, Open(), Migrate()
- internal/db/sessions.go - Removed (placeholder)
- internal/db/commands.go - Command CRUD + search
- internal/db/analyses.go - Analysis + AgentExecution CRUD
- internal/recorder/types.go - Marker struct, ParseMarker()
- internal/recorder/recorder.go - Recording engine
- internal/recorder/transcript.go - TranscriptWriter
- internal/wrapper/wrapper.go - PTY wrapper core
- internal/wrapper/bridge.go - I/O bridge with pause/resume
- internal/wrapper/marker.go - Marker FD3 handler
- internal/wrapper/signal.go - SIGWINCH handler
- internal/shell/detect.go - Shell type detection
- internal/shell/integration.go - Script management
- internal/tui/app.go - Bubble Tea App model
- internal/tui/styles.go - Lipgloss styles
- internal/tui/history.go - History view + keybindings
- internal/tui/preview.go - Output preview view
- internal/tui/input.go - Input bar with slash commands
- internal/tui/commands.go - Slash command handlers
- internal/tui/keys.go - Centralized keybindings
- internal/tui/agent.go - Agent mode view (stub)
- internal/tui/output.go - Transcript output loader
- internal/agent/config.go - AgentConfig, provider resolution
- internal/agent/factory.go - Model interface, NewModel()
- internal/agent/tools.go - LLM tools (query_commands, search)
- internal/agent/runner.go - Runner orchestrator
- internal/agent/planner.go - Plan generation + execution
- internal/agent/streaming.go - StreamHandler
- cmd/root.go - Cobra root command
- cmd/wrap.go - termia (default) / termia wrap
- cmd/init_shell.go - termia init [shell]
- cmd/tui.go - tui (internal-only)
- cmd/tai.go - tai / tai agent (internal-only)
- cmd/status.go - termia status
- cmd/doctor.go - termia doctor
- cmd/clean.go - termia clean
- cmd/config_cmd.go - termia config (show/path/edit/reset/set)

### Other
- PROGRESS.md - This file

---

## Session 2: Agent Package Refactor to trpc-agent-go

**Date**: 2026-02-09
**Goal**: Replace custom stub LLM types with real trpc-agent-go framework integration

| ID | Task | Status | Files Changed |
|----|------|--------|---------------|
| R1 | Update go.mod dependency | done | go.mod (v0.2.1 -> v1.1.0) |
| R2 | Rewrite factory.go | done | internal/agent/factory.go |
| R3 | Rewrite tools.go | done | internal/agent/tools.go |
| R4 | Rewrite streaming.go | done | internal/agent/streaming.go |
| R5 | Rewrite runner.go | done | internal/agent/runner.go |
| R6 | Rewrite planner.go | done | internal/agent/planner.go |
| R7 | Update tai.go consumer | done | cmd/tai.go |
| R8 | Update PROGRESS.md | done | PROGRESS.md |

### What Changed

**Removed** (custom stub types):
- `Model` interface, `Message` struct, `ModelInfo` struct
- `StreamEvent` struct, `ToolCallEvent` struct
- `stubModel` struct and its methods
- `Tool` struct (custom name+handler pattern)
- Manual tool call dispatching in `handleToolCall`/`handleToolCalls`

**Added** (trpc-agent-go integration):
- `factory.go`: Uses `openai.New()`, `anthropic.New()`, `ollama.New()` with proper variant/option patterns
- `tools.go`: Uses `function.NewFunctionTool()` with typed `QueryCommandsReq`/`GetCommandOutputReq`/`SearchHistoryReq` structs and `jsonschema` tags
- `runner.go`: Uses `llmagent.New()` + `runner.NewRunner()` + `runner.Run()` for event-driven execution; tool calls handled automatically by framework
- `streaming.go`: `StreamHandler.Process()` now accepts `<-chan *event.Event` instead of `<-chan StreamEvent`; extracts `Delta.Content` for streaming, captures `Usage` stats
- `planner.go`: Uses `RunRaw()` to get raw event stream with custom system prompt for planning; no more direct `Model.GenerateContent()` calls
- `tai.go`: Updated `NewRunner` call signature (5 args), added `defer runner.Close()`

### Provider Mapping
```
"openai"    → openai.New(model)
"deepseek"  → openai.New(model, openai.WithVariant(openai.VariantDeepSeek))
"anthropic" → anthropic.New(model)
"ollama"    → ollama.New(model, ollama.WithHost(baseURL))
```

### Key Imports Used
```go
"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
"trpc.group/trpc-go/trpc-agent-go/event"
"trpc.group/trpc-go/trpc-agent-go/model"
"trpc.group/trpc-go/trpc-agent-go/model/openai"
"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
"trpc.group/trpc-go/trpc-agent-go/model/ollama"
"trpc.group/trpc-go/trpc-agent-go/runner"
"trpc.group/trpc-go/trpc-agent-go/tool"
"trpc.group/trpc-go/trpc-agent-go/tool/function"
```

## Cross-Package Fixes Applied
1. All zap.Logger calls use structured fields (zap.String, zap.Error)
2. wrapper/marker.go - Uses marker.Phase/IsStart()/IsEnd() + db.Command struct
3. wrapper/bridge.go - Fixed WriteOutput to RecordBytes
4. recorder/recorder.go - Fixed to use db.CreateCommand with db.Command struct
5. status.go - Fixed cfg.General.StoragePath to config.DBPath()/ConfigPath()/TermiaDir()
6. doctor.go - Fixed termia_init.sh to shell-specific termia.zsh/termia.bash
7. clean.go - Fixed cfg.General.StoragePath to config.DBPath()
8. wrap.go - Fixed bare-string logger call to zap.Error()

## Session 3: TUI Rewrite — Modern opencode/Claude Code Style

**Date**: 2026-02-10
**Goal**: Complete TUI overhaul with tabbed layout, input bar, slash commands, viewport scrolling, modern styling

| ID | Task | Status | Files Changed |
|----|------|--------|---------------|
| U1 | Add charmbracelet/bubbles dependency | done | go.mod, go.sum |
| U2 | Centralized keybindings | done | internal/tui/keys.go (new) |
| U3 | Modern dark theme with adaptive colors | done | internal/tui/styles.go (rewrite) |
| U4 | Tab bar component | done | internal/tui/tabs.go (new) |
| U5 | Input bar with slash command parsing | done | internal/tui/input.go (new) |
| U6 | Slash command system | done | internal/tui/commands.go (new) |
| U7 | Viewport-based history panel | done | internal/tui/history.go (rewrite) |
| U8 | Viewport-based preview panel | done | internal/tui/preview.go (rewrite) |
| U9 | Agent panel with message log | done | internal/tui/agent.go (rewrite) |
| U10 | Main app orchestrator | done | internal/tui/app.go (rewrite) |
| U11 | Remove old search, update launcher | done | internal/tui/search.go (removed), cmd/tui.go |

### What Changed

**Architecture**:
- Switched from monolithic `App` struct with mode switching to component-based architecture
- Each panel (History, Preview, Agent) is its own model with viewport scrolling
- Tab bar component for panel switching (Tab/Shift+Tab)
- Persistent input bar at bottom with slash command support
- Layout: `[Tab Bar] → [Content Panel] → [Input Bar] → [Status Bar]`

**New Dependencies**:
- `github.com/charmbracelet/bubbles v0.18.0` (textinput, viewport, key)

**New Files**:
- `internal/tui/keys.go` — Centralized KeyMap with all keybindings
- `internal/tui/tabs.go` — Tab bar rendering with active/inactive styles
- `internal/tui/input.go` — InputModel with textinput, slash command parsing, autocomplete hints
- `internal/tui/commands.go` — Slash command definitions (/help, /search, /models, /clear, /quit) and handlers

**Rewritten Files**:
- `internal/tui/styles.go` — Full redesign with adaptive color palette (light/dark), rounded borders, semantic color naming
- `internal/tui/history.go` — HistoryModel with viewport scrolling, proper row rendering with metadata alignment, ensureVisible() for selection tracking
- `internal/tui/preview.go` — PreviewModel with viewport, command metadata header, scroll percentage indicator
- `internal/tui/agent.go` — AgentModel with viewport message log, welcome screen, provider info
- `internal/tui/app.go` — Complete rewrite as orchestrator: tab management, input focus routing, slash command dispatch, panel-level key handling, layout calculation

**Removed Files**:
- `internal/tui/search.go` — Functionality absorbed by input.go (text → search) and commands.go (/search)

**Updated Files**:
- `cmd/tui.go` — Updated `tui.Run()` call to pass full `*config.Config` instead of `*config.TUIConfig`

### Key Design Decisions
1. **Input always visible** — Like opencode/Claude Code, input bar is always at bottom regardless of active tab
2. **Slash commands** — `/help`, `/search <q>`, `/models`, `/clear`, `/quit` with autocomplete hints
3. **Mouse support** — `tea.WithMouseCellMotion()` for viewport scroll
4. **Adaptive colors** — `lipgloss.AdaptiveColor` for light/dark terminal support
5. **Instant UI** — Show chrome immediately, data loads async via `loadCommandsCmd`
6. **Component isolation** — Each panel manages its own viewport, can be tested independently

### Slash Commands
```
/help, /h          Show available commands and keybindings
/search <q>, /s    Search command history (filters History tab)
/models, /m        Show LLM model configuration (switches to Agent tab)
/clear, /c         Clear current view
/quit, /q          Exit TUI
```

### Keybindings
```
Tab / Shift+Tab    Switch between History/Preview/Agent panels
j/k or ↑↓          Navigate in history, scroll in preview
Enter              Preview selected command output / Submit input
d                  Delete selected command
f                  Toggle favorite
g / G              Jump to top/bottom
Esc                Blur input / go back
Ctrl+C             Force quit
```

## Next Steps (For User)
1. Wire actual PTY logic (creack/pty) in wrapper
2. Add tests
3. Implement agent AI integration (currently stub)
4. Add clipboard copy (y key)
5. Consider adding more slash commands as needed
