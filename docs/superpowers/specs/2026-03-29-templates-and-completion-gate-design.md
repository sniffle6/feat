# Feature Templates & Acceptance Criteria Gate

**Date:** 2026-03-29
**Status:** Approved
**Inspired by:** [steveklabnik/docket](https://github.com/steveklabnik/docket) — templates with structured sections and checkbox-based completion gating.

## Problem

Two gaps in docket's current feature tracking:

1. **Inconsistent feature structure.** When board-manager creates features, the subtask/item structure varies based on what Claude invents in the moment. There's no standard starting point for different kinds of work.

2. **No completion enforcement.** Features can be marked `done` with unchecked task items and open issues. Task items are meant to serve as acceptance criteria, but nothing enforces that — the gate is advisory at best.

## Feature 1: Feature Type & Templates

### Schema Change

Add `type TEXT NOT NULL DEFAULT ''` to the `features` table (new migration).

Valid values: `feature`, `bugfix`, `chore`, `spike`, or empty string (for existing/untyped features). No database CHECK constraint — validated in application code.

### Template Definitions

Hardcoded in Go, in a new file `internal/store/templates.go`.

```go
type TemplateSubtask struct {
    Title string
    Items []string
}

var FeatureTemplates = map[string][]TemplateSubtask{
    "feature": {
        {Title: "Planning", Items: []string{"Define acceptance criteria", "Identify key files"}},
        {Title: "Implementation", Items: []string{"Implement core logic", "Add tests"}},
        {Title: "Polish", Items: []string{"Update documentation", "Final review"}},
    },
    "bugfix": {
        {Title: "Investigation", Items: []string{"Reproduce the bug", "Identify root cause"}},
        {Title: "Fix", Items: []string{"Implement fix", "Add regression test"}},
    },
    "chore": {
        {Title: "Work", Items: []string{"Implement changes", "Verify no regressions"}},
    },
    "spike": {
        {Title: "Research", Items: []string{"Explore approaches", "Document findings"}},
    },
}
```

Templates are fire-and-forget: the type is stored for display/filtering, but has no behavioral effect after creation. Once subtasks exist, they're the source of truth.

### MCP Tool Change: `add_feature`

New optional parameter: `type` (string).

When provided:
1. Validate against known template keys. Reject unknown types with an error.
2. Store the type on the feature row.
3. Auto-create the corresponding subtasks and task items using the template.
4. Return the feature with its generated structure.

When omitted or empty: behavior unchanged, no subtasks generated.

### Dashboard

Show the feature type as a label/badge on the feature card. No other UI changes.

## Feature 2: Acceptance Criteria Gate

### Where It Lives

In `store.UpdateFeature()`, not in the MCP tool handler. This makes the check unavoidable regardless of how the update is triggered.

### Logic

When the incoming update sets `status = "done"`:

1. Count unchecked task items on non-archived subtasks.
2. Count open issues on the feature.
3. If either count > 0 and `Force` is not set on the update:
   - Return a structured error: `cannot mark feature "foo" as done: N unchecked task items, M open issues (use force=true to override)`
4. If `Force` is true, proceed with the update and auto-log a decision:
   - `approach`: `"Force-completed with N unchecked items, M open issues"`
   - `outcome`: `"accepted"`
   - `reason`: caller-provided `ForceReason`, or `"No reason given"` if empty.

### API Changes

**`FeatureUpdate` struct** gains two fields:

```go
type FeatureUpdate struct {
    // ...existing pointer fields...
    Force       *bool
    ForceReason *string
}
```

**`update_feature` MCP tool** gains two optional parameters:
- `force` (bool) — bypass the completion gate
- `force_reason` (string) — logged in the auto-created decision

### Edge Cases

- **Feature with zero subtasks/task items:** passes the gate (nothing to check).
- **Feature with only archived subtasks:** passes (archived items excluded from count).
- **Setting status to anything other than `done`:** no check runs.
- **`force` without `force_reason`:** allowed; decision logs "No reason given".
- **Feature already `done`, updating other fields:** no check (status isn't changing).

## What Doesn't Change

- **`import_plan`** — unchanged. If a typed feature has template-generated subtasks, importing a plan archives them (existing behavior).
- **`complete_task_item` / `complete_task_items`** — unchanged.
- **`add_subtask` / `add_task_items`** — unchanged. Templates don't prevent manual additions.
- **`quick_track`** — unchanged. No type field needed.
- **Status transitions other than `done`** — no gates on planned/in_progress/blocked/dev_complete.

## Key Files (will be touched)

- `internal/store/migrate.go` — new migration adding `type` column
- `internal/store/store.go` — `Feature` struct gets `Type` field, `FeatureUpdate` gets `Force`/`ForceReason`
- `internal/store/templates.go` — new file, template definitions and apply logic
- `internal/store/subtask.go` — query for unchecked item count (may already exist via `GetFeatureProgress`)
- `internal/mcp/tools.go` — `add_feature` gains `type`, `update_feature` gains `force`/`force_reason`
- `dashboard/index.html` — type badge display
