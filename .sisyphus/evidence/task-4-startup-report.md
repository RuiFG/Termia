# Startup timing bottleneck report (pass 1)

## Environment summary
- OS: Linux
- Shell: zsh
- Source log: `.sisyphus/evidence/task-4-timing-pass-1.log`

## Wrapper startup top 3 slowest steps (startup.*)
1. `startup.db.open` at 12 ms
   - Example: `ts=2026-02-23T00:16:27.704297+08:00 pid=66879 step=startup.db.open elapsed_ms=12 db_path=/Users/klein/Library/Application Support/termia/db/history.db`
2. `startup.db.migrate` at 7 ms
   - Example: `ts=2026-02-23T00:16:27.704179+08:00 pid=66879 step=startup.db.migrate elapsed_ms=7`
3. `startup.shell.detect` at 6 ms
   - Example: `ts=2026-02-23T00:16:27.691251+08:00 pid=66879 step=startup.shell.detect elapsed_ms=6`

Note: `startup.shell.version` is tied at 6 ms.
Example: `ts=2026-02-23T00:16:27.691116+08:00 pid=66879 step=startup.shell.version elapsed_ms=6`

## TUI init top 3 slowest steps (tui.*)
1. `tui.db.list_sessions` at 2 ms
   - Example: `ts=2026-02-23T00:16:30.323302+08:00 pid=66879 step=tui.db.list_sessions elapsed_ms=2`
2. `tui.db.list_commands` at 1 ms
   - Example: `ts=2026-02-23T00:16:30.322585+08:00 pid=66879 step=tui.db.list_commands elapsed_ms=1`
3. `tui.term.restore` at 0 ms
   - Example: `ts=2026-02-23T00:16:30.317093+08:00 pid=66879 step=tui.term.restore elapsed_ms=0`

Note: `tui.init.pending_prompt_count` is tied at 0 ms.
Example: `ts=2026-02-23T00:16:30.32081+08:00 pid=66879 step=tui.init.pending_prompt_count elapsed_ms=0`

## Next optimization candidates
- `startup.db.open` and `startup.db.migrate` are the largest startup costs at 12 ms and 7 ms.
- `startup.shell.detect` and `startup.shell.version` are tied at 6 ms each and happen back to back.
- TUI DB queries `tui.db.list_sessions` and `tui.db.list_commands` are small but top the TUI list at 2 ms and 1 ms.
