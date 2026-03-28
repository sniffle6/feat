# Context Reset Orchestration

## Problem

When a Claude Code session ends and a new one starts, the agent loses nuance: decisions made, approaches rejected, gotchas discovered, and the specific state of in-progress work. The current `get_context` tool returns ~15 lines and the SessionStart hook lists feature names. This isn't enough for a cold-starting agent to be productive immediately.

Anthropic's harness design article recommends file-based structured handoff artifacts over in-memory summaries for agent session transitions.

## Design

### Handoff file

At session end, docket generates a markdown file at `.docket/handoff/<feature-id>.md` for each active (in_progress) feature. This file contains everything a fresh agent needs to cold-start.

**Location:** `.docket/handoff/<feature-id>.md`

### Two-tier generation

**Tier 1: Mechanical baseline (Stop hook)**

The `docket hook` Stop handler reads the database and templates a structured handoff file. No LLM involved — instant, free, deterministic. Runs every session end, even sessions with no commits.

Template:

```markdown
# Handoff: <feature title>

## Status
<status> | Progress: <done>/<total> | Updated: <timestamp>

## Left Off
<left_off field contents>

## Next Tasks
- [ ] <first uncompleted task item>
- [ ] <second uncompleted task item>
- [ ] <third uncompleted task item>

## Key Files
<key_files list, one per line>

## Recent Activity
<last 3 session summaries with commit hashes>

## Active Subtasks
<subtask name> [<done>/<total>]
<subtask name> [<done>/<total>]
```

**Tier 2: Agent-enriched (board-manager)**

When the board-manager agent is dispatched after commits, it reads the existing handoff file and appends these sections below the mechanical content (never replaces the mechanical sections — those are the source of truth for raw state):

```markdown
## Decisions & Context
<synthesized from session history - what was tried, what worked, what was rejected>

## Gotchas
<things the next session should watch out for>

## Recommended Approach
<what to do next and why>
```

The board-manager already has access to `get_full_context` and file reading tools, so it can synthesize across sessions.

### Consumption on session start

The SessionStart hook changes behavior:

1. Find the top in-progress feature (most recently updated).
2. If a handoff file exists for it, read the file and inject its full contents as the system message.
3. For any other in-progress features, inject a one-line pointer: `[docket] Handoff available: .docket/handoff/<id>.md`
4. If no handoff file exists, fall back to current behavior (list active features with left_off).

This gives the most likely workstream zero-friction full context. Other features are discoverable but don't waste tokens.

### File lifecycle

- **Created/overwritten:** Every session end by the Stop hook.
- **Enriched:** By board-manager when dispatched after commits.
- **Read:** By SessionStart hook at next session start.
- **Cleaned up:** When a feature is marked `done`, its handoff file is deleted.

## Key files

- `cmd/docket/hook.go` — Stop handler writes mechanical handoff; SessionStart handler reads and injects it
- `plugin/agents/board-manager.md` — Updated instructions to enrich handoff files
- `internal/store/store.go` — May need helper methods to gather handoff data efficiently

## Out of scope

- Handoff files for non-active features
- Cross-project handoffs
- LLM-generated mechanical handoffs (tier 1 stays deterministic)
