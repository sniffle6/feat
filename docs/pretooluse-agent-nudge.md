# PreToolUse Agent Nudge

Docket fires a PreToolUse hook whenever Claude dispatches a subagent via the Agent tool. If docket tracking isn't set up — no active feature or no task items on the active feature — it injects a one-time reminder into the conversation.

## Why it exists

Superpowers' plan execution skills (executing-plans, subagent-driven-development) are prescriptive — they tell Claude exactly what steps to follow. The CLAUDE.md docket instructions get ignored. This hook fires at the exact moment work is about to be dispatched, catching the gap.

## How it works

1. PreToolUse fires on `Agent` tool
2. Checks `.docket/` exists (skips if not a docket project)
3. Checks for `.docket/agent-nudged` sentinel (skips if already nudged)
4. Opens store, checks for in_progress features
5. If no features -> nudge to call `get_ready`
6. If feature exists but zero task items -> nudge to add task items
7. If feature has task items -> silent pass-through
8. Writes sentinel after nudging (one nudge per session)

The sentinel is cleared on SessionStart, so each new session gets a fresh check.

## Superpowers detection

When `docket.exe update` runs, it checks `~/.claude/plugins/installed_plugins.json` for superpowers. If found, the CLAUDE.md snippet includes an extra paragraph about setting up docket before plan execution.

## Key files

- `cmd/docket/hook.go` — `handlePreToolUse` function, sentinel logic
- `plugin/hooks/hooks.json` — PreToolUse matcher for Agent
- `cmd/docket/update.go` — `buildDocketSection`, `detectSuperpowers`
- `cmd/docket/hook_test.go` — PreToolUse test cases
