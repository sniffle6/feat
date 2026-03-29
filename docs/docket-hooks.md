# Docket Auto-Tracking Hooks

Docket automatically tracks your session activity using Claude Code lifecycle hooks. No manual steps needed.

## What it does

- **Session start**: Injects active feature context (title, status, left_off, next task) into the conversation
- **After git commits**: Records each commit hash and message to `.docket/commits.log`
- **Session end (two-phase)**: Blocks Claude from stopping, prompts it to call `log_session` with a rich AI-generated summary (what was done, key decisions, dead ends, gotchas). On re-trigger, writes handoff files and cleans up.

## How it works

The plugin declares hooks in `plugin/hooks/hooks.json`. Claude Code fires these automatically:

1. `SessionStart` → runs `docket.exe hook` → outputs feature context as systemMessage
2. `PostToolUse` (Bash only) → runs `docket.exe hook` → checks for `git commit`, appends to commits.log
3. `Stop` (first, `stop_hook_active=false`) → if active feature + commits, returns `decision: "block"` with a `reason` prompting Claude to call `log_session` with a rich summary and `update_feature` to set `left_off`
4. `Stop` (re-trigger, `stop_hook_active=true`) → writes handoff files, cleans up commits.log, allows stop

## Why two-phase stop?

The hook binary can't see the conversation — it only knows commit hashes and messages. By blocking the first stop attempt and prompting Claude via the `reason` field, the AI generates the summary using its full conversation context: what it worked on, what it tried that didn't work, key decisions, and gotchas. This produces far richer session logs than the old mechanical approach ("3 commit(s): feat: add X; fix: Y").

## Key files

- `cmd/docket/hook.go` — hook subcommand logic (handleStop uses stopHookOutput with decision/reason)
- `plugin/hooks/hooks.json` — hook declarations
- `install.sh` — installs hooks and replaces binary path placeholder

## Gotchas

- Hooks load at session start. After updating docket, restart Claude Code.
- If a session crashes, stale `commits.log` may exist. SessionStart clears it.
- Multiple in_progress features: all are listed, but next task is only shown for the first one.
- The Stop hook only blocks when there's an active feature AND commits. Sessions with no commits pass through immediately.
- The `stop_hook_active` field prevents infinite loops — the hook always allows stop on re-trigger.
