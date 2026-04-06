# Feature Synthesis

Compounds checkpoint observations into an evolving per-feature summary. Prevents context loss across sessions.

## What it does

After a session ends, the checkpoint worker collects all observations for the feature (summaries, gotchas, dead ends, blockers, decisions) and sends them to the Anthropic summarizer. The result is stored as a `synthesis` field on the feature card -- a 3-8 sentence narrative of everything a new session needs to know.

## Why it exists

Without synthesis, each session's observations only survive in the handoff file until the next session overwrites it. Previous sessions compress to one-line summaries. Gotchas and dead ends from sessions 1 through N-1 are effectively invisible. Synthesis compounds this knowledge so it persists.

## How it works

1. Session ends -- SessionEnd hook enqueues a `session_end` checkpoint job
2. Checkpoint worker processes the job, writes observations
3. Worker calls `synthesizeFeature()` -- queries last 50 observations for the feature
4. Sends observations to Anthropic API with synthesis prompt
5. Writes result to `features.synthesis` + sets `synthesis_obs_id` high water mark
6. Next session sees synthesis in `get_context` output and handoff file

Synthesis is best-effort -- if the LLM call fails, it logs and continues. The feature still has its raw observations. The `synthesis_obs_id` tracks which observations have been incorporated so synthesis only runs when new observations exist.

## Where synthesis appears

- `get_context` -- shown between "Left off" and "Progress"
- `get_full_context` -- included in JSON as `synthesis` field
- Handoff files -- `## Synthesis` section between `## Left Off` and `## Last Session`
- FTS5 search -- synthesis text is indexed and searchable

## Board lint

`lint_board` MCP tool runs pure SQL health checks (no LLM calls):

- **Stale features** -- in_progress with no work session activity in 7+ days (excludes features with open sessions)
- **Gate bypasses** -- done features with unchecked task items
- **Empty features** -- 3+ days old, no sessions, subtasks, or notes
- **Stuck dev_complete** -- dev_complete status for 7+ days

Returns "Board health: all clear" when nothing is flagged.

## Key files

- `internal/store/migrate.go` -- schemaV20 (synthesis columns + FTS5 triggers)
- `internal/store/store.go` -- Feature struct with Synthesis/SynthesisObsID fields
- `internal/store/checkpoint.go` -- GetObservationsForFeature, UpdateFeatureSynthesis
- `internal/store/lint.go` -- LintBoard, LintReport, LintFinding
- `internal/checkpoint/worker.go` -- synthesizeFeature method
- `internal/checkpoint/summarizer.go` -- SynthesizeInput/Output types, SummarizerBackend interface
- `internal/checkpoint/anthropic.go` -- Synthesize method (Anthropic Messages API)
- `internal/checkpoint/noop.go` -- no-op Synthesize (when no API key)
- `internal/mcp/tools_lint.go` -- lint_board MCP tool handler
- `internal/mcp/tools_feature.go` -- get_context synthesis display
- `internal/handoff/render.go` -- synthesis section in handoff files
