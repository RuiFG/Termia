# Repository Guidelines

## Project Structure & Module Organization
`main.go` is the CLI entrypoint. Cobra commands live in `cmd/`. Core application code is under `internal/`, with major areas including `internal/agent/` for runtime orchestration, `internal/tui/` for Bubble Tea UI, `internal/db/` for SQLite persistence, `internal/shell/` for shell integration, and `internal/config/` for config defaults and paths. Embedded runtime assets live in `embedded/`; editable source copies live in `scripts/`. User-facing docs belong in `docs/`, and cross-compiled binaries are written to `dist/`.

## Build, Test, and Development Commands
Use standard Go commands from the repo root:

- `go run . --help` shows the CLI surface and is the fastest smoke test.
- `go test ./...` runs the full package test suite.
- `go run build.go` builds for the current platform into `dist/`.
- `go run build.go all` cross-compiles all supported targets.
- `go run build.go clean` removes build artifacts.
- `go run . init zsh` or `go run . doctor` are useful for validating shell integration changes.

## Coding Style & Naming Conventions
Follow idiomatic Go. Format every touched file with `gofmt -w <files>`. Keep package names lowercase, exported identifiers in `PascalCase`, and unexported helpers in `camelCase`. Prefer small package-local helpers over cross-package coupling. Place command wiring in `cmd/` and reusable logic in `internal/`.

When updating shell assets or defaults, keep paired files aligned: `scripts/*` with `embedded/*`, and `internal/config/defaults.go` with `scripts/config.toml`. If schema changes touch `embedded/schema.sql`, also bump `schemaVersion` in `internal/db/db.go`.

## Testing Guidelines
Tests are standard Go `_test.go` files colocated with their packages, for example `internal/tui/*_test.go` and `cmd/*_test.go`. Add focused tests beside the package you change, especially for config parsing, provider logic, session handling, and TUI state transitions. No coverage gate is configured in-repo, so rely on targeted assertions and `go test ./...` before opening a PR.

## Commit & Pull Request Guidelines
Recent history uses Conventional Commit prefixes such as `feat:`, `refactor:`, and `chore(perf):`. Keep commits scoped and descriptive, for example `feat: add provider selection to tui`. PRs should include a short summary, affected packages, test commands run, and screenshots or terminal captures for TUI output changes. Call out config, schema, or embedded-asset updates explicitly.
