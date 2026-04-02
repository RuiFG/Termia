# Unified Agent Session And Middleware Design

## Goal
Make `tai` and `tui` two adapters over the same session-centric agent application. Remove duplicated session, message, and timeline logic from the adapters. Add middleware as a first-class concept so slash commands can install run-scoped or session-scoped behavior without coupling that behavior to either adapter.

## Current Problems
- `cmd/tai` and `internal/tui` each maintain their own session creation, message normalization, timeline merge, and runtime event handling.
- `internal/agent/agent.go` mixes runtime orchestration, prompt rendering, event projection, and tool summarization into one file.
- Slash commands currently live inside TUI-only code, which makes agent extensions UI-bound instead of application-bound.

These problems create drift: the same feature has to be implemented twice, and adapter code keeps owning agent-domain logic.

## Core Decisions
1. There is one logical `AgentSession`.
   `tai` and `tui` both read and write the same active session via `internal/sessionstate` and the database.
2. `internal/agent` becomes runtime-only.
   It owns prompt construction, model calls, tools, HITL, and runtime events. It does not own persistence or adapter policy.
3. A new `internal/agentapp` package becomes the application layer.
   It owns session resolution, conversation loading, middleware activation, runtime execution, timeline reduction, and message persistence.
4. Middleware scope is explicit at registration time.
   Each slash command that installs middleware declares `run` or `session`; adapters do not guess.
5. TUI-local slash commands stay local.
   Commands like `/exit` remain in TUI. Shared agent commands and middleware commands are defined in `internal/agentapp`.

## Target Package Boundaries
### `internal/agent`
- Runtime only
- `Runtime.Run(...)`
- Prompt builders
- Runtime event extraction and tool call/result summarization
- Model providers and tools

### `internal/agentapp`
- Session state encode/decode
- Active-session resolution and creation
- Shared timeline reducer
- Middleware registry and middleware execution
- Shared slash command registry for agent commands
- Unified `Service.Run(...)` used by both `tai` and `tui`

### `cmd/tai`
- CLI input parsing
- Terminal rendering of returned events
- No session bootstrap logic
- No timeline persistence logic

### `internal/tui`
- Session list and panel rendering
- UI-local slash commands
- Calls `agentapp.Service`
- No duplicate timeline reducer or message normalization rules

## Session Model
The database session record remains the source of identity, creation time, and cwd. The existing `spec_snapshot_json` field is reused as serialized application state for this refactor to avoid schema churn during the cutover. That JSON now stores:

```json
{
  "mode": "assistant",
  "team_name": "",
  "session_middleware": [
    { "name": "example", "scope": "session", "args": { "k": "v" } }
  ]
}
```

This keeps the single truth in one place without forcing `cmd/tai` or `internal/tui` to understand the field layout.

## Run Model
`agentapp.Service.Run(...)` is the only entrypoint for agent execution. Its responsibilities are:
- Resolve or create the active session
- Load current conversation history
- Persist the new user message
- Build the effective middleware chain
- Start `agent.Runtime.Run(...)`
- Reduce runtime events into timeline messages
- Persist assistant/tool/error output once per run
- Update cwd when the command tool changes it

Both adapters receive the same runtime events and render them differently, but neither adapter owns the business rules around those events.

## Middleware Model
Middleware is declared in `internal/agentapp` with explicit scope:

```go
type MiddlewareScope string

const (
    MiddlewareScopeRun     MiddlewareScope = "run"
    MiddlewareScopeSession MiddlewareScope = "session"
)
```

Each registration declares:
- name
- description
- scope
- factory

The runtime-facing hook surface is intentionally small:
- `BeforeRun(...)`
- `AfterRun(...)`

`AfterRun(...)` returns a directive. That directive allows follow-up behavior such as:
- continue the run with a new query
- emit a final assistant message
- stop immediately

This is sufficient for `/ralph-loop`: it is registered as run-scoped, and after a run finishes it asks the application to continue if the run executed a command tool call. When a run finishes without command execution, it emits `已完成` and stops.

## Slash Command Model
Slash commands split into two categories:

### Adapter-local
- `/exit`
- palette or focus commands
- UI-only actions

These stay in `internal/tui`.

### Shared agent commands
- middleware installation or removal
- agent behavior modifiers
- future run/session agent controls

These live in `internal/agentapp/slash.go` and are available to both `tai` and `tui`.

Inference from requirements: shared agent slash commands should behave the same in both adapters, because `tai` is a shortcut into the same session rather than a separate workflow.

## Timeline Model
The current duplicated timeline reducers in `cmd/tai_session.go` and `internal/tui/agent.go` are replaced by one shared reducer in `internal/agentapp/timeline.go`. It owns:
- role normalization
- adjacent text merge rules
- tool call/result upsert rules
- marking latest pending tool failure

TUI keeps rendering-specific types if needed, but the merge semantics live in one place.

## Non-Goals
- Preserving adapter-owned duplicate helpers
- Keeping `cmd/tai_session.go` as a second session application layer
- Expanding middleware hooks beyond what current use cases need
- Moving UI rendering into the application layer

## Acceptance Criteria
- Running `tai "..."` writes to the same active session shown in TUI.
- TUI immediately shows messages produced by the latest `tai` run after reloading that session.
- Session middleware persists with the session and affects later runs from both adapters.
- Run middleware affects only the current execution.
- Shared slash commands are resolved consistently in both adapters.
- `internal/agent` no longer owns persistence or adapter-specific rules.
- Timeline merge semantics exist in exactly one shared reducer.
