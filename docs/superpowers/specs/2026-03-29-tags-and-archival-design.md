# Tags & Filtering + Feature Archival — Design Spec

## Overview

Two additions to docket's feature tracking:
1. **Tags** — free-form string tags on features, with new-tag warnings against existing tags to reduce sprawl
2. **Archival** — `archived` status with auto-archive of old done features

## Tags

### Schema

New column on features table (schemaV8):

```sql
ALTER TABLE features ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
```

Stores a JSON array of strings: `["auth","frontend"]`.

### Store Layer

**Feature struct** — add `Tags []string` field (same pattern as `KeyFiles`).

**FeatureUpdate struct** — add `Tags *[]string` field.

**All feature scan sites** — add `tags` to SELECT and Scan (GetFeature, ListFeatures, GetReadyFeatures — same pattern as the `type` column addition).

**UpdateFeature** — add tags handling:
```go
if u.Tags != nil {
    t, _ := json.Marshal(*u.Tags)
    sets = append(sets, "tags = ?")
    args = append(args, string(t))
}
```

**New method: `GetKnownTags() ([]string, error)`** — queries all distinct tags across features:
```sql
SELECT tags FROM features WHERE tags != '[]'
```
Parses each JSON array, deduplicates, returns sorted list. No file — database is the source of truth.

**New method: `CheckNewTags(tags []string) []string`** — compares input tags against `GetKnownTags()`. Returns list of tags that don't exist yet. No fuzzy matching — exact comparison only.

### MCP Tool Changes

**`add_feature`** — new `tags` param (comma-separated string). After creating the feature, if tags are provided, set them. If any tags are new, include warning in response listing existing tags.

**`update_feature`** — new `tags` param (comma-separated string, replaces all tags). Same new-tag warning.

**`list_features`** — new `tag` param. Filters to features whose `tags` JSON array contains the given tag. SQL: `WHERE tags LIKE '%"auth"%'` (simple LIKE match on JSON text — good enough for string tags, no JSON1 extension needed).

**Response format for new-tag warning:**
```
Feature "my-feature" created.
Note: new tag(s) "frntend" added. Existing tags: auth, frontend, backend
```

This lets Claude or the user spot typos without any fuzzy logic.

### Dashboard

Tag pills on feature cards, styled like the type badge but with a distinct color (e.g., muted blue outline). No filter dropdown — tags are visible for context, filtering happens via MCP tools.

### Snippet Update

Add to the CLAUDE.md install snippet, after the `type` param mention:

```
Use `tags` param (comma-separated) on `add_feature`/`update_feature` to categorize work.
```

## Archival

### Status Extension

`archived` becomes a valid feature status. No new table, no new column — just a new allowed value in the status field.

### Completion Gate

The completion gate in `UpdateFeature` only fires for `status=done`. `archived` bypasses it — you can archive a feature regardless of unchecked items or open issues. This is intentional: archival is about hiding old work, not certifying completion.

### List/Query Behavior

**`list_features`** — excludes `archived` by default. To see archived features, pass `status=archived` explicitly. Implementation: when no status filter is provided, add `WHERE status != 'archived'`.

**`get_ready`** — already only returns `in_progress` and `planned`. No change needed.

**`get_feature`** — returns any feature regardless of status. No change.

**`get_context`** — same, returns any feature. No change.

### Auto-Archive

**Where:** SessionStart hook (`handleSessionStart` in `hook.go`).

**When:** After opening the store, before building the context message.

**Logic:**
1. Query features with `status='done'` and `updated_at < datetime('now', '-7 days')`
2. For each, call `UpdateFeature(id, FeatureUpdate{Status: &archived})`
3. If any were archived, prepend to systemMessage: `[docket] Auto-archived N features done >7 days: id1, id2`

**Hardcoded 7 days.** No config file. If this needs to be configurable later, add it then.

### Dashboard

Archived features hidden from the kanban board by default. Add a "Show archived" toggle that reveals them in a separate column or greyed-out state.

### Unarchive

No special tool. `update_feature(id="old-feature", status="planned")` moves it back. The snippet doesn't need to document this — it's obvious from the status model.

## What This Does NOT Include

- No tag CRUD tools (no `list_tags`, `delete_tag`, `rename_tag`)
- No fuzzy matching on tags
- No tag-based dashboard filtering
- No config file for auto-archive timing
- No archive_feature MCP tool (use update_feature)
- No auto-pruning of tag list

## Test Plan

### Tags
- `TestFeatureTagsField` — add feature, set tags, verify roundtrip through GetFeature
- `TestFeatureTagsInList` — set tags, verify they appear in ListFeatures
- `TestListFeaturesFilterByTag` — create features with different tags, filter by tag
- `TestGetKnownTags` — create features with various tags, verify deduplication and sorting
- `TestCheckNewTags` — verify new tags are detected, existing tags are not
- `TestTagsInUpdateFeature` — update tags, verify replacement

### Archival
- `TestArchivedStatus` — set status to archived, verify it persists
- `TestListFeaturesExcludesArchived` — create archived feature, verify list_features omits it by default
- `TestListFeaturesShowArchived` — pass status=archived, verify it appears
- `TestAutoArchive` — create done feature with old updated_at, run auto-archive logic, verify status changed
- `TestAutoArchiveSkipsRecent` — done feature updated today should not be archived
- `TestCompletionGateBypassForArchived` — feature with unchecked items can be archived

## Key Files (will be modified)

- `internal/store/migrate.go` — schemaV8
- `internal/store/store.go` — Feature/FeatureUpdate structs, scan sites, tag methods
- `internal/mcp/tools.go` — add_feature, update_feature, list_features param updates
- `internal/mcp/tools_feature.go` — handler updates
- `cmd/docket/hook.go` — auto-archive in SessionStart
- `dashboard/index.html` — tag pills, archived toggle
- `cmd/docket/update.go` — snippet update
