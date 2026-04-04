# Handoff Files

## What it does

Docket generates structured markdown files at session end so the next session can cold-start with full context. These live at `.docket/handoff/<feature-id>.md`.

## Why it exists

When a Claude Code session ends and a new one starts, the agent loses nuance -- decisions made, approaches rejected, current progress. Handoff files give the fresh session everything it needs without re-reading code or re-discovering state.

## How it works

**Two-tier generation:**

1. **Stop hook (re-trigger phase)** -- on the second stop attempt (`stop_hook_active=true`), the hook reads the database and writes a structured markdown file with: status, progress, spec/plan paths, left_off, next 3 tasks, key files, recent sessions, subtask progress. No LLM involved -- instant and free.

2. **Board-manager (enriched)** -- when dispatched after commits, the board-manager reads the mechanical handoff and appends synthesized sections: decisions & context, gotchas, and recommended approach. These enrichment sections are preserved across rewrites (the Stop hook extracts and re-appends them).

**Session start injection:**

The SessionStart hook reads the top in-progress feature's handoff file and injects its full contents as the system message. Other in-progress features get one-line pointers to their handoff files.

**Cleanup:**

The Stop hook deletes handoff files for features that are no longer in_progress.

## Gotchas

- The mechanical baseline is rewritten every session end, but agent-enriched sections (Decisions & Context, Gotchas, Recommended Approach) are extracted from the existing file and re-appended.
- If no handoff file exists (first session for a feature), the SessionStart hook falls back to listing features with left_off text.
- Handoff files are in `.docket/` which should be in `.gitignore`.

## Key files

- `internal/handoff/render.go` -- Render, WriteFile, CleanStale
- `cmd/docket/hook.go` -- Stop handler calls WriteFile; SessionStart reads handoff files
- `internal/store/store.go` -- HandoffData struct and GetHandoffData method
- `plugin/agents/board-manager.md` -- agent instructions for enriching handoff files
