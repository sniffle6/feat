# Docket Auto-Tracking Hooks

Docket automatically tracks your session activity using Claude Code lifecycle hooks. No manual steps needed.

## What it does

- **Session start**: Injects active feature context (title, status, left_off, next task) into the conversation
- **After git commits**: Records each commit hash and message to `.docket/commits.log`
- **Session end**: Logs the session directly to SQLite (mechanical summary from commits) and cleans up commits.log. No Claude involvement.

## How it works

The plugin declares hooks in `plugin/hooks/hooks.json`. Claude Code fires these automatically:

1. `SessionStart` → runs `docket.exe hook` → outputs feature context as systemMessage
2. `PostToolUse` (Bash only) → runs `docket.exe hook` → checks for `git commit`, appends to commits.log
3. `Stop` → runs `docket.exe hook` → reads commits.log + active features → logs session directly to SQLite, deletes commits.log

## Key files

- `cmd/docket/hook.go` — hook subcommand logic
- `plugin/hooks/hooks.json` — hook declarations
- `install.sh` — installs hooks and replaces binary path placeholder

## Gotchas

- Hooks load at session start. After updating docket, restart Claude Code.
- If a session crashes, stale `commits.log` may exist. SessionStart clears it.
- Multiple in_progress features: all are listed, but next task is only shown for the first one.
- The Stop hook logs sessions mechanically — no LLM involvement, no token cost. Summaries are built from commit messages.
