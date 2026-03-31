# Docket Auto-Tracking Hooks

Docket automatically tracks session activity using Claude Code lifecycle hooks and background checkpoint summarization.

## What it does

- **Session start**: Injects active feature context (title, status, left_off, next task) into the conversation. Opens a work session linking the Claude session to the active feature.
- **After git commits**: Records each commit hash and message to `.docket/commits.log`.
- **Stop (every turn)**: If meaningful delta exists (commits, errors, failed tests, or substantial text), enqueues a checkpoint job. Never blocks.
- **PreCompact**: Forces a checkpoint before context compression — captures everything before the conversation shrinks.
- **SessionEnd**: Enqueues final checkpoint, writes handoff files with "Last Session" section from accumulated observations, closes the work session.

## How it works

The plugin declares hooks in `plugin/hooks/hooks.json`. Claude Code fires these automatically:

1. `SessionStart` → opens work session, resets transcript offset, injects feature context
2. `PostToolUse` (Bash only) → detects `git commit`, appends to commits.log, auto-imports plan files
3. `PreToolUse` (Agent only) → reminds to set up docket tracking before dispatching subagents
4. `Stop` → parses transcript delta since last checkpoint, enqueues checkpoint job if meaningful, always allows stop
5. `PreCompact` → forces a checkpoint (always enqueues, no threshold check)
6. `SessionEnd` → enqueues final delta, writes handoff files, closes work session

## Background summarizer

A background worker in the MCP server process polls the `checkpoint_jobs` table and calls the Anthropic Messages API (haiku-tier) to generate structured observations:

- **summary**: narrative of what happened
- **blockers**: anything blocking progress
- **dead_ends**: approaches that didn't work
- **decisions**: choices made
- **next_steps**: intent for next session
- **gotchas**: non-obvious discoveries

These observations are written to `checkpoint_observations` and rendered into the "Last Session" section of handoff files.

Set `ANTHROPIC_API_KEY` to enable. Without it, the noop summarizer runs (only mechanical facts captured).

## Meaningful delta threshold

Stop only enqueues a checkpoint if at least one of:
- commits.log has entries
- Transcript contains commits or errors
- Failed test runs detected
- Semantic text >= 300 chars
- Non-trivial user input

## Key files

- `cmd/docket/hook.go` — hook subcommand logic (Stop, PreCompact, SessionEnd, SessionStart, PreToolUse, PostToolUse)
- `cmd/docket/handoff.go` — handoff file renderer with Last Session section
- `internal/checkpoint/worker.go` — background job queue worker
- `internal/checkpoint/anthropic.go` — Anthropic Messages API summarizer
- `internal/transcript/parse.go` — JSONL transcript parser
- `internal/store/worksession.go` — work session store methods
- `internal/store/checkpoint.go` — checkpoint job and observation store methods
- `plugin/hooks/hooks.json` — hook declarations
- `plugin/skills/checkpoint/SKILL.md` — /checkpoint skill
- `plugin/skills/end-session/SKILL.md` — /end-session skill

## Gotchas

- Hooks load at session start. After updating docket, restart Claude Code.
- If a session crashes, stale `commits.log` may exist. SessionStart clears it.
- Multiple in_progress features: all listed, but work session tracks the first one.
- The summarizer has a 30-second timeout per job. Failures are logged and the job is marked failed.
- Transcript path must be provided by Claude Code for parsing to work. If missing, only commits.log-based detection runs.
