# Handoff Files

## What it does

Docket generates structured markdown files at session end so the next session can cold-start with full context. These live at `.docket/handoff/<feature-id>.md`.

## Why it exists

When a Claude Code session ends and a new one starts, the agent loses nuance -- decisions made, approaches rejected, current progress. Handoff files give the fresh session everything it needs without re-reading code or re-discovering state.

## How it works

**Two-tier generation:**

1. **Stop hook (mechanical)** -- every session end, the hook reads the database and writes a structured markdown file with: status, progress, left_off, next 3 tasks, key files, recent sessions, subtask progress. No LLM involved -- instant and free.

2. **Board-manager (enriched)** -- when dispatched after commits, the board-manager reads the mechanical handoff and appends synthesized sections: decisions & context, gotchas, and recommended approach.

**Session start injection:**

The SessionStart hook reads the top in-progress feature's handoff file and injects its full contents as the system message. Other in-progress features get one-line pointers to their handoff files.

**Cleanup:**

The Stop hook deletes handoff files for features that are no longer in_progress.

## Gotchas

- Handoff files are overwritten every session end. Agent-enriched sections are lost and re-generated. This is intentional -- the mechanical baseline is always the source of truth.
- If no handoff file exists (first session for a feature), the SessionStart hook falls back to listing features with left_off text.
- Handoff files are in `.docket/` which should be in `.gitignore`.

## Key files

- `cmd/docket/handoff.go` -- renderHandoff, writeHandoffFile, cleanStaleHandoffs
- `cmd/docket/hook.go` -- Stop handler calls writeHandoffFile; SessionStart reads handoff files
- `internal/store/handoff.go` -- HandoffData struct and GetHandoffData method
- `plugin/agents/board-manager.md` -- agent instructions for enriching handoff files
